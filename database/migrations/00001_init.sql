-- +goose Up
-- +goose StatementBegin
CREATE EXTENSION vector;
CREATE EXTENSION pgcrypto;
CREATE EXTENSION citext;


CREATE SCHEMA paradedb;
CREATE EXTENSION pg_search
WITH
  SCHEMA paradedb;


CREATE FUNCTION prefixed_nanoid (
  prefix text DEFAULT 'id',
  size int DEFAULT 16,
  alphabet text DEFAULT '123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz'
) RETURNS text LANGUAGE plpgsql VOLATILE PARALLEL SAFE AS $$
DECLARE id_builder text := '';
  counter int := 0;
  bytes bytea;
  alphabet_index int;
  alphabet_array text [ ];
  alphabet_length int := 64;
  mask int := 63;
  step int := 34;
BEGIN alphabet_array := regexp_split_to_array(alphabet, '');
  alphabet_length := array_length(alphabet_array, 1);

  LOOP bytes := gen_random_bytes(step);
    FOR counter IN 0 ..step - 1
    LOOP alphabet_index := (get_byte(bytes, counter) & mask) + 1;
      IF alphabet_index <= alphabet_length THEN id_builder := id_builder || alphabet_array [ alphabet_index ];
        IF length(id_builder) = size THEN RETURN prefix || '_' || id_builder;
        END IF;
      END IF;
    END LOOP;
  END LOOP;
END $$;
CREATE FUNCTION is_prefixed_pid (value text, prefix text, size int DEFAULT 16) RETURNS boolean LANGUAGE sql IMMUTABLE STRICT AS $$
SELECT value ~ (
    '^' || prefix || '_[1-9A-HJ-NP-Za-km-z]{' || size || '}$'
  ) $$;


CREATE FUNCTION update_updated_at_column () RETURNS trigger AS $$
BEGIN NEW .updated_at = now();
RETURN NEW;
END;
$$ language plpgsql;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP FUNCTION IF EXISTS prefixed_nanoid (text, int, text);
DROP FUNCTION IF EXISTS is_prefixed_pid (text, text, int);

DROP FUNCTION IF EXISTS update_updated_at_column ();

DROP SCHEMA IF EXISTS paradedb CASCADE;
DROP EXTENSION IF EXISTS pg_search CASCADE;

DROP EXTENSION IF EXISTS vector CASCADE;
DROP EXTENSION IF EXISTS pgcrypto CASCADE;
DROP EXTENSION IF EXISTS citext CASCADE;
-- +goose StatementEnd