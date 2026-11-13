package members

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/Jidetireni/ara-cooperative/internal/config"
	"github.com/Jidetireni/ara-cooperative/internal/constants"
	"github.com/Jidetireni/ara-cooperative/internal/dto"
	"github.com/Jidetireni/ara-cooperative/internal/helpers"
	"github.com/Jidetireni/ara-cooperative/internal/repository"
	svc "github.com/Jidetireni/ara-cooperative/internal/services"
	"github.com/Jidetireni/ara-cooperative/internal/services/users"
	"github.com/Jidetireni/ara-cooperative/pkg/cloudinary"
	"github.com/Jidetireni/ara-cooperative/pkg/email"
	"github.com/Jidetireni/ara-cooperative/pkg/logger"
	"github.com/Jidetireni/ara-cooperative/pkg/token"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"github.com/samber/lo"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var (
	_ MemberRepository     = (*repository.MemberRepository)(nil)
	_ UserRepository       = (*repository.UserRepository)(nil)
	_ RoleRepository       = (*repository.RoleRepository)(nil)
	_ PermissionRepository = (*repository.PermissionRepository)(nil)
	_ TokenRepository      = (*repository.TokenRepository)(nil)
)

var (
	_ EmailPkg      = (*email.Email)(nil)
	_ ImageUploader = (*cloudinary.Client)(nil)
)

type MemberRepository interface {
	NextNumber(ctx context.Context, tx *sqlx.Tx) (int64, error)
	Create(ctx context.Context, member *repository.Member, tx *sqlx.Tx) (*repository.Member, error)
	Get(ctx context.Context, filter repository.MemberRepositoryFilter, tx *sqlx.Tx) (*repository.Member, error)
	Update(ctx context.Context, member *repository.Member, tx *sqlx.Tx) (*repository.Member, error)
	ListPopulated(ctx context.Context, filter repository.MemberRepositoryFilter, opts repository.QueryOptions) (*repository.ListResult[repository.PopulatedMember], error)
	MapRepositoryToDTOModel(member *repository.Member) *dto.Member
	MapPopulatedToDTOModel(populated *repository.PopulatedMember) *dto.MemberSummary
	MapCardToDTOModel(member *repository.Member) *dto.MemberCard
	ListBeneficiaries(ctx context.Context, memberID uuid.UUID, tx *sqlx.Tx) ([]repository.MemberBeneficiary, error)
	ReplaceBeneficiaries(ctx context.Context, memberID uuid.UUID, beneficiaries []repository.MemberBeneficiary, tx *sqlx.Tx) ([]repository.MemberBeneficiary, error)
	SetPassportPhoto(ctx context.Context, memberID uuid.UUID, publicID string, version int64) (*repository.Member, error)
}

// ImageUploader signs direct browser uploads to Cloudinary and verifies
// Cloudinary's upload response.
type ImageUploader interface {
	Enabled() bool
	SignUpload(publicID string) (*cloudinary.UploadSignature, error)
	VerifyUpload(publicID string, version int64, signature string) error
}

type UserRepository interface {
	Get(ctx context.Context, filter repository.UserRepositoryFilter, tx *sqlx.Tx) (*repository.User, error)
	Create(ctx context.Context, user *repository.User, tx *sqlx.Tx) (*repository.User, error)
	Exists(ctx context.Context, filter repository.UserRepositoryFilter) (bool, error)
}

type RoleRepository interface {
	List(ctx context.Context, filter *repository.RoleRepositoryFilter) ([]repository.Role, error)
	AssignToUser(ctx context.Context, userID *uuid.UUID, roleIDs []uuid.UUID, tx *sqlx.Tx) error
}

type PermissionRepository interface {
	List(ctx context.Context, filter *repository.PermissionRepositoryFilter) ([]repository.Permission, error)
	AssignToUser(ctx context.Context, userID *uuid.UUID, permissionIDs []uuid.UUID, tx *sqlx.Tx) error
}

type TokenRepository interface {
	Create(ctx context.Context, token *repository.Token, tx *sqlx.Tx) (*repository.Token, error)
}

type EmailPkg interface {
	Send(ctx context.Context, input *email.SendEmailInput) error
}

type Member struct {
	DB               *sqlx.DB
	Config           *config.Config
	MemberRepository MemberRepository
	UserRepository   UserRepository
	RoleRepository   RoleRepository
	PermissionRepo   PermissionRepository
	TokenRepo        TokenRepository
	Email            EmailPkg
	Images           ImageUploader
	Logger           *logger.Logger
}

func New(
	db *sqlx.DB,
	config *config.Config,
	memberRepo MemberRepository,
	userRepo UserRepository,
	roleRepo RoleRepository,
	permissionRepo PermissionRepository,
	tokenRepo TokenRepository,
	emailPkg EmailPkg,
	images ImageUploader,
	log *logger.Logger,
) *Member {
	return &Member{
		DB:               db,
		Config:           config,
		MemberRepository: memberRepo,
		UserRepository:   userRepo,
		RoleRepository:   roleRepo,
		PermissionRepo:   permissionRepo,
		TokenRepo:        tokenRepo,
		Email:            emailPkg,
		Images:           images,
		Logger:           log,
	}
}

const invitationLifetime = 30 * time.Minute

func (m *Member) Create(
	ctx context.Context, input dto.CreateMemberInput,
) (result *dto.MemberInvitation, err error) {
	permissions := []constants.UserPermissions{constants.MemberWriteALL}
	if !users.HasAdminPermissions(ctx, permissions) {
		return nil, svc.AdminForbiddenError(permissions)
	}
	input.Email = strings.ToLower(strings.TrimSpace(input.Email))
	input.FirstName = strings.TrimSpace(input.FirstName)
	input.LastName = strings.TrimSpace(input.LastName)
	if input.Email == "" || input.FirstName == "" {
		return nil, &svc.APIError{Status: http.StatusBadRequest, Message: "email and member name required"}
	}
	baseURL, err := url.Parse(m.Config.Server.FEURL)
	if err != nil || baseURL.Host == "" || (baseURL.Scheme != "http" && baseURL.Scheme != "https") {
		return nil, errors.New("a valid FE_URL is required for invitations")
	}
	rawToken, err := helpers.GenerateSetupToken()
	if err != nil {
		return nil, fmt.Errorf("generate invitation token: %w", err)
	}
	expiresAt := time.Now().UTC().Add(invitationLifetime)
	tx, err := m.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin member invitation: %w", err)
	}
	defer func() {
		if rollbackErr := tx.Rollback(); rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
			err = errors.Join(err, fmt.Errorf("rollback member invitation: %w", rollbackErr))
		}
	}()
	user, err := m.UserRepository.Get(ctx, repository.UserRepositoryFilter{Email: &input.Email, ForUpdate: true}, tx)
	if errors.Is(err, sql.ErrNoRows) {
		user, err = m.UserRepository.Create(ctx, &repository.User{Email: input.Email}, tx)
		var duplicate *pq.Error
		if errors.As(err, &duplicate) && duplicate.Code == "23505" {
			return nil, &svc.APIError{Status: http.StatusConflict,
				Message: "account already exists; retry the invitation or review the account"}
		}
	}
	if err != nil {
		return nil, fmt.Errorf("find or create invited account: %w", err)
	}
	member, err := m.MemberRepository.Get(ctx, repository.MemberRepositoryFilter{UserID: &user.ID}, tx)
	if errors.Is(err, sql.ErrNoRows) {
		number, numberErr := m.MemberRepository.NextNumber(ctx, tx)
		if numberErr != nil {
			return nil, fmt.Errorf("allocate member number: %w", numberErr)
		}
		member, err = m.MemberRepository.Create(ctx, &repository.Member{
			UserID: user.ID, Slug: fmt.Sprintf("ara%06d", number),
			FirstName: input.FirstName, LastName: input.LastName,
		}, tx)
	}
	if err != nil {
		return nil, fmt.Errorf("find or create pending membership: %w", err)
	}
	if member.ActivatedAt.Valid {
		return nil, &svc.APIError{Status: http.StatusConflict, Message: "this account already has an active membership"}
	}
	if err := m.AssignDefaultRoleAndPermissions(ctx, user.ID, tx); err != nil {
		return nil, fmt.Errorf("assign invited member access: %w", err)
	}
	needsSetup := !user.PasswordHash.Valid || user.PasswordHash.String == ""
	if needsSetup {
		if _, err := m.TokenRepo.Create(ctx, &repository.Token{
			UserID: user.ID, Token: helpers.HashToken(rawToken), TokenType: token.SetPasswordToken,
			IsValid: true, ExpiresAt: expiresAt,
		}, tx); err != nil {
			return nil, fmt.Errorf("save invitation token: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit member invitation: %w", err)
	}
	result = &dto.MemberInvitation{
		Email: user.Email, Member: m.MemberRepository.MapRepositoryToDTOModel(member),
		RequiresPasswordSetup: needsSetup, Delivery: "sent",
	}
	link := baseURL.JoinPath("login")
	link.RawQuery = ""
	link.Fragment = ""
	if needsSetup {
		result.ExpiresAt = &expiresAt
		link = baseURL.JoinPath("set-password")
		link.RawQuery = ""
		// A fragment keeps the setup secret out of HTTP access logs and referrers.
		link.Fragment = "email=" + user.Email + "&token=" + rawToken
		link.RawFragment = url.Values{"email": {user.Email}, "token": {rawToken}}.Encode()
	}
	message, deliveryErr := email.Render(email.EmailTemplateTypeMemberInvitation, email.MemberInvitationData{
		FirstName:          member.FirstName,
		ActionURL:          link.String(),
		NeedsPasswordSetup: needsSetup,
		ExpiresInMinutes:   int(invitationLifetime / time.Minute),
	})
	if deliveryErr == nil {
		deliveryErr = m.Email.Send(ctx, &email.SendEmailInput{
			To: user.Email, Subject: "Your Ara Cooperative membership invitation",
			Body: message.HTML, Text: message.Text,
		})
	}
	if deliveryErr != nil {
		// The committed invitation can be retried by the admin without duplicating membership.
		result.Delivery = "failed"
		m.Logger.Sugar.Errorw("member invitation delivery failed", "error", deliveryErr, "member_id", member.ID.String())
	}
	return result, nil
}

// maxMemberSearchLength bounds the admin search term before it reaches ILIKE.
const maxMemberSearchLength = 100

// List returns members newest first with their account email, for admins.
func (m *Member) List(
	ctx context.Context, filters dto.MemberFilter, options *dto.QueryOptions,
) (*dto.ListResponse[dto.MemberSummary], error) {
	actor, ok := users.FromContext(ctx)
	if !ok || actor.ID == uuid.Nil {
		return nil, svc.UnauthenticatedError()
	}
	if !actor.IsAuthenticatedAsAdmin {
		return nil, &svc.APIError{Status: http.StatusForbidden, Message: "admin access required"}
	}

	repoFilter := repository.MemberRepositoryFilter{IsActive: filters.IsActive}
	if filters.Search != nil {
		search := strings.TrimSpace(*filters.Search)
		if len(search) > maxMemberSearchLength {
			return nil, &svc.APIError{Status: http.StatusBadRequest,
				Message: fmt.Sprintf("search must be at most %d characters", maxMemberSearchLength)}
		}
		if search != "" {
			repoFilter.Search = &search
		}
	}

	// The admin list is always newest first; the repository supplies the column.
	result, err := m.MemberRepository.ListPopulated(ctx, repoFilter, repository.QueryOptions{
		Limit:  options.Limit,
		Cursor: options.Cursor,
	})
	if err != nil {
		return nil, fmt.Errorf("list members: %w", err)
	}

	items := lo.Map(result.Items, func(item *repository.PopulatedMember, _ int) dto.MemberSummary {
		return *m.MemberRepository.MapPopulatedToDTOModel(item)
	})
	return &dto.ListResponse[dto.MemberSummary]{
		Items:      items,
		NextCursor: result.NextCursor,
	}, nil
}

func validProfilePhone(phone string) bool {
	digits := strings.TrimPrefix(phone, "+")
	if len(digits) < 7 || len(digits) > 15 {
		return false
	}
	for _, digit := range digits {
		if digit < '0' || digit > '9' {
			return false
		}
	}
	return true
}

func (m *Member) IsMemberActive(ctx context.Context, memberID uuid.UUID) (bool, error) {
	member, err := m.MemberRepository.Get(ctx, repository.MemberRepositoryFilter{
		ID: &memberID,
	}, nil)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, &svc.APIError{
				Status:  http.StatusNotFound,
				Message: "member not found",
			}
		}
		return false, err
	}

	isActive := false
	if member.ActivatedAt.Valid {
		isActive = true
	}

	return isActive, nil
}

func (m *Member) AssignDefaultRoleAndPermissions(ctx context.Context, userID uuid.UUID, tx *sqlx.Tx) error {
	defaultRole := []string{
		string(constants.RoleMember),
	}

	roles, err := m.RoleRepository.List(ctx, &repository.RoleRepositoryFilter{
		Name: defaultRole,
	})
	if err != nil {
		return err
	}

	if len(roles) != 1 {
		return errors.New("member role is missing; apply database setup first")
	}

	roleIDs := lo.Map(roles, func(role repository.Role, _ int) uuid.UUID {
		return role.ID
	})

	err = m.RoleRepository.AssignToUser(ctx, &userID, roleIDs, tx)
	if err != nil {
		return err
	}

	defaultPermmision := []string{
		string(constants.LoanApply),
	}

	permmisions, err := m.PermissionRepo.List(ctx, &repository.PermissionRepositoryFilter{
		Slug: defaultPermmision,
	})
	if err != nil {
		return err
	}

	permissionIDs := lo.Map(permmisions, func(permission repository.Permission, _ int) uuid.UUID {
		return permission.ID
	})

	if len(permissionIDs) == 0 {
		return nil
	}

	err = m.PermissionRepo.AssignToUser(ctx, &userID, permissionIDs, tx)
	if err != nil {
		return err
	}

	return nil
}
