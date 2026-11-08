package config

import "testing"

func TestParseEmailConfig(t *testing.T) {
	gmail, err := ParseEmailConfig("", "", "", "", "pw")
	if err != nil || gmail.Host != "smtp.gmail.com" || gmail.Port != 587 ||
		gmail.FromAddress != "jtirenipraise@gmail.com" || gmail.Username != gmail.FromAddress {
		t.Fatalf("defaults = %+v, %v", gmail, err)
	}
	resend, err := ParseEmailConfig("smtp.resend.com", "465", "resend", "Ara Cooperative <no-reply@mail.aracoop.xyz>", "key")
	if err != nil || resend.Port != 465 || resend.Username != "resend" ||
		resend.FromAddress != "no-reply@mail.aracoop.xyz" || resend.FromName != "Ara Cooperative" {
		t.Fatalf("resend = %+v, %v", resend, err)
	}
	for _, bad := range [][2]string{{"0", "a@b.co"}, {"smtp", "a@b.co"}, {"587", "not an address"}} {
		if _, err := ParseEmailConfig("", bad[0], "", bad[1], ""); err == nil {
			t.Fatalf("port %q from %q: want error", bad[0], bad[1])
		}
	}
}
