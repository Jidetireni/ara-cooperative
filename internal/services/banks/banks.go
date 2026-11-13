// Package banks lists Nigerian banks and resolves account holders through
// Paystack, so loan payouts go to accounts the bank confirms.
package banks

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Jidetireni/ara-cooperative/internal/dto"
	svc "github.com/Jidetireni/ara-cooperative/internal/services"
	"github.com/Jidetireni/ara-cooperative/internal/services/users"
	"github.com/Jidetireni/ara-cooperative/pkg/logger"
	"github.com/Jidetireni/ara-cooperative/pkg/paystack"
	"github.com/google/uuid"
)

// cacheTTL keeps Paystack's bank list for a day; it rarely changes.
const cacheTTL = 24 * time.Hour

var accountNumberPattern = regexp.MustCompile(`^[0-9]{10}$`)

// Directory is the part of the Paystack client this service uses.
type Directory interface {
	Enabled() bool
	ListBanks(ctx context.Context) ([]paystack.Bank, error)
	ResolveAccount(ctx context.Context, accountNumber, bankCode string) (*paystack.ResolvedAccount, error)
}

var _ Directory = (*paystack.Client)(nil)

type Banks struct {
	Directory Directory
	Logger    *logger.Logger

	mu       sync.Mutex
	cached   []dto.Bank
	cachedAt time.Time
	now      func() time.Time
}

func New(directory Directory, log *logger.Logger) *Banks {
	return &Banks{Directory: directory, Logger: log, now: time.Now}
}

func apiError(status int, message string) error {
	return &svc.APIError{Status: status, Message: message}
}

func signedIn(ctx context.Context) error {
	actor, ok := users.FromContext(ctx)
	if !ok || actor.ID == uuid.Nil {
		return svc.UnauthenticatedError()
	}
	return nil
}

func (b *Banks) ready() error {
	if b.Directory == nil || !b.Directory.Enabled() {
		return apiError(http.StatusServiceUnavailable, "bank lookups are not configured")
	}
	return nil
}

// List returns active Nigerian banks sorted by name.
func (b *Banks) List(ctx context.Context) ([]dto.Bank, error) {
	if err := signedIn(ctx); err != nil {
		return nil, err
	}
	return b.list(ctx)
}

func (b *Banks) list(ctx context.Context) ([]dto.Bank, error) {
	if err := b.ready(); err != nil {
		return nil, err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.cached != nil && b.now().Sub(b.cachedAt) < cacheTTL {
		return b.cached, nil
	}
	fetched, err := b.Directory.ListBanks(ctx)
	if err != nil {
		b.Logger.Sugar.Errorw("list paystack banks", "error", err)
		if b.cached != nil {
			return b.cached, nil
		}
		return nil, apiError(http.StatusBadGateway, "the bank list is unavailable; try again shortly")
	}
	seen := map[string]bool{}
	banks := make([]dto.Bank, 0, len(fetched))
	for _, bank := range fetched {
		if !bank.Active || bank.Code == "" || seen[bank.Code] {
			continue
		}
		seen[bank.Code] = true
		banks = append(banks, dto.Bank{Name: strings.TrimSpace(bank.Name), Code: bank.Code})
	}
	sort.Slice(banks, func(i, j int) bool { return strings.ToLower(banks[i].Name) < strings.ToLower(banks[j].Name) })
	b.cached, b.cachedAt = banks, b.now()
	return banks, nil
}

// Resolve asks the bank who owns accountNumber. The returned name is what
// the bank reports, not what the member typed.
func (b *Banks) Resolve(ctx context.Context, bankCode, accountNumber string) (*dto.BankAccount, error) {
	if err := signedIn(ctx); err != nil {
		return nil, err
	}
	bankCode, accountNumber = strings.TrimSpace(bankCode), strings.TrimSpace(accountNumber)
	if bankCode == "" || !accountNumberPattern.MatchString(accountNumber) {
		return nil, apiError(http.StatusBadRequest, "choose a bank and enter a 10-digit account number")
	}
	banks, err := b.list(ctx)
	if err != nil {
		return nil, err
	}
	var bank *dto.Bank
	for i := range banks {
		if banks[i].Code == bankCode {
			bank = &banks[i]
			break
		}
	}
	if bank == nil {
		return nil, apiError(http.StatusBadRequest, "unknown bank")
	}
	resolved, err := b.Directory.ResolveAccount(ctx, accountNumber, bankCode)
	if err != nil {
		var paystackErr *paystack.APIError
		if errors.As(err, &paystackErr) && paystackErr.HTTPStatus < http.StatusInternalServerError {
			return nil, apiError(http.StatusBadRequest, "the bank could not confirm this account number")
		}
		b.Logger.Sugar.Errorw("resolve bank account", "bank_code", bankCode, "error", err)
		return nil, apiError(http.StatusBadGateway, "the account could not be checked; try again shortly")
	}
	return &dto.BankAccount{
		BankCode: bankCode, BankName: bank.Name, AccountNumber: accountNumber,
		AccountName: strings.TrimSpace(resolved.AccountName),
	}, nil
}
