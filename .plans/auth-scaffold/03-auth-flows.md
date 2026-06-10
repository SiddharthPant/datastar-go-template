# Auth Flows

## Dashboard Flow

- `GET /` checks the current session.
- If no valid session exists, redirect to `/auth/login`.
- If a valid session exists, render the existing index page as the dashboard.
- Add a logout button to the dashboard.
- `POST /auth/logout` revokes the session, clears the cookie, and redirects to
  `/auth/login`.

The existing demo controls on the index page should keep working after the page
is protected.

## Login Page

`GET /auth/login` redirects to `/` when the visitor already has a valid
session; otherwise it renders one login page with two supported methods:

- email/password
- email/OTP

There is no `?next=` return-to parameter yet — a logged-out deep link always
lands on `/` after login. Known deferral: add it when the app has more than
one protected destination, and validate it as a relative path to avoid an
open redirect.

Use Datastar from the first pass for login method switching, inline errors,
loading states, OTP request status, and OTP verification status. Successful auth
can still redirect to `/`, but the intermediate interactions should patch templ
fragments instead of waiting for a later JavaScript enhancement pass.

## Email And Password Login

- `POST /auth/login/password` accepts email and password.
- Look up the password credential by normalized `citext` email.
- Return a generic error for missing users, disabled credentials, disabled
  principals, or invalid passwords.
- Verify the Argon2id password hash.
- Create a new JetStream-backed session with `auth_method = 'password'`.
- Set the session cookie and redirect to `/`.

Do not create account enumeration differences in status codes, body copy, or
timing where it is reasonable to avoid them.

## Email OTP Login

- `POST /auth/login/otp/request` accepts email.
- If the email maps to an enabled OTP credential, create a JetStream-backed OTP
  challenge and send the code asynchronously (the handler must not wait on
  SMTP; see doc 04 on enumeration timing).
- Always render a generic "check your email" response.
- `POST /auth/login/otp/verify` accepts email and code.
- Verify the latest usable challenge hash, expiry, consumed state, and attempt
  count.
- Mark the challenge consumed.
- Create a new JetStream-backed session with `auth_method = 'email_otp'`.
- Set the session cookie and redirect to `/`.

OTP codes should be short-lived and single-use. Store only code hashes in
JetStream.

## Forgot Password And Reset Password

- `GET /auth/forgot-password` renders the reset request page.
- `POST /auth/forgot-password` accepts email.
- If a password credential exists, create a short-lived JetStream-backed reset
  token and send a reset link asynchronously (same SMTP-timing rule as OTP
  requests).
- Always render a generic response.
- `GET /auth/reset-password?token=...` validates the token enough to render the
  reset form.
- `POST /auth/reset-password` validates the token and new password.
- Store the new Argon2id password hash.
- Consume the reset token.
- Revoke existing sessions for that principal.
- Redirect to `/auth/login`.

Reset links should use opaque random tokens. Store only token hashes in
JetStream.

## Public And Protected Routes

Public:

- `GET /auth/login`
- `POST /auth/login/password`
- `POST /auth/login/otp/request`
- `POST /auth/login/otp/verify`
- `GET /auth/forgot-password`
- `POST /auth/forgot-password`
- `GET /auth/reset-password`
- `POST /auth/reset-password`
- static assets
- dev reload endpoints

Protected:

- `GET /`
- dashboard Datastar mutation endpoints such as `/increment`, `/nats/ping`, and
  `/jobs/demo`
- `POST /auth/logout`

Logout should be protected because it mutates the current authenticated session.

When auth fails on a protected route, the response channel must match the
client: a normal navigation gets a 303 to `/auth/login`, but a Datastar
request (`Datastar-Request: true`) gets an SSE redirect event — fetch would
follow a 303 into login-page HTML and fail silently instead of navigating.

---

**Previous:** [Seed Data](02-seed-data.md) · **Next:** [Security Controls](04-security-controls.md)
