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
