// Package session parses harness transcripts into a shared Trace model.
package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/abdul-hamid-achik/minerva/internal/harness"
	"github.com/abdul-hamid-achik/minerva/internal/secret"
)

// Category classifies a tool call.
type Category string

const (
	CatShell    Category = "shell"
	CatRead     Category = "read"
	CatEdit     Category = "edit"
	CatSearch   Category = "search"
	CatMCP      Category = "mcp"
	CatSubagent Category = "subagent"
	CatSkill    Category = "skill"
	CatOther    Category = "other"
)

// ToolCall is one normalized tool invocation.
type ToolCall struct {
	Name     string   `json:"name"`
	Args     string   `json:"args,omitempty"`
	Result   string   `json:"result,omitempty"`
	IsError  bool     `json:"is_error,omitempty"`
	Category Category `json:"category"`
}

// Turn is one user or assistant step.
type Turn struct {
	Role          string     `json:"role"`
	Text          string     `json:"text,omitempty"`
	ToolCalls     []ToolCall `json:"tool_calls,omitempty"`
	SkillsInvoked []string   `json:"skills_invoked,omitempty"`
}

// Session is a normalized conversation.
type Session struct {
	Harness   string    `json:"harness"`
	ID        string    `json:"id"`
	Path      string    `json:"path"`
	Workspace string    `json:"workspace,omitempty"`
	Model     string    `json:"model,omitempty"`
	StartedAt time.Time `json:"started_at,omitempty"`
	MTime     time.Time `json:"mtime"`
	Turns     []Turn    `json:"turns,omitempty"`
	TurnCount int       `json:"turn_count"`
	ToolCount int       `json:"tool_count"`
	Skills    []string  `json:"skills,omitempty"`
}

// Filter selects sessions.
type Filter struct {
	Harness   string
	Workspace string
	Since     time.Duration
	Limit     int
	SessionID string
}

// List walks harness session globs and returns metadata (no turn bodies).
func List(env harness.Env, filter Filter) ([]Session, error) {
	var out []Session
	for _, h := range harness.Catalog(env) {
		if filter.Harness != "" && h.ID != filter.Harness {
			continue
		}
		for _, g := range h.SessionGlobs {
			matches, _ := filepath.Glob(g)
			for _, p := range matches {
				st, err := os.Stat(p)
				if err != nil {
					continue
				}
				if st.IsDir() {
					// directory sources (opencode/copilot): keep as a stub session
					id := filepath.Base(p)
					sess := Session{Harness: h.ID, ID: id, Path: p, MTime: st.ModTime()}
					if keep(env, filter, sess) {
						out = append(out, sess)
					}
					continue
				}
				if !strings.HasSuffix(strings.ToLower(p), ".jsonl") && !strings.HasSuffix(strings.ToLower(p), ".json") {
					if filepath.Base(p) == "sonar.db" {
						sess := Session{Harness: h.ID, ID: "sonar.db", Path: p, MTime: st.ModTime()}
						if keep(env, filter, sess) {
							out = append(out, sess)
						}
					}
					continue
				}
				id := sessionIDFromPath(h.ID, p)
				sess := Session{Harness: h.ID, ID: id, Path: p, MTime: st.ModTime()}
				if keep(env, filter, sess) {
					out = append(out, sess)
				}
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].MTime.After(out[j].MTime)
	})
	if filter.Limit > 0 && len(out) > filter.Limit {
		out = out[:filter.Limit]
	}
	return out, nil
}

func keep(env harness.Env, f Filter, s Session) bool {
	if f.SessionID != "" && s.ID != f.SessionID && !strings.HasPrefix(s.ID, f.SessionID) && !strings.Contains(s.Path, f.SessionID) {
		return false
	}
	if f.Since > 0 && env.Now.Sub(s.MTime) > f.Since {
		return false
	}
	return true
}

func sessionIDFromPath(harnessID, path string) string {
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	if harnessID == harness.Cursor {
		return base
	}
	if harnessID == harness.Claude {
		return base
	}
	if harnessID == harness.Codex {
		return base
	}
	return base
}

// Load parses a session file into a full Trace.
func Load(s Session) (Session, error) {
	switch s.Harness {
	case harness.Claude:
		return parseClaude(s)
	case harness.Cursor:
		return parseCursor(s)
	case harness.Codex:
		return parseCodex(s)
	case harness.OpenCode:
		return parseOpenCode(s)
	case harness.Copilot:
		return parseCopilot(s)
	case harness.Gemini:
		return parseGemini(s)
	default:
		return s, nil
	}
}

// LoadFiltered lists then fully parses matching sessions (workspace filter applied after parse).
func LoadFiltered(env harness.Env, filter Filter) ([]Session, error) {
	meta, err := List(env, filter)
	if err != nil {
		return nil, err
	}
	var out []Session
	for _, m := range meta {
		full, err := Load(m)
		if err != nil {
			continue
		}
		if filter.Workspace != "" && !workspaceMatch(full.Workspace, filter.Workspace) {
			continue
		}
		summarize(&full)
		out = append(out, full)
	}
	return out, nil
}

func workspaceMatch(got, want string) bool {
	if got == "" || want == "" {
		return false
	}
	got = filepath.Clean(got)
	want = filepath.Clean(want)
	return got == want || strings.HasSuffix(got, want) || strings.Contains(got, want)
}

func summarize(s *Session) {
	s.TurnCount = len(s.Turns)
	seen := map[string]bool{}
	tools := 0
	for _, t := range s.Turns {
		tools += len(t.ToolCalls)
		for _, name := range t.SkillsInvoked {
			if !seen[name] {
				seen[name] = true
				s.Skills = append(s.Skills, name)
			}
		}
		for _, tc := range t.ToolCalls {
			if tc.Category == CatSkill && tc.Args != "" {
				var obj map[string]any
				if json.Unmarshal([]byte(tc.Args), &obj) == nil {
					if n, ok := obj["skill"].(string); ok && n != "" && !seen[n] {
						seen[n] = true
						s.Skills = append(s.Skills, n)
					}
				}
			}
		}
	}
	s.ToolCount = tools
	sort.Strings(s.Skills)
}

// RedactSession strips secret-like values from texts and args.
func RedactSession(s *Session) {
	s.Workspace = secret.Redact(s.Workspace)
	for i := range s.Turns {
		s.Turns[i].Text = secret.Redact(s.Turns[i].Text)
		for j := range s.Turns[i].ToolCalls {
			s.Turns[i].ToolCalls[j].Args = secret.Redact(s.Turns[i].ToolCalls[j].Args)
			s.Turns[i].ToolCalls[j].Result = secret.Redact(s.Turns[i].ToolCalls[j].Result)
		}
	}
}

// CategoryOf classifies a raw tool name.
func CategoryOf(name string) Category {
	return categorize(name)
}

func categorize(name string) Category {
	n := strings.ToLower(name)
	switch {
	case n == "skill" || n == "load_skill" || strings.Contains(n, "skill"):
		return CatSkill
	case n == "bash" || n == "shell" || n == "exec_command" || n == "run_terminal_cmd":
		return CatShell
	case n == "read" || n == "readfile" || strings.HasPrefix(n, "read_"):
		return CatRead
	case n == "edit" || n == "write" || n == "strreplace" || strings.Contains(n, "edit") || n == "apply_patch":
		return CatEdit
	case n == "grep" || n == "glob" || n == "semanticsearch" || strings.Contains(n, "search"):
		return CatSearch
	case strings.Contains(n, "mcp") || strings.Contains(n, "__"):
		return CatMCP
	case n == "agent" || n == "task" || strings.Contains(n, "subagent"):
		return CatSubagent
	default:
		return CatOther
	}
}

func compactJSON(v any) string {
	if v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	default:
		b, err := json.Marshal(t)
		if err != nil {
			return ""
		}
		s := string(b)
		if len(s) > 2000 {
			return s[:2000]
		}
		return s
	}
}

// FirstUserPrompt returns the first non-empty user turn text.
func (s Session) FirstUserPrompt() string {
	for _, t := range s.Turns {
		if t.Role == "user" && strings.TrimSpace(t.Text) != "" {
			return t.Text
		}
	}
	return ""
}
