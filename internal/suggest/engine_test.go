package suggest

import (
	"os"
	"path/filepath"
	"strings"
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

func TestParseAddSkillsAction(t *testing.T) {
	p, skills, ok := parseAddSkillsAction("minerva profile add-skills dev qa-tester,doc-writer")
	if !ok || p != "dev" || len(skills) != 2 {
		t.Fatalf("got p=%q skills=%v ok=%v", p, skills, ok)
	}
	p, skills, ok = parseAddSkillsAction(`minerva profile add-skills "my profile" alpha`)
	if !ok || p != "my profile" || len(skills) != 1 || skills[0] != "alpha" {
		t.Fatalf("quoted: p=%q skills=%v ok=%v", p, skills, ok)
	}
}

func TestApplyAuto_AddsSkillsToProfile(t *testing.T) {
	dir := t.TempDir()
	skillsDir := filepath.Join(dir, "skills")
	sm := skill.NewManagerWithState(dir, skillsDir)
	if err := sm.Create(skillsDir, "alpha", "test skill", "body"); err != nil {
		t.Fatal(err)
	}
	pm := profile.NewManager(dir)
	if err := pm.Create(&profile.Profile{Name: "dev", Skills: []string{"beta"}, SystemPrompt: "hi"}); err != nil {
		t.Fatal(err)
	}

	suggestions := []Suggestion{{
		AutoApply: true,
		Action:    "minerva profile add-skills dev alpha",
		Message:   "add alpha",
	}}
	applied, skipped, err := ApplyAuto(sm, pm, suggestions, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(applied) != 1 {
		t.Fatalf("applied=%v skipped=%v", applied, skipped)
	}
	got := pm.Get("dev")
	found := false
	for _, s := range got.Skills {
		if s == "alpha" {
			found = true
		}
	}
	if !found {
		t.Fatalf("skills=%v missing alpha", got.Skills)
	}
}

func TestApplyAuto_ActivateRequiresLocalFlag(t *testing.T) {
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
	applied, skipped, err := ApplyAuto(mgr, nil, suggestions, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(applied) != 0 || len(skipped) != 1 {
		t.Fatalf("without --apply-local expected skip: applied=%v skipped=%v", applied, skipped)
	}
	if !strings.Contains(skipped[0], "--apply-local") {
		t.Fatalf("skip msg=%q", skipped[0])
	}

	applied, skipped, err = ApplyAuto(mgr, nil, suggestions, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(applied) != 1 || !mgr.IsActive("alpha") {
		t.Fatalf("applied=%v skipped=%v active=%v", applied, skipped, mgr.IsActive("alpha"))
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

func TestEngine_WorkspaceSuggestsProfileAdd(t *testing.T) {
	dir := t.TempDir()
	// Fake a go module workspace
	if err := osWrite(filepath.Join(dir, "go.mod"), "module example\n"); err != nil {
		t.Fatal(err)
	}
	skillsDir := filepath.Join(dir, "skills")
	sm := skill.NewManagerWithState(dir, skillsDir)
	for _, name := range []string{"software-architect", "qa-tester", "doc-writer"} {
		if err := sm.Create(skillsDir, name, name+" skill", "body"); err != nil {
			t.Fatal(err)
		}
	}
	pm := profile.NewManager(dir)
	if err := pm.Create(&profile.Profile{Name: "dev", SystemPrompt: "hi"}); err != nil {
		t.Fatal(err)
	}
	store := analytics.NewStore(dir)
	_ = store.Load()

	engine := NewEngine(sm, pm, store, dir)
	out := engine.Analyze()
	found := false
	for _, s := range out {
		if s.Category == "profile" && strings.Contains(s.Action, "profile add-skills") && s.AutoApply {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected auto-apply profile add-skills suggestion, got %#v", out)
	}
}

func osWrite(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o644)
}
