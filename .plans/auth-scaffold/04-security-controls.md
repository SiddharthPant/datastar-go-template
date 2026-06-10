# Security Controls

## Notes From The WorkOS Go Authentication Guide

The session-auth recommendations adopted from
workos.com/blog/go-authentication-guide, and where each lands in this plan:

1. **Server-side sessions for server-rendered apps** — chosen for immediate
   revocation without JWT refresh complexity. Here: JetStream KV instead of
   the guide's Redis/Postgres backends.
2. **Cookie attributes**: `HttpOnly` always, `Secure` in production,
   `SameSite=Lax`, explicit lifetime (guide default: 7 days). Here: doc 10's
   cookie helpers, 7-day session lifetime.
3. **Regenerate the session token after login** (session fixation). Here:
   structurally enforced — cookies are only ever set from a freshly minted
   `crypto/rand` token at login; no pre-auth session exists to upgrade, and
   no client-supplied value is ever honored.
4. **Memory-hard password hashing** with explicit parameters (guide: bcrypt
   cost 12, or Argon2id ~64 MiB). Here: Argon2id via
   `golang.org/x/crypto/argon2` with a self-owned policy layer, parameters
   below.
5. **Generic auth errors** — "invalid email or password" for every failure
   mode. Here: doc 03 step 4 plus the timing-equalizing dummy verification.
6. **Rate-limit login** (guide baseline: 5 attempts/minute/IP; shared-store
   limiter for multi-instance). Here: JetStream-KV-backed limiter keyed by
   IP + email, which is multi-instance-safe by construction. `x/time/rate`
   was evaluated and rejected for this job — see the table below.
7. **Constant-time comparison** (`crypto/subtle`) for any secret comparison.
   Here: Argon2id verification compares keys with
   `subtle.ConstantTimeCompare`; session/reset tokens are compared by
   SHA-256-hash KV key lookup, which never branches on secret bytes.
8. **Structured auth logging** with `log/slog`: log user ID, IP, and event
   reason — never passwords, tokens, or session IDs. Here: the logging rules
   below.
9. **Security headers middleware** (nosniff, frame deny, HSTS, CSP, referrer
   policy). Here: `SecurityHeaders` middleware, with a Datastar-specific CSP
   adjustment.
10. **Form/body validation** server-side; HTML attributes are convenience
    only. Here: the server-side password policy below.

Dependency rule: standard library and `golang.org/x/*` reference packages
are acceptable; third-party libraries are not. An `x` package still has to
actually fulfill the requirement, though — suggested packages were evaluated
individually:

| Guide suggestion | Verdict |
| --- | --- |
| `alexedwards/scs` session manager | Third-party — replaced by a hand-rolled token + JetStream KV store (doc 10 `state.go`). |
| bcrypt / Argon2id | `golang.org/x/crypto/argon2` is allowed and used directly; the PHC encoding, verification, and policy layer stay self-owned (doc 09). |
| `gorilla/csrf` double-submit tokens | Third-party — replaced by Fetch Metadata middleware + `SameSite=Lax`. |
| `x/time/rate` per-IP limiter | Allowed but **does not fit**: `rate.Limiter` is a purely in-memory token bucket with no storage interface, so its state cannot live in or sync through NATS. Limits would silently reset on every restart and fragment across instances — exactly what the guide warns about when it says to use a shared store in production. The JetStream KV CAS counter stays. (`x/time/rate` remains a fine choice for purely in-process throttles, e.g. capping outbound SMTP sends, if that ever comes up.) |

## Password Hashing

Argon2id via `golang.org/x/crypto/argon2` — the Go team's reference
implementation, with assembly-accelerated BLAKE2b, already present in this
module's dependency graph. What stays self-owned is everything around it:
the `internal/auth` policy layer (doc 09):

- PHC-encoded hashes: `$argon2id$v=19$m=65536,t=3,p=4$<salt>$<key>` so
  parameters travel with the hash and can be tightened later without
  breaking old hashes
- parameters: RFC 9106's "uniformly safe" option — 64 MiB memory, 3
  iterations, 4 lanes, 16-byte salt from `crypto/rand`, 32-byte key
- verification recomputes with the *stored* parameters and compares with
  `subtle.ConstantTimeCompare`
- `PasswordNeedsRehash` reports a hash created with outdated parameters, so
  hashes upgrade on next successful login

Benchmark the hash on the target machine and aim for roughly 100–300 ms; if
it is far off, adjust memory/iterations in code (new hashes pick it up,
`PasswordNeedsRehash` migrates old ones).

Argon2id at these parameters allocates 64 MB per verification, so concurrent
logins spike memory roughly linearly. The rate limiter is the backstop; if
login is ever exposed to heavy public traffic, put a small semaphore around
hashing rather than lowering the parameters.

### Server-side password policy

Enforce a minimum password length (12 characters, counted as runes) on the
server in `ValidatePassword`, and call it in every flow that accepts a
user-chosen password before hashing it. The HTML `minlength="12"` attribute
is only a browser convenience and is bypassed by any direct POST. Validate
the password *before* consuming any one-time token (e.g. a reset token), so
rejecting a weak password does not burn the user's single-use link.

### Login timing and account enumeration

Password login must take roughly the same time whether or not the email maps
to an account. When there is no matching user (or the user is disabled), run
a verification against a precomputed throwaway hash (`VerifyDummyPassword`)
before returning the generic error, so an attacker cannot distinguish
registered emails by measuring response latency.

The same rule applies to flows that send email. Forgot-password requests
must send mail **asynchronously** and return the generic response
immediately: a handler that waits on SMTP makes the "email exists" response
a full SMTP round-trip slower than the "no account" response, which is just
as good an enumeration oracle as a missing hash computation. Mail failures
are logged server-side only.

## Sessions

Opaque random session tokens (32 bytes from `crypto/rand`, base64url) in
HTTP-only cookies. Only SHA-256 hashes of tokens are stored in JetStream —
the server can never produce a valid cookie from its own storage.

Cookie rules:

- `HttpOnly` always
- `Secure` outside local development
- `SameSite=Lax`
- path `/`
- `MaxAge` aligned with the 7-day server-side session expiry
- production hardening note: once the app is HTTPS-only, rename the cookie
  with the `__Host-` prefix (requires `Secure`, `Path=/`, no `Domain`) so
  subdomains and plain HTTP can never plant it

Session fixation: a session cookie is only ever issued from a brand-new
random token generated *after* credentials verify. The server never adopts,
renames, or upgrades a client-supplied cookie value.

Session validation per request:

- token hash exists in the `auth_sessions` bucket
- session is not expired (`expires_at` field is authoritative; the bucket
  TTL is only garbage collection)
- session is not revoked
- then one Postgres query loads the user and checks: user exists, not
  disabled, and `auth_invalidated_at` (if set) is not after the session's
  issue time

Logout revokes server-side state first, then clears the cookie — clearing
the cookie alone is not revocation.

Last-seen metadata is updated at most once per few minutes — a KV write per
request is amplification, and last-seen is best-effort data, not a security
control.

## Fetch Metadata CSRF Protection

Use Fetch Metadata request headers as the CSRF boundary for unsafe methods.

For `POST`, `PUT`, `PATCH`, and `DELETE`:

- allow same-origin requests
- allow same-site requests
- reject cross-site requests
- reject suspicious `Sec-Fetch-Mode` and `Sec-Fetch-Dest` combinations
- conservative fallback for clients without Fetch Metadata headers: allow
  only when `Origin` or `Referer` matches the request host, otherwise 403

`SameSite=Lax` cookies remain a backup layer, but the middleware is the
primary CSRF control for form posts and Datastar mutations. No CSRF token
machinery needed.

## Security Headers

A small middleware (doc 09) applied to every response:

- `X-Content-Type-Options: nosniff`
- `X-Frame-Options: DENY`
- `Referrer-Policy: strict-origin-when-cross-origin`
- `Content-Security-Policy` — start from
  `default-src 'self'; script-src 'self' 'unsafe-eval'`. Datastar evaluates
  expressions from `data-*` attributes, which requires `'unsafe-eval'`; a
  strict `default-src 'self'` alone breaks every signal and action. Verify
  the dashboard, hot reload (SSE), and stylesheets in the browser console
  after enabling, and adjust directives deliberately rather than deleting
  the header.
- `Strict-Transport-Security` in production only (HSTS on plain-HTTP local
  dev just causes pain)

## Rate Limits

Rate-limit auth paths by both IP and account identifier where available.

Required limits:

- login attempts (baseline: 5 per 10 minutes per IP+email; the guide's 5/min
  per IP is the floor, not the ceiling)
- forgot-password requests
- reset-password submissions

Responses stay generic. Rate limits must not reveal whether an account
exists.

Implementation notes:

- Every optimistic-concurrency read-modify-write against JetStream KV — the
  rate limit counter, session revocation, reset-token consumption — goes
  through a single generic helper (`CASUpdate` in `internal/auth`). Retries
  on a revision conflict are **bounded** (a fixed small loop, never
  recursion), only a conflict is retried — any other KV error returns
  immediately — and exhausting the retry budget fails **closed** (the
  request is treated as not allowed).
- Service methods return `ErrRateLimited` when over the limit, distinct from
  a real limiter error. Returning a nil error on the over-limit path hides
  the condition from callers and logging.
- IP-keyed limits use `RemoteAddr` only. This is a **known deferral**: it is
  correct while the app terminates its own connections, but behind a reverse
  proxy every limit key collapses to the proxy address. When a proxy enters
  the picture, parse `X-Forwarded-For` from the configured trusted proxy
  only — never unconditionally, or clients can spoof their way past IP
  limits.

## Auth Logging

Use `log/slog` for every auth event: login success/failure, logout, reset
requested/completed, rate-limit trips. Log user ID/PID, IP, and the reason —
**never** passwords, session tokens, reset tokens, or full cookie values.
The session PID exists exactly so logs can reference a session without
containing its token.

## Account And Session Invalidation

`users.disabled_at` blocks login and fails validation of existing sessions
on their next request. `users.auth_invalidated_at` invalidates every session
issued before that timestamp — set it after password reset, suspected
compromise, or admin action. Session revocation updates the JetStream entry
and supports targeted logout of one session; the scaffold only needs
current-session logout in the UI, but the state model should not block
future session management.

## Audit And NATS

Record important auth events in NATS/JetStream. Postgres remains the source
of truth for identity; JetStream is the source of truth for sessions, reset
tokens, rate limits, and the auth event stream.

Event names:

- `auth.login.succeeded`
- `auth.login.failed`
- `auth.logout.succeeded`
- `auth.password_reset.requested`
- `auth.password_reset.completed`
- `auth.session.revoked`

---

**Previous:** [Auth Flows](03-auth-flows.md) · **Next:** [Implementation Phases](05-implementation-phases.md)
