# Membership and Payment Journey

This document records the agreed product direction. Implementation progress and frontend contracts are tracked in `../frontend.md`. Incoming payments go through Paystack checkout (Step 4 in `../frontend.md`); the older manual deposit endpoints remain in the API but the member app no longer uses them. Use `CONTEXT.md` for terminology.

## Current Version

Include invitation-based membership, account recovery, Paystack payments, personal savings, campaign contributions, share purchases, fines, loan applications, admin decisions, and records of external loan disbursements.

Exclude the entire loan-repayment feature, dividends, and member savings withdrawals. Existing accounts and financial records are test/demo data; a real-member migration is not required. Starting fresh does not authorize deleting data automatically.

## Roles and Access

- **Owner:** manages both the platform and cooperative, controls admin access and fee settings, and receives platform maintenance-fee income separately from cooperative funds.
- **Admin:** invites and manages members, manages campaigns, assigns fines, reviews loans, and records external disbursements and campaign refund labels.
- **Member:** maintains a personal profile and uses financial features after membership activation.

A user can be both an admin and a member. Admin access alone is free. An admin participating as a member needs a member profile and pays the same membership fee.

Initial owner email: `jtirenipraise@gmail.com`.

Implemented setup: `make owner/bootstrap` uses `OWNER_EMAIL` and `DB_URL` to create or reuse the owner account and persist its role. An optional `OWNER_PASSWORD` initializes a new or pending account's password; otherwise a pending account receives a one-time password-setup token in the operator's terminal. Existing passwords and memberships are preserved, and a different email cannot replace an existing owner.

Runtime authorization reloads current roles from storage rather than trusting email strings or old role claims. Only the owner grants or revokes admin access through the admin-management endpoints. Owner capabilities include admin access, while personal membership remains separate. See `../frontend.md` for the HTTP contract.

## Paper Forms Are the Source of Truth

The cooperative's paper membership application form, loan application form and bye-laws define what the app collects. The member profile stores every field on the membership form (title, names, contacts, place of birth, nationality, state of origin, home town, LGA, next of kin with address, current workplace, up to four beneficiaries, monthly contribution pledge, referrer, declaration and passport photo). Loan applications store every applicant field on the loan form (amount, purpose, duration, proposed repayment start/duration/monthly amount, disbursement bank details, two guarantors and the authorization declaration). Name, contact and membership figures already on record are not re-entered.

- The monthly contribution pledge is information only; savings remain voluntary.
- Passport photos are stored in Cloudinary as `ara/<ENV>/members/<member id>/passport`. The API names and signs the upload, the browser sends the file straight to Cloudinary, and the API verifies Cloudinary's response signature before saving. The database keeps only the public ID and version; the frontend builds the URL and shows a blurred placeholder first. The membership fee requires the saved form and declaration but not the photo.
- Guarantors must be members and accept in the app in place of signing. They may change their answer until the application is reviewed.
- The referrer ("introduced by") and loan guarantors are members picked with a member search (pending members are marked) by name or membership number; the API stores the referrer's name from their record.
- The loan payout account is chosen from Paystack's bank list and confirmed with Paystack's account resolve; the bank's account holder name is stored, not a typed one.
- Bye-law loan rules are advisory flags on admin review, overridable with a recorded reason: no loan within 6 months of activation (65), at least 50 shares (46), repayment within 12 months (71), two accepting, active guarantors (68) who are not on loan or indebted (loan form). Interest above 15% per annum (67) is rejected outright.
- Not yet modelled, kept as open decisions: committee acceptance or rejection of membership applications (bye-law 5, the form's "For official use" section) and two-step loan approval by the Loan Committee Chairman and the ARA President. One admin approval remains sufficient.

## Joining and Login

1. An admin invites a member using their name and email.
2. The invited member sets a password and signs in with email and password.
3. The member completes the membership application form (see above), accepts its declaration, and uploads a passport photo.
4. The application displays the NGN 1,000 one-time membership fee, maintenance fee, Paystack charge, and total payable.
5. The member pays through Paystack. The application shows processing until the backend verifies success.
6. Verified payment records the membership fee and activates membership together.
7. The member receives a receipt and access to financial features.

Before activation, members can complete their profiles and pay the joining fee. They cannot use financial features. Invited users can request a fresh invitation if it expires or fails to arrive; uninvited addresses cannot join through resending. Password reset is available by email.

## Payments and Fees

Paystack verifies incoming payments for membership fees, savings, campaigns, shares, and fines. Admins do not manually approve these payments.

Members pay the underlying amount, an additional maintenance fee, and Paystack processing charges. The maintenance fee defaults to 5% and is set with `MAINTENANCE_FEE_PERCENT`, a whole percent from 0 to 100. All amounts are stored and exchanged in kobo.

Example for a NGN 10,000 savings contribution:

| Component | Amount |
|---|---:|
| Savings contribution | NGN 10,000 |
| Maintenance fee at 5% | NGN 500 |
| Total payable | NGN 10,500 plus Paystack charge |
| Member savings credit | NGN 10,000 |

Use Paystack split settlement for separate cooperative and platform bank accounts. The cooperative receives the underlying contribution; the platform owner receives maintenance-fee income. The cooperative owns the main Paystack business and its payout account. Checkout names the owner subaccount (`PAYSTACK_OWNER_SUBACCOUNT`) with a `transaction_charge` equal to the principal, so the subaccount receives the maintenance fee plus processing charge (a dynamic flat split would reject owner shares under NGN 100). Paystack deducts its fee from that subaccount's allocation; the owner keeps the maintenance fee, and the cooperative keeps the full underlying contribution in the main account. Existing payment records retain the split agreed at checkout.

Each payment must retain its member, purpose, unique reference, currency, expected amounts, and applicable fee rate. Later configuration changes must not alter earlier records. Verify payment on the server through authenticated webhooks and transaction verification. A browser redirect alone is insufficient. Apply a verified payment's financial effect only once, even if notifications repeat.

## Member Financial Features

| Feature | Agreed behavior |
|---|---|
| Savings | Voluntary contributions whenever the member chooses, with a NGN 100 minimum before charges. No required monthly contribution. |
| Campaigns / special deposits | Contributions become cooperative funds for a named event or project, tracked separately from personal savings. Admins choose voluntary or required participation and fixed or flexible amounts per campaign. |
| Shares | Start at NGN 500 per unit. Fractional units are allowed: NGN 750 buys 1.5 units before charges. Operators configure future prices through `SHARE_UNIT_PRICE` in kobo and restart/redeploy the API; previous purchase prices and units are preserved. |
| Fines | An admin assigns a member, amount, reason, and due date. Members pay the full fine through Paystack; partial fine payments are excluded. Verified payment marks the fine paid. |

Proposed display label: "campaign contribution" instead of "special deposit"; the final label is not yet agreed. Dividend calculation and distribution are deferred.

Campaign clarification: required campaigns apply to all active members. Both fixed and flexible campaigns allow repeated contributions; each fixed contribution must equal the campaign's fixed amount. Campaign creation/editing, dates, closing/cancellation, admin resolution notes, Paystack association, verified totals, and scoped contribution history are implemented in Step 5 of `../frontend.md`. Required participation is displayed without automatic penalties. Existing checkout terms remain fixed after campaign edits. Closed/cancelled campaigns reject new contributions, and existing payments remain in history.

## Loan Applications and External Disbursement

A member completes the loan application form: amount, purpose, duration, proposed repayment plan, disbursement bank account, two guarantors, and the declaration. An admin sets the final approved terms. Record the selected flat-rate or reducing-balance interest method, rate, and monthly or annual rate period explicitly. Repayment calculations and schedules remain deferred.

Admins configure minimum savings and shareholding thresholds for advisory checks. Show savings, shares, and available loan application/disbursement history. An admin may approve despite an advisory flag by recording a reason. Passing checks must never automatically approve a loan.

Nobody may approve their own loan application. Another admin or the owner approves an admin's application; another admin must approve the owner's own application.

Admin approval is sufficient to proceed; separate member acceptance is not required. The cooperative sends funds outside Paystack. An admin records the amount sent, payment date, and transfer reference; this activates the loan. Approval alone does not prove disbursement.

Do not show calculated repayment progress, remaining repayment balances, or repayment-overdue assessments in this version. No repayment collection or schedule is included.

Implemented loan states are `PENDING`, `APPROVED`, `REJECTED`, and `ACTIVE`. Admins configure advisory minimums (initially zero), review current figures/history, and must record an override reason when approving despite flags. Review snapshots are retained. External disbursement records require the full approved amount, a transfer timestamp after approval, and a unique reference; they create one confirmed loan-disbursement ledger entry atomically. Active-loan flags identify recorded disbursements, not verified unpaid balances. See Step 6 in `../frontend.md` for the implemented contracts.

## Cancellations and Refunds

An admin decides how to resolve a cancelled campaign. Cancellation does not automatically refund or credit contributions.

Refund processing and detailed refund records are deferred. For now an admin sets a campaign-wide `is_refunded` flag after closure/cancellation to report that refunds were handled outside the app; they can correct the flag later. The flag does not change payments, contribution totals, balances, or fees. It does not verify a bank refund. Any actual approved refund remains an external cooperative action; the agreed principal-only refund policy retains the platform maintenance fee.

## Deferred Work and Open Decisions

- Loan repayments: earlier decisions and unresolved calculations are preserved in `future-loan-repayments.md`; the entire feature is deferred.
- Dividends and member savings withdrawals are outside this version.
- Confirm any maintenance-fee exemptions before introducing them.
- Committee acceptance of membership applications and two-step loan approval (Loan Committee Chairman, then ARA President) from the paper forms.
- Define non-refund campaign resolutions and which payments qualify for refunds.
- Define handling of duplicate successful payments and the refund treatment of Paystack processing charges.

## Paystack References

[Accept payments](https://paystack.com/docs/payments/accept-payments/), [Verify payments](https://paystack.com/docs/payments/verify-payments/), [Webhooks](https://paystack.com/docs/payments/webhooks/), [Split payments](https://paystack.com/docs/payments/split-payments/).
