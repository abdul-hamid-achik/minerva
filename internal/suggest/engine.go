// Package suggest produces ranked, actionable recommendations from skills,
// profiles, stack presence, analytics, and workspace type.
//
// Activation suggestions target Minerva's activation state under
// ~/.agents/.minerva-skills.json. They do NOT inject skills into a live
// local-agent session; local-agent loads profile skills and /skill from disk
// and its own session state. Prefer profile membership for durable behavior.
package suggest

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/abdul-hamid-achik/minerva/internal/analytics"
	"github.com/abdul-hamid-achik/minerva/internal/evidence"
	"github.com/abdul-hamid-achik/minerva/internal/integration"
	"github.com/abdul-hamid-achik/minerva/internal/monitor"
	"github.com/abdul-hamid-achik/minerva/internal/profile"
	"github.com/abdul-hamid-achik/minerva/internal/skill"
)

// Suggestion is one actionable recommendation.
type Suggestion struct {
	Priority  int    `json:"priority"` // 1=critical, 2=high, 3=medium, 4=low
	Category  string `json:"category"`
	Message   string `json:"message"`
	Action    string `json:"action,omitempty"`
	AutoApply bool   `json:"auto_apply"`
}

// Engine produces suggestions.
type Engine struct {
	skillMgr   *skill.Manager
	profileMgr *profile.Manager
	analytics  *analytics.Store
	workspace  string
	// IncludeReadiness runs deep stack probes (codemap/vecgrep/mcphub). Default false for tests; CLI/MCP enable it.
	IncludeReadiness bool
	// IncludeEvidence reads fcheap outcome:fail stashes for skill/profile suggestions.
	IncludeEvidence bool
}

// NewEngine creates a suggestion engine (presence-level only; enable IncludeReadiness for deep probes).
func NewEngine(skillMgr *skill.Manager, profileMgr *profile.Manager, analytics *analytics.Store, workspace string) *Engine {
	return &Engine{
		skillMgr:   skillMgr,
		profileMgr: profileMgr,
		analytics:  analytics,
		workspace:  workspace,
	}
}

// Analyze produces ranked suggestions.
func (e *Engine) Analyze() []Suggestion {
	var suggestions []Suggestion

	suggestions = append(suggestions, e.skillGapSuggestions()...)
	suggestions = append(suggestions, e.profileGapSuggestions()...)
	suggestions = append(suggestions, e.stackHealthSuggestions()...)
	if e.IncludeReadiness {
		suggestions = append(suggestions, e.readinessSuggestions()...)
	}
	if e.IncludeEvidence {
		suggestions = append(suggestions, e.evidenceFailSuggestions()...)
	}
	suggestions = append(suggestions, e.analyticsSuggestions()...)
	suggestions = append(suggestions, e.crossProfileSuggestions()...)
	suggestions = append(suggestions, e.workspaceAwareSuggestions()...)

	sort.Slice(suggestions, func(i, j int) bool {
		if suggestions[i].Priority == suggestions[j].Priority {
			return suggestions[i].Message < suggestions[j].Message
		}
		return suggestions[i].Priority < suggestions[j].Priority
	})

	suggestions = dedupeSuggestions(suggestions)

	if len(suggestions) == 0 {
		suggestions = append(suggestions, Suggestion{
			Priority: 4,
			Category: "general",
			Message:  "Library and core stack look fine. Activation is Minerva-local; use profiles for durable local-agent behavior.",
		})
	}

	return suggestions
}

func dedupeSuggestions(in []Suggestion) []Suggestion {
	seen := make(map[string]bool, len(in))
	out := make([]Suggestion, 0, len(in))
	for _, s := range in {
		key := s.Category + "\x00" + s.Message
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, s)
	}
	return out
}

func (e *Engine) skillGapSuggestions() []Suggestion {
	var suggestions []Suggestion

	allSkills := e.skillMgr.All()
	inactiveCount := 0
	for _, s := range allSkills {
		if !s.Active {
			inactiveCount++
		}
	}

	for _, s := range allSkills {
		if s.Description == "" {
			suggestions = append(suggestions, Suggestion{
				Priority:  2,
				Category:  "skill",
				Message:   fmt.Sprintf("Skill %q has no description — agents discover it poorly from the catalog", s.Name),
				Action:    fmt.Sprintf("# edit ~/.agents/skills/%s/SKILL.md frontmatter description", s.Name),
				AutoApply: false,
			})
		}
	}

	if inactiveCount > 10 {
		suggestions = append(suggestions, Suggestion{
			Priority:  3,
			Category:  "skill",
			Message:   fmt.Sprintf("%d skills are inactive in Minerva's local activation set — review with minerva skill list", inactiveCount),
			Action:    "minerva skill list",
			AutoApply: false,
		})
	}

	if e.analytics != nil {
		for _, ts := range e.analytics.TopSkills(5) {
			if !e.skillMgr.IsActive(ts.Name) && e.skillMgr.Has(ts.Name) {
				suggestions = append(suggestions, Suggestion{
					Priority:  2,
					Category:  "skill",
					Message:   fmt.Sprintf("Skill %q is frequently activated but not currently active in Minerva state", ts.Name),
					Action:    fmt.Sprintf("minerva skill activate %s", ts.Name),
					AutoApply: true,
				})
			}
		}
	}

	return suggestions
}

func (e *Engine) profileGapSuggestions() []Suggestion {
	var suggestions []Suggestion

	for _, p := range e.profileMgr.All() {
		if strings.TrimSpace(p.SystemPrompt) == "" {
			suggestions = append(suggestions, Suggestion{
				Priority:  1,
				Category:  "profile",
				Message:   fmt.Sprintf("Profile %q has no system prompt — local-agent loads this into session context", p.Name),
				Action:    fmt.Sprintf("minerva profile update-prompt %q \"<system prompt>\"", p.Name),
				AutoApply: false,
			})
		}
		if len(p.Skills) == 0 {
			suggestions = append(suggestions, Suggestion{
				Priority:  2,
				Category:  "profile",
				Message:   fmt.Sprintf("Profile %q has no skills — local-agent will not auto-activate specialized knowledge", p.Name),
				Action:    fmt.Sprintf("minerva profile update-skills %q \"skill1,skill2\"", p.Name),
				AutoApply: false,
			})
		}
		if p.Model == "" {
			suggestions = append(suggestions, Suggestion{
				Priority:  4,
				Category:  "profile",
				Message:   fmt.Sprintf("Profile %q has no model — harness default will be used", p.Name),
				Action:    "",
				AutoApply: false,
			})
		}
		// Validate profile skills exist on disk
		for _, sk := range p.Skills {
			if !e.skillMgr.Has(sk) {
				suggestions = append(suggestions, Suggestion{
					Priority:  1,
					Category:  "profile",
					Message:   fmt.Sprintf("Profile %q references missing skill %q", p.Name, sk),
					Action:    fmt.Sprintf("minerva skill list  # fix profile %q skills", p.Name),
					AutoApply: false,
				})
			}
		}
	}

	return suggestions
}

func (e *Engine) stackHealthSuggestions() []Suggestion {
	var suggestions []Suggestion

	status := monitor.CheckStack()
	for _, tool := range status.Tools {
		if tool.Found {
			continue
		}
		prio := 3
		if tool.Tier == monitor.TierCore {
			prio = 1
		}
		if tool.Tier == monitor.TierInfra {
			prio = 4
		}
		suggestions = append(suggestions, Suggestion{
			Priority:  prio,
			Category:  "stack",
			Message:   fmt.Sprintf("Tool %q (binary %q) is missing — %s", tool.Name, tool.Command, tool.Description),
			Action:    monitor.InstallHint(tool.Name),
			AutoApply: false,
		})
	}

	if status.CoreHealthy && status.Degraded {
		suggestions = append(suggestions, Suggestion{
			Priority:  4,
			Category:  "stack",
			Message:   "Core stack is present; some optional tools are missing (eval/ops degraded, not critical)",
			Action:    "minerva stack check --json",
			AutoApply: false,
		})
	}

	return suggestions
}

// readinessSuggestions uses deep stack probes (codemap/vecgrep/mcphub errors).
// Kept best-effort: probe failures become medium suggestions, not CRIT noise.
func (e *Engine) readinessSuggestions() []Suggestion {
	var suggestions []Suggestion

	// DeepCheck uses per-probe timeouts; overall wall time stays bounded.
	status := integration.DeepCheck(context.Background(), e.workspace)

	for _, r := range status.Readiness {
		if r.Ready {
			continue
		}
		prio := 2
		if r.Tool == "tvault" || r.Tool == "monitor" {
			prio = 3
		}
		msg := fmt.Sprintf("%s not ready", r.Tool)
		if r.Error != "" {
			msg = fmt.Sprintf("%s not ready — %s", r.Tool, r.Error)
		} else if r.Detail != "" {
			msg = fmt.Sprintf("%s not ready — %s", r.Tool, r.Detail)
		}
		action := ""
		if len(r.NextActions) > 0 {
			action = r.NextActions[0]
		}
		suggestions = append(suggestions, Suggestion{
			Priority:  prio,
			Category:  "readiness",
			Message:   msg,
			Action:    action,
			AutoApply: false,
		})
	}

	if status.MCPHub != nil && status.MCPHub.Error == "" {
		if status.MCPHub.TotalCalls > 20 && status.MCPHub.ErrorCount*100/maxInt(status.MCPHub.TotalCalls, 1) >= 15 {
			suggestions = append(suggestions, Suggestion{
				Priority:  2,
				Category:  "stack",
				Message:   fmt.Sprintf("mcphub error rate is high (%d errors / %d calls) — inspect failing servers", status.MCPHub.ErrorCount, status.MCPHub.TotalCalls),
				Action:    "mcphub stats --json",
				AutoApply: false,
			})
		}
		for _, he := range status.MCPHub.HighErrorServers {
			server := he
			if i := strings.IndexByte(he, ':'); i > 0 {
				server = he[:i]
			}
			suggestions = append(suggestions, Suggestion{
				Priority:  2,
				Category:  "stack",
				Message:   fmt.Sprintf("mcphub server high error rate: %s", he),
				Action:    fmt.Sprintf("mcphub doctor --server %s --probe --json", server),
				AutoApply: false,
			})
		}
		// Unused enabled servers (never called) — candidates to disable.
		for _, u := range status.MCPHub.UnusedEnabled {
			// Don't nag about minerva itself when you're running minerva suggest.
			prio := 3
			if u == "minerva" {
				prio = 4
			}
			suggestions = append(suggestions, Suggestion{
				Priority:  prio,
				Category:  "mcphub",
				Message:   fmt.Sprintf("mcphub server %q is enabled but unused (0 calls) — consider: mcphub disable %s", u, u),
				Action:    fmt.Sprintf("mcphub disable %s", u),
				AutoApply: false,
			})
		}
		for _, a := range status.MCPHub.AgentsDrift {
			suggestions = append(suggestions, Suggestion{
				Priority:  2,
				Category:  "mcphub",
				Message:   fmt.Sprintf("mcphub agent config drift: %s — run mcphub status / sync --write when ready", a),
				Action:    "mcphub status --json",
				AutoApply: false,
			})
		}

		// Profile mcp_servers allowlist hygiene against known hub servers.
		if e.profileMgr != nil && len(status.MCPHub.KnownServers) > 0 {
			known := map[string]bool{}
			for _, s := range status.MCPHub.KnownServers {
				known[s] = true
			}
			for _, p := range e.profileMgr.All() {
				for _, srv := range p.MCPServers {
					srv = strings.TrimSpace(srv)
					if srv == "" {
						continue
					}
					if !known[srv] {
						suggestions = append(suggestions, Suggestion{
							Priority:  2,
							Category:  "profile",
							Message:   fmt.Sprintf("profile %q allowlists unknown MCP server %q (not in mcphub)", p.Name, srv),
							Action:    fmt.Sprintf("mcphub list --json  # fix profile %q mcp_servers", p.Name),
							AutoApply: false,
						})
					}
				}
			}
		}
	}

	if status.Bob != nil {
		if status.Bob.Drift {
			action := "bob plan --json"
			if len(status.Bob.NextActions) > 0 {
				action = status.Bob.NextActions[0]
			}
			suggestions = append(suggestions, Suggestion{
				Priority:  2,
				Category:  "stack",
				Message:   fmt.Sprintf("bob reports workspace drift (recipe=%s)", status.Bob.Recipe),
				Action:    action,
				AutoApply: false,
			})
		}
		// missing_manifest is informational — only suggest when next_actions present
		if status.Bob.Code == "missing_manifest" && len(status.Bob.NextActions) > 0 {
			suggestions = append(suggestions, Suggestion{
				Priority:  4,
				Category:  "stack",
				Message:   "workspace has no bob.yaml (optional; only needed for bob-managed repos)",
				Action:    status.Bob.NextActions[0],
				AutoApply: false,
			})
		}
		// Surface remaining bob next_actions as low priority when not already covered
		if status.Bob.Code != "" && status.Bob.Code != "missing_manifest" && len(status.Bob.NextActions) > 0 {
			suggestions = append(suggestions, Suggestion{
				Priority:  2,
				Category:  "stack",
				Message:   fmt.Sprintf("bob: %s", firstNonEmpty(status.Bob.RawNote, status.Bob.Code)),
				Action:    status.Bob.NextActions[0],
				AutoApply: false,
			})
		}
	}

	// Retrieval green light: both codemap and vecgrep must be ready.
	if !status.RetrievalReady {
		action := "minerva stack deep --json"
		if len(status.RetrievalGaps) == 1 && status.RetrievalGaps[0] == "codemap" {
			action = "codemap index"
		} else if len(status.RetrievalGaps) == 1 && status.RetrievalGaps[0] == "vecgrep" {
			action = "vecgrep index"
		}
		suggestions = append(suggestions, Suggestion{
			Priority:  1,
			Category:  "retrieval",
			Message:   fmt.Sprintf("retrieval not ready — %s (do not trust semantic/graph answers until green)", status.RetrievalDetail),
			Action:    action,
			AutoApply: false,
		})
	}

	return suggestions
}

// evidenceFailSuggestions reads fcheap minerva+outcome:fail stashes and maps tags to skills/profiles.
func (e *Engine) evidenceFailSuggestions() []Suggestion {
	var suggestions []Suggestion

	fails, err := evidence.ListOutcomeFails(context.Background())
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil
		}
		suggestions = append(suggestions, Suggestion{
			Priority:  4,
			Category:  "evidence",
			Message:   fmt.Sprintf("could not list fcheap fails: %v", err),
			AutoApply: false,
		})
		return suggestions
	}
	if len(fails) == 0 {
		return nil
	}

	skillFails := map[string]int{}
	profileFails := map[string]int{}
	untagged := 0
	for _, f := range fails {
		if len(f.Skills) == 0 && len(f.Profiles) == 0 {
			untagged++
		}
		for _, s := range f.Skills {
			skillFails[s]++
		}
		for _, p := range f.Profiles {
			profileFails[p]++
		}
	}

	type counted struct {
		name  string
		count int
	}
	topN := func(m map[string]int, n int) []counted {
		var list []counted
		for k, v := range m {
			list = append(list, counted{k, v})
		}
		sort.Slice(list, func(i, j int) bool {
			if list[i].count == list[j].count {
				return list[i].name < list[j].name
			}
			return list[i].count > list[j].count
		})
		if len(list) > n {
			list = list[:n]
		}
		return list
	}

	suggestions = append(suggestions, Suggestion{
		Priority:  2,
		Category:  "evidence",
		Message:   fmt.Sprintf("%d fcheap stashes tagged outcome:fail (minerva) — review with: fcheap list --tag minerva --tag outcome:fail --json", len(fails)),
		Action:    "fcheap list --tag minerva --tag outcome:fail --json",
		AutoApply: false,
	})

	for _, c := range topN(skillFails, 5) {
		action := fmt.Sprintf("minerva skill show %s", c.name)
		if e.skillMgr != nil && e.skillMgr.IsActive(c.name) {
			action = fmt.Sprintf("minerva skill deactivate %s", c.name)
		}
		suggestions = append(suggestions, Suggestion{
			Priority:  2,
			Category:  "evidence",
			Message:   fmt.Sprintf("skill %q appears in %d failed evidence stashes — review or deactivate if harmful", c.name, c.count),
			Action:    action,
			AutoApply: false,
		})
	}
	for _, c := range topN(profileFails, 5) {
		suggestions = append(suggestions, Suggestion{
			Priority:  2,
			Category:  "evidence",
			Message:   fmt.Sprintf("profile %q appears in %d failed evidence stashes — review system prompt / skills", c.name, c.count),
			Action:    fmt.Sprintf("minerva profile show %s", c.name),
			AutoApply: false,
		})
	}
	if untagged > 0 {
		suggestions = append(suggestions, Suggestion{
			Priority:  3,
			Category:  "evidence",
			Message:   fmt.Sprintf("%d failed stashes lack skill:/profile: tags — tag future evidence for better attribution", untagged),
			Action:    "minerva evidence docs",
			AutoApply: false,
		})
	}

	return suggestions
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func (e *Engine) analyticsSuggestions() []Suggestion {
	var suggestions []Suggestion
	if e.analytics == nil {
		return nil
	}

	summary := e.analytics.Summary()
	if summary.TotalEvents == 0 {
		suggestions = append(suggestions, Suggestion{
			Priority:  4,
			Category:  "analytics",
			Message:   "No Minerva usage analytics yet — skill activate/create events will build a local history",
			Action:    "minerva skill list",
			AutoApply: false,
		})
		return suggestions
	}

	if len(summary.TopSkills) > 0 {
		suggestions = append(suggestions, Suggestion{
			Priority:  3,
			Category:  "analytics",
			Message:   fmt.Sprintf("Most-activated skills: %s — consider bundling into a profile for local-agent", strings.Join(summary.TopSkills, ", ")),
			Action:    fmt.Sprintf("minerva profile create my-bundle -s %s", strings.Join(summary.TopSkills, ",")),
			AutoApply: false,
		})
	}

	return suggestions
}

func (e *Engine) crossProfileSuggestions() []Suggestion {
	var suggestions []Suggestion

	allProfiles := e.profileMgr.All()
	if len(allProfiles) < 2 {
		return nil
	}

	skillUsage := make(map[string]int)
	for _, p := range allProfiles {
		for _, s := range p.Skills {
			skillUsage[s]++
		}
	}

	for _, p := range allProfiles {
		profileSkills := make(map[string]bool)
		for _, s := range p.Skills {
			profileSkills[s] = true
		}
		for skillName, count := range skillUsage {
			if count >= 2 && !profileSkills[skillName] && e.skillMgr.Has(skillName) {
				// Merge suggestion: show full new list, don't replace with one skill.
				merged := append(append([]string{}, p.Skills...), skillName)
				suggestions = append(suggestions, Suggestion{
					Priority:  3,
					Category:  "profile",
					Message:   fmt.Sprintf("Profile %q is missing skill %q used by %d other profiles", p.Name, skillName, count),
					Action:    fmt.Sprintf("minerva profile update-skills %q %q", p.Name, strings.Join(merged, ",")),
					AutoApply: false,
				})
			}
		}
	}

	return suggestions
}

func (e *Engine) workspaceAwareSuggestions() []Suggestion {
	var suggestions []Suggestion

	projectType := detectProjectType(e.workspace)
	if projectType == "" {
		return nil
	}

	recommendations := map[string][]string{
		"go":         {"software-architect", "qa-tester", "doc-writer"},
		"typescript": {"frontend-dev", "fullstack-dev", "ux-designer", "qa-tester"},
		"javascript": {"frontend-dev", "fullstack-dev", "ux-designer"},
		"python":     {"fullstack-dev", "qa-tester", "software-architect"},
		"rust":       {"software-architect", "qa-tester"},
		"react":      {"frontend-dev", "ux-designer", "qa-tester"},
		"vue":        {"frontend-dev", "ux-designer"},
		"ios":        {"ios-application-dev", "ux-designer"},
		"android":    {"android-native-dev", "ux-designer"},
	}

	recommended, ok := recommendations[projectType]
	if !ok {
		return nil
	}

	for _, skillName := range recommended {
		if e.skillMgr.Has(skillName) && !e.skillMgr.IsActive(skillName) {
			suggestions = append(suggestions, Suggestion{
				Priority:  2,
				Category:  "skill",
				Message:   fmt.Sprintf("Workspace looks like %s — activate Minerva skill %q (Minerva-local; also add to a profile for local-agent)", projectType, skillName),
				Action:    fmt.Sprintf("minerva skill activate %s", skillName),
				AutoApply: true,
			})
		}
	}

	return suggestions
}

func detectProjectType(workspace string) string {
	if workspace == "" {
		workspace = "."
	}
	markers := []struct {
		file string
		kind string
	}{
		{"go.mod", "go"},
		{"tsconfig.json", "typescript"},
		{"package.json", "javascript"},
		{"Cargo.toml", "rust"},
		{"pyproject.toml", "python"},
		{"setup.py", "python"},
		{"Podfile", "ios"},
		{"build.gradle.kts", "android"},
		{"build.gradle", "android"},
	}

	for _, m := range markers {
		if _, err := os.Stat(filepath.Join(workspace, m.file)); err == nil {
			if m.kind == "javascript" {
				if refined := detectJSFramework(workspace); refined != "" {
					return refined
				}
			}
			return m.kind
		}
	}
	return ""
}

func detectJSFramework(workspace string) string {
	data, err := os.ReadFile(filepath.Join(workspace, "package.json"))
	if err != nil {
		return ""
	}
	content := string(data)
	switch {
	case strings.Contains(content, `"react"`), strings.Contains(content, `"next"`):
		return "react"
	case strings.Contains(content, `"vue"`):
		return "vue"
	case strings.Contains(content, `"@angular"`):
		return "angular"
	}
	return "javascript"
}

// ApplyAuto runs allowlisted auto-apply suggestions against the skill manager.
// Only "minerva skill activate <name>" actions are executed.
func ApplyAuto(skillMgr *skill.Manager, suggestions []Suggestion) (applied []string, skipped []string, err error) {
	for _, s := range suggestions {
		if !s.AutoApply || s.Action == "" {
			continue
		}
		name, ok := parseActivateAction(s.Action)
		if !ok {
			skipped = append(skipped, s.Action)
			continue
		}
		if err := skillMgr.Activate(name); err != nil {
			skipped = append(skipped, fmt.Sprintf("%s (%v)", name, err))
			continue
		}
		applied = append(applied, name)
	}
	return applied, skipped, nil
}

func parseActivateAction(action string) (string, bool) {
	action = strings.TrimSpace(action)
	const prefix = "minerva skill activate "
	if !strings.HasPrefix(action, prefix) {
		return "", false
	}
	name := strings.TrimSpace(strings.TrimPrefix(action, prefix))
	name = strings.Trim(name, `"'`)
	if name == "" || strings.ContainsAny(name, " \t\n") {
		return "", false
	}
	return name, true
}
