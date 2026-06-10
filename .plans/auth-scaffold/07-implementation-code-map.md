# Implementation Code Map

This is the paste-and-build section of the auth plan. It is written for a
human implementing the scaffold manually, file by file.

Use these docs in order:

1. [Database And Queries Code](08-implementation-code-database.md)
2. [Core Auth And JetStream Code](09-implementation-code-core.md)
3. [Auth Feature And Routing Code](10-implementation-code-feature.md)
4. [Seeders And Tests Code](11-implementation-code-seeders-tests.md)

## Current State (2026-06-10)

Implementation has started against the **old, richer schema**, so the first
step is shrinking what exists — reconcile each section against the working
tree, do not paste blindly:

- `database/migrations/00001_init.sql` contains the extensions and the
  `prefixed_nanoid` / `is_prefixed_pid` / `update_updated_at_column` helpers.
  It is unchanged by this plan.
- `database/migrations/00002_auth_identity.sql` is committed with the old
  principal/scope/org/membership schema. Doc 08 §1 replaces it in place with
  the three-table schema (`teams`, `users`, `team_memberships`); the local
  database must then be rebuilt (`task db:nuke` or the drop/recreate reset
  task).
- `database/queries/seed.sql` and `cmd/seed/main.go` exist as WIP versions
  that create principals. Docs 08 §3 and 11 §1 replace them. Keep their
  established pattern: hardcoded UUID literals, `ON CONFLICT (id) DO
  UPDATE`, one transaction.
- `database/sqlc/` still contains generated code for the old schema
  (enums, `Principal`, `Scope`, …). Regenerating after the migration/query
  rewrite removes all of it.
- `features/auth` exists with a stub login page/handler (the stub logs the
  raw password — replace it, do not extend it). Doc 10 replaces these files.
- `config/config.go` already has `AppBaseURL`, `SMTPAddr`, `SMTPFrom`, and
  `SeedPassword` as **required** env vars, and `.env.example` includes them.
  The config API is `config.Env.<Field>`, and the environment check is
  `config.Env.AppEnv == config.Prod`.
- Nullable `timestamptz` columns generate as `pgtype.Timestamptz` (check
  `.Valid` / use `.Time`); nullable uuid columns generate as `uuid.NullUUID`.
  Docs 10 and 11 are written against this.

Important constraints:

- **No third-party dependencies.** Allowed: the standard library,
  `golang.org/x/*` reference packages, and deps the project already has
  (pgx/sqlc, chi, nats.go, templ, datastar-go, google/uuid). The only
  `go.mod` change in this plan is promoting `golang.org/x/crypto` (already
  in the module graph as an indirect dependency) to direct:

  ```sh
  go get golang.org/x/crypto
  ```

  An `x` package must also actually fit the requirement: `x/time/rate` was
  evaluated for rate limiting and rejected because its in-memory state
  cannot be shared through NATS (doc 04's table).
- Use UUIDv7 for internal database IDs, prefixed NanoID-style PIDs for
  UI/external references.
- Do not create Postgres tables for sessions or password reset tokens; that
  state lives in JetStream, along with rate limits and auth events.
- Use Datastar from the first pass for auth interactions.
- Regenerate `sqlc` and `templ` output after adding SQL or `.templ` files.

---

**Previous:** [Testing And Completion](06-testing-and-completion.md) · **Next:** [Database And Queries Code](08-implementation-code-database.md)
