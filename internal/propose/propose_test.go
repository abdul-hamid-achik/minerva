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

func TestLoadGapNotApplyable(t *testing.T) {
	p := Proposal{ID: "x", Kind: KindLoadGap, Name: "doc-writer"}
	if _, err := Apply(t.TempDir(), p); err == nil {
		t.Fatal("expected error")
	}
}
