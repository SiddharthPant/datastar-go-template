# Seed Data

## Goal

Add first-class seeders for orgs, teams, users, credentials, and memberships so
new projects can be scaffolded with useful data without clicking through UI
flows or hand-writing requests. Seeders should be deterministic, idempotent, and
safe to run repeatedly in local development and test environments.

## Seeder Shape

Add a dedicated command, for example:

```sh
go run ./cmd/seed
task db:seed
```

The command should read seed definitions from code or a small local fixture file
and create:

- org scopes and `orgs`
- team scopes and `teams`
- principals and `users`
- password credentials with Argon2id hashes
- email OTP credentials
- memberships that connect principals to org/team scopes

Do not seed sessions, OTP challenges, or password reset tokens. Those belong in
JetStream as runtime auth state and should be created only through auth flows.

## Default Local Fixture

Start with a tiny local fixture that covers real dashboard development:

- one org
- two teams in that org
- one owner user
- one admin user
- one regular member user
- password credentials for each user
- OTP credentials for each user
- memberships that exercise org-level and team-level access

Use predictable emails such as `owner@example.test`, but generate secure
password hashes through the same Argon2id helper used by production code. Local
plaintext seed passwords may live only in fixture/config intended for local dev
and tests.

## Idempotency Rules

Seeders must be upsert-like and keyed by stable natural identifiers:

- org slug
- team org plus team slug
- user email
- credential email
- membership scope plus principal

Running the seeder twice should not duplicate rows, rotate passwords
unexpectedly, or change roles unless the seed definition explicitly asks for
that update.

## Environment Safety

Seed commands should refuse to run in production by default. Require an explicit
environment check such as `APP_ENV=local`, `APP_ENV=dev`, or `APP_ENV=test`.

If a future production bootstrap command is needed, make it separate and
explicit, for example `cmd/bootstrap-admin`, so normal seed fixtures cannot
accidentally create demo users in production.

## Verification

Seeder completion means:

- migrations can run on an empty database;
- `task db:seed` creates the default org/team/user graph;
- running `task db:seed` again is a no-op or controlled update;
- seeded users can log in with email/password;
- seeded users can request email/OTP login;
- dashboard access works immediately after seeding;
- tests can use the same seeding package without depending on HTTP requests.

---

**Previous:** [Identity And Database Model](01-identity-and-database-model.md) · **Next:** [Auth Flows](03-auth-flows.md)
