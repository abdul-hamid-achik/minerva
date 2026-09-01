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

// ProductToolCount is the compact contract size.
const ProductToolCount = 8

// ProductTools is the session-native MCP surface.
func ProductTools() []ProductTool {
	return []ProductTool{
		{
			Name:        "minerva_learn",
			Title:       "Onboard to Minerva",
			Description: "Use when starting a session or asking how Minerva works. Returns the product thesis, commands, and MCP tools. Does not mutate disk.",
			UseWhen:     "First time using Minerva, or asking what this tool is for.",
			ReadOnly:    true,
		},
		{
			Name:        "minerva_sessions",
			Title:       "List harness sessions",
			Description: "Use when listing recent agent conversations from Claude Code, Codex, Cursor, or other harnesses. Filter by harness, workspace, or --since. Read-only.",
			UseWhen:     "Which sessions exist, and on which harness?",
			ReadOnly:    true,
		},
		{
			Name:        "minerva_analyze",
			Title:       "Analyze session signals",
			Description: "Use when extracting tool-call patterns, retries, corrections, and skill load-gaps from one or more sessions. Returns signals with evidence. Does not write skills.",
			UseWhen:     "What happened in this conversation, and which patterns repeat?",
			ReadOnly:    true,
		},
		{
			Name:        "minerva_propose",
			Title:       "Propose skills from sessions",
			Description: "Use when you want ranked skill proposals (new, update, load-gap) backed by session evidence. Saves drafts so minerva_apply can write them. Does not create SKILL.md until apply.",
			UseWhen:     "Which skills should I create or load from recent sessions?",
			ReadOnly:    true,
		},
		{
			Name:        "minerva_resolve_skill",
			Title:       "Resolve which skill to load",
			Description: "Use when deciding which catalog skill matches a natural-language intent. Returns ranked skills and the next action. Does not mutate disk.",
			UseWhen:     "Which skill should I load for this task?",
			ReadOnly:    true,
		},
		{
			Name:        "minerva_skill",
			Title:       "Read skills",
			Description: "Use when listing, showing, or comparing skill bodies (action=list|show|compare). Create/update/delete stay on the CLI unless you apply a proposal.",
			UseWhen:     "Read a skill body or list the catalog.",
			ReadOnly:    true,
		},
		{
			Name:        "minerva_harness",
			Title:       "Harness inventory and doctor",
			Description: "Use when listing known harnesses or checking skill-dir drift (action=list|doctor). Read-only. Sync and install stay on the CLI.",
			UseWhen:     "Which harnesses are present, and are their skills in sync?",
			ReadOnly:    true,
		},
		{
			Name:        "minerva_apply",
			Title:       "Apply a skill proposal",
			Description: "Use when writing a previously proposed new_skill or update_skill to ~/.agents/skills. Requires proposal_id from minerva_propose. This mutates disk.",
			UseWhen:     "Create or update a SKILL.md from a proposal id.",
			ReadOnly:    false,
		},
	}
}

// GatewayRoutes returns MCPHub-namespaced routes.
func GatewayRoutes() []string {
	tools := ProductTools()
	out := make([]string, 0, len(tools))
	for _, t := range tools {
		out = append(out, "minerva__"+strings.TrimPrefix(t.Name, "minerva_"))
	}
	return out
}

// ProductToolNames returns advertised tool names in contract order.
func ProductToolNames() []string {
	tools := ProductTools()
	out := make([]string, len(tools))
	for i, t := range tools {
		out[i] = t.Name
	}
	return out
}
