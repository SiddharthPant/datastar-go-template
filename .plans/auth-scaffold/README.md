# Auth Scaffold Plan

Build a simple, complete session-based auth system for this Go + templ +
Datastar app with **no third-party dependencies**. Allowed: the standard
library, `golang.org/x/*` reference packages (Go-team maintained), and the
core deps the project already uses (pgx/sqlc, chi, nats.go, templ,
datastar-go, google/uuid). Session management, tokens, CSRF, and rate
limiting are owned by this codebase; password hashing uses
`golang.org/x/crypto/argon2` under a self-owned policy layer.

The identity model is deliberately small:

- a `users` table with email, password hash, account state, and a
  system-wide `role`
- a `teams` table and a `team_memberships` join table — a user can belong to
  multiple teams
- three roles: **owner** (access to all teams, no membership rows needed),
  **admin** (access to the teams they are members of, one or more), and
  **member** (access to exactly one team)
- UUIDv7 internal primary keys, prefixed NanoID-style PIDs for anything
  shown in UI, URLs, or logs

Short-lived auth state (sessions, password reset tokens, rate limit
counters) lives in NATS JetStream KV, not Postgres — consistent with this
project's JetStream-first convention.

The session-auth design follows the recommendations in the WorkOS guide
"Session-based authentication in Go" (workos.com/blog/go-authentication-guide):
opaque tokens in HttpOnly/Secure/SameSite=Lax cookies, server-side session
state with immediate revocation, fresh tokens at login (session fixation),
generic auth errors, login rate limiting, security headers, and structured
auth logging that never includes secrets. Where the guide reaches for a
third-party library (`alexedwards/scs`, `gorilla/csrf`, bcrypt), this plan
substitutes a small self-owned equivalent; where it suggests an `x` package
that cannot meet a requirement (`x/time/rate` cannot share its in-memory
state through NATS), doc 04 records why and what is used instead.

Read the plan top to bottom in this order. Each document ends with a
`Previous` / `Next` footer, so you can read straight through without coming
back here.

Design and planning:

1. [Identity And Database Model](01-identity-and-database-model.md)
2. [Seed Data](02-seed-data.md)
3. [Auth Flows](03-auth-flows.md)
4. [Security Controls](04-security-controls.md)
5. [Implementation Phases](05-implementation-phases.md)
6. [Testing And Completion](06-testing-and-completion.md)

Build, file by file (start at the code map, which explains the prerequisites,
then work through the four code documents in order):

7. [Implementation Code Map](07-implementation-code-map.md)
   1. [Database And Queries Code](08-implementation-code-database.md)
   2. [Core Auth And JetStream Code](09-implementation-code-core.md)
   3. [Auth Feature And Routing Code](10-implementation-code-feature.md)
   4. [Seeders And Tests Code](11-implementation-code-seeders-tests.md)

The first usable milestone is the dashboard flow: logged-out `/` redirects to
`/auth/login`, successful login redirects back to `/`, and the existing index
page becomes the authenticated dashboard with a logout button. Datastar is
used from the first implementation pass for auth interactions, not added as a
later enhancement. The final milestone is a complete auth baseline:
JetStream-backed sessions and reset tokens (hash-only storage), Argon2id
password hashing behind a self-owned policy layer, role-aware team access
helpers, forgot/reset password pages, Fetch Metadata CSRF protection,
security headers, rate limits, an idempotent team/user/membership seeder,
tests, and generated sqlc/templ output kept in sync.

Deliberately out of scope for the scaffold (deferred, not forgotten):
self-registration, email OTP login, team management UI, per-team permission
levels beyond the three system roles, multi-session management UI, and a
`?next=` return-to parameter. Doc 03 lists what re-adding each one would
take.
