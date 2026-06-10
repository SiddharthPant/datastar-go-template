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
)

// NewToken returns a URL-safe opaque token with the given entropy in bytes.
// 32 bytes is the project default for session and reset tokens.
func NewToken(bytes int) (string, error) {
	raw := make([]byte, bytes)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("read random token bytes: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

// TokenHash is the storage form of every token: KV entries are keyed by this
// hash, never by the raw token, so a KV dump contains nothing replayable.
// Lookups by hashed key also never branch on secret bytes, which makes a
// separate constant-time comparison unnecessary for tokens.
func TokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
```

## 3. Use `golang.org/x/crypto/argon2`

`golang.org/x/*` reference packages are within the project's dependency
policy, so the Argon2id core comes straight from the Go team's
implementation (battle-tested, assembly-accelerated BLAKE2b). It is already
in the module graph as an indirect dependency — promote it to direct:

```sh
go get golang.org/x/crypto
```

The policy layer below uses exactly two symbols from it:
`argon2.IDKey(password, salt []byte, time, memory uint32, threads uint8, keyLen uint32) []byte`
and `argon2.Version`. Everything else — PHC encoding/decoding, parameter
policy, constant-time verification, rehash detection, the dummy timing
equalizer — is self-owned in §4, because that is the part worth owning;
hand-rolling the memory-hard primitive itself is where self-reliance stops
paying off. (A pure-stdlib fallback exists if the dependency ever needs to
go: Go ≥ 1.24 ships `crypto/pbkdf2`, OWASP-acceptable at high iteration
counts but weaker than Argon2id against GPU attackers.)

## 4. Add `internal/auth/password.go`

The self-owned policy layer: parameters, PHC encoding/decoding, constant-time
verification, minimum length, rehash detection, and the timing-equalizing
dummy verification.

```go
package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"sync"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"
)

// RFC 9106 "uniformly safe" parameters: 64 MiB, 3 passes, 4 lanes.
// Benchmark on the deploy target and aim for ~100-300ms per hash; change the
// constants here and PasswordNeedsRehash migrates old hashes on next login.
const (
	argonMemoryKiB   uint32 = 64 * 1024
	argonIterations  uint32 = 3
	argonParallelism uint8  = 4
	argonSaltLength         = 16
	argonKeyLength   uint32 = 32
)

// MinPasswordLength is enforced server-side; the HTML minlength attribute is
// only a browser convenience and is bypassed by any direct POST.
const MinPasswordLength = 12

var (
	ErrPasswordTooShort = fmt.Errorf("password must be at least %d characters", MinPasswordLength)
	ErrInvalidHash      = errors.New("invalid password hash encoding")
)

// ValidatePassword enforces the server-side password policy. Call it in every
// flow that accepts a user-chosen password, before consuming any one-time
// token. Length is counted in runes, not bytes.
func ValidatePassword(password string) error {
	if utf8.RuneCountInString(password) < MinPasswordLength {
		return ErrPasswordTooShort
	}
	return nil
}

// HashPassword returns a PHC-encoded Argon2id hash:
// $argon2id$v=19$m=65536,t=3,p=4$<base64 salt>$<base64 key>
// Parameters travel with the hash, so they can be raised later without
// breaking verification of existing hashes.
func HashPassword(password string) (string, error) {
	salt := make([]byte, argonSaltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("read salt: %w", err)
	}
	key := argon2.IDKey([]byte(password), salt, argonIterations, argonMemoryKiB, argonParallelism, argonKeyLength)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemoryKiB, argonIterations, argonParallelism,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key),
	), nil
}

type hashParams struct {
	version     int
	memoryKiB   uint32
	iterations  uint32
	parallelism uint8
}

func decodeHash(encoded string) (hashParams, []byte, []byte, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" {
		return hashParams{}, nil, nil, ErrInvalidHash
	}
	var p hashParams
	if _, err := fmt.Sscanf(parts[2], "v=%d", &p.version); err != nil {
		return hashParams{}, nil, nil, ErrInvalidHash
	}
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &p.memoryKiB, &p.iterations, &p.parallelism); err != nil {
		return hashParams{}, nil, nil, ErrInvalidHash
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return hashParams{}, nil, nil, ErrInvalidHash
	}
	key, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return hashParams{}, nil, nil, ErrInvalidHash
	}
	if len(key) == 0 {
		return hashParams{}, nil, nil, ErrInvalidHash
	}
	return p, salt, key, nil
}

// VerifyPassword recomputes the hash with the parameters stored in encoded
// and compares in constant time.
func VerifyPassword(password, encoded string) (bool, error) {
	p, salt, key, err := decodeHash(encoded)
	if err != nil {
		return false, err
	}
	if p.version != argon2.Version {
		return false, ErrInvalidHash
	}
	got := argon2.IDKey([]byte(password), salt, p.iterations, p.memoryKiB, p.parallelism, uint32(len(key)))
	return subtle.ConstantTimeCompare(got, key) == 1, nil
}

// PasswordNeedsRehash reports whether a stored hash was created with outdated
// parameters and should be rehashed on the next successful verification.
func PasswordNeedsRehash(encoded string) bool {
	p, _, _, err := decodeHash(encoded)
	if err != nil {
		return true
	}
	return p.memoryKiB != argonMemoryKiB ||
		p.iterations != argonIterations ||
		p.parallelism != argonParallelism
}

var dummyHash = sync.OnceValue(func() string {
	hash, err := HashPassword("dummy-timing-equalizer")
	if err != nil {
		return ""
	}
	return hash
})

// VerifyDummyPassword burns the same time as a real verification. Call it on
// the no-account and disabled-user login paths so response latency does not
// reveal whether an email is registered.
func VerifyDummyPassword(password string) {
	if hash := dummyHash(); hash != "" {
		_, _ = VerifyPassword(password, hash)
	}
}
```

## 5. Add `internal/auth/fetch_metadata.go`

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

## 6. Add `internal/auth/security_headers.go`

The header set from the WorkOS guide, adjusted for Datastar: expression
evaluation in `data-*` attributes requires `'unsafe-eval'`, and HSTS is
production-only so plain-HTTP local dev keeps working.

```go
package auth

import (
	"net/http"

	"datastar-go/config"
)

// SecurityHeaders sets baseline security response headers. The CSP includes
// 'unsafe-eval' because Datastar evaluates expressions from data-*
// attributes; verify the dashboard and hot reload in the browser console
// after changing any directive.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-eval'; style-src 'self' 'unsafe-inline'")
		if config.Env.AppEnv == config.Prod {
			h.Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
		}
		next.ServeHTTP(w, r)
	})
}
```

If styles are all served from `/static`, try dropping `'unsafe-inline'` from
`style-src` and see whether anything breaks — tighten deliberately, loosen
reluctantly.

## 7. Add `internal/auth/kv.go`

Every optimistic-concurrency read-modify-write against JetStream KV — rate
limit counters, session revocation, reset-token consumption — goes through
this one helper, so the bounded-retry, conflicts-only, fail-closed invariant
is enforced in a single place.

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

## 8. Add `internal/auth/rate_limiter.go`

Why not `x/time/rate`: its `rate.Limiter` is a purely in-memory token bucket
with no storage interface — state cannot live in or sync through NATS, so
limits would reset on every restart and fragment across instances. This
counter keeps its state in the JetStream KV bucket, shared by every
instance, surviving restarts, and failing closed under contention.

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

## 9. Add `natsx/auth.go`

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
	AuthPasswordResetsBucket = "auth_password_resets"
	AuthRateLimitsBucket     = "auth_rate_limits"
)

type AuthStores struct {
	Sessions       jetstream.KeyValue
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

	// TTLs are garbage collection with headroom over the in-record expiry
	// fields, which are the authoritative checks (a touched session resets
	// its entry TTL, so the TTL is deliberately longer than the lifetime).
	for _, cfg := range []jetstream.KeyValueConfig{
		{Bucket: AuthSessionsBucket, TTL: 8 * 24 * time.Hour, Storage: jetstream.FileStorage, History: 5},
		{Bucket: AuthPasswordResetsBucket, TTL: time.Hour, Storage: jetstream.FileStorage, History: 5},
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
		PasswordResets: resets,
		RateLimits:     limits,
	}, nil
}
```

## 10. Update `natsx/jetstream.go`

Call auth setup at the end of `EnsureStreams`:

```go
if err := c.EnsureAuthState(ctx); err != nil {
	return err
}
```

---

**Previous:** [Database And Queries Code](08-implementation-code-database.md) · **Next:** [Auth Feature And Routing Code](10-implementation-code-feature.md)
