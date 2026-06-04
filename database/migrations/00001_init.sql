-- +goose Up
-- +goose StatementBegin
CREATE EXTENSION IF NOT EXISTS vector;

CREATE SCHEMA paradedb;

CREATE EXTENSION IF NOT EXISTS pg_search
WITH
  SCHEMA paradedb;

CREATE TABLE users (
  id uuid PRIMARY KEY DEFAULT uuidv7(),
  email text NOT NULL UNIQUE,
  created_at timestamptz NOT NULL DEFAULT now()
);

-- +goose StatementEnd
-- +goose Down
-- +goose StatementBegin
DROP TABLE users;

DROP EXTENSION IF EXISTS pg_search;

DROP SCHEMA IF EXISTS paradedb;

DROP EXTENSION IF EXISTS vector;

-- +goose StatementEnd
