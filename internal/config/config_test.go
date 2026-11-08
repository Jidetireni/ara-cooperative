package config

import "testing"

func TestParseShareUnitPrice(t *testing.T) {
	for _, tt := range []struct {
		name  string
		value string
		want  int64
	}{
		{name: "default", want: 50_000},
		{name: "configured", value: "75000", want: 75_000},
		{name: "trimmed", value: " 50000 ", want: 50_000},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseShareUnitPrice(tt.value)
			if err != nil || got != tt.want {
				t.Fatalf("got %d, %v; want %d", got, err, tt.want)
			}
		})
	}
	for _, value := range []string{"0", "-1", "+50000", "5.5", "abc", "9223372036854775808"} {
		t.Run(value, func(t *testing.T) {
			if _, err := ParseShareUnitPrice(value); err == nil {
				t.Fatal("invalid share price accepted")
			}
		})
	}
}

func TestParseMaintenanceFeePercent(t *testing.T) {
	valid := map[string]int32{"": 5, "5": 5, " 5 ": 5, "0": 0, "100": 100}
	for input, want := range valid {
		got, err := ParseMaintenanceFeePercent(input)
		if err != nil || got != want {
			t.Errorf("ParseMaintenanceFeePercent(%q) = %d, %v; want %d", input, got, err, want)
		}
	}
	for _, input := range []string{"-1", "101", "2.5", "abc", "5%", "+5"} {
		if got, err := ParseMaintenanceFeePercent(input); err == nil {
			t.Errorf("ParseMaintenanceFeePercent(%q) = %d; want error", input, got)
		}
	}
}
