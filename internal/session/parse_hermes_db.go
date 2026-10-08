package session

import (
	"database/sql"
	"encoding/json"
	"math"
	"net/url"
	"strings"
	"time"

	"github.com/abdul-hamid-achik/minerva/internal/harness"
	_ "modernc.org/sqlite" // pure-Go driver keeps CGO_ENABLED=0 builds
)

// hermesDBName is the SQLite store current Hermes versions keep every
// session in (~/.hermes/state.db). Gateway JSONL logs are older copies.
const hermesDBName = "state.db"

// openHermesDB opens state.db read-only. Hermes keeps it in WAL mode, so the
// connection must still read the -wal file to see recent messages; mode=ro
// does that, whereas immutable=1 would not.
func openHermesDB(path string) (*sql.DB, error) {
	u := url.URL{Scheme: "file", Path: path, RawQuery: "mode=ro&_pragma=busy_timeout(2000)"}
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	return db, nil
}

// listHermesDB returns one stub per session in state.db. MTime is the last
// message time, so --since keeps sessions that are still active.
func listHermesDB(path string) ([]Session, error) {
	db, err := openHermesDB(path)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.Query(`
		SELECT s.id, s.started_at, COALESCE(s.model, ''), COALESCE(s.cwd, ''),
		       COALESCE(MAX(m.timestamp), s.ended_at, s.started_at)
		FROM sessions s LEFT JOIN messages m ON m.session_id = s.id
		GROUP BY s.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Session
	for rows.Next() {
		var (
			id, model, cwd string
			started, last  float64
		)
		if err := rows.Scan(&id, &started, &model, &cwd, &last); err != nil {
			return nil, err
		}
		out = append(out, Session{
			Harness: harness.Hermes, ID: id, Path: path, Model: model, Workspace: cwd,
			StartedAt: unixFloat(started), MTime: unixFloat(last),
		})
	}
	return out, rows.Err()
}

// parseHermesDB loads one session's messages from state.db. Rows have the
// same shape as gateway JSONL lines (tool_calls is JSON text), so they go
// through hermesMessage.
func parseHermesDB(s Session) (Session, error) {
	db, err := openHermesDB(s.Path)
	if err != nil {
		return s, err
	}
	defer db.Close()
	var cwd, model sql.NullString
	var started sql.NullFloat64
	err = db.QueryRow(`SELECT cwd, model, started_at FROM sessions WHERE id = ?`, s.ID).Scan(&cwd, &model, &started)
	if err != nil {
		return s, err
	}
	s.Workspace, s.Model = cwd.String, model.String
	if started.Valid {
		s.StartedAt = unixFloat(started.Float64)
	}
	rows, err := db.Query(`
		SELECT role, COALESCE(content, ''), COALESCE(tool_call_id, ''), COALESCE(tool_calls, '')
		FROM messages WHERE session_id = ? AND _compressed_summary = 0 ORDER BY id`, s.ID)
	if err != nil {
		return s, err
	}
	defer rows.Close()
	calls := callIndex{}
	for rows.Next() {
		var role, content, toolCallID, toolCalls string
		if err := rows.Scan(&role, &content, &toolCallID, &toolCalls); err != nil {
			return s, err
		}
		msg := map[string]any{"role": role, "content": content, "tool_call_id": toolCallID}
		if role == "user" {
			msg["content"] = decodeParts(content)
		}
		if toolCalls != "" {
			var list []any
			if json.Unmarshal([]byte(toolCalls), &list) == nil {
				msg["tool_calls"] = list
			}
		}
		hermesMessage(&s, calls, msg)
	}
	return s, rows.Err()
}

// decodeParts turns a stored multimodal content list ("[{"type":"text",…}]")
// back into parts; any other content stays a string.
func decodeParts(content string) any {
	if !strings.HasPrefix(content, "[") {
		return content
	}
	var parts []any
	if json.Unmarshal([]byte(content), &parts) != nil {
		return content
	}
	return parts
}

func unixFloat(sec float64) time.Time {
	if sec <= 0 {
		return time.Time{}
	}
	whole, frac := math.Modf(sec)
	return time.Unix(int64(whole), int64(frac*1e9))
}
