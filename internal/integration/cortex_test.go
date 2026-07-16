package integration

import (
	"encoding/json"
	"testing"
)

func TestRepoNameFromPath(t *testing.T) {
	if got := repoNameFromPath("/Users/x/projects/minerva"); got != "minerva" {
		t.Fatalf("got %q", got)
	}
	if got := repoNameFromPath("/Users/x/projects/minerva/"); got != "minerva" {
		t.Fatalf("got %q", got)
	}
}

func TestCortexOverviewShape(t *testing.T) {
	raw := []byte(`{
	  "sessions": 154,
	  "active": 91,
	  "stale": 69,
	  "completed": 63,
	  "verified": 2,
	  "completionRate": 0.4,
	  "verifiedRate": 0.01
	}`)
	var ov struct {
		Sessions       int     `json:"sessions"`
		Active         int     `json:"active"`
		Stale          int     `json:"stale"`
		Completed      int     `json:"completed"`
		Verified       int     `json:"verified"`
		CompletionRate float64 `json:"completionRate"`
		VerifiedRate   float64 `json:"verifiedRate"`
	}
	if err := json.Unmarshal(raw, &ov); err != nil {
		t.Fatal(err)
	}
	if ov.Stale != 69 || ov.VerifiedRate != 0.01 {
		t.Fatalf("%+v", ov)
	}
}

func TestCortexStatusJSONRoundTrip(t *testing.T) {
	cs := &CortexStatus{
		Ready:           true,
		Sessions:        10,
		Active:          4,
		Stale:           2,
		VerifiedRate:    0.2,
		StaleSamples:    []CortexSessionSample{{ID: "task_x", Goal: "g", Repository: "minerva"}},
		ActiveWorkspace: 2,
	}
	b, err := json.Marshal(cs)
	if err != nil {
		t.Fatal(err)
	}
	var back CortexStatus
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	if back.Stale != 2 || len(back.StaleSamples) != 1 {
		t.Fatalf("%+v", back)
	}
}
