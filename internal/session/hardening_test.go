package session

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/abdul-hamid-achik/minerva/internal/harness"
)

func mkdirT(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestOpenCode_IDsCannotLeaveStorage(t *testing.T) {
	root := t.TempDir()
	storage := filepath.Join(root, "storage")
	sessDir := filepath.Join(storage, "session", "proj")
	mkdirT(t, sessDir)
	// The session id climbs to root/outside, where a planted message waits.
	mkdirT(t, filepath.Join(root, "outside"))
	writeFileT(t, filepath.Join(root, "outside", "m.json"), `{"role":"user","summary":{"title":"LEAK session"}}`)
	sessPath := filepath.Join(sessDir, "ses_x.json")
	writeFileT(t, sessPath, `{"id":"../../outside","directory":"/w"}`)

	// A real message whose id climbs to root/parts-outside.
	mkdirT(t, filepath.Join(storage, "message", "ses_x"))
	writeFileT(t, filepath.Join(storage, "message", "ses_x", "m1.json"), `{"id":"../../parts-outside","role":"assistant"}`)
	mkdirT(t, filepath.Join(root, "parts-outside"))
	writeFileT(t, filepath.Join(root, "parts-outside", "p.json"), `{"type":"text","text":"LEAK part"}`)
	mkdirT(t, filepath.Join(storage, "part", "m1"))
	writeFileT(t, filepath.Join(storage, "part", "m1", "p.json"), `{"type":"text","text":"ok"}`)

	s, err := Load(Session{Harness: harness.OpenCode, Path: sessPath})
	if err != nil {
		t.Fatal(err)
	}
	if s.ID != "ses_x" {
		t.Fatalf("id=%q, want the file name", s.ID)
	}
	if len(s.Turns) != 1 || s.Turns[0].Text != "ok" {
		t.Fatalf("turns=%+v, want only the in-storage part", s.Turns)
	}
	for _, id := range []string{"", ".", "..", "a/b", `a\b`, "a\x00"} {
		if safeID(id) {
			t.Errorf("safeID(%q) = true", id)
		}
	}
}

func TestList_EqualMTimesSortStably(t *testing.T) {
	home := t.TempDir()
	proj := filepath.Join(home, ".claude", "projects", "-w")
	mkdirT(t, proj)
	when := time.Now().Add(-time.Hour).Truncate(time.Second)
	// Odd sessions share the newer time, even ones the older, so sorting
	// must move entries; with over 12 of them sort.Slice is not stable.
	for i := range 60 {
		p := filepath.Join(proj, fmt.Sprintf("s%02d.jsonl", i))
		writeFileT(t, p, "{}\n")
		mt := when.Add(time.Duration(i%2) * time.Second)
		if err := os.Chtimes(p, mt, mt); err != nil {
			t.Fatal(err)
		}
	}
	env := harness.Env{Home: home, AgentsDir: filepath.Join(home, ".agents"), Now: time.Now()}
	got, err := List(env, Filter{Harness: harness.Claude})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 60 {
		t.Fatalf("got %d sessions", len(got))
	}
	for i, s := range got {
		want := fmt.Sprintf("s%02d", 2*i+1)
		if i >= 30 {
			want = fmt.Sprintf("s%02d", 2*(i-30))
		}
		if s.ID != want {
			t.Fatalf("got[%d]=%s, want %s", i, s.ID, want)
		}
	}
}

func TestParseJSONL_CountsSkippedLines(t *testing.T) {
	path := writeLines(t, "c.jsonl",
		`{"type":"user","message":{"role":"user","content":"hi"}}`,
		`{"type":"assistant","message":{"role":"assist`, // cut off mid-write
		``,
		`not json`,
	)
	s, err := Load(Session{Harness: harness.Claude, Path: path})
	if err != nil {
		t.Fatal(err)
	}
	if s.SkippedLines != 2 {
		t.Fatalf("skipped=%d, want 2", s.SkippedLines)
	}
	if len(s.Turns) != 1 {
		t.Fatalf("turns=%d", len(s.Turns))
	}
}
