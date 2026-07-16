package integration

import (
	"encoding/json"
	"fmt"
	"strings"
)

// parseReadiness interprets tool-specific JSON into honest ready flags and next actions.
func parseReadiness(tool string, raw []byte, exitErr error) ReadinessProbe {
	rp := ReadinessProbe{Tool: tool}
	raw = bytesTrimSpace(raw)
	if len(raw) == 0 {
		if exitErr != nil {
			rp.Error = exitErr.Error()
		} else {
			rp.Error = "empty output"
		}
		return rp
	}

	switch tool {
	case "codemap":
		return parseCodemapStatus(raw, exitErr)
	case "vecgrep":
		return parseVecgrepStatus(raw, exitErr)
	case "fcheap":
		return parseGenericOK(tool, "fcheap doctor --json", raw, exitErr)
	case "tvault":
		return parseTvaultStatus(raw, exitErr)
	case "monitor":
		return parseMonitorDoctor(raw, exitErr)
	default:
		return parseGenericOK(tool, tool, raw, exitErr)
	}
}

func parseCodemapStatus(raw []byte, exitErr error) ReadinessProbe {
	rp := ReadinessProbe{Tool: "codemap", Source: "codemap status --json"}
	var s struct {
		Registered bool   `json:"registered"`
		Nodes      int    `json:"nodes"`
		Edges      int    `json:"edges"`
		Files      int    `json:"files"`
		Vectors    int    `json:"vectors"`
		Backend    string `json:"semantic_backend"`
		ProjectKey string `json:"project_key"`
		Stale      *struct {
			Changed int `json:"changed"`
			New     int `json:"new"`
			Deleted int `json:"deleted"`
		} `json:"stale"`
	}
	if err := json.Unmarshal(raw, &s); err != nil {
		rp.Error = fmt.Sprintf("parse: %v", err)
		rp.Detail = truncate(string(raw), 200)
		return rp
	}

	staleAny := false
	if s.Stale != nil {
		staleAny = s.Stale.Changed+s.Stale.New+s.Stale.Deleted > 0
	}

	rp.Detail = fmt.Sprintf("registered=%v nodes=%d vectors=%d backend=%s", s.Registered, s.Nodes, s.Vectors, s.Backend)
	if s.ProjectKey != "" {
		rp.Detail += " key=" + s.ProjectKey
	}

	switch {
	case !s.Registered || s.Nodes == 0:
		rp.Ready = false
		rp.NextActions = []string{"codemap init", "codemap index"}
		rp.Error = "project not indexed (registered=false or empty graph)"
	case staleAny:
		rp.Ready = false
		rp.NextActions = []string{"codemap index"}
		rp.Error = "index stale relative to workspace"
	default:
		rp.Ready = true
	}
	if exitErr != nil && rp.Ready {
		// non-zero exit with healthy JSON → degraded note
		rp.Detail += fmt.Sprintf("; exit=%v", exitErr)
	}
	return rp
}

func parseVecgrepStatus(raw []byte, exitErr error) ReadinessProbe {
	rp := ReadinessProbe{Tool: "vecgrep", Source: "vecgrep status --format json"}
	var s struct {
		IndexFresh     bool   `json:"index_fresh"`
		ProfileMatches bool   `json:"profile_matches"`
		ProfileStatus  string `json:"profile_status"`
		ProviderHealth string `json:"provider_health"`
		Provider       string `json:"provider"`
		Model          string `json:"embedding_model"`
		Database       string `json:"database"`
		Stats          struct {
			Chunks int `json:"chunks"`
			Files  int `json:"files"`
		} `json:"stats"`
		Freshness *struct {
			State string `json:"state"`
		} `json:"freshness"`
	}
	if err := json.Unmarshal(raw, &s); err != nil {
		// try nested / alternate shapes
		var loose map[string]any
		if err2 := json.Unmarshal(raw, &loose); err2 != nil {
			rp.Error = fmt.Sprintf("parse: %v", err)
			if exitErr != nil {
				rp.Error = exitErr.Error()
			}
			return rp
		}
		rp.Detail = truncate(string(raw), 240)
		if exitErr != nil {
			rp.Error = exitErr.Error()
			rp.Ready = false
			return rp
		}
		rp.Ready = true
		return rp
	}

	chunks := s.Stats.Chunks
	freshState := ""
	if s.Freshness != nil {
		freshState = s.Freshness.State
	}

	rp.Detail = fmt.Sprintf("chunks=%d fresh=%v profile_matches=%v provider=%s model=%s",
		chunks, s.IndexFresh, s.ProfileMatches, s.Provider, s.Model)
	if freshState != "" {
		rp.Detail += " freshness=" + freshState
	}

	var problems []string
	if chunks == 0 {
		problems = append(problems, "empty index (0 chunks)")
		rp.NextActions = append(rp.NextActions, "vecgrep index")
	}
	if !s.IndexFresh || freshState == "stale" || freshState == "unknown" {
		if chunks > 0 {
			problems = append(problems, "index not fresh")
			rp.NextActions = append(rp.NextActions, "vecgrep index")
		}
	}
	if !s.ProfileMatches {
		problems = append(problems, "embedding profile mismatch")
		rp.NextActions = append(rp.NextActions, "vecgrep index --full")
	}
	if s.ProviderHealth != "" && s.ProviderHealth != "ok" {
		problems = append(problems, "provider_health="+s.ProviderHealth)
		rp.NextActions = append(rp.NextActions, "check ollama / embedding provider")
	}
	if exitErr != nil && chunks == 0 {
		problems = append(problems, exitErr.Error())
	}

	if len(problems) > 0 {
		rp.Ready = false
		rp.Error = strings.Join(problems, "; ")
		return rp
	}
	rp.Ready = true
	return rp
}

func parseTvaultStatus(raw []byte, exitErr error) ReadinessProbe {
	rp := ReadinessProbe{Tool: "tvault", Source: "tvault status --json"}
	var s struct {
		Initialized bool   `json:"initialized"`
		Locked      bool   `json:"locked"`
		Agent       bool   `json:"agent_running"`
		Error       string `json:"error"`
		// alternate keys
		Status string `json:"status"`
	}
	if err := json.Unmarshal(raw, &s); err != nil {
		return parseGenericOK("tvault", rp.Source, raw, exitErr)
	}
	rp.Detail = fmt.Sprintf("initialized=%v locked=%v agent=%v", s.Initialized, s.Locked, s.Agent)
	if s.Status != "" {
		rp.Detail += " status=" + s.Status
	}
	// tvault present and responding is ready enough; locked is expected
	if s.Error != "" {
		rp.Ready = false
		rp.Error = s.Error
		return rp
	}
	if exitErr != nil && !jsonLooksObject(raw) {
		rp.Ready = false
		rp.Error = exitErr.Error()
		return rp
	}
	rp.Ready = true
	if !s.Initialized {
		rp.Ready = false
		rp.Error = "vault not initialized"
		rp.NextActions = []string{"tvault init"}
	}
	return rp
}

func parseMonitorDoctor(raw []byte, exitErr error) ReadinessProbe {
	rp := ReadinessProbe{Tool: "monitor", Source: "monitor doctor --json"}
	var tools map[string]struct {
		Available bool   `json:"available"`
		Version   string `json:"version"`
		Path      string `json:"path"`
	}
	if err := json.Unmarshal(raw, &tools); err != nil {
		return parseGenericOK("monitor", rp.Source, raw, exitErr)
	}
	missing := 0
	available := 0
	for name, t := range tools {
		if t.Available {
			available++
		} else {
			missing++
			_ = name
		}
	}
	rp.Detail = fmt.Sprintf("ecosystem tools available=%d missing=%d", available, missing)
	rp.Ready = available > 0
	if !rp.Ready {
		rp.Error = "monitor doctor reported no available tools"
	}
	if missing > 0 {
		rp.Detail += fmt.Sprintf(" (%d unavailable)", missing)
	}
	return rp
}

func parseGenericOK(tool, source string, raw []byte, exitErr error) ReadinessProbe {
	rp := ReadinessProbe{Tool: tool, Source: source, Detail: truncate(string(raw), 240)}
	if exitErr != nil && !json.Valid(raw) {
		rp.Error = exitErr.Error()
		rp.Ready = false
		return rp
	}
	// Prefer explicit ok:false when present
	var envelope struct {
		OK    *bool  `json:"ok"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(raw, &envelope); err == nil && envelope.OK != nil {
		rp.Ready = *envelope.OK
		if !*envelope.OK {
			rp.Error = envelope.Error
			if rp.Error == "" {
				rp.Error = "ok=false"
			}
		}
		return rp
	}
	rp.Ready = true
	if exitErr != nil {
		rp.Detail += fmt.Sprintf("; exit=%v", exitErr)
	}
	return rp
}

func bytesTrimSpace(b []byte) []byte {
	return []byte(strings.TrimSpace(string(b)))
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func jsonLooksObject(raw []byte) bool {
	s := strings.TrimSpace(string(raw))
	return strings.HasPrefix(s, "{") || strings.HasPrefix(s, "[")
}
