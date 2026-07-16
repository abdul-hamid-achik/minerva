package integration

import (
	"encoding/json"
	"testing"
)

func TestMCPHubStatusShape_UnusedEnabled(t *testing.T) {
	// Contract fixture matching live mcphub status --json fields we consume.
	raw := []byte(`{
	  "servers": 11,
	  "enabled": 11,
	  "calls": 100,
	  "errors": 5,
	  "est_tokens": 1000,
	  "unused_enabled": ["minerva", "glyph"],
	  "agents": [
	    {"agent": "local-agent", "state": "in sync", "pending": 0},
	    {"agent": "claude", "state": "pending changes", "pending": 2}
	  ]
	}`)
	var st struct {
		UnusedEnabled []string `json:"unused_enabled"`
		Agents        []struct {
			Agent   string `json:"agent"`
			State   string `json:"state"`
			Pending int    `json:"pending"`
		} `json:"agents"`
	}
	if err := json.Unmarshal(raw, &st); err != nil {
		t.Fatal(err)
	}
	if len(st.UnusedEnabled) != 2 {
		t.Fatalf("%v", st.UnusedEnabled)
	}
	if st.Agents[1].Pending != 2 {
		t.Fatal(st.Agents)
	}
}
