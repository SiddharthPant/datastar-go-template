# Testing And Completion

## Required Tests

Crypto (`internal/auth`):

- Argon2id hash and verify round trip
- invalid password rejection
- a pinned golden PHC hash verifies (regression vector — generate it once
  after the crypto code lands and hardcode it in the test, so refactors of
  the encoding or the algorithm import cannot silently break verification of
  existing stored hashes)
- malformed/truncated PHC strings return `ErrInvalidHash`, not panics
- `PasswordNeedsRehash` flags a hash with outdated parameters
- `ValidatePassword` rejects passwords shorter than 12 runes and accepts 12+
- token hashing is stable and never equals the input
- PID generation produces the right prefix, length, and alphabet

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

JetStream-backed tests connect to the NATS from `compose.yml` (honoring
`NATS_URL`, defaulting to localhost) and `t.Skip` with a clear message when
it is unreachable. Each test uses a uniquely named bucket and deletes it in
cleanup. This avoids adding the embedded `nats-server` module as a
dependency; the cost is that these tests require `docker compose up nats`.
Treat a skip as a red flag locally — run them with the stack up before
calling auth work done.

Sessions:

- session creation stores the record under the token hash, never the raw
  token
- valid session lookup
- expired session rejection (inject `now` via the `StateStore.now` field)
- revoked session rejection
- disabled user rejection
- `auth_invalidated_at` after session issue time rejects the session
- logout clears the cookie and revokes the session

HTTP flows:

- logged-out `/` redirects to `/auth/login`
- login succeeds with valid credentials and sets an HttpOnly cookie
- login fails generically with invalid credentials
- login for an unknown email still runs a hash verification (no fast path
  that would leak account existence by timing)
- login returns the generic error when rate limited (`ErrRateLimited` logged
  server-side)
- forgot-password returns the generic response for known and unknown emails
- forgot-password responds without waiting on the mailer (a slow/blocking
  mailer must not delay the generic response — SMTP latency is an
  enumeration oracle, see doc 04)
- reset-password updates the password and old sessions stop validating
- reset-password rejects a too-short password without consuming the reset
  token (token still usable on resubmit)
- a reset token cannot be consumed twice
- dashboard Datastar endpoints require auth
- a logged-out Datastar request to a protected endpoint receives an SSE
  redirect event, not an HTML redirect
- authenticated `GET /auth/login` redirects to `/`

Roles and team access (against a seeded database):

- `TeamsForUser` returns all teams for an owner without any membership rows
- `TeamsForUser` returns exactly the joined teams for an admin and a member
- `CanAccessTeam` allows an owner on any team, allows an admin/member only on
  joined teams, and denies otherwise

Seeder:

- refuses production-like environments
- creates the default team/user/membership set (owner with no memberships,
  admin on two teams, member on one)
- rejects a seed definition that gives a `member` more than one team
- is idempotent when run twice (same IDs, same emails, no duplicates)
- seeded users can log in without any setup requests

Security middleware:

- cross-site unsafe requests are rejected by Fetch Metadata checks
- same-origin unsafe requests are allowed
- security headers are present on responses

Note for test authors: the Fetch Metadata fallback is deliberately
conservative — an unsafe request with no `Sec-Fetch-Site` header and no
same-host `Origin`/`Referer` gets a 403. Handler tests (and curl-style
manual checks) must set `Sec-Fetch-Site: same-origin` or a matching `Origin`
header on every POST, or they will be rejected by the middleware rather than
exercise the handler.

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
- The dashboard renders the existing index page content and logout button,
  and the demo controls still work with the CSP header enabled.
- Logout redirects to `/auth/login`; using the back button does not resurrect
  an authenticated page that can mutate anything.
- Forgot password sends a reset link to Mailpit locally.
- Reset password accepts a valid token and rejects reused tokens.
- Old sessions fail after password reset.
- A fresh local database can be migrated, seeded, and used immediately.

## Completion Criteria

The auth system is complete when:

- UUIDv7 is used for internal IDs and foreign keys; prefixed PIDs for
  UI/external references.
- The schema is exactly `teams` + `users` + `team_memberships`, with the
  three system roles resolving team access as doc 01 defines (owner → all,
  admin → joined, member → exactly one).
- `go.mod` gained no third-party dependencies for auth — only
  `golang.org/x/crypto` promoted to direct. Sessions, tokens, CSRF, and rate
  limiting are stdlib plus existing project deps.
- Passwords are stored only as PHC-encoded Argon2id hashes behind the
  self-owned policy layer.
- Session and reset tokens are stored only as SHA-256 hashes, in JetStream,
  not Postgres.
- Cookies are HttpOnly, SameSite=Lax, Secure in production, with a 7-day
  lifetime matching server-side expiry.
- Login errors are generic with equalized timing; reset/forgot flows do not
  leak account existence.
- Fetch Metadata CSRF protection and security headers guard every response.
- Auth flows are rate limited; limits fail closed.
- Existing sessions are revoked after password reset.
- Auth lifecycle events are recorded in JetStream and logged via slog
  without secrets.
- The seeder is deterministic, idempotent, and production-refusing.
- templ and sqlc generated files are up to date.
- Unit and handler/integration tests cover the auth boundary.

---

**Previous:** [Implementation Phases](05-implementation-phases.md) · **Next:** [Implementation Code Map](07-implementation-code-map.md)
