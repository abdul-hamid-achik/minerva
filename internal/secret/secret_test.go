package secret

import (
	"strings"
	"testing"
)

func TestHits_APIKey(t *testing.T) {
	hits := Hits("api_key: abcdefghijklmnopqr")
	if len(hits) == 0 {
		t.Fatal("expected hit")
	}
}

func TestHits_Empty(t *testing.T) {
	if Hits("   ") != nil {
		t.Fatal("empty should be nil")
	}
	if Hits("just prose about tokens and keys in general") != nil {
		t.Fatal("prose should be clean")
	}
}

// Every family Hits can flag must also be stripped by Redact, and the secret
// material must be gone while the identifying prefix survives.
func TestRedact_CoversEveryFamily(t *testing.T) {
	cases := []struct{ in, mustKeep, mustDrop string }{
		{"token=abcdefghijklmnopqrstuv and more", "token=", "abcdefghijklmnopqrstuv"},
		{`STRIPE_SECRET_KEY: "sk_live_FAKEFAKEFAKEFAKE1234"`, "sk_live_", "FAKEFAKEFAKEFAKE1234"},
		{"export STRIPE_KEY=sk_test_51H8vY2eZvKYlo2C1234567890", "STRIPE_KEY=", "51H8vY2eZvKYlo2C1234567890"},
		{"whsec_1234567890abcdefghij", "whsec_", "1234567890abcdefghij"},
		{"Authorization: Bearer eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0In0.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV", "Bearer", "SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV"},
		{"sk-proj-abcdefghijklmnopqrstuvwxyz0123", "sk-proj-", "abcdefghijklmnopqrstuvwxyz0123"},
		{"sk-ant-api03-abcdefghijklmnopqrstuvwxyz", "sk-ant-", "abcdefghijklmnopqrstuvwxyz"},
		{"ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789", "ghp_", "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"},
		{"github_pat_11ABCDEFG0123456789_abcdefghijklmnop", "github_pat_", "11ABCDEFG0123456789_abcdefghijklmnop"},
		{"xoxb-123456789012-abcdefghijkl", "xoxb-", "123456789012-abcdefghijkl"},
		{"aws_access_key_id = AKIAIOSFODNN7EXAMPLE", "AKIA", "IOSFODNN7EXAMPLE"},
		{"maps: AIzaSyA-abcdefghijklmnopqrstuvwxyz0123456", "AIza", "SyA-abcdefghijklmnopqrstuvwxyz0123456"},
		{"npm_abcdefghijklmnopqrstuvwxyz0123456789", "npm_", "abcdefghijklmnopqrstuvwxyz0123456789"},
		{"postgres://minerva:SuperSecretPass1@db.internal:5432/app", "postgres://minerva:", "SuperSecretPass1"},
		{"export DATABASE_PASSWORD='correct-horse-battery-staple'", "DATABASE_PASSWORD=", "correct-horse-battery-staple"},
		{"MY_PRIVATE_TOKEN=abcdefghijklmnopqrstuvwxyz", "MY_PRIVATE_TOKEN=", "abcdefghijklmnopqrstuvwxyz"},
		{"-----BEGIN RSA PRIVATE KEY-----\nMIIEowIBAAKCAQEA\nabc\n-----END RSA PRIVATE KEY-----", "-----BEGIN RSA PRIVATE KEY-----", "MIIEowIBAAKCAQEA"},
	}
	for _, c := range cases {
		if Hits(c.in) == nil {
			t.Errorf("Hits missed: %q", c.in)
		}
		got := Redact(c.in)
		if strings.Contains(got, c.mustDrop) {
			t.Errorf("Redact leaked %q in %q → %q", c.mustDrop, c.in, got)
		}
		if !strings.Contains(got, c.mustKeep) {
			t.Errorf("Redact dropped prefix %q in %q → %q", c.mustKeep, c.in, got)
		}
		if !strings.Contains(got, "[redacted]") {
			t.Errorf("no marker in %q", got)
		}
	}
}

func TestRedact_LeavesNormalTextAlone(t *testing.T) {
	clean := []string{
		"run go test ./... and report",
		`{"command":"git commit -m \"add token refresh\""}`,
		"the API key is stored in 1Password, never in the repo",
		"PATH=/usr/local/bin:/usr/bin",
		"export EDITOR=vim",
		"skills: [a, b, c]",
	}
	for _, s := range clean {
		if got := Redact(s); got != s {
			t.Errorf("changed clean text %q → %q", s, got)
		}
		if h := Hits(s); h != nil {
			t.Errorf("false positive %v on %q", h, s)
		}
	}
}

func TestRedact_Idempotent(t *testing.T) {
	in := "export API_TOKEN=abcdefghijklmnopqrstuvwxyz\nsk_live_FAKEFAKEFAKEFAKE1234"
	once := Redact(in)
	if Redact(once) != once {
		t.Fatalf("not idempotent:\n%s\n%s", once, Redact(once))
	}
}
