package skill

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateName_SinglePathElement(t *testing.T) {
	for _, bad := range []string{".", "..", "../x", "a/b", `a\b`, "/abs", ".hidden"} {
		if err := ValidateName(bad); err == nil {
			t.Errorf("ValidateName(%q) = nil, want error", bad)
		}
	}
	for _, good := range []string{"go-workflow", "frontend-design", "a.b", "x_y"} {
		if err := ValidateName(good); err != nil {
			t.Errorf("ValidateName(%q) = %v, want nil", good, err)
		}
	}
}

func TestCreate_RejectsTraversal(t *testing.T) {
	dir := t.TempDir()
	skillsDir := filepath.Join(dir, "skills")
	mgr := ForAgents(dir)
	for _, name := range []string{"..", "../escaped", "a/b", "."} {
		if err := mgr.Create(skillsDir, name, "d", "body"); err == nil {
			t.Fatalf("Create(%q) succeeded", name)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "escaped")); !os.IsNotExist(err) {
		t.Fatalf("file written outside the library: %v", err)
	}
}

func TestCreate_DoesNotOverwriteMismatchedFolder(t *testing.T) {
	dir := t.TempDir()
	skillsDir := filepath.Join(dir, "skills")
	writeFile(t, filepath.Join(skillsDir, "foo", "SKILL.md"), "---\nname: foo-helper\n---\nPRECIOUS\n")
	mgr := ForAgents(dir)
	if err := mgr.LoadAll(); err != nil {
		t.Fatal(err)
	}
	if err := mgr.Create(skillsDir, "foo", "d", "replacement"); err == nil {
		t.Fatal("Create overwrote an existing skill folder")
	}
	data, _ := os.ReadFile(filepath.Join(skillsDir, "foo", "SKILL.md"))
	if string(data) != "---\nname: foo-helper\n---\nPRECIOUS\n" {
		t.Fatalf("existing skill changed:\n%s", data)
	}
}

func TestLoadAll_SkipsDotName(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "skills", "evil", "SKILL.md"), "---\nname: .\n---\nbody\n")
	mgr := ForAgents(dir)
	if err := mgr.LoadAll(); err != nil {
		t.Fatal(err)
	}
	if mgr.Has(".") || len(mgr.Problems()) != 1 {
		t.Fatalf("skills=%v problems=%v", mgr.All(), mgr.Problems())
	}
}

func TestDelete_UsesLoadedLocation(t *testing.T) {
	dir := t.TempDir()
	skillsDir := filepath.Join(dir, "skills")
	writeFile(t, filepath.Join(skillsDir, "folder", "SKILL.md"), "---\nname: other-name\n---\nbody\n")
	writeFile(t, filepath.Join(skillsDir, "keep", "SKILL.md"), "---\nname: keep\n---\nbody\n")
	writeFile(t, filepath.Join(skillsDir, "flat.md"), "---\nname: flat\n---\nbody\n")
	mgr := ForAgents(dir)
	if err := mgr.LoadAll(); err != nil {
		t.Fatal(err)
	}

	if err := mgr.Delete(skillsDir, "other-name"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(skillsDir, "folder")); !os.IsNotExist(err) {
		t.Fatalf("mismatched skill folder still present: %v", err)
	}

	if err := mgr.Delete(skillsDir, "flat"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(skillsDir, "flat.md")); !os.IsNotExist(err) {
		t.Fatalf("flat skill still present: %v", err)
	}
	if !mgr.Has("keep") {
		t.Fatal("deleting one skill removed another")
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadAll_OneBadSkillDoesNotHideTheRest(t *testing.T) {
	dir := t.TempDir()
	skills := filepath.Join(dir, "skills")
	writeFile(t, filepath.Join(skills, "good", "SKILL.md"), "---\nname: good\n---\nbody\n")
	writeFile(t, filepath.Join(skills, "broken", "SKILL.md"), "---\nname: [unclosed\n---\nbody\n")
	writeFile(t, filepath.Join(skills, "a-dup", "SKILL.md"), "---\nname: twin\n---\nfirst\n")
	writeFile(t, filepath.Join(skills, "b-dup", "SKILL.md"), "---\nname: twin\n---\nsecond\n")
	mgr := ForAgents(dir)
	if err := mgr.LoadAll(); err != nil {
		t.Fatal(err)
	}
	if !mgr.Has("good") || !mgr.Has("twin") || len(mgr.All()) != 2 {
		t.Fatalf("skills = %v", mgr.All())
	}
	if body, _ := mgr.Load("twin"); body != "first\n" && body != "first" {
		t.Fatalf("duplicate resolved to %q, want the first folder", body)
	}
	if len(mgr.Problems()) != 2 {
		t.Fatalf("problems = %v, want broken YAML and duplicate", mgr.Problems())
	}
}
