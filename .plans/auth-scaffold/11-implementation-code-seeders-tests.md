# Seeders And Tests Code

## 1. Replace `cmd/seed/main.go`

Self-contained, fixture-in-code, deterministic, transactional. This keeps the
pattern already established in the WIP version (hardcoded UUID literals, one
transaction, prod refusal) and extends it to teams, memberships, roles, and
password hashes. No separate seed package — three tables do not need one.

```go
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"datastar-go/config"
	"datastar-go/database"
	"datastar-go/database/sqlc"
	"datastar-go/features/auth"
	authcore "datastar-go/internal/auth"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Hardcoded UUIDs keep the seeder deterministic: re-runs converge via
// ON CONFLICT (id) DO UPDATE, and references between seed entries are plain
// literals.
var (
	teamPlatform   = uuid.MustParse("019eb1d2-c726-7aaa-8aaa-000000000001")
	teamOperations = uuid.MustParse("019eb1d2-c726-7aaa-8aaa-000000000002")

	userOwner  = uuid.MustParse("019eb1d2-c71c-7c59-8ff9-ba86da8fdfef")
	userAdmin  = uuid.MustParse("019eb1d2-c721-7bc6-babd-b361feb4074f")
	userMember = uuid.MustParse("019eb1d2-c723-760d-9f9b-08bb65de6d77")
)

type teamSeed struct {
	ID   uuid.UUID
	Name string
	Slug string
}

type userSeed struct {
	ID    uuid.UUID
	Email string
	Name  string
	Role  sqlc.UserRole
}

type membershipSeed struct {
	ID     uuid.UUID
	TeamID uuid.UUID
	UserID uuid.UUID
}

var seedTeams = []teamSeed{
	{ID: teamPlatform, Name: "Platform", Slug: "platform"},
	{ID: teamOperations, Name: "Operations", Slug: "operations"},
}

// One user per role: the owner has no membership rows (access to all teams
// is derived), the admin is on both teams, the member on exactly one.
var seedUsers = []userSeed{
	{ID: userOwner, Email: "owner@example.com", Name: "Owner User", Role: sqlc.UserRoleOwner},
	{ID: userAdmin, Email: "admin@example.com", Name: "Admin User", Role: sqlc.UserRoleAdmin},
	{ID: userMember, Email: "member@example.com", Name: "Member User", Role: sqlc.UserRoleMember},
}

var seedMemberships = []membershipSeed{
	{ID: uuid.MustParse("019eb1d2-c727-7bbb-8bbb-000000000001"), TeamID: teamPlatform, UserID: userAdmin},
	{ID: uuid.MustParse("019eb1d2-c727-7bbb-8bbb-000000000002"), TeamID: teamOperations, UserID: userAdmin},
	{ID: uuid.MustParse("019eb1d2-c727-7bbb-8bbb-000000000003"), TeamID: teamPlatform, UserID: userMember},
}

func main() {
	if config.Env.AppEnv == config.Prod {
		slog.Error("refusing to seed in prod")
		os.Exit(1)
	}
	ctx := context.Background()

	db, err := database.New(ctx)
	if err != nil {
		slog.Error("connect database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	if err := seed(ctx, db); err != nil {
		slog.Error("seed", "error", err)
		os.Exit(1)
	}
	slog.Info("seeding is complete")
}

// validateSeeds enforces the member-one-team invariant (doc 01) before
// anything touches the database, using the same rule the service exposes.
func validateSeeds() error {
	counts := make(map[uuid.UUID]int, len(seedUsers))
	for _, m := range seedMemberships {
		counts[m.UserID]++
	}
	for _, u := range seedUsers {
		if err := auth.ValidateMembershipCount(u.Role, counts[u.ID]); err != nil {
			return fmt.Errorf("seed user %s: %w", u.Email, err)
		}
	}
	return nil
}

func seed(ctx context.Context, db *pgxpool.Pool) error {
	if err := validateSeeds(); err != nil {
		return err
	}

	// One hash for all seed users: same password, and hashing is the slow
	// part by design.
	passwordHash, err := authcore.HashPassword(config.Env.SeedPassword)
	if err != nil {
		return fmt.Errorf("hash seed password: %w", err)
	}

	tx, err := db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	q := sqlc.New(tx)

	for _, t := range seedTeams {
		team, err := q.UpsertTeamForSeed(ctx, sqlc.UpsertTeamForSeedParams{
			ID:   t.ID,
			Name: t.Name,
			Slug: t.Slug,
		})
		if err != nil {
			return fmt.Errorf("upsert team %s: %w", t.Slug, err)
		}
		slog.Info("seeded team", "slug", team.Slug, "pid", team.Pid)
	}

	for _, u := range seedUsers {
		user, err := q.UpsertUserForSeed(ctx, sqlc.UpsertUserForSeedParams{
			ID:           u.ID,
			Email:        u.Email,
			Name:         u.Name,
			Role:         u.Role,
			PasswordHash: passwordHash,
		})
		if err != nil {
			return fmt.Errorf("upsert user %s: %w", u.Email, err)
		}
		slog.Info("seeded user", "email", user.Email, "pid", user.Pid, "role", user.Role)
	}

	for _, m := range seedMemberships {
		membership, err := q.UpsertTeamMembershipForSeed(ctx, sqlc.UpsertTeamMembershipForSeedParams{
			ID:     m.ID,
			TeamID: m.TeamID,
			UserID: m.UserID,
		})
		if err != nil {
			return fmt.Errorf("upsert membership %s: %w", m.ID, err)
		}
		slog.Info("seeded membership", "pid", membership.Pid)
	}

	return tx.Commit(ctx)
}
```

## 2. Add `internal/auth/password_test.go`

```go
package auth

import (
	"errors"
	"strings"
	"testing"
)

func TestPasswordHashVerify(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(hash, "$argon2id$v=19$") {
		t.Fatalf("unexpected hash encoding: %s", hash)
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

// TestGoldenHash pins a known-good PHC string so refactors of the encoding
// or the argon2 import can never silently break verification of hashes
// already stored in the database. Generate it once after the crypto code
// lands (small throwaway main calling HashPassword) and paste it here.
func TestGoldenHash(t *testing.T) {
	const golden = "PASTE-GENERATED-PHC-STRING-HERE"
	if strings.HasPrefix(golden, "PASTE") {
		t.Skip("golden hash not generated yet")
	}
	ok, err := VerifyPassword("golden-test-password", golden)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("golden hash no longer verifies — stored hashes would break")
	}
}

func TestVerifyRejectsMalformedHashes(t *testing.T) {
	for _, encoded := range []string{
		"",
		"not-a-hash",
		"$argon2i$v=19$m=65536,t=3,p=4$AAAA$AAAA",
		"$argon2id$v=19$m=65536,t=3,p=4$!!!$AAAA",
		"$argon2id$v=19$m=65536,t=3,p=4$AAAA$",
	} {
		if _, err := VerifyPassword("x", encoded); !errors.Is(err, ErrInvalidHash) {
			t.Errorf("expected ErrInvalidHash for %q, got %v", encoded, err)
		}
	}
}

func TestPasswordNeedsRehash(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if PasswordNeedsRehash(hash) {
		t.Fatal("fresh hash must not need rehash")
	}
	outdated := strings.Replace(hash, "m=65536", "m=32768", 1)
	if !PasswordNeedsRehash(outdated) {
		t.Fatal("hash with outdated params must need rehash")
	}
	if !PasswordNeedsRehash("garbage") {
		t.Fatal("undecodable hash must need rehash")
	}
}

func TestValidatePassword(t *testing.T) {
	if err := ValidatePassword("short"); !errors.Is(err, ErrPasswordTooShort) {
		t.Fatal("expected too-short rejection")
	}
	// 12 runes, more than 12 bytes — length is counted in runes.
	if err := ValidatePassword("ありがとう12345678"); err != nil {
		t.Fatal(err)
	}
	if err := ValidatePassword("a-long-enough-password"); err != nil {
		t.Fatal(err)
	}
}
```

## 3. Add `internal/auth/token_test.go`

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
	if len(pid) != len("usr_")+16 || pid[:4] != "usr_" {
		t.Fatalf("unexpected pid %q", pid)
	}
}
```

## 4. Add `features/auth/state_test.go`

No KV fake and no embedded-server dependency: tests connect to the NATS
already running from `compose.yml` and skip with a clear message when it is
down. Each test gets a uniquely named bucket and deletes it in cleanup. The
same helper pattern serves the `CASUpdate` and `RateLimiter` tests in
`internal/auth` (duplicate the small helper there; a shared testutil package
is not worth it yet).

```go
package auth

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	authcore "datastar-go/internal/auth"

	"github.com/google/uuid"
)

func newTestKV(t *testing.T) jetstream.KeyValue {
	t.Helper()

	url := os.Getenv("NATS_URL")
	if url == "" {
		url = nats.DefaultURL
	}
	nc, err := nats.Connect(url, nats.Timeout(2*time.Second))
	if err != nil {
		t.Skipf("nats unavailable at %s (run `docker compose up -d nats`): %v", url, err)
	}
	t.Cleanup(nc.Close)

	js, err := jetstream.New(nc)
	if err != nil {
		t.Fatal(err)
	}

	bucket := "test_" + strings.NewReplacer("/", "_", "#", "_").Replace(t.Name())
	kv, err := js.CreateKeyValue(context.Background(), jetstream.KeyValueConfig{Bucket: bucket})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = js.DeleteKeyValue(context.Background(), bucket)
	})
	return kv
}

func TestSessionLifecycle(t *testing.T) {
	kv := newTestKV(t)
	store := NewStateStore(kv, nil)
	ctx := context.Background()
	userID := uuid.New()

	token, record, err := store.CreateSession(ctx, userID, "127.0.0.1", "test-agent")
	if err != nil {
		t.Fatal(err)
	}
	if record.UserID != userID {
		t.Fatalf("unexpected user %s", record.UserID)
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
- password reset token can be consumed exactly once; a second
  `ConsumePasswordReset` returns `ErrInvalidResetToken`;
- `RateLimiter.Allow` denies once over the limit and resets after the window
  (in `internal/auth`, reusing the same compose-NATS helper);
- `CASUpdate`: missing key without create returns not-found, apply error
  aborts without writing, contention returns `ErrCASContention`.

Pure-function cases needing no fixture at all:

- `ValidateMembershipCount`: member with 0 or 1 memberships passes, member
  with 2 fails, admin/owner with any count passes;
- the role branch of `TeamsForUser`/`CanAccessTeam` is exercised end-to-end
  in handler/integration tests against the seeded database (doc 06's "Roles
  and team access" list).

## 5. Verification Commands

```sh
task db:migrate     # after the 00002 rewrite + db rebuild
task db:seed
task db:seed        # idempotency: second run must converge, not duplicate
task sqlc:generate
task sqlc:vet
task templ
go test ./...       # with compose up, so the NATS-backed tests run
task dev
```

Manual browser checks:

- visit `/`, confirm redirect to `/auth/login`;
- log in with `admin@example.com` and `SEED_PASSWORD`;
- confirm dashboard renders and demo controls work (CSP enabled);
- click logout;
- request a password reset and inspect Mailpit;
- reset the password and confirm the old session is invalidated;
- confirm the security headers on any response in devtools.

---

**Previous:** [Auth Feature And Routing Code](10-implementation-code-feature.md) · **End of plan** — back to [Auth Scaffold Plan](README.md)
