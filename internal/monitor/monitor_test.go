package monitor

import (
	"testing"
)

func TestKnownTools_BinaryMap(t *testing.T) {
	want := map[string]string{
		"glyphrun":   "glyph",
		"cairntrace": "cairn",
		"tinyvault":  "tvault",
		"bob":        "bob",
		"mcphub":     "mcphub",
		"monitor":    "monitor",
		"hitspec":    "hitspec",
	}
	got := map[string]string{}
	for _, tool := range KnownTools() {
		got[tool.Name] = tool.Command
	}
	for name, cmd := range want {
		if got[name] != cmd {
			t.Errorf("tool %q command = %q, want %q", name, got[name], cmd)
		}
	}
}

func TestKnownTools_CoreTier(t *testing.T) {
	core := map[string]bool{}
	for _, tool := range KnownTools() {
		if tool.Tier == TierCore {
			core[tool.Name] = true
		}
	}
	for _, name := range []string{"bob", "cortex", "mcphub", "codemap", "vecgrep", "fcheap"} {
		if !core[name] {
			t.Errorf("expected %q in core tier", name)
		}
	}
	if core["glyphrun"] || core["vidtrace"] {
		t.Error("optional tools must not be core")
	}
}

func TestCheckStack_DoesNotRequireOptional(t *testing.T) {
	// Smoke: function returns without panic and has tools.
	status := CheckStack()
	if len(status.Tools) < 10 {
		t.Fatalf("expected many tools, got %d", len(status.Tools))
	}
	// Healthy tracks core only.
	if status.Healthy != status.CoreHealthy {
		t.Fatalf("Healthy=%v CoreHealthy=%v", status.Healthy, status.CoreHealthy)
	}
}

func TestInstallHint_CairnNotGoInstall(t *testing.T) {
	h := InstallHint("cairntrace")
	if contains(h, "go install") {
		t.Fatalf("cairn install hint should not use go install: %s", h)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		(func() bool {
			for i := 0; i+len(sub) <= len(s); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
			return false
		})())
}
