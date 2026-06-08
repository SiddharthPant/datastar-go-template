# Database And Queries Code

## 1. Add `database/migrations/00002_auth_identity.sql`

This migration creates durable identity data only. Sessions, OTP challenges, and
password reset tokens intentionally do not appear here.

```sql
-- +goose Up
-- +goose StatementBegin
CREATE EXTENSION IF NOT EXISTS pgcrypto;
CREATE EXTENSION IF NOT EXISTS citext;

DROP TABLE IF EXISTS users;

CREATE OR REPLACE FUNCTION prefixed_nanoid(
  prefix text DEFAULT 'id',
  size int DEFAULT 16,
  alphabet text DEFAULT '123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz'
) RETURNS text
LANGUAGE plpgsql VOLATILE PARALLEL SAFE AS $$
DECLARE
  id_builder text := '';
  counter int := 0;
  bytes bytea;
  alphabet_index int;
  alphabet_array text[];
  alphabet_length int := 64;
  mask int := 63;
  step int := 34;
BEGIN
  alphabet_array := regexp_split_to_array(alphabet, '');
  alphabet_length := array_length(alphabet_array, 1);

  LOOP
    bytes := gen_random_bytes(step);
    FOR counter IN 0..step - 1 LOOP
      alphabet_index := (get_byte(bytes, counter) & mask) + 1;
      IF alphabet_index <= alphabet_length THEN
        id_builder := id_builder || alphabet_array[alphabet_index];
        IF length(id_builder) = size THEN
          RETURN prefix || '_' || id_builder;
        END IF;
      END IF;
    END LOOP;
  END LOOP;
END
$$;

CREATE OR REPLACE FUNCTION is_prefixed_pid(value text, prefix text, size int DEFAULT 16)
RETURNS boolean
LANGUAGE sql IMMUTABLE STRICT AS $$
  SELECT value ~ ('^' || prefix || '_[1-9A-HJ-NP-Za-km-z]{' || size || '}$')
$$;

CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS trigger AS $$
BEGIN
  NEW.updated_at = now();
  RETURN NEW;
END;
$$ language plpgsql;

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'principal_kind') THEN
    CREATE TYPE principal_kind AS ENUM ('user', 'service_account');
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'scope_kind') THEN
    CREATE TYPE scope_kind AS ENUM ('org', 'team');
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'membership_role') THEN
    CREATE TYPE membership_role AS ENUM ('owner', 'admin', 'member', 'viewer');
  END IF;
END $$;

CREATE TABLE principals (
  id uuid PRIMARY KEY DEFAULT uuidv7(),
  pid text NOT NULL UNIQUE DEFAULT prefixed_nanoid('prn') CHECK (is_prefixed_pid(pid, 'prn')),
  kind principal_kind NOT NULL,
  disabled_at timestamptz NULL,
  auth_invalidated_at timestamptz NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TRIGGER update_principals_updated_at
BEFORE UPDATE ON principals
FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TABLE users (
  id uuid PRIMARY KEY DEFAULT uuidv7(),
  pid text NOT NULL UNIQUE DEFAULT prefixed_nanoid('usr') CHECK (is_prefixed_pid(pid, 'usr')),
  principal_id uuid NOT NULL UNIQUE REFERENCES principals(id) ON DELETE CASCADE,
  email citext NOT NULL UNIQUE,
  name text NOT NULL,
  is_staff boolean NOT NULL DEFAULT false,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TRIGGER update_users_updated_at
BEFORE UPDATE ON users
FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TABLE scopes (
  id uuid PRIMARY KEY DEFAULT uuidv7(),
  pid text NOT NULL UNIQUE DEFAULT prefixed_nanoid('scp') CHECK (is_prefixed_pid(pid, 'scp')),
  kind scope_kind NOT NULL,
  parent_id uuid NULL REFERENCES scopes(id) ON DELETE CASCADE,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT scopes_parent_kind_chk CHECK (
    (kind = 'org' AND parent_id IS NULL) OR
    (kind = 'team' AND parent_id IS NOT NULL)
  )
);

CREATE INDEX scopes_parent_id_idx ON scopes(parent_id);

CREATE TRIGGER update_scopes_updated_at
BEFORE UPDATE ON scopes
FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TABLE orgs (
  id uuid PRIMARY KEY DEFAULT uuidv7(),
  pid text NOT NULL UNIQUE DEFAULT prefixed_nanoid('org') CHECK (is_prefixed_pid(pid, 'org')),
  scope_id uuid NOT NULL UNIQUE REFERENCES scopes(id) ON DELETE CASCADE,
  name text NOT NULL,
  slug citext NOT NULL UNIQUE,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TRIGGER update_orgs_updated_at
BEFORE UPDATE ON orgs
FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TABLE teams (
  id uuid PRIMARY KEY DEFAULT uuidv7(),
  pid text NOT NULL UNIQUE DEFAULT prefixed_nanoid('team') CHECK (is_prefixed_pid(pid, 'team')),
  scope_id uuid NOT NULL UNIQUE REFERENCES scopes(id) ON DELETE CASCADE,
  org_id uuid NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
  parent_team_id uuid NULL REFERENCES teams(id) ON DELETE CASCADE,
  name text NOT NULL,
  slug citext NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (org_id, slug)
);

CREATE INDEX teams_org_id_idx ON teams(org_id);
CREATE INDEX teams_parent_team_id_idx ON teams(parent_team_id);

CREATE TRIGGER update_teams_updated_at
BEFORE UPDATE ON teams
FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TABLE memberships (
  id uuid PRIMARY KEY DEFAULT uuidv7(),
  pid text NOT NULL UNIQUE DEFAULT prefixed_nanoid('mem') CHECK (is_prefixed_pid(pid, 'mem')),
  scope_id uuid NOT NULL REFERENCES scopes(id) ON DELETE CASCADE,
  principal_id uuid NOT NULL REFERENCES principals(id) ON DELETE CASCADE,
  role membership_role NOT NULL,
  joined_at timestamptz NOT NULL DEFAULT now(),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (scope_id, principal_id)
);

CREATE INDEX memberships_principal_id_idx ON memberships(principal_id);

CREATE TRIGGER update_memberships_updated_at
BEFORE UPDATE ON memberships
FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TABLE password_credentials (
  id uuid PRIMARY KEY DEFAULT uuidv7(),
  pid text NOT NULL UNIQUE DEFAULT prefixed_nanoid('pwd') CHECK (is_prefixed_pid(pid, 'pwd')),
  principal_id uuid NOT NULL REFERENCES principals(id) ON DELETE CASCADE,
  email citext NOT NULL UNIQUE,
  password_hash text NOT NULL,
  password_updated_at timestamptz NOT NULL DEFAULT now(),
  disabled_at timestamptz NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX password_credentials_principal_id_idx ON password_credentials(principal_id);

CREATE TRIGGER update_password_credentials_updated_at
BEFORE UPDATE ON password_credentials
FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TABLE email_otp_credentials (
  id uuid PRIMARY KEY DEFAULT uuidv7(),
  pid text NOT NULL UNIQUE DEFAULT prefixed_nanoid('otp') CHECK (is_prefixed_pid(pid, 'otp')),
  principal_id uuid NOT NULL REFERENCES principals(id) ON DELETE CASCADE,
  email citext NOT NULL UNIQUE,
  verified_at timestamptz NULL,
  disabled_at timestamptz NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX email_otp_credentials_principal_id_idx ON email_otp_credentials(principal_id);

CREATE TRIGGER update_email_otp_credentials_updated_at
BEFORE UPDATE ON email_otp_credentials
FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS email_otp_credentials;
DROP TABLE IF EXISTS password_credentials;
DROP TABLE IF EXISTS memberships;
DROP TABLE IF EXISTS teams;
DROP TABLE IF EXISTS orgs;
DROP TABLE IF EXISTS scopes;
DROP TABLE IF EXISTS users;
DROP TABLE IF EXISTS principals;

DROP TYPE IF EXISTS membership_role;
DROP TYPE IF EXISTS scope_kind;
DROP TYPE IF EXISTS principal_kind;

DROP FUNCTION IF EXISTS update_updated_at_column();
DROP FUNCTION IF EXISTS is_prefixed_pid(text, text, int);
DROP FUNCTION IF EXISTS prefixed_nanoid(text, int, text);
DROP EXTENSION IF EXISTS citext;
-- +goose StatementEnd
```

## 2. Add `database/queries/auth.sql`

```sql
-- name: GetUserByPrincipalID :one
SELECT *
FROM users
WHERE principal_id = $1;

-- name: GetPrincipalAuthState :one
SELECT disabled_at, auth_invalidated_at
FROM principals
WHERE id = $1;

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

## 3. Add `database/queries/seed.sql`

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

## 4. Update `Taskfile.yml`

Add this task:

```yaml
  db:seed:
    desc: Seed local org/team/user data
    deps: [templ]
    cmds:
      - go run ./cmd/seed
```

## 5. Update `.env.example`

Add these values:

```sh
APP_BASE_URL=http://localhost:8080

SMTP_ADDR=localhost:1025
SMTP_FROM=noreply@example.test

SEED_PASSWORD=ChangeMe123!
```

## 6. Update `config/config.go`

Add fields to `Config`:

```go
AppBaseURL string

SMTPAddr string
SMTPFrom string

SeedPassword string
```

Add values in `loadBase()`:

```go
AppBaseURL: getEnv("APP_BASE_URL", "http://localhost:8080"),

SMTPAddr: getEnv("SMTP_ADDR", "localhost:1025"),
SMTPFrom: getEnv("SMTP_FROM", "noreply@example.test"),

SeedPassword: getEnv("SEED_PASSWORD", "ChangeMe123!"),
```

## 7. Regenerate

```sh
task sqlc:generate
task sqlc:vet
```

---

**Previous:** [Implementation Code Map](07-implementation-code-map.md) · **Next:** [Core Auth And JetStream Code](09-implementation-code-core.md)
