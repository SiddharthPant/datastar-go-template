-- name: HealthCheck :one
SELECT
  now()::text AS now;
