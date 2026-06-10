# Auth Feature And Routing Code

## 1. Add `features/auth/state.go`

The self-owned session manager: opaque tokens, hash-keyed JetStream storage,
7-day lifetime (the WorkOS guide's default), throttled last-seen updates,
and revocation. No session library.

```go
package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	authcore "datastar-go/internal/auth"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go/jetstream"
)

const (
	SessionCookieName = "app_session"
	sessionTokenBytes = 32
	resetTokenBytes   = 32

	// SessionLifetime is the absolute session expiry; the cookie MaxAge in
	// middleware.go must stay aligned with it.
	SessionLifetime = 7 * 24 * time.Hour
	resetLifetime   = 30 * time.Minute

	// sessionTouchInterval throttles last-seen updates; one KV write per
	// request is pure amplification for best-effort metadata.
	sessionTouchInterval = 5 * time.Minute
)

type StateStore struct {
	sessions jetstream.KeyValue
	resets   jetstream.KeyValue
	now      func() time.Time
}

type SessionRecord struct {
	PID        string    `json:"pid"`
	UserID     uuid.UUID `json:"user_id"`
	CreatedIP  string    `json:"created_ip"`
	LastSeenIP string    `json:"last_seen_ip"`
	UserAgent  string    `json:"user_agent"`
	IssuedAt   time.Time `json:"issued_at"`
	ExpiresAt  time.Time `json:"expires_at"`
	LastSeenAt time.Time `json:"last_seen_at"`
	RevokedAt  time.Time `json:"revoked_at,omitempty"`
}

type PasswordReset struct {
	PID        string    `json:"pid"`
	UserID     uuid.UUID `json:"user_id"`
	Email      string    `json:"email"`
	TokenHash  string    `json:"token_hash"`
	IssuedAt   time.Time `json:"issued_at"`
	ExpiresAt  time.Time `json:"expires_at"`
	ConsumedAt time.Time `json:"consumed_at,omitempty"`
}

func NewStateStore(sessions, resets jetstream.KeyValue) *StateStore {
	return &StateStore{sessions: sessions, resets: resets, now: time.Now}
}

// CreateSession mints a fresh random token on every call — tokens are never
// derived from or reused across logins (session fixation control). The raw
// token goes only into the cookie; storage is keyed by its hash.
func (s *StateStore) CreateSession(ctx context.Context, userID uuid.UUID, ip, userAgent string) (string, SessionRecord, error) {
	token, err := authcore.NewToken(sessionTokenBytes)
	if err != nil {
		return "", SessionRecord{}, err
	}
	pid, err := authcore.NewPID("ses")
	if err != nil {
		return "", SessionRecord{}, err
	}

	now := s.now().UTC()
	record := SessionRecord{
		PID:        pid,
		UserID:     userID,
		CreatedIP:  ip,
		LastSeenIP: ip,
		UserAgent:  userAgent,
		IssuedAt:   now,
		ExpiresAt:  now.Add(SessionLifetime),
		LastSeenAt: now,
	}
	if err := s.putCreate(ctx, s.sessions, authcore.TokenHash(token), record); err != nil {
		return "", SessionRecord{}, fmt.Errorf("create session: %w", err)
	}
	return token, record, nil
}

func (s *StateStore) GetSession(ctx context.Context, token string, ip string) (SessionRecord, error) {
	key := authcore.TokenHash(token)
	entry, err := s.sessions.Get(ctx, key)
	if errors.Is(err, jetstream.ErrKeyNotFound) {
		return SessionRecord{}, ErrInvalidSession
	}
	if err != nil {
		return SessionRecord{}, fmt.Errorf("get session: %w", err)
	}

	var record SessionRecord
	if err := json.Unmarshal(entry.Value(), &record); err != nil {
		return SessionRecord{}, fmt.Errorf("decode session: %w", err)
	}

	now := s.now().UTC()
	if !record.RevokedAt.IsZero() || now.After(record.ExpiresAt) {
		return SessionRecord{}, ErrInvalidSession
	}

	if now.Sub(record.LastSeenAt) >= sessionTouchInterval {
		record.LastSeenAt = now
		record.LastSeenIP = ip
		_ = s.putUpdate(ctx, s.sessions, key, record, entry.Revision())
	}

	return record, nil
}

func (s *StateStore) RevokeSession(ctx context.Context, token string) error {
	_, err := authcore.CASUpdate(ctx, s.sessions, authcore.TokenHash(token), false, func(record *SessionRecord) error {
		record.RevokedAt = s.now().UTC()
		return nil
	})
	if errors.Is(err, jetstream.ErrKeyNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("revoke session: %w", err)
	}
	return nil
}

func (s *StateStore) CreatePasswordReset(ctx context.Context, userID uuid.UUID, email string) (string, PasswordReset, error) {
	token, err := authcore.NewToken(resetTokenBytes)
	if err != nil {
		return "", PasswordReset{}, err
	}
	pid, err := authcore.NewPID("rst")
	if err != nil {
		return "", PasswordReset{}, err
	}
	now := s.now().UTC()
	reset := PasswordReset{
		PID:       pid,
		UserID:    userID,
		Email:     email,
		TokenHash: authcore.TokenHash(token),
		IssuedAt:  now,
		ExpiresAt: now.Add(resetLifetime),
	}
	if err := s.putCreate(ctx, s.resets, reset.TokenHash, reset); err != nil {
		return "", PasswordReset{}, fmt.Errorf("create password reset: %w", err)
	}
	return token, reset, nil
}

// ConsumePasswordReset is atomic: the revision-checked update means two
// concurrent submissions of the same token cannot both consume it.
func (s *StateStore) ConsumePasswordReset(ctx context.Context, token string) (PasswordReset, error) {
	now := s.now().UTC()
	reset, err := authcore.CASUpdate(ctx, s.resets, authcore.TokenHash(token), false, func(r *PasswordReset) error {
		if !r.ConsumedAt.IsZero() || now.After(r.ExpiresAt) {
			return ErrInvalidResetToken
		}
		r.ConsumedAt = now
		return nil
	})
	if errors.Is(err, jetstream.ErrKeyNotFound) || errors.Is(err, authcore.ErrCASContention) {
		return PasswordReset{}, ErrInvalidResetToken
	}
	if err != nil {
		return PasswordReset{}, err
	}
	return reset, nil
}

func (s *StateStore) putCreate(ctx context.Context, kv jetstream.KeyValue, key string, value any) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode kv value: %w", err)
	}
	_, err = kv.Create(ctx, key, payload)
	return err
}

func (s *StateStore) putUpdate(ctx context.Context, kv jetstream.KeyValue, key string, value any, revision uint64) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode kv value: %w", err)
	}
	_, err = kv.Update(ctx, key, payload, revision)
	return err
}
```

## 2. Add `features/auth/errors.go`

```go
package auth

import "errors"

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrInvalidSession     = errors.New("invalid session")
	ErrInvalidResetToken  = errors.New("invalid reset token")
	ErrRateLimited        = errors.New("rate limited")
)
```

## 3. Add `features/auth/context.go`

```go
package auth

import (
	"context"

	"datastar-go/database/sqlc"
)

type contextKey string

const currentUserKey contextKey = "current_user"

func ContextWithUser(ctx context.Context, user sqlc.User) context.Context {
	return context.WithValue(ctx, currentUserKey, user)
}

func CurrentUser(ctx context.Context) (sqlc.User, bool) {
	user, ok := ctx.Value(currentUserKey).(sqlc.User)
	return user, ok
}
```

## 4. Add `features/auth/mailer.go`

`net/smtp` from the stdlib is enough for Mailpit and simple SMTP relays. The
service sends mail in the background (see `sendMailAsync` in service.go), so
this may block on SMTP without delaying the HTTP response.

```go
package auth

import (
	"context"
	"fmt"
	"net/smtp"

	"datastar-go/config"
)

type Mailer interface {
	SendPasswordReset(ctx context.Context, email string, link string) error
}

type SMTPMailer struct{}

func (SMTPMailer) SendPasswordReset(ctx context.Context, email string, link string) error {
	body := fmt.Sprintf("Subject: Reset your password\r\n\r\nReset your password here: %s\r\n", link)
	return smtp.SendMail(config.Env.SMTPAddr, nil, config.Env.SMTPFrom, []string{email}, []byte(body))
}
```

## 5. Add `features/auth/service.go`

```go
package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"datastar-go/config"
	"datastar-go/database/sqlc"
	authcore "datastar-go/internal/auth"
	"datastar-go/natsx"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go/jetstream"
)

type Service struct {
	queries *sqlc.Queries
	state   *StateStore
	limits  *authcore.RateLimiter
	mailer  Mailer
	js      jetstream.JetStream
}

func NewService(db *pgxpool.Pool, natsClient *natsx.Client, stores *natsx.AuthStores, mailer Mailer) *Service {
	return &Service{
		queries: sqlc.New(db),
		state:   NewStateStore(stores.Sessions, stores.PasswordResets),
		limits:  authcore.NewRateLimiter(stores.RateLimits),
		mailer:  mailer,
		js:      natsClient.JetStream,
	}
}

// Login follows the guide's sequence: rate limit, lookup, verify with
// equalized timing, then a fresh session token. Auth logging carries IDs and
// reasons only — never the password, token, or cookie value.
func (s *Service) Login(ctx context.Context, email, password, ip, userAgent string) (string, error) {
	allowed, err := s.limits.Allow(ctx, "login."+limitKey(ip, email), 5, 10*time.Minute)
	if err != nil {
		return "", err
	}
	if !allowed {
		slog.Warn("login rate limited", "ip", ip)
		return "", ErrRateLimited
	}

	user, err := s.queries.GetUserByEmail(ctx, email)
	if errors.Is(err, pgx.ErrNoRows) {
		// Equalize timing with the real-credential path below.
		authcore.VerifyDummyPassword(password)
		return "", ErrInvalidCredentials
	}
	if err != nil {
		return "", fmt.Errorf("get user by email: %w", err)
	}
	if user.DisabledAt.Valid {
		authcore.VerifyDummyPassword(password)
		return "", ErrInvalidCredentials
	}

	ok, err := authcore.VerifyPassword(password, user.PasswordHash)
	if err != nil || !ok {
		slog.Info("login failed", "user_id", user.ID, "ip", ip, "reason", "bad password")
		_ = s.publishAuthEvent(ctx, "auth.login.failed", user.Pid, user.ID.String())
		return "", ErrInvalidCredentials
	}

	token, session, err := s.state.CreateSession(ctx, user.ID, ip, userAgent)
	if err != nil {
		return "", err
	}
	slog.Info("user authenticated", "user_id", user.ID, "session", session.PID, "ip", ip)
	_ = s.publishAuthEvent(ctx, "auth.login.succeeded", session.PID, user.ID.String())
	return token, nil
}

// CurrentUser validates the session token, then checks the user row: it must
// exist, not be disabled, and not have auth invalidated after the session was
// issued.
func (s *Service) CurrentUser(ctx context.Context, token string, r *http.Request) (sqlc.User, error) {
	session, err := s.state.GetSession(ctx, token, clientIP(r))
	if err != nil {
		return sqlc.User{}, err
	}
	user, err := s.queries.GetUserByID(ctx, session.UserID)
	if errors.Is(err, pgx.ErrNoRows) {
		return sqlc.User{}, ErrInvalidSession
	}
	if err != nil {
		return sqlc.User{}, fmt.Errorf("get current user: %w", err)
	}
	if user.DisabledAt.Valid {
		return sqlc.User{}, ErrInvalidSession
	}
	if user.AuthInvalidatedAt.Valid && user.AuthInvalidatedAt.Time.After(session.IssuedAt) {
		return sqlc.User{}, ErrInvalidSession
	}
	return user, nil
}

func (s *Service) Logout(ctx context.Context, token string) error {
	if err := s.state.RevokeSession(ctx, token); err != nil {
		return err
	}
	_ = s.publishAuthEvent(ctx, "auth.logout.succeeded", "", "")
	return nil
}

func (s *Service) RequestPasswordReset(ctx context.Context, email, ip string) error {
	allowed, err := s.limits.Allow(ctx, "password_reset.request."+limitKey(ip, email), 5, time.Hour)
	if err != nil {
		return err
	}
	if !allowed {
		return ErrRateLimited
	}

	user, err := s.queries.GetUserByEmail(ctx, email)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("get user for reset: %w", err)
	}
	if user.DisabledAt.Valid {
		return nil
	}

	token, reset, err := s.state.CreatePasswordReset(ctx, user.ID, user.Email)
	if err != nil {
		return err
	}
	link := config.Env.AppBaseURL + "/auth/reset-password?token=" + url.QueryEscape(token)
	_ = s.publishAuthEvent(ctx, "auth.password_reset.requested", reset.PID, user.ID.String())

	// Async send: waiting on SMTP would make the generic response measurably
	// slower for registered emails — an enumeration oracle.
	s.sendMailAsync("password reset", func(ctx context.Context) error {
		return s.mailer.SendPasswordReset(ctx, user.Email, link)
	})
	return nil
}

func (s *Service) ResetPassword(ctx context.Context, token, newPassword string) error {
	// Validate before consuming the single-use token, so a rejected password
	// does not burn the user's reset link.
	if err := authcore.ValidatePassword(newPassword); err != nil {
		return err
	}

	reset, err := s.state.ConsumePasswordReset(ctx, token)
	if err != nil {
		return err
	}

	hash, err := authcore.HashPassword(newPassword)
	if err != nil {
		return err
	}
	// UpdateUserPassword also sets auth_invalidated_at = now(), revoking
	// every session issued before this moment in one statement.
	if err := s.queries.UpdateUserPassword(ctx, sqlc.UpdateUserPasswordParams{
		ID:           reset.UserID,
		PasswordHash: hash,
	}); err != nil {
		return fmt.Errorf("update password hash: %w", err)
	}
	slog.Info("password reset completed", "user_id", reset.UserID)
	_ = s.publishAuthEvent(ctx, "auth.password_reset.completed", reset.PID, reset.UserID.String())
	return nil
}

// sendMailAsync runs an email send in the background with its own timeout,
// detached from the request context. Failures are logged, never surfaced:
// the response must stay identical whether or not an email was sent.
func (s *Service) sendMailAsync(kind string, send func(ctx context.Context) error) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := send(ctx); err != nil {
			slog.Error("send auth mail", "kind", kind, "error", err)
		}
	}()
}

func (s *Service) publishAuthEvent(ctx context.Context, subject, id, userID string) error {
	payload, err := json.Marshal(map[string]string{
		"id":      id,
		"user_id": userID,
	})
	if err != nil {
		return err
	}
	_, err = s.js.Publish(ctx, subject, payload)
	return err
}

// clientIP trusts RemoteAddr only. Behind a reverse proxy, parse
// X-Forwarded-For here — but only from a configured trusted proxy, or clients
// can spoof their way past IP rate limits.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}

func limitKey(parts ...string) string {
	return strings.NewReplacer("@", "_", ".", "_", ":", "_", "/", "_").Replace(strings.ToLower(strings.Join(parts, ".")))
}
```

## 5a. Add `features/auth/authz.go`

The role semantics from doc 01 in one place: owners derive access to every
team; admins and members resolve through `team_memberships`. The
member-one-team rule is enforced here too, so any future membership-writing
flow inherits it. Needs the extra import `"github.com/google/uuid"`.

```go
package auth

import (
	"context"
	"errors"
	"fmt"

	"datastar-go/database/sqlc"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// TeamsForUser resolves the teams a user can access: owners see all teams
// (no membership rows needed), admins and members see their memberships.
func (s *Service) TeamsForUser(ctx context.Context, user sqlc.User) ([]sqlc.Team, error) {
	if user.Role == sqlc.UserRoleOwner {
		return s.queries.ListAllTeams(ctx)
	}
	return s.queries.ListTeamsForUser(ctx, user.ID)
}

// CanAccessTeam reports whether the user can access the given team.
func (s *Service) CanAccessTeam(ctx context.Context, user sqlc.User, teamID uuid.UUID) (bool, error) {
	if user.Role == sqlc.UserRoleOwner {
		return true, nil
	}
	_, err := s.queries.GetTeamMembership(ctx, sqlc.GetTeamMembershipParams{
		TeamID: teamID,
		UserID: user.ID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("get team membership: %w", err)
	}
	return true, nil
}

// ValidateMembershipCount enforces the member-one-team invariant (doc 01):
// a user with role 'member' may hold at most one membership. Call it before
// any code path that writes a membership row.
func ValidateMembershipCount(role sqlc.UserRole, memberships int) error {
	if role == sqlc.UserRoleMember && memberships > 1 {
		return fmt.Errorf("role %q allows exactly one team, got %d", role, memberships)
	}
	return nil
}
```

The scaffold's dashboard can call `TeamsForUser` to show the current user's
teams; `CanAccessTeam` is ready for the first team-scoped route.

## 6. Add `features/auth/middleware.go`

```go
package auth

import (
	"net/http"
	"time"

	"datastar-go/config"
)

// RequireUser redirects through redirectDatastarOrHTTP: protected Datastar
// endpoints are called via fetch, which would follow a 303 to the login page
// and choke on the HTML response — they need an SSE redirect instead. This is
// what makes an expired session navigate to login on the next click rather
// than failing silently.
func (s *Service) RequireUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(SessionCookieName)
		if err != nil || cookie.Value == "" {
			redirectDatastarOrHTTP(w, r, "/auth/login")
			return
		}

		user, err := s.CurrentUser(r.Context(), cookie.Value, r)
		if err != nil {
			clearSessionCookie(w)
			redirectDatastarOrHTTP(w, r, "/auth/login")
			return
		}

		next.ServeHTTP(w, r.WithContext(ContextWithUser(r.Context(), user)))
	})
}

// Cookie attributes per the WorkOS guide: HttpOnly always, Secure outside
// local dev, SameSite=Lax, MaxAge aligned with the server-side expiry. Once
// the app is HTTPS-only, rename to a __Host- prefixed cookie.
func setSessionCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   int(SessionLifetime / time.Second),
		HttpOnly: true,
		Secure:   config.Env.AppEnv == config.Prod,
		SameSite: http.SameSiteLaxMode,
	})
}

func clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   config.Env.AppEnv == config.Prod,
		SameSite: http.SameSiteLaxMode,
	})
}
```

## 7. Replace `features/auth/routes.go`

`features/auth` already exists with a stub: `routes.go` registers only
`GET/POST /auth/login`, and the stub `Login` handler logs the raw password.
Replace all of that — the new `SetupRoutes` takes the NATS client and
returns the `*Service` so the router can reuse its `RequireUser` middleware.

```go
package auth

import (
	"context"
	"fmt"

	"datastar-go/natsx"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func SetupRoutes(ctx context.Context, router chi.Router, db *pgxpool.Pool, natsClient *natsx.Client) (*Service, error) {
	stores, err := natsClient.AuthStores(ctx)
	if err != nil {
		return nil, fmt.Errorf("auth stores: %w", err)
	}

	service := NewService(db, natsClient, stores, SMTPMailer{})
	handler := NewHandler(service)

	router.Get("/auth/login", handler.LoginPage)
	router.Post("/auth/login", handler.Login)
	router.Get("/auth/forgot-password", handler.ForgotPasswordPage)
	router.Post("/auth/forgot-password", handler.RequestPasswordReset)
	router.Get("/auth/reset-password", handler.ResetPasswordPage)
	router.Post("/auth/reset-password", handler.ResetPassword)

	router.Group(func(protected chi.Router) {
		protected.Use(service.RequireUser)
		protected.Post("/auth/logout", handler.Logout)
	})

	return service, nil
}
```

## 8. Replace `features/auth/handlers.go`

```go
package auth

import (
	"errors"
	"log/slog"
	"net/http"

	"datastar-go/features/auth/pages"
	authcore "datastar-go/internal/auth"

	"github.com/starfederation/datastar-go/datastar"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) LoginPage(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(SessionCookieName); err == nil && cookie.Value != "" {
		if _, err := h.service.CurrentUser(r.Context(), cookie.Value, r); err == nil {
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
	}
	_ = pages.LoginPage(pages.LoginView{}).Render(r.Context(), w)
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	token, err := h.service.Login(
		r.Context(),
		r.FormValue("email"),
		r.FormValue("password"),
		clientIP(r),
		r.UserAgent(),
	)
	if err != nil {
		h.patchLogin(w, r, pages.LoginView{Error: loginError(err)})
		return
	}

	setSessionCookie(w, token)
	redirectDatastarOrHTTP(w, r, "/")
}

func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(SessionCookieName); err == nil {
		_ = h.service.Logout(r.Context(), cookie.Value)
	}
	clearSessionCookie(w)
	redirectDatastarOrHTTP(w, r, "/auth/login")
}

func (h *Handler) ForgotPasswordPage(w http.ResponseWriter, r *http.Request) {
	_ = pages.ForgotPasswordPage(pages.ResetView{}).Render(r.Context(), w)
}

func (h *Handler) RequestPasswordReset(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	if err := h.service.RequestPasswordReset(r.Context(), r.FormValue("email"), clientIP(r)); err != nil {
		slog.Warn("password reset request failed", "error", err)
	}
	sse := datastar.NewSSE(w, r)
	_ = sse.PatchElementTempl(pages.ResetMessage("If that email exists, reset instructions have been sent."))
}

func (h *Handler) ResetPasswordPage(w http.ResponseWriter, r *http.Request) {
	_ = pages.ResetPasswordPage(pages.ResetView{Token: r.URL.Query().Get("token")}).Render(r.Context(), w)
}

func (h *Handler) ResetPassword(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	err := h.service.ResetPassword(r.Context(), r.FormValue("token"), r.FormValue("password"))
	// A too-short password is not sensitive; the token is still unconsumed,
	// so the user can resubmit with the same link.
	if errors.Is(err, authcore.ErrPasswordTooShort) {
		sse := datastar.NewSSE(w, r)
		_ = sse.PatchElementTempl(pages.ResetMessage("Password must be at least 12 characters."))
		return
	}
	if err != nil {
		sse := datastar.NewSSE(w, r)
		_ = sse.PatchElementTempl(pages.ResetMessage("Invalid or expired reset link."))
		return
	}
	clearSessionCookie(w)
	redirectDatastarOrHTTP(w, r, "/auth/login")
}

func (h *Handler) patchLogin(w http.ResponseWriter, r *http.Request, view pages.LoginView) {
	sse := datastar.NewSSE(w, r)
	if err := sse.PatchElementTempl(pages.LoginPanel(view)); err != nil {
		slog.Error("patch login panel", "error", err)
	}
}

// loginError keeps every failure mode on one generic message — including
// rate limiting, which must not become an account-existence oracle.
func loginError(err error) string {
	if errors.Is(err, ErrRateLimited) {
		return "Too many attempts. Please try again later."
	}
	return "Invalid email or password."
}

// redirectDatastarOrHTTP picks the channel the client can act on: Datastar
// requests expect SSE events, plain form posts expect an HTTP redirect.
func redirectDatastarOrHTTP(w http.ResponseWriter, r *http.Request, path string) {
	if r.Header.Get("Datastar-Request") == "true" {
		sse := datastar.NewSSE(w, r)
		_ = sse.Redirect(path)
		return
	}
	http.Redirect(w, r, path, http.StatusSeeOther)
}
```

## 9. Replace `features/auth/pages/login.templ`

A stub login page already exists at this path; replace it (and regenerate the
`_templ.go` output). One form, no mode switching. Pages wrap in the shared
`layouts.BaseLayout`, which already loads datastar.js and the dev reload
hook — auth pages must not hand-roll their own `<html>` documents.

```templ
package pages

import "datastar-go/features/common/layouts"

type LoginView struct {
	Error string
}

templ LoginPage(view LoginView) {
	@layouts.BaseLayout("Login") {
		@LoginPanel(view)
	}
}

templ LoginPanel(view LoginView) {
	<main id="auth-panel">
		<h1>Sign in</h1>
		if view.Error != "" {
			<p id="auth-error">{ view.Error }</p>
		}
		<form method="post" action="/auth/login" data-on:submit__prevent="@post('/auth/login', {contentType: 'form'})">
			<label>Email <input name="email" type="email" autocomplete="email" required/></label>
			<label>Password <input name="password" type="password" autocomplete="current-password" required/></label>
			<button type="submit">Sign in</button>
		</form>
		<a href="/auth/forgot-password">Forgot password?</a>
	</main>
}
```

## 10. Add `features/auth/pages/reset.templ`

```templ
package pages

import "datastar-go/features/common/layouts"

type ResetView struct {
	Token string
	Error string
}

templ ForgotPasswordPage(view ResetView) {
	@layouts.BaseLayout("Forgot password") {
		<main>
			<h1>Reset password</h1>
			@ResetMessage("")
			<form method="post" action="/auth/forgot-password" data-on:submit__prevent="@post('/auth/forgot-password', {contentType: 'form'})">
				<label>Email <input name="email" type="email" autocomplete="email" required/></label>
				<button type="submit">Send reset link</button>
			</form>
			<a href="/auth/login">Back to login</a>
		</main>
	}
}

templ ResetPasswordPage(view ResetView) {
	@layouts.BaseLayout("Set new password") {
		<main>
			<h1>Set new password</h1>
			@ResetMessage("")
			<form method="post" action="/auth/reset-password" data-on:submit__prevent="@post('/auth/reset-password', {contentType: 'form'})">
				<input type="hidden" name="token" value={ view.Token }/>
				<label>New password <input name="password" type="password" autocomplete="new-password" required minlength="12"/></label>
				<button type="submit">Update password</button>
			</form>
		</main>
	}
}

templ ResetMessage(message string) {
	<p id="reset-message">{ message }</p>
}
```

## 11. Update `router/router.go`

Replace the `SetupRoutes` body so the security middleware comes first, auth
is registered with the new signature, and the index feature mounts behind
`RequireUser`. In chi, middleware must be registered before routes:

```go
func SetupRoutes(ctx context.Context, router chi.Router, db *pgxpool.Pool, natsClient *natsx.Client) (err error) {
	router.Use(authcore.SecurityHeaders)
	router.Use(authcore.FetchMetadata)

	if config.Env.AppEnv == config.Dev {
		setupReload(router)
	}
	router.Handle("/static/*", resources.Handler())

	authService, err := authFeature.SetupRoutes(ctx, router, db, natsClient)
	if err != nil {
		return fmt.Errorf("setup auth routes: %w", err)
	}

	protected := chi.NewRouter()
	protected.Use(authService.RequireUser)
	if err := indexFeature.SetupRoutes(ctx, protected, db, natsClient); err != nil {
		return fmt.Errorf("setup index routes: %w", err)
	}
	router.Mount("/", protected)

	return nil
}
```

with the extra import:

```go
authcore "datastar-go/internal/auth"
```

Keep `/reload`, `/hotreload`, and `/static/*` outside the protected router so
dev reload and assets stay public (doc 03's route list). They are GET
endpoints, so the Fetch Metadata middleware (which only gates unsafe
methods) does not affect them. After wiring `SecurityHeaders`, load the
dashboard with devtools open and confirm the CSP breaks neither Datastar
interactions nor the hot-reload SSE stream.

## 12. Update `features/index/pages/index.templ`

Add a logout form near the top of `IndexPage`:

```templ
<form method="post" action="/auth/logout" data-on:submit__prevent="@post('/auth/logout', {contentType: 'form'})">
	<button type="submit">Logout</button>
</form>
```

Then regenerate:

```sh
task templ
```

---

**Previous:** [Core Auth And JetStream Code](09-implementation-code-core.md) · **Next:** [Seeders And Tests Code](11-implementation-code-seeders-tests.md)
