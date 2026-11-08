package campaigns

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/Jidetireni/ara-cooperative/internal/dto"
	"github.com/Jidetireni/ara-cooperative/internal/repository"
	svc "github.com/Jidetireni/ara-cooperative/internal/services"
	"github.com/Jidetireni/ara-cooperative/internal/services/users"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

func requireAdmin(ctx context.Context) (*users.UserContextValue, error) {
	actor, ok := users.FromContext(ctx)
	if !ok || actor.ID == uuid.Nil {
		return nil, svc.UnauthenticatedError()
	}
	if !actor.IsAuthenticatedAsAdmin {
		return nil, &svc.APIError{Status: http.StatusForbidden, Message: "admin access required"}
	}
	return actor, nil
}

func (c *Campaign) Update(ctx context.Context, id uuid.UUID, input dto.CampaignInput) (*dto.Campaign, error) {
	actor, err := requireAdmin(ctx)
	if err != nil {
		return nil, err
	}
	amount, err := validateCampaignInput(&input)
	if err != nil {
		return nil, err
	}
	tx, err := c.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	campaign, err := c.lockCampaign(ctx, id, tx)
	if err != nil {
		return nil, err
	}
	if campaign.Status != "ACTIVE" || !time.Now().Before(campaign.EndsAt) {
		return nil, &svc.APIError{Status: http.StatusConflict, Message: "closed or cancelled campaigns cannot be edited"}
	}
	campaign.Name, campaign.Description = input.Name, input.Description
	campaign.Participation, campaign.AmountType, campaign.Amount = input.Participation, input.AmountType, amount
	campaign.StartsAt, campaign.EndsAt = input.StartsAt, input.EndsAt
	campaign.UpdatedBy = uuid.NullUUID{UUID: actor.ID, Valid: true}
	if _, err := c.CampaignRepo.Update(ctx, campaign, tx); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return c.Get(ctx, id)
}

func (c *Campaign) ChangeStatus(
	ctx context.Context, id uuid.UUID, input dto.CampaignStatusInput,
) (*dto.Campaign, error) {
	actor, err := requireAdmin(ctx)
	if err != nil {
		return nil, err
	}
	input.Reason = strings.TrimSpace(input.Reason)
	if input.Reason == "" || (input.Status != "CLOSED" && input.Status != "CANCELLED") {
		return nil, badRequest("a reason and CLOSED or CANCELLED status are required")
	}
	tx, err := c.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	campaign, err := c.lockCampaign(ctx, id, tx)
	if err != nil {
		return nil, err
	}
	if campaign.Status == "CANCELLED" || campaign.Status == input.Status {
		return nil, &svc.APIError{Status: http.StatusConflict, Message: "campaign is already closed or cancelled"}
	}
	campaign.Status = input.Status
	campaign.StatusReason = sql.NullString{String: input.Reason, Valid: true}
	campaign.StatusChangedBy = uuid.NullUUID{UUID: actor.ID, Valid: true}
	campaign.StatusChangedAt = sql.NullTime{Time: time.Now(), Valid: true}
	campaign.UpdatedBy = uuid.NullUUID{UUID: actor.ID, Valid: true}
	if _, err := c.CampaignRepo.Update(ctx, campaign, tx); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return c.Get(ctx, id)
}

func (c *Campaign) Resolve(
	ctx context.Context, id uuid.UUID, input dto.CampaignResolutionInput,
) (*dto.Campaign, error) {
	actor, err := requireAdmin(ctx)
	if err != nil {
		return nil, err
	}
	input.Resolution = strings.TrimSpace(input.Resolution)
	if input.Resolution == "" {
		return nil, badRequest("resolution is required")
	}
	tx, err := c.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	campaign, err := c.lockCampaign(ctx, id, tx)
	if err != nil {
		return nil, err
	}
	if campaign.Status != "CANCELLED" {
		return nil, &svc.APIError{Status: http.StatusConflict, Message: "only cancelled campaigns need a resolution"}
	}
	campaign.Resolution = sql.NullString{String: input.Resolution, Valid: true}
	campaign.ResolvedBy = uuid.NullUUID{UUID: actor.ID, Valid: true}
	campaign.ResolvedAt = sql.NullTime{Time: time.Now(), Valid: true}
	campaign.UpdatedBy = uuid.NullUUID{UUID: actor.ID, Valid: true}
	if _, err := c.CampaignRepo.Update(ctx, campaign, tx); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return c.Get(ctx, id)
}

func (c *Campaign) lockCampaign(ctx context.Context, id uuid.UUID, tx *sqlx.Tx) (*repository.Campaign, error) {
	campaign, err := c.CampaignRepo.Get(ctx, repository.CampaignRepositoryFilter{ID: &id, ForUpdate: true}, tx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, svc.ErrNotFound()
	}
	return campaign, err
}

func (c *Campaign) SetRefundStatus(
	ctx context.Context, id uuid.UUID, input dto.CampaignRefundStatusInput,
) (*dto.Campaign, error) {
	actor, err := requireAdmin(ctx)
	if err != nil {
		return nil, err
	}
	if input.IsRefunded == nil {
		return nil, badRequest("is_refunded is required")
	}
	tx, err := c.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	campaign, err := c.lockCampaign(ctx, id, tx)
	if err != nil {
		return nil, err
	}
	if campaign.Status == "ACTIVE" && time.Now().Before(campaign.EndsAt) {
		return nil, &svc.APIError{Status: http.StatusConflict, Message: "close or cancel the campaign before marking its refund status"}
	}
	campaign.IsRefunded = *input.IsRefunded
	campaign.UpdatedBy = uuid.NullUUID{UUID: actor.ID, Valid: true}
	if _, err := c.CampaignRepo.Update(ctx, campaign, tx); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return c.Get(ctx, id)
}
