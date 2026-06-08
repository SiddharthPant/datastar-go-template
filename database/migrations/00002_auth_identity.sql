-- +goose Up
-- +goose StatementBegin
DO $$
BEGIN IF NOT EXISTS (
        SELECT 1
        FROM pg_type
        WHERE typname = 'principal_kind'
    ) THEN CREATE TYPE principal_kind AS ENUM ('user', 'service_account');
END IF;
IF NOT EXISTS (
    SELECT 1
    FROM pg_type
    WHERE typname = 'scope_kind'
) THEN CREATE TYPE scope_kind AS ENUM ('org', 'team');
END IF;
IF NOT EXISTS (
    SELECT 1
    FROM pg_type
    WHERE typname = 'membership_role'
) THEN CREATE TYPE membership_role AS ENUM ('owner', 'admin', 'member', 'viewer');
END IF;
END $$;

CREATE TABLE IF NOT EXISTS principals (
    id uuid PRIMARY KEY DEFAULT uuidv7 (),
    pid text NOT NULL UNIQUE DEFAULT prefixed_nanoid ('prn') CHECK (is_prefixed_pid (pid, 'prn')),
    kind principal_kind NOT NULL,
    disabled_at timestamptz NULL,
    auth_invalidated_at timestamptz NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TRIGGER update_principals_updated_at BEFORE
UPDATE ON principals FOR EACH ROW
EXECUTE FUNCTION update_updated_at_column ();

CREATE TABLE IF NOT EXISTS users (
    id uuid PRIMARY KEY DEFAULT uuidv7 (),
    pid text NOT NULL UNIQUE DEFAULT prefixed_nanoid ('usr') CHECK (is_prefixed_pid (pid, 'usr')),
    principal_id uuid NOT NULL UNIQUE REFERENCES principals (id) ON DELETE CASCADE,
    email citext NOT NULL UNIQUE,
    name text NOT NULL,
    is_staff boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TRIGGER update_users_updated_at BEFORE
UPDATE ON users FOR EACH ROW
EXECUTE FUNCTION update_updated_at_column ();

CREATE TABLE scopes (
    id uuid PRIMARY KEY DEFAULT uuidv7 (),
    pid text NOT NULL UNIQUE DEFAULT prefixed_nanoid ('scp') CHECK (is_prefixed_pid (pid, 'scp')),
    kind scope_kind NOT NULL,
    parent_id uuid NULL REFERENCES scopes (id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT scopes_parent_kind_chk CHECK (
        (
            kind = 'org'
            AND parent_id IS NULL
        )
        OR (
            kind = 'team'
            AND parent_id IS NOT NULL
        )
    )
);

CREATE INDEX scopes_parent_id_idx ON scopes (parent_id);

CREATE TRIGGER update_scopes_updated_at BEFORE
UPDATE ON scopes FOR EACH ROW
EXECUTE FUNCTION update_updated_at_column ();

CREATE TABLE orgs (
    id uuid PRIMARY KEY DEFAULT uuidv7 (),
    pid text NOT NULL UNIQUE DEFAULT prefixed_nanoid ('org') CHECK (is_prefixed_pid (pid, 'org')),
    scope_id uuid NOT NULL UNIQUE REFERENCES scopes (id) ON DELETE CASCADE,
    name text NOT NULL,
    slug citext NOT NULL UNIQUE,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TRIGGER update_orgs_updated_at BEFORE
UPDATE ON orgs FOR EACH ROW
EXECUTE FUNCTION update_updated_at_column ();

CREATE TABLE teams (
    id uuid PRIMARY KEY DEFAULT uuidv7 (),
    pid text NOT NULL UNIQUE DEFAULT prefixed_nanoid ('team') CHECK (is_prefixed_pid (pid, 'team')),
    scope_id uuid NOT NULL UNIQUE REFERENCES scopes (id) ON DELETE CASCADE,
    org_id uuid NOT NULL REFERENCES orgs (id) ON DELETE CASCADE,
    parent_team_id uuid NULL REFERENCES teams (id) ON DELETE CASCADE,
    name text NOT NULL,
    slug citext NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (org_id, slug)
);

CREATE INDEX teams_org_id_idx ON teams (org_id);

CREATE INDEX teams_parent_team_id_idx ON teams (parent_team_id);

CREATE TRIGGER update_teams_updated_at BEFORE
UPDATE ON teams FOR EACH ROW
EXECUTE FUNCTION update_updated_at_column ();

CREATE TABLE memberships (
    id uuid PRIMARY KEY DEFAULT uuidv7 (),
    pid text NOT NULL UNIQUE DEFAULT prefixed_nanoid ('mem') CHECK (is_prefixed_pid (pid, 'mem')),
    scope_id uuid NOT NULL REFERENCES scopes (id) ON DELETE CASCADE,
    principal_id uuid NOT NULL REFERENCES principals (id) ON DELETE CASCADE,
    role membership_role NOT NULL,
    joined_at timestamptz NOT NULL DEFAULT now(),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (scope_id, principal_id)
);

CREATE INDEX memberships_principal_id_idx ON memberships (principal_id);

CREATE TRIGGER update_memberships_updated_at BEFORE
UPDATE ON memberships FOR EACH ROW
EXECUTE FUNCTION update_updated_at_column ();

CREATE TABLE password_credentials (
    id uuid PRIMARY KEY DEFAULT uuidv7 (),
    pid text NOT NULL UNIQUE DEFAULT prefixed_nanoid ('pwd') CHECK (is_prefixed_pid (pid, 'pwd')),
    principal_id uuid NOT NULL REFERENCES principals (id) ON DELETE CASCADE,
    email citext NOT NULL UNIQUE,
    password_hash text NOT NULL,
    password_updated_at timestamptz NOT NULL DEFAULT now(),
    disabled_at timestamptz NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX password_credentials_principal_id_idx ON password_credentials (principal_id);

CREATE TRIGGER update_password_credentials_updated_at BEFORE
UPDATE ON password_credentials FOR EACH ROW
EXECUTE FUNCTION update_updated_at_column ();

CREATE TABLE email_otp_credentials (
    id uuid PRIMARY KEY DEFAULT uuidv7 (),
    pid text NOT NULL UNIQUE DEFAULT prefixed_nanoid ('otp') CHECK (is_prefixed_pid (pid, 'otp')),
    principal_id uuid NOT NULL REFERENCES principals (id) ON DELETE CASCADE,
    email citext NOT NULL UNIQUE,
    verified_at timestamptz NULL,
    disabled_at timestamptz NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX email_otp_credentials_principal_id_idx ON email_otp_credentials (principal_id);

CREATE TRIGGER update_email_otp_credentials_updated_at BEFORE
UPDATE ON email_otp_credentials FOR EACH ROW
EXECUTE FUNCTION update_updated_at_column ();

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

DROP TYPE IF EXISTS principal_kind;

DROP TYPE IF EXISTS scope_kind;

DROP TYPE IF EXISTS membership_role;

-- +goose StatementEnd