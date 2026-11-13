// Package payments runs online (Paystack) payments: checkout, verification,
// and applying each verified payment's effect exactly once.
package payments

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Jidetireni/ara-cooperative/internal/config"
	"github.com/Jidetireni/ara-cooperative/internal/dto"
	"github.com/Jidetireni/ara-cooperative/internal/repository"
	svc "github.com/Jidetireni/ara-cooperative/internal/services"
	"github.com/Jidetireni/ara-cooperative/internal/services/transactions"
	"github.com/Jidetireni/ara-cooperative/internal/services/users"
	"github.com/Jidetireni/ara-cooperative/pkg/logger"
	"github.com/Jidetireni/ara-cooperative/pkg/paystack"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/samber/lo"
)

var (
	_ PaymentRepository     = (*repository.PaymentRepository)(nil)
	_ MemberRepository      = (*repository.MemberRepository)(nil)
	_ UserRepository        = (*repository.UserRepository)(nil)
	_ TransactionRepository = (*repository.TransactionRepository)(nil)
	_ ShareRepository       = (*repository.ShareRepository)(nil)
	_ FineRepository        = (*repository.FineRepository)(nil)
	_ ShareQuoter           = (*transactions.Transaction)(nil)
	_ PaystackClient        = (*paystack.Client)(nil)
	_ CampaignRepository    = (*repository.CampaignRepository)(nil)
)

const currencyNGN = "NGN"

type PaymentRepository interface {
	Get(ctx context.Context, filter repository.PaymentRepositoryFilter, tx *sqlx.Tx) (*repository.Payment, error)
	Create(ctx context.Context, payment *repository.Payment, tx *sqlx.Tx) (*repository.Payment, error)
	Update(ctx context.Context, payment *repository.Payment, tx *sqlx.Tx) (*repository.Payment, error)
	GetPopulated(ctx context.Context, filter repository.PaymentRepositoryFilter, tx *sqlx.Tx) (*repository.PopulatedPayment, error)
	ListPopulated(ctx context.Context, filter repository.PaymentRepositoryFilter, opts repository.QueryOptions) (*repository.ListResult[repository.PopulatedPayment], error)
	MapRepositoryToDTOModel(payment *repository.Payment) *dto.Payment
	MapPopulatedToDTOModel(populated *repository.PopulatedPayment) *dto.Payment
}

type MemberRepository interface {
	Get(ctx context.Context, filter repository.MemberRepositoryFilter, tx *sqlx.Tx) (*repository.Member, error)
	Update(ctx context.Context, member *repository.Member, tx *sqlx.Tx) (*repository.Member, error)
}

type UserRepository interface {
	Get(ctx context.Context, filter repository.UserRepositoryFilter, tx *sqlx.Tx) (*repository.User, error)
}

type TransactionRepository interface {
	Create(ctx context.Context, transaction repository.Transaction, tx *sqlx.Tx) (*repository.Transaction, error)
	CreateStatus(ctx context.Context, transactionStatus repository.TransactionStatus, tx *sqlx.Tx) (*repository.TransactionStatus, error)
}

type ShareRepository interface {
	Create(ctx context.Context, share repository.Share, tx *sqlx.Tx) (*repository.Share, error)
}

type FineRepository interface {
	GetPopulated(ctx context.Context, filter repository.FineRepositoryFilter, tx *sqlx.Tx) (*repository.PopulatedFine, error)
	Update(ctx context.Context, fine *repository.Fine, tx *sqlx.Tx) (*repository.Fine, error)
}

// ShareQuoter is the transactions service's share quote, so online and
// manual share purchases price units the same way.
type ShareQuoter interface {
	GetShareQuote(ctx context.Context, amount int64) (*dto.GetUnitsQuote, error)
}

type CampaignRepository interface {
	Get(ctx context.Context, filter repository.CampaignRepositoryFilter, tx *sqlx.Tx) (*repository.Campaign, error)
}

type PaystackClient interface {
	Enabled() bool
	InitializeTransaction(ctx context.Context, input paystack.InitializeInput) (*paystack.InitializeResult, error)
	VerifyTransaction(ctx context.Context, reference string) (*paystack.Transaction, error)
	ValidSignature(body []byte, signature string) bool
}

type Payment struct {
	DB              *sqlx.DB
	Config          *config.Config
	PaymentRepo     PaymentRepository
	MemberRepo      MemberRepository
	UserRepo        UserRepository
	TransactionRepo TransactionRepository
	ShareRepo       ShareRepository
	FineRepo        FineRepository
	CampaignRepo    CampaignRepository
	ShareQuoter     ShareQuoter
	Paystack        PaystackClient
	Logger          *logger.Logger
}

func New(
	db *sqlx.DB,
	cfg *config.Config,
	paymentRepo PaymentRepository,
	memberRepo MemberRepository,
	userRepo UserRepository,
	transactionRepo TransactionRepository,
	shareRepo ShareRepository,
	fineRepo FineRepository,
	campaignRepo CampaignRepository,
	shareQuoter ShareQuoter,
	paystackClient PaystackClient,
	log *logger.Logger,
) *Payment {
	return &Payment{
		DB:              db,
		Config:          cfg,
		PaymentRepo:     paymentRepo,
		MemberRepo:      memberRepo,
		UserRepo:        userRepo,
		TransactionRepo: transactionRepo,
		ShareRepo:       shareRepo,
		FineRepo:        fineRepo,
		CampaignRepo:    campaignRepo,
		ShareQuoter:     shareQuoter,
		Paystack:        paystackClient,
		Logger:          log,
	}
}

// checkoutPlan is a validated payment before it is stored.
type checkoutPlan struct {
	member         *repository.Member
	purpose        repository.LedgerType
	amount         int64
	description    string
	fineID         uuid.NullUUID
	campaignID     uuid.NullUUID
	shareUnitPrice sql.NullInt64
	shareUnits     sql.NullString
	quote          dto.PaymentQuote
}

// Quote returns what the member would pay, without starting a checkout.
func (p *Payment) Quote(ctx context.Context, input dto.CreatePaymentInput) (*dto.PaymentQuote, error) {
	plan, err := p.plan(ctx, input)
	if err != nil {
		return nil, err
	}
	return &plan.quote, nil
}

// Create stores a pending payment and starts a Paystack checkout. The member
// pays on the returned authorization_url.
func (p *Payment) Create(ctx context.Context, input dto.CreatePaymentInput) (*dto.Payment, error) {
	if !p.Paystack.Enabled() {
		return nil, &svc.APIError{Status: http.StatusServiceUnavailable, Message: "online payments are not available"}
	}
	ownerSubaccount := strings.TrimSpace(p.Config.Paystack.OwnerSubaccount)
	if ownerSubaccount == "" {
		return nil, &svc.APIError{
			Status:  http.StatusServiceUnavailable,
			Message: "online payments are not available: owner settlement account is not configured",
		}
	}
	plan, err := p.plan(ctx, input)
	if err != nil {
		return nil, err
	}
	if plan.purpose == repository.LedgerTypeREGISTRATIONFEE {
		// The membership form's declaration is accepted only when the whole form
		// is saved, so it marks a complete application. The passport photo is
		// not required here, so a storage outage cannot block joining.
		incomplete := !plan.member.DeclarationAcceptedAt.Valid
		for _, field := range []sql.NullString{
			plan.member.Phone, plan.member.Address, plan.member.NextOfKinName, plan.member.NextOfKinPhone,
		} {
			incomplete = incomplete || !field.Valid || strings.TrimSpace(field.String) == ""
		}
		if incomplete {
			return nil, &svc.APIError{
				Status:  http.StatusConflict,
				Message: "complete your membership application (contact, next-of-kin and beneficiary details, and the declaration) before paying the membership fee",
			}
		}
	}
	var tx *sqlx.Tx
	if plan.campaignID.Valid {
		tx, err = p.DB.BeginTxx(ctx, nil)
		if err != nil {
			return nil, err
		}
		defer func() { _ = tx.Rollback() }()
		// Lock and recheck current campaign terms against edits and closure.
		description := plan.description
		if err := p.planCampaign(ctx, plan, input, tx); err != nil {
			return nil, err
		}
		if strings.TrimSpace(input.Description) != "" {
			plan.description = description
		}
		p.quotePlan(plan)
	}
	user, err := p.UserRepo.Get(ctx, repository.UserRepositoryFilter{ID: &plan.member.UserID}, nil)
	if err != nil {
		return nil, fmt.Errorf("load member account: %w", err)
	}
	reference, err := newReference(plan.purpose)
	if err != nil {
		return nil, err
	}

	// The main cooperative account keeps the principal (transaction_charge).
	// The owner's subaccount receives the rest, the maintenance fee plus
	// processing charge, and bears Paystack's fee. A dynamic flat split would
	// reject owner shares under NGN 100.
	ownerShare := plan.quote.MaintenanceFee + plan.quote.ProcessingCharge

	payment, err := p.PaymentRepo.Create(ctx, &repository.Payment{
		MemberID:              plan.member.ID,
		Purpose:               plan.purpose,
		Reference:             reference,
		Currency:              currencyNGN,
		Amount:                plan.quote.Amount,
		MaintenanceFee:        plan.quote.MaintenanceFee,
		MaintenanceFeePercent: p.Config.Payments.MaintenanceFeePercent,
		ProcessingCharge:      plan.quote.ProcessingCharge,
		Total:                 plan.quote.Total,
		Description:           plan.description,
		FineID:                plan.fineID,
		CampaignID:            plan.campaignID,
		ShareUnitPrice:        plan.shareUnitPrice,
		ShareUnits:            plan.shareUnits,
		SettlementSubaccount:  sql.NullString{String: ownerSubaccount, Valid: true},
		SettlementShare:       sql.NullInt64{Int64: ownerShare, Valid: true},
	}, tx)
	if err != nil {
		return nil, fmt.Errorf("create payment: %w", err)
	}
	if tx != nil {
		if err := tx.Commit(); err != nil {
			return nil, err
		}
	}

	metadata := paystack.Metadata{
		PaymentID:        payment.ID.String(),
		MemberID:         plan.member.ID.String(),
		MemberNumber:     plan.member.Slug,
		Purpose:          string(plan.purpose),
		Amount:           payment.Amount,
		MaintenanceFee:   payment.MaintenanceFee,
		ProcessingCharge: payment.ProcessingCharge,
		CancelAction:     p.callbackURL(reference, true),
	}
	if plan.fineID.Valid {
		metadata.FineID = plan.fineID.UUID.String()
	}
	if plan.campaignID.Valid {
		metadata.CampaignID = plan.campaignID.UUID.String()
	}

	checkout, err := p.Paystack.InitializeTransaction(ctx, paystack.InitializeInput{
		Email:       user.Email,
		Amount:      payment.Total,
		Currency:    currencyNGN,
		Reference:   reference,
		CallbackURL: p.callbackURL(reference, false),
		Metadata:    metadata,
		// Persisted payments keep Total = Amount + ownerShare, so the
		// subaccount receives exactly the stored settlement share.
		Subaccount:        ownerSubaccount,
		TransactionCharge: payment.Amount,
		Bearer:            paystack.BearerSubaccount,
	})
	if err != nil {
		p.Logger.Sugar.Errorw("paystack checkout failed", "error", err, "reference", reference)
		payment.Status = repository.PaymentStatusFAILED
		payment.FailureReason = sql.NullString{String: "checkout could not be started", Valid: true}
		if _, updateErr := p.PaymentRepo.Update(ctx, payment, nil); updateErr != nil {
			p.Logger.Sugar.Errorw("mark payment failed", "error", updateErr, "reference", reference)
		}
		return nil, &svc.APIError{Status: http.StatusBadGateway, Message: "could not start the payment; please try again"}
	}

	payment.AuthorizationUrl = sql.NullString{String: checkout.AuthorizationURL, Valid: true}
	payment.AccessCode = sql.NullString{String: checkout.AccessCode, Valid: checkout.AccessCode != ""}
	payment, err = p.PaymentRepo.Update(ctx, payment, nil)
	if err != nil {
		return nil, fmt.Errorf("save checkout url: %w", err)
	}
	return p.PaymentRepo.MapRepositoryToDTOModel(payment), nil
}

// Get returns the stored state of the member's own payment.
// Verification and financial effects are handled by the signed webhook.
func (p *Payment) Get(ctx context.Context, reference string) (*dto.Payment, error) {
	member, err := p.currentMember(ctx)
	if err != nil {
		return nil, err
	}
	payment, err := p.PaymentRepo.Get(ctx, repository.PaymentRepositoryFilter{
		Reference: &reference, MemberID: &member.ID,
	}, nil)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, svc.ErrNotFound()
	}
	if err != nil {
		return nil, fmt.Errorf("load payment: %w", err)
	}
	return p.PaymentRepo.MapRepositoryToDTOModel(payment), nil
}

// HandleWebhook verifies the signature and dispatches Paystack events. It
// returns nil for events Ara does not handle, so Paystack stops retrying them.
func (p *Payment) HandleWebhook(ctx context.Context, body []byte, signature string) error {
	if !p.Paystack.Enabled() {
		return &svc.APIError{Status: http.StatusServiceUnavailable, Message: "online payments are not available"}
	}
	if !p.Paystack.ValidSignature(body, signature) {
		return &svc.APIError{Status: http.StatusUnauthorized, Message: "invalid signature"}
	}

	var event paystack.WebhookEvent
	if err := json.Unmarshal(body, &event); err != nil {
		return &svc.APIError{Status: http.StatusBadRequest, Message: "invalid webhook payload"}
	}

	switch event.Event {
	case paystack.EventChargeSuccess:
		var charge paystack.ChargeData
		if err := json.Unmarshal(event.Data, &charge); err != nil || charge.Reference == "" {
			return &svc.APIError{Status: http.StatusBadRequest, Message: "invalid charge payload"}
		}
		payment, err := p.PaymentRepo.Get(ctx, repository.PaymentRepositoryFilter{Reference: &charge.Reference}, nil)
		if errors.Is(err, sql.ErrNoRows) {
			// Not an Ara checkout (e.g. another integration on the account).
			p.Logger.Sugar.Infow("ignoring charge for unknown reference", "reference", charge.Reference)
			return nil
		}
		if err != nil {
			return fmt.Errorf("load payment for webhook: %w", err)
		}
		if _, _, err := p.reconcile(ctx, payment); err != nil {
			return fmt.Errorf("apply charge %s: %w", charge.Reference, err)
		}
		return nil
	default:
		p.Logger.Sugar.Infow("ignoring paystack event", "event", event.Event)
		return nil
	}
}

// reconcile verifies a pending payment with Paystack (never trusting the
// webhook body alone) and applies it if it succeeded.
func (p *Payment) reconcile(ctx context.Context, payment *repository.Payment) (*repository.Payment, string, error) {
	if payment.Status != repository.PaymentStatusPENDING {
		return payment, "", nil
	}
	verified, err := p.Paystack.VerifyTransaction(ctx, payment.Reference)
	if err != nil {
		return nil, "", fmt.Errorf("verify with paystack: %w", err)
	}
	if verified.Status != paystack.StatusSuccess {
		// Abandoned or failed attempts can still be retried on the same checkout.
		return payment, verified.Status, nil
	}

	tx, err := p.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, verified.Status, err
	}
	defer func() { _ = tx.Rollback() }()

	// Lock the row so concurrent webhook deliveries apply it only once.
	locked, err := p.PaymentRepo.Get(ctx, repository.PaymentRepositoryFilter{ID: &payment.ID, ForUpdate: true}, tx)
	if err != nil {
		return nil, verified.Status, fmt.Errorf("lock payment: %w", err)
	}
	if locked.Status != repository.PaymentStatusPENDING {
		return locked, verified.Status, tx.Commit()
	}

	locked.ProviderTransactionID = sql.NullInt64{Int64: verified.ID, Valid: verified.ID != 0}
	locked.ProviderFees = sql.NullInt64{Int64: verified.Fees, Valid: true}
	locked.Channel = sql.NullString{String: verified.Channel, Valid: verified.Channel != ""}

	if verified.Amount != locked.Total || !strings.EqualFold(verified.Currency, locked.Currency) {
		p.Logger.Sugar.Errorw("paystack amount mismatch; payment not applied",
			"reference", locked.Reference, "expected", locked.Total, "received", verified.Amount, "currency", verified.Currency)
		locked.Status = repository.PaymentStatusFAILED
		locked.FailureReason = sql.NullString{
			String: fmt.Sprintf("paid %d %s, expected %d %s", verified.Amount, verified.Currency, locked.Total, locked.Currency),
			Valid:  true,
		}
		updated, err := p.PaymentRepo.Update(ctx, locked, tx)
		if err != nil {
			return nil, verified.Status, err
		}
		return updated, verified.Status, tx.Commit()
	}

	paidAt := time.Now()
	if verified.PaidAt != nil {
		paidAt = *verified.PaidAt
	}
	transactionID, err := p.apply(ctx, locked, tx)
	if err != nil {
		return nil, verified.Status, err
	}

	locked.Status = repository.PaymentStatusSUCCESS
	locked.TransactionID = uuid.NullUUID{UUID: transactionID, Valid: true}
	locked.PaidAt = sql.NullTime{Time: paidAt, Valid: true}
	updated, err := p.PaymentRepo.Update(ctx, locked, tx)
	if err != nil {
		return nil, verified.Status, fmt.Errorf("mark payment successful: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, verified.Status, err
	}
	p.Logger.Sugar.Infow("payment applied", "reference", updated.Reference, "purpose", updated.Purpose)
	return updated, verified.Status, nil
}

// apply records the confirmed ledger entry and the purpose-specific effect.
func (p *Payment) apply(ctx context.Context, payment *repository.Payment, tx *sqlx.Tx) (uuid.UUID, error) {
	// The Paystack reference doubles as the ledger reference; its unique
	// constraint is a second guard against applying a payment twice.
	transaction, err := p.TransactionRepo.Create(ctx, repository.Transaction{
		MemberID:    payment.MemberID,
		Description: payment.Description,
		Amount:      payment.Amount,
		Reference:   payment.Reference,
		Type:        repository.TransactionTypeDEPOSIT,
		Ledger:      payment.Purpose,
	}, tx)
	if err != nil {
		return uuid.Nil, fmt.Errorf("record transaction: %w", err)
	}
	// Paystack verified it, so no admin approval is needed.
	if _, err := p.TransactionRepo.CreateStatus(ctx, repository.TransactionStatus{
		TransactionID: transaction.ID,
		ConfirmedAt:   sql.NullTime{Time: time.Now(), Valid: true},
	}, tx); err != nil {
		return uuid.Nil, fmt.Errorf("confirm transaction: %w", err)
	}

	switch payment.Purpose {
	case repository.LedgerTypeREGISTRATIONFEE:
		member, err := p.MemberRepo.Get(ctx, repository.MemberRepositoryFilter{ID: &payment.MemberID, ForUpdate: true}, tx)
		if err != nil {
			return uuid.Nil, fmt.Errorf("load member for activation: %w", err)
		}
		if member.ActivatedAt.Valid {
			p.Logger.Sugar.Warnw("membership fee paid again; member already active",
				"reference", payment.Reference, "member_id", member.ID.String())
			break
		}
		member.ActivatedAt = sql.NullTime{Time: time.Now(), Valid: true}
		if _, err := p.MemberRepo.Update(ctx, member, tx); err != nil {
			return uuid.Nil, fmt.Errorf("activate member: %w", err)
		}

	case repository.LedgerTypeSAVINGS, repository.LedgerTypeSPECIALDEPOSIT:
		// The confirmed ledger entry is the credit.

	case repository.LedgerTypeSHARES:
		if !payment.ShareUnitPrice.Valid || !payment.ShareUnits.Valid {
			return uuid.Nil, errors.New("share payment has no quote")
		}
		if _, err := p.ShareRepo.Create(ctx, repository.Share{
			TransactionID: transaction.ID,
			Units:         payment.ShareUnits.String,
			UnitPrice:     payment.ShareUnitPrice.Int64,
		}, tx); err != nil {
			return uuid.Nil, fmt.Errorf("record shares: %w", err)
		}

	case repository.LedgerTypeFINES:
		if !payment.FineID.Valid {
			return uuid.Nil, errors.New("fine payment has no fine")
		}
		fine, err := p.FineRepo.GetPopulated(ctx, repository.FineRepositoryFilter{ID: &payment.FineID.UUID}, tx)
		if err != nil {
			return uuid.Nil, fmt.Errorf("load fine: %w", err)
		}
		if fine.PaidAt.Valid {
			p.Logger.Sugar.Warnw("fine paid again; already marked paid",
				"reference", payment.Reference, "fine_id", fine.ID.String())
			break
		}
		if _, err := p.FineRepo.Update(ctx, &repository.Fine{
			ID:            fine.ID,
			AdminID:       fine.AdminID,
			MemberID:      fine.MemberID,
			TransactionID: uuid.NullUUID{UUID: transaction.ID, Valid: true},
			Amount:        fine.Amount,
			Reason:        fine.Reason,
			Deadline:      fine.Deadline,
			PaidAt:        sql.NullTime{Time: time.Now(), Valid: true},
		}, tx); err != nil {
			return uuid.Nil, fmt.Errorf("mark fine paid: %w", err)
		}

	default:
		return uuid.Nil, fmt.Errorf("unsupported payment purpose %q", payment.Purpose)
	}

	return transaction.ID, nil
}

// plan validates a checkout request against the member's state and computes
// the breakdown with the configured maintenance fee.
func (p *Payment) plan(ctx context.Context, input dto.CreatePaymentInput) (*checkoutPlan, error) {
	member, err := p.currentMember(ctx)
	if err != nil {
		return nil, err
	}

	plan := &checkoutPlan{member: member, purpose: repository.LedgerType(input.Purpose)}
	switch plan.purpose {
	case repository.LedgerTypeREGISTRATIONFEE:
		if member.ActivatedAt.Valid {
			return nil, &svc.APIError{Status: http.StatusConflict, Message: "membership is already active"}
		}
		if input.Amount != 0 && input.Amount != transactions.DefaultRegistrationFee {
			return nil, badRequest("membership fee is NGN %s", formatNaira(transactions.DefaultRegistrationFee))
		}
		plan.amount = transactions.DefaultRegistrationFee
		plan.description = "Membership fee"

	case repository.LedgerTypeSAVINGS:
		if err := requireActive(member); err != nil {
			return nil, err
		}
		if input.Amount < transactions.MinSavingsDepositAmount {
			return nil, badRequest("minimum savings deposit is NGN %s", formatNaira(transactions.MinSavingsDepositAmount))
		}
		plan.amount = input.Amount
		plan.description = "Savings deposit"

	case repository.LedgerTypeSPECIALDEPOSIT:
		if err := requireActive(member); err != nil {
			return nil, err
		}
		if err := p.planCampaign(ctx, plan, input, nil); err != nil {
			return nil, err
		}

	case repository.LedgerTypeSHARES:
		if err := requireActive(member); err != nil {
			return nil, err
		}
		if input.Amount <= 0 {
			return nil, badRequest("amount is required")
		}
		quote, err := p.ShareQuoter.GetShareQuote(ctx, input.Amount)
		if err != nil {
			return nil, err
		}
		if quote.Units <= 0 {
			return nil, badRequest("amount is too small to purchase any shares")
		}
		plan.amount = input.Amount
		plan.shareUnitPrice = sql.NullInt64{Int64: quote.UnitPrice, Valid: true}
		plan.shareUnits = sql.NullString{String: fmt.Sprintf("%.4f", quote.Units), Valid: true}
		plan.description = fmt.Sprintf("Purchase of %.4f shares", quote.Units)

	case repository.LedgerTypeFINES:
		if err := requireActive(member); err != nil {
			return nil, err
		}
		if input.FineID == nil {
			return nil, badRequest("fine_id is required")
		}
		fine, err := p.FineRepo.GetPopulated(ctx, repository.FineRepositoryFilter{ID: input.FineID, MemberID: &member.ID}, nil)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, svc.ErrNotFound()
		}
		if err != nil {
			return nil, fmt.Errorf("load fine: %w", err)
		}
		if fine.PaidAt.Valid {
			return nil, &svc.APIError{Status: http.StatusConflict, Message: "fine already paid"}
		}
		if input.Amount != 0 && input.Amount != fine.Amount {
			return nil, badRequest("payment amount does not match fine amount")
		}
		plan.amount = fine.Amount
		plan.fineID = uuid.NullUUID{UUID: fine.ID, Valid: true}
		plan.description = "Fine payment"

	default:
		return nil, badRequest("unsupported payment purpose")
	}

	if input.FineID != nil && plan.purpose != repository.LedgerTypeFINES {
		return nil, badRequest("fine_id is only used for FINES")
	}
	if input.CampaignID != nil && plan.purpose != repository.LedgerTypeSPECIALDEPOSIT {
		return nil, badRequest("campaign_id is only used for SPECIAL_DEPOSIT")
	}
	if description := strings.TrimSpace(input.Description); description != "" {
		plan.description = description
	}

	p.quotePlan(plan)
	return plan, nil
}

func (p *Payment) quotePlan(plan *checkoutPlan) {
	fee := MaintenanceFee(plan.amount, p.Config.Payments.MaintenanceFeePercent)
	charge := ProcessingCharge(plan.amount + fee)
	plan.quote = dto.PaymentQuote{
		Purpose:               dto.LedgerType(plan.purpose),
		Currency:              currencyNGN,
		Amount:                plan.amount,
		MaintenanceFee:        fee,
		MaintenanceFeePercent: p.Config.Payments.MaintenanceFeePercent,
		ProcessingCharge:      charge,
		Total:                 plan.amount + fee + charge,
	}
	if plan.campaignID.Valid {
		plan.quote.CampaignID = &plan.campaignID.UUID
	}
}

func (p *Payment) currentMember(ctx context.Context) (*repository.Member, error) {
	actor, ok := users.FromContext(ctx)
	if !ok || actor.ID == uuid.Nil {
		return nil, svc.UnauthenticatedError()
	}
	if !actor.IsAuthenticatedAsMember {
		return nil, &svc.APIError{Status: http.StatusForbidden, Message: "member access required"}
	}
	member, err := p.MemberRepo.Get(ctx, repository.MemberRepositoryFilter{UserID: &actor.ID}, nil)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, svc.ErrNotFound()
	}
	if err != nil {
		return nil, fmt.Errorf("load member: %w", err)
	}
	return member, nil
}

func (p *Payment) callbackURL(reference string, cancelled bool) string {
	callback := p.Config.Paystack.CallbackURL
	if !cancelled {
		return callback
	}
	separator := "?"
	if strings.Contains(callback, "?") {
		separator = "&"
	}
	return callback + separator + "reference=" + reference + "&cancelled=true"
}

func requireActive(member *repository.Member) error {
	if !member.ActivatedAt.Valid {
		return &svc.APIError{Status: http.StatusForbidden, Message: "active membership required"}
	}
	return nil
}

func badRequest(format string, args ...any) error {
	return &svc.APIError{Status: http.StatusBadRequest, Message: fmt.Sprintf(format, args...)}
}

var referencePrefixes = map[repository.LedgerType]string{
	repository.LedgerTypeREGISTRATIONFEE: "reg",
	repository.LedgerTypeSAVINGS:         "sav",
	repository.LedgerTypeSPECIALDEPOSIT:  "camp",
	repository.LedgerTypeSHARES:          "shr",
	repository.LedgerTypeFINES:           "fine",
}

// newReference makes a unique, URL-safe Paystack reference such as
// ara_sav_4f9c0a1b2c3d4e5f6a7b.
func newReference(purpose repository.LedgerType) (string, error) {
	random := make([]byte, 10)
	if _, err := rand.Read(random); err != nil {
		return "", fmt.Errorf("generate payment reference: %w", err)
	}
	return fmt.Sprintf("ara_%s_%s", lo.ValueOr(referencePrefixes, purpose, "pay"), hex.EncodeToString(random)), nil
}

func formatNaira(kobo int64) string {
	if kobo%100 == 0 {
		return fmt.Sprintf("%d", kobo/100)
	}
	return fmt.Sprintf("%d.%02d", kobo/100, kobo%100)
}
