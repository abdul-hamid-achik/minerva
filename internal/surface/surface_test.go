package surface

import (
	"strings"
	"testing"
)

func TestProductTools_CompactReadOnlyContract(t *testing.T) {
	tools := ProductTools()
	if len(tools) != ProductToolCount {
		t.Fatalf("got %d tools, want %d", len(tools), ProductToolCount)
	}
	seen := map[string]bool{}
	descs := map[string]bool{}
	for _, tool := range tools {
		if tool.Name == "" || !strings.HasPrefix(tool.Name, "minerva_") {
			t.Fatalf("bad name %q", tool.Name)
		}
		if seen[tool.Name] {
			t.Fatalf("duplicate %s", tool.Name)
		}
		seen[tool.Name] = true
		if !tool.ReadOnly {
			t.Fatalf("%s must be read-only", tool.Name)
		}
		if tool.UseWhen == "" {
			t.Fatalf("%s missing UseWhen", tool.Name)
		}
		if !strings.HasPrefix(tool.Description, "Use when") {
			t.Fatalf("%s description should start with use-when guidance", tool.Name)
		}
		if descs[tool.Description] {
			t.Fatalf("duplicate description on %s", tool.Name)
		}
		descs[tool.Description] = true
	}
	for _, required := range []string{
		"minerva_learn", "minerva_status", "minerva_suggest", "minerva_resolve_skill",
		"minerva_skill", "minerva_profile", "minerva_library", "minerva_stack_check", "minerva_evidence",
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
	if routes[3] != "minerva__resolve_skill" {
		t.Fatalf("resolve route %q", routes[3])
	}
}
