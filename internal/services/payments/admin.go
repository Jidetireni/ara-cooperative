package payments

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"

	"github.com/Jidetireni/ara-cooperative/internal/dto"
	"github.com/Jidetireni/ara-cooperative/internal/repository"
	svc "github.com/Jidetireni/ara-cooperative/internal/services"
	"github.com/Jidetireni/ara-cooperative/internal/services/users"
	"github.com/google/uuid"
	"github.com/samber/lo"
)

var (
	paymentStatuses = map[dto.PaymentStatus]repository.PaymentStatus{
		dto.PaymentStatusPending: repository.PaymentStatusPENDING,
		dto.PaymentStatusSuccess: repository.PaymentStatusSUCCESS,
		dto.PaymentStatusFailed:  repository.PaymentStatusFAILED,
	}
	paymentPurposes = map[dto.LedgerType]repository.LedgerType{
		"REGISTRATION_FEE": repository.LedgerTypeREGISTRATIONFEE,
		"SAVINGS":          repository.LedgerTypeSAVINGS,
		"SPECIAL_DEPOSIT":  repository.LedgerTypeSPECIALDEPOSIT,
		"SHARES":           repository.LedgerTypeSHARES,
		"FINES":            repository.LedgerTypeFINES,
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

// List returns Paystack checkouts with their members for admins, newest
// first. Pending ones include checkouts the member never finished.
func (p *Payment) List(
	ctx context.Context, filters dto.PaymentFilters, options *dto.QueryOptions,
) (*dto.ListResponse[dto.Payment], error) {
	if err := requireAdmin(ctx); err != nil {
		return nil, err
	}

	repoFilter := repository.PaymentRepositoryFilter{MemberID: filters.MemberID, CampaignID: filters.CampaignID}
	if filters.Status != nil {
		status, ok := paymentStatuses[*filters.Status]
		if !ok {
			return nil, badRequest("status must be PENDING, SUCCESS or FAILED")
		}
		repoFilter.Status = &status
	}
	if filters.Purpose != nil {
		purpose, ok := paymentPurposes[*filters.Purpose]
		if !ok {
			return nil, badRequest("purpose must be one of REGISTRATION_FEE, SAVINGS, SPECIAL_DEPOSIT, SHARES, FINES")
		}
		repoFilter.Purpose = &purpose
	}

	// Newest first, always; this list ignores client sort.
	result, err := p.PaymentRepo.ListPopulated(ctx, repoFilter, repository.QueryOptions{
		Limit: options.Limit, Cursor: options.Cursor,
	})
	if err != nil {
		return nil, err
	}
	return &dto.ListResponse[dto.Payment]{
		Items: lo.Map(result.Items, func(item *repository.PopulatedPayment, _ int) dto.Payment {
			return *p.PaymentRepo.MapPopulatedToDTOModel(item)
		}),
		NextCursor: result.NextCursor,
	}, nil
}

// Verify lets an admin re-check a pending payment with Paystack, for when the
// webhook never arrived. It applies a successful payment exactly as the
// webhook would; anything else leaves the payment pending.
func (p *Payment) Verify(ctx context.Context, reference string) (*dto.PaymentVerification, error) {
	if err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	if !p.Paystack.Enabled() {
		return nil, &svc.APIError{Status: http.StatusServiceUnavailable, Message: "online payments are not available"}
	}

	payment, err := p.PaymentRepo.Get(ctx, repository.PaymentRepositoryFilter{Reference: &reference}, nil)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, svc.ErrNotFound()
	}
	if err != nil {
		return nil, fmt.Errorf("load payment: %w", err)
	}
	if payment.Status != repository.PaymentStatusPENDING {
		return nil, &svc.APIError{Status: http.StatusConflict, Message: "only pending payments can be verified"}
	}

	_, providerStatus, err := p.reconcile(ctx, payment)
	if err != nil {
		p.Logger.Sugar.Errorw("admin payment verification failed", "error", err, "reference", reference)
		return nil, &svc.APIError{Status: http.StatusBadGateway, Message: "could not verify with Paystack; please try again"}
	}

	populated, err := p.PaymentRepo.GetPopulated(ctx, repository.PaymentRepositoryFilter{ID: &payment.ID}, nil)
	if err != nil {
		return nil, fmt.Errorf("reload payment: %w", err)
	}
	p.Logger.Sugar.Infow("admin verified payment",
		"reference", reference, "paystack_status", providerStatus, "status", populated.Status)
	return &dto.PaymentVerification{
		Payment:        p.PaymentRepo.MapPopulatedToDTOModel(populated),
		PaystackStatus: providerStatus,
	}, nil
}
