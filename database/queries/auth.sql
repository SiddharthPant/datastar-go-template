-- name: GetUserByPrincipalID :one
SELECT
    *
FROM
    users
WHERE
    principal_id = $1;

-- name: GetPrincipalAuthState :one
SELECT
    disabled_at,
    auth_invalidated_at
FROM
    principals
WHERE
    id = $1;

-- name: GetPrincipalByUserEmail :one
SELECT
    p.*
FROM
    users u
    JOIN principals p ON p.id = u.principal_id
WHERE
    u.email = $1;

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
FROM
    password_credentials pc
    JOIN principals p ON p.id = pc.principal_id
WHERE
    pc.email = $1;

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
FROM
    email_otp_credentials oc
    JOIN principals p ON p.id = oc.principal_id
WHERE
    oc.email = $1;

-- name: TouchPrincipalAuthInvalidatedAt :exec
UPDATE principals
SET
    auth_invalidated_at = now()
WHERE
    id = $1;

-- name: UpdatePasswordCredentialHash :exec
UPDATE password_credentials
SET
    password_hash = $2,
    password_updated_at = now()
WHERE
    principal_id = $1;

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
FROM
    memberships m
    JOIN scopes s ON s.id = m.scope_id
    JOIN orgs o ON o.scope_id = s.id
    JOIN teams t ON t.scope_id = s.id
WHERE
    m.principal_id = $1;