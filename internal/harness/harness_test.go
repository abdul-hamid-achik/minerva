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

func presentMap(env Env) map[string]Harness {
	out := map[string]Harness{}
	for _, h := range Catalog(env) {
		out[h.ID] = h
	}
	return out
}

func TestCatalog_HermesAndOMPShape(t *testing.T) {
	home := t.TempDir()
	env := Env{Home: home, AgentsDir: filepath.Join(home, ".agents")}
	cat := presentMap(env)

	hermes := cat[Hermes]
	if hermes.DisplayName != "Hermes Agent" || hermes.SkillsDir != filepath.Join(home, ".hermes", "skills") {
		t.Fatalf("hermes: %+v", hermes)
	}
	if !hermes.LinkSkills || !hermes.SyncWritable || hermes.Native {
		t.Fatalf("hermes must be a symlink-synced harness: %+v", hermes)
	}
	if len(hermes.SessionGlobs) == 0 {
		t.Fatal("hermes session glob missing")
	}

	omp := cat[OMP]
	if omp.DisplayName != "oh-my-pi (omp)" || omp.SkillsDir != filepath.Join(env.AgentsDir, "skills") {
		t.Fatalf("omp: %+v", omp)
	}
	if omp.LinkSkills || omp.SyncWritable || !omp.Native {
		t.Fatalf("omp is a native reader and must never be synced into: %+v", omp)
	}
}

func TestCatalog_NativeReadersNeedOwnPresence(t *testing.T) {
	home := t.TempDir()
	env := Env{Home: home, AgentsDir: filepath.Join(home, ".agents")}
	// The shared canonical tree exists, but neither omp nor sonar is installed.
	if err := os.MkdirAll(filepath.Join(env.AgentsDir, "skills"), 0o755); err != nil {
		t.Fatal(err)
	}
	cat := presentMap(env)
	for _, id := range []string{OMP, Sonar, Hermes} {
		if cat[id].Present {
			t.Fatalf("%s must not be present just because ~/.agents/skills exists", id)
		}
	}

	if err := os.MkdirAll(filepath.Join(home, ".omp", "agent"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(home, ".sonar"), 0o755); err != nil {
		t.Fatal(err)
	}
	cat = presentMap(env)
	if !cat[OMP].Present || !cat[Sonar].Present {
		t.Fatalf("omp=%v sonar=%v, want both present once their own dirs exist", cat[OMP].Present, cat[Sonar].Present)
	}
}

func TestCatalog_PresentFromSessionsOnly(t *testing.T) {
	home := t.TempDir()
	env := Env{Home: home, AgentsDir: filepath.Join(home, ".agents")}
	dir := filepath.Join(home, ".hermes", "sessions")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "20260101_000000_ab.jsonl"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !presentMap(env)[Hermes].Present {
		t.Fatal("hermes with sessions but no skills dir should be present")
	}
}

func TestGet_NewHarnesses(t *testing.T) {
	env := Env{Home: t.TempDir()}
	if Get(env, "Hermes") == nil || Get(env, "omp") == nil {
		t.Fatal("hermes and omp must resolve by id")
	}
}
