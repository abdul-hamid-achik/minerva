package status

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/minerva/internal/profile"
	"github.com/abdul-hamid-achik/minerva/internal/skill"
)

func TestBuild_LibraryOnly(t *testing.T) {
	dir := t.TempDir()
	sm := skill.NewManagerWithState(dir, filepath.Join(dir, "skills"))
	if err := sm.Create(filepath.Join(dir, "skills"), "alpha", "desc", "body"); err != nil {
		t.Fatal(err)
	}
	pm := profile.NewManager(dir)
	if err := pm.Create(&profile.Profile{Name: "dev", SystemPrompt: "hi", Skills: []string{"alpha"}}); err != nil {
		t.Fatal(err)
	}

	rep := Build(context.Background(), sm, pm, Options{
		Workspace:       dir,
		Deep:            false,
		IncludeEvidence: false,
		IncludeSuggest:  true,
		MaxNextActions:  3,
	})
	if rep.Library.Skills != 1 || rep.Library.Profiles != 1 {
		t.Fatalf("library=%+v", rep.Library)
	}
	if rep.Verdict == "" || rep.Summary == "" {
		t.Fatalf("empty verdict/summary: %+v", rep)
	}
	human := FormatHuman(rep)
	if !strings.Contains(human, "Library:") {
		t.Fatalf("human format missing library:\n%s", human)
	}
}

func TestBuild_MissingSkillRef(t *testing.T) {
	dir := t.TempDir()
	sm := skill.NewManagerWithState(dir, filepath.Join(dir, "skills"))
	_ = sm.LoadAll()
	pm := profile.NewManager(dir)
	if err := pm.Create(&profile.Profile{Name: "dev", SystemPrompt: "hi", Skills: []string{"missing"}}); err != nil {
		t.Fatal(err)
	}
	rep := Build(context.Background(), sm, pm, Options{
		Workspace: dir, Deep: false, IncludeEvidence: false, IncludeSuggest: false,
	})
	if rep.Library.MissingSkillRefs != 1 {
		t.Fatalf("missing refs=%d", rep.Library.MissingSkillRefs)
	}
	if rep.Verdict == "healthy" {
		t.Fatal("expected degraded for missing skill refs")
	}
}
