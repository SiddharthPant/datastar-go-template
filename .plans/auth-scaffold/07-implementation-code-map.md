# Implementation Code Map

This is the paste-and-build section of the auth plan. It is written for a human
implementing the scaffold manually, file by file.

Use these docs in order:

1. [Database And Queries Code](08-implementation-code-database.md)
2. [Core Auth And JetStream Code](09-implementation-code-core.md)
3. [Auth Feature And Routing Code](10-implementation-code-feature.md)
4. [Seeders And Tests Code](11-implementation-code-seeders-tests.md)

## Current State (2026-06-10)

Implementation has started, so do not paste these docs blindly — reconcile each
section against the working tree first:

- `database/migrations/00001_init.sql` already contains the extensions and the
  `prefixed_nanoid` / `is_prefixed_pid` / `update_updated_at_column` helpers.
- `database/migrations/00002_auth_identity.sql` already exists and creates the
  full identity schema from doc 08.
- `database/queries/auth.sql` was removed in commit 5e82b5e and must be
  re-added; the working tree already has the matching `database/sqlc/auth.sql.go`
  deletion pending.
- `database/queries/seed.sql` and `cmd/seed/main.go` exist as WIP subsets and
  should be replaced with the full versions in docs 08 and 11.
- `features/auth` exists with a stub login page/handler (it logs the raw
  password — replace it, do not extend it). Doc 10 replaces these files.
- `config/config.go` already has `AppBaseURL`, `SMTPAddr`, `SMTPFrom`, and
  `SeedPassword` as **required** env vars, and `.env.example` already includes
  them. The config API is `config.Env.<Field>` (not `config.Global`), and the
  environment check is `config.Env.AppEnv == config.Prod`.
- Nullable `timestamptz` columns generate as `pgtype.Timestamptz` (check
  `.Valid` / use `.Time`), not pointer types. Doc 10's service code is written
  against this.

Important constraints:

- Use UUIDv7 for internal database IDs.
- Use prefixed NanoID-style PIDs for UI/external references.
- Do not create Postgres tables for sessions, OTP challenges, or password reset
  tokens.
- Store session state, OTP challenges, password reset tokens, auth rate limits,
  and auth events in JetStream.
- Use Datastar from the first pass for auth interactions.
- Regenerate `sqlc` and `templ` output after adding SQL or `.templ` files.

The schema assumed this project was early enough that replacing the toy
`users` table was acceptable; the committed `00002_auth_identity.sql` already
did this.

Before implementing the Go files, add the password hashing library (a thin,
well-tested wrapper over `golang.org/x/crypto/argon2` that owns PHC encoding,
decoding, and constant-time comparison) and the embedded NATS server used by
the JetStream tests:

```sh
go get github.com/alexedwards/argon2id
go get github.com/nats-io/nats-server/v2
```

---

**Previous:** [Testing And Completion](06-testing-and-completion.md) · **Next:** [Database And Queries Code](08-implementation-code-database.md)
