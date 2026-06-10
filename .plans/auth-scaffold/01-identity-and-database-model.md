# Identity And Database Model

## ID Strategy

Use UUIDv7 for all internal primary keys and foreign keys. Application code,
sqlc queries, joins, and database constraints use UUIDs, not public IDs.

Use prefixed NanoID-style PIDs for public references: `usr_...`, `team_...`,
`mem_...`, `ses_...`, `rst_...`. PIDs are unique, stable, readable enough
for support/debugging, and safe to show in UI.

The database helpers already exist in `00001_init.sql`:

- `prefixed_nanoid(prefix text, size int default 16)`
- `is_prefixed_pid(value text, prefix text, size int default 16)`
- `update_updated_at_column()`

For JetStream-backed auth state that has no Postgres row (sessions, reset
tokens), generate PIDs in Go with the same prefix, size, and alphabet rules.

Every durable domain table has:

- `id uuid primary key default uuidv7()`
- `pid text not null unique`
- `created_at timestamptz not null default now()`
- `updated_at timestamptz not null default now()`
- an `updated_at` trigger

## The Three Tables

Identity is exactly three tables. No principals, no scopes, no orgs, no
separate credential tables.

- `teams`
  - `id`, `pid` (`team`)
  - `name text not null`
  - `slug citext not null unique`
  - timestamps
- `users`
  - `id`, `pid` (`usr`)
  - `email citext not null unique` — the login identifier; `citext` makes
    lookups case-insensitive without `lower()` gymnastics
  - `name text not null`
  - `role user_role not null default 'member'` — system-wide role, see below
  - `password_hash text not null` — PHC-encoded Argon2id string (doc 04)
  - `password_updated_at timestamptz not null default now()`
  - `disabled_at timestamptz null` — soft kill switch; a disabled user cannot
    log in and existing sessions stop validating
  - `auth_invalidated_at timestamptz null` — sessions issued before this
    timestamp fail validation; set it on password reset or suspected
    compromise to revoke everything at once
  - timestamps
- `team_memberships`
  - `id`, `pid` (`mem`)
  - `team_id references teams(id) on delete cascade`
  - `user_id references users(id) on delete cascade`
  - `unique (team_id, user_id)` — one membership per user per team
  - timestamps

One enum, `user_role`, created as a plain top-level `CREATE TYPE` (the
earlier sqlc friction only applied to types created inside `DO $$` blocks).

## Roles And Team Access

The role lives on the **user**, not on the membership. Team access is
derived:

- **owner** — has access to **all** teams. Owners carry no membership rows;
  their access is computed, so creating a new team never requires backfilling
  owner memberships.
- **admin** — has access to the teams they are members of (one or more
  membership rows).
- **member** — has access to exactly **one** team (exactly one membership
  row).

Resolution is one helper pair in the auth service (doc 10):

- `TeamsForUser`: owner → all teams; otherwise → joined memberships
- `CanAccessTeam`: owner → true; otherwise → membership row exists

The member-one-team rule is an **application-level invariant**, enforced
wherever memberships are written (currently only the seeder, later any team
management flow): writing a second membership for a `member`, or demoting a
multi-team user to `member`, is rejected in code. Postgres cannot express
"unique user_id when the user's role is member" without a trigger, and a
trigger is more machinery than this playground needs — revisit if membership
writes ever come from more than one code path.

The previous schema's per-membership roles (`owner/admin/member/viewer` per
scope) are gone. If per-team permission levels are ever needed, add a `role`
column to `team_memberships` then — purely additive.

## JetStream Auth State

Do not create Postgres tables for sessions or password reset tokens. They
live in NATS JetStream KV:

- `auth_sessions`
  - key: SHA-256 hash of the session token
  - value: session PID, user ID, created IP, last seen IP, user agent, issued
    time, expiry time, revoked time, last seen time
- `auth_password_resets`
  - key: SHA-256 hash of the reset token
  - value: reset PID, user ID, email, issued time, expiry time, consumed time
- `auth_rate_limits`
  - key: scoped limit key (e.g. `login.<ip>.<email>`)
  - value: hit count, window reset time

Only hashes of session and reset tokens are stored — a leaked KV dump must
not contain usable credentials. Bucket TTLs garbage-collect dead entries, but
the explicit expiry fields are the authoritative check. One-time consumption
(reset tokens) and counters (rate limits) use JetStream revision-checked
updates so concurrent requests cannot double-spend a token or share a
counter increment.

## Auth Events

Publish auth lifecycle events to an `AUTH_EVENTS` JetStream stream on subject
`auth.>`: login succeeded/failed, logout, password reset requested/completed,
session revoked. Postgres remains the source of truth for identity;
JetStream is the source of truth for short-lived auth state and carries the
audit/event stream for projections, notifications, and future CQRS
experiments.

---

**Previous:** [Auth Scaffold Plan](README.md) · **Next:** [Seed Data](02-seed-data.md)
