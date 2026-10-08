package mcp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/abdul-hamid-achik/minerva/internal/skill"
	"github.com/abdul-hamid-achik/minerva/internal/surface"
)

// newTestSession wires a client to the server over in-memory transports with
// a temp home that holds one Claude transcript and one catalog skill.
func newTestSession(t *testing.T) (*sdkmcp.ClientSession, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("MINERVA_HOME", home)
	agents := filepath.Join(home, ".agents")
	t.Setenv("MINERVA_AGENTS_DIR", agents)

	mgr := skill.ForAgents(agents)
	if err := mgr.Create(filepath.Join(agents, "skills"), "doc-writer", "Use when writing or editing READMEs and documentation", "# Doc Writer\n\nWrite docs.\n"); err != nil {
		t.Fatal(err)
	}
	proj := filepath.Join(home, ".claude", "projects", "demo")
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	src, err := os.ReadFile(filepath.Join("..", "session", "testdata", "claude.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	// Plant a secret in the transcript to prove redaction on the wire.
	src = append(src, []byte("\n{\"type\":\"assistant\",\"message\":{\"role\":\"assistant\",\"content\":[{\"type\":\"tool_use\",\"name\":\"Bash\",\"input\":{\"command\":\"export STRIPE_KEY=sk_live_FAKEFAKEFAKEFAKE1234 && git status\"}}]}}\n")...)
	if err := os.WriteFile(filepath.Join(proj, "abc.jsonl"), src, 0o644); err != nil {
		t.Fatal(err)
	}

	srv, err := NewServer(agents)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	st, ct := sdkmcp.NewInMemoryTransports()
	ss, err := srv.Connect(ctx, st)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ss.Close() })
	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "test", Version: "0"}, nil)
	cs, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	return cs, agents
}

func call(t *testing.T, cs *sdkmcp.ClientSession, name string, args map[string]any) (*sdkmcp.CallToolResult, string) {
	t.Helper()
	res, err := cs.CallTool(context.Background(), &sdkmcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	var text strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*sdkmcp.TextContent); ok {
			text.WriteString(tc.Text)
		}
	}
	return res, text.String()
}

func TestServer_ToolsMatchContract(t *testing.T) {
	cs, _ := newTestSession(t)
	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	want := surface.ProductTools()
	if len(res.Tools) != len(want) || len(res.Tools) != surface.ProductToolCount {
		t.Fatalf("got %d tools, contract says %d", len(res.Tools), surface.ProductToolCount)
	}
	byName := map[string]*sdkmcp.Tool{}
	for _, tool := range res.Tools {
		byName[tool.Name] = tool
	}
	for _, w := range want {
		got := byName[w.Name]
		if got == nil {
			t.Fatalf("missing tool %s", w.Name)
		}
		if got.Annotations == nil || got.Annotations.ReadOnlyHint != w.ReadOnly {
			t.Fatalf("%s: read-only hint mismatch: %#v", w.Name, got.Annotations)
		}
		if got.Annotations.DestructiveHint == nil || *got.Annotations.DestructiveHint != w.Destructive {
			t.Fatalf("%s: destructive hint mismatch: %#v", w.Name, got.Annotations)
		}
	}
}

func TestServer_LearnAndSessions(t *testing.T) {
	cs, _ := newTestSession(t)
	res, text := call(t, cs, "minerva_learn", nil)
	if res.IsError || !strings.Contains(text, "not a second agent runtime") {
		t.Fatalf("learn: %v %s", res.IsError, text)
	}
	res, text = call(t, cs, "minerva_sessions", map[string]any{"harness": "claude"})
	if res.IsError {
		t.Fatalf("sessions: %s", text)
	}
	var list []map[string]any
	if err := json.Unmarshal([]byte(text), &list); err != nil || len(list) != 1 {
		t.Fatalf("sessions payload: %v %s", err, text)
	}
	if list[0]["harness"] != "claude" {
		t.Fatalf("%#v", list[0])
	}
}

func TestServer_AnalyzeRedactsAndFindsSignals(t *testing.T) {
	cs, _ := newTestSession(t)
	res, text := call(t, cs, "minerva_analyze", map[string]any{"harness": "claude", "last": true})
	if res.IsError {
		t.Fatalf("analyze: %s", text)
	}
	if strings.Contains(text, "FAKEFAKEFAKEFAKE1234") {
		t.Fatal("secret leaked through MCP")
	}
	var payload struct {
		Sessions int `json:"sessions"`
		Signals  []struct {
			Kind string `json:"kind"`
		} `json:"signals"`
	}
	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Sessions != 1 {
		t.Fatalf("sessions=%d", payload.Sessions)
	}
}

func TestServer_BadSinceIsToolError(t *testing.T) {
	cs, _ := newTestSession(t)
	res, text := call(t, cs, "minerva_sessions", map[string]any{"since": "soon"})
	if !res.IsError || !strings.Contains(text, "invalid since") {
		t.Fatalf("expected tool error, got %v %s", res.IsError, text)
	}
}

func TestServer_ResolveAndSkillRead(t *testing.T) {
	cs, _ := newTestSession(t)
	_, text := call(t, cs, "minerva_resolve_skill", map[string]any{"query": "write the readme documentation"})
	if !strings.Contains(text, `"doc-writer"`) {
		t.Fatalf("resolve: %s", text)
	}
	_, text = call(t, cs, "minerva_skill", map[string]any{"action": "show", "name": "doc-writer"})
	if !strings.Contains(text, "Write docs.") {
		t.Fatalf("show: %s", text)
	}
	res, _ := call(t, cs, "minerva_skill", map[string]any{"action": "delete", "name": "doc-writer"})
	if !res.IsError {
		t.Fatal("mutating skill actions must not exist on MCP")
	}
}

func TestServer_ApplyRequiresProposal(t *testing.T) {
	cs, agents := newTestSession(t)
	res, text := call(t, cs, "minerva_apply", map[string]any{"proposal_id": "new_skill-deadbeef"})
	if !res.IsError || !strings.Contains(text, "not found") {
		t.Fatalf("apply without proposal must fail: %v %s", res.IsError, text)
	}
	// propose then apply round-trip
	_, text = call(t, cs, "minerva_propose", map[string]any{"harness": "claude", "since": "3650d"})
	var payload struct {
		Proposals []struct {
			ID   string `json:"id"`
			Kind string `json:"kind"`
		} `json:"proposals"`
	}
	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(agents, ".minerva", "proposals.json")); err != nil {
		t.Fatalf("propose must persist the store: %v", err)
	}
	for _, p := range payload.Proposals {
		if p.Kind == "load_gap" {
			res, text := call(t, cs, "minerva_apply", map[string]any{"proposal_id": p.ID})
			if !res.IsError || !strings.Contains(text, "not applyable") {
				t.Fatalf("load_gap must not be applyable: %v %s", res.IsError, text)
			}
			return
		}
	}
}

func TestNewServer_StartsWithABrokenSkill(t *testing.T) {
	agents := filepath.Join(t.TempDir(), ".agents")
	for name, body := range map[string]string{
		"good":   "---\nname: good\n---\nbody\n",
		"broken": "---\nname: [unclosed\n---\nbody\n",
	} {
		dir := filepath.Join(agents, "skills", name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	srv, err := NewServer(agents)
	if err != nil {
		t.Fatalf("server did not start: %v", err)
	}
	if !srv.skillManager.Has("good") {
		t.Fatal("valid skill missing from the catalog")
	}
}
