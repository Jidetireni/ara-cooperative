//go:build integration

package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/Jidetireni/ara-cooperative/internal/dto"
	"github.com/Jidetireni/ara-cooperative/pkg/cloudinary"
	"github.com/samber/lo"
)

// uploadedPhoto plays the browser's part: it takes the signed upload from the
// API and returns what Cloudinary would answer for that public ID.
func uploadedPhoto(t *testing.T, router http.Handler, access string, version int64) map[string]any {
	t.Helper()
	w := sessionRequest(router, http.MethodPost, "/api/v1/members/me/passport-photo/upload", "", access, nil)
	var signed struct{ Data cloudinary.UploadSignature }
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &signed) != nil {
		t.Fatalf("sign upload: %d %s", w.Code, w.Body.String())
	}
	want := cloudinary.Sign(map[string]string{
		"allowed_formats": signed.Data.AllowedFormats, "invalidate": "true", "overwrite": "true",
		"public_id": signed.Data.PublicID, "timestamp": strconv.FormatInt(signed.Data.Timestamp, 10),
	}, testImagesSecret)
	if signed.Data.Signature != want || signed.Data.CloudName != "demo" || signed.Data.APIKey != "test-key" {
		t.Fatalf("upload signature does not cover the signed fields: %+v", signed.Data)
	}
	return map[string]any{
		"public_id": signed.Data.PublicID, "version": version,
		"signature": cloudinary.Sign(map[string]string{
			"public_id": signed.Data.PublicID, "version": strconv.FormatInt(version, 10),
		}, testImagesSecret),
	}
}

func savePhoto(router http.Handler, t *testing.T, body map[string]any, access string) *httptest.ResponseRecorder {
	return sessionRequest(router, http.MethodPut, "/api/v1/members/me/passport-photo", invitationBody(t, body), access, nil)
}

func TestMemberApplicationPhotoAndAdminView(t *testing.T) {
	t.Parallel()
	db := sessionTestDB(t)
	router := sessionTestRouter(t, db)
	userID := seedSessionUser(t, db, []string{"member"}, true, true)
	member := invitationAdminToken(t, router)
	_, admin := loanActor(t, db, router, []string{"admin"}, false)
	var memberID, slug string
	if err := db.QueryRow("SELECT id, slug FROM members WHERE user_id = $1", userID).Scan(&memberID, &slug); err != nil {
		t.Fatal(err)
	}

	profileResult(t, sessionRequest(router, http.MethodPut, "/api/v1/members/me", invitationBody(t, validProfileInput()), member, nil))

	first := uploadedPhoto(t, router, member, 100)
	want := "ara/development/members/" + memberID + "/passport"
	if first["public_id"] != want {
		t.Fatalf("public_id = %v, want %s", first["public_id"], want)
	}
	forged := map[string]any{"public_id": want, "version": 101, "signature": first["signature"]}
	otherMember := map[string]any{"public_id": "ara/development/members/someone-else/passport", "version": 100, "signature": first["signature"]}
	for name, body := range map[string]map[string]any{"forged version": forged, "other member": otherMember} {
		if w := savePhoto(router, t, body, member); w.Code != http.StatusBadRequest {
			t.Fatalf("%s: got %d %s", name, w.Code, w.Body.String())
		}
	}
	saved := profileResult(t, savePhoto(router, t, first, member))
	if saved.PassportPhoto == nil || saved.PassportPhoto.PublicID != want || saved.PassportPhoto.Version != 100 || !saved.ProfileComplete {
		t.Fatalf("photo not recorded or profile incomplete: %+v", saved)
	}
	again := profileResult(t, savePhoto(router, t, uploadedPhoto(t, router, member, 200), member))
	if again.PassportPhoto.PublicID != want || again.PassportPhoto.Version != 200 {
		t.Fatal("replacing the photo did not keep the ID and take the new version")
	}

	var full struct{ Data dto.Member }
	w := sessionRequest(router, http.MethodGet, "/api/v1/members/"+slug+"/profile", "", admin, nil)
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &full) != nil || len(full.Data.Beneficiaries) != 2 ||
		full.Data.PassportPhoto == nil || full.Data.PlaceOfBirth != "Ado Ekiti" || !full.Data.ProfileComplete {
		t.Fatalf("admin profile view: %d %s", w.Code, w.Body.String())
	}
	if w := sessionRequest(router, http.MethodGet, "/api/v1/members/"+slug+"/profile", "", member, nil); w.Code != http.StatusForbidden {
		t.Fatalf("member read the admin profile view: %d", w.Code)
	}
	if w := sessionRequest(router, http.MethodGet, "/api/v1/members/nobody/profile", "", admin, nil); w.Code != http.StatusNotFound {
		t.Fatalf("unknown member: %d", w.Code)
	}

	// Other members see only the directory card, not the application details.
	otherUser, other := loanActor(t, db, router, []string{"member"}, true)
	var otherSlug, otherFirst, otherLast string
	if err := db.QueryRow("SELECT slug, first_name, last_name FROM members WHERE user_id = $1", otherUser).
		Scan(&otherSlug, &otherFirst, &otherLast); err != nil {
		t.Fatal(err)
	}

	// Member search: by name or number, never by email; pending members are
	// included and marked inactive.
	search := func(access, q string) (int, []dto.MemberCard) {
		t.Helper()
		var out struct{ Data []dto.MemberCard }
		w := sessionRequest(router, http.MethodGet, "/api/v1/members/search?q="+url.QueryEscape(q), "", access, nil)
		_ = json.Unmarshal(w.Body.Bytes(), &out)
		return w.Code, out.Data
	}
	found := func(cards []dto.MemberCard, slug string) bool {
		return lo.ContainsBy(cards, func(c dto.MemberCard) bool { return c.Slug == slug })
	}
	if code, cards := search(member, otherSlug); code != http.StatusOK || !found(cards, otherSlug) {
		t.Fatalf("search by number: %d %v", code, cards)
	}
	if code, cards := search(member, strings.ToUpper(otherFirst[:3])); code != http.StatusOK || !found(cards, otherSlug) {
		t.Fatalf("search by name: %d %v", code, cards)
	}
	var otherEmail string
	if err := db.Get(&otherEmail, "SELECT email FROM users WHERE id = $1", otherUser); err != nil {
		t.Fatal(err)
	}
	if _, cards := search(member, otherEmail); found(cards, otherSlug) {
		t.Fatal("member search matched an email address")
	}
	pendingUser, _ := loanActor(t, db, router, []string{"member"}, true)
	if _, err := db.Exec("UPDATE members SET activated_at = NULL, first_name = 'Pendo' WHERE user_id = $1", pendingUser); err != nil {
		t.Fatal(err)
	}
	if code, cards := search(member, "pendo"); code != http.StatusOK || len(cards) != 1 || cards[0].IsActive {
		t.Fatalf("pending member not returned as inactive: %d %+v", code, cards)
	}
	if code, _ := search(member, "a"); code != http.StatusBadRequest {
		t.Fatalf("one-letter search: %d", code)
	}

	// The referral is a member picked from the search; the API takes the name
	// from their record and rejects numbers that are not members.
	referred := validProfileInput()
	referred.IntroducedByName, referred.IntroducedByNumber = "Typed Name", strings.ToUpper(otherSlug)
	withReferrer := profileResult(t, sessionRequest(router, http.MethodPut, "/api/v1/members/me", invitationBody(t, referred), member, nil))
	if withReferrer.IntroducedByNumber != otherSlug || withReferrer.IntroducedByName != otherFirst+" "+otherLast {
		t.Fatalf("referrer not taken from the member record: %q %q", withReferrer.IntroducedByNumber, withReferrer.IntroducedByName)
	}
	for _, number := range []string{"ara999999", slug} {
		referred.IntroducedByNumber = number
		if w := sessionRequest(router, http.MethodPut, "/api/v1/members/me", invitationBody(t, referred), member, nil); w.Code != http.StatusBadRequest {
			t.Fatalf("referrer %s accepted: %d", number, w.Code)
		}
	}
	w = sessionRequest(router, http.MethodGet, "/api/v1/members/"+slug, "", other, nil)
	var card map[string]any
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &card) != nil {
		t.Fatalf("member card: %d %s", w.Code, w.Body.String())
	}
	data := card["data"].(map[string]any)
	for _, private := range []string{"phone", "address", "beneficiaries", "place_of_birth", "next_of_kin_name"} {
		if _, ok := data[private]; ok {
			t.Fatalf("member card exposed %s", private)
		}
	}
	if data["slug"] != slug || data["middle_name"] != "Adunni" {
		t.Fatalf("member card missing identity: %v", data)
	}
}
