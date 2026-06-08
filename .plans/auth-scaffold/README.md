# Auth Scaffold Plan

Build a complete session-based auth system for this Go + templ + Datastar app,
with a richer identity model than a single `users` table. The target model uses
UUIDv7 for internal primary and foreign keys, and prefixed NanoID-style PIDs for
anything shown in UI, URLs, logs, support tools, or external references.

The identity model is built around principal records, user records, scoped
membership, prefixed public IDs, password credentials, and email OTP
credentials. It uses an org/team/member hierarchy with email/password and
email/OTP login.

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

The first usable milestone is still the dashboard flow: logged-out `/` redirects
to `/auth/login`, successful login redirects back to `/`, and the existing index
page becomes the authenticated dashboard with a logout button. Datastar should
be used from the first implementation pass for auth interactions, not added as a
later enhancement. The final milestone is a complete auth baseline with
JetStream-backed sessions and reset tokens, Argon2id password hashing, email OTP
login, forgot/reset password management pages, Fetch Metadata CSRF protection,
rate limits, idempotent org/team/user seeders, tests, and generated sqlc/templ
output kept in sync.
