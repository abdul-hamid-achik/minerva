package textdiff

import (
	"strings"
	"testing"
)

func TestUnified_Equal(t *testing.T) {
	if Unified("a", "b", "same", "same") != "" {
		t.Fatal("expected empty for equal inputs")
	}
}

func TestUnified_insertDelete(t *testing.T) {
	a := "line1\nline2\nline3"
	b := "line1\nline2-changed\nline3\nline4"
	out := Unified("old.md", "new.md", a, b)
	if !strings.Contains(out, "--- old.md") || !strings.Contains(out, "+++ new.md") {
		t.Fatalf("headers missing:\n%s", out)
	}
	if !strings.Contains(out, "-line2") {
		t.Fatalf("expected delete of line2:\n%s", out)
	}
	if !strings.Contains(out, "+line2-changed") {
		t.Fatalf("expected insert:\n%s", out)
	}
	if !strings.Contains(out, "+line4") {
		t.Fatalf("expected insert line4:\n%s", out)
	}
	if !strings.Contains(out, " line1") {
		t.Fatalf("expected equal line1:\n%s", out)
	}
}
