package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/abdul-hamid-achik/minerva/internal/harness"
)

func TestParseJSONL_SkipsOversizedLine(t *testing.T) {
	huge := `{"type":"user","message":{"role":"user","content":"` + strings.Repeat("x", maxJSONLLine+1) + `"}}`
	path := writeLines(t, "big.jsonl",
		`{"type":"user","message":{"role":"user","content":"before"}}`,
		huge,
		`{"type":"user","message":{"role":"user","content":"after"}}`,
	)
	s, err := Load(Session{Harness: harness.Claude, Path: path})
	if err != nil {
		t.Fatalf("an oversized line failed the session: %v", err)
	}
	if len(s.Turns) != 2 || s.Turns[0].Text != "before" || s.Turns[1].Text != "after" {
		t.Fatalf("turns = %+v", s.Turns)
	}
}

func TestParseJSONL_LastLineWithoutNewline(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.jsonl")
	writeFileT(t, path, `{"type":"user","message":{"role":"user","content":"only"}}`)
	s, err := Load(Session{Harness: harness.Claude, Path: path})
	if err != nil || len(s.Turns) != 1 {
		t.Fatalf("turns=%v err=%v", s.Turns, err)
	}
}

// claudeHome writes one Claude transcript per workspace, the first one
// newest, and returns the env.
func claudeHome(t *testing.T, workspaces ...string) harness.Env {
	t.Helper()
	home := t.TempDir()
	now := time.Now()
	for i, ws := range workspaces {
		dir := filepath.Join(home, ".claude", "projects", "p"+string(rune('a'+i)))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(dir, "s"+string(rune('a'+i))+".jsonl")
		writeFileT(t, path, `{"cwd":"`+ws+`","type":"user","message":{"role":"user","content":"hi"}}`+"\n")
		mtime := now.Add(-time.Duration(i) * time.Hour)
		if err := os.Chtimes(path, mtime, mtime); err != nil {
			t.Fatal(err)
		}
	}
	return harness.Env{Home: home, AgentsDir: filepath.Join(home, ".agents"), Now: now}
}

func TestWorkspaceFilter_AppliesBeforeLimit(t *testing.T) {
	env := claudeHome(t, "/work/a", "/work/a", "/work/b")
	got, err := LoadFiltered(env, Filter{Harness: harness.Claude, Workspace: "/work/b", Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Workspace != "/work/b" {
		t.Fatalf("got %+v, want the older /work/b session", got)
	}
	listed, err := List(env, Filter{Harness: harness.Claude, Workspace: "/work/a"})
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 2 || listed[0].Turns != nil {
		t.Fatalf("List with workspace = %+v, want 2 metadata-only sessions", listed)
	}
}

func TestMatchesSessionID(t *testing.T) {
	codex := Session{ID: "rollout-2026-10-07T11-58-20-01a11784-2ad8", Path: "/s/2026/10/07/rollout-2026-10-07T11-58-20-01a11784-2ad8.jsonl"}
	if !matchesSessionID(codex, "01a11784") {
		t.Fatal("codex uuid prefix did not match")
	}
	inDir := Session{ID: "zzz", Path: "/home/.claude/projects/proj-abc/zzz.jsonl"}
	if matchesSessionID(inDir, "abc") {
		t.Fatal("a directory name matched the session id")
	}
}

func TestCursorWorkspace(t *testing.T) {
	path := "/Users/me/projects/home/.cursor/projects/Users-me-projects-my-app/agent-transcripts/u1/u1.jsonl"
	slug := inferCursorWorkspace(path)
	if slug != "Users-me-projects-my-app" {
		t.Fatalf("slug = %q", slug)
	}
	for _, want := range []string{"/Users/me/projects/my-app", "/users/me/projects/my-app/", "my-app"} {
		if !workspaceMatch(slug, want) {
			t.Errorf("workspaceMatch(%q, %q) = false", slug, want)
		}
	}
	if workspaceMatch(slug, "/Users/me/projects/app") {
		t.Error("a different project matched")
	}
}

func TestParseSince_Strict(t *testing.T) {
	for in, want := range map[string]time.Duration{"": 0, "24h": 24 * time.Hour, "7d": 7 * 24 * time.Hour, "0": 0} {
		got, err := ParseSince(in)
		if err != nil || got != want {
			t.Errorf("ParseSince(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	for _, bad := range []string{"-24h", "-1d", "1.5d", "7xd", "99999999999d", "garbage"} {
		if _, err := ParseSince(bad); err == nil {
			t.Errorf("ParseSince(%q) accepted", bad)
		}
	}
}

func TestCodex_ModelFromTurnContext(t *testing.T) {
	path := writeLines(t, "rollout.jsonl",
		`{"type":"session_meta","payload":{"session_id":"x","model_provider":"openai","cwd":"/w"}}`,
		`{"type":"turn_context","payload":{"model":"gpt-5-codex","cwd":"/w"}}`,
	)
	s, err := Load(Session{Harness: harness.Codex, Path: path})
	if err != nil || s.Model != "gpt-5-codex" {
		t.Fatalf("model = %q, err = %v", s.Model, err)
	}
}

func TestStubWorkspace_SkipsOtherProjectsWithoutParsing(t *testing.T) {
	home := t.TempDir()
	write := func(dir, cwd string) {
		p := filepath.Join(home, ".claude", "projects", dir)
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatal(err)
		}
		writeFileT(t, filepath.Join(p, "s.jsonl"), `{"cwd":"`+cwd+`","type":"user","message":{"role":"user","content":"hi"}}`+"\n")
	}
	write("-work-a", "/work/a")
	write("-work-b", "/work/b")
	if ws := stubWorkspace(Session{Harness: harness.Claude, Path: filepath.Join(home, ".claude", "projects", "-work-a", "s.jsonl")}); !workspaceMatch(ws, "/work/a") || workspaceMatch(ws, "/work/b") {
		t.Fatalf("stub workspace %q", ws)
	}
	if ws := stubWorkspace(Session{Harness: harness.Claude, Path: "/x/.claude/projects/custom/s.jsonl"}); ws != "" {
		t.Fatalf("a dir that is not a path slug was trusted: %q", ws)
	}
	env := harness.Env{Home: home, AgentsDir: filepath.Join(home, ".agents"), Now: time.Now()}
	got, err := LoadFiltered(env, Filter{Harness: harness.Claude, Workspace: "/work/b"})
	if err != nil || len(got) != 1 || got[0].Workspace != "/work/b" {
		t.Fatalf("got %+v, err %v", got, err)
	}
}
