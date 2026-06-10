-- +goose Up
-- +goose StatementBegin
CREATE TYPE user_role AS ENUM ('owner', 'admin', 'member');
CREATE TABLE users (
    id uuid PRIMARY KEY DEFAULT uuidv7 (),
    pid text NOT NULL UNIQUE DEFAULT prefixed_nanoid ('usr') CHECK (is_prefixed_pid (pid, 'usr')),
    email citext NOT NULL UNIQUE,
    name text NOT NULL,
    role user_role NOT NULL DEFAULT 'member',
    password_hash text NULL,
    password_updated_at timestamptz NOT NULL DEFAULT now(),
    disabled_at timestamptz NULL,
    auth_invalidated_at timestamptz NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TRIGGER update_users_updated_at BEFORE
    UPDATE ON users FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column ();


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


CREATE TABLE team_memberships (
    id uuid PRIMARY KEY DEFAULT uuidv7 (),
    pid text NOT NULL UNIQUE DEFAULT prefixed_nanoid ('mem') CHECK (is_prefixed_pid (pid, 'mem')),
    team_id uuid NOT NULL REFERENCES teams (id) ON DELETE CASCADE,
    user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (team_id, user_id)
);
CREATE TRIGGER update_team_memberships_updated_at BEFORE
    UPDATE ON team_memberships FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column ();
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS team_memberships;

DROP TABLE IF EXISTS teams;

DROP TABLE IF EXISTS users;
DROP TYPE IF EXISTS user_role;
-- +goose StatementEnd