package payments

// All amounts are kobo. Paystack's NGN pricing: 1.5% plus NGN 100, the NGN 100
// waived under NGN 2,500, capped at NGN 2,000. https://paystack.com/pricing
const (
	paystackFlatFee     int64 = 10_000  // NGN 100
	paystackFlatFeeFrom int64 = 250_000 // NGN 2,500
	paystackFeeCap      int64 = 200_000 // NGN 2,000
)

// MaintenanceFee is percent% of amount, in whole kobo.
func MaintenanceFee(amount int64, percent int32) int64 {
	return amount * int64(percent) / 100
}

// paystackFee is what Paystack keeps from a charge of total kobo.
func paystackFee(total int64) int64 {
	fee := (total*15 + 999) / 1000 // 1.5%, rounded up to the next kobo
	if total >= paystackFlatFeeFrom {
		fee += paystackFlatFee
	}
	return min(fee, paystackFeeCap)
}

// ProcessingCharge is the smallest amount to add to net so that, after
// Paystack takes its fee, at least net is left. Members pay this charge.
func ProcessingCharge(net int64) int64 {
	if net <= 0 {
		return 0
	}
	total := net
	for total-paystackFee(total) < net {
		total = net + paystackFee(total)
	}
	return total - net
}
