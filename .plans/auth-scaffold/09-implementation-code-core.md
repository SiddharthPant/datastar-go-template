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

func NewPID(prefix string) (string, error) {
	buf := make([]byte, pidSize)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("read random pid bytes: %w", err)
	}

	out := make([]byte, pidSize)
	for i, b := range buf {
		out[i] = pidAlphabet[int(b)%len(pidAlphabet)]
	}

	return prefix + "_" + string(out), nil
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

```go
package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"
)

type PasswordParams struct {
	Memory      uint32
	Iterations  uint32
	Parallelism uint8
	SaltLength  uint32
	KeyLength   uint32
}

var DefaultPasswordParams = PasswordParams{
	Memory:      64 * 1024,
	Iterations:  3,
	Parallelism: 2,
	SaltLength:  16,
	KeyLength:   32,
}

// MinPasswordLength is the minimum password length the server enforces.
//
// The login and reset forms also set minlength="12" in their HTML, but that is
// only a hint for real browsers: a direct POST (curl, a script, a tampered
// page, a non-browser client) bypasses it completely. The server must enforce
// the same rule itself, so that no flow can ever store a weak or empty
// password.
const MinPasswordLength = 12

// ErrPasswordTooShort is returned by ValidatePassword. It is a package-level
// sentinel so callers can detect it with errors.Is and show a specific,
// non-generic message ("password too short" is not sensitive information).
var ErrPasswordTooShort = fmt.Errorf("password must be at least %d characters", MinPasswordLength)

// ValidatePassword enforces the server-side password policy. Call it in EVERY
// flow that accepts a user-chosen password before hashing it -- password reset
// today, and any future registration or change-password flow -- and call it
// before consuming any one-time token, so a rejected password does not burn the
// user's reset link.
//
// We count runes rather than bytes so a 12-character password made of
// multi-byte characters is not wrongly rejected.
func ValidatePassword(password string) error {
	if utf8.RuneCountInString(password) < MinPasswordLength {
		return ErrPasswordTooShort
	}
	return nil
}

func HashPassword(password string) (string, error) {
	return HashPasswordWithParams(password, DefaultPasswordParams)
}

func HashPasswordWithParams(password string, params PasswordParams) (string, error) {
	salt := make([]byte, params.SaltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("read password salt: %w", err)
	}

	key := argon2.IDKey(
		[]byte(password),
		salt,
		params.Iterations,
		params.Memory,
		params.Parallelism,
		params.KeyLength,
	)

	b64Salt := base64.RawStdEncoding.EncodeToString(salt)
	b64Key := base64.RawStdEncoding.EncodeToString(key)
	return fmt.Sprintf(
		"$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s",
		params.Memory,
		params.Iterations,
		params.Parallelism,
		b64Salt,
		b64Key,
	), nil
}

func VerifyPassword(password, encoded string) (bool, error) {
	params, salt, expected, err := decodePasswordHash(encoded)
	if err != nil {
		return false, err
	}

	actual := argon2.IDKey(
		[]byte(password),
		salt,
		params.Iterations,
		params.Memory,
		params.Parallelism,
		uint32(len(expected)),
	)

	return subtle.ConstantTimeCompare(actual, expected) == 1, nil
}

// dummyHash holds a throwaway Argon2id hash built once on first use. It backs
// VerifyDummyPassword below.
var (
	dummyHashOnce sync.Once
	dummyHash     string
)

// VerifyDummyPassword runs the full Argon2id verification path against a
// throwaway hash and discards the result. Its only purpose is to BURN TIME.
//
// Why this exists: a password login for a real email runs one Argon2id
// computation (tens of milliseconds); a login for an email that has no account
// would otherwise return immediately with no hashing at all. That timing
// difference lets an attacker enumerate which emails are registered just by
// measuring response latency. Calling this on the "no such user" (and
// "credential disabled") branch makes both cases take roughly the same time.
//
// The dummy hash is computed once, lazily, using DefaultPasswordParams so its
// cost matches a real verification. If that one-time setup ever fails we leave
// dummyHash empty and this becomes a cheap no-op -- that only weakens the
// timing defense, it never blocks a login.
func VerifyDummyPassword(password string) {
	dummyHashOnce.Do(func() {
		buf := make([]byte, 32)
		if _, err := rand.Read(buf); err != nil {
			return
		}
		h, err := HashPassword(base64.RawStdEncoding.EncodeToString(buf))
		if err != nil {
			return
		}
		dummyHash = h
	})
	if dummyHash == "" {
		return
	}
	_, _ = VerifyPassword(password, dummyHash)
}

func PasswordNeedsRehash(encoded string) bool {
	params, _, _, err := decodePasswordHash(encoded)
	if err != nil {
		return true
	}
	return params.Memory != DefaultPasswordParams.Memory ||
		params.Iterations != DefaultPasswordParams.Iterations ||
		params.Parallelism != DefaultPasswordParams.Parallelism
}

func decodePasswordHash(encoded string) (PasswordParams, []byte, []byte, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 {
		return PasswordParams{}, nil, nil, errors.New("invalid password hash format")
	}
	if parts[1] != "argon2id" || parts[2] != "v=19" {
		return PasswordParams{}, nil, nil, errors.New("unsupported password hash")
	}

	params := PasswordParams{}
	for _, item := range strings.Split(parts[3], ",") {
		kv := strings.SplitN(item, "=", 2)
		if len(kv) != 2 {
			return PasswordParams{}, nil, nil, errors.New("invalid password params")
		}
		n, err := strconv.ParseUint(kv[1], 10, 32)
		if err != nil {
			return PasswordParams{}, nil, nil, fmt.Errorf("parse password param %q: %w", kv[0], err)
		}
		switch kv[0] {
		case "m":
			params.Memory = uint32(n)
		case "t":
			params.Iterations = uint32(n)
		case "p":
			params.Parallelism = uint8(n)
		}
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return PasswordParams{}, nil, nil, fmt.Errorf("decode password salt: %w", err)
	}
	key, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return PasswordParams{}, nil, nil, fmt.Errorf("decode password key: %w", err)
	}

	params.SaltLength = uint32(len(salt))
	params.KeyLength = uint32(len(key))
	return params, salt, key, nil
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

## 5. Add `internal/auth/rate_limiter.go`

```go
package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

// Allow records one hit against `key` and reports whether the caller is still
// under `limit` for the current `window`.
//
// The counter lives in JetStream KV and is updated with optimistic
// concurrency: we read the current value together with its revision number,
// compute the next value, and write it back only if the revision has not
// changed in the meantime. When two requests for the same key race, one write
// wins and the other gets a conflict; the loser re-reads and tries again.
//
// Two things matter here, and the first version of this code got both wrong by
// calling `Allow` recursively on any failure:
//
//  1. Retries must be BOUNDED. An unbounded retry (recursion or an infinite
//     loop) turns a permanent failure -- NATS down, context cancelled, a
//     malformed entry -- into a hang or a stack overflow.
//  2. Only a *revision conflict* should be retried. Any other error means the
//     KV operation itself failed and retrying it will just fail again, so we
//     return it straight away.
//
// `maxAttempts` is small because real contention resolves in one or two
// retries; if we somehow exhaust it we fail CLOSED (return `false`), so a
// pathologically hot key can never be used to slip past the limit.
func (l *RateLimiter) Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, error) {
	const maxAttempts = 5
	for attempt := 0; attempt < maxAttempts; attempt++ {
		now := l.now().UTC()
		entry, err := l.kv.Get(ctx, key)
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			// First hit in this window: create the counter at 1. If another
			// request created it first we lose the race -- re-read and retry.
			payload, err := json.Marshal(rateLimitEntry{Count: 1, ResetAt: now.Add(window)})
			if err != nil {
				return false, fmt.Errorf("encode rate limit: %w", err)
			}
			if _, err := l.kv.Create(ctx, key, payload); err != nil {
				if errors.Is(err, jetstream.ErrKeyExists) {
					continue
				}
				return false, fmt.Errorf("create rate limit: %w", err)
			}
			return true, nil
		}
		if err != nil {
			return false, fmt.Errorf("get rate limit: %w", err)
		}

		var current rateLimitEntry
		if err := json.Unmarshal(entry.Value(), &current); err != nil {
			return false, fmt.Errorf("decode rate limit: %w", err)
		}

		if now.After(current.ResetAt) {
			current = rateLimitEntry{Count: 1, ResetAt: now.Add(window)}
		} else {
			current.Count++
		}

		payload, err := json.Marshal(current)
		if err != nil {
			return false, fmt.Errorf("encode rate limit: %w", err)
		}
		if _, err := l.kv.Update(ctx, key, payload, entry.Revision()); err != nil {
			// A revision mismatch means a concurrent request updated the key
			// between our Get and Update -- re-read and retry. We retry on any
			// Update error here (rather than matching a specific conflict
			// sentinel) because the conflict error type differs across nats.go
			// versions; the bounded loop keeps that safe. If your nats.go
			// version exports a wrong-revision sentinel, prefer matching it and
			// returning other errors immediately.
			continue
		}

		return current.Count <= limit, nil
	}
	// Too much contention on this key to settle within maxAttempts. Fail closed.
	return false, nil
}
```

## 6. Add `natsx/auth.go`

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

## 7. Update `natsx/jetstream.go`

Call auth setup at the end of `EnsureStreams`:

```go
if err := c.EnsureAuthState(ctx); err != nil {
	return err
}
```

---

**Previous:** [Database And Queries Code](08-implementation-code-database.md) · **Next:** [Auth Feature And Routing Code](10-implementation-code-feature.md)
