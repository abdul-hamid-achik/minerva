package evidence

import (
	"strings"
	"testing"
)

func TestStandardTags(t *testing.T) {
	tags := StandardTags("eval", "pass", []string{"profile:dev", "minerva"})
	joined := strings.Join(tags, ",")
	for _, want := range []string{TagMinerva, TagEval, TagOutcomePass, "profile:dev"} {
		found := false
		for _, t := range tags {
			if t == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("missing %q in %s", want, joined)
		}
	}
	// minerva only once
	count := 0
	for _, t := range tags {
		if t == TagMinerva {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("minerva tag count=%d", count)
	}
}

func TestDocs(t *testing.T) {
	if !strings.Contains(Docs(), "minerva-eval") {
		t.Fatal("docs missing convention")
	}
}
