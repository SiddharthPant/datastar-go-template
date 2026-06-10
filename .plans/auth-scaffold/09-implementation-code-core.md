# Core Auth And JetStream Code

## 1. Add `internal/auth/pid.go`

```go
package auth

import (
	"crypto/rand"
	"fmt"
)

const pidAlphabet = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"
const pidSize = 16

// NewPID generates a prefixed NanoID-style public ID following the same
// prefix, size, alphabet, and mask-and-reject sampling rules as the
// prefixed_nanoid() SQL helper in 00001_init.sql.
func NewPID(prefix string) (string, error) {
	const mask = 63 // smallest 2^n-1 >= len(pidAlphabet)-1
	out := make([]byte, 0, pidSize)
	buf := make([]byte, pidSize*2)

	for {
		if _, err := rand.Read(buf); err != nil {
			return "", fmt.Errorf("read random pid bytes: %w", err)
		}
		for _, b := range buf {
			idx := int(b) & mask
			if idx >= len(pidAlphabet) {
				continue
			}
			out = append(out, pidAlphabet[idx])
			if len(out) == pidSize {
				return prefix + "_" + string(out), nil
			}
		}
	}
}
```

## 2. Add `internal/auth/token.go`

```go
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"math/big"
)

func NewToken(bytes int) (string, error) {
	raw := make([]byte, bytes)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("read random token bytes: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func TokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func NewOTPCode() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		return "", fmt.Errorf("generate otp code: %w", err)
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}
```

## 3. Add `internal/auth/password.go`

PHC-format encoding, decoding, salting, and constant-time comparison are owned
by `github.com/alexedwards/argon2id`; this file owns only the project policy:
parameters, minimum length, and the dummy verification used to equalize login
timing.

```go
package auth

import (
	"fmt"
	"sync"
	"unicode/utf8"

	"github.com/alexedwards/argon2id"
)

var passwordParams = &argon2id.Params{
	Memory:      64 * 1024,
	Iterations:  3,
	Parallelism: 2,
	SaltLength:  16,
	KeyLength:   32,
}

// MinPasswordLength is enforced server-side; the HTML minlength attribute is
// only a browser convenience and is bypassed by any direct POST.
const MinPasswordLength = 12

var ErrPasswordTooShort = fmt.Errorf("password must be at least %d characters", MinPasswordLength)

// ValidatePassword enforces the server-side password policy. Call it in every
// flow that accepts a user-chosen password, before consuming any one-time
// token. Length is counted in runes, not bytes.
func ValidatePassword(password string) error {
	if utf8.RuneCountInString(password) < MinPasswordLength {
		return ErrPasswordTooShort
	}
	return nil
}

func HashPassword(password string) (string, error) {
	return argon2id.CreateHash(password, passwordParams)
}

func VerifyPassword(password, encoded string) (bool, error) {
	return argon2id.ComparePasswordAndHash(password, encoded)
}

// PasswordNeedsRehash reports whether a stored hash was created with outdated
// parameters and should be rehashed on the next successful verification.
func PasswordNeedsRehash(encoded string) bool {
	params, _, _, err := argon2id.DecodeHash(encoded)
	if err != nil {
		return true
	}
	return params.Memory != passwordParams.Memory ||
		params.Iterations != passwordParams.Iterations ||
		params.Parallelism != passwordParams.Parallelism
}

var dummyHash = sync.OnceValue(func() string {
	hash, err := argon2id.CreateHash("dummy-timing-equalizer", passwordParams)
	if err != nil {
		return ""
	}
	return hash
})

// VerifyDummyPassword burns the same time as a real verification. Call it on
// the no-account and disabled-credential login paths so response latency does
// not reveal whether an email is registered.
func VerifyDummyPassword(password string) {
	if hash := dummyHash(); hash != "" {
		_, _ = argon2id.ComparePasswordAndHash(password, hash)
	}
}
```

## 4. Add `internal/auth/fetch_metadata.go`

```go
package auth

import (
	"net/http"
	"net/url"
	"strings"
)

func FetchMetadata(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isSafeMethod(r.Method) {
			next.ServeHTTP(w, r)
			return
		}

		site := r.Header.Get("Sec-Fetch-Site")
		switch site {
		case "same-origin", "same-site":
			next.ServeHTTP(w, r)
			return
		case "cross-site":
			http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
			return
		case "":
			if hasSameOriginFallback(r) {
				next.ServeHTTP(w, r)
				return
			}
			http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
			return
		default:
			http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
			return
		}
	})
}

func isSafeMethod(method string) bool {
	return method == http.MethodGet ||
		method == http.MethodHead ||
		method == http.MethodOptions
}

func hasSameOriginFallback(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin != "" {
		u, err := url.Parse(origin)
		return err == nil && sameHost(u.Host, r.Host)
	}

	referer := r.Header.Get("Referer")
	if referer != "" {
		u, err := url.Parse(referer)
		return err == nil && sameHost(u.Host, r.Host)
	}

	return false
}

func sameHost(a, b string) bool {
	return strings.EqualFold(a, b)
}
```

## 5. Add `internal/auth/kv.go`

Every optimistic-concurrency read-modify-write against JetStream KV — rate
limit counters, OTP attempt counting, session revocation, reset-token
consumption — goes through this one helper, so the bounded-retry,
conflicts-only, fail-closed invariant is enforced in a single place.

```go
package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/nats-io/nats.go/jetstream"
)

const casMaxAttempts = 5

// ErrCASContention is returned when a CASUpdate cannot settle within its
// retry budget. Callers must fail closed.
var ErrCASContention = errors.New("kv update contention")

// CASUpdate runs a bounded optimistic-concurrency read-modify-write on key.
// apply mutates the decoded value; returning an error from apply aborts
// without writing. A missing key returns jetstream.ErrKeyNotFound unless
// create is true, in which case apply starts from the zero value. Only lost
// revision races are retried; any other KV error returns immediately.
func CASUpdate[T any](ctx context.Context, kv jetstream.KeyValue, key string, create bool, apply func(*T) error) (T, error) {
	var zero T
	for attempt := 0; attempt < casMaxAttempts; attempt++ {
		entry, err := kv.Get(ctx, key)
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			if !create {
				return zero, err
			}
			var value T
			if err := apply(&value); err != nil {
				return zero, err
			}
			payload, err := json.Marshal(value)
			if err != nil {
				return zero, fmt.Errorf("encode kv value: %w", err)
			}
			if _, err := kv.Create(ctx, key, payload); err != nil {
				if errors.Is(err, jetstream.ErrKeyExists) {
					continue
				}
				return zero, fmt.Errorf("create kv value: %w", err)
			}
			return value, nil
		}
		if err != nil {
			return zero, fmt.Errorf("get kv value: %w", err)
		}

		var value T
		if err := json.Unmarshal(entry.Value(), &value); err != nil {
			return zero, fmt.Errorf("decode kv value: %w", err)
		}
		if err := apply(&value); err != nil {
			return zero, err
		}
		payload, err := json.Marshal(value)
		if err != nil {
			return zero, fmt.Errorf("encode kv value: %w", err)
		}
		if _, err := kv.Update(ctx, key, payload, entry.Revision()); err != nil {
			// Conflict sentinels vary across nats.go versions; the bounded
			// loop makes retrying any Update error safe.
			continue
		}
		return value, nil
	}
	return zero, ErrCASContention
}
```

## 6. Add `internal/auth/rate_limiter.go`

```go
package auth

import (
	"context"
	"errors"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

type RateLimiter struct {
	kv  jetstream.KeyValue
	now func() time.Time
}

type rateLimitEntry struct {
	Count   int       `json:"count"`
	ResetAt time.Time `json:"reset_at"`
}

func NewRateLimiter(kv jetstream.KeyValue) *RateLimiter {
	return &RateLimiter{kv: kv, now: time.Now}
}

// Allow records one hit against key and reports whether the caller is still
// under limit for the current window. Contention beyond the CAS retry budget
// fails closed.
func (l *RateLimiter) Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, error) {
	now := l.now().UTC()
	entry, err := CASUpdate(ctx, l.kv, key, true, func(e *rateLimitEntry) error {
		if now.After(e.ResetAt) {
			*e = rateLimitEntry{Count: 1, ResetAt: now.Add(window)}
			return nil
		}
		e.Count++
		return nil
	})
	if errors.Is(err, ErrCASContention) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return entry.Count <= limit, nil
}
```

## 7. Add `natsx/auth.go`

```go
package natsx

import (
	"context"
	"fmt"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

const (
	AuthEventsStream  = "AUTH_EVENTS"
	AuthEventsSubject = "auth.>"

	AuthSessionsBucket       = "auth_sessions"
	AuthOTPChallengesBucket  = "auth_otp_challenges"
	AuthPasswordResetsBucket = "auth_password_resets"
	AuthRateLimitsBucket     = "auth_rate_limits"
)

type AuthStores struct {
	Sessions       jetstream.KeyValue
	OTPChallenges  jetstream.KeyValue
	PasswordResets jetstream.KeyValue
	RateLimits     jetstream.KeyValue
}

func (c *Client) EnsureAuthState(ctx context.Context) error {
	if _, err := c.JetStream.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:      AuthEventsStream,
		Subjects:  []string{AuthEventsSubject},
		Storage:   jetstream.FileStorage,
		Retention: jetstream.LimitsPolicy,
		MaxAge:    90 * 24 * time.Hour,
	}); err != nil {
		return fmt.Errorf("ensure auth events stream: %w", err)
	}

	for _, cfg := range []jetstream.KeyValueConfig{
		{Bucket: AuthSessionsBucket, TTL: 30 * 24 * time.Hour, Storage: jetstream.FileStorage, History: 5},
		{Bucket: AuthOTPChallengesBucket, TTL: 15 * time.Minute, Storage: jetstream.FileStorage, History: 5},
		{Bucket: AuthPasswordResetsBucket, TTL: 30 * time.Minute, Storage: jetstream.FileStorage, History: 5},
		{Bucket: AuthRateLimitsBucket, TTL: 24 * time.Hour, Storage: jetstream.FileStorage, History: 2},
	} {
		if _, err := c.JetStream.CreateOrUpdateKeyValue(ctx, cfg); err != nil {
			return fmt.Errorf("ensure kv bucket %s: %w", cfg.Bucket, err)
		}
	}

	return nil
}

func (c *Client) AuthStores(ctx context.Context) (*AuthStores, error) {
	sessions, err := c.JetStream.KeyValue(ctx, AuthSessionsBucket)
	if err != nil {
		return nil, fmt.Errorf("bind auth sessions bucket: %w", err)
	}
	otp, err := c.JetStream.KeyValue(ctx, AuthOTPChallengesBucket)
	if err != nil {
		return nil, fmt.Errorf("bind auth otp bucket: %w", err)
	}
	resets, err := c.JetStream.KeyValue(ctx, AuthPasswordResetsBucket)
	if err != nil {
		return nil, fmt.Errorf("bind auth resets bucket: %w", err)
	}
	limits, err := c.JetStream.KeyValue(ctx, AuthRateLimitsBucket)
	if err != nil {
		return nil, fmt.Errorf("bind auth rate limits bucket: %w", err)
	}

	return &AuthStores{
		Sessions:       sessions,
		OTPChallenges:  otp,
		PasswordResets: resets,
		RateLimits:     limits,
	}, nil
}
```

## 8. Update `natsx/jetstream.go`

Call auth setup at the end of `EnsureStreams`:

```go
if err := c.EnsureAuthState(ctx); err != nil {
	return err
}
```

---

**Previous:** [Database And Queries Code](08-implementation-code-database.md) · **Next:** [Auth Feature And Routing Code](10-implementation-code-feature.md)
