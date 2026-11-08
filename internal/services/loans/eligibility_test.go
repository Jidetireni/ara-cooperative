package loans

import "testing"

func TestDecimal(t *testing.T) {
	for _, tt := range []struct{ name, input, want string }{
		{name: "zero", input: "0", want: "0.0000"},
		{name: "fraction", input: " 5.25 ", want: "5.2500"},
		{name: "four places", input: "10.1234", want: "10.1234"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, _, err := decimal(tt.input)
			if err != nil || got != tt.want {
				t.Fatalf("got %q, %v; want %q", got, err, tt.want)
			}
		})
	}
	for _, input := range []string{"", "-1", "NaN", "1e4", "1.12345", "100000000000000", "1/2"} {
		t.Run(input, func(t *testing.T) {
			if _, _, err := decimal(input); err == nil {
				t.Fatal("invalid decimal accepted")
			}
		})
	}
}
