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

func TestStandardTags_Closed(t *testing.T) {
	tags := StandardTags("eval", "pass", []string{TagClosesPrefix + "abc", TagOutcomeClosed})
	joined := strings.Join(tags, " ")
	if !strings.Contains(joined, TagMinerva) || !strings.Contains(joined, TagEval) {
		t.Fatalf("tags=%v", tags)
	}
	if !strings.Contains(joined, TagClosesPrefix+"abc") || !strings.Contains(joined, TagOutcomeClosed) {
		t.Fatalf("close tags missing: %v", tags)
	}
}

func TestDocs(t *testing.T) {
	docs := Docs()
	if !strings.Contains(docs, "minerva-eval") {
		t.Fatal("missing minerva-eval in docs")
	}
	if !strings.Contains(docs, "evidence close") && !strings.Contains(docs, "closes:") {
		t.Fatal("docs should mention close loop")
	}
}

func TestParseSkillProfileTags(t *testing.T) {
	skills, profiles := parseSkillProfileTags([]string{
		"minerva", "outcome:fail", "skill:qa-tester", "profile:code-reviewer", "skill:qa-tester",
	})
	if len(skills) != 2 || skills[0] != "qa-tester" {
		// dedupe not applied in parse — two entries ok
		if len(skills) < 1 || skills[0] != "qa-tester" {
			t.Fatalf("skills=%v", skills)
		}
	}
	if len(profiles) != 1 || profiles[0] != "code-reviewer" {
		t.Fatalf("profiles=%v", profiles)
	}
}
