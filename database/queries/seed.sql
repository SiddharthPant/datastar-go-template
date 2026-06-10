-- name: CreatePrincipal :one
INSERT INTO
    principals (kind)
VALUES
    (@kind::principal_kind)
RETURNING
    *;

-- name: UpsertUserForSeed :one
INSERT INTO
    users (principal_id, email, name, is_staff)
VALUES
    (@principal_id, @email, @name, @is_staff)
ON CONFLICT (email) DO UPDATE
SET
    name = EXCLUDED.name,
    is_staff = EXCLUDED.is_staff
RETURNING
    *;