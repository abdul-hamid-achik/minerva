// Package session parses harness transcripts into a shared Trace model.
package session

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

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
	// Command is the leading executable of a shell call (git, go, npm, …).
	// Empty for non-shell tools or when no command could be extracted.
	Command string `json:"command,omitempty"`
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
// A workspace is only known after parsing, so with filter.Workspace set
// sessions are parsed (newest first) until filter.Limit of them match.
func List(env harness.Env, filter Filter) ([]Session, error) {
	if filter.Workspace == "" {
		return listStubs(env, filter)
	}
	full, err := LoadFiltered(env, filter)
	for i := range full {
		full[i].Turns = nil
	}
	return full, err
}

// listStubs lists sessions from their files alone. It applies every filter
// but Workspace.
func listStubs(env harness.Env, filter Filter) ([]Session, error) {
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
				if h.ID == harness.Hermes && filepath.Base(p) == hermesDBName {
					stubs, err := listHermesDB(p)
					if err != nil {
						continue
					}
					for _, sess := range stubs {
						if keep(env, filter, sess) {
							out = append(out, sess)
						}
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
	out = dropHermesLogCopies(out)
	sort.Slice(out, func(i, j int) bool {
		return out[i].MTime.After(out[j].MTime)
	})
	if filter.Limit > 0 && len(out) > filter.Limit {
		out = out[:filter.Limit]
	}
	return out, nil
}

// dropHermesLogCopies removes Hermes gateway JSONL sessions that state.db
// also holds, so a session is not analyzed twice.
func dropHermesLogCopies(in []Session) []Session {
	inDB := map[string]bool{}
	for _, s := range in {
		if s.Harness == harness.Hermes && filepath.Base(s.Path) == hermesDBName {
			inDB[s.ID] = true
		}
	}
	if len(inDB) == 0 {
		return in
	}
	out := in[:0]
	for _, s := range in {
		if s.Harness == harness.Hermes && filepath.Base(s.Path) != hermesDBName && inDB[s.ID] {
			continue
		}
		out = append(out, s)
	}
	return out
}

func keep(env harness.Env, f Filter, s Session) bool {
	if f.SessionID != "" && !matchesSessionID(s, f.SessionID) {
		return false
	}
	if f.Since > 0 && env.Now.Sub(s.MTime) > f.Since {
		return false
	}
	return true
}

// matchesSessionID reports whether q is a prefix of the session id or of an
// id-like part of its file name. Codex names files rollout-<time>-<uuid>, so
// a uuid prefix matches after a dash; directory names never match.
func matchesSessionID(s Session, q string) bool {
	if strings.HasPrefix(s.ID, q) {
		return true
	}
	stem := strings.TrimSuffix(filepath.Base(s.Path), filepath.Ext(s.Path))
	return strings.HasPrefix(stem, q) || strings.Contains(stem, "-"+q)
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
	case harness.Hermes:
		return parseHermes(s)
	case harness.OMP:
		return parseOMP(s)
	default:
		return s, nil
	}
}

// LoadFiltered lists then fully parses matching sessions, newest first. The
// workspace filter needs the parsed session, so with a workspace the limit
// counts matching sessions rather than files.
func LoadFiltered(env harness.Env, filter Filter) ([]Session, error) {
	stubFilter := filter
	if filter.Workspace != "" {
		stubFilter.Limit = 0
	}
	meta, err := listStubs(env, stubFilter)
	if err != nil {
		return nil, err
	}
	var out []Session
	for _, m := range meta {
		if filter.Limit > 0 && len(out) >= filter.Limit {
			break
		}
		// Skip a session whose location already shows another workspace;
		// parsing every transcript to find a few takes tens of seconds.
		if filter.Workspace != "" {
			if ws := stubWorkspace(m); ws != "" && !workspaceMatch(ws, filter.Workspace) {
				continue
			}
		}
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
	if !strings.Contains(got, "/") {
		// Cursor only records its project slug: the path with every
		// separator turned into "-" (Users-me-projects-app).
		g, w := pathSlug(got), pathSlug(want)
		return w != "" && (g == w || strings.HasSuffix(g, "-"+w))
	}
	got = filepath.Clean(got)
	want = filepath.Clean(want)
	return got == want || strings.HasSuffix(got, want) || strings.Contains(got, want)
}

// stubWorkspace returns the workspace a session's file location already
// tells, without parsing the session, or "" when only parsing can. Claude
// and Cursor name the project dir after the workspace path; Codex and omp
// open with an entry holding cwd; an OpenCode session file has directory.
func stubWorkspace(s Session) string {
	switch {
	case s.Workspace != "":
		return s.Workspace
	case s.Harness == harness.Claude:
		// Claude names the dir after the launch cwd, "/" → "-"; a dir
		// that does not look like that tells nothing.
		if dir := filepath.Base(filepath.Dir(s.Path)); strings.HasPrefix(dir, "-") {
			return dir
		}
		return ""
	case s.Harness == harness.Cursor:
		return inferCursorWorkspace(s.Path)
	case s.Harness == harness.Codex:
		return peekJSONL(s.Path, "session_meta", func(obj map[string]any) any { return obj["payload"] })
	case s.Harness == harness.OMP:
		return peekJSONL(s.Path, "session", func(obj map[string]any) any { return obj })
	case s.Harness == harness.OpenCode && strings.EqualFold(filepath.Ext(s.Path), ".json"):
		return peekOpenCodeDir(s.Path)
	}
	return ""
}

// peekJSONL reads the first few lines of a transcript for the entry of the
// given type and returns the "cwd" of the object holder picks from it.
func peekJSONL(path, typ string, holder func(map[string]any) any) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	r := bufio.NewReaderSize(f, 64*1024)
	for i := 0; i < 5; i++ {
		line, tooLong, err := readLine(r, maxJSONLLine)
		var obj map[string]any
		if !tooLong && json.Unmarshal(line, &obj) == nil && obj["type"] == typ {
			m, _ := holder(obj).(map[string]any)
			cwd, _ := m["cwd"].(string)
			return cwd
		}
		if err != nil {
			return ""
		}
	}
	return ""
}

// peekOpenCodeDir reads the directory from an OpenCode session file, which
// is small; the messages and parts live in other files.
func peekOpenCodeDir(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var meta struct {
		Directory string `json:"directory"`
	}
	if json.Unmarshal(data, &meta) != nil {
		return ""
	}
	return meta.Directory
}

// pathSlug lowercases p and turns every run of non-alphanumerics into one
// "-", so a path and the Cursor project slug of it compare equal.
func pathSlug(p string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(p) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			dash = false
		case !dash && b.Len() > 0:
			b.WriteByte('-')
			dash = true
		}
	}
	return strings.TrimSuffix(b.String(), "-")
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
		for j := range t.ToolCalls {
			tc := &t.ToolCalls[j]
			if tc.Category == CatShell && tc.Command == "" {
				tc.Command = ShellCommand(tc.Args)
			}
			if tc.Category == CatSkill && tc.Args != "" {
				var obj map[string]any
				if json.Unmarshal([]byte(tc.Args), &obj) == nil {
					if n, ok := obj["skill"].(string); ok && n != "" && !seen[bareSkillName(n)] {
						n = bareSkillName(n)
						seen[n] = true
						s.Skills = append(s.Skills, n)
					}
				}
			}
			if tc.Category == CatRead {
				if n := skillFromRead(tc.Args); n != "" && !seen[n] {
					seen[n] = true
					s.Skills = append(s.Skills, n)
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

// IsBrowse reports whether a category only inspects state (read/search).
func IsBrowse(cat Category) bool {
	return cat == CatRead || cat == CatSearch
}

// BrowseOnly reports whether every tool name is a read or search tool.
// A sequence of pure browsing is not a skill candidate.
func BrowseOnly(names ...string) bool {
	if len(names) == 0 {
		return false
	}
	for _, name := range names {
		if !IsBrowse(categorize(name)) {
			return false
		}
	}
	return true
}

func categorize(name string) Category {
	raw := strings.ToLower(strings.TrimSpace(name))
	if strings.Contains(raw, "__") || strings.HasPrefix(raw, "mcp_") || strings.HasPrefix(raw, "mcp__") {
		// Gateway-namespaced tools (server__tool) are MCP regardless of suffix.
		return CatMCP
	}
	n := strings.ReplaceAll(strings.ReplaceAll(raw, "-", ""), "_", "")
	switch {
	case n == "skill" || n == "loadskill" || strings.Contains(n, "skill"):
		return CatSkill
	case n == "bash" || n == "shell" || n == "execcommand" || n == "runterminalcmd" || n == "runshellcommand" || n == "terminal" || n == "powershell":
		return CatShell
	case n == "read" || n == "readfile" || n == "readmanyfiles" || n == "cat" || n == "view" || n == "viewfile" || n == "ls" || n == "list" || n == "listdir" || n == "listdirectory" || n == "readlints" || n == "webfetch" || n == "fetch" || strings.HasPrefix(n, "read"):
		return CatRead
	case n == "edit" || n == "write" || n == "strreplace" || n == "replace" || n == "writefile" || n == "createfile" || n == "delete" || n == "deletefile" || strings.Contains(n, "edit") || n == "applypatch":
		return CatEdit
	case n == "grep" || n == "glob" || n == "find" || n == "rg" || n == "semanticsearch" || n == "codebasesearch" || n == "websearch" || strings.Contains(n, "search"):
		return CatSearch
	case strings.Contains(n, "mcp"):
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
		// Cap long string values rather than the encoded text, so the
		// result stays valid JSON and a long command keeps its executable.
		b, err := json.Marshal(capStrings(t, 2000))
		if err != nil {
			return ""
		}
		return string(b)
	}
}

// capStrings returns v with every string value cut to at most n bytes.
func capStrings(v any, n int) any {
	switch t := v.(type) {
	case string:
		return truncate(t, n)
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, x := range t {
			out[k] = capStrings(x, n)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, x := range t {
			out[i] = capStrings(x, n)
		}
		return out
	default:
		return v
	}
}

// shellNoise are commands that never define a workflow on their own.
var shellNoise = map[string]bool{
	"cd": true, "ls": true, "cat": true, "echo": true, "pwd": true, "mkdir": true,
	"rm": true, "cp": true, "mv": true, "head": true, "tail": true, "sed": true,
	"awk": true, "grep": true, "rg": true, "find": true, "true": true, "false": true,
	"sleep": true, "which": true, "test": true, "wc": true, "sort": true, "uniq": true,
	"touch": true, "printf": true, "export": true, "set": true, "exit": true, "tee": true,
	"tr": true, "cut": true, "xargs": true, "date": true, "env": true, "sudo": true,
	"time": true, "nohup": true, "command": true, "builtin": true, "eval": true,
	"kill": true, "killall": true, "pkill": true, "ps": true, "lsof": true, "open": true,
	// shell syntax that can lead a segment
	"for": true, "while": true, "until": true, "if": true, "then": true, "else": true,
	"elif": true, "fi": true, "do": true, "done": true, "case": true, "esac": true,
	"function": true, "return": true, "local": true, "declare": true, "source": true,
	".": true, "{": true, "}": true, "[": true, "[[": true, "!": true,
}

// shellWrappers precede the real executable and are skipped, flags included.
var shellWrappers = map[string]bool{
	"sudo": true, "env": true, "time": true, "nohup": true, "command": true, "exec": true,
	"builtin": true, "do": true, "then": true, "else": true, "{": true, "!": true, "timeout": true,
}

// shellAliases folds interpreter/version variants into a family.
var shellAliases = map[string]string{
	"python3": "python", "python2": "python", "py": "python",
	"pip3": "pip", "node.exe": "node", "npx": "npm", "pnpx": "pnpm", "bunx": "bun",
	"go.exe": "go", "git.exe": "git",
}

// ShellCommand extracts the leading executable from a shell tool's args.
// Args may be a JSON object ({"command": "git status"}, {"cmd": [...]}) or a
// raw command line. Environment assignments, sudo/env/time wrappers, and
// leading `cd dir &&` hops are skipped. Returns "" when nothing meaningful
// remains (pure ls/cat/echo pipelines).
func ShellCommand(args string) string {
	line := commandLine(args)
	if line == "" {
		return ""
	}
	// Walk each &&/;/| segment; the first non-noise executable wins.
	for _, seg := range splitShellSegments(line) {
		if strings.HasPrefix(strings.TrimSpace(seg), "#") {
			continue // comment
		}
		tok := leadingExecutable(seg)
		if tok == "" || shellNoise[tok] {
			continue
		}
		return tok
	}
	return ""
}

func commandLine(args string) string {
	args = strings.TrimSpace(args)
	if args == "" {
		return ""
	}
	if !strings.HasPrefix(args, "{") {
		return args
	}
	var obj map[string]any
	if json.Unmarshal([]byte(args), &obj) != nil {
		return ""
	}
	for _, key := range []string{"command", "cmd", "script", "commandLine", "command_line"} {
		switch v := obj[key].(type) {
		case string:
			if strings.TrimSpace(v) != "" {
				return v
			}
		case []any:
			var parts []string
			for _, p := range v {
				if s, ok := p.(string); ok {
					parts = append(parts, s)
				}
			}
			if len(parts) > 0 {
				return strings.Join(parts, " ")
			}
		}
	}
	if v, ok := obj["args"].([]any); ok {
		var parts []string
		for _, p := range v {
			if s, ok := p.(string); ok {
				parts = append(parts, s)
			}
		}
		if len(parts) > 0 {
			return strings.Join(parts, " ")
		}
	}
	return ""
}

func splitShellSegments(line string) []string {
	var out []string
	var cur strings.Builder
	flush := func() {
		if s := strings.TrimSpace(cur.String()); s != "" {
			out = append(out, s)
		}
		cur.Reset()
	}
	for i := 0; i < len(line); i++ {
		c := line[i]
		switch {
		case c == '\n' || c == ';':
			flush()
		case c == '&' && i+1 < len(line) && line[i+1] == '&':
			flush()
			i++
		case c == '|':
			flush()
			if i+1 < len(line) && line[i+1] == '|' {
				i++
			}
		default:
			cur.WriteByte(c)
		}
	}
	flush()
	return out
}

func leadingExecutable(seg string) string {
	fields := strings.Fields(seg)
	for len(fields) > 0 {
		tok := strings.Trim(fields[0], `"'`+"`()")
		// bash -lc "…" / sh -c "…": recurse into the quoted command.
		if (tok == "bash" || tok == "sh" || tok == "zsh") && len(fields) >= 3 && strings.HasPrefix(fields[1], "-") && strings.Contains(fields[1], "c") {
			inner := strings.Join(fields[2:], " ")
			inner = strings.Trim(inner, `"'`)
			return ShellCommand(inner)
		}
		switch {
		case tok == "":
			fields = fields[1:]
		case strings.Contains(tok, "=") && !strings.HasPrefix(tok, "-"):
			// FOO=bar prefix
			fields = fields[1:]
		case shellWrappers[tok]:
			fields = fields[1:]
			// skip option flags for wrappers (env -i, sudo -E)
			for len(fields) > 0 && strings.HasPrefix(fields[0], "-") {
				fields = fields[1:]
			}
		default:
			tok = strings.ToLower(filepath.Base(tok))
			if alias, ok := shellAliases[tok]; ok {
				tok = alias
			}
			return tok
		}
	}
	return ""
}

// maxSinceDays keeps a day count far from overflowing time.Duration.
const maxSinceDays = 100 * 365

// ParseSince accepts Go durations (24h, 90m) or day counts (7d).
func ParseSince(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}
	if d, err := time.ParseDuration(s); err == nil {
		if d < 0 {
			return 0, fmt.Errorf("invalid since %q: must not be negative", s)
		}
		return d, nil
	}
	if n, ok := strings.CutSuffix(s, "d"); ok {
		if days, err := strconv.Atoi(n); err == nil && days >= 0 && days <= maxSinceDays {
			return time.Duration(days) * 24 * time.Hour, nil
		}
	}
	return 0, fmt.Errorf("invalid since %q (use 24h or 7d)", s)
}

// MaxPromptBytes bounds the text FirstUserPrompt returns.
const MaxPromptBytes = 1500

// FirstUserPrompt returns the first user turn that reads as an actual
// request. Harnesses prepend injected context (AGENTS.md instructions,
// <environment_context>, <user_info>, …) as user turns; those are stripped or
// skipped. When a <user_query> wrapper is present, only its body is used.
func (s Session) FirstUserPrompt() string {
	for _, t := range s.Turns {
		if t.Role != "user" {
			continue
		}
		if p := CleanPrompt(t.Text); p != "" {
			return p
		}
	}
	return ""
}

var (
	userQueryRe  = regexp.MustCompile(`(?s)<user_query>\s*(.*?)\s*</user_query>`)
	xmlBlockRe   = regexp.MustCompile(`(?s)<([a-z][a-z0-9_-]*)(?:\s[^>]*)?>.*?</[a-z][a-z0-9_-]*>`)
	xmlOrphanRe  = regexp.MustCompile(`(?s)^\s*<[a-z][a-z0-9_-]*(?:\s[^>]*)?>`)
	injectedHead = []string{"# agents.md", "<environment_context>", "<user_instructions>", "# claude.md", "<system-reminder>", "<system_reminder>"}
)

// CleanPrompt strips harness-injected wrappers from a user turn and returns
// the human request, capped at MaxPromptBytes. Returns "" when nothing
// human remains.
func CleanPrompt(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	if m := userQueryRe.FindStringSubmatch(text); len(m) > 1 {
		return capPrompt(m[1])
	}
	low := strings.ToLower(text)
	for _, h := range injectedHead {
		if strings.HasPrefix(low, h) {
			return ""
		}
	}
	// Drop <tag>…</tag> blocks (attached files, git status, rules, …).
	cleaned := xmlBlockRe.ReplaceAllString(text, " ")
	cleaned = xmlOrphanRe.ReplaceAllString(cleaned, "")
	cleaned = strings.TrimSpace(cleaned)
	if cleaned == "" {
		return ""
	}
	return capPrompt(cleaned)
}

func capPrompt(p string) string {
	p = strings.TrimSpace(p)
	if len(p) > MaxPromptBytes {
		p = p[:MaxPromptBytes]
		// don't cut a UTF-8 sequence in half
		for len(p) > 0 && !utf8.ValidString(p) {
			p = p[:len(p)-1]
		}
	}
	return p
}
