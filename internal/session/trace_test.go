package session

import (
	"strings"
	"testing"
)

func TestShellCommand(t *testing.T) {
	cases := map[string]string{
		`{"command":"git status --short"}`:                      "git",
		`{"command":"cd /tmp/proj && go test ./..."}`:           "go",
		`{"command":"FOO=1 BAR=2 npm run build"}`:               "npm",
		`{"command":"sudo -E docker compose up"}`:               "docker",
		`{"command":"/usr/bin/python3 -m pytest"}`:              "python",
		`{"command":"npx vitest run"}`:                          "npm",
		`{"command":"ls -la | head -n 5"}`:                      "",
		`{"command":"echo hi; cat README.md"}`:                  "",
		`{"command":"bash -lc \"cd x && cargo build\""}`:        "cargo",
		`{"cmd":["git","commit","-m","x"]}`:                     "git",
		`{"description":"run python","command":"go vet ./..."}`: "go",
		`go build ./cmd/minerva`:                                "go",
		``:                                                      "",
		`{"path":"/tmp"}`:                                       "",
		`{"command":"# rebuild\ngo build ./..."}`:               "go",
		`{"command":"for f in *.go; do gofmt -l $f; done"}`:     "gofmt",
		`{"command":"kill -9 1234"}`:                            "",
	}
	for in, want := range cases {
		if got := ShellCommand(in); got != want {
			t.Errorf("ShellCommand(%s) = %q, want %q", in, got, want)
		}
	}
}

func TestCategorize(t *testing.T) {
	cases := map[string]Category{
		"Read":                     CatRead,
		"read_file":                CatRead,
		"list_dir":                 CatRead,
		"LS":                       CatRead,
		"Grep":                     CatSearch,
		"codebase_search":          CatSearch,
		"StrReplace":               CatEdit,
		"replace":                  CatEdit,
		"write_file":               CatEdit,
		"Bash":                     CatShell,
		"run_shell_command":        CatShell,
		"exec_command":             CatShell,
		"Skill":                    CatSkill,
		"minerva__resolve_skill":   CatMCP,
		"mcp__codemap__search":     CatMCP,
		"codemap__search_symbols":  CatMCP,
		"Task":                     CatSubagent,
		"something_completely_new": CatOther,
	}
	for name, want := range cases {
		if got := categorize(name); got != want {
			t.Errorf("categorize(%q) = %s, want %s", name, got, want)
		}
	}
	if !BrowseOnly("Read", "Grep", "list_dir") {
		t.Fatal("expected browse-only")
	}
	if BrowseOnly("Read", "Edit") || BrowseOnly() {
		t.Fatal("unexpected browse-only")
	}
}

func TestCleanPrompt(t *testing.T) {
	cursor := "<user_info>\nOS: darwin\n</user_info>\n<git_status>\n## main\n</git_status>\n<user_query>\nadd a changelog entry\n</user_query>"
	if got := CleanPrompt(cursor); got != "add a changelog entry" {
		t.Fatalf("cursor: %q", got)
	}
	if got := CleanPrompt("# AGENTS.md instructions for /Users/x/proj\n\n<INSTRUCTIONS>\nbe nice\n</INSTRUCTIONS>"); got != "" {
		t.Fatalf("agents.md should be skipped: %q", got)
	}
	if got := CleanPrompt("<environment_context>\n  <cwd>/tmp</cwd>\n</environment_context>"); got != "" {
		t.Fatalf("environment_context should be skipped: %q", got)
	}
	if got := CleanPrompt("<attached_files>\nstuff\n</attached_files>\n\nfix the failing test please"); got != "fix the failing test please" {
		t.Fatalf("attached files should be stripped: %q", got)
	}
	if got := CleanPrompt("just a normal request"); got != "just a normal request" {
		t.Fatalf("plain: %q", got)
	}
	long := strings.Repeat("palabra ", 400)
	if got := CleanPrompt(long); len(got) > MaxPromptBytes {
		t.Fatalf("not capped: %d", len(got))
	}
	sess := Session{Turns: []Turn{
		{Role: "user", Text: "# AGENTS.md instructions\nblah"},
		{Role: "user", Text: "<environment_context>x</environment_context>"},
		{Role: "user", Text: "review this PR"},
	}}
	if got := sess.FirstUserPrompt(); got != "review this PR" {
		t.Fatalf("FirstUserPrompt: %q", got)
	}
}

func TestParseSince(t *testing.T) {
	if d, err := ParseSince("7d"); err != nil || d.Hours() != 24*7 {
		t.Fatalf("7d: %v %v", d, err)
	}
	if d, err := ParseSince("36h"); err != nil || d.Hours() != 36 {
		t.Fatalf("36h: %v %v", d, err)
	}
	if d, err := ParseSince(""); err != nil || d != 0 {
		t.Fatalf("empty: %v %v", d, err)
	}
	if _, err := ParseSince("soon"); err == nil {
		t.Fatal("expected error")
	}
}
