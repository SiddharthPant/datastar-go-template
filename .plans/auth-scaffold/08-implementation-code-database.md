# Database And Queries Code

## 1. Migration `database/migrations/00002_auth_identity.sql` — DONE

This is already committed. The split differs slightly from the original plan:

- The extensions (`pgcrypto`, `citext`) and the `prefixed_nanoid`,
  `is_prefixed_pid`, and `update_updated_at_column` helpers live in
  `database/migrations/00001_init.sql`, not in `00002`.
- `00002_auth_identity.sql` creates the enum types (`principal_kind`,
  `scope_kind`, `membership_role`) and the tables: `principals`, `users`,
  `scopes`, `orgs`, `teams`, `memberships`, `password_credentials`,
  `email_otp_credentials`, with PID checks, indexes, and `updated_at` triggers.
- Its `Down` drops only the auth tables and enum types; extensions and helpers
  are owned by `00001`.

Postgres in `compose.yml` is pg18, so native `uuidv7()` is available — no
extension or polyfill needed.

Sessions, OTP challenges, and password reset tokens intentionally do not appear
in any migration; they live in JetStream.

## 2. Add `database/queries/auth.sql`

`database/queries/auth.sql` was removed in commit 5e82b5e ("remove queries not
being worked on right now") and the stale `database/sqlc/auth.sql.go` deletion
is pending in the working tree. Re-add it when implementing the auth service:

```sql
-- name: GetUserWithAuthState :one
SELECT
  sqlc.embed(users),
  principals.disabled_at,
  principals.auth_invalidated_at
FROM users
JOIN principals ON principals.id = users.principal_id
WHERE users.principal_id = $1;

-- name: GetPrincipalByUserEmail :one
SELECT p.*
FROM users u
JOIN principals p ON p.id = u.principal_id
WHERE u.email = $1;

-- name: GetPasswordCredentialByEmail :one
SELECT
  pc.id,
  pc.pid,
  pc.principal_id,
  pc.email,
  pc.password_hash,
  pc.password_updated_at,
  pc.disabled_at AS credential_disabled_at,
  p.disabled_at AS principal_disabled_at,
  p.auth_invalidated_at,
  p.created_at AS principal_created_at
FROM password_credentials pc
JOIN principals p ON p.id = pc.principal_id
WHERE pc.email = $1;

-- name: GetOTPCredentialByEmail :one
SELECT
  oc.id,
  oc.pid,
  oc.principal_id,
  oc.email,
  oc.verified_at,
  oc.disabled_at AS credential_disabled_at,
  p.disabled_at AS principal_disabled_at,
  p.auth_invalidated_at,
  p.created_at AS principal_created_at
FROM email_otp_credentials oc
JOIN principals p ON p.id = oc.principal_id
WHERE oc.email = $1;

-- name: TouchPrincipalAuthInvalidatedAt :exec
UPDATE principals
SET auth_invalidated_at = now()
WHERE id = $1;

-- name: UpdatePasswordCredentialHash :exec
UPDATE password_credentials
SET
  password_hash = $2,
  password_updated_at = now()
WHERE principal_id = $1;

-- name: ListMembershipsForPrincipal :many
SELECT
  m.id,
  m.pid,
  m.role,
  m.scope_id,
  s.kind AS scope_kind,
  o.pid AS org_pid,
  o.name AS org_name,
  o.slug AS org_slug,
  t.pid AS team_pid,
  t.name AS team_name,
  t.slug AS team_slug
FROM memberships m
JOIN scopes s ON s.id = m.scope_id
LEFT JOIN orgs o ON o.scope_id = s.id
LEFT JOIN teams t ON t.scope_id = s.id
WHERE m.principal_id = $1
ORDER BY s.kind, o.name, t.name;
```

Notes:

- Nullable `timestamptz` columns (`credential_disabled_at`,
  `principal_disabled_at`, `auth_invalidated_at`) generate as
  `pgtype.Timestamptz` — callers check `.Valid` and read `.Time`, not `!= nil`.
  Doc 10's service code depends on this.
- `GetUserWithAuthState` is the per-request session-validation query: one
  round trip returns the user row (via `sqlc.embed`) together with the
  principal's disabled/invalidated state.

## 3. Replace `database/queries/seed.sql`

A WIP version of this file exists in the working tree with a different
`CreatePrincipal` shape (explicit `id`/`pid` parameters). Replace it with this
version, which lets the database defaults generate `id` and `pid`:

```sql
-- name: CreatePrincipal :one
INSERT INTO principals(kind)
VALUES ($1)
RETURNING *;

-- name: UpsertUserForSeed :one
INSERT INTO users(principal_id, email, name, is_staff)
VALUES ($1, $2, $3, $4)
ON CONFLICT (email) DO UPDATE
SET
  name = EXCLUDED.name,
  is_staff = EXCLUDED.is_staff
RETURNING *;

-- name: GetUserByEmail :one
SELECT *
FROM users
WHERE email = $1;

-- name: UpsertPasswordCredentialForSeed :one
INSERT INTO password_credentials(principal_id, email, password_hash)
VALUES ($1, $2, $3)
ON CONFLICT (email) DO UPDATE
SET password_hash = EXCLUDED.password_hash
RETURNING *;

-- name: UpsertOTPCredentialForSeed :one
INSERT INTO email_otp_credentials(principal_id, email, verified_at)
VALUES ($1, $2, now())
ON CONFLICT (email) DO UPDATE
SET verified_at = COALESCE(email_otp_credentials.verified_at, now())
RETURNING *;

-- name: CreateOrgScopeForSeed :one
INSERT INTO scopes(kind, parent_id)
VALUES ('org', NULL)
RETURNING *;

-- name: UpsertOrgForSeed :one
INSERT INTO orgs(scope_id, name, slug)
VALUES ($1, $2, $3)
ON CONFLICT (slug) DO UPDATE
SET name = EXCLUDED.name
RETURNING *;

-- name: GetOrgBySlug :one
SELECT *
FROM orgs
WHERE slug = $1;

-- name: CreateTeamScopeForSeed :one
INSERT INTO scopes(kind, parent_id)
VALUES ('team', $1)
RETURNING *;

-- name: UpsertTeamForSeed :one
INSERT INTO teams(scope_id, org_id, parent_team_id, name, slug)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (org_id, slug) DO UPDATE
SET
  name = EXCLUDED.name,
  parent_team_id = EXCLUDED.parent_team_id
RETURNING *;

-- name: GetTeamByOrgAndSlug :one
SELECT *
FROM teams
WHERE org_id = $1 AND slug = $2;

-- name: UpsertMembershipForSeed :one
INSERT INTO memberships(scope_id, principal_id, role)
VALUES ($1, $2, $3)
ON CONFLICT (scope_id, principal_id) DO UPDATE
SET role = EXCLUDED.role
RETURNING *;
```

`UpsertOrgForSeed` / `UpsertTeamForSeed` keep their `DO UPDATE` clauses so a
changed fixture name propagates on re-run; `seed.Run` in doc 11 calls them on
both the create and the already-exists path for exactly that reason.

## 4. Update `Taskfile.yml`

Add this task (not yet present):

```yaml
  db:seed:
    desc: Seed local org/team/user data
    cmds:
      - go run ./cmd/seed
```

## 5. `.env.example` — DONE

`APP_BASE_URL`, `SMTP_ADDR`, `SMTP_FROM`, and `SEED_PASSWORD` are already in
`.env.example`. Note they are **required** by config loading, so a `.env`
missing them fails at boot.

## 6. `config/config.go` — DONE

The config package already has `AppBaseURL`, `SMTPAddr`, `SMTPFrom`, and
`SeedPassword` fields, loaded as required values in `load()`. Access pattern
used throughout docs 10 and 11:

```go
config.Env.AppBaseURL
config.Env.SMTPAddr
config.Env.SMTPFrom
config.Env.SeedPassword
config.Env.AppEnv == config.Prod // environment check
```

There is no `config.Global` and no `loadBase()` — earlier drafts of this plan
referenced both; any remaining snippet that does is stale.

One removal: drop `SessionSecret` from `Config`/`load()` and `SESSION_SECRET`
from `.env.example`. Sessions are opaque random tokens stored as hashes — there
is nothing to sign, so the secret is dead config that future projects would
cargo-cult.

## 7. Regenerate

```sh
task sqlc:generate
task sqlc:vet
```

---

**Previous:** [Implementation Code Map](07-implementation-code-map.md) · **Next:** [Core Auth And JetStream Code](09-implementation-code-core.md)
