package email

import (
	"html"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

func TestMemberInvitationTemplates(t *testing.T) {
	setup := "https://staging.example.com/set-password#" +
		url.Values{"email": {"ada+1@example.com"}, "token": {strings.Repeat("a", 64)}}.Encode()
	tests := []struct {
		name       string
		data       MemberInvitationData
		wantLabel  string
		wantExpiry bool
	}{
		{"password setup", MemberInvitationData{FirstName: `Ada <b>"x"</b>`, ActionURL: setup, NeedsPasswordSetup: true, ExpiresInMinutes: 30}, "Set your password", true},
		{"existing password", MemberInvitationData{FirstName: "Bola", ActionURL: "https://staging.example.com/login"}, "Sign in", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Render(EmailTemplateTypeMemberInvitation, tt.data)
			if err != nil {
				t.Fatal(err)
			}
			// The first link is the call to action; the invitation test relies on it.
			match := regexp.MustCompile(`href="([^"]+)"`).FindStringSubmatch(got.HTML)
			if len(match) != 2 || html.UnescapeString(match[1]) != tt.data.ActionURL {
				t.Fatalf("first link = %q, want %q", match, tt.data.ActionURL)
			}
			if !strings.Contains(got.HTML, tt.wantLabel) || !strings.Contains(got.Text, tt.data.ActionURL) {
				t.Fatal("call to action missing")
			}
			if strings.Contains(got.HTML, "<b>") {
				t.Fatal("member name must be escaped in HTML")
			}
			if hasExpiry := strings.Contains(got.Text, "expires in 30 minutes"); hasExpiry != tt.wantExpiry {
				t.Fatalf("expiry note shown = %v, want %v", hasExpiry, tt.wantExpiry)
			}
		})
	}
}

func TestLoanGuarantorRequestTemplate(t *testing.T) {
	data := LoanGuarantorRequestData{
		FirstName: "Bola", ApplicantName: `Ada <script>`, ApplicantNumber: "ARA000001", Amount: "₦400,000.00",
		Purpose: "School fees", DurationMonths: 1, ActionURL: "https://app.example.com/guarantees/abc",
	}
	got, err := Render(EmailTemplateTypeLoanGuarantorRequest, data)
	if err != nil {
		t.Fatal(err)
	}
	match := regexp.MustCompile(`href="([^"]+)"`).FindStringSubmatch(got.HTML)
	if len(match) != 2 || html.UnescapeString(match[1]) != data.ActionURL {
		t.Fatalf("first link = %q, want %q", match, data.ActionURL)
	}
	if strings.Contains(got.HTML, "<script>") {
		t.Fatal("applicant name must be escaped in HTML")
	}
	for _, want := range []string{"a loan of ₦400,000.00", "(ARA000001)", "Duration: 1 month\n", data.ActionURL} {
		if !strings.Contains(got.Text, want) {
			t.Fatalf("text part missing %q:\n%s", want, got.Text)
		}
	}
}
