package propose

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/minerva/internal/signal"
	"github.com/abdul-hamid-achik/minerva/internal/skill"
)

func TestFromSignalsAndApply(t *testing.T) {
	sigs := []signal.Signal{{
		Kind:    signal.KindShellFam,
		Key:     "git",
		Message: "repeated shell family git",
		Weight:  4,
		Evidence: []signal.Evidence{{
			Harness: "claude", SessionID: "s1", Workspace: "/tmp/proj",
		}},
	}}
	proposals := FromSignals(sigs, nil)
	if len(proposals) == 0 {
		t.Fatal("expected proposal")
	}
	p := proposals[0]
	if p.Kind != KindNew || p.Draft == "" {
		t.Fatalf("%#v", p)
	}
	dir := t.TempDir()
	if err := Save(dir, proposals); err != nil {
		t.Fatal(err)
	}
	got, err := Find(dir, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	path, err := Apply(dir, *got)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(path, "SKILL.md") {
		t.Fatalf("path=%s", path)
	}
	mgr := skill.ForAgents(dir)
	if err := mgr.LoadAll(); err != nil {
		t.Fatal(err)
	}
	if !mgr.Has(got.Name) {
		t.Fatal("skill not created")
	}
	if _, err := os.Stat(filepath.Join(dir, ".minerva", "proposals.json")); err != nil {
		t.Fatal(err)
	}
}

func TestFromSignals_CollapsesEditNgrams(t *testing.T) {
	sigs := []signal.Signal{
		{Kind: signal.KindRepeat, Key: "Read → StrReplace → StrReplace", Message: "a", Weight: 4},
		{Kind: signal.KindRepeat, Key: "StrReplace → Read → StrReplace", Message: "b", Weight: 3},
	}
	got := FromSignals(sigs, nil)
	if len(got) != 1 {
		t.Fatalf("expected one collapsed proposal, got %#v", got)
	}
	if got[0].Name != "read-strreplace-loop" {
		t.Fatalf("name=%s", got[0].Name)
	}
}

func TestFromSignals_SkipsBrowseOnlyNames(t *testing.T) {
	sigs := []signal.Signal{{
		Kind:    signal.KindRepeat,
		Key:     "Read → Grep → Read",
		Message: "repeated tool sequence Read → Grep → Read",
		Weight:  4,
	}}
	if got := FromSignals(sigs, nil); len(got) != 0 {
		t.Fatalf("expected no browse-only proposal, got %#v", got)
	}
}

func TestMergeObserved_Idempotent(t *testing.T) {
	body := "# Git Workflow\n\n## When\n\nUse when running git.\n\n## Steps\n\n1. Do it.\n"
	p := Proposal{
		Kind: KindUpdate, Name: "git-workflow", Description: "repeated shell family git",
		Evidence: []signal.Evidence{{Harness: "claude", SessionID: "abcdef123456"}},
	}
	once := MergeObserved(body, p)
	if !strings.Contains(once, ObservedHeading) || !strings.Contains(once, "- repeated shell family git (1 session; last claude abcdef12)") {
		t.Fatalf("first merge:\n%s", once)
	}
	if !strings.HasPrefix(once, "# Git Workflow\n\n## When\n\nUse when running git.\n\n## Steps\n\n1. Do it.") {
		t.Fatalf("original body altered:\n%s", once)
	}
	// Same proposal again: no growth.
	twice := MergeObserved(once, p)
	if twice != once {
		t.Fatalf("second merge changed body:\n%s\n---\n%s", once, twice)
	}
	// Same pattern, more evidence: bullet updated in place, not duplicated.
	p.Evidence = append(p.Evidence, signal.Evidence{Harness: "codex", SessionID: "zz"})
	third := MergeObserved(twice, p)
	if strings.Count(third, "repeated shell family git") != 1 {
		t.Fatalf("bullet duplicated:\n%s", third)
	}
	if !strings.Contains(third, "(2 sessions; last codex zz)") {
		t.Fatalf("count not refreshed:\n%s", third)
	}
	// A different pattern adds a second bullet.
	other := Proposal{Kind: KindUpdate, Name: "git-workflow", Description: "retry after error: Bash"}
	fourth := MergeObserved(third, other)
	if strings.Count(fourth, "\n- ") != 2 || strings.Count(fourth, ObservedHeading) != 1 {
		t.Fatalf("expected two bullets under one heading:\n%s", fourth)
	}
}

func TestMergeObserved_MigratesLegacyAppends(t *testing.T) {
	legacy := "# Skill\n\nbody\n\n## Observed later\n\nrepeated shell family git\n\n## Observed later\n\nretry after error: Bash\n\n## Notes\n\nkeep me\n"
	p := Proposal{Kind: KindUpdate, Name: "x", Description: "repeated shell family git"}
	got := MergeObserved(legacy, p)
	if strings.Contains(got, "## Observed later") {
		t.Fatalf("legacy heading survived:\n%s", got)
	}
	if strings.Count(got, ObservedHeading) != 1 || strings.Count(got, "\n- ") != 2 {
		t.Fatalf("expected one section with two bullets:\n%s", got)
	}
	if !strings.HasSuffix(got, "## Notes\n\nkeep me") {
		t.Fatalf("trailing section lost:\n%s", got)
	}
}

func TestApplyUpdate_TwiceDoesNotGrow(t *testing.T) {
	dir := t.TempDir()
	mgr := skill.ForAgents(dir)
	if err := mgr.Create(filepath.Join(dir, "skills"), "git-workflow", "Use when running git.", "# Git\n\nSteps here.\n"); err != nil {
		t.Fatal(err)
	}
	sig := signal.Signal{Kind: signal.KindShellFam, Key: "git", Message: "repeated shell family git",
		Evidence: []signal.Evidence{{Harness: "claude", SessionID: "s1"}}}
	props := FromSignals([]signal.Signal{sig}, mgr.All())
	if len(props) != 1 || props[0].Kind != KindUpdate {
		t.Fatalf("expected update proposal, got %#v", props)
	}
	if _, err := Apply(dir, props[0]); err != nil {
		t.Fatal(err)
	}
	first, _ := os.ReadFile(filepath.Join(dir, "skills", "git-workflow", "SKILL.md"))
	if _, err := Apply(dir, props[0]); err != nil {
		t.Fatal(err)
	}
	second, _ := os.ReadFile(filepath.Join(dir, "skills", "git-workflow", "SKILL.md"))
	if string(first) != string(second) {
		t.Fatalf("re-apply changed file:\n%s\n---\n%s", first, second)
	}
	if !strings.Contains(string(first), "Steps here.") || !strings.Contains(string(first), ObservedHeading) {
		t.Fatalf("unexpected body:\n%s", first)
	}
}

func TestLoadGapNotApplyable(t *testing.T) {
	p := Proposal{ID: "x", Kind: KindLoadGap, Name: "doc-writer"}
	if _, err := Apply(t.TempDir(), p); err == nil {
		t.Fatal("expected error")
	}
}
