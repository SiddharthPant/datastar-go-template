# datastar-go

An exploration of **Go + Datastar + NATS** as a production-oriented application
scaffold.

This repository is a working playground for building server-rendered,
event-driven Go applications with a small operational footprint. The current
goal is to pressure-test the stack, learn where the boundaries should sit, and
shape this into a default starter template for future projects.

The longer-term direction is to use this codebase to explore practical
event-sourcing and CQRS patterns: durable event streams, command handling,
read-model projections, idempotent consumers, and boring, repeatable deployment
habits.

## Stack

- **Go** for the application runtime.
- **chi** for HTTP routing and middleware.
- **templ** for typed server-rendered HTML components.
- **Datastar** for reactive HTML updates over SSE without a SPA runtime.
- **NATS + JetStream** for application messaging and durable streams.
- **Postgres + pgx** for relational storage.
- **sqlc** for generated, type-safe database access.
- **goose** for migrations.
- **Docker Compose** for local infrastructure.

## What Exists Today

The app currently boots a web server, connects to Postgres, connects to NATS,
ensures a demo JetStream stream exists, and serves a small Datastar-powered page.

That page exercises the main pieces of the scaffold:

- server-rendered HTML via templ;
- Datastar SSE patches for UI updates;
- a Postgres health query through sqlc;
- a NATS request/reply ping;
- a JetStream publish path with message counting.

This is intentionally small, but it gives each important production concern a
real place in the codebase.

## Project Layout

```text
cmd/
  web/                 # HTTP application entrypoint
  migrate/             # Embedded goose migration runner

config/                # Environment-driven application config
database/
  migrations/          # Goose migrations
  queries/             # sqlc query definitions
  sqlc/                # Generated sqlc package

features/
  index/               # Example vertical slice: routes, handlers, service, pages

natsx/                 # NATS connection and JetStream setup
router/                # HTTP route assembly and static asset wiring
web/resources/         # Static assets, embedded in prod and direct-served in dev

compose.yml            # Local Postgres, NATS, Mailpit, RustFS, observability
Taskfile.yml           # Common development commands
sqlc.yml               # sqlc configuration
```

## Local Development

Prerequisites:

- Go 1.26+
- Docker with Compose
- Task

Create your local environment file:

```sh
cp .env.example .env
```

Start the core local infrastructure:

```sh
docker compose up -d postgres nats mailpit rustfs
```

Apply database migrations:

```sh
task db:migrate
```

Run the web app in development mode:

```sh
task dev
```

The application defaults to `http://localhost:8080`.

Useful local service ports:

- Postgres: `localhost:5432`
- NATS: `localhost:4222`
- NATS monitoring: `http://localhost:8222`
- Mailpit: `http://localhost:8025`
- RustFS S3 API: `http://localhost:9000`
- RustFS console: `http://localhost:9001`

To start the optional observability stack:

```sh
docker compose up -d observability
```

Grafana LGTM is exposed at `http://localhost:8080`, which conflicts with the
app's default port. If both are running locally, change either `PORT` in `.env`
or the observability port mapping in `compose.yml`.

## Common Commands

```sh
task dev            # Run the web server with dev build tags
task run            # Run the web server with prod-style build tags
task gen            # Regenerate sqlc and templ output
task templ          # Regenerate templ output
task templ:watch    # Watch .templ files and regenerate on change
task sqlc:generate  # Regenerate database/sqlc
task sqlc:vet       # Vet sqlc queries
task sqlc:diff      # Check generated sqlc output for drift
task db:migrate     # Apply migrations
task db:rollback    # Roll back the last migration
task db:status      # Show migration status
task db:new -- name # Create a new SQL migration
```

There is also an embedded migration runner:

```sh
go run ./cmd/migrate
go run ./cmd/migrate status
go run ./cmd/migrate down
```

## Development Model

The intended shape is vertical slices under `features/`.

Each feature can own its:

- routes;
- handlers;
- application service;
- templ pages and fragments;
- commands, queries, and projections as the CQRS model evolves.

Shared infrastructure should stay small and explicit:

- `database` owns Postgres connection and migration embedding;
- `natsx` owns NATS and JetStream setup;
- `router` wires cross-cutting HTTP concerns;
- `config` owns environment parsing.

This keeps feature code close to user-facing behavior while still giving
production infrastructure stable homes.

## Event Sourcing And CQRS Direction

The current JetStream demo is the seed for the eventing model. The direction is
to evolve toward:

- commands that validate intent and append domain events;
- JetStream streams as durable event logs where appropriate;
- Postgres tables for transactional state and read models;
- projections that consume events and maintain query-optimized views;
- idempotent message handling with explicit message IDs;
- request handlers that either execute commands or read from projections;
- background workers for async policies, integrations, and projection rebuilds.

The core question this repository should answer is not "can this pattern work?"
but "what is the smallest version of this pattern that stays production-ready?"

## Template Goals

As this becomes a default scaffold, new projects should be able to inherit:

- a predictable Go package layout;
- local infrastructure that starts with one command;
- typed database access and migrations from day one;
- a server-rendered UI path that remains reactive without becoming a SPA;
- a NATS/JetStream foundation for async workflows;
- production-minded config, logging, and shutdown behavior;
- a clear place to add auth, jobs, media storage, observability, and deployment.

The scaffold should stay boring in the best way: easy to clone, easy to reason
about, easy to replace in pieces, and hard to accidentally turn into a tangle.

## Configuration

Configuration currently loads `.env` on startup and then reads environment
variables with local-friendly defaults. Keep a `.env` file present for local
runs.

Important variables include:

- `HOST` and `PORT`
- `LOG_LEVEL`
- `DATABASE_URL`
- `DATABASE_MAX_CONNECTIONS`
- `DATABASE_MIN_CONNECTIONS`
- `DATABASE_CONNECT_TIMEOUT`
- `DATABASE_IDLE_TIMEOUT`
- `NATS_URL`
- `NATS_NAME`
- `NATS_CONNECT_TIMEOUT`

See `.env.example` for the current local defaults.

## Production Notes

This repo is still an exploration, but the production shape is already visible:

- graceful HTTP shutdown with `errgroup`;
- structured JSON logs through `slog`;
- direct static asset serving in dev;
- embedded, hash-addressed static assets in prod;
- embedded database migrations;
- explicit Postgres pool settings;
- NATS drain on shutdown;
- JetStream stream creation on boot.

Before using this as a production template, the next pieces to settle are auth,
session storage, environment-only config loading, observability conventions,
background worker supervision, event schema/versioning policy, deployment
packaging, and a test strategy for commands, projections, and HTTP flows.
