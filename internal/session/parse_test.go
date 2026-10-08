package session

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/abdul-hamid-achik/minerva/internal/harness"
)

func TestParseClaude(t *testing.T) {
	s, err := Load(Session{Harness: harness.Claude, ID: "x", Path: filepath.Join("testdata", "claude.jsonl")})
	if err != nil {
		t.Fatal(err)
	}
	summarize(&s)
	if s.Workspace != "/Users/abdulachik/projects/bob" {
		t.Fatalf("workspace=%q", s.Workspace)
	}
	if s.ToolCount < 3 {
		t.Fatalf("tools=%d turns=%d", s.ToolCount, s.TurnCount)
	}
	if len(s.Skills) == 0 {
		t.Fatalf("expected skill invoke, turns=%#v", s.Turns)
	}
	if s.FirstUserPrompt() == "" {
		t.Fatal("missing user prompt")
	}
}

func TestParseCursor(t *testing.T) {
	s, err := Load(Session{Harness: harness.Cursor, Path: filepath.Join("testdata", "cursor.jsonl")})
	if err != nil {
		t.Fatal(err)
	}
	summarize(&s)
	if s.ToolCount < 2 {
		t.Fatalf("tools=%d", s.ToolCount)
	}
}

func TestParseCodex(t *testing.T) {
	s, err := Load(Session{Harness: harness.Codex, Path: filepath.Join("testdata", "codex.jsonl")})
	if err != nil {
		t.Fatal(err)
	}
	summarize(&s)
	if s.Workspace != "/Users/abdulachik/projects/local-agent" {
		t.Fatalf("workspace=%q", s.Workspace)
	}
	if s.ToolCount < 2 {
		t.Fatalf("tools=%d", s.ToolCount)
	}
}

func TestParseOpenCode(t *testing.T) {
	s, err := Load(Session{Harness: harness.OpenCode, Path: filepath.Join("testdata", "opencode", "storage", "session", "proj", "ses_demo.json")})
	if err != nil {
		t.Fatal(err)
	}
	summarize(&s)
	if s.Workspace != "/tmp/opencode-demo" {
		t.Fatalf("workspace=%q", s.Workspace)
	}
	if s.FirstUserPrompt() == "" {
		t.Fatal("missing user prompt")
	}
	if s.ToolCount < 1 {
		t.Fatalf("tools=%d turns=%#v", s.ToolCount, s.Turns)
	}
}

func TestParseCopilot(t *testing.T) {
	s, err := Load(Session{Harness: harness.Copilot, Path: filepath.Join("testdata", "copilot")})
	if err != nil {
		t.Fatal(err)
	}
	summarize(&s)
	if s.Workspace != "/tmp/copilot-demo" {
		t.Fatalf("workspace=%q", s.Workspace)
	}
	if s.FirstUserPrompt() == "" {
		t.Fatal("missing user prompt")
	}
	if s.ToolCount < 1 {
		t.Fatalf("tools=%d turns=%#v", s.ToolCount, s.Turns)
	}
}

func TestParseGemini(t *testing.T) {
	path := filepath.Join("testdata", "gemini", "tmp", "abc123", "chats", "session-2026-09-01T10-00-demo.jsonl")
	s, err := Load(Session{Harness: harness.Gemini, Path: path})
	if err != nil {
		t.Fatal(err)
	}
	summarize(&s)
	if s.ID != "gem-demo-1" {
		t.Fatalf("id=%q", s.ID)
	}
	if s.Workspace != "/tmp/gemini-demo" {
		t.Fatalf("workspace=%q (projects.json reverse map)", s.Workspace)
	}
	if s.StartedAt.IsZero() {
		t.Fatal("startedAt not parsed")
	}
	if s.FirstUserPrompt() != "add a changelog entry for the release" {
		t.Fatalf("prompt=%q", s.FirstUserPrompt())
	}
	// m1 user, m2 gemini (latest revision), m4 gemini from $set, m5 user from $push; info skipped.
	if s.TurnCount != 4 {
		t.Fatalf("turns=%d %#v", s.TurnCount, s.Turns)
	}
	if s.ToolCount != 3 {
		t.Fatalf("tools=%d %#v", s.ToolCount, s.Turns)
	}
	var sawErr, sawShell bool
	for _, turn := range s.Turns {
		for _, tc := range turn.ToolCalls {
			if tc.Name == "replace" && tc.IsError {
				sawErr = true
			}
			if tc.Name == "run_shell_command" && tc.Category == CatShell && tc.Command == "git" {
				sawShell = true
			}
		}
	}
	if !sawErr || !sawShell {
		t.Fatalf("err=%v shell=%v turns=%#v", sawErr, sawShell, s.Turns)
	}
	// The revised m2 must carry the final text, not the pending one.
	if s.Turns[1].Text != "Reading the changelog." {
		t.Fatalf("m2 text=%q", s.Turns[1].Text)
	}
}

func TestList_FilterHarness(t *testing.T) {
	home := t.TempDir()
	proj := filepath.Join(home, ".claude", "projects", "demo")
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	src, err := os.ReadFile(filepath.Join("testdata", "claude.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proj, "abc.jsonl"), src, 0o644); err != nil {
		t.Fatal(err)
	}
	env := harness.Env{Home: home, AgentsDir: filepath.Join(home, ".agents")}
	got, err := List(env, Filter{Harness: harness.Claude, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d", len(got))
	}
}

func TestParseHermes(t *testing.T) {
	s, err := Load(Session{Harness: harness.Hermes, Path: filepath.Join("testdata", "hermes", "sess-hermes-1.jsonl")})
	if err != nil {
		t.Fatal(err)
	}
	summarize(&s)
	if s.ID != "sess-hermes-1" || s.Model != "test-model" || s.StartedAt.IsZero() {
		t.Fatalf("meta: id=%q model=%q start=%v", s.ID, s.Model, s.StartedAt)
	}
	if s.TurnCount != 3 || s.ToolCount != 2 {
		t.Fatalf("turns=%d tools=%d: %#v", s.TurnCount, s.ToolCount, s.Turns)
	}
	if s.FirstUserPrompt() != "run the tests and load the demo skill" {
		t.Fatalf("prompt=%q", s.FirstUserPrompt())
	}
	sh := s.Turns[1].ToolCalls[0]
	if sh.Category != CatShell || sh.Command != "go" || !sh.IsError || sh.Result == "" {
		t.Fatalf("shell call: %#v", sh)
	}
	if sk := s.Turns[1].ToolCalls[1]; sk.IsError || sk.Category != CatSkill {
		t.Fatalf("skill call: %#v", sk)
	}
	if len(s.Skills) != 1 || s.Skills[0] != "demo" {
		t.Fatalf("skills=%v", s.Skills)
	}
}

func TestParseOMP(t *testing.T) {
	s, err := Load(Session{Harness: harness.OMP, Path: filepath.Join("testdata", "omp", "session.jsonl")})
	if err != nil {
		t.Fatal(err)
	}
	summarize(&s)
	if s.ID != "omp-sess-1" || s.Workspace != "/work/demo" || s.Model != "claude-test" || s.StartedAt.IsZero() {
		t.Fatalf("meta: id=%q ws=%q model=%q", s.ID, s.Workspace, s.Model)
	}
	// user, assistant(2 tool calls), assistant("done"); the empty error turn is dropped.
	if s.TurnCount != 3 || s.ToolCount != 2 {
		t.Fatalf("turns=%d tools=%d: %#v", s.TurnCount, s.ToolCount, s.Turns)
	}
	if s.FirstUserPrompt() != "fix the failing tests" {
		t.Fatalf("prompt=%q", s.FirstUserPrompt())
	}
	asst := s.Turns[1]
	if asst.Text != "running" {
		t.Fatalf("thinking must not leak into text: %q", asst.Text)
	}
	sh := asst.ToolCalls[0]
	if sh.Category != CatShell || sh.Command != "go" || !sh.IsError || sh.Result != "FAIL" {
		t.Fatalf("shell call: %#v", sh)
	}
	if rd := asst.ToolCalls[1]; rd.Category != CatRead || rd.IsError {
		t.Fatalf("read call: %#v", rd)
	}
}

func TestList_HermesAndOMPGlobs(t *testing.T) {
	home := t.TempDir()
	hdir := filepath.Join(home, ".hermes", "sessions")
	odir := filepath.Join(home, ".omp", "agent", "sessions", "-work-demo")
	for _, d := range []string{hdir, filepath.Join(odir, "sub-artifacts")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	copyFile := func(src, dst string) {
		b, err := os.ReadFile(src)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(dst, b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	copyFile(filepath.Join("testdata", "hermes", "sess-hermes-1.jsonl"), filepath.Join(hdir, "20260718_100000_abc.jsonl"))
	// Not transcripts: legacy snapshot, index, request dump.
	for _, n := range []string{"session_x.json", "sessions.json", "request_dump_x.json"} {
		if err := os.WriteFile(filepath.Join(hdir, n), []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	copyFile(filepath.Join("testdata", "omp", "session.jsonl"), filepath.Join(odir, "2026-07-18T10-00-00-000Z_abc.jsonl"))
	if err := os.WriteFile(filepath.Join(odir, "sub-artifacts", "1.bash-original.log"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	env := harness.Env{Home: home, AgentsDir: filepath.Join(home, ".agents")}
	for _, id := range []string{harness.Hermes, harness.OMP} {
		got, err := List(env, Filter{Harness: id, Limit: 10})
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 || got[0].Harness != id {
			t.Fatalf("%s: got %#v", id, got)
		}
	}
}
