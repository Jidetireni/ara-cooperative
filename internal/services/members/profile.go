package members

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Jidetireni/ara-cooperative/internal/dto"
	"github.com/Jidetireni/ara-cooperative/internal/repository"
	svc "github.com/Jidetireni/ara-cooperative/internal/services"
	"github.com/Jidetireni/ara-cooperative/internal/services/users"
	"github.com/Jidetireni/ara-cooperative/pkg/cloudinary"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"github.com/samber/lo"
)

func memberAccess(ctx context.Context) (*users.UserContextValue, error) {
	actor, ok := users.FromContext(ctx)
	if !ok || actor.ID == uuid.Nil {
		return nil, svc.UnauthenticatedError()
	}
	if !actor.IsAuthenticatedAsMember {
		return nil, &svc.APIError{Status: http.StatusForbidden, Message: "member access required"}
	}
	return actor, nil
}

func (m *Member) currentMember(ctx context.Context, actorID uuid.UUID, forUpdate bool, tx *sqlx.Tx) (*repository.Member, error) {
	member, err := m.MemberRepository.Get(ctx, repository.MemberRepositoryFilter{UserID: &actorID, ForUpdate: forUpdate}, tx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, svc.ErrNotFound()
	}
	if err != nil {
		return nil, fmt.Errorf("load current member: %w", err)
	}
	return member, nil
}

// profile builds the full membership-form view: scalar fields, beneficiaries,
// passport-photo presence and completeness.
func (m *Member) profile(ctx context.Context, member *repository.Member, tx *sqlx.Tx) (*dto.Member, error) {
	result := m.MemberRepository.MapRepositoryToDTOModel(member)
	beneficiaries, err := m.MemberRepository.ListBeneficiaries(ctx, member.ID, tx)
	if err != nil {
		return nil, fmt.Errorf("load beneficiaries: %w", err)
	}
	result.Beneficiaries = lo.Map(beneficiaries, func(b repository.MemberBeneficiary, _ int) dto.MemberBeneficiary {
		return dto.MemberBeneficiary{Name: b.Name, Relationship: b.Relationship, Address: b.Address, Phone: b.Phone}
	})
	result.ProfileComplete = profileComplete(result)
	return result, nil
}

func profileComplete(p *dto.Member) bool {
	required := []string{
		p.Title, p.Phone, p.Address, p.PlaceOfBirth, p.Nationality, p.StateOfOrigin, p.HomeTown, p.LGA,
		p.NextOfKinName, p.NextOfKinAddress, p.NextOfKinPhone,
	}
	for _, value := range required {
		if strings.TrimSpace(value) == "" {
			return false
		}
	}
	return len(p.Beneficiaries) > 0 && p.PassportPhoto != nil && p.DeclarationAcceptedAt != nil
}

func (m *Member) Current(ctx context.Context) (*dto.Member, error) {
	actor, err := memberAccess(ctx)
	if err != nil {
		return nil, err
	}
	member, err := m.currentMember(ctx, actor.ID, false, nil)
	if err != nil {
		return nil, err
	}
	return m.profile(ctx, member, nil)
}

func text(value string) sql.NullString {
	value = strings.TrimSpace(value)
	return sql.NullString{String: value, Valid: value != ""}
}

func profileError(message string) error {
	return &svc.APIError{Status: http.StatusBadRequest, Message: message}
}

// normalizeProfile trims the input and checks the rules the struct tags cannot express.
func normalizeProfile(input *dto.UpdateMemberProfileInput) error {
	for _, field := range []*string{
		&input.Title, &input.MiddleName, &input.Phone, &input.AlternatePhone, &input.Address, &input.PlaceOfBirth,
		&input.Nationality, &input.StateOfOrigin, &input.HomeTown, &input.LGA, &input.NextOfKinName,
		&input.NextOfKinAddress, &input.NextOfKinPhone, &input.IntroducedByName, &input.IntroducedByNumber,
	} {
		*field = strings.TrimSpace(*field)
	}
	required := []string{
		input.Title, input.Address, input.PlaceOfBirth, input.Nationality, input.StateOfOrigin, input.HomeTown,
		input.LGA, input.NextOfKinName, input.NextOfKinAddress,
	}
	if lo.Contains(required, "") {
		return profileError("complete all required membership form fields")
	}
	phones := []string{input.Phone, input.NextOfKinPhone}
	if input.AlternatePhone != "" {
		phones = append(phones, input.AlternatePhone)
	}
	if len(input.Beneficiaries) == 0 || len(input.Beneficiaries) > 4 {
		return profileError("name between one and four beneficiaries")
	}
	for i := range input.Beneficiaries {
		b := &input.Beneficiaries[i]
		b.Name, b.Relationship = strings.TrimSpace(b.Name), strings.TrimSpace(b.Relationship)
		b.Address, b.Phone = strings.TrimSpace(b.Address), strings.TrimSpace(b.Phone)
		if b.Name == "" || b.Relationship == "" || b.Address == "" {
			return profileError("each beneficiary needs a name, relationship, address and phone")
		}
		phones = append(phones, b.Phone)
	}
	for _, phone := range phones {
		if !validProfilePhone(phone) {
			return profileError("phone numbers need 7 to 15 digits with an optional leading +")
		}
	}
	if w := input.Workplace; w != nil {
		w.Company, w.Address = strings.TrimSpace(w.Company), strings.TrimSpace(w.Address)
		w.Since, w.Position = strings.TrimSpace(w.Since), strings.TrimSpace(w.Position)
		w.Responsibilities = strings.TrimSpace(w.Responsibilities)
		if w.Company == "" || w.Address == "" {
			return profileError("workplace needs a company name and address")
		}
		if w.Since != "" {
			since, err := time.Parse(time.DateOnly, w.Since)
			if err != nil || since.After(time.Now()) {
				return profileError("workplace since must be a past date in YYYY-MM-DD format")
			}
		}
	}
	if input.MonthlyContributionPledge != nil && *input.MonthlyContributionPledge < 0 {
		return profileError("monthly_contribution_pledge must not be negative")
	}
	if !input.DeclarationAccepted {
		return profileError("accept the membership declaration to save your profile")
	}
	return nil
}

func (m *Member) UpdateProfile(
	ctx context.Context, input dto.UpdateMemberProfileInput,
) (result *dto.Member, err error) {
	actor, err := memberAccess(ctx)
	if err != nil {
		return nil, err
	}
	if err := normalizeProfile(&input); err != nil {
		return nil, err
	}
	tx, err := m.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin member profile update: %w", err)
	}
	defer func() {
		if rollbackErr := tx.Rollback(); rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
			err = errors.Join(err, fmt.Errorf("rollback member profile update: %w", rollbackErr))
		}
	}()
	// Lock the current row so saving the profile preserves the latest activation state.
	member, err := m.currentMember(ctx, actor.ID, true, tx)
	if err != nil {
		return nil, err
	}
	member.Title = text(input.Title)
	member.MiddleName = text(input.MiddleName)
	member.Phone = text(input.Phone)
	member.AlternatePhone = text(input.AlternatePhone)
	member.Address = text(input.Address)
	member.PlaceOfBirth = text(input.PlaceOfBirth)
	member.Nationality = text(input.Nationality)
	member.StateOfOrigin = text(input.StateOfOrigin)
	member.HomeTown = text(input.HomeTown)
	member.Lga = text(input.LGA)
	member.NextOfKinName = text(input.NextOfKinName)
	member.NextOfKinAddress = text(input.NextOfKinAddress)
	member.NextOfKinPhone = text(input.NextOfKinPhone)
	member.IntroducedByName, member.IntroducedByNumber = sql.NullString{}, sql.NullString{}
	if number := strings.ToLower(input.IntroducedByNumber); number != "" {
		// The referrer is picked from the member search; store the number and
		// take the name from their record so the two always agree.
		referrer, err := m.MemberRepository.Get(ctx, repository.MemberRepositoryFilter{Slug: &number}, tx)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, profileError("introduced_by_number is not a member")
		}
		if err != nil {
			return nil, fmt.Errorf("load referrer: %w", err)
		}
		if referrer.ID == member.ID {
			return nil, profileError("you cannot introduce yourself")
		}
		member.IntroducedByNumber = text(referrer.Slug)
		member.IntroducedByName = text(strings.TrimSpace(referrer.FirstName + " " + referrer.LastName))
	}
	member.MonthlyContributionPledge = sql.NullInt64{}
	if input.MonthlyContributionPledge != nil {
		member.MonthlyContributionPledge = sql.NullInt64{Int64: *input.MonthlyContributionPledge, Valid: true}
	}
	member.WorkplaceCompany, member.WorkplaceAddress = sql.NullString{}, sql.NullString{}
	member.WorkplacePosition, member.WorkplaceResponsibilities = sql.NullString{}, sql.NullString{}
	member.WorkplaceSince = sql.NullTime{}
	if w := input.Workplace; w != nil {
		member.WorkplaceCompany, member.WorkplaceAddress = text(w.Company), text(w.Address)
		member.WorkplacePosition, member.WorkplaceResponsibilities = text(w.Position), text(w.Responsibilities)
		if w.Since != "" {
			since, _ := time.Parse(time.DateOnly, w.Since)
			member.WorkplaceSince = sql.NullTime{Time: since, Valid: true}
		}
	}
	// Keep the first acceptance time; later saves re-confirm the same declaration.
	if !member.DeclarationAcceptedAt.Valid {
		member.DeclarationAcceptedAt = sql.NullTime{Time: time.Now(), Valid: true}
	}
	updated, err := m.MemberRepository.Update(ctx, member, tx)
	var duplicate *pq.Error
	if errors.As(err, &duplicate) && duplicate.Code == "23505" {
		return nil, &svc.APIError{Status: http.StatusConflict, Message: "phone number is already used by another member"}
	}
	if errors.Is(err, sql.ErrNoRows) {
		return nil, svc.ErrNotFound()
	}
	if err != nil {
		return nil, fmt.Errorf("update member profile: %w", err)
	}
	beneficiaries := lo.Map(input.Beneficiaries, func(b dto.MemberBeneficiaryInput, _ int) repository.MemberBeneficiary {
		return repository.MemberBeneficiary{Name: b.Name, Relationship: b.Relationship, Address: b.Address, Phone: b.Phone}
	})
	if _, err := m.MemberRepository.ReplaceBeneficiaries(ctx, member.ID, beneficiaries, tx); err != nil {
		return nil, fmt.Errorf("save beneficiaries: %w", err)
	}
	result, err = m.profile(ctx, updated, tx)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit member profile update: %w", err)
	}
	return result, nil
}

func (m *Member) bySlug(ctx context.Context, slug string) (*repository.Member, error) {
	member, err := m.MemberRepository.Get(ctx, repository.MemberRepositoryFilter{Slug: &slug}, nil)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, &svc.APIError{Status: http.StatusNotFound, Message: "member not found"}
	}
	if err != nil {
		return nil, fmt.Errorf("load member by number: %w", err)
	}
	return member, nil
}

// GetBySlug returns the directory card for a membership number. Members use it
// to confirm who they are naming, for example as a loan guarantor.
func (m *Member) GetBySlug(ctx context.Context, slug string) (*dto.MemberCard, error) {
	member, err := m.bySlug(ctx, strings.TrimSpace(slug))
	if err != nil {
		return nil, err
	}
	return m.MemberRepository.MapCardToDTOModel(member), nil
}

// MaxMemberSearchResults caps the member search used to pick referrers and
// guarantors.
const MaxMemberSearchResults = 10

// Search finds members by name or membership number and returns directory
// cards only, with is_active so clients can mark members who have not paid
// the membership fee yet (a pending guarantor is an advisory flag at review).
// Any signed-in member may search, including pending members choosing who
// introduced them; email addresses are never matched.
func (m *Member) Search(ctx context.Context, query string) ([]dto.MemberCard, error) {
	if _, err := memberAccess(ctx); err != nil {
		return nil, err
	}
	query = strings.TrimSpace(query)
	if len(query) < 2 || len(query) > maxMemberSearchLength {
		return nil, profileError(fmt.Sprintf("q must be 2 to %d characters", maxMemberSearchLength))
	}
	result, err := m.MemberRepository.ListPopulated(ctx,
		repository.MemberRepositoryFilter{NameOrNumber: &query},
		repository.QueryOptions{Limit: MaxMemberSearchResults})
	if err != nil {
		return nil, fmt.Errorf("search members: %w", err)
	}
	cards := make([]dto.MemberCard, 0, len(result.Items))
	for _, item := range result.Items {
		cards = append(cards, *m.MemberRepository.MapCardToDTOModel(&item.Member))
	}
	return cards, nil
}

func requireAdminAccess(ctx context.Context) error {
	actor, ok := users.FromContext(ctx)
	if !ok || actor.ID == uuid.Nil {
		return svc.UnauthenticatedError()
	}
	if !actor.IsAuthenticatedAsAdmin {
		return &svc.APIError{Status: http.StatusForbidden, Message: "admin access required"}
	}
	return nil
}

// AdminProfile returns a member's full membership application for admins.
func (m *Member) AdminProfile(ctx context.Context, slug string) (*dto.Member, error) {
	if err := requireAdminAccess(ctx); err != nil {
		return nil, err
	}
	member, err := m.bySlug(ctx, slug)
	if err != nil {
		return nil, err
	}
	return m.profile(ctx, member, nil)
}

// passportPublicID names the photo by environment and member, so test and
// production images never collide: ara/<env>/members/<member id>/passport.
func (m *Member) passportPublicID(memberID uuid.UUID) string {
	env := strings.ToLower(strings.TrimSpace(m.Config.Server.Env))
	if env == "" {
		env = "development"
	}
	return fmt.Sprintf("ara/%s/members/%s/passport", env, memberID)
}

func (m *Member) imagesEnabled() error {
	if m.Images == nil || !m.Images.Enabled() {
		return &svc.APIError{Status: http.StatusServiceUnavailable, Message: "photo uploads are not configured"}
	}
	return nil
}

// PassportPhotoUpload signs a direct browser upload of the current member's
// passport photograph. The API chooses the public ID, so a member can only
// ever write to their own image.
func (m *Member) PassportPhotoUpload(ctx context.Context) (*cloudinary.UploadSignature, error) {
	actor, err := memberAccess(ctx)
	if err != nil {
		return nil, err
	}
	if err := m.imagesEnabled(); err != nil {
		return nil, err
	}
	member, err := m.currentMember(ctx, actor.ID, false, nil)
	if err != nil {
		return nil, err
	}
	return m.Images.SignUpload(m.passportPublicID(member.ID))
}

// SavePassportPhoto records an image the browser uploaded to Cloudinary.
// Cloudinary's response signature must match the member's own public ID.
func (m *Member) SavePassportPhoto(ctx context.Context, input dto.PassportPhotoInput) (*dto.Member, error) {
	actor, err := memberAccess(ctx)
	if err != nil {
		return nil, err
	}
	if err := m.imagesEnabled(); err != nil {
		return nil, err
	}
	member, err := m.currentMember(ctx, actor.ID, false, nil)
	if err != nil {
		return nil, err
	}
	publicID := m.passportPublicID(member.ID)
	if input.PublicID != publicID {
		return nil, profileError("public_id does not belong to this member")
	}
	if err := m.Images.VerifyUpload(publicID, input.Version, input.Signature); err != nil {
		return nil, profileError("the upload could not be verified; upload the photo again")
	}
	updated, err := m.MemberRepository.SetPassportPhoto(ctx, member.ID, publicID, input.Version)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, svc.ErrNotFound()
	}
	if err != nil {
		return nil, fmt.Errorf("record passport photo: %w", err)
	}
	return m.profile(ctx, updated, nil)
}
