package harness

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCatalog_DetectsPresent(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".claude", "skills"), 0o755); err != nil {
		t.Fatal(err)
	}
	env := Env{Home: home, AgentsDir: filepath.Join(home, ".agents")}
	cat := Catalog(env)
	found := false
	for _, h := range cat {
		if h.ID == Claude && h.Present {
			found = true
		}
		if h.ID == Cursor && h.SyncWritable {
			t.Fatal("cursor skills tree must not be writable")
		}
	}
	if !found {
		t.Fatal("expected claude present")
	}
}

func TestGet(t *testing.T) {
	env := Env{Home: t.TempDir()}
	if Get(env, "codex") == nil {
		t.Fatal("codex")
	}
	if Get(env, "nope") != nil {
		t.Fatal("unknown")
	}
}
