package shares

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"sync"

	"github.com/Jidetireni/ara-cooperative/internal/dto"
	"github.com/Jidetireni/ara-cooperative/internal/repository"
	svc "github.com/Jidetireni/ara-cooperative/internal/services"
	"github.com/Jidetireni/ara-cooperative/internal/services/users"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/samber/lo"
)

var (
	_ SharesRepository      = (*repository.ShareRepository)(nil)
	_ MemberRepository      = (*repository.MemberRepository)(nil)
	_ TransactionRepository = (*repository.TransactionRepository)(nil)
)

type SharesRepository interface {
	Create(ctx context.Context, share repository.Share, tx *sqlx.Tx) (*repository.Share, error)
}

type MemberRepository interface {
	Get(ctx context.Context, filter repository.MemberRepositoryFilter) (*repository.Member, error)
}

type TransactionRepository interface {
	Create(ctx context.Context, transaction repository.Transaction, tx *sqlx.Tx) (*repository.Transaction, error)
	CreateStatus(ctx context.Context, transactionStatus repository.TransactionStatus, tx *sqlx.Tx) (*repository.TransactionStatus, error)
}

type Shares struct {
	DB          *sqlx.DB
	SharesRepo  SharesRepository
	MembersRepo MemberRepository
	TransRepo   TransactionRepository

	mu        sync.RWMutex
	unitPrice int64
}

func New(db *sqlx.DB, sharesRepo SharesRepository, membersRepo MemberRepository, transRepo TransactionRepository) *Shares {
	return &Shares{
		DB:          db,
		SharesRepo:  sharesRepo,
		MembersRepo: membersRepo,
		TransRepo:   transRepo,
	}
}

func (s *Shares) SetUnitPrice(ctx context.Context, input dto.SetShareUnitPriceInput) (string, error) {
	if input.UnitPrice <= 0 {
		return "", fmt.Errorf("unit price must be > 0")
	}
	s.mu.Lock()
	if s.unitPrice == 0 {
		s.unitPrice = SharesUnitPrice
	}
	s.unitPrice = input.UnitPrice
	SharesUnitPrice = input.UnitPrice
	s.mu.Unlock()
	return "Share unit price updated successfully", nil
}

func (s *Shares) GetUnitPrice(ctx context.Context) int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	unitPrice := s.unitPrice
	if unitPrice == 0 {
		unitPrice = SharesUnitPrice
	}
	return unitPrice
}

func (s *Shares) BuyShares(ctx context.Context, input dto.BuySharesInput) (*dto.Shares, error) {
	userID := users.FromContext(ctx).ID
	if userID == uuid.Nil {
		return nil, fmt.Errorf("user ID not found in context")
	}

	unitPrice := s.GetUnitPrice(ctx)
	if unitPrice <= 0 {
		return nil, &svc.ApiError{
			Status:  http.StatusServiceUnavailable,
			Message: "unit price is not set",
		}
	}

	// Compute fractional units with fixed precision
	const precision = 4
	computedUnits := float64(input.Amount) / float64(unitPrice)
	computedUnits = math.Round(computedUnits*math.Pow10(precision)) / math.Pow10(precision)

	if input.Units > 0 {
		const eps = 1e-4
		if math.Abs(computedUnits-input.Units) > eps {
			return nil, &svc.ApiError{
				Status:  http.StatusBadRequest,
				Message: fmt.Sprintf("amount %d does not correspond to units %.4f at unit price %d", input.Amount, input.Units, unitPrice),
			}
		}
	} else {
		// Populate units server-side if client didn't send it
		input.Units = computedUnits
	}

	member, err := s.MembersRepo.Get(ctx, repository.MemberRepositoryFilter{
		UserID: &userID,
	})
	if err != nil {
		return nil, err
	}

	tx, err := s.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	reference := lo.RandomString(12, lo.AlphanumericCharset)
	transaction, err := s.TransRepo.Create(ctx, repository.Transaction{
		MemberID:    member.ID,
		Description: fmt.Sprintf("Purchase of %.4f shares", input.Units),
		Type:        repository.TransactionTypeDEPOSIT,
		Ledger:      repository.LedgerTypeSHARES,
		Amount:      input.Amount,
		Reference:   reference,
	}, tx)
	if err != nil {
		return nil, err
	}

	status, err := s.TransRepo.CreateStatus(ctx, repository.TransactionStatus{
		TransactionID: transaction.ID,
	}, tx)
	if err != nil {
		return nil, err
	}

	shares, err := s.SharesRepo.Create(ctx, repository.Share{
		TransactionID: transaction.ID,
		Units:         fmt.Sprintf("%.4f", input.Units),
		UnitPrice:     unitPrice,
	}, tx)

	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return s.MapRepositoryToDTO(&repository.PopShare{
		ID:            shares.ID,
		TransactionID: shares.TransactionID,
		MemberID:      member.ID,
		Description:   transaction.Description,
		Reference:     transaction.Reference,
		Amount:        transaction.Amount,
		Type:          transaction.Type,
		Units:         shares.Units,
		UnitPrice:     shares.UnitPrice,
		CreatedAt:     shares.CreatedAt,
		ConfirmedAt:   status.ConfirmedAt,
		RejectedAt:    status.RejectedAt,
	}), nil
}

func (s *Shares) MapRepositoryToDTO(share *repository.PopShare) *dto.Shares {
	units, err := strconv.ParseFloat(share.Units, 64)
	if err != nil {
		return nil
	}

	status := dto.SavingsStatusPending
	if share.ConfirmedAt.Valid {
		status = dto.SavingsStatusConfirmed
	} else if share.RejectedAt.Valid {
		status = dto.SavingsStatusRejected
	}
	return &dto.Shares{
		ID:            share.ID,
		TransactionID: share.TransactionID,
		MemberID:      share.MemberID,
		Description:   share.Description,
		Reference:     share.Reference,
		Amount:        share.Amount,
		Type:          dto.TransactionType(share.Type),
		Units:         units,
		UnitPrice:     share.UnitPrice,
		CreatedAt:     share.CreatedAt,
		Status:        status,
	}

}
