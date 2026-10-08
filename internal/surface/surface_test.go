package surface

import (
	"strings"
	"testing"
)

func TestProductTools_CompactContract(t *testing.T) {
	tools := ProductTools()
	if len(tools) != ProductToolCount {
		t.Fatalf("got %d tools, want %d", len(tools), ProductToolCount)
	}
	seen := map[string]bool{}
	for _, tool := range tools {
		if tool.Name == "" || !strings.HasPrefix(tool.Name, "minerva_") {
			t.Fatalf("bad name %q", tool.Name)
		}
		if seen[tool.Name] {
			t.Fatalf("duplicate %s", tool.Name)
		}
		seen[tool.Name] = true
		if tool.UseWhen == "" {
			t.Fatalf("%s missing UseWhen", tool.Name)
		}
		if !strings.HasPrefix(tool.Description, "Use when") {
			t.Fatalf("%s description should start with use-when guidance", tool.Name)
		}
		writes := tool.Name == "minerva_apply" || tool.Name == "minerva_propose"
		if tool.ReadOnly == writes {
			t.Fatalf("%s: read_only=%v, but writes disk=%v", tool.Name, tool.ReadOnly, writes)
		}
		if tool.Destructive != (tool.Name == "minerva_apply") {
			t.Fatalf("%s: only minerva_apply is destructive", tool.Name)
		}
	}
	for _, required := range []string{
		"minerva_learn", "minerva_sessions", "minerva_analyze", "minerva_propose",
		"minerva_resolve_skill", "minerva_skill", "minerva_harness", "minerva_apply",
	} {
		if !seen[required] {
			t.Fatalf("missing %s", required)
		}
	}
}

func TestGatewayRoutes(t *testing.T) {
	routes := GatewayRoutes()
	if len(routes) != ProductToolCount {
		t.Fatalf("routes=%d", len(routes))
	}
	if routes[0] != "minerva__learn" {
		t.Fatalf("first route %q", routes[0])
	}
}
