package learn

import (
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/minerva/internal/surface"
)

func TestBuild_ContainsContract(t *testing.T) {
	brief := Build()
	if !strings.Contains(brief.Thesis, "not a second agent runtime") {
		t.Fatalf("thesis=%q", brief.Thesis)
	}
	if !strings.Contains(brief.How, "SKILL.md") {
		t.Fatalf("how=%q", brief.How)
	}
	if len(brief.Tools) != surface.ProductToolCount {
		t.Fatalf("tools=%d", len(brief.Tools))
	}
	found := false
	for _, c := range brief.Commands {
		if strings.Contains(c, "skill resolve") {
			found = true
		}
	}
	if !found {
		t.Fatalf("commands=%v", brief.Commands)
	}
}
