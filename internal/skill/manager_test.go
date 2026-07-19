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

func TestUpdate_DescriptionAndContent(t *testing.T) {
	dir := t.TempDir()
	skillsDir := filepath.Join(dir, "skills")
	mgr := NewManagerWithState(dir, skillsDir)
	if err := mgr.Create(skillsDir, "editme", "old desc", "old body"); err != nil {
		t.Fatal(err)
	}
	if err := mgr.Activate("editme"); err != nil {
		t.Fatal(err)
	}

	newDesc := "new desc"
	newBody := "new body content"
	if err := mgr.Update("editme", &newDesc, &newBody); err != nil {
		t.Fatal(err)
	}
	if !mgr.IsActive("editme") {
		t.Fatal("activation should survive update")
	}
	content, ok := mgr.Load("editme")
	if !ok || content != newBody {
		t.Fatalf("content=%q ok=%v", content, ok)
	}
	// Description lives in catalog
	found := false
	for _, e := range mgr.Catalog() {
		if e.Name == "editme" {
			found = true
			if e.Description != newDesc {
				t.Fatalf("desc=%q", e.Description)
			}
		}
	}
	if !found {
		t.Fatal("skill missing from catalog")
	}

	// Partial update: description only
	onlyDesc := "desc only"
	if err := mgr.Update("editme", &onlyDesc, nil); err != nil {
		t.Fatal(err)
	}
	content, _ = mgr.Load("editme")
	if content != newBody {
		t.Fatalf("body should be unchanged, got %q", content)
	}
}

func TestUpdate_NothingToUpdate(t *testing.T) {
	dir := t.TempDir()
	skillsDir := filepath.Join(dir, "skills")
	mgr := NewManagerWithState(dir, skillsDir)
	if err := mgr.Create(skillsDir, "x", "d", "body"); err != nil {
		t.Fatal(err)
	}
	if err := mgr.Update("x", nil, nil); err == nil {
		t.Fatal("expected error for empty update")
	}
}
