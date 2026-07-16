package suggest

import (
	"path/filepath"
	"testing"

	"github.com/abdul-hamid-achik/minerva/internal/analytics"
	"github.com/abdul-hamid-achik/minerva/internal/profile"
	"github.com/abdul-hamid-achik/minerva/internal/skill"
)

func TestParseActivateAction(t *testing.T) {
	name, ok := parseActivateAction("minerva skill activate doc-writer")
	if !ok || name != "doc-writer" {
		t.Fatalf("got %q %v", name, ok)
	}
	if _, ok := parseActivateAction("minerva profile create x"); ok {
		t.Fatal("should reject non-activate")
	}
}

func TestApplyAuto_ActivatesSkill(t *testing.T) {
	dir := t.TempDir()
	skillsDir := filepath.Join(dir, "skills")
	mgr := skill.NewManagerWithState(dir, skillsDir)
	if err := mgr.Create(skillsDir, "alpha", "test skill", "body"); err != nil {
		t.Fatal(err)
	}
	if err := mgr.LoadAll(); err != nil {
		t.Fatal(err)
	}

	suggestions := []Suggestion{{
		AutoApply: true,
		Action:    "minerva skill activate alpha",
		Message:   "activate alpha",
	}}
	applied, skipped, err := ApplyAuto(mgr, suggestions)
	if err != nil {
		t.Fatal(err)
	}
	if len(applied) != 1 || applied[0] != "alpha" {
		t.Fatalf("applied=%v skipped=%v", applied, skipped)
	}
	if !mgr.IsActive("alpha") {
		t.Fatal("skill not active after apply")
	}
}

func TestEngine_Analyze_EmptyIsOK(t *testing.T) {
	dir := t.TempDir()
	sm := skill.NewManagerWithState(dir, filepath.Join(dir, "skills"))
	_ = sm.LoadAll()
	pm := profile.NewManager(dir)
	_ = pm.LoadAll()
	store := analytics.NewStore(dir)
	_ = store.Load()

	engine := NewEngine(sm, pm, store, dir)
	out := engine.Analyze()
	if len(out) == 0 {
		t.Fatal("expected at least one suggestion or general ok")
	}
}
