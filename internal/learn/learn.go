// Package learn builds a one-page onboarding brief for agents and operators.
package learn

import (
	"github.com/abdul-hamid-achik/minerva/internal/profile"
	"github.com/abdul-hamid-achik/minerva/internal/skill"
	"github.com/abdul-hamid-achik/minerva/internal/suggest"
	"github.com/abdul-hamid-achik/minerva/internal/surface"
)

// Brief is the stable learn --json payload.
type Brief struct {
	Thesis     string                `json:"thesis"`
	Activation string                `json:"activation"`
	Commands   []string              `json:"commands"`
	ExitCodes  map[string]string     `json:"exit_codes"`
	Tools      []surface.ProductTool `json:"tools"`
	Next       []suggest.Suggestion  `json:"next,omitempty"`
}

const thesis = "Minerva is the agent library operator for ~/.agents and a stack readiness orchestrator. It is not a second agent runtime, not Cortex, and not MCPHub."

const activation = "minerva skill activate only flips ~/.agents/.minerva-skills.json. Harnesses (sonar, local-agent) do not read that file. Put skills on a profile (minerva profile add-skills) for durable loading; use the harness load_skill for one-shot session bodies."

// Build returns a cheap onboarding brief. It does not run deep stack probes.
func Build(skillMgr *skill.Manager, profileMgr *profile.Manager, workspace string) Brief {
	var next []suggest.Suggestion
	if skillMgr != nil && profileMgr != nil {
		engine := suggest.NewEngine(skillMgr, profileMgr, nil, workspace)
		engine.IncludeReadiness = false
		engine.IncludeEvidence = false
		all := engine.Analyze()
		if len(all) > 5 {
			all = all[:5]
		}
		next = all
	}
	return Brief{
		Thesis:     thesis,
		Activation: activation,
		Commands: []string{
			"minerva learn --json",
			"minerva skill resolve \"<intent>\" --json",
			"minerva status --json",
			"minerva suggest --json",
			"minerva library lint --json",
			"minerva stack check --json",
			"minerva profile add-skills <profile> <skill>",
		},
		ExitCodes: map[string]string{
			"0": "healthy / ok",
			"1": "unhealthy (core binaries missing) or lint errors",
			"2": "degraded optional stack (stack check --strict)",
			"3": "retrieval not ready (status/stack deep --require-retrieval)",
		},
		Tools: surface.ProductTools(),
		Next:  next,
	}
}
