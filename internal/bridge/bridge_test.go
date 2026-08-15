package bridge

import (
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/minerva/internal/profile"
)

func TestRender_Markdown(t *testing.T) {
	p := &profile.Profile{
		Name: "dev", Model: "gpt", Skills: []string{"alpha"},
		MCPServers: []string{"minerva"}, SystemPrompt: "hi", Path: "/tmp/agent.yaml",
	}
	snip, err := Render(p, Options{AgentsDir: "/tmp/agents", Harness: "local-agent"}, FormatMarkdown)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(snip.Body, "profile \"dev\"") {
		t.Fatalf("body:\n%s", snip.Body)
	}
	if !strings.Contains(snip.Body, "minerva_status") || !strings.Contains(snip.Body, "minerva_resolve_skill") {
		t.Fatal("expected compact trust tool list")
	}
	if !strings.Contains(snip.Body, "mcp, serve") && !strings.Contains(snip.Body, "args: [mcp") {
		t.Fatal("expected mcphub entry")
	}
}

func TestRender_ShellAndYAML(t *testing.T) {
	p := &profile.Profile{Name: "dev", Skills: []string{"a"}, SystemPrompt: "x"}
	sh, err := Render(p, Options{AgentsDir: "~/.agents"}, FormatShell)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sh.Body, "#!/usr/bin/env bash") {
		t.Fatal(sh.Body)
	}
	y, err := Render(p, Options{AgentsDir: "~/.agents"}, FormatYAML)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(y.Body, "profile:") || !strings.Contains(y.Body, "trust:") {
		t.Fatal(y.Body)
	}
}

func TestRender_SonarHarness(t *testing.T) {
	p := &profile.Profile{Name: "dev", Skills: []string{"a"}, SystemPrompt: "x"}
	snip, err := Render(p, Options{AgentsDir: "~/.agents", Harness: "sonar"}, FormatMarkdown)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(snip.Body, "load_skill") {
		t.Fatal(snip.Body)
	}
	if !strings.Contains(snip.Body, "minerva__resolve_skill") {
		t.Fatal(snip.Body)
	}
}
