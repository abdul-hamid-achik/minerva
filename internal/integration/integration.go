// Package integration deep-probes the intelligence stack using sibling
// tools' public CLI contracts (bob/cortex/mcphub doctors and stats).
// It does not reimplement gateway, task lifecycle, or repo apply.
package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// BobContext is workspace contract state from bob check/context.
type BobContext struct {
	Workspace   string   `json:"workspace"`
	Recipe      string   `json:"recipe,omitempty"`
	Clean       bool     `json:"clean"`
	Drift       bool     `json:"drift"`
	OK          *bool    `json:"ok,omitempty"`
	Code        string   `json:"code,omitempty"` // e.g. missing_manifest
	Error       string   `json:"error,omitempty"`
	RawNote     string   `json:"note,omitempty"`
	NextActions []string `json:"next_actions,omitempty"`
}

// CortexStatus is cortex doctor/version readiness plus overview/session signals.
type CortexStatus struct {
	Version string `json:"version,omitempty"`
	Ready   bool   `json:"ready"`
	Detail  string `json:"detail,omitempty"`
	Error   string `json:"error,omitempty"`
	Source  string `json:"source,omitempty"`

	// Overview aggregates (cortex overview --json).
	Sessions       int     `json:"sessions,omitempty"`
	Active         int     `json:"active,omitempty"`
	Stale          int     `json:"stale,omitempty"`
	Completed      int     `json:"completed,omitempty"`
	Verified       int     `json:"verified,omitempty"`
	CompletionRate float64 `json:"completion_rate,omitempty"`
	VerifiedRate   float64 `json:"verified_rate,omitempty"`
	// StaleSamples are a few active/stale session ids for operators.
	StaleSamples []CortexSessionSample `json:"stale_samples,omitempty"`
	// ActiveWorkspace is count of active sessions for the deep-check workspace repo.
	ActiveWorkspace int `json:"active_workspace,omitempty"`
}

// CortexSessionSample is a bounded session row for display/suggest.
type CortexSessionSample struct {
	ID         string `json:"id"`
	Goal       string `json:"goal,omitempty"`
	Phase      string `json:"phase,omitempty"`
	Repository string `json:"repository,omitempty"`
	UpdatedAt  string `json:"updated_at,omitempty"`
}

// MCPHubStats is gateway usage intelligence from mcphub stats + status.
type MCPHubStats struct {
	TotalCalls       int      `json:"total_calls"`
	ErrorCount       int      `json:"error_count"`
	EstTokens        int      `json:"estimated_tokens"`
	ServerCount      int      `json:"server_count"`
	EnabledCount     int      `json:"enabled_count,omitempty"`
	TopServers       []string `json:"top_servers,omitempty"`
	HighErrorServers []string `json:"high_error_servers,omitempty"` // "server:errors/calls"
	// UnusedEnabled are enabled servers with zero recorded calls (mcphub status).
	UnusedEnabled []string `json:"unused_enabled,omitempty"`
	// KnownServers is the union of servers seen in stats + unused_enabled (for profile hygiene).
	KnownServers []string `json:"known_servers,omitempty"`
	// AgentsDrift lists harness agents not "in sync" with mcphub config.
	AgentsDrift []string `json:"agents_drift,omitempty"`
	Error       string   `json:"error,omitempty"`
	Source      string   `json:"source,omitempty"`
}

// ReadinessProbe is a domain-aware readiness result for one tool.
type ReadinessProbe struct {
	Tool        string   `json:"tool"`
	Ready       bool     `json:"ready"`
	Source      string   `json:"source,omitempty"`
	Detail      string   `json:"detail,omitempty"`
	Error       string   `json:"error,omitempty"`
	NextActions []string `json:"next_actions,omitempty"`
}

// DeepStackStatus is rich stack intelligence for operators and agents.
type DeepStackStatus struct {
	Bob       *BobContext      `json:"bob"`
	Cortex    *CortexStatus    `json:"cortex"`
	MCPHub    *MCPHubStats     `json:"mcphub"`
	Readiness []ReadinessProbe `json:"readiness,omitempty"`
	// RetrievalReady is true only when both codemap and vecgrep report Ready.
	// Presence of binaries is not enough — indexes must be usable.
	RetrievalReady  bool     `json:"retrieval_ready"`
	RetrievalDetail string   `json:"retrieval_detail,omitempty"`
	RetrievalGaps   []string `json:"retrieval_gaps,omitempty"` // e.g. codemap, vecgrep
	Summary         string   `json:"summary"`
}

// ProbeBob calls bob check and bob context for a workspace.
func ProbeBob(ctx context.Context, workspace string) *BobContext {
	if workspace == "" {
		workspace = "."
	}
	bc := &BobContext{Workspace: workspace}

	if _, err := exec.LookPath("bob"); err != nil {
		bc.Error = "bob not found in PATH"
		return bc
	}

	checkCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	checkCmd := exec.CommandContext(checkCtx, "bob", "check", workspace, "--json")
	checkOut, checkErr := checkCmd.CombinedOutput()
	applyBobEnvelope(bc, checkOut, checkErr)

	ctxCtx, ctxCancel := context.WithTimeout(ctx, 8*time.Second)
	defer ctxCancel()

	ctxCmd := exec.CommandContext(ctxCtx, "bob", "context", workspace, "--json")
	ctxOut, ctxErr := ctxCmd.CombinedOutput()
	if ctxErr == nil {
		var ctxResult struct {
			OK   bool `json:"ok"`
			Data struct {
				Recipe struct {
					ID string `json:"id"`
				} `json:"recipe"`
			} `json:"data"`
			NextActions []string `json:"next_actions"`
		}
		if err := json.Unmarshal(ctxOut, &ctxResult); err == nil {
			if ctxResult.OK {
				bc.Recipe = ctxResult.Data.Recipe.ID
			}
			// Merge context next_actions if check had none.
			if len(bc.NextActions) == 0 {
				bc.NextActions = normalizeBobActions(ctxResult.NextActions)
			}
		}
	} else if bc.Error == "" && bc.RawNote == "" && bc.Code == "" {
		bc.RawNote = "bob context unavailable"
	}

	return bc
}

// applyBobEnvelope parses bob's schema_version envelope into BobContext.
func applyBobEnvelope(bc *BobContext, raw []byte, runErr error) {
	if len(raw) == 0 {
		if runErr != nil {
			bc.RawNote = fmt.Sprintf("bob check failed: %v", runErr)
		}
		return
	}

	var env struct {
		OK          *bool    `json:"ok"`
		Command     string   `json:"command"`
		NextActions []string `json:"next_actions"`
		Warnings    []string `json:"warnings"`
		Data        struct {
			Clean bool `json:"clean"`
			Error *struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		} `json:"data"`
		// Some failures also put error at top level in older shapes.
		Error *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		bc.RawNote = firstLine(string(raw))
		if runErr != nil {
			bc.RawNote = fmt.Sprintf("%s (%v)", bc.RawNote, runErr)
		}
		return
	}

	bc.OK = env.OK
	bc.NextActions = normalizeBobActions(env.NextActions)
	bc.Clean = env.Data.Clean
	if env.OK != nil && *env.OK {
		bc.Drift = !env.Data.Clean
	}

	errObj := env.Data.Error
	if errObj == nil {
		errObj = env.Error
	}
	if errObj != nil {
		bc.Code = errObj.Code
		bc.RawNote = errObj.Message
		if errObj.Code == "missing_manifest" {
			// Not a binary failure — workspace simply has no bob.yaml.
			bc.Error = ""
		}
	} else if runErr != nil && (env.OK == nil || !*env.OK) {
		bc.RawNote = fmt.Sprintf("bob check failed: %v", runErr)
	}
}

func normalizeBobActions(actions []string) []string {
	out := make([]string, 0, len(actions))
	for _, a := range actions {
		a = strings.TrimSpace(a)
		a = strings.TrimPrefix(a, "run: ")
		a = strings.TrimSpace(a)
		if a != "" {
			out = append(out, a)
		}
	}
	return out
}

// ProbeCortex prefers cortex doctor --json; falls back to --version,
// then enriches with overview + optional stale session samples.
func ProbeCortex(ctx context.Context) *CortexStatus {
	return ProbeCortexWorkspace(ctx, "")
}

// ProbeCortexWorkspace is like ProbeCortex but tags active sessions for workspace.
func ProbeCortexWorkspace(ctx context.Context, workspace string) *CortexStatus {
	cs := &CortexStatus{}

	if _, err := exec.LookPath("cortex"); err != nil {
		cs.Error = "cortex not found in PATH"
		return cs
	}

	// Prefer doctor for readiness.
	docCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	docCmd := exec.CommandContext(docCtx, "cortex", "doctor", "--json")
	docOut, docErr := docCmd.Output()
	cancel()
	if docErr == nil && len(docOut) > 0 {
		cs.Source = "cortex doctor+overview"
		cs.Ready = true
		cs.Detail = compactJSONSummary(docOut, 240)
		if ver := runVersion(ctx, "cortex", []string{"--version"}); ver != "" {
			cs.Version = ver
		}
	} else {
		ver := runVersion(ctx, "cortex", []string{"--version"})
		if ver == "" {
			cs.Error = "cortex doctor and version probes failed"
			if docErr != nil {
				cs.Error = fmt.Sprintf("cortex unavailable: %v", docErr)
			}
			return cs
		}
		cs.Version = ver
		cs.Ready = true
		cs.Source = "cortex --version"
		cs.Detail = "doctor unavailable; version only"
	}

	// Overview dashboard (workspace-independent).
	enrichCortexOverview(ctx, cs)
	// Stale/active samples (bounded).
	enrichCortexSessions(ctx, cs, workspace)
	return cs
}

func enrichCortexOverview(ctx context.Context, cs *CortexStatus) {
	cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(cctx, "cortex", "overview", "--json")
	out, err := cmd.Output()
	if err != nil || len(out) == 0 {
		return
	}
	var ov struct {
		Sessions           int     `json:"sessions"`
		Active             int     `json:"active"`
		Stale              int     `json:"stale"`
		Completed          int     `json:"completed"`
		Verified           int     `json:"verified"`
		CompletionRate     float64 `json:"completionRate"`
		VerifiedRate       float64 `json:"verifiedRate"`
		MeanTimeToComplete int64   `json:"meanTimeToCompleteMs"`
	}
	if err := json.Unmarshal(out, &ov); err != nil {
		return
	}
	cs.Sessions = ov.Sessions
	cs.Active = ov.Active
	cs.Stale = ov.Stale
	cs.Completed = ov.Completed
	cs.Verified = ov.Verified
	cs.CompletionRate = ov.CompletionRate
	cs.VerifiedRate = ov.VerifiedRate
	if !strings.Contains(cs.Source, "overview") {
		cs.Source += "+overview"
	}
}

func enrichCortexSessions(ctx context.Context, cs *CortexStatus, workspace string) {
	// Prefer stale sessions for operator attention.
	cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(cctx, "cortex", "sessions", "--stale", "--json")
	out, err := cmd.Output()
	if err != nil || len(out) == 0 {
		cctx2, cancel2 := context.WithTimeout(ctx, 8*time.Second)
		defer cancel2()
		cmd = exec.CommandContext(cctx2, "cortex", "sessions", "--active", "--json")
		out, err = cmd.Output()
		if err != nil || len(out) == 0 {
			// still try workspace active count below
		}
	}

	if len(out) > 0 {
		var rows []struct {
			ID         string `json:"id"`
			Goal       string `json:"goal"`
			Phase      string `json:"phase"`
			Repository string `json:"repository"`
			UpdatedAt  string `json:"updatedAt"`
		}
		if err := json.Unmarshal(out, &rows); err == nil {
			for _, r := range rows {
				if len(cs.StaleSamples) >= 5 {
					break
				}
				goal := r.Goal
				if len(goal) > 80 {
					goal = goal[:80] + "…"
				}
				cs.StaleSamples = append(cs.StaleSamples, CortexSessionSample{
					ID:         r.ID,
					Goal:       goal,
					Phase:      r.Phase,
					Repository: r.Repository,
					UpdatedAt:  r.UpdatedAt,
				})
			}
		}
	}

	repo := repoNameFromPath(workspace)
	if repo == "" {
		return
	}
	cctx3, cancel3 := context.WithTimeout(ctx, 8*time.Second)
	defer cancel3()
	cmd3 := exec.CommandContext(cctx3, "cortex", "sessions", "--active", "--repo", repo, "--json")
	out3, err3 := cmd3.Output()
	if err3 == nil && len(out3) > 0 {
		var activeRows []struct {
			ID string `json:"id"`
		}
		if json.Unmarshal(out3, &activeRows) == nil {
			cs.ActiveWorkspace = len(activeRows)
		}
	}
}

func repoNameFromPath(workspace string) string {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return ""
	}
	if workspace == "." {
		if wd, err := os.Getwd(); err == nil {
			workspace = wd
		} else {
			return ""
		}
	}
	return filepath.Base(filepath.Clean(workspace))
}

// ProbeMCPHub calls mcphub stats --json and status --json.
func ProbeMCPHub(ctx context.Context) *MCPHubStats {
	ms := &MCPHubStats{Source: "mcphub stats+status --json"}

	if _, err := exec.LookPath("mcphub"); err != nil {
		ms.Error = "mcphub not found in PATH"
		return ms
	}

	known := map[string]bool{}

	// --- stats ---
	{
		ctxCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
		cmd := exec.CommandContext(ctxCtx, "mcphub", "stats", "--json")
		out, err := cmd.Output()
		cancel()
		if err != nil {
			ms.Error = fmt.Sprintf("mcphub stats failed: %v", err)
			// still try status
		} else {
			var result struct {
				Totals struct {
					Calls     int `json:"calls"`
					EstTokens int `json:"est_tokens"`
					Errors    int `json:"errors"`
				} `json:"totals"`
				Servers []struct {
					Server    string `json:"server"`
					Calls     int    `json:"calls"`
					Errors    int    `json:"errors"`
					EstTokens int    `json:"est_tokens"`
				} `json:"servers"`
			}
			if err := json.Unmarshal(out, &result); err != nil {
				ms.Error = fmt.Sprintf("parse mcphub stats: %v", err)
			} else {
				ms.ServerCount = len(result.Servers)
				if result.Totals.Calls > 0 || result.Totals.Errors > 0 || result.Totals.EstTokens > 0 {
					ms.TotalCalls = result.Totals.Calls
					ms.ErrorCount = result.Totals.Errors
					ms.EstTokens = result.Totals.EstTokens
				} else {
					for _, s := range result.Servers {
						ms.TotalCalls += s.Calls
						ms.ErrorCount += s.Errors
						ms.EstTokens += s.EstTokens
					}
				}
				type ranked struct {
					name  string
					calls int
				}
				var ranks []ranked
				for _, s := range result.Servers {
					known[s.Server] = true
					if s.Calls > 0 {
						ranks = append(ranks, ranked{s.Server, s.Calls})
					}
					if s.Calls >= 5 && s.Errors*100/s.Calls >= 20 {
						ms.HighErrorServers = append(ms.HighErrorServers,
							fmt.Sprintf("%s:%d/%d", s.Server, s.Errors, s.Calls))
					}
				}
				for i := 0; i < len(ranks); i++ {
					for j := i + 1; j < len(ranks); j++ {
						if ranks[j].calls > ranks[i].calls {
							ranks[i], ranks[j] = ranks[j], ranks[i]
						}
					}
				}
				for i, r := range ranks {
					if i >= 5 {
						break
					}
					ms.TopServers = append(ms.TopServers, fmt.Sprintf("%s(%d)", r.name, r.calls))
				}
			}
		}
	}

	// --- status (unused_enabled, agent sync) ---
	{
		ctxCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
		cmd := exec.CommandContext(ctxCtx, "mcphub", "status", "--json")
		out, err := cmd.Output()
		cancel()
		if err == nil && len(out) > 0 {
			var st struct {
				Servers       int      `json:"servers"`
				Enabled       int      `json:"enabled"`
				Calls         int      `json:"calls"`
				Errors        int      `json:"errors"`
				EstTokens     int      `json:"est_tokens"`
				UnusedEnabled []string `json:"unused_enabled"`
				Agents        []struct {
					Agent   string `json:"agent"`
					State   string `json:"state"`
					Pending int    `json:"pending"`
				} `json:"agents"`
			}
			if err := json.Unmarshal(out, &st); err == nil {
				ms.EnabledCount = st.Enabled
				if st.Servers > ms.ServerCount {
					ms.ServerCount = st.Servers
				}
				// Prefer status totals when stats lacked totals object.
				if ms.TotalCalls == 0 && st.Calls > 0 {
					ms.TotalCalls = st.Calls
					ms.ErrorCount = st.Errors
					ms.EstTokens = st.EstTokens
				}
				ms.UnusedEnabled = append([]string(nil), st.UnusedEnabled...)
				for _, u := range st.UnusedEnabled {
					known[u] = true
				}
				for _, a := range st.Agents {
					state := strings.ToLower(strings.TrimSpace(a.State))
					if a.Pending > 0 || (state != "" && state != "in sync" && state != "in_sync") {
						label := a.Agent
						if a.Pending > 0 {
							label = fmt.Sprintf("%s(pending=%d)", a.Agent, a.Pending)
						} else {
							label = fmt.Sprintf("%s(%s)", a.Agent, a.State)
						}
						ms.AgentsDrift = append(ms.AgentsDrift, label)
					}
				}
			}
		}
	}

	for name := range known {
		ms.KnownServers = append(ms.KnownServers, name)
	}
	sort.Strings(ms.KnownServers)
	sort.Strings(ms.UnusedEnabled)
	sort.Strings(ms.HighErrorServers)
	sort.Strings(ms.AgentsDrift)

	return ms
}

// ProbeReadiness runs optional readiness checks in parallel.
func ProbeReadiness(ctx context.Context) []ReadinessProbe {
	type job struct {
		bin     string
		args    []string
		source  string
		timeout time.Duration
	}
	jobs := []job{
		{"codemap", []string{"status", "--json"}, "codemap status --json", 6 * time.Second},
		{"vecgrep", []string{"status", "--format", "json"}, "vecgrep status --format json", 15 * time.Second},
		{"fcheap", []string{"doctor", "--json"}, "fcheap doctor --json", 6 * time.Second},
		{"tvault", []string{"status", "--json"}, "tvault status --json", 4 * time.Second},
		{"monitor", []string{"doctor", "--json"}, "monitor doctor --json", 10 * time.Second},
	}

	out := make([]ReadinessProbe, len(jobs))
	done := make(chan struct{}, len(jobs))
	for i, j := range jobs {
		i, j := i, j
		go func() {
			out[i] = probeJSONCommand(ctx, j.bin, j.args, j.source, j.timeout)
			done <- struct{}{}
		}()
	}
	for range jobs {
		<-done
	}
	return out
}

func probeJSONCommand(ctx context.Context, bin string, args []string, source string, timeout time.Duration) ReadinessProbe {
	if _, err := exec.LookPath(bin); err != nil {
		return ReadinessProbe{Tool: bin, Source: source, Error: fmt.Sprintf("%s not found", bin)}
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(cctx, bin, args...)
	out, err := cmd.CombinedOutput()
	// Always try domain-aware parse when we have JSON (even on non-zero exit / timeout).
	if len(out) > 0 && json.Valid(bytes.TrimSpace(out)) {
		rp := parseReadiness(bin, out, err)
		if rp.Source == "" {
			rp.Source = source
		}
		return rp
	}
	rp := ReadinessProbe{Tool: bin, Source: source}
	if err != nil {
		rp.Error = err.Error()
	} else {
		rp.Error = "non-json output"
	}
	if len(out) > 0 {
		rp.Detail = firstLine(string(out))
	}
	// Timeouts/kills without JSON still need a recovery path.
	switch bin {
	case "vecgrep":
		rp.NextActions = []string{"vecgrep status --format json", "vecgrep index"}
	case "codemap":
		rp.NextActions = []string{"codemap status --json", "codemap index"}
	}
	return rp
}

// DeepCheck runs deep probes and returns a rich status.
func DeepCheck(ctx context.Context, workspace string) *DeepStackStatus {
	status := &DeepStackStatus{}

	status.Bob = ProbeBob(ctx, workspace)
	status.Cortex = ProbeCortexWorkspace(ctx, workspace)
	status.MCPHub = ProbeMCPHub(ctx)
	status.Readiness = ProbeReadiness(ctx)
	status.computeRetrieval()

	var parts []string
	if status.Bob.Error != "" {
		parts = append(parts, "bob: unavailable")
	} else if status.Bob.Drift {
		parts = append(parts, fmt.Sprintf("bob: drift (recipe=%s)", status.Bob.Recipe))
	} else if status.Bob.Clean {
		parts = append(parts, fmt.Sprintf("bob: clean (recipe=%s)", status.Bob.Recipe))
	} else if status.Bob.Recipe != "" {
		parts = append(parts, fmt.Sprintf("bob: recipe=%s", status.Bob.Recipe))
	} else {
		parts = append(parts, "bob: no workspace contract")
	}

	if status.Cortex.Error != "" {
		parts = append(parts, "cortex: unavailable")
	} else {
		if status.Cortex.Sessions > 0 {
			parts = append(parts, fmt.Sprintf("cortex: sessions=%d active=%d stale=%d verified_rate=%.1f%%",
				status.Cortex.Sessions, status.Cortex.Active, status.Cortex.Stale, status.Cortex.VerifiedRate*100))
		} else if status.Cortex.Version != "" {
			parts = append(parts, fmt.Sprintf("cortex: %s", firstLine(status.Cortex.Version)))
		} else {
			parts = append(parts, "cortex: ready")
		}
	}

	if status.MCPHub.Error != "" {
		parts = append(parts, "mcphub: unavailable")
	} else {
		parts = append(parts, fmt.Sprintf("mcphub: %d calls, %d errors, %d servers",
			status.MCPHub.TotalCalls, status.MCPHub.ErrorCount, status.MCPHub.ServerCount))
	}

	readyOK, readyFail := 0, 0
	for _, r := range status.Readiness {
		if r.Ready {
			readyOK++
		} else {
			readyFail++
		}
	}
	parts = append(parts, fmt.Sprintf("readiness probes: %d ok / %d failed", readyOK, readyFail))

	if status.RetrievalReady {
		parts = append(parts, "retrieval: ready")
	} else {
		parts = append(parts, "retrieval: not ready ("+strings.Join(status.RetrievalGaps, ", ")+")")
	}

	status.Summary = strings.Join(parts, "; ")
	return status
}

// computeRetrieval sets RetrievalReady when codemap and vecgrep are both Ready.
func (s *DeepStackStatus) computeRetrieval() {
	var codemap, vecgrep *ReadinessProbe
	for i := range s.Readiness {
		r := &s.Readiness[i]
		switch r.Tool {
		case "codemap":
			codemap = r
		case "vecgrep":
			vecgrep = r
		}
	}

	var gaps []string
	var details []string
	if codemap == nil {
		gaps = append(gaps, "codemap")
		details = append(details, "codemap: not probed")
	} else if !codemap.Ready {
		gaps = append(gaps, "codemap")
		if codemap.Error != "" {
			details = append(details, "codemap: "+codemap.Error)
		} else {
			details = append(details, "codemap: not ready")
		}
	} else {
		details = append(details, "codemap: ready")
	}

	if vecgrep == nil {
		gaps = append(gaps, "vecgrep")
		details = append(details, "vecgrep: not probed")
	} else if !vecgrep.Ready {
		gaps = append(gaps, "vecgrep")
		if vecgrep.Error != "" {
			details = append(details, "vecgrep: "+vecgrep.Error)
		} else {
			details = append(details, "vecgrep: not ready")
		}
	} else {
		details = append(details, "vecgrep: ready")
	}

	s.RetrievalGaps = gaps
	s.RetrievalDetail = strings.Join(details, "; ")
	s.RetrievalReady = len(gaps) == 0
}

func runVersion(ctx context.Context, bin string, args []string) string {
	cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(cctx, bin, args...)
	out, err := cmd.CombinedOutput()
	if err != nil && len(out) == 0 {
		return ""
	}
	return firstLine(strings.TrimSpace(string(out)))
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return s
}

func compactJSONSummary(raw []byte, max int) string {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return ""
	}
	var buf bytes.Buffer
	if err := json.Compact(&buf, raw); err != nil {
		s := string(raw)
		if len(s) > max {
			return s[:max] + "…"
		}
		return s
	}
	s := buf.String()
	if len(s) > max {
		return s[:max] + "…"
	}
	return s
}
