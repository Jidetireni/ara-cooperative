package payments

import "testing"

func TestMaintenanceFee(t *testing.T) {
	tests := []struct {
		amount  int64
		percent int32
		want    int64
	}{
		{100_000, 5, 5_000},    // NGN 1,000 at 5% = NGN 50
		{1_000_000, 5, 50_000}, // NGN 10,000 at 5% = NGN 500
		{33, 5, 1},             // part kobo is dropped
		{1_000_000, 0, 0},      // fee disabled
		{1_000_000, 100, 1_000_000},
	}
	for _, tt := range tests {
		if got := MaintenanceFee(tt.amount, tt.percent); got != tt.want {
			t.Errorf("MaintenanceFee(%d, %d) = %d, want %d", tt.amount, tt.percent, got, tt.want)
		}
	}
}

func TestProcessingChargeCoversPaystackFee(t *testing.T) {
	for _, net := range []int64{
		1, 100, 10_500, 105_000, 240_000, 245_000, 246_000, 250_000, 1_050_000,
		10_000_000, 12_600_000, 12_700_000, 13_000_000, 100_000_000, 1_000_000_000,
	} {
		charge := ProcessingCharge(net)
		total := net + charge
		if received := total - paystackFee(total); received < net {
			t.Fatalf("net %d: total %d leaves %d after Paystack fee", net, total, received)
		}
		// Minimal: one kobo less would not cover the fee.
		if smaller := total - 1; smaller >= net && smaller-paystackFee(smaller) >= net {
			t.Fatalf("net %d: total %d is not the smallest covering total", net, total)
		}
		if charge > paystackFeeCap {
			t.Fatalf("net %d: charge %d exceeds Paystack cap", net, charge)
		}
	}
}

func TestProcessingChargeExamples(t *testing.T) {
	// NGN 1,050 (membership fee + 5%): under NGN 2,500, so 1.5% only.
	if got := ProcessingCharge(105_000); got != 1_599 {
		t.Errorf("ProcessingCharge(NGN 1,050) = %d kobo, want 1599", got)
	}
	// NGN 10,500 (savings example): 1.5% + NGN 100.
	if got := ProcessingCharge(1_050_000); got != 26_143 {
		t.Errorf("ProcessingCharge(NGN 10,500) = %d kobo, want 26143", got)
	}
	// Large payments hit the NGN 2,000 cap.
	if got := ProcessingCharge(100_000_000); got != paystackFeeCap {
		t.Errorf("ProcessingCharge(NGN 1,000,000) = %d kobo, want cap", got)
	}
	if got := ProcessingCharge(0); got != 0 {
		t.Errorf("ProcessingCharge(0) = %d", got)
	}
}
