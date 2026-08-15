package learn

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/minerva/internal/profile"
	"github.com/abdul-hamid-achik/minerva/internal/skill"
	"github.com/abdul-hamid-achik/minerva/internal/surface"
)

func TestBuild_ContainsContract(t *testing.T) {
	dir := t.TempDir()
	sm := skill.NewManagerWithState(dir, filepath.Join(dir, "skills"))
	_ = sm.LoadAll()
	pm := profile.NewManager(dir)
	_ = pm.LoadAll()

	brief := Build(sm, pm, dir)
	if !strings.Contains(brief.Thesis, "not a second agent runtime") {
		t.Fatalf("thesis=%q", brief.Thesis)
	}
	if !strings.Contains(brief.Activation, "profile add-skills") {
		t.Fatalf("activation=%q", brief.Activation)
	}
	if len(brief.Tools) != surface.ProductToolCount {
		t.Fatalf("tools=%d", len(brief.Tools))
	}
	if brief.ExitCodes["3"] == "" {
		t.Fatal("missing retrieval exit code")
	}
	foundResolve := false
	for _, c := range brief.Commands {
		if strings.Contains(c, "skill resolve") {
			foundResolve = true
		}
	}
	if !foundResolve {
		t.Fatalf("commands=%v", brief.Commands)
	}
}
