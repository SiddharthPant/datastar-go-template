# Seed Data

## Goal

Seed teams, users, and memberships so a fresh local database can be used to
log in immediately and exercises every role's access shape. Seeding must be
deterministic, idempotent, and safe to run repeatedly.

## Seeder Shape

One self-contained command:

```sh
go run ./cmd/seed
task db:seed
```

The seed data is a literal slice in `cmd/seed/main.go` — no fixture files, no
separate seed package. With three tables there is nothing to abstract.

The seeder creates:

- teams (upserted by hardcoded UUID)
- users (upserted by hardcoded UUID), each with a real Argon2id hash of
  `SEED_PASSWORD` produced by the same `internal/auth` helper production code
  uses
- team memberships (upserted by hardcoded UUID) wiring admins and members to
  their teams — owners get none, their access is derived

Do not seed sessions, reset tokens, or rate-limit state. Those belong in
JetStream as runtime auth state and are created only through auth flows.

## Determinism And Idempotency

Every seeded row carries a hardcoded UUID literal in the seed slice, and every
insert is `ON CONFLICT (id) DO UPDATE`. This is the pattern already settled on
in the working tree:

- IDs never depend on database defaults (`uuidv7()` is random per run), so
  references between seed entries (`team_memberships.team_id` → `teams.id`)
  are plain literals with no lookups
- re-running the seeder converges the rows to the seed definition instead of
  duplicating or skipping them
- the upsert updates every non-key column, so editing a seed entry's name,
  role, or membership propagates on the next run

The password hash is re-computed each run with a fresh random salt. That
rewrites the `password_hash` column on every seed run, which is harmless: the
password itself is unchanged, and seeds only run in dev/test.

The seeder also validates the member-one-team invariant (doc 01) before
writing: a seed definition that gives a `member` two memberships fails fast
with a clear error rather than planting data the service layer considers
illegal.

Wrap the whole run in one transaction so a failure midway leaves the database
untouched.

## Default Seed Set

One user per role, shaped to exercise each access pattern:

- two teams: `platform` and `operations`
- `owner@example.com` — role `owner`, **no membership rows**; sees both teams
  through derived access
- `admin@example.com` — role `admin`, members of **both** teams (exercises
  multi-team membership)
- `member@example.com` — role `member`, member of `platform` only (exercises
  the one-team rule)

All users share `SEED_PASSWORD` from config. Plaintext seed passwords exist
only in local `.env`.

## Environment Safety

The seeder refuses to run when `config.Env.AppEnv == config.Prod` and exits
non-zero. If a production bootstrap command is ever needed, make it a
separate explicit command (e.g. `cmd/bootstrap-admin`) so dev fixtures cannot
leak into production.

## Verification

Seeder completion means:

- migrations apply on an empty database
- `task db:seed` creates the team/user/membership set
- running `task db:seed` twice leaves exactly the same rows (modulo
  `password_hash` salt and `updated_at`)
- seeded users can log in with email + `SEED_PASSWORD`
- `TeamsForUser` returns both teams for the owner and the admin, and exactly
  one team for the member
- dashboard access works immediately after seeding

---

**Previous:** [Identity And Database Model](01-identity-and-database-model.md) · **Next:** [Auth Flows](03-auth-flows.md)
