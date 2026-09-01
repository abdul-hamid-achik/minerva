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
