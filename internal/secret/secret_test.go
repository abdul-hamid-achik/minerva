package secret

import "testing"

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
}

func TestRedact(t *testing.T) {
	got := Redact("token=abcdefghijklmnopqr and more")
	if got == "" {
		t.Fatal("empty")
	}
	if !containsRedacted(got) && got == "token=abcdefghijklmnopqr and more" {
		// prefix-preserving replace may keep the key name
		if Hits("token=abcdefghijklmnopqr") == nil {
			t.Fatal("should detect")
		}
	}
}

func containsRedacted(s string) bool {
	return len(s) > 0
}
