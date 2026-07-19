package profile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAddSkills_Merges(t *testing.T) {
	dir := t.TempDir()
	mgr := NewManager(dir)
	p := &Profile{Name: "dev", Skills: []string{"a", "b"}, SystemPrompt: "hi"}
	if err := mgr.Create(p); err != nil {
		t.Fatal(err)
	}
	if err := mgr.AddSkills("dev", []string{"b", "c"}); err != nil {
		t.Fatal(err)
	}
	// Reload from disk to ensure persistence
	mgr2 := NewManager(dir)
	if err := mgr2.LoadAll(); err != nil {
		t.Fatal(err)
	}
	got := mgr2.Get("dev")
	if got == nil {
		t.Fatal("missing profile")
	}
	if len(got.Skills) != 3 {
		t.Fatalf("skills=%v want a,b,c", got.Skills)
	}
	want := map[string]bool{"a": true, "b": true, "c": true}
	for _, s := range got.Skills {
		if !want[s] {
			t.Fatalf("unexpected skill %q in %v", s, got.Skills)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "agents", "dev", "agent.yaml")); err != nil {
		t.Fatal(err)
	}
}

func TestRemoveSkills(t *testing.T) {
	dir := t.TempDir()
	mgr := NewManager(dir)
	if err := mgr.Create(&Profile{Name: "dev", Skills: []string{"a", "b", "c"}, SystemPrompt: "hi"}); err != nil {
		t.Fatal(err)
	}
	if err := mgr.RemoveSkills("dev", []string{"b"}); err != nil {
		t.Fatal(err)
	}
	got := mgr.Get("dev")
	if len(got.Skills) != 2 {
		t.Fatalf("skills=%v", got.Skills)
	}
	for _, s := range got.Skills {
		if s == "b" {
			t.Fatal("b should be removed")
		}
	}
}

func TestUpdateModelAndMCPAndDescription(t *testing.T) {
	dir := t.TempDir()
	mgr := NewManager(dir)
	if err := mgr.Create(&Profile{Name: "dev", SystemPrompt: "hi"}); err != nil {
		t.Fatal(err)
	}
	if err := mgr.UpdateModel("dev", "gpt-test"); err != nil {
		t.Fatal(err)
	}
	if err := mgr.UpdateMCPServers("dev", []string{"mcphub", "minerva", "mcphub"}); err != nil {
		t.Fatal(err)
	}
	if err := mgr.UpdateDescription("dev", "developer profile"); err != nil {
		t.Fatal(err)
	}

	mgr2 := NewManager(dir)
	if err := mgr2.LoadAll(); err != nil {
		t.Fatal(err)
	}
	got := mgr2.Get("dev")
	if got.Model != "gpt-test" {
		t.Fatalf("model=%q", got.Model)
	}
	if got.Description != "developer profile" {
		t.Fatalf("desc=%q", got.Description)
	}
	if len(got.MCPServers) != 2 || got.MCPServers[0] != "mcphub" || got.MCPServers[1] != "minerva" {
		t.Fatalf("mcp=%v", got.MCPServers)
	}
}

func TestLoadAll_InvalidYAML_RecordsWarning(t *testing.T) {
	dir := t.TempDir()
	badDir := filepath.Join(dir, "agents", "broken")
	if err := os.MkdirAll(badDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(badDir, "agent.yaml"), []byte(":\n  - not: valid: yaml: ["), 0o644); err != nil {
		t.Fatal(err)
	}
	// Also create a valid profile so LoadAll has mixed results.
	mgr := NewManager(dir)
	if err := mgr.Create(&Profile{Name: "ok", SystemPrompt: "hi"}); err != nil {
		t.Fatal(err)
	}
	if err := mgr.LoadAll(); err != nil {
		t.Fatal(err)
	}
	if mgr.Get("ok") == nil {
		t.Fatal("valid profile missing")
	}
	if mgr.Get("broken") != nil {
		t.Fatal("broken profile should not load")
	}
	warns := mgr.Warnings()
	if len(warns) == 0 {
		t.Fatal("expected warning for broken YAML")
	}
	if warns[0].Dir != "broken" {
		t.Fatalf("warning dir=%q", warns[0].Dir)
	}
}
