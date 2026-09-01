package skill

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCreate_QuotesDescription(t *testing.T) {
	dir := t.TempDir()
	mgr := ForAgents(dir)
	desc := "does: things\nwith: colons"
	if err := mgr.Create(filepath.Join(dir, "skills"), "quoted", desc, "body here"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "skills", "quoted", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, "description:") {
		t.Fatalf("expected description in:\n%s", content)
	}
	if err := mgr.LoadAll(); err != nil {
		t.Fatalf("LoadAll after create: %v\nfile:\n%s", err, content)
	}
	if !mgr.Has("quoted") {
		t.Fatal("skill not found after reload")
	}
}

func TestUpdate_DescriptionAndContent(t *testing.T) {
	dir := t.TempDir()
	skillsDir := filepath.Join(dir, "skills")
	mgr := ForAgents(dir)
	if err := mgr.Create(skillsDir, "editme", "old desc", "old body"); err != nil {
		t.Fatal(err)
	}

	newDesc := "new desc"
	newBody := "new body content"
	if err := mgr.Update("editme", &newDesc, &newBody); err != nil {
		t.Fatal(err)
	}
	content, ok := mgr.Load("editme")
	if !ok || content != newBody {
		t.Fatalf("content=%q ok=%v", content, ok)
	}
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
	mgr := ForAgents(dir)
	if err := mgr.Create(skillsDir, "x", "d", "body"); err != nil {
		t.Fatal(err)
	}
	if err := mgr.Update("x", nil, nil); err == nil {
		t.Fatal("expected error for empty update")
	}
}
