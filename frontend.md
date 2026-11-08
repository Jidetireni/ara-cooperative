# Frontend Integration Guide

This file tracks the frontend contract as backend work lands. Use `/api/v1` as the base path. Product decisions live in [docs/member-journey.md](docs/member-journey.md).

## Implementation Tracker

| Step | Status | Frontend impact |
|---|---|---|
| 1. Account/session context | Implemented and verified | Login, refresh, and current-user lookup expose roles and optional membership. |
| 2. Owner bootstrap and admin permissions | Implemented and verified | Stored owner role, owner-only admin grants/revocations, and live authorization checks. |
| 3. Invitation and profile onboarding | Invitations and profile completion implemented | Name/email-only invitations, password setup, admin retries, pending-member access checks, member profile read/update (`GET`/`PUT /members/me`), and the admin member list (`GET /members`) are ready. Self-service invitation resend and password reset remain planned. |
| 4. Paystack checkout and split settlement | Implemented and tested | Quote, checkout (`POST /payments` returns `authorization_url`), status reads, signed webhook verification, and flat split to the owner subaccount. Amounts are kobo. |
| 5. Campaigns | Implemented | Creation/editing, dates, participation and amount rules, Paystack contributions, totals/history, closing/cancellation, resolution notes, and a manual refunded flag. |
| 6. Loans | Implemented | Applications, admin history/review, configurable advisory thresholds, approval/rejection, and recording external disbursements. Repayments remain deferred. |

Dividends, savings withdrawals, and the entire loan-repayment feature are outside this version.

## Connection and Response Format

Use a same-origin reverse proxy for the frontend and API. The API currently has no browser CORS configuration. A frontend dev-server proxy can forward `/api/v1` to the configured API port.

Successful responses use:

```json
{ "data": {}, "status": 200 }
```

Errors use:

```json
{ "status": 401, "message": "invalid email or password" }
```

Validation errors may include `errors`, an array of `{ "field", "message" }`. Field names currently use Go names such as `Email` and `Password`; map them to form fields. Treat HTTP status as authoritative and show messages as text.

### List pagination

Lists return `{ "items": [], "next_cursor": "..." }` inside `data`. `next_cursor` is `null` on the last page. Query parameters shared by list endpoints:

| Parameter | Meaning |
|---|---|
| `limit` | Page size, default 20, clamped to 100. |
| `cursor` | The previous page's `next_cursor`, sent unchanged with the same filters. |
| `sort` | Optional: `created_at:desc` (default, newest first) or `created_at:asc`. No other column or format is accepted. The member list ignores `sort` and is always newest first. |

Any other `sort` value, or a malformed `cursor`, returns 400 (`invalid sort; use created_at:asc or created_at:desc` or `invalid cursor`). Keep the same `sort` for every page of one listing.

## Step 1: Login and Session Context

### Sign in

`POST /api/v1/login`

```json
{ "email": "member@example.com", "password": "their-password" }
```

Send JSON with `Content-Type: application/json` and `credentials: "include"`.

Example response for a member awaiting activation:

```json
{
  "data": {
    "user": {
      "id": "11111111-1111-4111-8111-111111111111",
      "email": "member@example.com",
      "roles": ["member"],
      "member": {
        "id": "22222222-2222-4222-8222-222222222222",
        "first_name": "Ada",
        "last_name": "Member",
        "slug": "ara000001",
        "phone": "08000000000",
        "address": "Example address",
        "next_of_kin_name": "Example relative",
        "next_of_kin_phone": "08000000001",
        "is_active": false
      },
      "confirmed_at": "2026-11-07T10:00:00Z"
    },
    "access_token": "<access-token>"
  },
  "status": 200
}
```

- `roles` is an array, including `[]` when there are no assigned roles.
- `member` is omitted when the account has no member profile. This is valid for an admin account.
- `member.is_active` reports the current activation state.
- `confirmed_at` is omitted when email confirmation is absent.
- Password hashes and refresh tokens are not included in the response body.

The browser receives a `refresh_token` HttpOnly cookie. Production cookies are Secure and SameSite=Strict; use HTTPS. Development cookies are not Secure and use SameSite=Lax. JavaScript must not read the refresh cookie or send a refresh token as a bearer token.

Keep the access token in application memory and attach it to protected requests:

```js
const response = await fetch('/api/v1/me', {
  headers: { Authorization: `Bearer ${accessToken}` },
  credentials: 'include',
});
```

### Restore a session

`POST /api/v1/refresh`, with `credentials: "include"` and no JSON body.

The browser supplies the cookie automatically. Success rotates the cookie and returns the same `{ user, access_token }` shape as login. Replace the in-memory access token and account context with the response.

**Contract change:** refresh previously returned only an access-token string inside `data`. It now returns an object, matching login.

On application startup, call refresh to restore the session. If a protected request returns 401, perform one shared refresh request and retry the original request once. Concurrent requests should await the same refresh operation.

A refresh 401 means the session cannot be restored: clear local account state and show login. Network failures and 5xx responses should show a retry state rather than report incorrect credentials.

The current database stores one active refresh token per user. A new login replaces that user's previous refresh token. Access tokens expire after 15 minutes in production and one day in development.

### Reload the current account

`GET /api/v1/me`, authenticated with the access token. Its `data` is the `user` object above, without an access token. It reloads account roles and membership from storage, including activation changes made after login. Missing or deleted accounts return 401.

Use this endpoint when refreshing account information without rotating the session. Navigation can use the returned roles; server-side authorization remains authoritative. Protected requests now reload current roles and permissions from storage, so revoked admin access stops working even with an older access token.

Old access tokens without a token type are rejected after this change. Restore through refresh or sign in again. Refresh tokens cannot authenticate protected routes.

### Existing password-setup endpoint

`POST /api/v1/set-password` accepts `{ "email", "password", "token" }` and returns the same session data with HTTP 201. `token` is the secret from the emailed setup link or owner setup code. The password requires at least eight characters and must fit bcrypt's 72-byte limit.

Treat setup tokens as opaque text. New owner and member tokens contain 64 hexadecimal characters. Setup tokens expire after 30 minutes, work only once, and cannot replace an established password.

Member invitations use this existing endpoint; see Step 3 for the link format. Password reset remains a separate, unimplemented journey.

## Navigation and Current Limits

| Account context | Intended destination |
|---|---|
| `roles` includes `owner` or `admin` | Admin area; membership is optional. |
| `roles` includes `owner` | Also show admin-access management controls. |
| Member role with `member.is_active === true` | Member financial area. |
| Member role with an inactive profile | Profile/onboarding and membership-payment journey. |
| Both admin and member roles | Support both management and personal-member views. |
| No suitable role or missing required profile | Account-support state; do not assume membership or activation. |

Profile completion is available through `GET`/`PUT /api/v1/members/me`, and the membership fee is paid through Step 4. Financial member routes require an active membership; `/me`, login, and refresh remain available to pending members.

Server logout, password recovery, and self-service invitation resend are not implemented yet. Admins can retry an invitation through the existing member-creation endpoint. A local sign-out alone does not revoke the refresh cookie.

The requested initial owner login is `jtirenipraise@gmail.com`. Use returned roles for navigation rather than grant privileges by comparing the email address. The owner login address is independent of the application's email-sending address.

## Step 2: Owner Bootstrap and Admin Access

### Initial setup (operator command)

For a fresh database, apply the initial schema migrations first. The roles migration seeds `owner`, `admin`, and `member`; no separate owner migration is needed. The local development database was migrated to version 10 on 8 November 2026. Use forward migrations for databases already initialized.

Configure `OWNER_EMAIL=jtirenipraise@gmail.com` and a reachable PostgreSQL `DB_URL` in the ignored `.env`. Optionally set `OWNER_PASSWORD` to initialize a new or pending owner's password directly, then run:

```sh
make owner/bootstrap
```

This runs `go run ./cmd/owner`. It needs PostgreSQL, `OWNER_EMAIL`, and `DB_URL`, without email credentials or a running API. It never runs the development seeder or resets tables.

With `OWNER_PASSWORD`, bootstrap hashes the password, initializes a new or pending owner, and invalidates earlier password-setup codes. The password must contain at least eight characters and fit bcrypt's 72-byte limit. The owner can then use the normal login form. `OWNER_PASSWORD` is a backend operator setting; it is not frontend configuration or an API response field.

Without `OWNER_PASSWORD`, the command prints a strong one-time setup token valid for 30 minutes. The owner submits that token, email, and a chosen password through the existing `POST /api/v1/set-password` form. Bootstrap does not send email.

Re-running bootstrap preserves an existing password and membership, even if `OWNER_PASSWORD` changes. For a pending account without an environment password, it generates a fresh setup token, replacing the earlier code. A different configured email cannot replace an existing owner. Ownership is persisted in the database; changing `OWNER_EMAIL` does not transfer ownership.

The owner signs in through the normal login endpoint. Login, refresh, and `/me` return the stored `owner` role. Ownership grants admin capabilities without automatically granting a personal membership or a `member` role.

### Grant admin access

`PUT /api/v1/admins`, with the owner's bearer token:

```json
{ "email": "existing-user@example.com" }
```

The target must already have a user account. This endpoint does not create an account or send an invitation; staff invitations remain future work.

Example response:

```json
{
  "data": {
    "id": "11111111-1111-4111-8111-111111111111",
    "email": "existing-user@example.com",
    "roles": ["admin", "member"]
  },
  "status": 200
}
```

The member role appears only if the target already has it. Existing personal membership and savings are preserved. Repeating the request is safe.

### Revoke admin access

`DELETE /api/v1/admins/{user_id}`, with the owner's bearer token and no body. Use the user `id` returned by the grant response. The member profile has a separate ID.

Success returns the same account shape, with `admin` removed from `roles`. Remaining roles and personal membership are preserved. Repeating revocation is safe. The user's next protected request uses the current stored roles, even if their access token still contains the old admin role. Reload `/me` to update navigation.

### Access-management errors

| HTTP status | Meaning |
|---|---|
| 400 | Invalid email or user UUID. |
| 401 | Missing/invalid access token or deleted account. |
| 403 | Caller is not the stored owner. Ordinary admins cannot grant or revoke admin access. |
| 404 | Target user does not exist or is deleted. |
| 409 | Target is the owner; admin-management endpoints cannot change owner access. |

Owner creation and transfer are not public HTTP operations. Ordinary admin permissions and token/email claims cannot grant ownership.

### Email delivery setup

No sending domain is available yet, and Resend is not integrated. Development currently prints email contents and invitation links to the backend terminal instead of delivering them. This supports local development, not real-member email delivery. Owner bootstrap does not require a sending domain.

Resend remains a proposed provider. Production delivery to members requires a verified domain controlled by the cooperative/platform. Its `resend.dev` sender is restricted to testing; see [Resend's testing restriction](https://resend.com/docs/knowledge-base/403-error-resend-dev-domain).

## Step 3: Member Invitations

### Admin form and response

Use the existing `POST /api/v1/members/` endpoint with the owner/admin bearer token. Collect name and email only:

```json
{
  "first_name": "Ada",
  "last_name": "Member",
  "email": "ada@example.com"
}
```

`first_name` and `email` are required; `last_name` is optional for single-name members. Phone, address, and next-of-kin fields are no longer accepted here. The member will supply them in the next onboarding step. Email identity is case-insensitive.

Success returns HTTP 201:

```json
{
  "data": {
    "email": "ada@example.com",
    "member": {
      "id": "11111111-1111-4111-8111-111111111111",
      "first_name": "Ada",
      "last_name": "Member",
      "slug": "ara000001",
      "phone": "",
      "address": "",
      "next_of_kin_name": "",
      "next_of_kin_phone": "",
      "is_active": false
    },
    "requires_password_setup": true,
    "expires_at": "2026-11-08T12:30:00Z",
    "delivery": "sent"
  },
  "status": 201
}
```

The response contains no setup token. `delivery: "sent"` means the email transport accepted the message; in development it means the message was printed in API logs. If `delivery` is `"failed"`, the membership still exists: show a delivery warning and let the admin retry the same request. Retrying a pending invitation preserves its member ID and profile, renews the setup link, and invalidates the old link. An already active membership returns 409. Concurrent requests creating the same new account may also return 409; retry that invitation.

Existing accounts keep their password and current roles, gaining a separate pending member profile. Their email links to login, `requires_password_setup` is false, and `expires_at` is omitted. Ordinary members receive 403 when inviting; missing/invalid authentication receives 401. Invalid input receives 400.

### Member accepts the invitation

The email links to `${FE_URL}/set-password#email=...&token=...`. Read `email` and `token` with `new URLSearchParams(window.location.hash.slice(1))`, keep them in component memory, then remove the fragment with `history.replaceState`. Keep setup credentials out of analytics and application logs.

Submit the email, token, and chosen password to the existing `POST /api/v1/set-password`. HTTP 201 returns session data and a refresh cookie. Show a pending-onboarding screen: password setup confirms the email and leaves `member.is_active` false. Invalid, expired, replaced, and consumed tokens return 400. The frontend form itself is not included in this backend repository.

`GET /api/v1/me`, login, and refresh work before activation. Existing member-only financial routes return 403 with `active membership required` until activation. Profile completion is described below. Paying the membership fee through Step 4 activates the member; self-service resend and password reset are still planned.

### Member completes their profile

A member reads and saves their own profile. Both endpoints need a bearer token and the `member` role, and both work before activation. Saving a profile never activates membership or unlocks financial routes.

`GET /api/v1/members/me` returns the member object (HTTP 200). An invited profile has empty `phone`, `address`, `next_of_kin_name`, and `next_of_kin_phone` until the member saves it. Treat the profile as complete when all four are non-empty.

`PUT /api/v1/members/me` replaces all four contact fields. Every field is required:

```json
{
  "phone": "08012345678",
  "address": "12 Example Road, Lagos",
  "next_of_kin_name": "Tola Member",
  "next_of_kin_phone": "+2348098765432"
}
```

The server trims surrounding whitespace. Phone numbers need 7 to 15 digits with an optional leading `+`; spaces, dashes, and brackets are rejected. HTTP 200 returns the saved member object in the usual envelope. Name, slug, ID, and `is_active` are unchanged; this endpoint cannot edit the name. The member can save again later to edit a completed profile. After saving, `GET /api/v1/me` returns the updated `member`.

| Status | Meaning |
|---|---|
| 400 | Missing, blank, too long, or invalid fields. Validation failures list `errors[]` with Go field names (`Phone`, `Address`, `NextOfKinName`, `NextOfKinPhone`); format failures have only a message. |
| 401 | Missing or invalid access token. |
| 403 | The account has no member role, e.g. an admin or owner without personal membership. |
| 404 | The member role has no member profile, or the profile was deleted. |
| 409 | `phone number is already used by another member`. |

### Admin lists members

`GET /api/v1/members` returns members newest first, for admins and the owner (bearer token required). Each item is the member object plus the account fields an admin needs:

```json
{
  "data": {
    "items": [
      {
        "id": "11111111-1111-4111-8111-111111111111",
        "first_name": "Ada",
        "last_name": "Member",
        "slug": "ara000001",
        "phone": "",
        "address": "",
        "next_of_kin_name": "",
        "next_of_kin_phone": "",
        "is_active": false,
        "email": "ada@example.com",
        "email_confirmed": false,
        "created_at": "2026-11-08T12:00:00Z"
      }
    ],
    "next_cursor": null
  },
  "status": 200
}
```

`email_confirmed` is false until the invited person sets their password. Derive a display status: `is_active` → active; `!email_confirmed` → invited (offer to resend with `POST /members`); otherwise → awaiting the membership fee. Profile completeness follows the same rule as `GET /members/me`. Soft-deleted members are excluded, and the response never contains password data.

Query parameters, all optional:

| Parameter | Meaning |
|---|---|
| `search` | Case-insensitive match on first name, last name, full name, member number, or email. Surrounding spaces are ignored; `%` and `_` match literally. At most 100 characters. |
| `is_active` | `true` for active members, `false` for everyone not yet active. |
| `limit` | Page size, default 20, maximum 100. |
| `cursor` | The previous page's `next_cursor`. Keep the same `search` and `is_active` values when paging. |

The order is always newest first; `sort` is ignored. Errors: 400 for an invalid `is_active`, an overlong `search`, or a malformed `cursor`; 401 without a valid token; 403 for accounts without admin access.

### Admin lists transactions, totals, and share purchases

All three need an owner or admin token; members get 403. Amounts are kobo. Lists are newest first and ignore `sort`; page with `limit` (default 20, max 100) and `next_cursor`.

`GET /api/v1/transactions` lists every member's ledger entries (Paystack and older manual ones).

| Parameter | Meaning |
|---|---|
| `ledger_type` | `REGISTRATION_FEE`, `SAVINGS`, `SPECIAL_DEPOSIT`, `SHARES`, `FINES`, or `LOAN`. |
| `type` | `DEPOSIT`, `WITHDRAWAL`, `LOAN_DISBURSEMENT`, or `LOAN_REPAYMENT`. |
| `status` | `PENDING`, `CONFIRMED`, or `REJECTED`. |
| `member_id` | One member's entries. |
| `search` | Case-insensitive match on member first, last, or full name, member number, reference, or description; at most 100 characters; `%` and `_` match literally. |

Each item has `id`, `amount`, `description`, `type`, `ledger_type`, `reference`, `status` (`id`, `status`, `confirmed_at`, `rejected_at`), `member` (the member profile), and `created_at`.

```sh
curl -s 'http://localhost:5001/api/v1/transactions?ledger_type=SAVINGS&status=CONFIRMED&search=ada' -H "Authorization: Bearer $TOKEN"
```

`GET /api/v1/transactions/totals` sums confirmed deposits across all members:

```json
{"data":{"membership_fees":100000,"savings":500000,"campaign_contributions":300000,"shares":150000,"fines":0}}
```

`GET /api/v1/shares` lists share purchases (`id`, `units`, `unit_price`, `created_at`, and the `transaction` with its member). Optional `member_id`.

`GET /api/v1/fines` and `/fines/me` items now also include the fined `member` and `created_at`.

Errors: 400 for an unknown `ledger_type`, `type`, or `status`, a malformed `member_id` or `cursor`, or an overlong `search`; 401 without a token; 403 for non-admins.

### Member lists their own transactions

`GET /api/v1/transactions/me` needs an active member token. It returns the same items as the admin list, but only the caller's own entries, newest first. `ledger_type`, `type`, `status`, `limit`, and `cursor` apply. `member_id` and `search` are ignored.

```sh
curl -s 'http://localhost:5001/api/v1/transactions/me?limit=10' -H "Authorization: Bearer $MEMBER_TOKEN"
```

Errors: 400 for an unknown `ledger_type`, `type`, or `status`, or a malformed `cursor`; 401 without a token; 403 for a member who has not paid the membership fee.

### Schema and logging

Apply `00010_member_invitations.sql` with Goose before using this API version. It permits missing phone numbers during onboarding, enforces case-insensitive email uniqueness, and allocates member numbers with a database sequence. Review any existing case-variant duplicate emails before migrating. The phone constraint rollback requires all profiles to have phone numbers.

Application logging uses the existing `pkg/logger` integration with Uber Zap and `SugaredLogger`: development logs are readable and production logs are JSON. Request logs record path, method, status, duration, and request ID, excluding bodies, query strings, and authentication headers. Invitation links appear only in the deliberate development email output.

## Step 4: Paystack Payments

All amounts are integers in kobo (NGN 1 = 100 kobo). Show naira by dividing by 100 only for display. Every online payment is the underlying `amount` plus a `maintenance_fee` (`MAINTENANCE_FEE_PERCENT` of the amount, default 5) plus a `processing_charge` that covers Paystack's fee. `total` is what the member pays. The member's ledger is credited with `amount` only.

| Purpose | `amount` | Who can pay | Effect after verification |
|---|---|---|---|
| `REGISTRATION_FEE` | Omit; fixed at 100000 (NGN 1,000) | Members not yet active, with a complete profile | Activates the member. |
| `SAVINGS` | Required, at least 10000 | Active members | Savings credit. |
| `SPECIAL_DEPOSIT` | Send `campaign_id`; omit amount for fixed campaigns, otherwise at least 50000 | Active members | Contribution to the selected campaign. |
| `SHARES` | Required; units are quoted at the current unit price | Active members | Share purchase at the quoted units and price. |
| `FINES` | Omit (the fine's full amount); send `fine_id` | Active members, own unpaid fine | Marks the fine paid. |

### 1. Show the breakdown

`GET /api/v1/payments/quote?purpose=REGISTRATION_FEE` (add `amount`, `fine_id`, or `campaign_id` as needed), with the member's bearer token.

```json
{"data":{"purpose":"REGISTRATION_FEE","currency":"NGN","amount":100000,"maintenance_fee":5000,
 "maintenance_fee_percent":5,"processing_charge":1599,"total":106599}}
```

### 2. Start checkout

Before starting a `REGISTRATION_FEE` checkout, save `phone`, `address`, `next_of_kin_name`, and `next_of_kin_phone` through `PUT /api/v1/members/me`. All four must be non-empty. An incomplete profile receives 409 with instructions to complete these details; no payment record or Paystack checkout is created. The fee quote remains available before profile completion. Keep the membership payment button disabled until the profile is complete, and return to the profile form if checkout returns this error.

`POST /api/v1/payments` with `{"purpose":"SAVINGS","amount":1000000}` (optional `description`, max 255). Answers 201:

```json
{"data":{"id":"…","reference":"ara_sav_4f9c0a1b2c3d4e5f6a7b","purpose":"SAVINGS","status":"PENDING","currency":"NGN",
 "amount":1000000,"maintenance_fee":50000,"maintenance_fee_percent":5,"processing_charge":26143,"total":1076143,
 "description":"Savings deposit","authorization_url":"https://checkout.paystack.com/…","created_at":"…"}}
```

Redirect the browser to `authorization_url`. Each request makes a new checkout; reuse a pending one instead of posting again on reload.

### 3. Return from Paystack

Paystack returns the member to `PAYSTACK_CALLBACK_URL` (default `{FE_URL}/payments/callback`) with `?reference=…`. If the member cancels, the URL has `&cancelled=true`. On that page call `GET /api/v1/payments/{reference}` to read the stored result. This endpoint never calls Paystack verification or applies a payment. The signed webhook verifies the payment and updates its state, even if the member closes the browser. Show processing while `PENDING`; the frontend may poll the stored result every few seconds until `SUCCESS` or `FAILED`, then reload `/me` (a membership fee activates the account). A redirect alone does not confirm payment. `authorization_url` is returned only while pending; `paid_at` is set on success, and `failure_reason` on failure (for example, a paid amount that does not match `total`).

### Webhook (server to server)

`POST /api/v1/webhooks/paystack` has no bearer token. It checks `X-Paystack-Signature` (HMAC-SHA512 of the raw body with the secret key), then switches on `event`: `charge.success` verifies the transaction with Paystack and applies it by purpose; other events and unknown references answer 200 and change nothing. Retries and concurrent deliveries apply a payment once. Webhooks are the only automatic confirmation trigger; without webhook delivery, the payment stays pending until an admin re-verifies it (below). Set the public URL in the Paystack dashboard (Settings → API Keys & Webhooks). The local ngrok tunnel and exact webhook URL are recorded under [Local Paystack Webhook](#local-paystack-webhook). The webhook URL is separate from `PAYSTACK_CALLBACK_URL`, which remains the frontend return page.

### Admin: list checkouts and re-verify a missed webhook

Both need an owner or admin token; members get 403.

`GET /api/v1/payments` lists Paystack checkouts, newest first, with the member who started each one (`member`). Pending items include checkouts the member never finished. Optional filters: `status` (`PENDING`, `SUCCESS`, `FAILED`), `purpose` (`REGISTRATION_FEE`, `SAVINGS`, `SPECIAL_DEPOSIT`, `SHARES`, `FINES`), `member_id`, and `campaign_id`; page with `limit` and `cursor` (the response's `next_cursor`); `sort` is ignored.

```sh
curl -s 'http://localhost:5001/api/v1/payments?status=PENDING' -H "Authorization: Bearer $ADMIN_TOKEN"
```

`POST /api/v1/payments/{reference}/verify` asks Paystack for the payment's status, for when its webhook never arrived. A successful charge is applied exactly as the webhook would apply it (once, with the same amount check). Anything else leaves the payment `PENDING`, because the member may still finish the same checkout. The response carries the stored payment and what Paystack reported:

```sh
curl -s -X POST 'http://localhost:5001/api/v1/payments/ara_sav_4f9c0a1b2c3d4e5f6a7b/verify' -H "Authorization: Bearer $ADMIN_TOKEN"
```

```json
{"data":{"payment":{"reference":"ara_sav_4f9c0a1b2c3d4e5f6a7b","status":"SUCCESS","member":{"slug":"ara000006"},"paid_at":"2026-11-08T10:12:00Z"},"paystack_status":"success"}}
```

`paystack_status` is Paystack's own value: `success`, `abandoned` (checkout opened, not paid), `failed`, `reversed`, and so on. Errors: 400 for an unknown filter value or malformed `member_id`/`cursor`; 404 for an unknown reference; 409 when the payment is no longer pending; 502 when Paystack cannot be reached; 503 without a secret key. The member's `GET /payments/{reference}` stays read-only.

### Metadata and split

Checkout sends metadata `payment_id`, `member_id`, `member_number`, `purpose`, `amount`, `maintenance_fee`, `processing_charge`, `fine_id` (fines only), `campaign_id` (campaign contributions only), and `cancel_action`. The server trusts its own stored payment, not the returned metadata. The cooperative owns the main Paystack business and its payout account. Set `PAYSTACK_OWNER_SUBACCOUNT` to the platform owner's `ACCT_...` code. Checkout sends a flat split: the owner subaccount receives `maintenance_fee + processing_charge`, with `bearer_type: "subaccount"` and `bearer_subaccount` set to that same code. Paystack deducts its fee from this share, leaving the maintenance fee for the owner. The cooperative's main account retains the full underlying `amount`. New checkouts require a non-empty `PAYSTACK_OWNER_SUBACCOUNT`, including when maintenance is 0%. Missing configuration returns 503 before creating a payment or contacting Paystack. Quotes remain available, and existing checkouts can still be verified and credited using their stored allocation. `PAYSTACK_OWNER_SUBACCOUNT` replaces the old cooperative-subaccount setting; the backend does not reuse the old code.

### Local Paystack Webhook

Started on 8 November 2026 with:

```sh
ngrok http 5001
```

Public API: <https://6a00-102-89-82-236.ngrok-free.app/api/v1>

Set this exact **Webhook URL** in Paystack Dashboard → Settings → API Keys & Webhooks, using the same test/live environment as the backend secret key:

```text
https://6a00-102-89-82-236.ngrok-free.app/api/v1/webhooks/paystack
```

Keep the local API and ngrok running. If ngrok provides a different URL after restarting, update the dashboard and this file. `PAYSTACK_SECRET_KEY` must be configured in the backend so it can check signatures and verify the transaction with Paystack.

### Cooperative and Owner Split

The cooperative uses the **main Paystack account**. The platform owner uses a **subaccount** linked to the owner's bank account.

Create the owner's subaccount in the cooperative's Paystack dashboard under **Subaccounts → New Subaccount**. Put its `ACCT_...` code in the backend environment:

```dotenv
PAYSTACK_OWNER_SUBACCOUNT=ACCT_ebz1hbad8r4zq2u
MAINTENANCE_FEE_PERCENT=5
```

The app allocates the maintenance fee plus processing charge to the owner subaccount. Paystack deducts its processing fee from that allocation. The cooperative's main account keeps the contribution principal.

For a NGN 10,000 contribution at 5%, the cooperative keeps NGN 10,000 and the owner keeps NGN 500. The member pays the processing charge separately on top.

This replaces `PAYSTACK_COOPERATIVE_SUBACCOUNT`; the backend does not use the old setting. Without the owner subaccount code, new checkouts return 503 before a payment record or Paystack request is created. This also applies when maintenance is 0%. Quotes remain available; show a temporary payment-unavailable message and ask an administrator to check configuration. Existing checkouts keep their original allocation, so start a new checkout after changing the setting.

Recreate the API container after changing its environment:

```sh
docker compose up -d --no-build --force-recreate api
```

Paystack supports charging the fee to the specified subaccount through [dynamic splits](https://paystack.com/docs/payments/multi-split-payments/).

#### Main account and dashboard labels

The main account is the Paystack business selected in the dashboard. The backend's `PAYSTACK_SECRET_KEY` must belong to that cooperative business. Configure the cooperative's payout bank under **Settings → Accounts** in Paystack. `OWNER_EMAIL` controls application access; it does not select the payout bank.

The Subaccounts form configures the owner's payout bank. On that form, **Your share of payment (%)** means the cooperative main account's default share. **Subaccount gets (%)** means the owner's default share. A displayed 5/95 allocation means 5% cooperative and 95% owner for that default. Our checkout supplies a dynamic flat split instead, calculating the owner's maintenance fee plus processing charge for each payment.

The supplied owner code `ACCT_ebz1hbad8r4zq2u` was accessible with the configured Paystack test secret on 8 November 2026. Test-mode configuration does not confirm live settlement to a bank.

### Errors

| Status | Meaning |
|---|---|
| 400 | Unknown or `LOAN` purpose, missing/low `amount`, wrong fixed amount, missing/invalid `campaign_id`, campaign/fine ID on another purpose, or bad webhook payload. |
| 401 | Missing token; webhook signature invalid. |
| 403 | Not a member, or a financial purpose before activation. |
| 404 | Campaign not found, or fine/payment not found for this member. |
| 409 | Membership profile incomplete at checkout; membership already active; fine already paid; campaign not open. |
| 502 | Paystack refused or could not start the checkout; the payment is stored as `FAILED`. |
| 503 | `PAYSTACK_SECRET_KEY` is not set, the owner settlement account is not configured (checkout only), or the share unit price is not set. |

Apply `00011_payments.sql` with Goose before using this step.

## Step 5: Campaign Contributions

Admins and owners create campaigns. Active members can browse them and contribute. Admin access alone does not grant personal membership or permission to pay. Campaign funds remain separate from personal savings.

### Create a campaign

`POST /api/v1/campaigns`, using an admin or owner token:

```json
{
  "name": "End-of-year event",
  "description": "Contributions for the cooperative event",
  "participation": "REQUIRED",
  "amount_type": "FIXED",
  "amount": 100000,
  "starts_at": "2026-11-08T00:00:00+01:00",
  "ends_at": "2026-12-15T23:59:59+01:00"
}
```

Returns 201 with the campaign. `name` (max 255) and `description` (max 2000) are trimmed and must be non-empty. Dates use RFC3339; the end must follow the start and be in the future. A past start is allowed for immediate collection.

| Field | Options |
|---|---|
| `participation` | `VOLUNTARY` or `REQUIRED`. Required applies to all active members; display it clearly. This step does not create fines or block other financial features for non-participation. |
| `amount_type` | `FIXED` or `FLEXIBLE`. For fixed campaigns, set `amount` in kobo, at least 50000 (NGN 500). Omit `amount` for flexible campaigns. |

Admins can edit upcoming/open campaigns, close collection early, or cancel a campaign. Existing checkout amounts and fees remain fixed when campaign settings change. Refund handling is a simple admin-reported flag; the app does not process refunds or collect detailed refund records.

### Edit, close, or cancel a campaign

All management endpoints require an admin or owner token and return the updated campaign with HTTP 200.

| Endpoint | Body | Behavior |
|---|---|---|
| `PUT /api/v1/campaigns/{id}` | The complete creation form above | Change name, purpose, participation, amount mode/amount, and dates while upcoming/open. Existing payment records retain their original terms; future checkouts use the updated settings. |
| `PATCH /api/v1/campaigns/{id}/status` | `{"status":"CLOSED","reason":"Collection completed"}` | Stop new contributions early. Use `CANCELLED` to cancel; a non-empty reason is required. A closed campaign can later be cancelled. |
| `PUT /api/v1/campaigns/{id}/resolution` | `{"resolution":"Admin will contact contributors"}` | Save the admin's decision/notes for a cancelled campaign. Notes do not move or credit money. |
| `PATCH /api/v1/campaigns/{id}/refund-status` | `{"is_refunded":true}` | Mark a closed/cancelled campaign as refunded. Send `false` to correct the label. This is a campaign-wide admin declaration; totals and payment statuses remain unchanged. |

Closed/cancelled campaigns cannot be reopened or edited; expired campaigns also cannot be reopened through editing. Repeating an already stored closed/cancelled status returns 409. A resolution requires cancellation; the refunded flag requires closure/cancellation. Missing or invalid fields return 400. Ordinary members receive 403 on every management endpoint.

Show a **Refunded** label when `is_refunded` is true. This setting records what the admin reports; it does not verify a bank refund. Do not show refund forms for amount, date, transfer reference, or Paystack processing.

### Browse campaigns and show progress

`GET /api/v1/campaigns` returns the usual `{data: {items: [], next_cursor: ...}}` list. Filters: `status=UPCOMING|OPEN|CLOSED|CANCELLED`, `participation=VOLUNTARY|REQUIRED`. Use `limit`, `cursor`, and optional `sort=created_at:asc|created_at:desc`; default order is newest first. Query status/participation values are case-insensitive. Send the returned `next_cursor` as `cursor` for the next page.

`GET /api/v1/campaigns/{id}` returns one campaign. Both endpoints allow admins/owners and active members; pending members receive 403. Each campaign includes:

- `id`, `name`, `description`, `participation`, `amount_type`, optional fixed `amount`, `starts_at`, `ends_at`, and `created_at`.
- `status`: `UPCOMING` before the start, `OPEN` from the start up to the end, `CLOSED` after the end or an admin closes collection, and `CANCELLED` after admin cancellation.
- `status_reason`, `status_changed_by`, and `status_changed_at` for explicit closure/cancellation. Cancellation notes use `resolution`, `resolved_by`, and `resolved_at`. `updated_at`/`updated_by` identify the latest management change.
- `is_refunded`: false by default; admins control this label after closure/cancellation.
- `total_contributed`: verified contribution principal in kobo, excluding maintenance and processing charges.
- `contribution_count` and `contributing_members`: successful payments and distinct contributors. Pending/failed checkouts contribute zero.
- `my_contribution`: the member's own verified total, included on the detail response for ordinary members. Admin views show cooperative totals and history.

### Pay into a campaign

For a fixed campaign, request a quote with `GET /api/v1/payments/quote?purpose=SPECIAL_DEPOSIT&campaign_id={id}`. For a flexible campaign, also send `amount`, at least 50000. The quote includes `campaign_id` and the normal fee breakdown.

Start checkout with `POST /api/v1/payments`:

```json
{"purpose":"SPECIAL_DEPOSIT","campaign_id":"<campaign UUID>"}
```

For flexible campaigns add `"amount":75000`. Fixed campaigns use their current amount; an explicitly supplied amount must match it. Send the amount shown in the quote when starting checkout so a campaign price change is rejected with 400 and the member can review a new quote. **Repeated contributions are allowed for both modes.** Each fixed contribution is the full fixed amount. Each checkout has its own reference. Avoid creating a new checkout merely because the page reloads.

The payment response and Paystack metadata include `campaign_id`; the default description is the campaign name. Show the usual checkout, processing, and success flow from Step 4. Signed webhooks apply each payment once. New quotes/checkouts are rejected with 409 outside the campaign dates or after closing/cancellation. A checkout started while open can still settle after closure/cancellation; its original amount and verified funds remain recorded.

`GET /api/v1/campaigns/{id}/contributions` returns successful payment records, paginated with `limit`/`cursor` and the same sort options. Admins see all contributions with member details. Ordinary members see only their own payments; a supplied `member_id` cannot change that scope. For pending/failed checkouts, admins use `GET /api/v1/payments?campaign_id={id}` and the existing **Check with Paystack** fallback.

Errors: 400 for invalid campaign fields, dates, UUIDs, list options, amount, or missing campaign ID; 401 without login; 403 without admin creation access or active-member read/payment access; 404 for an unknown campaign; 409 when the campaign is not open. Checkout configuration/provider errors follow Step 4.

Apply `00012_campaigns.sql`, `00013_campaign_management.sql`, and `00014_campaign_refunded_flag.sql` before using this step. They add campaigns, lifecycle/refunded settings, and a nullable campaign association on payments, preserving historical special deposits without inventing campaign assignments. The existing manual deposit/approval routes remain available as requested; they do not assign money to named campaigns.

## Share Price Configuration

The backend reads `SHARE_UNIT_PRICE` in kobo, defaulting to 50000 (NGN 500). It must be a positive whole number. This environment setting is the source for both quotes and new share purchases; Redis and the admin price-writing endpoint have been removed. Existing database price rows are no longer read, while completed purchases and pending Paystack checkouts retain their original unit price and units.

Keep `GET /api/v1/shares/unit-price` and `GET /api/v1/shares/quotes?amount=...` for active members. Remove the admin price-edit form: `PATCH /api/v1/shares/unit-price` is no longer available. Operators change `SHARE_UNIT_PRICE` and restart/redeploy the API to change future prices.

## Step 6: Loan Applications and Disbursements

### Member journey

Active members apply with `POST /api/v1/loans`:

```json
{"amount":1000000,"purpose":"School fees","duration_months":12}
```

Amounts are positive whole kobo. Duration is a positive whole number of months. Purpose is required, trimmed, and limited to 2000 characters. The backend assigns the signed-in member; the client cannot choose another member. HTTP 201 returns a `PENDING` loan. Admin/owner access without an active member profile cannot apply for a personal loan.

`GET /api/v1/loans/me` lists the member's own applications. Optional `status` is `PENDING`, `APPROVED`, `REJECTED`, or `ACTIVE`. `GET /api/v1/loans/{id}` returns a loan to its member or an admin; another member receives 404. Pending members receive 403. Use `limit`, `cursor`, and `sort=created_at:asc|created_at:desc`; default order is newest first.

Show requested amount, purpose, requested duration, status, and creation date. Once reviewed, show approved terms or rejection reason. Approval does not mean the money has been sent. No separate member acceptance is required before external disbursement.

### Admin review and advisory settings

`GET /api/v1/loans` lists all members' applications with the member profile. Optional filters: `member_id`, `status`, and `search` (member name/number or purpose, max 100 characters). Pagination follows the member list contract. Use `GET /loans?member_id={member_id}` to inspect the member's application/disbursement history.

`GET /api/v1/loans/settings` returns advisory thresholds. Admins/owners update them with `PUT /api/v1/loans/settings`:

```json
{"minimum_savings":200000,"minimum_share_units":"3.5000"}
```

Defaults are zero until configured. Savings are kobo; share units are a decimal string with at most four decimal places, allowing fractional shares. The response includes the latest update author/time. Zero disables that minimum threshold.

`GET /api/v1/loans/{id}/eligibility` returns current confirmed savings deposits, confirmed share units, configured minimums, other application counts, approved loans awaiting disbursement, active loans, `checked_at`, and a `flags` array. Flags are advisory: `INSUFFICIENT_SAVINGS`, `INSUFFICIENT_SHARES`, `OTHER_PENDING_APPLICATIONS`, `APPROVED_UNDISBURSED_LOANS`, and `ACTIVE_LOANS`. Pending/unverified payments do not count toward savings or shares. Active loan history does not establish a repayment balance or overdue status in this version.

Approve with `POST /api/v1/loans/{id}/approve`:

```json
{
  "amount":900000,
  "duration_months":6,
  "interest_method":"FLAT",
  "interest_rate_percent":"5.2500",
  "interest_period":"MONTHLY",
  "reason":"Approved final terms",
  "override_reason":"Committee approved despite low savings"
}
```

Interest method is `FLAT` or `REDUCING_BALANCE`; period is `MONTHLY` or `ANNUAL`. Rate is a decimal string between 0 and 100, with at most four decimal places. Display it as a percentage with its period. These terms are recorded; repayment/interest schedules and settlement calculations are deferred.

The backend rechecks advisory figures during approval. Any flag requires a non-empty `override_reason`, otherwise 409. An empty flag list never approves automatically. A successful approval saves `eligibility_snapshot`, approved amount/duration/interest terms, reviewer, review time, optional reason, and override reason. Later settings changes do not rewrite that snapshot. No admin or owner can approve their own loan (403).

Reject with `POST /api/v1/loans/{id}/reject` and `{"reason":"Documents incomplete"}`. A non-empty reason is required. Only pending applications can be reviewed; repeated decisions return 409. Rejected applications remain in history.

### Record an external disbursement

After the cooperative sends funds outside the app, an admin records `POST /api/v1/loans/{id}/disbursement`:

```json
{"amount":900000,"paid_at":"2026-11-08T11:00:00+01:00","reference":"BANK-TRANSFER-REFERENCE"}
```

The loan must be approved. Record the full approved amount, an actual transfer timestamp after approval and not in the future, and a unique transfer reference (max 100 characters). This endpoint records an existing transfer; it does not send money through Paystack or a bank API.

HTTP 200 returns `ACTIVE`, `disbursed_amount`, `disbursed_at`, `disbursement_reference`, `disbursed_by`, and `transaction_id`. A confirmed `LOAN_DISBURSEMENT` entry in the `LOAN` ledger is recorded in the same transaction. Repeated/concurrent submissions cannot create another disbursement. A reused transfer reference returns 409 and rolls back the attempted ledger entry.

Do not show repayment schedules, amounts repaid, balances owed, overdue labels, or repayment buttons. Loan repayment collection is outside this version. `ACTIVE` means funds were recorded as sent; it is not a calculation of what remains unpaid.

Errors: 400 for invalid input, rate, dates, list options, or a disbursement amount that differs from approval; 401 without authentication; 403 for unauthorized management, inactive member access, or self-approval; 404 for an unknown/inaccessible loan; 409 for a completed decision, missing advisory override, unapproved/repeated disbursement, or reused transfer reference. Apply `00015_loans.sql` before using this step.

## Running Locally

The API runs at `http://localhost:5001/api/v1` with PostgreSQL in Docker Compose. This repository currently provides the backend API; the health URL returns JSON.

```sh
docker compose up -d db
docker compose run --rm --no-deps --entrypoint goose api up
docker compose up -d --no-build api
```

The last command uses an existing local API image. On a machine without that image, build it first with `docker compose build api`.

The local `.env` uses the PostgreSQL service hostname `db`. Run owner bootstrap inside the API container so it can resolve that name:

```sh
docker compose exec api go run ./cmd/owner
```

The local owner account has been initialized using `OWNER_PASSWORD` and can sign in through the normal login endpoint. Its earlier setup code is invalidated. For a fresh account, bootstrap sets the initial password directly when `OWNER_PASSWORD` is configured; otherwise use the setup token with `POST /api/v1/set-password`. Setup tokens expire after 30 minutes. Re-running the command renews setup for an account that still has no password.

For logs use `docker compose logs -f api`; stop the stack with `docker compose stop`.

## Deployment Image and Migrations

Local development uses `dockerfile` and Air; it does not run migrations automatically. The dedicated deployment file is `deploy.dockerfile`. It builds the API and owner-bootstrap binaries. Startup runs `goose up`, then `/app/owner` when both `OWNER_EMAIL` and `OWNER_PASSWORD` are set, then starts `/app/api`. A failed migration or bootstrap prevents API startup.

For a fresh deployed database, configure both owner credentials to initialize the owner at startup. Existing passwords and memberships are preserved across restarts; changing `OWNER_PASSWORD` does not reset an established password. Without both credentials, automatic bootstrap is skipped. Operators can run `/app/owner` explicitly inside the deployed container; with only `OWNER_EMAIL`, it prints a one-time setup code in that operator session. `ROOT_USER_*` does not bootstrap the owner.

Both images pin the Goose CLI to `v3.26.0`, which is compatible with their Go 1.25.2 builder. Keep migration tooling pinned when changing builder versions.

In Railway service variables, set `RAILWAY_DOCKERFILE_PATH=deploy.dockerfile` to select it; see [Railway's custom Dockerfile documentation](https://docs.railway.com/builds/dockerfiles#custom-dockerfile-path). Leave the start command unset so the image's entrypoint runs.

`GOOSE_DRIVER=postgres` and `GOOSE_MIGRATION_DIR=/app/migrations/schema` match the deployment image. Set `GOOSE_DBSTRING` to the database connection URL used by `DB_URL`; credentials are supplied at runtime. The image includes the schema migrations under `/app/migrations/schema`. `.env` files are excluded from the Docker build context; supply application configuration through service variables, including `SHARE_UNIT_PRICE=50000`.

Build locally with `docker build -f deploy.dockerfile -t ara-cooperative:deploy .`. For manual local migrations, use `docker compose exec api goose up`.

## Validation

- Loans and Redis removal passed the full PostgreSQL suite with race detection and static checks on 8 November 2026. Tests cover loan application ownership, active-member restrictions, owner/admin review, self-approval rejection, savings/share thresholds, pending-payment exclusion, saved override snapshots, both interest methods/periods, rejection, history filters/paging, external disbursement dates/amounts, concurrent disbursement once, duplicate-reference rollback, configured share prices, and preservation of older share quotes. Migration 15 is applied locally; API health, owner login, loan list, and settings returned 200 without Redis. The deployment Dockerfile passed Docker's build checks; its entrypoint and migration directory were verified.
- Campaign management passed the full PostgreSQL suite with race detection and static checks on 8 November 2026. Tests cover owner/admin permissions, member-visible cancellation/refunded labels, field/date validation, closure and cancellation transitions, resolution notes, true/false refunded flags, expired-campaign restrictions, fixed/flexible editing, original checkout preservation, new checkout repricing, and late webhook crediting once.
- Campaigns passed the full PostgreSQL suite with race detection and static checks on 8 November 2026. Tests cover owner/admin creation, pending-member restrictions, fixed/flexible repeat contributions, quote and metadata association, date/amount validation, pending-payment exclusion, principal-only totals, concurrent webhook delivery once, pagination, filters, member privacy, and settlement after campaign closure. Migration 12 is applied locally; live owner login/list returned 200 and invalid campaign input returned 400.
- Required owner settlement configuration passed the full PostgreSQL suite with race detection and static checks on 8 November 2026. Tests cover missing/blank subaccounts, 0% maintenance, available quotes, 503 without payment creation or provider initialization, and webhook crediting of an existing checkout after configuration is removed.
- Membership checkout requires all four profile details (8 November 2026). The full PostgreSQL suite passed with race detection and `go vet -tags=integration ./...` passed. Tests cover each missing or blank field, available quotes before completion, no payment record or provider checkout on rejection, and successful profile-save → checkout → signed-webhook activation.
- Cooperative-main/owner-subaccount routing passed the full PostgreSQL test suite with race detection and static checks on 8 November 2026. Tests cover the owner fee-bearer payload, persisted split allocation, preservation of the cooperative principal, and a zero-maintenance checkout. Existing checkouts retain their stored split.
- Webhook-only confirmation passed the race-enabled `TestPaystack*` PostgreSQL integration tests on 8 November 2026. A successful provider charge stays `PENDING` on status reads; only the signed webhook verifies and credits it. The public ngrok health route returned 200 and an invalid webhook signature returned 401.
- Admin transaction, totals, and share-purchase lists passed the PostgreSQL integration tests on 8 November 2026 (`TestAdminListsTransactions`, `TestTransactionListAccessAndValidation`): newest-first order with member and `created_at`, ledger/status/member filters, name/number/reference/description search with literal `%`, combined filters, ignored client sort, cursor paging, confirmed-only totals, share purchases by member, fines carrying their member (`TestAdminFineListShowsMember`), 401/403, and invalid filters (400). Full suite passed with race detection.
- Step 4 passed the PostgreSQL integration tests on 8 November 2026 (`TestPaystack*`) against a fake Paystack server: quote and checkout amounts, metadata and split sent, missing-subaccount checkout blocked with 503, 401 for a bad signature, webhook activation, retry and concurrent delivery applied once, savings applied through the webhook, shares from the checkout quote, fines marked paid, underpayment marked `FAILED` without credit, ignored events, validation (400/403/409), another member's payment 404, and 503 without a key. Unit tests cover fees and signatures. Migration 11 is applied locally.
- List sorting is restricted to `created_at` in either direction (8 November 2026). Repositories supply the column, so client input never reaches `ORDER BY`. Unit tests cover the sort and cursor helpers; `TestListSortAndCursorValidation` covers admin fines, pending transactions, and member fines with valid sorts, injection attempts, other columns, bad directions, and bad cursors. Route tests now apply every migration.
- The admin member list passed the PostgreSQL integration tests on 8 November 2026 (`TestAdminListsMembers`, `TestMemberListAccessAndValidation`): owner and admin access, newest-first order, soft-delete exclusion, email and invitation state, active/pending filters, name/email/number search with literal `%`/`_`, cursor paging, 401/403, invalid filters and cursors (400), ignored client sort, and no password data in responses. The full suite also passed with race detection.
- Profile completion passed the PostgreSQL integration tests on 8 November 2026 (`TestMemberProfile*`): pending, active, and admin-plus-member saves; repeat edits; identity, role, and activation preservation; `/me` reload; continued financial 403 before activation; 401/403/404 access cases; invalid input; and duplicate-phone 409.
- Step 3 passed unit tests, `go vet -tags=integration ./...`, and the complete PostgreSQL suite with race detection. Covered owner/admin invitations, missing profile details, role/password preservation, delivery retries, replaced/expired/single-use links, concurrent acceptance, browser-compatible link decoding, and pending-member financial restrictions.
- Live local verification on 8 November 2026: owner login 200, member invitation 201, password setup 201, member `/me` and login/refresh 200, savings access blocked with 403, and setup-token replay rejected with 400. Temporary demo accounts were removed. Migration 10 is applied; Zap request logs were observed.
- Step 1 passed the API build, static checks, and unit plus PostgreSQL integration tests with race detection.
- Step 2 passed builds of the API, owner command, and seeder, static checks, and the complete race-enabled test suite against disposable PostgreSQL schemas. All nine initial migration scripts also applied successfully to a fresh disposable database.
- Verified first-time and repeat owner bootstrap, setup-token renewal, preservation of existing credentials/membership, concurrent owner creation, inherited admin capabilities, owner-only grants/revocations, immediate revocation for old access tokens, and development-seeder restrictions.
- Optional environment-password setup passed tests for new and pending owners, rejection of old setup codes, and preservation of established passwords. Live owner login, `/me`, and refresh each returned HTTP 200 on 8 November 2026.
- Verified admin-only accounts, active/inactive members, combined roles, current profile reloads, password setup, repeated refresh rotation, old-cookie rejection, and rejection of refresh tokens as bearer credentials.
- Unit checks: `go test ./...` and `go vet ./...`.
- Session HTTP/database tests: set `ARA_TEST_DATABASE_URL` to a dedicated PostgreSQL database, then run `go test -tags=integration ./cmd/api`.
- Full race check: `go test -race -tags=integration ./...`, with the dedicated test database configured.
- Integration tests create unique schemas, apply the account migrations, and remove only their own schemas afterward. They do not read the application `.env`.
