package sync

import (
	"os"
	"path/filepath"
	"strings"
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

func TestSync_DryRunTouchesNothing(t *testing.T) {
	home := t.TempDir()
	agents := filepath.Join(home, ".agents")
	mgr := skill.ForAgents(agents)
	if err := mgr.Create(filepath.Join(agents, "skills"), "demo", "desc", "body"); err != nil {
		t.Fatal(err)
	}
	env := harness.Env{Home: home, AgentsDir: agents}
	acts, err := Sync(SyncOptions{Env: env, To: []string{harness.Claude}, DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(acts) != 1 || acts[0].Done {
		t.Fatalf("%#v", acts)
	}
	if _, err := os.Stat(filepath.Join(home, ".claude")); !os.IsNotExist(err) {
		t.Fatalf("dry-run created the harness dir: %v", err)
	}
}

func TestSync_DoesNotReplaceDivergedHarnessCopy(t *testing.T) {
	home := t.TempDir()
	agents := filepath.Join(home, ".agents")
	mgr := skill.ForAgents(agents)
	if err := mgr.Create(filepath.Join(agents, "skills"), "demo", "desc", "canonical body"); err != nil {
		t.Fatal(err)
	}
	// The user hand-wrote a different "demo" skill inside Claude's tree.
	local := filepath.Join(home, ".claude", "skills", "demo")
	if err := os.MkdirAll(local, 0o755); err != nil {
		t.Fatal(err)
	}
	theirs := []byte("---\nname: demo\n---\nmy own notes\n")
	if err := os.WriteFile(filepath.Join(local, "SKILL.md"), theirs, 0o644); err != nil {
		t.Fatal(err)
	}
	env := harness.Env{Home: home, AgentsDir: agents}

	acts, err := Sync(SyncOptions{Env: env, To: []string{harness.Claude}})
	if err != nil {
		t.Fatal(err)
	}
	if len(acts) != 1 || acts[0].Method != MethodSkip || acts[0].Done || acts[0].Reason != ReasonDiverged {
		t.Fatalf("expected skip with reason, got %#v", acts)
	}
	got, _ := os.ReadFile(filepath.Join(local, "SKILL.md"))
	if string(got) != string(theirs) {
		t.Fatal("harness copy was modified without --force")
	}

	// --force replaces it with a symlink.
	acts, err = Sync(SyncOptions{Env: env, To: []string{harness.Claude}, Force: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(acts) != 1 || acts[0].Method != MethodSymlink || !acts[0].Done {
		t.Fatalf("%#v", acts)
	}
	st, err := os.Lstat(local)
	if err != nil || st.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("expected symlink after --force: %v %v", st, err)
	}
}

func TestSync_ReplacesIdenticalCopyAndStaleLink(t *testing.T) {
	home := t.TempDir()
	agents := filepath.Join(home, ".agents")
	mgr := skill.ForAgents(agents)
	if err := mgr.Create(filepath.Join(agents, "skills"), "demo", "desc", "body"); err != nil {
		t.Fatal(err)
	}
	env := harness.Env{Home: home, AgentsDir: agents}
	// identical real copy → replaced (no skip)
	local := filepath.Join(home, ".claude", "skills", "demo")
	if err := copyDir(filepath.Join(agents, "skills", "demo"), local); err != nil {
		t.Fatal(err)
	}
	acts, err := Sync(SyncOptions{Env: env, To: []string{harness.Claude}})
	if err != nil || len(acts) != 1 || acts[0].Method != MethodSymlink || !acts[0].Done {
		t.Fatalf("%v %#v", err, acts)
	}
	// stale symlink → replaced
	_ = os.Remove(local)
	if err := os.Symlink("/nonexistent/skill", local); err != nil {
		t.Fatal(err)
	}
	acts, err = Sync(SyncOptions{Env: env, To: []string{harness.Claude}})
	if err != nil || len(acts) != 1 || !acts[0].Done {
		t.Fatalf("%v %#v", err, acts)
	}
	target, _ := os.Readlink(local)
	if target == "/nonexistent/skill" {
		t.Fatal("stale link kept")
	}
}

func TestDoctor_ReportsBrokenAndForeignLinks(t *testing.T) {
	home := t.TempDir()
	agents := filepath.Join(home, ".agents")
	mgr := skill.ForAgents(agents)
	if err := mgr.Create(filepath.Join(agents, "skills"), "demo", "desc", "body"); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(home, ".claude", "skills")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/nonexistent/demo", filepath.Join(dir, "demo")); err != nil {
		t.Fatal(err)
	}
	env := harness.Env{Home: home, AgentsDir: agents}
	findings, err := Doctor(env)
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[string]bool{}
	for _, f := range findings {
		if f.Harness == harness.Claude {
			kinds[f.Kind] = true
		}
	}
	if !kinds["broken-link"] {
		t.Fatalf("expected broken-link, got %#v", findings)
	}
}

func TestFindSkill_AmbiguousListsCandidates(t *testing.T) {
	repo := t.TempDir()
	for _, n := range []string{"alpha", "beta"} {
		dir := filepath.Join(repo, "skills", n)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: "+n+"\n---\nx"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, _, err := findSkill(repo, repo); err == nil || !strings.Contains(err.Error(), "skills/alpha") || !strings.Contains(err.Error(), "skills/beta") {
		t.Fatalf("expected ambiguity error listing both, got %v", err)
	}
	folder, name, _, err := findSkill(filepath.Join(repo, "skills", "beta"), repo)
	if err != nil || name != "beta" || filepath.Base(folder) != "beta" {
		t.Fatalf("%v %s %s", err, folder, name)
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

// seedHermesSkillsDir builds a ~/.hermes/skills with Hermes-owned entries
// next to (and instead of) real skills.
func seedHermesSkillsDir(t *testing.T, home string) string {
	t.Helper()
	dir := filepath.Join(home, ".hermes", "skills")
	owned := map[string]string{
		".archive/old/SKILL.md":       "archived\n",
		".curator_backups/b/SKILL.md": "backup\n",
		".bundled_manifest":           "manifest\n",
		".curator_ledger.jsonl":       "{}\n",
		"_shared/helper.md":           "shared\n",
		"creative/tool/SKILL.md":      "---\nname: tool\n---\nnested\n", // category folder
		"hermes-only/SKILL.md":        "---\nname: hermes-only\n---\nmine\n",
	}
	for rel, body := range owned {
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func hermesSnapshot(t *testing.T, dir string, rels ...string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, rel := range rels {
		b, err := os.ReadFile(filepath.Join(dir, rel))
		if err != nil {
			t.Fatal(err)
		}
		out[rel] = string(b)
	}
	return out
}

func TestHermes_SyncAndDoctorIgnoreHarnessOwnedEntries(t *testing.T) {
	home := t.TempDir()
	agents := filepath.Join(home, ".agents")
	if err := skill.ForAgents(agents).Create(filepath.Join(agents, "skills"), "demo", "desc", "body"); err != nil {
		t.Fatal(err)
	}
	dir := seedHermesSkillsDir(t, home)
	keep := []string{".archive/old/SKILL.md", ".curator_backups/b/SKILL.md", ".bundled_manifest", ".curator_ledger.jsonl", "_shared/helper.md", "creative/tool/SKILL.md", "hermes-only/SKILL.md"}
	before := hermesSnapshot(t, dir, keep...)
	env := harness.Env{Home: home, AgentsDir: agents}

	acts, err := Sync(SyncOptions{Env: env, To: []string{harness.Hermes}})
	if err != nil {
		t.Fatal(err)
	}
	if len(acts) != 1 || acts[0].Skill != "demo" || acts[0].Method != MethodSymlink || !acts[0].Done {
		t.Fatalf("expected a single symlink of demo, got %#v", acts)
	}
	if st, err := os.Lstat(filepath.Join(dir, "demo")); err != nil || st.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("demo should be a symlink: %v %v", st, err)
	}
	after := hermesSnapshot(t, dir, keep...)
	for rel, body := range before {
		if after[rel] != body {
			t.Fatalf("%s was modified by sync", rel)
		}
	}

	findings, err := Doctor(env)
	if err != nil {
		t.Fatal(err)
	}
	var extras []string
	for _, f := range findings {
		if f.Harness != harness.Hermes {
			continue
		}
		if f.Kind == "extra" {
			extras = append(extras, f.Skill)
			continue
		}
		t.Fatalf("unexpected finding %+v", f)
	}
	// Only the real Hermes-authored skill is "extra"; dot-entries, _shared and
	// the creative/ category folder are not skills.
	if len(extras) != 1 || extras[0] != "hermes-only" {
		t.Fatalf("extras=%v", extras)
	}
}

func TestHermes_DivergedSkillKeptWithoutForce(t *testing.T) {
	home := t.TempDir()
	agents := filepath.Join(home, ".agents")
	if err := skill.ForAgents(agents).Create(filepath.Join(agents, "skills"), "hermes-only", "desc", "canonical"); err != nil {
		t.Fatal(err)
	}
	dir := seedHermesSkillsDir(t, home)
	env := harness.Env{Home: home, AgentsDir: agents}

	acts, err := Sync(SyncOptions{Env: env, To: []string{harness.Hermes}})
	if err != nil {
		t.Fatal(err)
	}
	if len(acts) != 1 || acts[0].Method != MethodSkip || acts[0].Reason != ReasonDiverged {
		t.Fatalf("expected diverged skip, got %#v", acts)
	}
	got := hermesSnapshot(t, dir, "hermes-only/SKILL.md")
	if got["hermes-only/SKILL.md"] != "---\nname: hermes-only\n---\nmine\n" {
		t.Fatal("hermes-authored skill was overwritten without --force")
	}
	if _, err := os.Stat(filepath.Join(dir, ".archive", "old", "SKILL.md")); err != nil {
		t.Fatalf("archive touched: %v", err)
	}

	// --force replaces only that same-named skill.
	if _, err := Sync(SyncOptions{Env: env, To: []string{harness.Hermes}, Force: true}); err != nil {
		t.Fatal(err)
	}
	if st, err := os.Lstat(filepath.Join(dir, "hermes-only")); err != nil || st.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("expected symlink after --force: %v %v", st, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "_shared", "helper.md")); err != nil {
		t.Fatalf("_shared touched: %v", err)
	}
}

func TestSync_NeverWritesNativeHarnesses(t *testing.T) {
	home := t.TempDir()
	agents := filepath.Join(home, ".agents")
	if err := skill.ForAgents(agents).Create(filepath.Join(agents, "skills"), "demo", "desc", "body"); err != nil {
		t.Fatal(err)
	}
	env := harness.Env{Home: home, AgentsDir: agents}
	acts, err := Sync(SyncOptions{Env: env, To: []string{harness.OMP, harness.Sonar}})
	if err != nil {
		t.Fatal(err)
	}
	if len(acts) != 0 {
		t.Fatalf("native readers must not be synced into: %#v", acts)
	}
	if _, err := os.Stat(filepath.Join(home, ".omp")); !os.IsNotExist(err) {
		t.Fatalf("sync created ~/.omp: %v", err)
	}
	findings, err := Doctor(env)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range findings {
		if f.Harness == harness.OMP || f.Harness == harness.Sonar {
			t.Fatalf("native harness produced a finding: %+v", f)
		}
	}
}
