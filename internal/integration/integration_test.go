package integration

import (
	"encoding/json"
	"testing"
)

func TestMCPHubStatsFieldMapping(t *testing.T) {
	// Ensure we understand the real schema shape used by ProbeMCPHub.
	raw := []byte(`{
		"totals": {"calls": 10, "errors": 2, "est_tokens": 100, "total_ms": 50},
		"servers": [
			{"server": "cortex", "calls": 7, "errors": 1, "est_tokens": 70},
			{"server": "bob", "calls": 3, "errors": 1, "est_tokens": 30}
		]
	}`)
	var result struct {
		Totals struct {
			Calls     int `json:"calls"`
			EstTokens int `json:"est_tokens"`
			Errors    int `json:"errors"`
		} `json:"totals"`
		Servers []struct {
			Server string `json:"server"`
			Calls  int    `json:"calls"`
		} `json:"servers"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	if result.Totals.Calls != 10 || result.Totals.EstTokens != 100 {
		t.Fatalf("totals parse failed: %+v", result.Totals)
	}
}
