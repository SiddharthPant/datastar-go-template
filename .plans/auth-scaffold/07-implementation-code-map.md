# Implementation Code Map

This is the paste-and-build section of the auth plan. It is written for a human
implementing the scaffold manually, file by file.

Use these docs in order:

1. [Database And Queries Code](08-implementation-code-database.md)
2. [Core Auth And JetStream Code](09-implementation-code-core.md)
3. [Auth Feature And Routing Code](10-implementation-code-feature.md)
4. [Seeders And Tests Code](11-implementation-code-seeders-tests.md)

Important constraints:

- Use UUIDv7 for internal database IDs.
- Use prefixed NanoID-style PIDs for UI/external references.
- Do not create Postgres tables for sessions, OTP challenges, or password reset
  tokens.
- Store session state, OTP challenges, password reset tokens, auth rate limits,
  and auth events in JetStream.
- Use Datastar from the first pass for auth interactions.
- Regenerate `sqlc` and `templ` output after adding SQL or `.templ` files.

The code below assumes this project is still early enough that replacing the
toy `users` table from `database/migrations/00001_init.sql` is acceptable. If
you have real data, write a careful migration instead of dropping that table.

Before implementing the Go files, make `golang.org/x/crypto` a direct
dependency because Argon2id is part of the auth boundary:

```sh
go get golang.org/x/crypto
```

---

**Previous:** [Testing And Completion](06-testing-and-completion.md) · **Next:** [Database And Queries Code](08-implementation-code-database.md)
