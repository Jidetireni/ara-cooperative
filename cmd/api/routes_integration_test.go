//go:build integration

package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/Jidetireni/ara-cooperative/factory"
	"github.com/Jidetireni/ara-cooperative/internal/api/handlers"
	"github.com/Jidetireni/ara-cooperative/internal/config"
	"github.com/Jidetireni/ara-cooperative/internal/dto"
	"github.com/Jidetireni/ara-cooperative/internal/helpers"
	"github.com/Jidetireni/ara-cooperative/internal/middleware"
	"github.com/Jidetireni/ara-cooperative/internal/repository"
	"github.com/Jidetireni/ara-cooperative/internal/services/campaigns"
	"github.com/Jidetireni/ara-cooperative/internal/services/loans"
	"github.com/Jidetireni/ara-cooperative/internal/services/members"
	"github.com/Jidetireni/ara-cooperative/internal/services/payments"
	"github.com/Jidetireni/ara-cooperative/internal/services/transactions"
	"github.com/Jidetireni/ara-cooperative/internal/services/users"
	loggerpkg "github.com/Jidetireni/ara-cooperative/pkg/logger"
	"github.com/Jidetireni/ara-cooperative/pkg/paystack"
	"github.com/Jidetireni/ara-cooperative/pkg/token"
	"github.com/go-chi/chi/v5"
	"github.com/go-playground/locales/en"
	ut "github.com/go-playground/universal-translator"
	"github.com/go-playground/validator/v10"
	enTranslations "github.com/go-playground/validator/v10/translations/en"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/jmoiron/sqlx/reflectx"
	"github.com/lib/pq"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
)

func TestServerSessionFlow(t *testing.T) {
	tests := []struct {
		name   string
		roles  []string
		member bool
		active bool
	}{
		{name: "owner without membership", roles: []string{"owner"}},
		{name: "admin without membership", roles: []string{"admin"}},
		{name: "inactive member", roles: []string{"member"}, member: true},
		{name: "active member", roles: []string{"member"}, member: true, active: true},
		{name: "admin who is a member", roles: []string{"admin", "member"}, member: true, active: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			db := sessionTestDB(t)
			router := sessionTestRouter(t, db)
			userID := seedSessionUser(t, db, tt.roles, tt.member, tt.active)
			body := `{"email":"session@example.com","password":"test-password"}`
			login := sessionRequest(router, http.MethodPost, "/api/v1/login", body, "", nil)
			response := readAuthResponse(t, login)
			if response.User.ID != userID || !reflect.DeepEqual(response.User.Roles, tt.roles) {
				t.Fatalf("incorrect login identity: %+v", response.User)
			}
			if response.User.ConfirmedAt == nil {
				t.Fatal("confirmed email timestamp missing")
			}
			if (response.User.Member != nil) != tt.member {
				t.Fatalf("unexpected membership: %+v", response.User.Member)
			}
			if tt.member && response.User.Member.IsActive != tt.active {
				t.Fatal("incorrect membership activation state")
			}
			if bytes.Contains(login.Body.Bytes(), []byte("password_hash")) || response.RefreshToken != "" {
				t.Fatal("session body contains private credentials")
			}

			me := sessionRequest(router, http.MethodGet, "/api/v1/me", "", response.AccessToken, nil)
			var profile struct{ Data dto.AuthUser }
			if me.Code != http.StatusOK || json.Unmarshal(me.Body.Bytes(), &profile) != nil {
				t.Fatalf("GET /me failed: %d %s", me.Code, me.Body.String())
			}
			if !reflect.DeepEqual(profile.Data, *response.User) {
				t.Fatalf("login and /me disagree: %+v versus %+v", response.User, profile.Data)
			}

			cookie := refreshCookie(t, login)
			invalidBearer := sessionRequest(router, http.MethodGet, "/api/v1/me", "", cookie.Value, nil)
			if invalidBearer.Code != http.StatusUnauthorized {
				t.Fatal("refresh token authenticated as an access token")
			}
			if sessionRequest(router, http.MethodGet, "/api/v1/me", "", "", nil).Code != http.StatusUnauthorized {
				t.Fatal("unauthenticated profile request accepted")
			}

			// Multiple rotations exercise the database's token-type uniqueness constraint.
			for range 3 {
				refreshed := sessionRequest(router, http.MethodPost, "/api/v1/refresh", "", "", cookie)
				rotated := readAuthResponse(t, refreshed)
				if !reflect.DeepEqual(rotated.User, response.User) {
					t.Fatal("refresh lost the account context")
				}
				old := sessionRequest(router, http.MethodPost, "/api/v1/refresh", "", "", cookie)
				if old.Code != http.StatusUnauthorized {
					t.Fatalf("old refresh cookie accepted: %d", old.Code)
				}
				cookie = refreshCookie(t, refreshed)
			}

			if _, err := db.Exec("UPDATE users SET deleted_at = NOW() WHERE id = $1", userID); err != nil {
				t.Fatal(err)
			}
			deleted := sessionRequest(router, http.MethodGet, "/api/v1/me", "", response.AccessToken, nil)
			if deleted.Code != http.StatusUnauthorized {
				t.Fatalf("deleted account can load profile: %d", deleted.Code)
			}
			deletedRefresh := sessionRequest(router, http.MethodPost, "/api/v1/refresh", "", "", cookie)
			if deletedRefresh.Code != http.StatusUnauthorized {
				t.Fatalf("deleted account refresh must return 401: %d", deletedRefresh.Code)
			}
		})
	}
}

func TestServerCurrentUserReloadsMembership(t *testing.T) {
	t.Parallel()
	db := sessionTestDB(t)
	router := sessionTestRouter(t, db)
	userID := seedSessionUser(t, db, []string{"member"}, true, false)
	login := sessionRequest(router, http.MethodPost, "/api/v1/login",
		`{"email":"session@example.com","password":"test-password"}`, "", nil)
	response := readAuthResponse(t, login)
	if _, err := db.Exec("UPDATE members SET activated_at = NOW() WHERE user_id = $1", userID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("DELETE FROM user_roles WHERE user_id = $1", userID); err != nil {
		t.Fatal(err)
	}
	me := sessionRequest(router, http.MethodGet, "/api/v1/me", "", response.AccessToken, nil)
	var profile struct{ Data dto.AuthUser }
	if me.Code != http.StatusOK || json.Unmarshal(me.Body.Bytes(), &profile) != nil {
		t.Fatalf("GET /me failed: %d %s", me.Code, me.Body.String())
	}
	if profile.Data.Member == nil || !profile.Data.Member.IsActive || len(profile.Data.Roles) != 0 {
		t.Fatalf("profile still uses stale token state: %+v", profile.Data)
	}
	if profile.Data.Roles == nil {
		t.Fatal("empty roles must serialize as an array")
	}
}

func TestServerSetPasswordReturnsSessionContext(t *testing.T) {
	t.Parallel()
	db := sessionTestDB(t)
	router := sessionTestRouter(t, db)
	userID := seedSessionUser(t, db, []string{"member"}, true, false)
	if _, err := db.Exec(`UPDATE users SET password_hash = NULL, email_confirmed_at = NULL WHERE id = $1`, userID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO tokens (user_id, token, token_type, is_valid, expires_at)
		VALUES ($1, $2, $3, TRUE, NOW() + INTERVAL '30 minutes')`,
		userID, helpers.HashToken("123456"), token.SetPasswordToken); err != nil {
		t.Fatal(err)
	}
	body := `{"email":"session@example.com","password":"new-password","token":"123456"}`
	setup := sessionRequest(router, http.MethodPost, "/api/v1/set-password", body, "", nil)
	var response struct{ Data dto.AuthResponse }
	if setup.Code != http.StatusCreated || json.Unmarshal(setup.Body.Bytes(), &response) != nil {
		t.Fatalf("password setup failed: %d %s", setup.Code, setup.Body.String())
	}
	user := response.Data.User
	if user == nil || user.Member == nil || user.Member.IsActive || user.ConfirmedAt == nil {
		t.Fatalf("password setup lost membership context: %+v", user)
	}
	if !reflect.DeepEqual(user.Roles, []string{"member"}) || response.Data.AccessToken == "" {
		t.Fatal("password setup did not create a complete session")
	}
	refreshed := sessionRequest(router, http.MethodPost, "/api/v1/refresh", "", "", refreshCookie(t, setup))
	if got := readAuthResponse(t, refreshed); !reflect.DeepEqual(got.User, user) {
		t.Fatal("setup and refresh account context disagree")
	}
	usedCode := sessionRequest(router, http.MethodPost, "/api/v1/set-password", body, "", nil)
	if usedCode.Code != http.StatusBadRequest {
		t.Fatal("password setup code was accepted twice")
	}
}

func sessionTestDB(t *testing.T) *sqlx.DB {
	t.Helper()
	dsn := os.Getenv("ARA_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set ARA_TEST_DATABASE_URL to a dedicated PostgreSQL test database")
	}
	parsed, err := url.Parse(dsn)
	if err != nil || (parsed.Scheme != "postgres" && parsed.Scheme != "postgresql") {
		t.Fatal("ARA_TEST_DATABASE_URL must be a PostgreSQL URL")
	}
	admin, err := sqlx.Connect("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := "ara_session_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := admin.Exec("CREATE SCHEMA " + pq.QuoteIdentifier(schema)); err != nil {
		if closeErr := admin.Close(); closeErr != nil {
			t.Error(closeErr)
		}
		t.Fatal(err)
	}
	var db *sqlx.DB
	t.Cleanup(func() {
		if db != nil {
			if err := db.Close(); err != nil {
				t.Error(err)
			}
		}
		if _, err := admin.Exec("DROP SCHEMA " + pq.QuoteIdentifier(schema) + " CASCADE"); err != nil {
			t.Error(err)
		}
		if err := admin.Close(); err != nil {
			t.Error(err)
		}
	})
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	db, err = sqlx.Connect("postgres", parsed.String())
	if err != nil {
		t.Fatal(err)
	}
	db.Mapper = reflectx.NewMapper("json")
	// Apply every migration in order so route tests see the real schema.
	migrations, err := filepath.Glob(filepath.Join("..", "..", "migrations", "schema", "*.sql"))
	if err != nil || len(migrations) == 0 {
		t.Fatalf("find migrations: %v", err)
	}
	sort.Strings(migrations)
	for _, name := range migrations {
		source, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		up := strings.SplitN(string(source), "-- +goose Down", 2)[0]
		if _, err := db.Exec(up); err != nil {
			t.Fatalf("apply %s: %v", name, err)
		}
	}
	return db
}

func sessionTestRouter(t *testing.T, db *sqlx.DB) http.Handler {
	t.Helper()
	return invitationTestRouter(t, db, &invitationEmailSender{})
}

func invitationTestRouter(t *testing.T, db *sqlx.DB, sender members.EmailPkg) http.Handler {
	t.Helper()
	cfg := &config.Config{IsDev: false, Server: config.ServerConfig{FEURL: "http://localhost:3000"}}
	return buildTestRouter(t, db, cfg, sender, paystack.New("", ""))
}

func buildTestRouter(t *testing.T, db *sqlx.DB, cfg *config.Config, sender members.EmailPkg, paystackClient *paystack.Client) http.Handler {
	t.Helper()
	if cfg.Shares.UnitPrice == 0 {
		cfg.Shares.UnitPrice = config.DefaultShareUnitPrice
	}
	jwt := token.NewJwt("session-integration-key", false)
	service := users.New(
		db,
		cfg,
		jwt,
		repository.NewUserRepository(db),
		repository.NewRoleRepository(db),
		repository.NewPermissionRepository(db),
		repository.NewTokenRepository(db),
		repository.NewMemberRepository(db),
	)
	zlog := zap.NewNop()
	log := &loggerpkg.Logger{Logger: zlog, Sugar: zlog.Sugar()}
	memberService := members.New(
		db,
		cfg,
		repository.NewMemberRepository(db),
		repository.NewUserRepository(db),
		repository.NewRoleRepository(db),
		repository.NewPermissionRepository(db),
		repository.NewTokenRepository(db),
		sender,
		log,
	)
	repos := &factory.Repositories{
		Member:      repository.NewMemberRepository(db),
		User:        repository.NewUserRepository(db),
		Transaction: repository.NewTransactionRepository(db),
		Share:       repository.NewShareRepository(db),
		Fine:        repository.NewFineRepository(db),
		Payment:     repository.NewPaymentRepository(db),
		Campaign:    repository.NewCampaignRepository(db),
		Loan:        repository.NewLoanRepository(db),
	}
	transactionService := transactions.New(db, repos.Transaction, repos.Member, repos.Share, repos.Fine, cfg.Shares.UnitPrice, log)
	paymentService := payments.New(db, cfg, repos.Payment, repos.Member, repos.User, repos.Transaction,
		repos.Share, repos.Fine, repos.Campaign, transactionService, paystackClient, log)
	fx := &factory.Factory{
		Router: chi.NewRouter(),
		Services: &factory.Services{
			User: service, Member: memberService, Transactions: transactionService, Payments: paymentService,
			Campaigns: campaigns.New(db, repos.Campaign, repos.Member, repos.Payment),
			Loans:     loans.New(db, repos.Loan, repos.Member, repos.Transaction, repos.Share),
		},
		Repositories: repos,
		Pkgs:         &factory.Packages{Logger: log, Paystack: paystackClient},
		Middleware:   middleware.New(jwt, log, service),
	}
	validate := validator.New()
	trans, _ := ut.New(en.New()).GetTranslator("en")
	if err := enTranslations.RegisterDefaultTranslations(validate, trans); err != nil {
		t.Fatal(err)
	}
	s := &Server{Factory: *fx, Handlers: handlers.NewHandlers(fx, cfg, validate, trans)}
	s.router()
	return fx.Router
}

func seedSessionUser(t *testing.T, db *sqlx.DB, roles []string, member, active bool) uuid.UUID {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte("test-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	var id uuid.UUID
	if err := db.Get(&id, `INSERT INTO users (email, password_hash, email_confirmed_at)
		VALUES ('session@example.com', $1, NOW()) RETURNING id`, string(hash)); err != nil {
		t.Fatal(err)
	}
	for _, role := range roles {
		var roleID uuid.UUID
		if err := db.Get(&roleID, `INSERT INTO roles (name) VALUES ($1)
			ON CONFLICT (name) DO UPDATE SET name = EXCLUDED.name RETURNING id`, role); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec("INSERT INTO user_roles (user_id, role_id) VALUES ($1, $2)", id, roleID); err != nil {
			t.Fatal(err)
		}
	}
	if member {
		var activatedAt *time.Time
		if active {
			now := time.Now()
			activatedAt = &now
		}
		if _, err := db.Exec(`INSERT INTO members
			(user_id, slug, first_name, last_name, phone, activated_at)
			VALUES ($1, 'ara000001', 'Ada', 'Member', '08000000000', $2)`, id, activatedAt); err != nil {
			t.Fatal(err)
		}
	}
	return id
}

func sessionRequest(h http.Handler, method, path, body, access string, cookie *http.Cookie) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	if access != "" {
		r.Header.Set("Authorization", "Bearer "+access)
	}
	if cookie != nil {
		r.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func readAuthResponse(t *testing.T, w *httptest.ResponseRecorder) *dto.AuthResponse {
	t.Helper()
	var body struct{ Data dto.AuthResponse }
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &body) != nil {
		t.Fatalf("session request failed: %d %s", w.Code, w.Body.String())
	}
	if body.Data.User == nil || body.Data.AccessToken == "" {
		t.Fatalf("missing session context: %s", w.Body.String())
	}
	return &body.Data
}

func refreshCookie(t *testing.T, w *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	for _, cookie := range w.Result().Cookies() {
		if cookie.Name == token.RefreshTokenName {
			if !cookie.HttpOnly || !cookie.Secure || cookie.SameSite != http.SameSiteStrictMode {
				t.Fatal("refresh cookie is missing production security attributes")
			}
			return cookie
		}
	}
	t.Fatal("refresh cookie missing")
	return nil
}
