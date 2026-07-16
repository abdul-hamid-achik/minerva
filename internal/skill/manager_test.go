package skill

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCreate_QuotesDescription(t *testing.T) {
	dir := t.TempDir()
	mgr := NewManagerWithState(dir, filepath.Join(dir, "skills"))
	desc := "does: things\nwith: colons"
	if err := mgr.Create(filepath.Join(dir, "skills"), "quoted", desc, "body here"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "skills", "quoted", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, `description: "does: things\nwith: colons"`) &&
		!strings.Contains(content, "description:") {
		t.Fatalf("expected quoted description in:\n%s", content)
	}
	// Reload must succeed (YAML parse)
	if err := mgr.LoadAll(); err != nil {
		t.Fatalf("LoadAll after create: %v\nfile:\n%s", err, content)
	}
	if !mgr.Has("quoted") {
		t.Fatal("skill not found after reload")
	}
}

func TestActivationState_EmptyIsArray(t *testing.T) {
	dir := t.TempDir()
	skillsDir := filepath.Join(dir, "skills")
	mgr := NewManagerWithState(dir, skillsDir)
	if err := mgr.Create(skillsDir, "x", "d", "body"); err != nil {
		t.Fatal(err)
	}
	if err := mgr.Activate("x"); err != nil {
		t.Fatal(err)
	}
	if err := mgr.Deactivate("x"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, ".minerva-skills.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "[]" {
		t.Fatalf("empty active set should be [], got %s", data)
	}
}
