# Complete Session Auth Plan

## Goal

Build a complete session-based auth system around the existing app.
Unauthenticated users who hit `/` should be redirected to `/auth/login`.
Successful login should create a server-side session and redirect back to `/`.
The existing index page should become the authenticated dashboard and keep its
current demo controls, with a logout button added.

Complete means the scaffold is not just a demo login. It must include secure
password storage with Argon2id, server-side sessions, logout, Fetch Metadata
based CSRF protection, rate limits, forgot-password and reset-password
management pages, tests, and enough operational hooks to reuse this as the auth
baseline for future projects.

## Route Flow

- `GET /` checks the current session.
- If no valid session exists, redirect to `/auth/login`.
- If a valid session exists, render the current index/dashboard page.
- `GET /auth/login` renders the login page.
- `POST /auth/login` validates credentials, creates a session, sets the session
  cookie, and redirects to `/`.
- `POST /auth/logout` deletes the session, clears the cookie, and redirects to
  `/auth/login`.
- `GET /auth/forgot-password` renders the password reset request page.
- `POST /auth/forgot-password` creates a single-use reset token and sends reset
  instructions when the submitted account exists.
- `GET /auth/reset-password` validates the reset token and renders the reset
  password page.
- `POST /auth/reset-password` validates the token, updates the password, revokes
  existing sessions, and redirects to `/auth/login`.

## Session Model

Use an opaque, random session token in an HTTP-only cookie. Store only a hashed
version server-side, with user identity, creation time, expiry time, and optional
revocation metadata. Keep the cookie `Secure` outside local development,
`HttpOnly` always, and `SameSite=Lax` unless a future flow needs otherwise.

Rotate the session on login, revoke it on logout, expire it server-side, and
make reuse of expired or revoked cookies fail closed. Store a hash of the token,
not the token itself, so a database leak does not immediately become live
session replay.

## Password Model

Store passwords with Argon2id using a random per-password salt and versioned
encoded hashes. Keep parameters explicit in code so they can be raised later.
Password verification must use constant-time comparison for derived hashes and
must return generic login/reset errors so account existence is not leaked.

## CSRF Model

Protect all unsafe methods with Fetch Metadata HTTP headers. Reject cross-site
mutating requests using `Sec-Fetch-Site`, allow same-origin and same-site
requests, and define a conservative fallback for clients that do not send these
headers. Keep `SameSite=Lax` cookies as a second layer, but make Fetch Metadata
the primary CSRF boundary for form posts.

## Phase 1: Data And Crypto Foundation

- Add migrations for users, sessions, and password reset tokens.
- Add sqlc queries for user lookup, password hash updates, session creation,
  session lookup, session revocation, reset token creation, reset token lookup,
  token consumption, and cleanup.
- Add Argon2id hash and verify helpers with encoded, versioned password hashes.
- Add secure random token generation and token hashing helpers for sessions and
  password resets.
- Add local bootstrap strategy for the first user without building public
  registration.

Phase 1 is complete when credentials, sessions, and reset tokens have durable
tables and the crypto helpers have focused tests.

## Phase 2: Login, Session, Dashboard, Logout

- Add an `auth` feature with login/logout handlers, templ pages, and a service
  that owns credential validation and session lifecycle.
- Add auth middleware that loads the current session from the cookie and stores
  the current user/session in request context.
- Move `/` behind the auth middleware and leave `/auth/login`, static assets,
  and dev reload endpoints public.
- Add a logout button to the current index page as a form that posts to
  `/auth/logout`.
- Ensure successful login redirects to `/` and invalid login returns the login
  page with a generic error.

Phase 2 is complete when the dashboard flow works end to end:

- Visiting `/` while logged out redirects to `/auth/login`.
- Submitting valid credentials at `/auth/login` redirects to `/`.
- Visiting `/` while logged in renders the existing index/dashboard UI.
- Clicking logout clears the session and redirects to `/auth/login`.
- Reusing a cleared or expired session cookie does not authenticate the user.

## Phase 3: CSRF And Rate Limits

- Add Fetch Metadata middleware for unsafe methods.
- Exempt only routes that are intentionally public and safe to exempt; document
  every exemption near the middleware.
- Add login rate limits by IP and account identifier.
- Add password reset request limits by IP and account identifier.
- Add reset token attempt limits to prevent token guessing.
- Make rate-limit responses generic and avoid account enumeration.

Phase 3 is complete when cross-site form posts are rejected, same-origin form
posts still work, and abusive login/reset attempts are throttled.

## Phase 4: Forgot Password And Reset Password

- Add `GET /auth/forgot-password` and `POST /auth/forgot-password`.
- Add `GET /auth/reset-password` and `POST /auth/reset-password`.
- Send reset links through the project mail path, with Mailpit working locally.
- Store only hashed reset tokens server-side.
- Make reset tokens single-use and short-lived.
- Revoke existing sessions after a successful password reset.
- Always return generic forgot-password responses so account existence is not
  exposed.

Phase 4 is complete when a user can request a reset, receive a link locally,
set a new password, have old sessions revoked, and log in with the new password.

## Phase 5: Tests And Verification

- Add unit tests for password hashing, token generation, token hashing, session
  expiry, reset token expiry, and generic error behavior.
- Add handler or integration tests for login, logout, dashboard redirect,
  forgot-password, reset-password, CSRF rejection, and rate limiting.
- Add sqlc/templ generation checks to the normal verification path.
- Run the narrowest meaningful verification after each phase and the full auth
  verification before calling the system complete.

Phase 5 is complete when auth behavior is covered by tests and generated files
are up to date.

## Completion Criteria

The auth system is complete when all phases are done and these checks pass:

- Login, logout, dashboard access, forgot password, and reset password work from
  the browser.
- Passwords are stored only as Argon2id hashes.
- Session and reset tokens are stored only as hashes.
- Unsafe requests are protected by Fetch Metadata CSRF middleware.
- Login and reset flows are rate limited.
- Expired, revoked, or consumed credentials cannot be reused.
- User-facing errors do not leak account existence.
- Existing sessions are revoked after password reset.
- templ and sqlc generated files are up to date.
- Auth tests pass and any skipped verification is explicitly documented.
