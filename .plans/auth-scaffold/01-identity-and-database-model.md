# Identity And Database Model

## ID Strategy

Use UUIDv7 for all internal primary keys and foreign keys. Application code,
sqlc queries, joins, and database constraints should use UUIDs, not public IDs.

Use prefixed NanoID-style PIDs for public references. PIDs should be unique,
stable, readable enough for support/debugging, and safe to show in UI. Examples:
`usr_...`, `org_...`, `team_...`, `mem_...`, `ses_...`, `pwd_...`, `otp_...`,
`rst_...`, `prn_...`.

The migration should add these database helpers:

- `prefixed_nanoid(prefix text, size int default 16)`
- `is_prefixed_pid(value text, prefix text, size int default 16)`
- `update_updated_at_column()`

For JetStream-backed auth state that does not have a Postgres row, generate PIDs
in Go with the same prefix, size, and alphabet rules. Session, OTP challenge,
and password reset PIDs should still exist even though their state is stored in
JetStream.

Every durable domain table should have:

- `id uuid primary key default uuidv7()`
- `pid text not null unique`
- `created_at timestamptz not null default now()`
- `updated_at timestamptz not null default now()`
- an `updated_at` trigger when records are mutable

## Principal Model

Use a principal as the authenticatable actor. A user is one kind of principal;
service accounts can be added without reshaping auth later.

Core tables:

- `principals`
  - `id`, `pid`
  - `kind`: `user` or `service_account`
  - `disabled_at`
  - `auth_invalidated_at`
  - timestamps
- `users`
  - `id`, `pid`
  - `principal_id unique references principals(id)`
  - `email citext unique`
  - `name`
  - `is_staff`
  - timestamps
- `service_accounts`
  - keep in the first migration only if needed for the scaffold
  - otherwise reserve the principal shape and add service accounts later

`auth_invalidated_at` is the coarse kill switch for a principal. Any session or
credential issued before that timestamp should fail authentication.

## Org, Team, Member Hierarchy

Represent hierarchy with scopes, then attach concrete org/team records to those
scopes. This keeps membership rules uniform while still giving orgs and teams
their own tables.

Core tables:

- `scopes`
  - `id`, `pid`
  - `kind`: `org` or `team`
  - `parent_id references scopes(id)`
  - org scopes have no parent
  - team scopes must have an org or team parent
- `orgs`
  - `id`, `pid`
  - `scope_id unique references scopes(id)`
  - `name`
  - `slug citext unique`
- `teams`
  - `id`, `pid`
  - `scope_id unique references scopes(id)`
  - `org_id references orgs(id)`
  - `parent_team_id references teams(id)` nullable
  - `name`
  - `slug citext`
- `memberships`
  - `id`, `pid`
  - `scope_id references scopes(id)`
  - `principal_id references principals(id)`
  - `role`: `owner`, `admin`, `member`, `viewer`
  - `joined_at`
  - timestamps

Add a unique index on `(scope_id, principal_id)` so a principal has one role per
scope. Add indexes for principal lookup, org team lookup, and parent scope
navigation.

The first UI can simply show the current user's org/team context later. The
schema should still be ready for dashboards that need org and team switching.

## Credential Tables

Keep credentials separate from `users`. A user can authenticate through
email/password, email/OTP, or both.

- `password_credentials`
  - `id`, `pid`
  - `principal_id references principals(id)`
  - `email citext unique`
  - `password_hash`
  - `password_updated_at`
  - `disabled_at`
  - timestamps
- `email_otp_credentials`
  - `id`, `pid`
  - `principal_id references principals(id)`
  - `email citext unique`
  - `verified_at`
  - `disabled_at`
  - timestamps

OTP credentials are durable identity records. OTP challenges are short-lived auth
state and should live in JetStream, not Postgres.

## JetStream Auth State

Do not create Postgres tables for sessions or password reset tokens. Session
state, password reset tokens, and OTP challenges should be stored in
NATS/JetStream.

Use separate JetStream KV buckets or purpose-specific streams:

- `auth_sessions`
  - key: session token hash
  - value: session PID, principal ID, auth method, created IP, last seen IP,
    user agent, issued time, expiry time, revoked time, last seen time
- `auth_password_resets`
  - key: reset token hash
  - value: reset PID, principal ID, email, issued time, expiry time, consumed
    time
- `auth_otp_challenges`
  - key: challenge PID or OTP code hash
  - value: credential ID, email, code hash, issued time, expiry time, consumed
    time, attempt count

Only hashes of session tokens, reset tokens, and OTP codes should be stored.
Use bucket TTLs and explicit expiry fields. For one-time consumption, use
JetStream revision checks or another atomic compare-and-set style update so a
reset token or OTP code cannot be consumed twice.

## Auth Events

Add an `auth_events` stream or an outbox-backed equivalent before relying on
auth events for downstream behavior. Useful events include login
success/failure, logout, session revoked, OTP requested, OTP verified, password
reset requested, password reset completed, and principal auth invalidated.

For this scaffold, Postgres is the source of truth for durable identity and
credential records. JetStream is the source of truth for short-lived auth state
such as sessions, OTP challenges, and reset tokens, and also carries auth events
for audit streams, projections, notifications, and future CQRS workflows.

---

**Previous:** [Auth Scaffold Plan](README.md) · **Next:** [Seed Data](02-seed-data.md)
