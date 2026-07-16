package profile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAddSkills_Merges(t *testing.T) {
	dir := t.TempDir()
	mgr := NewManager(dir)
	p := &Profile{Name: "dev", Skills: []string{"a", "b"}, SystemPrompt: "hi"}
	if err := mgr.Create(p); err != nil {
		t.Fatal(err)
	}
	if err := mgr.AddSkills("dev", []string{"b", "c"}); err != nil {
		t.Fatal(err)
	}
	// Reload from disk to ensure persistence
	mgr2 := NewManager(dir)
	if err := mgr2.LoadAll(); err != nil {
		t.Fatal(err)
	}
	got := mgr2.Get("dev")
	if got == nil {
		t.Fatal("missing profile")
	}
	if len(got.Skills) != 3 {
		t.Fatalf("skills=%v want a,b,c", got.Skills)
	}
	want := map[string]bool{"a": true, "b": true, "c": true}
	for _, s := range got.Skills {
		if !want[s] {
			t.Fatalf("unexpected skill %q in %v", s, got.Skills)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "agents", "dev", "agent.yaml")); err != nil {
		t.Fatal(err)
	}
}
