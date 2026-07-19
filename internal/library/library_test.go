package library

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/abdul-hamid-achik/minerva/internal/profile"
	"github.com/abdul-hamid-achik/minerva/internal/skill"
	"github.com/abdul-hamid-achik/minerva/internal/templates"
)

func setupAgents(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	sm := skill.NewManagerWithState(dir, filepath.Join(dir, "skills"))
	if err := sm.Create(filepath.Join(dir, "skills"), "alpha", "desc", "body of alpha"); err != nil {
		t.Fatal(err)
	}
	pm := profile.NewManager(dir)
	if err := pm.Create(&profile.Profile{
		Name: "dev", SystemPrompt: "you are dev", Skills: []string{"alpha"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := templates.Save(templates.DefaultDir(dir), templates.Template{
		Name: "custom", Prompt: "custom role",
	}); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestExportImport_Directory(t *testing.T) {
	src := setupAgents(t)
	dest := filepath.Join(t.TempDir(), "bundle")
	res, err := Export(ExportOptions{AgentsDir: src, Dest: dest, IncludeTemplates: true})
	if err != nil {
		t.Fatal(err)
	}
	if res.Manifest.Skills != 1 || res.Manifest.Profiles != 1 || res.Manifest.Templates != 1 {
		t.Fatalf("manifest=%+v", res.Manifest)
	}
	if _, err := os.Stat(filepath.Join(dest, ManifestName)); err != nil {
		t.Fatal(err)
	}

	dstAgents := t.TempDir()
	imp, err := Import(ImportOptions{Source: dest, AgentsDir: dstAgents, Force: false, IncludeTemplates: true})
	if err != nil {
		t.Fatal(err)
	}
	if imp.Skills != 1 || imp.Profiles != 1 || imp.Templates != 1 {
		t.Fatalf("import=%+v", imp)
	}
	// skip on re-import without force
	imp2, err := Import(ImportOptions{Source: dest, AgentsDir: dstAgents, Force: false, IncludeTemplates: true})
	if err != nil {
		t.Fatal(err)
	}
	if imp2.Skipped < 1 {
		t.Fatalf("expected skips, got %+v", imp2)
	}
}

func TestExportImport_Tarball(t *testing.T) {
	src := setupAgents(t)
	dest := filepath.Join(t.TempDir(), "lib.tgz")
	if _, err := Export(ExportOptions{AgentsDir: src, Dest: dest, IncludeTemplates: true}); err != nil {
		t.Fatal(err)
	}
	dstAgents := t.TempDir()
	imp, err := Import(ImportOptions{Source: dest, AgentsDir: dstAgents, Force: true, IncludeTemplates: true})
	if err != nil {
		t.Fatal(err)
	}
	if imp.Skills != 1 {
		t.Fatalf("import=%+v", imp)
	}
}

func TestLint_MissingSkillAndSecret(t *testing.T) {
	dir := t.TempDir()
	sm := skill.NewManagerWithState(dir, filepath.Join(dir, "skills"))
	if err := sm.Create(filepath.Join(dir, "skills"), "ok", "desc", "safe body"); err != nil {
		t.Fatal(err)
	}
	// secret-like skill
	if err := sm.Create(filepath.Join(dir, "skills"), "bad", "desc", "api_key=sk-abcdefghijklmnopqrstuvwxyz"); err != nil {
		t.Fatal(err)
	}
	pm := profile.NewManager(dir)
	if err := pm.Create(&profile.Profile{
		Name: "dev", SystemPrompt: "hi", Skills: []string{"missing", "ok"},
	}); err != nil {
		t.Fatal(err)
	}
	// orphan skill ok is referenced; bad is orphan

	rep, err := Lint(dir)
	if err != nil {
		t.Fatal(err)
	}
	if rep.OK {
		t.Fatalf("expected errors: %+v", rep)
	}
	if rep.Errors == 0 {
		t.Fatalf("expected error issues: %+v", rep.Issues)
	}
	foundMissing := false
	foundSecret := false
	for _, i := range rep.Issues {
		if i.Kind == "missing-skill" {
			foundMissing = true
		}
		if i.Kind == "secret" {
			foundSecret = true
		}
	}
	if !foundMissing {
		t.Fatal("expected missing-skill issue")
	}
	if !foundSecret {
		t.Fatal("expected secret issue")
	}
}
