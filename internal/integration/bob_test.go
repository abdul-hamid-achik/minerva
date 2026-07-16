package integration

import (
	"encoding/json"
	"testing"
)

func TestApplyBobEnvelope_MissingManifest(t *testing.T) {
	raw := []byte(`{
	  "schema_version": 1,
	  "ok": false,
	  "command": "check",
	  "data": {
	    "error": {
	      "code": "missing_manifest",
	      "message": "check: no bob.yaml found in ."
	    }
	  },
	  "warnings": [],
	  "next_actions": [
	    "run: bob init --module <module> --write",
	    "run: bob learn --json"
	  ]
	}`)
	bc := &BobContext{Workspace: "."}
	applyBobEnvelope(bc, raw, nil)
	if bc.Code != "missing_manifest" {
		t.Fatalf("code=%q", bc.Code)
	}
	if bc.Error != "" {
		t.Fatalf("missing_manifest should not set Error field, got %q", bc.Error)
	}
	if len(bc.NextActions) != 2 {
		t.Fatalf("next_actions=%v", bc.NextActions)
	}
	if bc.NextActions[0] != "bob init --module <module> --write" {
		t.Fatalf("action0=%q", bc.NextActions[0])
	}
	if bc.RawNote == "" {
		t.Fatal("expected note from message")
	}
}

func TestApplyBobEnvelope_Clean(t *testing.T) {
	raw := []byte(`{
	  "schema_version": 1,
	  "ok": true,
	  "command": "check",
	  "data": { "clean": true },
	  "next_actions": []
	}`)
	bc := &BobContext{}
	applyBobEnvelope(bc, raw, nil)
	if !bc.Clean || bc.Drift {
		t.Fatalf("clean=%v drift=%v", bc.Clean, bc.Drift)
	}
	if bc.OK == nil || !*bc.OK {
		t.Fatal("ok should be true")
	}
}

func TestNormalizeBobActions(t *testing.T) {
	got := normalizeBobActions([]string{"run: bob plan", "  bob check  ", ""})
	if len(got) != 2 || got[0] != "bob plan" {
		t.Fatalf("%v", got)
	}
	// ensure JSON still round-trips NextActions field
	bc := BobContext{NextActions: got}
	b, _ := json.Marshal(bc)
	var back BobContext
	_ = json.Unmarshal(b, &back)
	if len(back.NextActions) != 2 {
		t.Fatal(back.NextActions)
	}
}
