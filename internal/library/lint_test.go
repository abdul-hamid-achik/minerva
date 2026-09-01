package library

import (
	"path/filepath"
	"testing"

	"github.com/abdul-hamid-achik/minerva/internal/skill"
)

func TestLint_EmptyOK(t *testing.T) {
	dir := t.TempDir()
	rep, err := Lint(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !rep.OK {
		t.Fatalf("empty library should not error: %+v", rep)
	}
}

func TestLint_SecretIsError(t *testing.T) {
	dir := t.TempDir()
	mgr := skill.ForAgents(dir)
	if err := mgr.Create(filepath.Join(dir, "skills"), "leaky", "desc", "api_key: abcdefghijklmnopqr"); err != nil {
		t.Fatal(err)
	}
	rep, err := Lint(dir)
	if err != nil {
		t.Fatal(err)
	}
	if rep.OK {
		t.Fatal("expected secret error")
	}
}
