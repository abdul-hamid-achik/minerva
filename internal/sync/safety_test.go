package sync

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/minerva/internal/harness"
)

func TestSync_SkipsFlatSkill(t *testing.T) {
	home := t.TempDir()
	agents := filepath.Join(home, ".agents")
	skillsDir := filepath.Join(agents, "skills")
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillsDir, "flat.md"), []byte("---\nname: flat\n---\nbody\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	env := harness.Env{Home: home, AgentsDir: agents}
	acts, err := Sync(SyncOptions{Env: env, To: []string{harness.Claude}})
	if err != nil {
		t.Fatal(err)
	}
	if len(acts) != 1 || acts[0].Method != MethodSkip || acts[0].Done {
		t.Fatalf("flat skill not skipped: %#v", acts)
	}
	if _, err := os.Lstat(filepath.Join(home, ".claude", "skills", "flat")); !os.IsNotExist(err) {
		t.Fatalf("flat skill linked into harness: %v", err)
	}
}

func TestCopyDir_RefusesSymlinks(t *testing.T) {
	src := t.TempDir()
	secret := filepath.Join(t.TempDir(), "id_rsa")
	if err := os.WriteFile(secret, []byte("PRIVATE"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "SKILL.md"), []byte("body"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secret, filepath.Join(src, "leak.txt")); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(t.TempDir(), "out")
	if err := copyDir(src, dst); err == nil {
		t.Fatal("copyDir followed a symlink")
	}
	if data, err := os.ReadFile(filepath.Join(dst, "leak.txt")); err == nil {
		t.Fatalf("symlink target copied: %q", data)
	}
}

func TestInstall_RejectsTraversalSpec(t *testing.T) {
	for _, spec := range []string{"owner/repo/../../etc", "owner/../x", "owner/repo/./x", "owner//x"} {
		_, err := Install(t.TempDir(), spec, false)
		if err == nil || !strings.Contains(err.Error(), "invalid path element") {
			t.Errorf("Install(%q) = %v, want invalid path element", spec, err)
		}
	}
}
