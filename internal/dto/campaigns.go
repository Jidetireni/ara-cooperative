package dto

import (
	"time"

	"github.com/google/uuid"
)

type CampaignInput struct {
	Name          string    `json:"name" validate:"required,max=255"`
	Description   string    `json:"description" validate:"required,max=2000"`
	Participation string    `json:"participation" validate:"required,oneof=VOLUNTARY REQUIRED"`
	AmountType    string    `json:"amount_type" validate:"required,oneof=FIXED FLEXIBLE"`
	Amount        int64     `json:"amount,omitempty" validate:"omitempty,gt=0"`
	StartsAt      time.Time `json:"starts_at" validate:"required"`
	EndsAt        time.Time `json:"ends_at" validate:"required"`
}

type Campaign struct {
	ID                  uuid.UUID  `json:"id"`
	Name                string     `json:"name"`
	Description         string     `json:"description"`
	Participation       string     `json:"participation"`
	AmountType          string     `json:"amount_type"`
	Amount              *int64     `json:"amount,omitempty"`
	StartsAt            time.Time  `json:"starts_at"`
	EndsAt              time.Time  `json:"ends_at"`
	Status              string     `json:"status"`
	TotalContributed    int64      `json:"total_contributed"`
	ContributionCount   int64      `json:"contribution_count"`
	ContributingMembers int64      `json:"contributing_members"`
	MyContribution      *int64     `json:"my_contribution,omitempty"`
	StatusReason        *string    `json:"status_reason,omitempty"`
	StatusChangedBy     *uuid.UUID `json:"status_changed_by,omitempty"`
	StatusChangedAt     *time.Time `json:"status_changed_at,omitempty"`
	Resolution          *string    `json:"resolution,omitempty"`
	ResolvedBy          *uuid.UUID `json:"resolved_by,omitempty"`
	ResolvedAt          *time.Time `json:"resolved_at,omitempty"`
	IsRefunded          bool       `json:"is_refunded"`
	UpdatedBy           *uuid.UUID `json:"updated_by,omitempty"`
	UpdatedAt           *time.Time `json:"updated_at,omitempty"`
	CreatedAt           time.Time  `json:"created_at"`
}

type CampaignStatusInput struct {
	Status string `json:"status" validate:"required,oneof=CLOSED CANCELLED"`
	Reason string `json:"reason" validate:"required,max=2000"`
}

type CampaignResolutionInput struct {
	Resolution string `json:"resolution" validate:"required,max=2000"`
}

type CampaignFilters struct {
	Status        *string
	Participation *string
}

type CampaignRefundStatusInput struct {
	IsRefunded *bool `json:"is_refunded" validate:"required"`
}
