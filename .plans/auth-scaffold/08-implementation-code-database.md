# Database And Queries Code

## 1. Replace `database/migrations/00002_auth_identity.sql`

The committed version creates principals, scopes, orgs, teams, memberships,
and credential tables. Replace its entire contents with the three-table
schema. This rewrites an already-applied migration, which is fine for a
playground with disposable data — rebuild afterwards with `task db:nuke` (or
the drop/recreate reset task) so goose applies the new version from scratch.

```sql
-- +goose Up
-- +goose StatementBegin
CREATE TYPE user_role AS ENUM ('owner', 'admin', 'member');

CREATE TABLE teams (
    id uuid PRIMARY KEY DEFAULT uuidv7 (),
    pid text NOT NULL UNIQUE DEFAULT prefixed_nanoid ('team') CHECK (is_prefixed_pid (pid, 'team')),
    name text NOT NULL,
    slug citext NOT NULL UNIQUE,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TRIGGER update_teams_updated_at BEFORE
    UPDATE ON teams FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column ();


CREATE TABLE users (
    id uuid PRIMARY KEY DEFAULT uuidv7 (),
    pid text NOT NULL UNIQUE DEFAULT prefixed_nanoid ('usr') CHECK (is_prefixed_pid (pid, 'usr')),
    email citext NOT NULL UNIQUE,
    name text NOT NULL,
    role user_role NOT NULL DEFAULT 'member',
    password_hash text NOT NULL,
    password_updated_at timestamptz NOT NULL DEFAULT now(),
    disabled_at timestamptz NULL,
    auth_invalidated_at timestamptz NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TRIGGER update_users_updated_at BEFORE
    UPDATE ON users FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column ();


CREATE TABLE team_memberships (
    id uuid PRIMARY KEY DEFAULT uuidv7 (),
    pid text NOT NULL UNIQUE DEFAULT prefixed_nanoid ('mem') CHECK (is_prefixed_pid (pid, 'mem')),
    team_id uuid NOT NULL REFERENCES teams (id) ON DELETE CASCADE,
    user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (team_id, user_id)
);
CREATE INDEX team_memberships_user_id_idx ON team_memberships (user_id);
CREATE TRIGGER update_team_memberships_updated_at BEFORE
    UPDATE ON team_memberships FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column ();
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS team_memberships;

DROP TABLE IF EXISTS users;

DROP TABLE IF EXISTS teams;

DROP TYPE IF EXISTS user_role;
-- +goose StatementEnd
```

Notes:

- `user_role` is a plain top-level `CREATE TYPE` — the earlier sqlc enum
  friction only applied to types created inside `DO $$` blocks. sqlc
  generates a `UserRole` Go type with `UserRoleOwner` / `UserRoleAdmin` /
  `UserRoleMember` constants.
- Owners have no `team_memberships` rows; their all-teams access is derived
  in the service layer (doc 01). The member-one-team rule is an application
  invariant, not a constraint — Postgres can't express it without a trigger.
- The `uuidv7()`, `prefixed_nanoid`, and trigger helpers are owned by
  `00001_init.sql` and unchanged.

## 2. Add `database/queries/auth.sql`

The runtime auth queries plus the role-aware team lookups.

```sql
-- name: GetUserByEmail :one
SELECT *
FROM users
WHERE email = @email;

-- name: GetUserByID :one
SELECT *
FROM users
WHERE id = @id;

-- name: UpdateUserPassword :exec
UPDATE users
SET
    password_hash = @password_hash,
    password_updated_at = now(),
    auth_invalidated_at = now()
WHERE id = @id;

-- name: ListAllTeams :many
SELECT *
FROM teams
ORDER BY name;

-- name: ListTeamsForUser :many
SELECT t.*
FROM teams t
JOIN team_memberships m ON m.team_id = t.id
WHERE m.user_id = @user_id
ORDER BY t.name;

-- name: GetTeamMembership :one
SELECT *
FROM team_memberships
WHERE team_id = @team_id AND user_id = @user_id;
```

Notes:

- `GetUserByEmail` backs login and forgot-password; `citext` makes the match
  case-insensitive without `lower()`.
- `GetUserByID` is the per-request session-validation query. The handler
  checks `disabled_at` and `auth_invalidated_at` from the returned row —
  both generate as `pgtype.Timestamptz` (check `.Valid`, read `.Time`).
- `UpdateUserPassword` sets `auth_invalidated_at` in the same statement, so
  a password reset atomically kills every previously issued session — no
  transaction needed.
- `ListAllTeams` is the owner path, `ListTeamsForUser` the admin/member
  path, and `GetTeamMembership` backs `CanAccessTeam` — the service picks by
  `users.role` (doc 10 §5a).

## 3. Replace `database/queries/seed.sql`

Replace the WIP version (which creates principals). Deterministic IDs come
from the seeder; every statement converges on conflict:

```sql
-- name: UpsertTeamForSeed :one
INSERT INTO
    teams (id, name, slug)
VALUES
    (@id, @name, @slug)
ON CONFLICT (id) DO UPDATE
SET
    name = EXCLUDED.name,
    slug = EXCLUDED.slug
RETURNING
    *;

-- name: UpsertUserForSeed :one
INSERT INTO
    users (id, email, name, role, password_hash)
VALUES
    (@id, @email, @name, @role, @password_hash)
ON CONFLICT (id) DO UPDATE
SET
    email = EXCLUDED.email,
    name = EXCLUDED.name,
    role = EXCLUDED.role,
    password_hash = EXCLUDED.password_hash
RETURNING
    *;

-- name: UpsertTeamMembershipForSeed :one
INSERT INTO
    team_memberships (id, team_id, user_id)
VALUES
    (@id, @team_id, @user_id)
ON CONFLICT (id) DO UPDATE
SET
    team_id = EXCLUDED.team_id,
    user_id = EXCLUDED.user_id
RETURNING
    *;
```

## 4. `Taskfile.yml`

`db:seed` already exists (`go run ./cmd/seed`). No new tasks required by
this plan; the db reset/nuke tasks discussed separately are nice-to-haves
for the rebuild in §1.

## 5. `.env.example` And `config/config.go` — DONE

`APP_BASE_URL`, `SMTP_ADDR`, `SMTP_FROM`, and `SEED_PASSWORD` are already
present and **required** by config loading. Access pattern used throughout
docs 10 and 11:

```go
config.Env.AppBaseURL
config.Env.SMTPAddr
config.Env.SMTPFrom
config.Env.SeedPassword
config.Env.AppEnv == config.Prod // environment check
```

One removal if still present: drop `SessionSecret` / `SESSION_SECRET`.
Sessions are opaque random tokens stored as hashes — there is nothing to
sign, so the secret is dead config that future projects would cargo-cult.

## 6. Regenerate

```sh
task sqlc:generate
task sqlc:vet
```

After regeneration, `database/sqlc/models.go` must contain only `Team`,
`User`, `TeamMembership`, and the `UserRole` enum (no
`Principal`/`Scope`/`Org`), and the stale `auth.sql.go`/`seed.sql.go`
contents are replaced. sqlc deletes output for removed query files on
regeneration — no manual `rm` needed.

---

**Previous:** [Implementation Code Map](07-implementation-code-map.md) · **Next:** [Core Auth And JetStream Code](09-implementation-code-core.md)
