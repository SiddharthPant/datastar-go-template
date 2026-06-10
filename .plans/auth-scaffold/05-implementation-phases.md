# Implementation Phases

## Phase 1: Database Simplification

- Rewrite `database/migrations/00002_auth_identity.sql` to create `teams`,
  `users`, and `team_memberships` plus the `user_role` enum (doc 08 §1). The
  committed version creates the old principal/scope/org schema; since this
  is a pre-production playground, edit the migration in place rather than
  stacking a corrective migration, then rebuild the local database
  (`task db:nuke` or the reset task).
- Replace `database/queries/seed.sql` and add `database/queries/auth.sql`
  (doc 08 §2–3).
- Regenerate sqlc output; `database/sqlc/models.go` should end up with
  `Team`, `User`, `TeamMembership`, and the `UserRole` enum — nothing else.

Done when migrations apply cleanly on an empty database and sqlc output is
regenerated.

Do not add Postgres tables for sessions or password reset tokens.

## Phase 2: Crypto And Token Primitives

- Promote `golang.org/x/crypto` to a direct dependency
  (`go get golang.org/x/crypto`) for the `argon2` package (doc 09 §3).
- Add `internal/auth/password.go`: PHC encode/decode, hash, constant-time
  verify, `ValidatePassword`, `PasswordNeedsRehash`, dummy verification.
- Add secure token generation and SHA-256 token hashing.
- Add PID generation matching the SQL helper's alphabet rules.
- Add the generic `CASUpdate` JetStream KV helper and the KV-backed rate
  limiter.
- Add JetStream bucket/stream setup for sessions, password resets, rate
  limits, and auth events.
- Add unit tests for password hashing (including a pinned golden hash),
  token hashing, and PIDs; KV tests run against the compose NATS (doc 11).

Done when crypto and KV helpers are covered by focused tests.

## Phase 3: Seed Data

- Rewrite `cmd/seed/main.go`: hardcoded-UUID teams, users, and memberships,
  all inserts `ON CONFLICT (id) DO UPDATE`, one transaction, passwords
  hashed with the real Argon2id helper from `SEED_PASSWORD`.
- Validate the member-one-team invariant before writing.
- Keep the production refusal check.

Done when an empty database can be migrated, seeded, and used to log in
without any HTTP setup requests, and a second seed run converges instead of
duplicating.

## Phase 4: Session Middleware

- Add session cookie helpers (HttpOnly, Secure-in-prod, SameSite=Lax, 7-day
  MaxAge).
- Add JetStream session creation, lookup, throttled touch, and revoke.
- Add middleware that loads the current user into request context and
  redirects logged-out users to `/auth/login` (SSE redirect for Datastar
  requests).

Done when unauthenticated `/` redirects and authenticated `/` renders the
current dashboard.

## Phase 5: Login And Logout

- Replace the stub login page with the email/password form, wired with
  Datastar for submission state and inline errors.
- Add the login handler following doc 03's sequence (rate limit → lookup →
  generic-error verify → fresh token → cookie → redirect).
- Add the logout handler and dashboard logout button.
- Add the role-aware access helpers (`TeamsForUser`, `CanAccessTeam`) so the
  dashboard can show the current user's teams.
- Add structured auth logging (no secrets).

Done when login creates a session, dashboard access works, and logout
revokes the session server-side.

## Phase 6: Forgot And Reset Password

- Add forgot-password request page and handler (async mail through Mailpit
  locally).
- Add reset-password page and handler: validate password first, consume the
  token atomically, update the hash, set `auth_invalidated_at`.
- Store only reset token hashes in JetStream.

Done when a user can reset a password and old sessions stop authenticating.

## Phase 7: CSRF, Headers, Rate Limits, And Audit

- Add Fetch Metadata CSRF middleware for unsafe methods.
- Add the security headers middleware; verify the dashboard, Datastar
  interactions, and hot reload still work with the CSP enabled.
- Add IP+email rate limits to login, forgot-password, and reset flows.
- Add auth event publishing to the `AUTH_EVENTS` stream.

Done when abusive auth attempts are throttled, cross-site unsafe requests
are rejected, headers are present on every response, and auth lifecycle
events are recorded.

## Phase 8: Verification Pass

- Run templ generation.
- Run sqlc generation and vet/diff checks.
- Run seed command idempotency checks.
- Run auth unit tests and handler/integration tests.
- Manually verify the browser flows if automated browser tests are not in
  place.

Done when verification is green or any skipped check is explicitly
documented.

---

**Previous:** [Security Controls](04-security-controls.md) · **Next:** [Testing And Completion](06-testing-and-completion.md)
