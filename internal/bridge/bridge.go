// Package bridge generates local-agent integration snippets from Minerva profiles:
// launch commands, MCP trust route lists, and operator docs.
//
// Minerva does not invoke local-agent; it only documents how to wire shared disk state.
package bridge

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/abdul-hamid-achik/minerva/internal/profile"
)

// Format selects output shape.
type Format string

const (
	FormatMarkdown Format = "md"
	FormatShell    Format = "shell"
	FormatYAML     Format = "yaml"
)

// Options for bridge export.
type Options struct {
	AgentsDir   string
	ProfileName string
	// Harness is a label for docs (default local-agent).
	Harness string
	// MinervaBinary is used in MCP config examples.
	MinervaBinary string
}

// Snippet is a generated integration artifact.
type Snippet struct {
	Profile     string   `json:"profile"`
	Format      string   `json:"format"`
	Body        string   `json:"body"`
	Skills      []string `json:"skills,omitempty"`
	MCPServers  []string `json:"mcp_servers,omitempty"`
	ProfilePath string   `json:"profile_path,omitempty"`
}

// Generate builds a bridge snippet for a profile.
func Generate(p *profile.Profile, opts Options) (*Snippet, error) {
	if p == nil {
		return nil, fmt.Errorf("profile is required")
	}
	if opts.Harness == "" {
		opts.Harness = "local-agent"
	}
	if opts.MinervaBinary == "" {
		opts.MinervaBinary = "minerva"
	}
	agentsDir := opts.AgentsDir
	if agentsDir == "" {
		agentsDir = "~/.agents"
	}
	profilePath := p.Path
	if profilePath == "" {
		profilePath = filepath.Join(agentsDir, "agents", p.Name, "agent.yaml")
	}

	format := FormatMarkdown
	// caller sets via Render

	s := &Snippet{
		Profile:     p.Name,
		Skills:      append([]string(nil), p.Skills...),
		MCPServers:  append([]string(nil), p.MCPServers...),
		ProfilePath: profilePath,
	}
	_ = format
	return s, nil
}

// Render produces the body in the requested format.
func Render(p *profile.Profile, opts Options, format Format) (*Snippet, error) {
	s, err := Generate(p, opts)
	if err != nil {
		return nil, err
	}
	if format == "" {
		format = FormatMarkdown
	}
	s.Format = string(format)
	switch format {
	case FormatShell:
		s.Body = renderShell(p, opts, s)
	case FormatYAML:
		s.Body = renderYAML(p, opts, s)
	default:
		s.Body = renderMarkdown(p, opts, s)
	}
	return s, nil
}

func renderMarkdown(p *profile.Profile, opts Options, s *Snippet) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Bridge: profile %q → %s\n\n", p.Name, opts.Harness)
	fmt.Fprintf(&b, "Minerva manages the **shared disk SSOT**. %s loads it into a live session.\n\n", opts.Harness)

	fmt.Fprintf(&b, "## Shared files\n\n")
	fmt.Fprintf(&b, "| Path | Role |\n|------|------|\n")
	fmt.Fprintf(&b, "| `%s` | This profile |\n", s.ProfilePath)
	fmt.Fprintf(&b, "| `%s/skills/*/SKILL.md` | Skill bodies |\n", opts.AgentsDir)
	fmt.Fprintf(&b, "| `%s/.minerva-skills.json` | **Not** read by %s |\n\n", opts.AgentsDir, opts.Harness)

	fmt.Fprintf(&b, "## Profile summary\n\n")
	if p.Description != "" {
		fmt.Fprintf(&b, "- **Description:** %s\n", p.Description)
	}
	if p.Model != "" {
		fmt.Fprintf(&b, "- **Model:** `%s`\n", p.Model)
	}
	fmt.Fprintf(&b, "- **Skills:** %s\n", listOrNone(p.Skills))
	fmt.Fprintf(&b, "- **MCP servers:** %s\n\n", listOrNone(p.MCPServers))

	fmt.Fprintf(&b, "## Launch (%s)\n\n", opts.Harness)
	fmt.Fprintf(&b, "Exact flags vary by harness version. Common patterns:\n\n")
	fmt.Fprintf(&b, "```bash\n")
	fmt.Fprintf(&b, "# Ensure library is ready\n")
	fmt.Fprintf(&b, "minerva library lint\n")
	fmt.Fprintf(&b, "minerva status --require-retrieval   # optional gate\n\n")
	fmt.Fprintf(&b, "# Start harness with this profile (examples — adjust to your CLI)\n")
	fmt.Fprintf(&b, "export AGENTS_DIR=%q\n", opts.AgentsDir)
	fmt.Fprintf(&b, "%s --profile %s\n", opts.Harness, p.Name)
	fmt.Fprintf(&b, "# or: %s --agent %s\n", opts.Harness, p.Name)
	fmt.Fprintf(&b, "# or: %s --config %s\n", opts.Harness, s.ProfilePath)
	fmt.Fprintf(&b, "```\n\n")

	fmt.Fprintf(&b, "## MCP trust routes (exact names only)\n\n")
	fmt.Fprintf(&b, "Prefer **exact** tool names — no wildcards. Minerva mutations should be approval-gated.\n\n")
	fmt.Fprintf(&b, "### Read-only (AUTO-safe)\n\n")
	for _, t := range readOnlyTools() {
		fmt.Fprintf(&b, "- `%s`\n", t)
	}
	fmt.Fprintf(&b, "\n### Effectful (approval-gated)\n\n")
	for _, t := range effectfulTools() {
		fmt.Fprintf(&b, "- `%s`\n", t)
	}

	if len(p.MCPServers) > 0 {
		fmt.Fprintf(&b, "\n### Profile MCP allowlist\n\n")
		fmt.Fprintf(&b, "This profile allowlists: %s\n\n", listOrNone(p.MCPServers))
		fmt.Fprintf(&b, "Ensure MCPHub has them enabled:\n\n```bash\n")
		for _, srv := range p.MCPServers {
			fmt.Fprintf(&b, "mcphub list --json | # check %s\n", srv)
		}
		fmt.Fprintf(&b, "```\n")
	}

	fmt.Fprintf(&b, "\n## MCPHub server entry for Minerva\n\n")
	fmt.Fprintf(&b, "```yaml\nservers:\n  minerva:\n    command: %s\n    args: [mcp, serve]\n    enabled: true\n```\n\n", opts.MinervaBinary)

	fmt.Fprintf(&b, "## Honesty checklist\n\n")
	fmt.Fprintf(&b, "1. Skills on the **profile** are durable for %s.\n", opts.Harness)
	fmt.Fprintf(&b, "2. `minerva skill activate` is Minerva-local only — does not inject into a live session.\n")
	fmt.Fprintf(&b, "3. Stack presence ≠ retrieval readiness — use `minerva status` or `stack deep`.\n")
	fmt.Fprintf(&b, "4. Do not reimplement Cortex/MCPHub/Bob through Minerva.\n")
	return b.String()
}

func renderShell(p *profile.Profile, opts Options, s *Snippet) string {
	var b strings.Builder
	fmt.Fprintf(&b, "#!/usr/bin/env bash\n")
	fmt.Fprintf(&b, "# Generated by minerva bridge for profile %q\n", p.Name)
	fmt.Fprintf(&b, "set -euo pipefail\n\n")
	fmt.Fprintf(&b, "export AGENTS_DIR=%q\n", opts.AgentsDir)
	fmt.Fprintf(&b, "export MINERVA_AGENTS_DIR=%q\n\n", opts.AgentsDir)
	fmt.Fprintf(&b, "minerva library lint\n")
	fmt.Fprintf(&b, "# minerva status --require-retrieval || exit $?\n\n")
	fmt.Fprintf(&b, "PROFILE=%q\n", p.Name)
	fmt.Fprintf(&b, "PROFILE_PATH=%q\n\n", s.ProfilePath)
	fmt.Fprintf(&b, "echo \"Starting %s with profile $PROFILE\"\n", opts.Harness)
	fmt.Fprintf(&b, "# Uncomment the invocation that matches your harness:\n")
	fmt.Fprintf(&b, "# %s --profile \"$PROFILE\"\n", opts.Harness)
	fmt.Fprintf(&b, "# %s --agent \"$PROFILE\"\n", opts.Harness)
	fmt.Fprintf(&b, "# %s --config \"$PROFILE_PATH\"\n", opts.Harness)
	if len(p.Skills) > 0 {
		fmt.Fprintf(&b, "\n# Profile skills: %s\n", strings.Join(p.Skills, ", "))
	}
	if len(p.MCPServers) > 0 {
		fmt.Fprintf(&b, "# Profile MCP: %s\n", strings.Join(p.MCPServers, ", "))
	}
	return b.String()
}

func renderYAML(p *profile.Profile, opts Options, s *Snippet) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# minerva bridge — profile %s\n", p.Name)
	fmt.Fprintf(&b, "harness: %s\n", opts.Harness)
	fmt.Fprintf(&b, "agents_dir: %q\n", opts.AgentsDir)
	fmt.Fprintf(&b, "profile:\n")
	fmt.Fprintf(&b, "  name: %s\n", p.Name)
	fmt.Fprintf(&b, "  path: %q\n", s.ProfilePath)
	if p.Model != "" {
		fmt.Fprintf(&b, "  model: %s\n", p.Model)
	}
	fmt.Fprintf(&b, "  skills:\n")
	if len(p.Skills) == 0 {
		fmt.Fprintf(&b, "    []\n")
	} else {
		for _, sk := range p.Skills {
			fmt.Fprintf(&b, "    - %s\n", sk)
		}
	}
	fmt.Fprintf(&b, "  mcp_servers:\n")
	if len(p.MCPServers) == 0 {
		fmt.Fprintf(&b, "    []\n")
	} else {
		for _, m := range p.MCPServers {
			fmt.Fprintf(&b, "    - %s\n", m)
		}
	}
	fmt.Fprintf(&b, "mcphub:\n")
	fmt.Fprintf(&b, "  minerva:\n")
	fmt.Fprintf(&b, "    command: %s\n", opts.MinervaBinary)
	fmt.Fprintf(&b, "    args: [mcp, serve]\n")
	fmt.Fprintf(&b, "    enabled: true\n")
	fmt.Fprintf(&b, "trust:\n")
	fmt.Fprintf(&b, "  read_only:\n")
	for _, t := range readOnlyTools() {
		fmt.Fprintf(&b, "    - %s\n", t)
	}
	fmt.Fprintf(&b, "  effectful:\n")
	for _, t := range effectfulTools() {
		fmt.Fprintf(&b, "    - %s\n", t)
	}
	fmt.Fprintf(&b, "launch_examples:\n")
	fmt.Fprintf(&b, "  - %s --profile %s\n", opts.Harness, p.Name)
	fmt.Fprintf(&b, "  - %s --config %s\n", opts.Harness, s.ProfilePath)
	return b.String()
}

func listOrNone(items []string) string {
	if len(items) == 0 {
		return "_(none)_"
	}
	return "`" + strings.Join(items, "`, `") + "`"
}

func readOnlyTools() []string {
	tools := []string{
		"minerva_skill_list", "minerva_skill_show", "minerva_skill_compare",
		"minerva_profile_list", "minerva_profile_show", "minerva_profile_compare",
		"minerva_stack_check", "minerva_stack_deep", "minerva_status",
		"minerva_suggest", "minerva_analytics",
		"minerva_template_list", "minerva_template_show",
		"minerva_evidence_docs", "minerva_evidence_search",
		"minerva_library_lint",
		"minerva_bridge_show",
	}
	sort.Strings(tools)
	return tools
}

func effectfulTools() []string {
	tools := []string{
		"minerva_skill_create", "minerva_skill_update", "minerva_skill_activate",
		"minerva_skill_deactivate", "minerva_skill_delete",
		"minerva_profile_create", "minerva_profile_update_prompt", "minerva_profile_update_skills",
		"minerva_profile_add_skills", "minerva_profile_remove_skills",
		"minerva_profile_update_model", "minerva_profile_update_mcp", "minerva_profile_update_desc",
		"minerva_profile_delete",
		"minerva_template_apply",
		"minerva_evidence_save", "minerva_evidence_close",
		"minerva_library_export", "minerva_library_import",
	}
	sort.Strings(tools)
	return tools
}
