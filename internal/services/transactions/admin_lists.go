package transactions

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/Jidetireni/ara-cooperative/internal/dto"
	"github.com/Jidetireni/ara-cooperative/internal/repository"
	svc "github.com/Jidetireni/ara-cooperative/internal/services"
	"github.com/Jidetireni/ara-cooperative/internal/services/users"
	"github.com/google/uuid"
	"github.com/samber/lo"
)

const maxTransactionSearchLength = 100

var (
	ledgerTypes = map[string]repository.LedgerType{
		"SAVINGS": repository.LedgerTypeSAVINGS, "SPECIAL_DEPOSIT": repository.LedgerTypeSPECIALDEPOSIT,
		"SHARES": repository.LedgerTypeSHARES, "FINES": repository.LedgerTypeFINES,
		"REGISTRATION_FEE": repository.LedgerTypeREGISTRATIONFEE, "LOAN": repository.LedgerTypeLOAN,
	}
	transactionTypes = map[string]repository.TransactionType{
		"DEPOSIT": repository.TransactionTypeDEPOSIT, "WITHDRAWAL": repository.TransactionTypeWITHDRAWAL,
		"LOAN_DISBURSEMENT": repository.TransactionTypeLOANDISBURSEMENT, "LOAN_REPAYMENT": repository.TransactionTypeLOANREPAYMENT,
	}
)

func requireAdmin(ctx context.Context) error {
	actor, ok := users.FromContext(ctx)
	if !ok || actor.ID == uuid.Nil {
		return svc.UnauthenticatedError()
	}
	if !actor.IsAuthenticatedAsAdmin {
		return &svc.APIError{Status: http.StatusForbidden, Message: "admin access required"}
	}
	return nil
}

func badRequest(message string) error {
	return &svc.APIError{Status: http.StatusBadRequest, Message: message}
}

// ListTransactions lists every member's ledger entries for admins, newest first.
func (t *Transaction) ListTransactions(
	ctx context.Context, filters dto.TransactionFilters, options *dto.QueryOptions,
) (*dto.ListResponse[dto.Transactions], error) {
	if err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	return t.listTransactions(ctx, filters, options)
}

// ListMyTransactions lists the signed-in member's own ledger entries, newest first.
// Only ledger_type, type and status filters apply; member_id and search are ignored.
func (t *Transaction) ListMyTransactions(
	ctx context.Context, filters dto.TransactionFilters, options *dto.QueryOptions,
) (*dto.ListResponse[dto.Transactions], error) {
	actor, ok := users.FromContext(ctx)
	if !ok || actor.ID == uuid.Nil {
		return nil, svc.UnauthenticatedError()
	}
	member, err := t.getMemberByUserID(ctx, actor.ID)
	if err != nil {
		return nil, err
	}
	filters.MemberID = &member.ID
	filters.Search = nil
	return t.listTransactions(ctx, filters, options)
}

func (t *Transaction) listTransactions(
	ctx context.Context, filters dto.TransactionFilters, options *dto.QueryOptions,
) (*dto.ListResponse[dto.Transactions], error) {
	repoFilter := repository.TransactionRepositoryFilter{MemberID: filters.MemberID}
	if filters.LedgerType != nil {
		ledger, ok := ledgerTypes[*filters.LedgerType]
		if !ok {
			return nil, badRequest("invalid ledger_type")
		}
		repoFilter.LedgerType = &ledger
	}
	if filters.Type != nil {
		txnType, ok := transactionTypes[*filters.Type]
		if !ok {
			return nil, badRequest("invalid type")
		}
		repoFilter.Type = &txnType
	}
	if filters.Status != nil {
		switch *filters.Status {
		case dto.TransactionStatusTypePending:
			repoFilter.Confirmed, repoFilter.Rejected = lo.ToPtr(false), lo.ToPtr(false)
		case dto.TransactionStatusTypeConfirmed:
			repoFilter.Confirmed = lo.ToPtr(true)
		case dto.TransactionStatusTypeRejected:
			repoFilter.Rejected = lo.ToPtr(true)
		default:
			return nil, badRequest("status must be PENDING, CONFIRMED or REJECTED")
		}
	}
	if filters.Search != nil {
		search := strings.TrimSpace(*filters.Search)
		if len(search) > maxTransactionSearchLength {
			return nil, badRequest(fmt.Sprintf("search must be at most %d characters", maxTransactionSearchLength))
		}
		if search != "" {
			repoFilter.Search = &search
		}
	}

	// Newest first, always; this list ignores client sort.
	result, err := t.TransactionRepo.ListPopulated(ctx, repoFilter, repository.QueryOptions{
		Limit: options.Limit, Cursor: options.Cursor,
	})
	if err != nil {
		return nil, err
	}
	return &dto.ListResponse[dto.Transactions]{
		Items: lo.Map(result.Items, func(item *repository.PopulatedTransaction, _ int) dto.Transactions {
			return *t.TransactionRepo.MapRepositoryToDTOModel(item)
		}),
		NextCursor: result.NextCursor,
	}, nil
}

// ListSharePurchases lists share purchases for admins, newest first.
func (t *Transaction) ListSharePurchases(
	ctx context.Context, memberID *uuid.UUID, options *dto.QueryOptions,
) (*dto.ListResponse[dto.Shares], error) {
	if err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	result, err := t.ShareRepo.ListPopulated(ctx, repository.ShareRepositoryFilter{MemberID: memberID},
		repository.QueryOptions{Limit: options.Limit, Cursor: options.Cursor})
	if err != nil {
		return nil, err
	}
	return &dto.ListResponse[dto.Shares]{
		Items: lo.Map(result.Items, func(item *repository.PopulatedShare, _ int) dto.Shares {
			return *t.ShareRepo.MapRepositoryToDTOModel(item)
		}),
		NextCursor: result.NextCursor,
	}, nil
}

// LedgerTotals sums confirmed deposits per ledger across all members.
func (t *Transaction) LedgerTotals(ctx context.Context) (*dto.LedgerTotals, error) {
	if err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	total := func(ledger repository.LedgerType) (int64, error) {
		return t.TransactionRepo.GetBalance(ctx, repository.TransactionRepositoryFilter{
			Type:       lo.ToPtr(repository.TransactionTypeDEPOSIT),
			Confirmed:  lo.ToPtr(true),
			LedgerType: lo.ToPtr(ledger),
		})
	}
	var totals dto.LedgerTotals
	for ledger, target := range map[repository.LedgerType]*int64{
		repository.LedgerTypeREGISTRATIONFEE: &totals.MembershipFees,
		repository.LedgerTypeSAVINGS:         &totals.Savings,
		repository.LedgerTypeSPECIALDEPOSIT:  &totals.CampaignContributions,
		repository.LedgerTypeSHARES:          &totals.Shares,
		repository.LedgerTypeFINES:           &totals.Fines,
	} {
		amount, err := total(ledger)
		if err != nil {
			return nil, fmt.Errorf("total %s: %w", ledger, err)
		}
		*target = amount
	}
	return &totals, nil
}
