package loans

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/Jidetireni/ara-cooperative/internal/repository"
	"github.com/Jidetireni/ara-cooperative/pkg/email"
	"github.com/google/uuid"
)

type EmailSender interface {
	Send(ctx context.Context, input *email.SendEmailInput) error
}

// requestGuarantees emails each guarantor a link to accept or decline the
// application. Delivery is best effort: the application is already saved and
// guarantors also see it under their guarantor requests.
func (l *Loan) requestGuarantees(ctx context.Context, loanID uuid.UUID) {
	if l.Email == nil {
		return
	}
	guarantors, err := l.LoanRepo.ListGuarantors(ctx, repository.LoanGuarantorRepositoryFilter{LoanID: &loanID}, nil)
	if err != nil {
		l.logDeliveryFailure("load guarantors", err, loanID, uuid.Nil)
		return
	}
	link, err := guaranteeLink(l.FEURL, loanID)
	if err != nil {
		l.logDeliveryFailure("build guarantee link", err, loanID, uuid.Nil)
		return
	}
	for _, g := range guarantors {
		if g.MemberEmail == "" {
			continue
		}
		message, err := email.Render(email.EmailTemplateTypeLoanGuarantorRequest, email.LoanGuarantorRequestData{
			FirstName: g.Member.FirstName, ApplicantName: fullName(g.Applicant),
			ApplicantNumber: strings.ToUpper(g.Applicant.Slug), Amount: formatNaira(g.Loan.RequestedAmount),
			Purpose: g.Loan.Purpose, DurationMonths: g.Loan.RequestedDurationMonths, ActionURL: link,
		})
		if err == nil {
			err = l.Email.Send(ctx, &email.SendEmailInput{
				To:      g.MemberEmail,
				Subject: fullName(g.Applicant) + " wants you to guarantee a loan",
				Body:    message.HTML, Text: message.Text,
			})
		}
		if err != nil {
			l.logDeliveryFailure("send guarantor request", err, loanID, g.MemberID)
		}
	}
}

func fullName(m repository.Member) string {
	return strings.Join(strings.Fields(m.FirstName+" "+m.MiddleName.String+" "+m.LastName), " ")
}

func (l *Loan) logDeliveryFailure(step string, err error, loanID, memberID uuid.UUID) {
	if l.Logger == nil {
		return
	}
	l.Logger.Sugar.Errorw("guarantor request email failed", "step", step, "error", err,
		"loan_id", loanID.String(), "guarantor_member_id", memberID.String())
}

// guaranteeLink is the member app page where a guarantor answers.
func guaranteeLink(feURL string, loanID uuid.UUID) (string, error) {
	base, err := url.Parse(strings.TrimSpace(feURL))
	if err != nil || base.Scheme == "" || base.Host == "" {
		return "", fmt.Errorf("a valid FE_URL is required for guarantor links")
	}
	base.RawQuery, base.Fragment = "", ""
	return base.JoinPath("guarantees", loanID.String()).String(), nil
}

// formatNaira renders an amount in kobo as ₦1,234.50.
func formatNaira(kobo int64) string {
	sign := ""
	if kobo < 0 {
		sign, kobo = "-", -kobo
	}
	whole := fmt.Sprint(kobo / 100)
	var grouped strings.Builder
	for i, digit := range whole {
		if i > 0 && (len(whole)-i)%3 == 0 {
			grouped.WriteByte(',')
		}
		grouped.WriteRune(digit)
	}
	return fmt.Sprintf("%s₦%s.%02d", sign, grouped.String(), kobo%100)
}
