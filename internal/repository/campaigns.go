package repository

import (
	"context"
	"errors"
	"time"

	"github.com/Jidetireni/ara-cooperative/internal/dto"
	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/samber/lo"
)

type CampaignRepository struct {
	db   *sqlx.DB
	psql sq.StatementBuilderType
}

func NewCampaignRepository(db *sqlx.DB) *CampaignRepository {
	return &CampaignRepository{db: db, psql: sq.StatementBuilder.PlaceholderFormat(sq.Dollar)}
}

type CampaignRepositoryFilter struct {
	ID            *uuid.UUID
	Participation *string
	Status        *string
	ForUpdate     bool
}

type PopulatedCampaign struct {
	Campaign
	TotalContributed    int64
	ContributionCount   int64
	ContributingMembers int64
}

type populatedCampaignFlat struct {
	Campaign
	TotalContributed    int64 `json:"total_contributed"`
	ContributionCount   int64 `json:"contribution_count"`
	ContributingMembers int64 `json:"contributing_members"`
}

func (c *CampaignRepository) applyFilter(builder sq.SelectBuilder, filter CampaignRepositoryFilter) sq.SelectBuilder {
	if filter.ID != nil {
		builder = builder.Where(sq.Eq{"c.id": *filter.ID})
	}
	if filter.Participation != nil {
		builder = builder.Where(sq.Eq{"c.participation": *filter.Participation})
	}
	if filter.Status != nil {
		now := time.Now()
		switch *filter.Status {
		case "UPCOMING":
			builder = builder.Where(sq.Eq{"c.status": "ACTIVE"}).Where(sq.Gt{"c.starts_at": now})
		case "OPEN":
			builder = builder.Where(sq.Eq{"c.status": "ACTIVE"}).
				Where(sq.LtOrEq{"c.starts_at": now}).Where(sq.Gt{"c.ends_at": now})
		case "CLOSED":
			builder = builder.Where(sq.Or{
				sq.Eq{"c.status": "CLOSED"},
				sq.And{sq.Eq{"c.status": "ACTIVE"}, sq.LtOrEq{"c.ends_at": now}},
			})
		case "CANCELLED":
			builder = builder.Where(sq.Eq{"c.status": "CANCELLED"})
		}
	}
	return builder
}

func (c *CampaignRepository) buildQuery(filter CampaignRepositoryFilter, opts QueryOptions) (string, []any, error) {
	return c.generateQuery(filter, opts, false)
}

func (c *CampaignRepository) buildPopulatedQuery(
	filter CampaignRepositoryFilter, opts QueryOptions,
) (string, []any, error) {
	return c.generateQuery(filter, opts, true)
}

func (c *CampaignRepository) generateQuery(
	filter CampaignRepositoryFilter, opts QueryOptions, populated bool,
) (string, []any, error) {
	queryType := lo.FromPtrOr(opts.Type, QueryTypeSelect)
	builder := c.psql.Select("c.*").From("campaigns c")
	if queryType == QueryTypeCount {
		builder = c.psql.Select("COUNT(*)").From("campaigns c")
	} else if populated {
		totals := sq.Select("campaign_id", "SUM(amount) AS total_contributed",
			"COUNT(*) AS contribution_count", "COUNT(DISTINCT member_id) AS contributing_members").
			From("payments").Where(sq.Eq{"status": PaymentStatusSUCCESS}).GroupBy("campaign_id")
		builder = builder.Columns(
			"COALESCE(totals.total_contributed, 0) AS total_contributed",
			"COALESCE(totals.contribution_count, 0) AS contribution_count",
			"COALESCE(totals.contributing_members, 0) AS contributing_members",
		).JoinClause(sq.Expr("LEFT JOIN (?) totals ON totals.campaign_id = c.id", totals))

	}
	builder = c.applyFilter(builder, filter)
	if queryType != QueryTypeCount {
		var err error
		opts.Sort, err = SortFor(opts.Sort, "c")
		if err != nil {
			return "", nil, err
		}
		if filter.ForUpdate {
			builder = builder.Suffix("FOR UPDATE OF c")
		}
		builder, err = ApplyPagination(builder, opts)
		if err != nil {
			return "", nil, err
		}
	}
	return builder.ToSql()
}

func (c *CampaignRepository) Get(ctx context.Context, filter CampaignRepositoryFilter, tx *sqlx.Tx) (*Campaign, error) {
	query, args, err := c.buildQuery(filter, QueryOptions{})
	if err != nil {
		return nil, err
	}
	var campaign Campaign
	if tx != nil {
		err = tx.GetContext(ctx, &campaign, query, args...)
	} else {
		if filter.ForUpdate {
			return nil, errors.New("row locking requires a transaction")
		}
		err = c.db.GetContext(ctx, &campaign, query, args...)
	}
	return &campaign, err
}

func (c *CampaignRepository) GetPopulated(
	ctx context.Context, filter CampaignRepositoryFilter, tx *sqlx.Tx,
) (*PopulatedCampaign, error) {
	query, args, err := c.buildPopulatedQuery(filter, QueryOptions{})
	if err != nil {
		return nil, err
	}
	var flat populatedCampaignFlat
	if tx != nil {
		err = tx.GetContext(ctx, &flat, query, args...)
	} else {
		if filter.ForUpdate {
			return nil, errors.New("row locking requires a transaction")
		}
		err = c.db.GetContext(ctx, &flat, query, args...)
	}
	if err != nil {
		return nil, err
	}
	return c.mapFlatToPopulated(&flat), nil
}

func (c *CampaignRepository) ListPopulated(
	ctx context.Context, filter CampaignRepositoryFilter, opts QueryOptions,
) (*ListResult[PopulatedCampaign], error) {
	query, args, err := c.buildPopulatedQuery(filter, opts)
	if err != nil {
		return nil, err
	}
	flatList := []populatedCampaignFlat{}
	if err := c.db.SelectContext(ctx, &flatList, query, args...); err != nil {
		return nil, err
	}
	items := lo.Map(flatList, func(flat populatedCampaignFlat, _ int) *PopulatedCampaign {
		return c.mapFlatToPopulated(&flat)
	})
	result := &ListResult[PopulatedCampaign]{Items: lo.Slice(items, 0, min(len(items), int(opts.Limit)))}
	if len(items) > int(opts.Limit) {
		last := items[len(items)-1]
		result.NextCursor = lo.ToPtr(EncodeCursor(last.CreatedAt, last.ID))
	}
	return result, nil
}

func (c *CampaignRepository) Create(ctx context.Context, campaign *Campaign, tx *sqlx.Tx) (*Campaign, error) {
	query, args, err := c.psql.Insert("campaigns").
		Columns("admin_id", "name", "description", "participation", "amount_type", "amount", "starts_at", "ends_at").
		Values(campaign.AdminID, campaign.Name, campaign.Description, campaign.Participation,
			campaign.AmountType, campaign.Amount, campaign.StartsAt, campaign.EndsAt).
		Suffix("RETURNING *").ToSql()
	if err != nil {
		return nil, err
	}
	var created Campaign
	if tx != nil {
		err = tx.GetContext(ctx, &created, query, args...)
	} else {
		err = c.db.GetContext(ctx, &created, query, args...)
	}
	return &created, err
}

func (c *CampaignRepository) mapFlatToPopulated(flat *populatedCampaignFlat) *PopulatedCampaign {
	return &PopulatedCampaign{
		Campaign: flat.Campaign, TotalContributed: flat.TotalContributed,
		ContributionCount: flat.ContributionCount, ContributingMembers: flat.ContributingMembers,
	}
}

func (c *CampaignRepository) MapRepositoryToDTOModel(campaign *PopulatedCampaign) *dto.Campaign {
	if campaign == nil {
		return nil
	}
	status := "OPEN"
	now := time.Now()
	if now.Before(campaign.StartsAt) {
		status = "UPCOMING"
	}
	if !now.Before(campaign.EndsAt) {
		status = "CLOSED"
	}
	if campaign.Status != "ACTIVE" {
		status = campaign.Status
	}
	result := &dto.Campaign{
		ID: campaign.ID, Name: campaign.Name, Description: campaign.Description,
		Participation: campaign.Participation, AmountType: campaign.AmountType,
		StartsAt: campaign.StartsAt, EndsAt: campaign.EndsAt, Status: status,
		TotalContributed: campaign.TotalContributed, ContributionCount: campaign.ContributionCount,
		ContributingMembers: campaign.ContributingMembers, CreatedAt: campaign.CreatedAt,
	}
	if campaign.Amount.Valid {
		result.Amount = &campaign.Amount.Int64
	}
	if campaign.StatusReason.Valid {
		result.StatusReason = &campaign.StatusReason.String
		result.StatusChangedBy = &campaign.StatusChangedBy.UUID
		result.StatusChangedAt = &campaign.StatusChangedAt.Time
	}
	if campaign.Resolution.Valid {
		result.Resolution = &campaign.Resolution.String
		result.ResolvedBy = &campaign.ResolvedBy.UUID
		result.ResolvedAt = &campaign.ResolvedAt.Time
	}
	if campaign.UpdatedAt.Valid {
		result.UpdatedAt = &campaign.UpdatedAt.Time
	}
	if campaign.UpdatedBy.Valid {
		result.UpdatedBy = &campaign.UpdatedBy.UUID
	}
	result.IsRefunded = campaign.IsRefunded
	return result
}

func (c *CampaignRepository) Update(ctx context.Context, campaign *Campaign, tx *sqlx.Tx) (*Campaign, error) {
	query, args, err := c.psql.Update("campaigns").
		Set("name", campaign.Name).Set("description", campaign.Description).
		Set("participation", campaign.Participation).Set("amount_type", campaign.AmountType).
		Set("amount", campaign.Amount).Set("starts_at", campaign.StartsAt).Set("ends_at", campaign.EndsAt).
		Set("status", campaign.Status).Set("status_reason", campaign.StatusReason).
		Set("status_changed_by", campaign.StatusChangedBy).Set("status_changed_at", campaign.StatusChangedAt).
		Set("resolution", campaign.Resolution).Set("resolved_by", campaign.ResolvedBy).Set("resolved_at", campaign.ResolvedAt).
		Set("updated_by", campaign.UpdatedBy).Set("is_refunded", campaign.IsRefunded).
		Set("updated_at", time.Now()).Where(sq.Eq{"id": campaign.ID}).Suffix("RETURNING *").ToSql()
	if err != nil {
		return nil, err
	}
	var updated Campaign
	if tx != nil {
		err = tx.GetContext(ctx, &updated, query, args...)
	} else {
		err = c.db.GetContext(ctx, &updated, query, args...)
	}
	return &updated, err
}
