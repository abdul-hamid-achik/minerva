package templates

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCatalog_DiskOverridesBuiltin(t *testing.T) {
	dir := t.TempDir()
	custom := Template{
		Name:        "architect",
		Description: "custom arch",
		Role:        "architect",
		Prompt:      "custom prompt",
	}
	if err := Save(dir, custom); err != nil {
		t.Fatal(err)
	}
	all, err := Catalog(dir)
	if err != nil {
		t.Fatal(err)
	}
	var found *Template
	for i := range all {
		if all[i].Name == "architect" {
			found = &all[i]
			break
		}
	}
	if found == nil {
		t.Fatal("architect missing")
	}
	if found.Source != "disk" || found.Prompt != "custom prompt" {
		t.Fatalf("got source=%s prompt=%q", found.Source, found.Prompt)
	}
}

func TestInstallBuiltin(t *testing.T) {
	dir := t.TempDir()
	tplt, err := InstallBuiltin(dir, "qa-engineer")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(tplt.Path); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadDir(dir)
	if err != nil || len(loaded) != 1 {
		t.Fatalf("loaded=%v err=%v", loaded, err)
	}
}

func TestSaveAndGetFrom(t *testing.T) {
	dir := t.TempDir()
	if err := Save(dir, Template{Name: "mine", Prompt: "hello", Skills: []string{"a"}}); err != nil {
		t.Fatal(err)
	}
	got := GetFrom("mine", dir)
	if got == nil || got.Prompt != "hello" {
		t.Fatalf("got=%v", got)
	}
	if GetFrom("nope", dir) != nil {
		t.Fatal("expected nil")
	}
	// path exists
	if _, err := os.Stat(filepath.Join(dir, "mine", "template.yaml")); err != nil {
		t.Fatal(err)
	}
}
