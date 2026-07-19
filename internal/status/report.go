// Package status builds a unified operator report (library + presence +
// readiness + open evidence + top next actions).
package status

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/abdul-hamid-achik/minerva/internal/evidence"
	"github.com/abdul-hamid-achik/minerva/internal/integration"
	"github.com/abdul-hamid-achik/minerva/internal/monitor"
	"github.com/abdul-hamid-achik/minerva/internal/profile"
	"github.com/abdul-hamid-achik/minerva/internal/skill"
	"github.com/abdul-hamid-achik/minerva/internal/suggest"
)

// Options controls which probes run.
type Options struct {
	Workspace        string
	Deep             bool // run stack deep readiness probes
	IncludeEvidence  bool // count open fail stashes (needs fcheap)
	IncludeSuggest   bool // attach top next actions
	MaxNextActions   int  // default 5
}

// LibraryStatus summarizes the shared ~/.agents library.
type LibraryStatus struct {
	Skills            int                    `json:"skills"`
	Profiles          int                    `json:"profiles"`
	ActiveSkills      int                    `json:"active_skills"`
	ProfileWarnings   []profile.LoadWarning  `json:"profile_warnings,omitempty"`
	MissingSkillRefs  int                    `json:"missing_skill_refs"`
	EmptyPromptCount  int                    `json:"empty_prompt_count"`
}

// EvidenceStatus summarizes open Minerva fail evidence.
type EvidenceStatus struct {
	Available   bool   `json:"available"`
	OpenFails   int    `json:"open_fails"`
	Error       string `json:"error,omitempty"`
	Note        string `json:"note,omitempty"`
}

// Report is the unified doctor/status payload.
type Report struct {
	GeneratedAt string                      `json:"generated_at"`
	Workspace   string                      `json:"workspace"`
	Library     LibraryStatus               `json:"library"`
	Presence    monitor.StackStatus         `json:"presence"`
	Deep        *integration.DeepStackStatus `json:"deep,omitempty"`
	Evidence    EvidenceStatus              `json:"evidence"`
	Next        []suggest.Suggestion        `json:"next,omitempty"`
	Summary     string                      `json:"summary"`
	// Verdict is a short traffic-light: healthy | degraded | unhealthy
	Verdict string `json:"verdict"`
}

// Build composes a status report from managers and sibling probes.
func Build(
	ctx context.Context,
	skillMgr *skill.Manager,
	profileMgr *profile.Manager,
	opts Options,
) Report {
	if opts.Workspace == "" {
		opts.Workspace = "."
	}
	if opts.MaxNextActions <= 0 {
		opts.MaxNextActions = 5
	}
	if ctx == nil {
		ctx = context.Background()
	}

	rep := Report{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Workspace:   opts.Workspace,
		Library:     buildLibrary(skillMgr, profileMgr),
		Presence:    monitor.CheckStack(),
	}

	if opts.Deep {
		rep.Deep = integration.DeepCheck(ctx, opts.Workspace)
	}

	if opts.IncludeEvidence {
		rep.Evidence = buildEvidence(ctx)
	}

	if opts.IncludeSuggest {
		engine := suggest.NewEngine(skillMgr, profileMgr, nil, opts.Workspace)
		engine.IncludeReadiness = opts.Deep
		engine.IncludeEvidence = opts.IncludeEvidence
		all := engine.Analyze()
		if len(all) > opts.MaxNextActions {
			all = all[:opts.MaxNextActions]
		}
		rep.Next = all
	}

	rep.Verdict, rep.Summary = summarize(rep)
	return rep
}

func buildLibrary(skillMgr *skill.Manager, profileMgr *profile.Manager) LibraryStatus {
	ls := LibraryStatus{}
	if skillMgr != nil {
		all := skillMgr.All()
		ls.Skills = len(all)
		for _, s := range all {
			if s.Active {
				ls.ActiveSkills++
			}
		}
	}
	if profileMgr != nil {
		profiles := profileMgr.All()
		ls.Profiles = len(profiles)
		ls.ProfileWarnings = profileMgr.Warnings()
		for _, p := range profiles {
			if strings.TrimSpace(p.SystemPrompt) == "" {
				ls.EmptyPromptCount++
			}
			if skillMgr != nil {
				for _, sk := range p.Skills {
					if !skillMgr.Has(sk) {
						ls.MissingSkillRefs++
					}
				}
			}
		}
	}
	return ls
}

func buildEvidence(ctx context.Context) EvidenceStatus {
	es := EvidenceStatus{}
	fails, err := evidence.ListOpenOutcomeFails(ctx)
	if err != nil {
		es.Available = false
		es.Error = err.Error()
		if strings.Contains(err.Error(), "not found") {
			es.Note = "fcheap not installed; evidence skipped"
		}
		return es
	}
	es.Available = true
	es.OpenFails = len(fails)
	return es
}

func summarize(rep Report) (verdict, summary string) {
	var parts []string

	if !rep.Presence.CoreHealthy {
		verdict = "unhealthy"
		parts = append(parts, fmt.Sprintf("core incomplete (%d missing)", rep.Presence.CoreMissing))
	} else if rep.Presence.Degraded {
		verdict = "degraded"
		parts = append(parts, fmt.Sprintf("optional missing (%d)", rep.Presence.OptionalMiss))
	} else {
		verdict = "healthy"
		parts = append(parts, "core present")
	}

	if rep.Deep != nil {
		if !rep.Deep.RetrievalReady {
			if verdict == "healthy" {
				verdict = "degraded"
			}
			parts = append(parts, "retrieval not ready")
		} else {
			parts = append(parts, "retrieval ready")
		}
	}

	parts = append(parts, fmt.Sprintf("library %d skills / %d profiles", rep.Library.Skills, rep.Library.Profiles))
	if len(rep.Library.ProfileWarnings) > 0 {
		if verdict == "healthy" {
			verdict = "degraded"
		}
		parts = append(parts, fmt.Sprintf("%d profile warnings", len(rep.Library.ProfileWarnings)))
	}
	if rep.Library.MissingSkillRefs > 0 {
		if verdict == "healthy" {
			verdict = "degraded"
		}
		parts = append(parts, fmt.Sprintf("%d missing skill refs", rep.Library.MissingSkillRefs))
	}
	if rep.Evidence.Available && rep.Evidence.OpenFails > 0 {
		if verdict == "healthy" {
			verdict = "degraded"
		}
		parts = append(parts, fmt.Sprintf("%d open evidence fails", rep.Evidence.OpenFails))
	}
	if len(rep.Next) > 0 {
		parts = append(parts, fmt.Sprintf("next: %s", shortMsg(rep.Next[0].Message, 60)))
	}

	return verdict, strings.Join(parts, "; ")
}

func shortMsg(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// FormatHuman renders a terminal-friendly status block.
func FormatHuman(rep Report) string {
	var b strings.Builder
	fmt.Fprintf(&b, "=== Minerva status (%s) ===\n", rep.Verdict)
	fmt.Fprintf(&b, "workspace: %s\n", rep.Workspace)
	fmt.Fprintf(&b, "\nLibrary:  %d skills (%d minerva-active) · %d profiles\n",
		rep.Library.Skills, rep.Library.ActiveSkills, rep.Library.Profiles)
	if rep.Library.EmptyPromptCount > 0 {
		fmt.Fprintf(&b, "          empty prompts: %d\n", rep.Library.EmptyPromptCount)
	}
	if rep.Library.MissingSkillRefs > 0 {
		fmt.Fprintf(&b, "          missing skill refs: %d\n", rep.Library.MissingSkillRefs)
	}
	for _, w := range rep.Library.ProfileWarnings {
		fmt.Fprintf(&b, "          warning: %s — %s\n", w.Dir, w.Message)
	}

	fmt.Fprintf(&b, "\nPresence: core=%v degraded=%v (%d/%d core, optional miss=%d)\n",
		rep.Presence.CoreHealthy, rep.Presence.Degraded,
		rep.Presence.CoreFound, rep.Presence.CoreFound+rep.Presence.CoreMissing,
		rep.Presence.OptionalMiss)

	if rep.Deep != nil {
		ret := "ready"
		if !rep.Deep.RetrievalReady {
			ret = "NOT ready"
		}
		fmt.Fprintf(&b, "Deep:     retrieval %s", ret)
		if rep.Deep.RetrievalDetail != "" {
			fmt.Fprintf(&b, " — %s", rep.Deep.RetrievalDetail)
		}
		b.WriteByte('\n')
		if rep.Deep.Cortex != nil && rep.Deep.Cortex.Error == "" && rep.Deep.Cortex.Stale > 0 {
			fmt.Fprintf(&b, "          cortex stale sessions: %d\n", rep.Deep.Cortex.Stale)
		}
		if rep.Deep.MCPHub != nil && rep.Deep.MCPHub.Error == "" {
			if len(rep.Deep.MCPHub.UnusedEnabled) > 0 {
				fmt.Fprintf(&b, "          mcphub unused_enabled: %s\n", strings.Join(rep.Deep.MCPHub.UnusedEnabled, ", "))
			}
			if len(rep.Deep.MCPHub.AgentsDrift) > 0 {
				fmt.Fprintf(&b, "          mcphub agents_drift: %s\n", strings.Join(rep.Deep.MCPHub.AgentsDrift, ", "))
			}
		}
	}

	if rep.Evidence.Available {
		fmt.Fprintf(&b, "Evidence: %d open outcome:fail stashes\n", rep.Evidence.OpenFails)
	} else if rep.Evidence.Note != "" {
		fmt.Fprintf(&b, "Evidence: %s\n", rep.Evidence.Note)
	} else if rep.Evidence.Error != "" {
		fmt.Fprintf(&b, "Evidence: unavailable (%s)\n", rep.Evidence.Error)
	}

	if len(rep.Next) > 0 {
		fmt.Fprintf(&b, "\nNext actions:\n")
		for i, s := range rep.Next {
			fmt.Fprintf(&b, "  %d. [%s] %s\n", i+1, s.Category, s.Message)
			if s.Action != "" {
				fmt.Fprintf(&b, "      → %s\n", s.Action)
			}
		}
	}

	fmt.Fprintf(&b, "\n%s\n", rep.Summary)
	return b.String()
}
