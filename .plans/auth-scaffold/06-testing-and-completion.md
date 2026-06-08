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

Rate limiter:

- counts hits and denies once over the limit within the window
- counter resets after the window elapses
- a non-conflict KV error is returned, not retried
- fails closed (denies) if the counter cannot be written within the retry budget

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
