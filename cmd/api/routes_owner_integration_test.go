//go:build integration

package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"

	"github.com/Jidetireni/ara-cooperative/cmd/seed/seed"
	"github.com/Jidetireni/ara-cooperative/internal/config"
	"github.com/Jidetireni/ara-cooperative/internal/dto"
	"github.com/Jidetireni/ara-cooperative/internal/helpers"
	"github.com/Jidetireni/ara-cooperative/internal/repository"
	"github.com/Jidetireni/ara-cooperative/internal/services/owners"
	"github.com/Jidetireni/ara-cooperative/pkg/database"
	"github.com/Jidetireni/ara-cooperative/pkg/token"
	"golang.org/x/crypto/bcrypt"
)

func TestServerOwnerBootstrapAndAdminAccess(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := sessionTestDB(t)
	router := sessionTestRouter(t, db)
	bootstrap := owners.New(repository.NewOwnerRepository(db))
	owner, err := bootstrap.Bootstrap(ctx, "jtirenipraise@gmail.com")
	if err != nil {
		t.Fatal(err)
	}
	if len(owner.SetupToken) != 64 {
		t.Fatal("new owner needs a strong password-setup token")
	}
	var stored string
	if err := db.Get(&stored, "SELECT token FROM tokens WHERE user_id = $1", owner.ID); err != nil {
		t.Fatal(err)
	}
	if stored == owner.SetupToken || stored != helpers.HashToken(owner.SetupToken) {
		t.Fatal("owner setup token must be stored as a hash")
	}
	var memberCount int
	if err := db.Get(&memberCount, "SELECT COUNT(*) FROM members WHERE user_id = $1", owner.ID); err != nil {
		t.Fatal(err)
	}
	if memberCount != 0 {
		t.Fatal("bootstrap created an unsolicited personal membership")
	}
	firstToken := owner.SetupToken
	renewed, err := bootstrap.Bootstrap(ctx, owner.Email)
	if err != nil || renewed.ID != owner.ID || renewed.SetupToken == firstToken {
		t.Fatalf("pending owner setup was not safely renewed: %+v %v", renewed, err)
	}
	oldSetup, err := json.Marshal(map[string]string{
		"email": owner.Email, "password": "owner-test-password", "token": firstToken,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := sessionRequest(router, http.MethodPost, "/api/v1/set-password", string(oldSetup), "", nil); got.Code != http.StatusBadRequest {
		t.Fatal("renewed bootstrap still accepts an old setup token")
	}
	owner = renewed
	setup, err := json.Marshal(map[string]string{
		"email": owner.Email, "password": "owner-test-password", "token": owner.SetupToken,
	})
	if err != nil {
		t.Fatal(err)
	}
	setPassword := sessionRequest(router, http.MethodPost, "/api/v1/set-password", string(setup), "", nil)
	if setPassword.Code != http.StatusCreated {
		t.Fatalf("owner password setup failed: %d %s", setPassword.Code, setPassword.Body.String())
	}
	login := sessionRequest(router, http.MethodPost, "/api/v1/login",
		`{"email":"jtirenipraise@gmail.com","password":"owner-test-password"}`, "", nil)
	ownerSession := readAuthResponse(t, login)
	if !slices.Equal(ownerSession.User.Roles, []string{"owner"}) || ownerSession.User.Member != nil {
		t.Fatalf("wrong owner account context: %+v", ownerSession.User)
	}
	if got := sessionRequest(router, http.MethodGet, "/api/v1/me", "", ownerSession.AccessToken, nil); got.Code != http.StatusOK {
		t.Fatalf("owner cannot load profile: %d %s", got.Code, got.Body.String())
	}
	refreshed := sessionRequest(router, http.MethodPost, "/api/v1/refresh", "", "", refreshCookie(t, login))
	if got := readAuthResponse(t, refreshed); !slices.Contains(got.User.Roles, "owner") {
		t.Fatal("refresh lost owner role")
	}
	// Invalid input reaches validation only when both admin role and permission checks pass.
	probeAdmin := func(access string) int {
		return sessionRequest(router, http.MethodPost, "/api/v1/members",
			`{}`, access, nil).Code
	}
	if probeAdmin(ownerSession.AccessToken) != http.StatusBadRequest {
		t.Fatal("owner does not inherit admin capabilities")
	}

	targetID := seedSessionUser(t, db, []string{"member"}, true, true)
	targetLogin := sessionRequest(router, http.MethodPost, "/api/v1/login",
		`{"email":"session@example.com","password":"test-password"}`, "", nil)
	memberSession := readAuthResponse(t, targetLogin)
	if probeAdmin(memberSession.AccessToken) != http.StatusForbidden {
		t.Fatal("member has admin access")
	}
	grant := func(access, email string) *httptest.ResponseRecorder {
		t.Helper()
		body, err := json.Marshal(dto.GrantAdminInput{Email: email})
		if err != nil {
			t.Fatal(err)
		}
		return sessionRequest(router, http.MethodPut, "/api/v1/admins", string(body), access, nil)
	}
	if grant(memberSession.AccessToken, "session@example.com").Code != http.StatusForbidden {
		t.Fatal("member can grant admin access")
	}
	for range 2 {
		promoted := grant(ownerSession.AccessToken, "session@example.com")
		if promoted.Code != http.StatusOK {
			t.Fatalf("grant failed: %d %s", promoted.Code, promoted.Body.String())
		}
	}
	if probeAdmin(memberSession.AccessToken) != http.StatusBadRequest {
		t.Fatal("grant was not effective for an already-issued access token")
	}
	adminLogin := sessionRequest(router, http.MethodPost, "/api/v1/login",
		`{"email":"session@example.com","password":"test-password"}`, "", nil)
	adminSession := readAuthResponse(t, adminLogin)
	if !slices.Contains(adminSession.User.Roles, "admin") || adminSession.User.Member == nil {
		t.Fatal("promotion lost roles or personal membership")
	}
	if grant(adminSession.AccessToken, "session@example.com").Code != http.StatusForbidden {
		t.Fatal("ordinary admin can manage admins")
	}
	selfRevoke := sessionRequest(router, http.MethodDelete, "/api/v1/admins/"+targetID.String(),
		"", adminSession.AccessToken, nil)
	if selfRevoke.Code != http.StatusForbidden {
		t.Fatal("ordinary admin can change admin assignments")
	}
	for range 2 {
		revoked := sessionRequest(router, http.MethodDelete, "/api/v1/admins/"+targetID.String(),
			"", ownerSession.AccessToken, nil)
		if revoked.Code != http.StatusOK {
			t.Fatalf("revoke failed: %d %s", revoked.Code, revoked.Body.String())
		}
	}
	if probeAdmin(adminSession.AccessToken) != http.StatusForbidden {
		t.Fatal("revoked admin still has access through an old token")
	}
	me := sessionRequest(router, http.MethodGet, "/api/v1/me", "", adminSession.AccessToken, nil)
	var profile struct{ Data dto.AuthUser }
	if me.Code != http.StatusOK || json.Unmarshal(me.Body.Bytes(), &profile) != nil {
		t.Fatalf("profile after revocation failed: %d %s", me.Code, me.Body.String())
	}
	if !slices.Equal(profile.Data.Roles, []string{"member"}) || profile.Data.Member == nil || !profile.Data.Member.IsActive {
		t.Fatal("admin revocation changed personal membership")
	}
	if grant(ownerSession.AccessToken, owner.Email).Code != http.StatusConflict {
		t.Fatal("admin management modified owner access")
	}
	ownerRevoke := sessionRequest(router, http.MethodDelete, "/api/v1/admins/"+owner.ID.String(),
		"", ownerSession.AccessToken, nil)
	if ownerRevoke.Code != http.StatusConflict {
		t.Fatal("owner access can be revoked through admin management")
	}
	if grant(ownerSession.AccessToken, "missing@example.com").Code != http.StatusNotFound {
		t.Fatal("grant created an account instead of requiring an existing one")
	}
	if sessionRequest(router, http.MethodPut, "/api/v1/admins", `{"email":"session@example.com"}`, "", nil).Code != http.StatusUnauthorized {
		t.Fatal("admin management is not authenticated")
	}
	forged, err := token.NewJwt("session-integration-key", false).GenerateTokenPair(&token.TokenPairParams{
		ID: targetID, Email: owner.Email, Roles: []string{"owner"}, Permissions: []string{"role:assign"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if grant(forged.AccessToken, "session@example.com").Code != http.StatusForbidden {
		t.Fatal("email or token-role claims granted ownership without a stored owner role")
	}
	repeated, err := bootstrap.Bootstrap(ctx, owner.Email)
	if err != nil || repeated.ID != owner.ID || repeated.SetupToken != "" {
		t.Fatalf("repeat bootstrap reset credentials: %+v %v", repeated, err)
	}
	if _, err := bootstrap.Bootstrap(ctx, "another-owner@example.com"); !errors.Is(err, repository.ErrOwnerExists) {
		t.Fatalf("bootstrap allowed a different owner: %v", err)
	}
}

func TestOwnerBootstrapPreservesExistingMember(t *testing.T) {
	t.Parallel()
	db := sessionTestDB(t)
	id := seedSessionUser(t, db, []string{"member"}, true, true)
	var oldHash string
	if err := db.Get(&oldHash, "SELECT password_hash FROM users WHERE id = $1", id); err != nil {
		t.Fatal(err)
	}
	result, err := owners.New(repository.NewOwnerRepository(db)).Bootstrap(context.Background(), "SESSION@example.com")
	if err != nil || result.ID != id || result.SetupToken != "" {
		t.Fatalf("existing account was not preserved: %+v %v", result, err)
	}
	var currentHash string
	if err := db.Get(&currentHash, "SELECT password_hash FROM users WHERE id = $1", id); err != nil {
		t.Fatal(err)
	}
	if currentHash != oldHash {
		t.Fatal("bootstrap replaced an existing password")
	}
	identity, err := repository.NewRoleRepository(db).List(context.Background(), &repository.RoleRepositoryFilter{UserID: &id})
	if err != nil || len(identity) != 2 {
		t.Fatalf("bootstrap did not preserve membership role: %+v %v", identity, err)
	}
}

func TestOwnerBootstrapWithPassword(t *testing.T) {
	for _, pending := range []bool{false, true} {
		name := "new owner"
		if pending {
			name = "pending owner"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			db := sessionTestDB(t)
			router := sessionTestRouter(t, db)
			bootstrap := owners.New(repository.NewOwnerRepository(db))
			email := "owner-password@example.com"
			oldToken := ""
			if pending {
				result, err := bootstrap.Bootstrap(context.Background(), email)
				if err != nil {
					t.Fatal(err)
				}
				oldToken = result.SetupToken
			}
			password := "bootstrap-test-password"
			result, err := bootstrap.BootstrapWithPassword(context.Background(), email, password)
			if err != nil || result.SetupToken != "" {
				t.Fatalf("password bootstrap failed: %+v %v", result, err)
			}
			user, err := repository.NewUserRepository(db).Get(context.Background(), repository.UserRepositoryFilter{ID: &result.ID}, nil)
			if err != nil {
				t.Fatal(err)
			}
			if !user.EmailConfirmedAt.Valid || user.PasswordHash.String == password {
				t.Fatal("password was not safely initialized")
			}
			if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash.String), []byte(password)); err != nil {
				t.Fatal("stored password hash does not match the bootstrap password")
			}
			loginBody, err := json.Marshal(map[string]string{"email": email, "password": password})
			if err != nil {
				t.Fatal(err)
			}
			login := sessionRequest(router, http.MethodPost, "/api/v1/login", string(loginBody), "", nil)
			session := readAuthResponse(t, login)
			if !slices.Contains(session.User.Roles, "owner") || session.User.Member != nil {
				t.Fatal("password bootstrap changed the owner account model")
			}
			if oldToken != "" {
				setupBody, err := json.Marshal(map[string]string{
					"email": email, "password": "replacement-password", "token": oldToken,
				})
				if err != nil {
					t.Fatal(err)
				}
				if got := sessionRequest(router, http.MethodPost, "/api/v1/set-password", string(setupBody), "", nil); got.Code != http.StatusBadRequest {
					t.Fatal("the old setup code can reset the initialized password")
				}
			}
			if _, err := bootstrap.BootstrapWithPassword(context.Background(), email, "different-bootstrap-password"); err != nil {
				t.Fatal(err)
			}
			var currentHash string
			if err := db.Get(&currentHash, "SELECT password_hash FROM users WHERE id = $1", result.ID); err != nil {
				t.Fatal(err)
			}
			if currentHash != user.PasswordHash.String {
				t.Fatal("re-running bootstrap replaced an established password")
			}
		})
	}
}

func TestOwnerBootstrapSerializesDifferentOwners(t *testing.T) {
	t.Parallel()
	db := sessionTestDB(t)
	bootstrap := owners.New(repository.NewOwnerRepository(db))
	errorsOut := make(chan error, 2)
	var wg sync.WaitGroup
	for _, email := range []string{"first@example.com", "second@example.com"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := bootstrap.Bootstrap(context.Background(), email)
			errorsOut <- err
		}()
	}
	wg.Wait()
	close(errorsOut)
	successes, conflicts := 0, 0
	for err := range errorsOut {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, repository.ErrOwnerExists):
			conflicts++
		default:
			t.Fatal(err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("concurrent bootstrap: %d successes, %d conflicts", successes, conflicts)
	}
	var count int
	if err := db.Get(&count, `SELECT COUNT(*) FROM user_roles ur JOIN roles r ON r.id = ur.role_id WHERE r.name = 'owner'`); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatal("multiple owners were created")
	}
}

func TestDevelopmentSeederDoesNotGrantOwnership(t *testing.T) {
	t.Parallel()
	db := sessionTestDB(t)
	for _, slug := range []string{
		"member:read:all", "member:write:all", "loan:apply", "loan:approve", "ledger:read:all", "role:assign",
	} {
		if _, err := db.Exec("INSERT INTO permissions (slug) VALUES ($1)", slug); err != nil {
			t.Fatal(err)
		}
	}
	seeder := &seed.Seed{
		Config: &config.Config{IsDev: true, Server: config.ServerConfig{
			RootUserEmail: "seed-admin@example.com", RootUserPassword: "seed-test-password",
		}},
		DB:             &database.PostgresDB{DB: db},
		UserRepo:       repository.NewUserRepository(db),
		RolesRepo:      repository.NewRoleRepository(db),
		PermissionRepo: repository.NewPermissionRepository(db),
	}
	if err := seeder.CreateRootUser(); err != nil {
		t.Fatal(err)
	}
	email := "seed-admin@example.com"
	account, err := seeder.UserRepo.Get(context.Background(), repository.UserRepositoryFilter{Email: &email}, nil)
	if err != nil {
		t.Fatal(err)
	}
	roles, err := seeder.RolesRepo.List(context.Background(), &repository.RoleRepositoryFilter{UserID: &account.ID})
	if err != nil || len(roles) != 1 || roles[0].Name != "admin" {
		t.Fatalf("seed account received unexpected roles: %+v %v", roles, err)
	}
	permissions, err := seeder.PermissionRepo.List(context.Background(), &repository.PermissionRepositoryFilter{UserID: &account.ID})
	if err != nil {
		t.Fatal(err)
	}
	for _, permission := range permissions {
		if permission.Slug == "role:assign" {
			t.Fatal("development admin received owner-only role assignment permission")
		}
	}
}
