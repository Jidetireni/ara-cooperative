package loans

import (
	"testing"

	"github.com/google/uuid"
)

func TestFormatNaira(t *testing.T) {
	for kobo, want := range map[int64]string{
		0: "₦0.00", 5: "₦0.05", 40_000: "₦400.00", 100_000_000: "₦1,000,000.00", 12_345_678: "₦123,456.78", -150: "-₦1.50",
	} {
		if got := formatNaira(kobo); got != want {
			t.Errorf("formatNaira(%d) = %q, want %q", kobo, got, want)
		}
	}
}

func TestGuaranteeLink(t *testing.T) {
	id := uuid.MustParse("6b1f8a3e-1d2c-4f7a-9b8e-0c1d2e3f4a5b")
	got, err := guaranteeLink("https://app.example.com/?x=1#y", id)
	if err != nil || got != "https://app.example.com/guarantees/"+id.String() {
		t.Fatalf("got %q, %v", got, err)
	}
	if _, err := guaranteeLink("", id); err == nil {
		t.Fatal("empty FE_URL accepted")
	}
}
