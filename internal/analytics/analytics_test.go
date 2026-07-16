package analytics

import (
	"path/filepath"
	"testing"
)

func TestRecord_DoesNotWipeHistory(t *testing.T) {
	dir := t.TempDir()

	s1 := NewStore(dir)
	if err := s1.Record("skill_activate", "a", ""); err != nil {
		t.Fatal(err)
	}

	// New process-style store must merge with disk.
	s2 := NewStore(dir)
	if err := s2.Record("skill_activate", "b", ""); err != nil {
		t.Fatal(err)
	}

	s3 := NewStore(dir)
	if err := s3.Load(); err != nil {
		t.Fatal(err)
	}
	sum := s3.Summarize()
	if sum.TotalEvents != 2 {
		t.Fatalf("TotalEvents=%d want 2 (file=%s)", sum.TotalEvents, filepath.Join(dir, analyticsFileName))
	}
	if sum.SkillActivations["a"] != 1 || sum.SkillActivations["b"] != 1 {
		t.Fatalf("activations=%v", sum.SkillActivations)
	}
}

func TestSummarize_SuggestionAppliedKinds(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)
	_ = s.Record("suggestion_applied", "1", "x")
	_ = s.Record("suggest_apply", "1", "y") // legacy kind
	sum := s.Summarize()
	if sum.SuggestionsApplied != 2 {
		t.Fatalf("SuggestionsApplied=%d want 2", sum.SuggestionsApplied)
	}
}
