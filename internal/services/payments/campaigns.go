package payments

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"net/http"
	"time"

	"github.com/Jidetireni/ara-cooperative/internal/dto"
	"github.com/Jidetireni/ara-cooperative/internal/repository"
	svc "github.com/Jidetireni/ara-cooperative/internal/services"
	"github.com/Jidetireni/ara-cooperative/internal/services/transactions"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

func campaignOpen(campaign *repository.Campaign) error {
	now := time.Now()
	if campaign.Status != "ACTIVE" {
		return &svc.APIError{Status: http.StatusConflict, Message: "campaign is not open for contributions"}
	}
	if now.Before(campaign.StartsAt) || !now.Before(campaign.EndsAt) {
		return &svc.APIError{Status: http.StatusConflict, Message: "campaign is not open for contributions"}
	}
	return nil
}

func (p *Payment) planCampaign(
	ctx context.Context, plan *checkoutPlan, input dto.CreatePaymentInput, tx *sqlx.Tx,
) error {
	if input.CampaignID == nil || *input.CampaignID == uuid.Nil {
		return badRequest("campaign_id is required for SPECIAL_DEPOSIT")
	}
	campaign, err := p.CampaignRepo.Get(ctx, repository.CampaignRepositoryFilter{
		ID: input.CampaignID, ForUpdate: tx != nil,
	}, tx)
	if errors.Is(err, sql.ErrNoRows) {
		return svc.ErrNotFound()
	}
	if err != nil {
		return err
	}
	if err := campaignOpen(campaign); err != nil {
		return err
	}
	amount := input.Amount
	if campaign.AmountType == "FIXED" {
		if amount != 0 && amount != campaign.Amount.Int64 {
			return badRequest("payment amount does not match the campaign's fixed amount")
		}
		amount = campaign.Amount.Int64
	}
	if amount < transactions.MinSpecialDepositAmount || amount > math.MaxInt64/2000 {
		return badRequest("campaign contribution must be at least NGN 500 and within the supported range")
	}
	plan.amount = amount
	plan.campaignID = uuid.NullUUID{UUID: campaign.ID, Valid: true}
	plan.description = campaign.Name
	return nil
}
