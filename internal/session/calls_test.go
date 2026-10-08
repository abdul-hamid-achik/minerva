package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/minerva/internal/harness"
)

// writeLines writes JSONL fixture lines shaped like each harness's real
// on-disk format and returns the file path.
func writeLines(t *testing.T, name string, lines ...string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func allCalls(s Session) []ToolCall {
	var out []ToolCall
	for _, t := range s.Turns {
		out = append(out, t.ToolCalls...)
	}
	return out
}

func TestClaude_ToolResultInNextMessageMatchedByID(t *testing.T) {
	path := writeLines(t, "c.jsonl",
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"Bash","input":{"command":"make build"}},{"type":"tool_use","id":"t2","name":"Bash","input":{"command":"make lint"}}]}}`,
		`{"type":"user","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"t2","content":"ok"},{"type":"tool_result","tool_use_id":"t1","is_error":true,"content":[{"type":"text","text":"exit 2"}]}]}}`,
	)
	s, err := Load(Session{Harness: harness.Claude, Path: path})
	if err != nil {
		t.Fatal(err)
	}
	calls := allCalls(s)
	if len(calls) != 2 {
		t.Fatalf("calls=%d", len(calls))
	}
	if !calls[0].IsError || calls[0].Result != "exit 2" {
		t.Fatalf("t1 = %+v, want error with its result", calls[0])
	}
	if calls[1].IsError || calls[1].Result != "ok" {
		t.Fatalf("t2 = %+v, want success", calls[1])
	}
}

func TestCodex_OutputsMatchedByCallIDAndFailuresDetected(t *testing.T) {
	path := writeLines(t, "rollout.jsonl",
		`{"type":"response_item","payload":{"type":"custom_tool_call","name":"exec","call_id":"c1","input":"run tests"}}`,
		`{"type":"response_item","payload":{"type":"custom_tool_call","name":"exec","call_id":"c2","input":"run lint"}}`,
		`{"type":"response_item","payload":{"type":"custom_tool_call_output","call_id":"c2","output":[{"type":"text","text":"Script completed\nWall time 1s\n{\"exit_code\":1}\nExit code: 3"}]}}`,
		`{"type":"response_item","payload":{"type":"custom_tool_call_output","call_id":"c1","output":[{"type":"text","text":"Script failed\nWall time 2s"}]}}`,
		`{"type":"response_item","payload":{"type":"function_call","name":"shell","call_id":"c3","arguments":"{\"command\":[\"go\",\"test\"]}"}}`,
		`{"type":"response_item","payload":{"type":"function_call_output","call_id":"c3","output":"{\"output\":\"FAIL\",\"metadata\":{\"exit_code\":1}}"}}`,
		`{"type":"response_item","payload":{"type":"function_call","name":"shell","call_id":"c4","arguments":"{}"}}`,
		`{"type":"response_item","payload":{"type":"function_call_output","call_id":"c4","output":"Exit code: 0\nWall time: 1s"}}`,
		`{"type":"response_item","payload":{"type":"function_call","name":"shell","call_id":"c5","arguments":"{}"}}`,
		`{"type":"response_item","payload":{"type":"function_call_output","call_id":"c5","output":"Process exited with code 2"}}`,
	)
	s, err := Load(Session{Harness: harness.Codex, Path: path})
	if err != nil {
		t.Fatal(err)
	}
	got := []bool{}
	for _, c := range allCalls(s) {
		got = append(got, c.IsError)
	}
	want := []bool{true, false, true, false, true}
	if len(got) != len(want) {
		t.Fatalf("calls=%v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("IsError = %v, want %v", got, want)
		}
	}
}

func TestCopilot_ToolRequestAndExecutionStartAreOneCall(t *testing.T) {
	dir := t.TempDir()
	writeFileT(t, filepath.Join(dir, "events.jsonl"), strings.Join([]string{
		`{"type":"assistant.message","data":{"content":"","toolRequests":[{"toolCallId":"x1","name":"bash","arguments":{"command":"go test"},"type":"function"}]}}`,
		`{"type":"tool.execution_start","data":{"toolCallId":"x1","toolName":"bash","arguments":{"command":"go test"}}}`,
		`{"type":"tool.execution_complete","data":{"toolCallId":"x1","success":false,"result":{"content":"FAIL"}}}`,
	}, "\n"))
	s, err := Load(Session{Harness: harness.Copilot, Path: dir})
	if err != nil {
		t.Fatal(err)
	}
	calls := allCalls(s)
	if len(calls) != 1 || !calls[0].IsError {
		t.Fatalf("calls = %+v, want one failed call", calls)
	}
}

func TestOMP_RepeatedCallIDAndSkillRead(t *testing.T) {
	path := writeLines(t, "omp.jsonl",
		`{"type":"session","id":"o1","cwd":"/w","timestamp":"2026-10-01T10:00:00Z"}`,
		`{"type":"message","message":{"role":"assistant","content":[{"type":"toolCall","id":"k1","name":"read","arguments":{"path":"skill://commit-push-tag"}}]}}`,
		`{"type":"message","message":{"role":"assistant","content":[{"type":"toolCall","id":"k1","name":"read","arguments":{"path":"skill://commit-push-tag"}}]}}`,
		`{"type":"message","message":{"role":"toolResult","toolCallId":"k1","isError":false,"content":[{"type":"text","text":"# skill"}]}}`,
	)
	s, err := Load(Session{Harness: harness.OMP, Path: path})
	if err != nil {
		t.Fatal(err)
	}
	summarize(&s)
	if s.ToolCount != 1 {
		t.Fatalf("ToolCount = %d, want 1", s.ToolCount)
	}
	if len(s.Skills) != 1 || s.Skills[0] != "commit-push-tag" {
		t.Fatalf("Skills = %v", s.Skills)
	}
}

func TestHermes_SkillManageIsNotALoad(t *testing.T) {
	path := writeLines(t, "h.jsonl",
		`{"role":"assistant","content":"","tool_calls":[{"id":"a","function":{"name":"skill_manage","arguments":"{\"action\":\"create\",\"name\":\"newthing\"}"}},{"id":"b","function":{"name":"skill_view","arguments":"{\"name\":\"demo\"}"}},{"id":"c","function":{"name":"terminal","arguments":"{\"command\":\"make\"}"}}]}`,
		`{"role":"tool","tool_call_id":"c","content":"{\"output\": \"boom\", \"exit_code\": 2} [truncated]"}`,
	)
	s, err := Load(Session{Harness: harness.Hermes, Path: path})
	if err != nil {
		t.Fatal(err)
	}
	summarize(&s)
	if len(s.Skills) != 1 || s.Skills[0] != "demo" {
		t.Fatalf("Skills = %v, want [demo]", s.Skills)
	}
	if calls := allCalls(s); !calls[2].IsError {
		t.Fatal("unparseable envelope with exit_code 2 not marked as an error")
	}
}

func TestCompactJSON_LongCommandStaysValid(t *testing.T) {
	cmd := "go test ./... " + strings.Repeat("-run X ", 600)
	args := compactJSON(map[string]any{"command": cmd})
	if !json.Valid([]byte(args)) {
		t.Fatal("compactJSON produced invalid JSON")
	}
	if ShellCommand(args) == "" {
		t.Fatal("long command lost its executable")
	}
}

func TestSkillNames(t *testing.T) {
	if got := skillNamesFromArgs(`{"skill":"vercel:nextjs"}`); len(got) != 1 || got[0] != "nextjs" {
		t.Fatalf("plugin skill = %v", got)
	}
	for args, want := range map[string]string{
		`{"file_path":"/Users/x/.agents/skills/go-workflow/SKILL.md"}`: "go-workflow",
		`{"path":"skill://commit-push-tag/refs/a.md"}`:                 "commit-push-tag",
		`{"file_path":"/Users/x/project/SKILL.md"}`:                    "",
		`{"file_path":"/Users/x/project/main.go"}`:                     "",
	} {
		if got := skillFromRead(args); got != want {
			t.Errorf("skillFromRead(%s) = %q, want %q", args, got, want)
		}
	}
}

func TestTruncate_KeepsRunes(t *testing.T) {
	if got := truncate("añb", 2); got != "a" {
		t.Fatalf("truncate split a rune: %q", got)
	}
}

func writeFileT(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
