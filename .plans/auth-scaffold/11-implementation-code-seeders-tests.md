# Seeders And Tests Code

## 1. Add `seed/seed.go`

The fixture gives every user an org-level role and, where set, team-level
roles, so seeded data exercises both membership scopes (doc 02). The org and
team upserts run on the already-exists path too, so renaming a fixture entry
propagates on re-run while slugs stay the stable natural keys.

```go
package seed

import (
	"context"
	"errors"
	"fmt"

	"datastar-go/config"
	"datastar-go/database/sqlc"
	authcore "datastar-go/internal/auth"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type UserSeed struct {
	Email     string
	Name      string
	IsStaff   bool
	OrgRole   sqlc.MembershipRole
	TeamRoles map[string]sqlc.MembershipRole // team slug -> role
}

type TeamSeed struct {
	Name string
	Slug string
}

type Fixture struct {
	OrgName string
	OrgSlug string
	Teams   []TeamSeed
	Users   []UserSeed
}

func DefaultFixture() Fixture {
	return Fixture{
		OrgName: "Acme",
		OrgSlug: "acme",
		Teams: []TeamSeed{
			{Name: "Platform", Slug: "platform"},
			{Name: "Operations", Slug: "operations"},
		},
		Users: []UserSeed{
			{Email: "owner@example.test", Name: "Owner", IsStaff: true, OrgRole: sqlc.MembershipRoleOwner},
			{Email: "admin@example.test", Name: "Admin", IsStaff: true, OrgRole: sqlc.MembershipRoleAdmin,
				TeamRoles: map[string]sqlc.MembershipRole{"platform": sqlc.MembershipRoleAdmin}},
			{Email: "member@example.test", Name: "Member", IsStaff: false, OrgRole: sqlc.MembershipRoleMember,
				TeamRoles: map[string]sqlc.MembershipRole{
					"platform":   sqlc.MembershipRoleMember,
					"operations": sqlc.MembershipRoleViewer,
				}},
		},
	}
}

func Run(ctx context.Context, db *pgxpool.Pool, fixture Fixture) error {
	if config.Env.AppEnv == config.Prod {
		return fmt.Errorf("refusing to seed in prod")
	}

	q := sqlc.New(db)

	orgScopeID, err := orgScopeIDForSeed(ctx, q, fixture.OrgSlug)
	if err != nil {
		return err
	}
	org, err := q.UpsertOrgForSeed(ctx, sqlc.UpsertOrgForSeedParams{
		ScopeID: orgScopeID,
		Name:    fixture.OrgName,
		Slug:    fixture.OrgSlug,
	})
	if err != nil {
		return fmt.Errorf("upsert org: %w", err)
	}

	teamScopes := make(map[string]uuid.UUID, len(fixture.Teams))
	for _, teamSeed := range fixture.Teams {
		scopeID, err := teamScopeIDForSeed(ctx, q, org, teamSeed.Slug)
		if err != nil {
			return err
		}
		team, err := q.UpsertTeamForSeed(ctx, sqlc.UpsertTeamForSeedParams{
			ScopeID:      scopeID,
			OrgID:        org.ID,
			ParentTeamID: uuid.NullUUID{},
			Name:         teamSeed.Name,
			Slug:         teamSeed.Slug,
		})
		if err != nil {
			return fmt.Errorf("upsert team %s: %w", teamSeed.Slug, err)
		}
		teamScopes[teamSeed.Slug] = team.ScopeID
	}

	passwordHash, err := authcore.HashPassword(config.Env.SeedPassword)
	if err != nil {
		return fmt.Errorf("hash seed password: %w", err)
	}

	for _, userSeed := range fixture.Users {
		principalID, err := principalIDForSeedUser(ctx, q, userSeed.Email)
		if err != nil {
			return err
		}

		user, err := q.UpsertUserForSeed(ctx, sqlc.UpsertUserForSeedParams{
			PrincipalID: principalID,
			Email:       userSeed.Email,
			Name:        userSeed.Name,
			IsStaff:     userSeed.IsStaff,
		})
		if err != nil {
			return fmt.Errorf("upsert user %s: %w", userSeed.Email, err)
		}

		if _, err := q.UpsertPasswordCredentialForSeed(ctx, sqlc.UpsertPasswordCredentialForSeedParams{
			PrincipalID:  user.PrincipalID,
			Email:        user.Email,
			PasswordHash: passwordHash,
		}); err != nil {
			return fmt.Errorf("upsert password credential: %w", err)
		}
		if _, err := q.UpsertOTPCredentialForSeed(ctx, sqlc.UpsertOTPCredentialForSeedParams{
			PrincipalID: user.PrincipalID,
			Email:       user.Email,
		}); err != nil {
			return fmt.Errorf("upsert otp credential: %w", err)
		}
		if _, err := q.UpsertMembershipForSeed(ctx, sqlc.UpsertMembershipForSeedParams{
			ScopeID:     org.ScopeID,
			PrincipalID: user.PrincipalID,
			Role:        userSeed.OrgRole,
		}); err != nil {
			return fmt.Errorf("upsert org membership: %w", err)
		}

		for teamSlug, role := range userSeed.TeamRoles {
			scopeID, ok := teamScopes[teamSlug]
			if !ok {
				return fmt.Errorf("user %s references unknown team %q", userSeed.Email, teamSlug)
			}
			if _, err := q.UpsertMembershipForSeed(ctx, sqlc.UpsertMembershipForSeedParams{
				ScopeID:     scopeID,
				PrincipalID: user.PrincipalID,
				Role:        role,
			}); err != nil {
				return fmt.Errorf("upsert team membership %s/%s: %w", userSeed.Email, teamSlug, err)
			}
		}
	}

	return nil
}

func orgScopeIDForSeed(ctx context.Context, q *sqlc.Queries, slug string) (uuid.UUID, error) {
	org, err := q.GetOrgBySlug(ctx, slug)
	if err == nil {
		return org.ScopeID, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, fmt.Errorf("get org by slug: %w", err)
	}
	scope, err := q.CreateOrgScopeForSeed(ctx)
	if err != nil {
		return uuid.Nil, fmt.Errorf("create org scope: %w", err)
	}
	return scope.ID, nil
}

func teamScopeIDForSeed(ctx context.Context, q *sqlc.Queries, org sqlc.Org, slug string) (uuid.UUID, error) {
	team, err := q.GetTeamByOrgAndSlug(ctx, sqlc.GetTeamByOrgAndSlugParams{OrgID: org.ID, Slug: slug})
	if err == nil {
		return team.ScopeID, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, fmt.Errorf("get team by org and slug: %w", err)
	}
	// scopes.parent_id is nullable, so sqlc generates a uuid.NullUUID param.
	scope, err := q.CreateTeamScopeForSeed(ctx, uuid.NullUUID{UUID: org.ScopeID, Valid: true})
	if err != nil {
		return uuid.Nil, fmt.Errorf("create team scope: %w", err)
	}
	return scope.ID, nil
}

func principalIDForSeedUser(ctx context.Context, q *sqlc.Queries, email string) (uuid.UUID, error) {
	principal, err := q.GetPrincipalByUserEmail(ctx, email)
	if err == nil {
		return principal.ID, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, fmt.Errorf("get principal by user email: %w", err)
	}

	principal, err = q.CreatePrincipal(ctx, sqlc.PrincipalKindUser)
	if err != nil {
		return uuid.Nil, fmt.Errorf("create principal for seed user: %w", err)
	}
	return principal.ID, nil
}
```

## 2. Replace `cmd/seed/main.go`

A WIP version exists that creates a single principal directly; replace it with
the fixture-driven runner:

```go
package main

import (
	"context"
	"log/slog"
	"os"

	"datastar-go/database"
	"datastar-go/seed"
)

func main() {
	ctx := context.Background()
	if err := run(ctx); err != nil {
		slog.Error("seed failed", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	db, err := database.New(ctx)
	if err != nil {
		return err
	}
	defer db.Close()

	return seed.Run(ctx, db, seed.DefaultFixture())
}
```

## 3. Add `internal/auth/password_test.go`

```go
package auth

import "testing"

func TestPasswordHashVerify(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}

	ok, err := VerifyPassword("correct horse battery staple", hash)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected password to verify")
	}

	ok, err = VerifyPassword("wrong", hash)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("expected wrong password to fail")
	}
}
```

## 4. Add `internal/auth/token_test.go`

```go
package auth

import "testing"

func TestTokenHashIsStableAndOpaque(t *testing.T) {
	token := "secret"
	if TokenHash(token) != TokenHash(token) {
		t.Fatal("token hash should be stable")
	}
	if TokenHash(token) == token {
		t.Fatal("token hash should not equal token")
	}
}

func TestNewPID(t *testing.T) {
	pid, err := NewPID("usr")
	if err != nil {
		t.Fatal(err)
	}
	if len(pid) <= len("usr_") || pid[:4] != "usr_" {
		t.Fatalf("unexpected pid %q", pid)
	}
}
```

## 5. Add `features/auth/state_test.go`

No KV fake: `nats-server` embeds in-process, so every test gets a real
JetStream in milliseconds with no Docker dependency. The same fixture serves
the rate limiter and CAS helper tests in `internal/auth`.

```go
package auth

import (
	"context"
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	authcore "datastar-go/internal/auth"

	"github.com/google/uuid"
)

func newTestKV(t *testing.T, bucket string) jetstream.KeyValue {
	t.Helper()

	srv, err := server.NewServer(&server.Options{
		Port:      -1,
		JetStream: true,
		StoreDir:  t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}
	go srv.Start()
	if !srv.ReadyForConnections(5 * time.Second) {
		t.Fatal("embedded nats server not ready")
	}
	t.Cleanup(srv.Shutdown)

	nc, err := nats.Connect(srv.ClientURL())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(nc.Close)

	js, err := jetstream.New(nc)
	if err != nil {
		t.Fatal(err)
	}
	kv, err := js.CreateKeyValue(context.Background(), jetstream.KeyValueConfig{Bucket: bucket})
	if err != nil {
		t.Fatal(err)
	}
	return kv
}

func TestSessionLifecycle(t *testing.T) {
	kv := newTestKV(t, "sessions")
	store := NewStateStore(kv, nil, nil)
	ctx := context.Background()
	principalID := uuid.New()

	token, record, err := store.CreateSession(ctx, principalID, "password", "127.0.0.1", "test-agent")
	if err != nil {
		t.Fatal(err)
	}
	if record.PrincipalID != principalID {
		t.Fatalf("unexpected principal %s", record.PrincipalID)
	}
	if _, err := kv.Get(ctx, token); err == nil {
		t.Fatal("session must be keyed by token hash, not the raw token")
	}
	if _, err := kv.Get(ctx, authcore.TokenHash(token)); err != nil {
		t.Fatalf("expected session under token hash: %v", err)
	}

	got, err := store.GetSession(ctx, token, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if got.PID != record.PID {
		t.Fatalf("got session %s, want %s", got.PID, record.PID)
	}

	if err := store.RevokeSession(ctx, token); err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetSession(ctx, token, "127.0.0.1"); err == nil {
		t.Fatal("revoked session must not authenticate")
	}
}
```

Remaining cases on the same fixture:

- expired session lookup fails (inject `now` via the `StateStore.now` field);
- OTP challenge can be consumed exactly once;
- OTP challenge rejects a wrong code and increments the attempt count;
- the attempt cap holds under concurrent wrong guesses;
- password reset token can be consumed exactly once;
- `RateLimiter.Allow` denies once over the limit and resets after the window
  (in `internal/auth`, reusing the same embedded-server helper);
- `CASUpdate`: missing key without create returns not-found, apply error
  aborts without writing, contention returns `ErrCASContention`.

## 6. Verification Commands

```sh
task db:migrate
task db:seed
task sqlc:generate
task sqlc:vet
task templ
go test ./...
task dev
```

Manual browser checks:

- visit `/`, confirm redirect to `/auth/login`;
- log in with `owner@example.test` and `SEED_PASSWORD`;
- confirm dashboard renders;
- click logout;
- request an OTP and inspect Mailpit;
- request a password reset and inspect Mailpit;
- reset the password and confirm old session is invalidated.

---

**Previous:** [Auth Feature And Routing Code](10-implementation-code-feature.md) · **End of plan** — back to [Auth Scaffold Plan](README.md)
