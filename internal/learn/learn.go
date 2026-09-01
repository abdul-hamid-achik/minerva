// Package learn builds a one-page onboarding brief for agents and operators.
package learn

import "github.com/abdul-hamid-achik/minerva/internal/surface"

// Brief is the stable learn --json payload.
type Brief struct {
	Thesis    string                `json:"thesis"`
	How       string                `json:"how"`
	Commands  []string              `json:"commands"`
	ExitCodes map[string]string     `json:"exit_codes"`
	Tools     []surface.ProductTool `json:"tools"`
}

const thesis = "Minerva reads agent-harness conversations and tool calls, proposes skills from those traces, and keeps SKILL.md libraries in sync across harnesses. It is not a second agent runtime, not Cortex, and not a stack monitor."

const how = "Point Minerva at Claude Code, Codex, Cursor, OpenCode, Copilot, Gemini, or sonar session files. Analyze extracts deterministic signals (retries, corrections, load-gaps). Propose writes SKILL.md drafts. Sync links the canonical ~/.agents/skills tree into each writable harness."

// Build returns a cheap onboarding brief.
func Build() Brief {
	return Brief{
		Thesis: thesis,
		How:    how,
		Commands: []string{
			"minerva learn --json",
			"minerva harness list",
			"minerva sessions --since 7d --json",
			"minerva analyze --last --json",
			"minerva propose --since 30d --json",
			"minerva propose apply <id>",
			"minerva skill resolve \"<intent>\" --json",
			"minerva skill sync --dry-run",
		},
		ExitCodes: map[string]string{
			"0": "ok",
			"1": "error (lint failures, missing proposal, command error)",
		},
		Tools: surface.ProductTools(),
	}
}
