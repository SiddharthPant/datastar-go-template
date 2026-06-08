# Seeders And Tests Code

## 1. Add `seed/seed.go`

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
	Email   string
	Name    string
	IsStaff bool
	Role    sqlc.MembershipRole
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
			{Email: "owner@example.test", Name: "Owner", IsStaff: true, Role: sqlc.MembershipRoleOwner},
			{Email: "admin@example.test", Name: "Admin", IsStaff: true, Role: sqlc.MembershipRoleAdmin},
			{Email: "member@example.test", Name: "Member", IsStaff: false, Role: sqlc.MembershipRoleMember},
		},
	}
}

func Run(ctx context.Context, db *pgxpool.Pool, fixture Fixture) error {
	if config.Global.Environment == config.Prod {
		return fmt.Errorf("refusing to seed in prod")
	}

	q := sqlc.New(db)

	org, err := q.GetOrgBySlug(ctx, fixture.OrgSlug)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("get org by slug: %w", err)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		scope, err := q.CreateOrgScopeForSeed(ctx)
		if err != nil {
			return fmt.Errorf("create org scope: %w", err)
		}
		org, err = q.UpsertOrgForSeed(ctx, sqlc.UpsertOrgForSeedParams{
			ScopeID: scope.ID,
			Name:    fixture.OrgName,
			Slug:    fixture.OrgSlug,
		})
		if err != nil {
			return fmt.Errorf("upsert org: %w", err)
		}
	}

	for _, teamSeed := range fixture.Teams {
		_, err := q.GetTeamByOrgAndSlug(ctx, sqlc.GetTeamByOrgAndSlugParams{OrgID: org.ID, Slug: teamSeed.Slug})
		if err == nil {
			continue
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("get team by org and slug: %w", err)
		}
		scope, err := q.CreateTeamScopeForSeed(ctx, org.ScopeID)
		if err != nil {
			return fmt.Errorf("create team scope: %w", err)
		}
		if _, err := q.UpsertTeamForSeed(ctx, sqlc.UpsertTeamForSeedParams{
			ScopeID:      scope.ID,
			OrgID:        org.ID,
			ParentTeamID: uuid.NullUUID{},
			Name:         teamSeed.Name,
			Slug:         teamSeed.Slug,
		}); err != nil {
			return fmt.Errorf("upsert team %s: %w", teamSeed.Slug, err)
		}
	}

	passwordHash, err := authcore.HashPassword(config.Global.SeedPassword)
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
			Role:        userSeed.Role,
		}); err != nil {
			return fmt.Errorf("upsert org membership: %w", err)
		}
	}

	return nil
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

## 2. Add `cmd/seed/main.go`

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

Use a real local NATS in integration tests, or keep this as the first table of
test cases until you add a KV fake:

```go
package auth

import "testing"

func TestSessionStateCases(t *testing.T) {
	t.Skip("requires JetStream KV test fixture")
}
```

Required cases once the fixture exists:

- create session stores only token hash as the key;
- valid session lookup succeeds;
- revoked session lookup fails;
- expired session lookup fails;
- OTP challenge can be consumed once;
- OTP challenge rejects wrong code and increments attempts;
- password reset token can be consumed once.

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
