package sync

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/abdul-hamid-achik/minerva/internal/harness"
	"github.com/abdul-hamid-achik/minerva/internal/skill"
)

func TestSync_SymlinkAndDoctor(t *testing.T) {
	home := t.TempDir()
	agents := filepath.Join(home, ".agents")
	mgr := skill.ForAgents(agents)
	if err := mgr.Create(filepath.Join(agents, "skills"), "demo", "desc", "body"); err != nil {
		t.Fatal(err)
	}
	env := harness.Env{Home: home, AgentsDir: agents}
	acts, err := Sync(SyncOptions{Env: env, To: []string{harness.Claude}})
	if err != nil {
		t.Fatal(err)
	}
	if len(acts) != 1 || acts[0].Method != "symlink" || !acts[0].Done {
		t.Fatalf("%#v", acts)
	}
	target := filepath.Join(home, ".claude", "skills", "demo")
	st, err := os.Lstat(target)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode()&os.ModeSymlink == 0 {
		t.Fatal("expected symlink")
	}
	findings, err := Doctor(env)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range findings {
		if f.Harness == harness.Claude && (f.Kind == "missing" || f.Kind == "drift") {
			t.Fatalf("unexpected %+v", f)
		}
	}
}

func TestFolderHashStable(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	h1, err := FolderHash(dir)
	if err != nil {
		t.Fatal(err)
	}
	h2, err := FolderHash(dir)
	if err != nil {
		t.Fatal(err)
	}
	if h1 == "" || h1 != h2 {
		t.Fatalf("%s vs %s", h1, h2)
	}
}
