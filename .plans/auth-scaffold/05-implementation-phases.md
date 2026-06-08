# Implementation Phases

## Phase 1: Database Foundation

- Add Postgres extensions needed by the model: `pgcrypto`, `citext`, and UUIDv7
  support as required by the target Postgres version.
- Add `prefixed_nanoid`, `is_prefixed_pid`, and `update_updated_at_column`
  helpers.
- Add enum types for principal kind, scope kind, membership role, and auth
  method.
- Add tables for principals, users, scopes, orgs, teams, memberships, and
  credentials.
- Add sqlc queries for all auth lookups and mutations.

Done when migrations apply cleanly and sqlc output is regenerated.

Do not add Postgres tables for sessions, OTP challenges, or password reset
tokens.

## Phase 2: Crypto And Token Primitives

- Add Argon2id hash, verify, and rehash-needed helpers.
- Add secure token generation.
- Add token/code hashing helpers for sessions, password resets, and OTP codes.
- Add JetStream bucket/stream setup for auth sessions, OTP challenges, password
  reset tokens, and auth events.
- Add unit tests for password hashing and token hashing.

Done when crypto helpers are covered by focused tests.

## Phase 3: Seed Data

- Add a reusable seeding package for orgs, teams, users, credentials, and
  memberships.
- Add `cmd/seed` and `task db:seed`.
- Add a default local fixture with one org, two teams, and owner/admin/member
  users.
- Hash seeded passwords with the real Argon2id helper.
- Keep seeders idempotent and refuse to run in production-like environments by
  default.

Done when an empty database can be migrated, seeded, and used to log in without
creating orgs, teams, or users through HTTP requests.

## Phase 4: Session Middleware

- Add session cookie configuration.
- Add JetStream session creation, lookup, touch, revoke, and cleanup operations.
- Add middleware that loads the current session and principal into request
  context.
- Add middleware that redirects logged-out users from protected routes to
  `/auth/login`.

Done when unauthenticated `/` redirects and authenticated `/` renders the
current dashboard.

## Phase 5: Email/Password Login And Logout

- Add login page with email/password form, wired with Datastar for submission
  state and inline validation.
- Add password login handler.
- Add current-session logout handler.
- Add dashboard logout button.
- Keep invalid login errors generic.

Done when login creates a session, dashboard access works, and logout revokes
the session.

## Phase 6: Email OTP Login

- Add OTP request form and handler, wired with Datastar for status updates.
- Add OTP verification form and handler, wired with Datastar for inline success
  and failure states.
- Send OTP codes through the project mail path, with Mailpit working locally.
- Store OTP challenges in JetStream.
- Enforce expiry, attempt counts, consumed state, and generic responses.

Done when email/OTP can create a session without requiring a password.

## Phase 7: Forgot And Reset Password

- Add forgot-password request page and handler.
- Add reset-password page and handler.
- Send reset links through the project mail path.
- Store only reset token hashes in JetStream.
- Consume reset tokens atomically after use.
- Revoke existing sessions after reset.

Done when a user can reset a password and old sessions stop authenticating.

## Phase 8: CSRF, Rate Limits, And Audit

- Add Fetch Metadata CSRF middleware for unsafe methods.
- Add IP and account-keyed rate limits to login, OTP, forgot-password, and reset
  flows.
- Add auth event recording in NATS/JetStream.
- Add cleanup jobs or commands only where JetStream TTLs are not enough for
  expired sessions, OTP challenges, and reset tokens.

Done when abusive auth attempts are throttled, cross-site unsafe requests are
rejected, and auth lifecycle events are recorded.

## Phase 9: Verification Pass

- Run templ generation.
- Run sqlc generation and vet/diff checks.
- Run seed command idempotency checks.
- Run auth unit tests.
- Run auth handler/integration tests.
- Manually verify the browser flows if automated browser tests are not yet in
  place.

Done when verification is green or any skipped check is explicitly documented.

---

**Previous:** [Security Controls](04-security-controls.md) · **Next:** [Testing And Completion](06-testing-and-completion.md)
