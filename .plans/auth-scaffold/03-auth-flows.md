# Auth Flows

Exactly three flows: login, logout, and forgot/reset password. No
self-registration (users come from the seeder for now), no OTP login.

## Dashboard Flow

- `GET /` checks the current session.
- If no valid session exists, redirect to `/auth/login`.
- If a valid session exists, render the existing index page as the dashboard.
- Add a logout button to the dashboard.
- `POST /auth/logout` revokes the session server-side, clears the cookie, and
  redirects to `/auth/login`.

The existing demo controls on the index page keep working after the page is
protected.

## Login

`GET /auth/login` redirects to `/` when the visitor already has a valid
session; otherwise it renders a single email/password form.

There is no `?next=` return-to parameter yet — a logged-out deep link always
lands on `/` after login. Known deferral: add it when the app has more than
one protected destination, and validate it as a relative path to avoid an
open redirect.

Use Datastar from the first pass for form submission, inline errors, and
loading states. Successful auth redirects to `/`; failures patch the login
panel fragment instead of reloading the page.

`POST /auth/login` (the WorkOS-guide login sequence, adapted to forms + SSE):

1. Parse the form; reject malformed requests with 400.
2. Rate-limit by IP + email (doc 04). Over-limit returns the same generic
   error copy as a bad password.
3. Look up the user by normalized `citext` email.
4. On any failure — unknown email, disabled user, wrong password — return one
   generic "Invalid email or password." error. No status, copy, or timing
   difference between the cases (doc 04 covers the timing-equalizing dummy
   verification).
5. Verify the Argon2id hash with constant-time comparison.
6. Mint a **fresh random session token** (never derived from or reusing any
   client-supplied value — this is the session-fixation control; see doc 04),
   store its hash in JetStream, set the cookie, log the auth event, and
   redirect to `/`.

## Forgot Password And Reset Password

- `GET /auth/forgot-password` renders the reset request page.
- `POST /auth/forgot-password` accepts email.
- If a user exists and is not disabled, create a short-lived JetStream-backed
  reset token and send the link asynchronously — the handler must not wait on
  SMTP (doc 04 explains the enumeration-timing reason).
- Always render the same generic "check your email" response.
- `GET /auth/reset-password?token=...` renders the reset form with the token
  embedded.
- `POST /auth/reset-password` validates the new password **before** consuming
  the single-use token (a rejected weak password must not burn the link),
  then atomically consumes the token, stores the new Argon2id hash, sets
  `auth_invalidated_at = now()` so all existing sessions die, and redirects
  to `/auth/login`.

Reset links use opaque random tokens; only token hashes are stored in
JetStream.

## Public And Protected Routes

Public:

- `GET /auth/login`
- `POST /auth/login`
- `GET /auth/forgot-password`
- `POST /auth/forgot-password`
- `GET /auth/reset-password`
- `POST /auth/reset-password`
- static assets
- dev reload endpoints

Protected:

- `GET /`
- dashboard Datastar mutation endpoints such as `/increment`, `/nats/ping`,
  and `/jobs/demo`
- `POST /auth/logout`

Logout is protected because it mutates the current authenticated session.

When auth fails on a protected route, the response channel must match the
client: a normal navigation gets a 303 to `/auth/login`, but a Datastar
request (`Datastar-Request: true`) gets an SSE redirect event — fetch would
follow a 303 into login-page HTML and fail silently instead of navigating.

## Deferred Flows

- **Self-registration** — when added: validate email shape and password
  policy server-side, hash before insert, and respond to duplicate emails
  with the same generic copy as success (enumeration).
- **Email OTP login** — needs no schema change: challenges are JetStream KV
  entries keyed by email, storing only the code hash, expiry, consumed flag,
  and an attempt counter advanced with revision-checked updates.
- **`?next=` return-to** — validate as a relative path only.
- **Session management UI** (list/revoke other sessions) — the session
  record already carries IP/user-agent/last-seen, so this is purely additive.

---

**Previous:** [Seed Data](02-seed-data.md) · **Next:** [Security Controls](04-security-controls.md)
