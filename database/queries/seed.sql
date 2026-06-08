-- name: CreatePrincipal :one
INSERT INTO
    principals (kind)
VALUES
    ($1)
RETURNING
    *;

-- name: UpsertUserForSeed :one
INSERT INTO
    users (principal_id, email, name, is_staff)
VALUES
    ($1, $2, $3, $4)
ON CONFLICT (email) DO UPDATE
SET
    name = EXCLUDED.name,
    is_staff = EXCLUDED.is_staff
RETURNING
    *;

-- name: GetUserByEmail :one
SELECT
    *
FROM
    users
WHERE
    email = $1;

-- name: UpsertPasswordCredentialForSeed :one
INSERT INTO
    password_credentials (principal_id, email, password_hash)
VALUES
    ($1, $2, $3)
ON CONFLICT (email) DO UPDATE
SET
    password_hash = EXCLUDED.password_hash
RETURNING
    *;

-- name: UpsertOTPCredentialForSeed :one
INSERT INTO
    email_otp_credentials (principal_id, email, verified_at)
VALUES
    ($1, $2, now())
ON CONFLICT (email) DO UPDATE
SET
    verified_at = COALESCE(email_otp_credentials.verified_at, now())
RETURNING
    *;

-- name: CreateOrgScopeForSeed :one
INSERT INTO
    scopes (kind, parent_id)
VALUES
    ('org', NULL)
RETURNING
    *;

-- name: UpsertOrgForSeed :one
INSERT INTO
    orgs (scope_id, name, slug)
VALUES
    ($1, $2, $3)
ON CONFLICT (slug) DO UPDATE
SET
    name = EXCLUDED.name
RETURNING
    *;

-- name: GetOrgBySlug :one
SELECT
    *
FROM
    orgs
WHERE
    slug = $1;

-- name: CreateTeamScopeForSeed :one
INSERT INTO
    scopes (kind, parent_id)
VALUES
    ('team', $1)
RETURNING
    *;

-- name: UpsertTeamForSeed :one
INSERT INTO
    teams (scope_id, org_id, parent_team_id, name, slug)
VALUES
    ($1, $2, $3, $4, $5)
ON CONFLICT (slug) DO UPDATE
SET
    name = EXCLUDED.name,
    parent_team_id = EXCLUDED.parent_team_id
RETURNING
    *;

-- name: GetTeamByOrgAndSlug :one
SELECT
    *
FROM
    teams
WHERE
    org_id = $1
    AND slug = $2;

-- name: UpsertMembershipForSeed :one
INSERT INTO
    memberships (scope_id, principal_id, role)
VALUES
    ($1, $2, $3)
ON CONFLICT (scope_id, principal_id) DO UPDATE
SET role = EXCLUDED.role
RETURNING
    *;