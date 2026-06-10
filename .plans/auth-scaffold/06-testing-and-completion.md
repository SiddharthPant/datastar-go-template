# Testing And Completion

## Required Tests

Crypto:

- Argon2id hash and verify
- invalid password rejection
- rehash-needed behavior
- `ValidatePassword` rejects passwords shorter than 12 runes and accepts 12+
- session token hashing
- OTP code hashing
- reset token hashing
- JetStream key naming for session, OTP, and reset state

KV CAS helper (`CASUpdate`):

- missing key without create returns not-found
- create path initializes from the zero value
- an apply error aborts without writing
- contention beyond the retry budget returns `ErrCASContention`
- a non-conflict KV error is returned, not retried

Rate limiter:

- counts hits and denies once over the limit within the window
- counter resets after the window elapses
- fails closed (denies) on `ErrCASContention`

All JetStream tests run against an embedded in-process `nats-server` (see doc
11) — no Docker dependency, no KV fakes.

Sessions:

- JetStream session creation
- valid JetStream session lookup
- expired session rejection
- revoked session rejection
- disabled principal rejection
- `auth_invalidated_at` rejection
- logout clears the cookie and revokes the session

HTTP flows:

- logged-out `/` redirects to `/auth/login`
- password login succeeds with valid credentials
- password login fails generically with invalid credentials
- password login for an unknown email still runs a hash verification (no fast
  path that would leak account existence by timing)
- OTP request returns generic response
- OTP request and forgot-password respond without waiting on the mailer (a
  slow/blocking mailer must not delay the generic response — SMTP latency is
  an enumeration oracle, see doc 04)
- OTP request returns ErrRateLimited (logged, generic response to user) when
  over the limit
- OTP verification succeeds with valid code
- OTP verification rejects expired, consumed, or over-attempted JetStream
  challenges
- the OTP attempt cap holds under concurrent wrong guesses (counter advances
  per guess, not per winning revision)
- forgot-password returns generic response
- reset-password updates the password and revokes sessions
- reset-password rejects a too-short password without consuming the reset token
  (token still usable on resubmit)
- dashboard Datastar endpoints require auth
- a logged-out Datastar request to a protected endpoint receives an SSE
  redirect event, not an HTML redirect
- authenticated `GET /auth/login` redirects to `/`

Seeders:

- seed command refuses production-like environments by default
- seed command creates the default org/team/user graph
- seed command is idempotent when run twice
- seeded passwords use the real Argon2id helper
- seeded users can log in without any setup requests

Security middleware:

- cross-site unsafe requests are rejected by Fetch Metadata checks
- same-origin unsafe requests are allowed
- login rate limits trigger
- OTP request and verify limits trigger
- reset limits trigger

Note for test authors: the Fetch Metadata fallback is deliberately
conservative — an unsafe request with no `Sec-Fetch-Site` header and no
same-host `Origin`/`Referer` gets a 403. Handler tests (and curl-style manual
checks) must set `Sec-Fetch-Site: same-origin` or a matching `Origin` header
on every POST, or they will be rejected by the middleware rather than exercise
the handler.

## Generated Output Checks

Run these whenever relevant inputs change:

```sh
task templ
task sqlc:generate
task sqlc:vet
task sqlc:diff
go test ./...
```

## Browser Acceptance Checks

- Visiting `/` while logged out redirects to `/auth/login`.
- Email/password login redirects to `/`.
- Email/OTP login redirects to `/`.
- The dashboard renders the existing index page content and logout button.
- Logout redirects to `/auth/login`.
- Forgot password sends a reset link to Mailpit locally.
- Reset password accepts a valid token and rejects reused tokens.
- Old sessions fail after password reset.
- A fresh local database can be migrated, seeded, and used immediately.

## Completion Criteria

The auth system is complete when:

- UUIDv7 is used for internal IDs and foreign keys.
- Prefixed NanoID PIDs are used for UI/external references.
- The org/team/member model exists and supports scoped memberships.
- Seeders can create orgs, teams, users, credentials, and memberships without
  HTTP setup requests.
- Users can authenticate with email/password.
- Users can authenticate with email/OTP.
- Passwords are stored only as Argon2id hashes.
- Session, OTP, and reset tokens are stored only as hashes.
- Sessions, OTP challenges, and password reset tokens are stored in JetStream,
  not Postgres tables.
- Fetch Metadata CSRF protection guards unsafe requests.
- Auth flows are rate limited without account enumeration.
- Forgot-password and reset-password pages work end to end.
- Existing sessions are revoked after password reset.
- Auth lifecycle events are recorded and ready for NATS/outbox publishing.
- templ and sqlc generated files are up to date.
- Unit and handler/integration tests cover the auth boundary.

---

**Previous:** [Implementation Phases](05-implementation-phases.md) · **Next:** [Implementation Code Map](07-implementation-code-map.md)
