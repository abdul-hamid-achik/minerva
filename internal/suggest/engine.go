// Package suggest produces ranked, actionable recommendations from skills,
// profiles, stack presence, analytics, and workspace type.
//
// Durable recommendations prefer profile skill membership (shared SSOT with
// local-agent). Minerva-local activation under ~/.agents/.minerva-skills.json
// is still available but is not the default auto-apply path.
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
	Priority  int      `json:"priority"` // 1=critical, 2=high, 3=medium, 4=low
	Category  string   `json:"category"`
	Message   string   `json:"message"`
	Action    string   `json:"action,omitempty"`
	AutoApply bool     `json:"auto_apply"`
	Source    []string `json:"source,omitempty"` // e.g. readiness:codemap, evidence, library
}

// Default per-category caps after ranking (anti-noise). Priority-1 is never capped out first.
const (
	maxPerCategoryDefault = 8
	maxCrossProfile       = 5
	maxWorkspaceSkills    = 5
	maxMCPHubUnused       = 4
	maxEvidenceItems      = 8
)

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
	// MaxPerCategory overrides default cap (0 = default).
	MaxPerCategory int
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
	suggestions = capSuggestions(suggestions, e.MaxPerCategory)

	if len(suggestions) == 0 {
		suggestions = append(suggestions, Suggestion{
			Priority: 4,
			Category: "general",
			Message:  "Library and core stack look fine. Activation is Minerva-local; use profiles for durable local-agent behavior.",
			Source:   []string{"library"},
		})
	}

	return suggestions
}

// sug is a small constructor so callers set Source consistently.
func sug(priority int, category, message, action string, auto bool, source ...string) Suggestion {
	return Suggestion{
		Priority:  priority,
		Category:  category,
		Message:   message,
		Action:    action,
		AutoApply: auto,
		Source:    source,
	}
}

func capSuggestions(in []Suggestion, maxPerCat int) []Suggestion {
	if maxPerCat <= 0 {
		maxPerCat = maxPerCategoryDefault
	}
	// Special sub-caps by message fingerprint / category.
	counts := map[string]int{}
	cross := 0
	workspace := 0
	unused := 0
	evidence := 0
	out := make([]Suggestion, 0, len(in))
	for _, s := range in {
		// Never drop criticals due to category noise.
		if s.Priority == 1 {
			out = append(out, s)
			counts[s.Category]++
			continue
		}
		if s.Category == "profile" && strings.Contains(s.Message, "missing skill") && strings.Contains(s.Message, "other profiles") {
			if cross >= maxCrossProfile {
				continue
			}
			cross++
		}
		if s.Category == "profile" && strings.Contains(s.Message, "Workspace looks like") {
			if workspace >= maxWorkspaceSkills {
				continue
			}
			workspace++
		}
		if s.Category == "mcphub" && strings.Contains(s.Message, "unused") {
			if unused >= maxMCPHubUnused {
				continue
			}
			unused++
		}
		if s.Category == "evidence" {
			if evidence >= maxEvidenceItems {
				continue
			}
			evidence++
		}
		if counts[s.Category] >= maxPerCat {
			continue
		}
		counts[s.Category]++
		out = append(out, s)
	}
	return out
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
			suggestions = append(suggestions, sug(2, "skill",
				fmt.Sprintf("Skill %q has no description — agents discover it poorly from the catalog", s.Name),
				fmt.Sprintf("minerva skill update %s -d \"...\"", s.Name),
				false, "library", "skill:"+s.Name))
		}
	}

	if inactiveCount > 10 {
		suggestions = append(suggestions, sug(3, "skill",
			fmt.Sprintf("%d skills are inactive in Minerva's local activation set — review with minerva skill list", inactiveCount),
			"minerva skill list", false, "library"))
	}

	if e.analytics != nil {
		for _, ts := range e.analytics.TopSkills(5) {
			if !e.skillMgr.Has(ts.Name) {
				continue
			}
			if e.skillInAnyProfile(ts.Name) {
				continue
			}
			if action, auto := e.profileAddSkillAction(ts.Name); action != "" {
				suggestions = append(suggestions, sug(2, "profile",
					fmt.Sprintf("Skill %q is frequently used in Minerva analytics but not on any profile — add it for durable local-agent loading", ts.Name),
					action, auto, "analytics", "skill:"+ts.Name))
			}
		}
	}

	return suggestions
}

func (e *Engine) profileGapSuggestions() []Suggestion {
	var suggestions []Suggestion

	for _, w := range e.profileMgr.Warnings() {
		suggestions = append(suggestions, sug(1, "profile",
			fmt.Sprintf("Profile directory %q unreadable — %s", w.Dir, w.Message),
			fmt.Sprintf("# fix %s", firstNonEmpty(w.Path, w.Dir)),
			false, "library", "profile:"+w.Dir))
	}

	for _, p := range e.profileMgr.All() {
		if strings.TrimSpace(p.SystemPrompt) == "" {
			suggestions = append(suggestions, sug(1, "profile",
				fmt.Sprintf("Profile %q has no system prompt — local-agent loads this into session context", p.Name),
				fmt.Sprintf("minerva profile update-prompt %q \"<system prompt>\"", p.Name),
				false, "library", "profile:"+p.Name))
		}
		if len(p.Skills) == 0 {
			suggestions = append(suggestions, sug(2, "profile",
				fmt.Sprintf("Profile %q has no skills — local-agent will not auto-activate specialized knowledge", p.Name),
				fmt.Sprintf("minerva profile add-skills %q skill1,skill2", p.Name),
				false, "library", "profile:"+p.Name))
		}
		if p.Model == "" {
			suggestions = append(suggestions, sug(4, "profile",
				fmt.Sprintf("Profile %q has no model — harness default will be used", p.Name),
				fmt.Sprintf("minerva profile update-model %q <model>", p.Name),
				false, "library", "profile:"+p.Name))
		}
		// Validate profile skills exist on disk
		for _, sk := range p.Skills {
			if !e.skillMgr.Has(sk) {
				suggestions = append(suggestions, sug(1, "profile",
					fmt.Sprintf("Profile %q references missing skill %q", p.Name, sk),
					fmt.Sprintf("minerva profile remove-skills %q %s", p.Name, sk),
					false, "library", "profile:"+p.Name, "skill:"+sk))
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
		suggestions = append(suggestions, sug(prio, "stack",
			fmt.Sprintf("Tool %q (binary %q) is missing — %s", tool.Name, tool.Command, tool.Description),
			monitor.InstallHint(tool.Name), false, "presence", "tool:"+tool.Name))
	}

	if status.CoreHealthy && status.Degraded {
		suggestions = append(suggestions, sug(4, "stack",
			"Core stack is present; some optional tools are missing (eval/ops degraded, not critical)",
			"minerva stack check --json", false, "presence"))
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
		suggestions = append(suggestions, sug(prio, "readiness", msg, action, false, "readiness:"+r.Tool))
	}

	if status.MCPHub != nil && status.MCPHub.Error == "" {
		if status.MCPHub.TotalCalls > 20 && status.MCPHub.ErrorCount*100/maxInt(status.MCPHub.TotalCalls, 1) >= 15 {
			suggestions = append(suggestions, sug(2, "stack",
				fmt.Sprintf("mcphub error rate is high (%d errors / %d calls) — inspect failing servers", status.MCPHub.ErrorCount, status.MCPHub.TotalCalls),
				"mcphub stats --json", false, "mcphub"))
		}
		for _, he := range status.MCPHub.HighErrorServers {
			server := he
			if i := strings.IndexByte(he, ':'); i > 0 {
				server = he[:i]
			}
			suggestions = append(suggestions, sug(2, "stack",
				fmt.Sprintf("mcphub server high error rate: %s", he),
				fmt.Sprintf("mcphub doctor --server %s --probe --json", server), false, "mcphub", "server:"+server))
		}
		// Unused enabled servers (never called) — candidates to disable.
		for _, u := range status.MCPHub.UnusedEnabled {
			// Don't nag about minerva itself when you're running minerva suggest.
			prio := 3
			if u == "minerva" {
				prio = 4
			}
			suggestions = append(suggestions, sug(prio, "mcphub",
				fmt.Sprintf("mcphub server %q is enabled but unused (0 calls) — consider: mcphub disable %s", u, u),
				fmt.Sprintf("mcphub disable %s", u), false, "mcphub", "server:"+u))
		}
		for _, a := range status.MCPHub.AgentsDrift {
			suggestions = append(suggestions, sug(2, "mcphub",
				fmt.Sprintf("mcphub agent config drift: %s — run mcphub status / sync --write when ready", a),
				"mcphub status --json", false, "mcphub"))
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
						suggestions = append(suggestions, sug(2, "profile",
							fmt.Sprintf("profile %q allowlists unknown MCP server %q (not in mcphub)", p.Name, srv),
							fmt.Sprintf("minerva profile update-mcp %q <fixed-list>", p.Name),
							false, "mcphub", "profile:"+p.Name, "server:"+srv))
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
			suggestions = append(suggestions, sug(2, "stack",
				fmt.Sprintf("bob reports workspace drift (recipe=%s)", status.Bob.Recipe),
				action, false, "bob"))
		}
		// missing_manifest is informational — only suggest when next_actions present
		if status.Bob.Code == "missing_manifest" && len(status.Bob.NextActions) > 0 {
			suggestions = append(suggestions, sug(4, "stack",
				"workspace has no bob.yaml (optional; only needed for bob-managed repos)",
				status.Bob.NextActions[0], false, "bob"))
		}
		// Surface remaining bob next_actions as low priority when not already covered
		if status.Bob.Code != "" && status.Bob.Code != "missing_manifest" && len(status.Bob.NextActions) > 0 {
			suggestions = append(suggestions, sug(2, "stack",
				fmt.Sprintf("bob: %s", firstNonEmpty(status.Bob.RawNote, status.Bob.Code)),
				status.Bob.NextActions[0], false, "bob"))
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
		suggestions = append(suggestions, sug(1, "retrieval",
			fmt.Sprintf("retrieval not ready — %s (do not trust semantic/graph answers until green)", status.RetrievalDetail),
			action, false, "retrieval"))
	}

	// Cortex task health (operator view — never mutates tasks).
	if status.Cortex != nil && status.Cortex.Error == "" && status.Cortex.Sessions > 0 {
		if status.Cortex.Stale > 0 {
			action := "cortex sessions --stale --json"
			if len(status.Cortex.StaleSamples) > 0 {
				action = fmt.Sprintf("cortex show %s", status.Cortex.StaleSamples[0].ID)
			}
			suggestions = append(suggestions, sug(2, "cortex",
				fmt.Sprintf("cortex has %d stale sessions (active=%d total=%d) — review or resolve abandoned work", status.Cortex.Stale, status.Cortex.Active, status.Cortex.Sessions),
				action, false, "cortex"))
		}
		// Low verified rate with enough completed work is a process smell.
		if status.Cortex.Completed >= 5 && status.Cortex.VerifiedRate < 0.1 {
			suggestions = append(suggestions, sug(2, "cortex",
				fmt.Sprintf("cortex verified_rate is low (%.1f%% of %d sessions) — tasks complete without verification; prefer cortex verify before remember", status.Cortex.VerifiedRate*100, status.Cortex.Sessions),
				"cortex overview --json", false, "cortex"))
		}
		if status.Cortex.ActiveWorkspace >= 3 {
			suggestions = append(suggestions, sug(3, "cortex",
				fmt.Sprintf("%d active cortex sessions in this workspace — consider finishing or aborting before opening more", status.Cortex.ActiveWorkspace),
				"cortex sessions --active --json", false, "cortex"))
		}
	}

	return suggestions
}

// evidenceFailSuggestions reads open fcheap minerva+outcome:fail stashes (no close receipt).
func (e *Engine) evidenceFailSuggestions() []Suggestion {
	var suggestions []Suggestion

	fails, err := evidence.ListOpenOutcomeFails(context.Background())
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil
		}
		return []Suggestion{sug(4, "evidence", fmt.Sprintf("could not list fcheap fails: %v", err), "", false, "evidence")}
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

	firstID := fails[0].ID
	closeHint := "minerva evidence close <stash-id>"
	if firstID != "" {
		closeHint = fmt.Sprintf("minerva evidence close %s", firstID)
	}
	suggestions = append(suggestions, sug(2, "evidence",
		fmt.Sprintf("%d open fcheap outcome:fail stashes (minerva) — review or close resolved ones", len(fails)),
		closeHint, false, "evidence"))

	for _, c := range topN(skillFails, 5) {
		action := fmt.Sprintf("minerva skill show %s", c.name)
		suggestions = append(suggestions, sug(2, "evidence",
			fmt.Sprintf("skill %q appears in %d open failed evidence stashes — review body or profile membership", c.name, c.count),
			action, false, "evidence", "skill:"+c.name))
	}
	for _, c := range topN(profileFails, 5) {
		suggestions = append(suggestions, sug(2, "evidence",
			fmt.Sprintf("profile %q appears in %d open failed evidence stashes — review system prompt / skills", c.name, c.count),
			fmt.Sprintf("minerva profile show %s", c.name), false, "evidence", "profile:"+c.name))
	}
	if untagged > 0 {
		suggestions = append(suggestions, sug(3, "evidence",
			fmt.Sprintf("%d open failed stashes lack skill:/profile: tags — tag future evidence for better attribution", untagged),
			"minerva evidence docs", false, "evidence"))
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
		return []Suggestion{sug(4, "analytics",
			"No Minerva usage analytics yet — skill activate/create events will build a local history",
			"minerva skill list", false, "analytics")}
	}

	if len(summary.TopSkills) > 0 {
		action := fmt.Sprintf("minerva profile create my-bundle -s %s", strings.Join(summary.TopSkills, ","))
		if profiles := e.profileMgr.All(); len(profiles) == 1 {
			action = fmt.Sprintf("minerva profile add-skills %q %s", profiles[0].Name, strings.Join(summary.TopSkills, ","))
		}
		suggestions = append(suggestions, sug(3, "analytics",
			fmt.Sprintf("Most-used skills in Minerva analytics: %s — put them on a profile for durable local-agent loading", strings.Join(summary.TopSkills, ", ")),
			action, false, "analytics"))
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
				suggestions = append(suggestions, sug(3, "profile",
					fmt.Sprintf("Profile %q is missing skill %q used by %d other profiles", p.Name, skillName, count),
					fmt.Sprintf("minerva profile add-skills %q %s", p.Name, skillName),
					false, "library", "profile:"+p.Name, "skill:"+skillName))
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
		if !e.skillMgr.Has(skillName) {
			continue
		}
		if e.skillInAnyProfile(skillName) {
			continue
		}
		action, auto := e.profileAddSkillAction(skillName)
		if action == "" {
			continue
		}
		suggestions = append(suggestions, sug(2, "profile",
			fmt.Sprintf("Workspace looks like %s — add skill %q to a profile for durable local-agent loading", projectType, skillName),
			action, auto, "workspace:"+projectType, "skill:"+skillName))
	}

	return suggestions
}

// skillInAnyProfile reports whether any loaded profile already lists skillName.
func (e *Engine) skillInAnyProfile(skillName string) bool {
	if e.profileMgr == nil {
		return false
	}
	for _, p := range e.profileMgr.All() {
		for _, s := range p.Skills {
			if s == skillName {
				return true
			}
		}
	}
	return false
}

// profileAddSkillAction returns a durable profile action for skillName.
// AutoApply is true only when exactly one profile exists (unambiguous).
func (e *Engine) profileAddSkillAction(skillName string) (action string, autoApply bool) {
	if e.profileMgr == nil {
		return "", false
	}
	profiles := e.profileMgr.All()
	switch len(profiles) {
	case 0:
		return fmt.Sprintf("minerva profile create default -s %s", skillName), false
	case 1:
		return fmt.Sprintf("minerva profile add-skills %s %s", profiles[0].Name, skillName), true
	default:
		// Prefer a profile named after common defaults, else first alphabetically.
		for _, preferred := range []string{"default", "dev", "code-reviewer"} {
			if e.profileMgr.Has(preferred) {
				return fmt.Sprintf("minerva profile add-skills %s %s", preferred, skillName), false
			}
		}
		return fmt.Sprintf("minerva profile add-skills %s %s", profiles[0].Name, skillName), false
	}
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

// ApplyAuto runs allowlisted auto-apply suggestions.
// Preferred: "minerva profile add-skills <profile> <skill>[,skill…]"
// Legacy (opt-in via applyLocal): "minerva skill activate <name>"
func ApplyAuto(skillMgr *skill.Manager, profileMgr *profile.Manager, suggestions []Suggestion, applyLocal bool) (applied []string, skipped []string, err error) {
	for _, s := range suggestions {
		if !s.AutoApply || s.Action == "" {
			continue
		}
		if profileName, skills, ok := parseAddSkillsAction(s.Action); ok {
			if profileMgr == nil {
				skipped = append(skipped, s.Action+" (no profile manager)")
				continue
			}
			if err := profileMgr.AddSkills(profileName, skills); err != nil {
				skipped = append(skipped, fmt.Sprintf("%s (%v)", s.Action, err))
				continue
			}
			applied = append(applied, s.Action)
			continue
		}
		if name, ok := parseActivateAction(s.Action); ok {
			if !applyLocal {
				skipped = append(skipped, s.Action+" (use --apply-local for Minerva-only activate)")
				continue
			}
			if skillMgr == nil {
				skipped = append(skipped, s.Action+" (no skill manager)")
				continue
			}
			if err := skillMgr.Activate(name); err != nil {
				skipped = append(skipped, fmt.Sprintf("%s (%v)", name, err))
				continue
			}
			applied = append(applied, "activate:"+name)
			continue
		}
		skipped = append(skipped, s.Action)
	}
	return applied, skipped, nil
}

func parseAddSkillsAction(action string) (profileName string, skills []string, ok bool) {
	action = strings.TrimSpace(action)
	const prefix = "minerva profile add-skills "
	if !strings.HasPrefix(action, prefix) {
		return "", nil, false
	}
	rest := strings.TrimSpace(strings.TrimPrefix(action, prefix))
	// Support: profile skill  OR  profile skill1,skill2  OR quoted profile
	parts := splitActionArgs(rest)
	if len(parts) < 2 {
		return "", nil, false
	}
	profileName = parts[0]
	// Remaining tokens may be comma-separated skills or space-separated.
	var raw []string
	for _, p := range parts[1:] {
		for _, s := range strings.Split(p, ",") {
			s = strings.TrimSpace(s)
			if s != "" {
				raw = append(raw, s)
			}
		}
	}
	if profileName == "" || len(raw) == 0 {
		return "", nil, false
	}
	return profileName, raw, true
}

// splitActionArgs splits on spaces while respecting simple double quotes.
func splitActionArgs(s string) []string {
	var out []string
	var cur strings.Builder
	inQuote := false
	for _, r := range s {
		switch {
		case r == '"':
			inQuote = !inQuote
		case r == ' ' || r == '\t':
			if inQuote {
				cur.WriteRune(r)
			} else if cur.Len() > 0 {
				out = append(out, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteRune(r)
		}
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out
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
