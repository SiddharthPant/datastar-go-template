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

-- name: UpsertTeamForSeed :one
INSERT INTO
    teams (id, name, slug)
VALUES
    (@id, @name, @slug)
ON CONFLICT (slug) DO UPDATE
SET
    name = EXCLUDED.name,
    slug = EXCLUDED.slug
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
