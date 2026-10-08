package sync

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/minerva/internal/harness"
	"github.com/abdul-hamid-achik/minerva/internal/skill"
)

func newLibrary(t *testing.T, names ...string) (harness.Env, string) {
	t.Helper()
	home := t.TempDir()
	agents := filepath.Join(home, ".agents")
	mgr := skill.ForAgents(agents)
	for _, n := range names {
		if err := mgr.Create(filepath.Join(agents, "skills"), n, "desc", "body"); err != nil {
			t.Fatal(err)
		}
	}
	return harness.Env{Home: home, AgentsDir: agents}, home
}

func TestSync_TargetsAreValidated(t *testing.T) {
	env, _ := newLibrary(t, "demo")
	for _, to := range []string{"bogus", harness.Cursor} {
		if _, err := Sync(SyncOptions{Env: env, To: []string{to}}); err == nil {
			t.Errorf("--to %s accepted", to)
		}
	}
}

func TestSync_SkipsHarnessesThatAreNotInstalled(t *testing.T) {
	env, home := newLibrary(t, "demo")
	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Sync(SyncOptions{Env: env}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(home, ".claude", "skills", "demo")); err != nil {
		t.Fatalf("installed harness not synced: %v", err)
	}
	for _, d := range []string{".codex", ".gemini", ".copilot"} {
		if _, err := os.Stat(filepath.Join(home, d)); !os.IsNotExist(err) {
			t.Errorf("sync created ~/%s for a harness that is not installed", d)
		}
	}
}

func TestSync_KeepsForeignLinksAndCategoryFolders(t *testing.T) {
	env, home := newLibrary(t, "demo", "cat")
	skills := filepath.Join(home, ".claude", "skills")
	elsewhere := filepath.Join(home, "work", "demo-dev")
	if err := os.MkdirAll(elsewhere, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(skills, "cat", "nested-skill"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(elsewhere, filepath.Join(skills, "demo")); err != nil {
		t.Fatal(err)
	}
	// Without --force neither is touched; --force may replace the foreign
	// link but never the category folder.
	acts, err := Sync(SyncOptions{Env: env, To: []string{harness.Claude}})
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range acts {
		if a.Done {
			t.Fatalf("replaced something without --force: %+v", a)
		}
	}
	if got, _ := os.Readlink(filepath.Join(skills, "demo")); got != elsewhere {
		t.Fatalf("user link repointed to %q", got)
	}
	if _, err := Sync(SyncOptions{Env: env, To: []string{harness.Claude}, Force: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(skills, "cat", "nested-skill")); err != nil {
		t.Fatalf("--force removed a category folder: %v", err)
	}
}

func TestRemoveLinksAndDoctorBrokenLink(t *testing.T) {
	env, home := newLibrary(t, "demo")
	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Sync(SyncOptions{Env: env, To: []string{harness.Claude}}); err != nil {
		t.Fatal(err)
	}
	canonical := filepath.Join(env.AgentsDir, "skills", "demo")
	// A dangling link to a skill deleted outside Minerva is a broken link.
	if err := os.Symlink(filepath.Join(env.AgentsDir, "skills", "gone"), filepath.Join(home, ".claude", "skills", "gone")); err != nil {
		t.Fatal(err)
	}
	findings, err := Doctor(env)
	if err != nil {
		t.Fatal(err)
	}
	broken := false
	for _, f := range findings {
		broken = broken || (f.Skill == "gone" && f.Kind == "broken-link")
	}
	if !broken {
		t.Fatalf("dangling link not reported as broken-link: %+v", findings)
	}
	removed, err := RemoveLinks(env, canonical)
	if err != nil || len(removed) != 1 || !strings.HasSuffix(removed[0], filepath.Join(".claude", "skills", "demo")) {
		t.Fatalf("removed=%v err=%v", removed, err)
	}
}

func TestReplaceDir_FailedCopyKeepsExisting(t *testing.T) {
	parent := t.TempDir()
	dest := filepath.Join(parent, "skill")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dest, "SKILL.md"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	src := t.TempDir()
	if err := os.Symlink("/etc/hosts", filepath.Join(src, "leak")); err != nil {
		t.Fatal(err)
	}
	if err := replaceDir(src, dest); err == nil {
		t.Fatal("copy with a symlink succeeded")
	}
	if data, _ := os.ReadFile(filepath.Join(dest, "SKILL.md")); string(data) != "old" {
		t.Fatalf("existing skill lost: %q", data)
	}
	entries, _ := os.ReadDir(parent)
	if len(entries) != 1 {
		t.Fatalf("staging left behind: %v", entries)
	}
}
