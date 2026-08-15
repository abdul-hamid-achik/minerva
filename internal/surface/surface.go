// Package surface is the compact session-native MCP contract.
package surface

import "strings"

// ProductTool is one advertised MCP tool.
type ProductTool struct {
	Name        string `json:"name"`
	Title       string `json:"title"`
	Description string `json:"description"`
	UseWhen     string `json:"use_when"`
	ReadOnly    bool   `json:"read_only"`
}

// ProductToolCount is the compact contract size (no CRUD aliases).
const ProductToolCount = 9

// ProductTools is the session-native MCP surface. Mutations stay on the CLI.
func ProductTools() []ProductTool {
	return []ProductTool{
		{
			Name:        "minerva_learn",
			Title:       "Onboard to Minerva",
			Description: "Use when starting a session or asking how Minerva works. Returns a one-page brief: thesis, commands, exit codes, product tools, and next actions. Does not mutate disk. Not a skill resolver — use minerva_resolve_skill for that.",
			UseWhen:     "First time using Minerva, onboarding, or asking how the library operator works.",
			ReadOnly:    true,
		},
		{
			Name:        "minerva_status",
			Title:       "Unified library and stack status",
			Description: "Use when you need an honest doctor report: library inventory, binary presence, optional deep readiness, open evidence, top next actions. Read-only. Prefer this over stack_check when you want a verdict. Set deep=false for a cheap presence-only report.",
			UseWhen:     "Is the agent stack actually ready? Need a unified verdict.",
			ReadOnly:    true,
		},
		{
			Name:        "minerva_suggest",
			Title:       "Ranked next actions",
			Description: "Use when you want ranked next commands for the library or stack. Proposals only — does not apply changes. Prefer profile add-skills over activate. Not for picking a skill for the current task (use minerva_resolve_skill).",
			UseWhen:     "What should I do next to fix the library or stack?",
			ReadOnly:    true,
		},
		{
			Name:        "minerva_resolve_skill",
			Title:       "Resolve which skill to load",
			Description: "Use when deciding which skill to load for the current task (PR review, frontend, docs, Stripe, testing). Returns ranked skills, whether they sit on a profile, and the exact next action (load_skill or profile add-skills). Does not activate Minerva-local pins and does not mutate disk.",
			UseWhen:     "Which skill should I load for this task?",
			ReadOnly:    true,
		},
		{
			Name:        "minerva_skill",
			Title:       "Read skills",
			Description: "Use when listing, showing, or comparing skill bodies (action=list|show|compare). Read-only. Create/update/delete stay on the CLI. Not for choosing a skill by intent — use minerva_resolve_skill.",
			UseWhen:     "Read a skill body or list the catalog.",
			ReadOnly:    true,
		},
		{
			Name:        "minerva_profile",
			Title:       "Read profiles",
			Description: "Use when listing, showing, or comparing agent profiles (action=list|show|compare). Read-only. add-skills and other mutations stay on the CLI so harnesses can approval-gate them.",
			UseWhen:     "Inspect agent.yaml profiles on the shared library.",
			ReadOnly:    true,
		},
		{
			Name:        "minerva_library",
			Title:       "Lint the shared library",
			Description: "Use when checking ~/.agents for missing skill refs, empty prompts, orphans, or secret-like strings (action=lint). Read-only. Export/import stay on the CLI.",
			UseWhen:     "Is the shared agents library internally consistent?",
			ReadOnly:    true,
		},
		{
			Name:        "minerva_stack_check",
			Title:       "Check stack presence",
			Description: "Use when you only need PATH presence and tiers (core vs optional). Fast. Not domain readiness — indexes can still be stale. Use minerva_status with deep=true for retrieval_ready.",
			UseWhen:     "Are the companion binaries installed?",
			ReadOnly:    true,
		},
		{
			Name:        "minerva_evidence",
			Title:       "Read evidence conventions",
			Description: "Use when you need Minerva fcheap tag docs or a search of minerva-tagged stashes (action=docs|search). Read-only. Save/close stay on the CLI.",
			UseWhen:     "How are Minerva outcomes tagged, or search existing stashes.",
			ReadOnly:    true,
		},
	}
}

// GatewayRoutes returns MCPHub-namespaced read-only routes (minerva__learn, …).
func GatewayRoutes() []string {
	tools := ProductTools()
	out := make([]string, 0, len(tools))
	for _, t := range tools {
		out = append(out, "minerva__"+strings.TrimPrefix(t.Name, "minerva_"))
	}
	return out
}

// ProductToolNames returns the advertised tool names in contract order.
func ProductToolNames() []string {
	tools := ProductTools()
	out := make([]string, len(tools))
	for i, t := range tools {
		out[i] = t.Name
	}
	return out
}
