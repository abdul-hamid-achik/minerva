package session

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/abdul-hamid-achik/minerva/internal/harness"
)

// makeHermesDB builds a state.db with the columns Minerva reads, shaped like
// Hermes's real schema (sessions + messages, tool_calls as JSON text).
func makeHermesDB(t *testing.T, path string) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := float64(time.Now().Unix())
	stmts := []string{
		`CREATE TABLE sessions (id TEXT PRIMARY KEY, source TEXT NOT NULL, model TEXT,
			started_at REAL NOT NULL, ended_at REAL, cwd TEXT)`,
		`CREATE TABLE messages (id INTEGER PRIMARY KEY AUTOINCREMENT, session_id TEXT NOT NULL,
			role TEXT NOT NULL, content TEXT, tool_call_id TEXT, tool_calls TEXT,
			timestamp REAL NOT NULL, _compressed_summary INTEGER NOT NULL DEFAULT 0)`,
	}
	for _, q := range stmts {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	mustExec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.Exec(q, args...); err != nil {
			t.Fatal(err)
		}
	}
	mustExec(`INSERT INTO sessions VALUES ('20260718_100000_abc', 'cli', 'm1', ?, NULL, '/work/demo')`, now-60)
	msgs := [][]any{
		{"user", `[{"type":"text","text":"run the tests"},{"type":"image_url","image_url":{"url":"data:x"}}]`, nil, nil},
		{"assistant", "", nil, `[{"id":"c1","call_id":"c1","type":"function","function":{"name":"terminal","arguments":"{\"command\":\"make test\"}"}},{"id":"c2","type":"function","function":{"name":"skill_view","arguments":"{\"name\":\"demo\"}"}}]`},
		{"tool", `{"output":"FAIL","exit_code":2}`, "c1", nil},
		{"tool", `{"success":true}`, "c2", nil},
	}
	for i, m := range msgs {
		mustExec(`INSERT INTO messages (session_id, role, content, tool_call_id, tool_calls, timestamp) VALUES ('20260718_100000_abc', ?, ?, ?, ?, ?)`,
			m[0], m[1], m[2], m[3], now-50+float64(i))
	}
	mustExec(`INSERT INTO messages (session_id, role, content, timestamp, _compressed_summary) VALUES ('20260718_100000_abc', 'user', 'summary of earlier turns', ?, 1)`, now)
}

func TestHermesDB_ListLoadAndDedupe(t *testing.T) {
	home := t.TempDir()
	hdir := filepath.Join(home, ".hermes")
	if err := os.MkdirAll(filepath.Join(hdir, "sessions"), 0o755); err != nil {
		t.Fatal(err)
	}
	makeHermesDB(t, filepath.Join(hdir, "state.db"))
	// The same session's older gateway log must not be listed twice.
	writeFileT(t, filepath.Join(hdir, "sessions", "20260718_100000_abc.jsonl"), `{"role":"user","content":"old copy"}`+"\n")

	env := harness.Env{Home: home, AgentsDir: filepath.Join(home, ".agents"), Now: time.Now()}
	got, err := LoadFiltered(env, Filter{Harness: harness.Hermes, Since: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("sessions = %d, want 1 (deduplicated)", len(got))
	}
	s := got[0]
	if s.ID != "20260718_100000_abc" || s.Workspace != "/work/demo" || s.Model != "m1" || s.StartedAt.IsZero() {
		t.Fatalf("metadata = %+v", s)
	}
	if s.FirstUserPrompt() != "run the tests" {
		t.Fatalf("prompt = %q", s.FirstUserPrompt())
	}
	calls := allCalls(s)
	if len(calls) != 2 || !calls[0].IsError || calls[1].IsError {
		t.Fatalf("calls = %+v", calls)
	}
	if len(s.Skills) != 1 || s.Skills[0] != "demo" {
		t.Fatalf("skills = %v", s.Skills)
	}
	for _, turn := range s.Turns {
		if turn.Text == "summary of earlier turns" {
			t.Fatal("compression summary was read as a user turn")
		}
	}
}
