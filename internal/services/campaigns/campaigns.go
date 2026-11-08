package campaigns

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/Jidetireni/ara-cooperative/internal/dto"
	"github.com/Jidetireni/ara-cooperative/internal/repository"
	svc "github.com/Jidetireni/ara-cooperative/internal/services"
	"github.com/Jidetireni/ara-cooperative/internal/services/transactions"
	"github.com/Jidetireni/ara-cooperative/internal/services/users"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/samber/lo"
)

type CampaignRepository interface {
	Create(ctx context.Context, campaign *repository.Campaign, tx *sqlx.Tx) (*repository.Campaign, error)
	Get(ctx context.Context, filter repository.CampaignRepositoryFilter, tx *sqlx.Tx) (*repository.Campaign, error)
	Update(ctx context.Context, campaign *repository.Campaign, tx *sqlx.Tx) (*repository.Campaign, error)
	GetPopulated(
		ctx context.Context, filter repository.CampaignRepositoryFilter, tx *sqlx.Tx,
	) (*repository.PopulatedCampaign, error)
	ListPopulated(
		ctx context.Context, filter repository.CampaignRepositoryFilter, opts repository.QueryOptions,
	) (*repository.ListResult[repository.PopulatedCampaign], error)
	MapRepositoryToDTOModel(campaign *repository.PopulatedCampaign) *dto.Campaign
}

type MemberRepository interface {
	Get(ctx context.Context, filter repository.MemberRepositoryFilter, tx *sqlx.Tx) (*repository.Member, error)
}

type PaymentRepository interface {
	GetTotal(ctx context.Context, filter repository.PaymentRepositoryFilter, tx *sqlx.Tx) (int64, error)
	ListPopulated(
		ctx context.Context, filter repository.PaymentRepositoryFilter, opts repository.QueryOptions,
	) (*repository.ListResult[repository.PopulatedPayment], error)
	MapPopulatedToDTOModel(payment *repository.PopulatedPayment) *dto.Payment
	MapRepositoryToDTOModel(payment *repository.Payment) *dto.Payment
}

var (
	_ CampaignRepository = (*repository.CampaignRepository)(nil)
	_ MemberRepository   = (*repository.MemberRepository)(nil)
	_ PaymentRepository  = (*repository.PaymentRepository)(nil)
)

type Campaign struct {
	DB           *sqlx.DB
	CampaignRepo CampaignRepository
	MemberRepo   MemberRepository
	PaymentRepo  PaymentRepository
}

func New(
	db *sqlx.DB,
	campaignRepo CampaignRepository,
	memberRepo MemberRepository,
	paymentRepo PaymentRepository,
) *Campaign {
	return &Campaign{DB: db, CampaignRepo: campaignRepo, MemberRepo: memberRepo, PaymentRepo: paymentRepo}
}

func (c *Campaign) Create(ctx context.Context, input dto.CampaignInput) (*dto.Campaign, error) {
	actor, ok := users.FromContext(ctx)
	if !ok || actor.ID == uuid.Nil {
		return nil, svc.UnauthenticatedError()
	}
	if !actor.IsAuthenticatedAsAdmin {
		return nil, &svc.APIError{Status: http.StatusForbidden, Message: "admin access required"}
	}
	amount, err := validateCampaignInput(&input)
	if err != nil {
		return nil, err
	}
	campaign, err := c.CampaignRepo.Create(ctx, &repository.Campaign{
		AdminID: actor.ID, Name: input.Name, Description: input.Description,
		Participation: input.Participation, AmountType: input.AmountType, Amount: amount,
		StartsAt: input.StartsAt, EndsAt: input.EndsAt,
	}, nil)
	if err != nil {
		return nil, fmt.Errorf("create campaign: %w", err)
	}
	return c.CampaignRepo.MapRepositoryToDTOModel(&repository.PopulatedCampaign{Campaign: *campaign}), nil
}

// access allows admins and active members. A returned member limits personal views.
func (c *Campaign) access(ctx context.Context) (*repository.Member, error) {
	actor, ok := users.FromContext(ctx)
	if !ok || actor.ID == uuid.Nil {
		return nil, svc.UnauthenticatedError()
	}
	if actor.IsAuthenticatedAsAdmin {
		return nil, nil
	}
	if !actor.IsAuthenticatedAsMember {
		return nil, &svc.APIError{Status: http.StatusForbidden, Message: "member or admin access required"}
	}
	member, err := c.MemberRepo.Get(ctx, repository.MemberRepositoryFilter{UserID: &actor.ID}, nil)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, svc.ErrNotFound()
	}
	if err != nil {
		return nil, err
	}
	if !member.ActivatedAt.Valid {
		return nil, &svc.APIError{Status: http.StatusForbidden, Message: "active membership required"}
	}
	return member, nil
}

func (c *Campaign) Get(ctx context.Context, id uuid.UUID) (*dto.Campaign, error) {
	member, err := c.access(ctx)
	if err != nil {
		return nil, err
	}
	campaign, err := c.CampaignRepo.GetPopulated(ctx, repository.CampaignRepositoryFilter{ID: &id}, nil)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, svc.ErrNotFound()
	}
	if err != nil {
		return nil, err
	}
	result := c.CampaignRepo.MapRepositoryToDTOModel(campaign)
	if member != nil {
		total, err := c.PaymentRepo.GetTotal(ctx, repository.PaymentRepositoryFilter{
			CampaignID: &id, MemberID: &member.ID, Status: lo.ToPtr(repository.PaymentStatusSUCCESS),
		}, nil)
		if err != nil {
			return nil, err
		}
		result.MyContribution = &total

	}
	return result, nil
}

func (c *Campaign) List(
	ctx context.Context, filters dto.CampaignFilters, options *dto.QueryOptions,
) (*dto.ListResponse[dto.Campaign], error) {
	if _, err := c.access(ctx); err != nil {
		return nil, err
	}
	if filters.Status != nil {
		switch *filters.Status {
		case "UPCOMING", "OPEN", "CLOSED", "CANCELLED":
		default:
			return nil, badRequest("status must be UPCOMING, OPEN, CLOSED or CANCELLED")
		}
	}
	if filters.Participation != nil {
		if *filters.Participation != "VOLUNTARY" && *filters.Participation != "REQUIRED" {
			return nil, badRequest("participation must be VOLUNTARY or REQUIRED")
		}
	}
	result, err := c.CampaignRepo.ListPopulated(ctx, repository.CampaignRepositoryFilter{
		Status: filters.Status, Participation: filters.Participation,
	}, repository.QueryOptions{Limit: options.Limit, Cursor: options.Cursor, Sort: options.Sort})
	if err != nil {
		return nil, err
	}
	return &dto.ListResponse[dto.Campaign]{
		Items: lo.Map(result.Items, func(item *repository.PopulatedCampaign, _ int) dto.Campaign {
			return *c.CampaignRepo.MapRepositoryToDTOModel(item)
		}),
		NextCursor: result.NextCursor,
	}, nil
}

func (c *Campaign) Contributions(
	ctx context.Context, id uuid.UUID, options *dto.QueryOptions,
) (*dto.ListResponse[dto.Payment], error) {
	member, err := c.access(ctx)
	if err != nil {
		return nil, err
	}
	if _, err := c.Get(ctx, id); err != nil {
		return nil, err
	}
	filter := repository.PaymentRepositoryFilter{CampaignID: &id, Status: lo.ToPtr(repository.PaymentStatusSUCCESS)}
	if member != nil {
		filter.MemberID = &member.ID
	}
	result, err := c.PaymentRepo.ListPopulated(ctx, filter,
		repository.QueryOptions{Limit: options.Limit, Cursor: options.Cursor, Sort: options.Sort})
	if err != nil {
		return nil, err
	}
	return &dto.ListResponse[dto.Payment]{
		Items: lo.Map(result.Items, func(item *repository.PopulatedPayment, _ int) dto.Payment {
			if member != nil {
				return *c.PaymentRepo.MapRepositoryToDTOModel(&item.Payment)
			}
			return *c.PaymentRepo.MapPopulatedToDTOModel(item)
		}),
		NextCursor: result.NextCursor,
	}, nil
}

func badRequest(message string) error {
	return &svc.APIError{Status: http.StatusBadRequest, Message: message}
}

func validateCampaignInput(input *dto.CampaignInput) (sql.NullInt64, error) {
	input.Name, input.Description = strings.TrimSpace(input.Name), strings.TrimSpace(input.Description)
	if input.Name == "" || input.Description == "" {
		return sql.NullInt64{}, badRequest("campaign name and description are required")
	}
	if input.Participation != "VOLUNTARY" && input.Participation != "REQUIRED" {
		return sql.NullInt64{}, badRequest("participation must be VOLUNTARY or REQUIRED")
	}
	validPeriod := !input.StartsAt.IsZero() && input.EndsAt.After(input.StartsAt)
	if !validPeriod || !input.EndsAt.After(time.Now()) {
		return sql.NullInt64{}, badRequest("ends_at must be in the future and after starts_at")
	}
	var amount sql.NullInt64
	switch input.AmountType {
	case "FIXED":
		if input.Amount < transactions.MinSpecialDepositAmount || input.Amount > math.MaxInt64/2000 {
			return sql.NullInt64{}, badRequest("fixed amount must be at least NGN 500 and within the supported range")
		}
		amount = sql.NullInt64{Int64: input.Amount, Valid: true}
	case "FLEXIBLE":
		if input.Amount != 0 {
			return sql.NullInt64{}, badRequest("omit amount for a flexible campaign")
		}
	default:
		return sql.NullInt64{}, badRequest("amount_type must be FIXED or FLEXIBLE")
	}
	return amount, nil
}
