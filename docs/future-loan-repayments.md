# Future Loan Repayments

## Status

The entire loan-repayment feature is deferred. This document preserves earlier decisions for future planning; it is not implementation scope for the current version.

The current version must not introduce Paystack loan-repayment collection, repayment schedules, repayment-balance calculations, overdue repayment reminders, or automatic collection.

## Decisions to Revisit When Work Resumes

- Members initiate repayments through Paystack themselves.
- An admin chooses weekly or monthly instalments and the first due date; the system generates the schedule.
- Partial and extra repayments are allowed up to the amount owed, with remaining amounts tracked.
- The maintenance fee applies to the full repayment amount, including principal and interest. Paystack processing charges are added separately.
- Flat-rate loans retain their full agreed interest even when settled early.
- Overdue instalments are flagged and trigger reminders. Admins issue any fines manually; automatic late-payment penalties are not required.

## Unresolved Calculations

- Allocation of partial and extra payments between interest, principal, and scheduled instalments.
- Reducing-balance accrual periods, early settlement, and treatment of extra payments.
- Rounding, schedule adjustments, and reminder timing.
- How interest behaves after the original loan term.

These questions remain open. No answer should be inferred from the earlier proposed repayment-allocation options.
