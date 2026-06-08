# Security Controls

## Password Hashing

Use Argon2id for password storage. Generate a random salt per password and store
encoded, versioned hashes so parameters can change over time.

The password package should make these operations explicit:

- hash password
- verify password
- identify whether a stored hash needs rehashing

Password verification should use constant-time comparison for derived hashes.
User-facing errors must stay generic.

### Server-side password policy

Enforce a minimum password length (12 characters, counted as runes) on the
server in `ValidatePassword`, and call it in every flow that accepts a
user-chosen password before hashing it. The HTML `minlength="12"` attribute is
only a browser convenience and is bypassed by any direct POST, so it cannot be
the only check. Validate the password *before* consuming any one-time token
(e.g. a reset token), so rejecting a weak password does not burn the user's
single-use link.

### Login timing and account enumeration

Password login must take roughly the same time whether or not the email maps to
an account. When there is no matching credential (or the credential/principal is
disabled), run a verification against a precomputed throwaway hash
(`VerifyDummyPassword`) before returning the generic error, so an attacker
cannot distinguish registered emails by measuring response latency.

## Sessions

Use opaque random session tokens in HTTP-only cookies. Store only token hashes
in JetStream.

Cookie rules:

- `HttpOnly` always
- `Secure` outside local development
- `SameSite=Lax`
- path `/`
- explicit expiry aligned with the server-side session expiry

Session validation must read JetStream auth session state and check:

- token hash exists
- session is not expired
- session is not revoked
- principal is not disabled
- principal auth was not invalidated after the session was issued

## Fetch Metadata CSRF Protection

Use Fetch Metadata request headers as the CSRF boundary for unsafe methods.

For `POST`, `PUT`, `PATCH`, and `DELETE`:

- allow same-origin requests
- allow same-site requests
- reject cross-site requests
- reject suspicious `Sec-Fetch-Mode` and `Sec-Fetch-Dest` combinations
- define a conservative fallback for clients without Fetch Metadata headers

`SameSite=Lax` cookies remain a backup layer, but the middleware should be the
primary CSRF control for form posts and Datastar mutations.

## Rate Limits

Rate-limit auth paths by both IP and account identifier where available.

Required limits:

- password login attempts
- OTP request attempts
- OTP verification attempts
- forgot-password requests
- reset-password submissions

Responses should stay generic. Rate limits should not reveal whether an account
exists.

Implementation notes:

- The limiter uses optimistic concurrency on a JetStream KV counter. Retries on
  a revision conflict must be **bounded** (a fixed small loop, never recursion),
  and only a conflict should be retried -- any other KV error is returned
  immediately. If the counter cannot be settled within the retry budget, fail
  **closed** (treat the request as not allowed).
- Service methods must return `ErrRateLimited` when over the limit, distinct from
  a real limiter error. Returning a nil error on the over-limit path hides the
  condition from callers and logging.
- The per-challenge OTP attempt counter must advance on every guess, including
  wrong ones. Persist the incremented counter with the same bounded
  optimistic-concurrency retry so concurrent guesses cannot share one revision
  and bypass the attempt cap.

## Account And Session Invalidation

Use `principals.auth_invalidated_at` from Postgres to invalidate all credentials
and JetStream sessions issued before a point in time. Use it after password
reset, suspected compromise, or admin disable actions.

Session revocation should update the JetStream session entry and support
targeted logout of one session. The scaffold only needs current-session logout
in the UI, but the state model should not block future session management.

## Audit And NATS

Record important auth events in NATS/JetStream. Postgres remains the source of
truth for identity and credentials; JetStream is the source of truth for
sessions, OTP challenges, reset tokens, and auth event streams.

Important event names:

- `auth.login.succeeded`
- `auth.login.failed`
- `auth.logout.succeeded`
- `auth.otp.requested`
- `auth.otp.verified`
- `auth.password_reset.requested`
- `auth.password_reset.completed`
- `auth.session.revoked`
- `auth.principal.invalidated`

---

**Previous:** [Auth Flows](03-auth-flows.md) · **Next:** [Implementation Phases](05-implementation-phases.md)
