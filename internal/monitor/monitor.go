// Package monitor probes intelligence-stack tool presence with correct
// binary names and tiered health. Presence is PATH + version only; domain
// readiness lives in the integration package (sibling doctors/status).
package monitor

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Tier classifies how a missing tool affects overall health.
type Tier string

const (
	TierCore     Tier = "core"     // missing → stack not core-healthy
	TierOptional Tier = "optional" // missing → degraded only
	TierInfra    Tier = "infra"    // derived backend; missing is informational
)

// Tool is one monitored product in the intelligence stack.
type Tool struct {
	// Name is the stable product id (glyphrun, cairntrace, tinyvault).
	Name string `json:"name"`
	// Command is the real on-PATH binary (glyph, cairn, tvault).
	Command string `json:"command"`
	// VersionArgs is tried for a version string (first success wins).
	VersionArgs [][]string `json:"-"`
	Description string     `json:"description"`
	Tier        Tier       `json:"tier"`
}

// Status is presence + version for one tool.
type Status struct {
	Name        string `json:"name"`
	Command     string `json:"command"`
	Tier        Tier   `json:"tier"`
	Found       bool   `json:"found"`
	Path        string `json:"path,omitempty"`
	Version     string `json:"version,omitempty"`
	VersionOK   bool   `json:"version_ok"`
	Description string `json:"description,omitempty"`
	Error       string `json:"error,omitempty"`
}

// StackStatus is the aggregated presence report.
type StackStatus struct {
	Tools         []Status `json:"tools"`
	CoreHealthy   bool     `json:"core_healthy"`
	Healthy       bool     `json:"healthy"` // same as CoreHealthy (compat)
	Degraded      bool     `json:"degraded"`
	CoreFound     int      `json:"core_found"`
	CoreMissing   int      `json:"core_missing"`
	OptionalFound int      `json:"optional_found"`
	OptionalMiss  int      `json:"optional_missing"`
	Summary       string   `json:"summary"`
}

// KnownTools returns the catalog with correct binary names and tiers.
func KnownTools() []Tool {
	return []Tool{
		// Core control / code plane
		{Name: "bob", Command: "bob", VersionArgs: [][]string{{"version"}}, Description: "Deterministic repository factory", Tier: TierCore},
		{Name: "cortex", Command: "cortex", VersionArgs: [][]string{{"--version"}, {"version"}}, Description: "Evidence-guided agent kernel", Tier: TierCore},
		{Name: "mcphub", Command: "mcphub", VersionArgs: [][]string{{"--version"}, {"version"}}, Description: "MCP gateway and control plane", Tier: TierCore},
		{Name: "codemap", Command: "codemap", VersionArgs: [][]string{{"version"}, {"--version"}}, Description: "Code knowledge graph", Tier: TierCore},
		{Name: "vecgrep", Command: "vecgrep", VersionArgs: [][]string{{"version"}, {"--version"}}, Description: "Semantic code search", Tier: TierCore},
		{Name: "fcheap", Command: "fcheap", VersionArgs: [][]string{{"version"}, {"--version"}}, Description: "Durable artifact vault (file.cheap)", Tier: TierCore},

		// Optional ops / evidence / eval
		{Name: "monitor", Command: "monitor", VersionArgs: [][]string{{"--version"}, {"version"}, {"-v"}}, Description: "Host/process observability and ecosystem doctor", Tier: TierOptional},
		{Name: "hitspec", Command: "hitspec", VersionArgs: [][]string{{"version"}, {"--version"}}, Description: "HTTP/API testing and bounded web fetch", Tier: TierOptional},
		{Name: "glyphrun", Command: "glyph", VersionArgs: [][]string{{"version"}, {"--version"}}, Description: "Terminal/CLI contract testing (binary: glyph)", Tier: TierOptional},
		{Name: "cairntrace", Command: "cairn", VersionArgs: [][]string{{"--version"}, {"version"}}, Description: "Browser behavior-spec runner (binary: cairn)", Tier: TierOptional},
		{Name: "vidtrace", Command: "vidtrace", VersionArgs: [][]string{{"version"}, {"--version"}}, Description: "Video → timeline evidence", Tier: TierOptional},
		{Name: "tinyvault", Command: "tvault", VersionArgs: [][]string{{"--version"}, {"version"}}, Description: "Local secrets vault (binary: tvault)", Tier: TierOptional},

		// Infra (backend for others)
		{Name: "veclite", Command: "veclite", VersionArgs: [][]string{{"version"}, {"--version"}}, Description: "Embeddable vector store (backend for vecgrep/fcheap)", Tier: TierInfra},
	}
}

// Probe checks presence and version for one tool.
func Probe(tool Tool) Status {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	status := Status{
		Name:        tool.Name,
		Command:     tool.Command,
		Tier:        tool.Tier,
		Description: tool.Description,
	}

	path, err := exec.LookPath(tool.Command)
	if err != nil {
		status.Error = fmt.Sprintf("not found in PATH (expected binary %q)", tool.Command)
		return status
	}
	status.Found = true
	status.Path = path

	if len(tool.VersionArgs) == 0 {
		status.VersionOK = true
		return status
	}

	var lastErr error
	for _, args := range tool.VersionArgs {
		cmd := exec.CommandContext(ctx, path, args...)
		out, err := cmd.CombinedOutput()
		if err == nil {
			status.Version = firstLine(strings.TrimSpace(string(out)))
			status.VersionOK = true
			return status
		}
		lastErr = err
		// Some tools print version to stderr with non-zero exit; keep trying.
		if trimmed := firstLine(strings.TrimSpace(string(out))); trimmed != "" && looksLikeVersion(trimmed) {
			status.Version = trimmed
			status.VersionOK = true
			status.Error = ""
			return status
		}
	}

	if lastErr != nil {
		status.Error = fmt.Sprintf("version check failed: %v", lastErr)
	}
	// Found but version unknown is still presence-OK for optional tools.
	return status
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return s
}

func looksLikeVersion(s string) bool {
	lower := strings.ToLower(s)
	if strings.Contains(lower, "version") || strings.Contains(lower, "v0.") || strings.Contains(lower, "v1.") {
		return true
	}
	// bare semver-ish
	for _, r := range s {
		if r >= '0' && r <= '9' {
			return true
		}
	}
	return false
}

// CheckStack probes all known tools and returns tiered presence status.
func CheckStack() StackStatus {
	tools := KnownTools()
	statuses := make([]Status, 0, len(tools))
	coreFound, coreMissing := 0, 0
	optFound, optMiss := 0, 0
	coreHealthy := true
	degraded := false

	for _, tool := range tools {
		status := Probe(tool)
		statuses = append(statuses, status)
		switch tool.Tier {
		case TierCore:
			if status.Found {
				coreFound++
			} else {
				coreMissing++
				coreHealthy = false
			}
		case TierOptional:
			if status.Found {
				optFound++
			} else {
				optMiss++
				degraded = true
			}
		case TierInfra:
			if !status.Found {
				degraded = true
			}
		}
	}

	summary := buildSummary(coreFound, coreMissing, optFound, optMiss, coreHealthy, degraded)
	return StackStatus{
		Tools:         statuses,
		CoreHealthy:   coreHealthy,
		Healthy:       coreHealthy,
		Degraded:      degraded || !coreHealthy,
		CoreFound:     coreFound,
		CoreMissing:   coreMissing,
		OptionalFound: optFound,
		OptionalMiss:  optMiss,
		Summary:       summary,
	}
}

func buildSummary(coreFound, coreMissing, optFound, optMiss int, coreHealthy, degraded bool) string {
	if coreHealthy && !degraded {
		return fmt.Sprintf("Core stack ready (%d tools); optional tools present (%d)", coreFound, optFound)
	}
	if coreHealthy {
		return fmt.Sprintf("Core stack ready (%d tools); %d optional missing (degraded)", coreFound, optMiss)
	}
	return fmt.Sprintf("Core stack incomplete: %d found, %d missing; optional %d found / %d missing",
		coreFound, coreMissing, optFound, optMiss)
}

// CheckStackJSON returns the stack status as indented JSON.
func CheckStackJSON() ([]byte, error) {
	return json.MarshalIndent(CheckStack(), "", "  ")
}

// CheckTool checks a specific tool by product name or binary name.
func CheckTool(name string) *Status {
	for _, tool := range KnownTools() {
		if tool.Name == name || tool.Command == name {
			status := Probe(tool)
			return &status
		}
	}
	return &Status{Name: name, Error: "unknown tool"}
}

// InstallHint returns a human install suggestion for a product id.
func InstallHint(name string) string {
	switch name {
	case "glyphrun", "glyph":
		return "brew install abdul-hamid-achik/tap/glyph  # binary: glyph"
	case "cairntrace", "cairn":
		return "install cairn via project docs (Bun); binary: cairn"
	case "tinyvault", "tvault":
		return "brew install abdul-hamid-achik/tap/tvault  # binary: tvault"
	case "bob":
		return "brew install abdul-hamid-achik/tap/bob"
	case "cortex":
		return "brew install abdul-hamid-achik/tap/cortex"
	case "mcphub":
		return "brew install abdul-hamid-achik/tap/mcphub"
	case "codemap":
		return "brew install abdul-hamid-achik/tap/codemap"
	case "vecgrep":
		return "brew install abdul-hamid-achik/tap/vecgrep"
	case "fcheap":
		return "brew install abdul-hamid-achik/tap/fcheap"
	case "monitor":
		return "brew install abdul-hamid-achik/tap/monitor"
	case "hitspec":
		return "brew install abdul-hamid-achik/tap/hitspec"
	case "vidtrace":
		return "brew install abdul-hamid-achik/tap/vidtrace"
	case "veclite":
		return "brew install abdul-hamid-achik/tap/veclite  # usually pulled in by vecgrep/fcheap"
	default:
		return fmt.Sprintf("install %s (see project docs in ~/projects)", name)
	}
}
