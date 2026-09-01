package session

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func parseJSONL(path string, handle func(map[string]any)) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 8<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var obj map[string]any
		if json.Unmarshal([]byte(line), &obj) != nil {
			continue
		}
		handle(obj)
	}
	return sc.Err()
}

func parseClaude(s Session) (Session, error) {
	err := parseJSONL(s.Path, func(obj map[string]any) {
		if cwd, ok := obj["cwd"].(string); ok && s.Workspace == "" {
			s.Workspace = cwd
		}
		if id, ok := obj["sessionId"].(string); ok && id != "" {
			s.ID = id
		}
		if ts, ok := obj["timestamp"].(string); ok {
			if t, err := time.Parse(time.RFC3339Nano, ts); err == nil && s.StartedAt.IsZero() {
				s.StartedAt = t
			}
		}
		typ, _ := obj["type"].(string)
		msg, _ := obj["message"].(map[string]any)
		if msg == nil {
			return
		}
		role, _ := msg["role"].(string)
		if role == "" {
			role = typ
		}
		turn := Turn{Role: role}
		appendContent(&turn, msg["content"])
		if turn.Text != "" || len(turn.ToolCalls) > 0 {
			s.Turns = append(s.Turns, turn)
		}
	})
	return s, err
}

func parseCursor(s Session) (Session, error) {
	err := parseJSONL(s.Path, func(obj map[string]any) {
		role, _ := obj["role"].(string)
		msg, _ := obj["message"].(map[string]any)
		if msg == nil {
			return
		}
		if role == "" {
			role, _ = msg["role"].(string)
		}
		turn := Turn{Role: role}
		appendContent(&turn, msg["content"])
		if turn.Text != "" || len(turn.ToolCalls) > 0 {
			s.Turns = append(s.Turns, turn)
		}
	})
	// Infer workspace from path: .../projects/<slug>/agent-transcripts/...
	if s.Workspace == "" {
		s.Workspace = inferCursorWorkspace(s.Path)
	}
	return s, err
}

func inferCursorWorkspace(path string) string {
	parts := strings.Split(filepath.ToSlash(path), "/")
	for i, p := range parts {
		if p == "projects" && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return ""
}

func parseCodex(s Session) (Session, error) {
	err := parseJSONL(s.Path, func(obj map[string]any) {
		typ, _ := obj["type"].(string)
		payload, _ := obj["payload"].(map[string]any)
		if ts, ok := obj["timestamp"].(string); ok && s.StartedAt.IsZero() {
			if t, err := time.Parse(time.RFC3339Nano, ts); err == nil {
				s.StartedAt = t
			}
		}
		switch typ {
		case "session_meta":
			if payload != nil {
				if cwd, ok := payload["cwd"].(string); ok && s.Workspace == "" {
					s.Workspace = cwd
				}
				if id, ok := payload["session_id"].(string); ok && id != "" {
					s.ID = id
				}
				if mp, ok := payload["model_provider"].(string); ok {
					s.Model = mp
				}
			}
		case "response_item":
			if payload == nil {
				return
			}
			pt, _ := payload["type"].(string)
			switch pt {
			case "function_call", "custom_tool_call":
				name, _ := payload["name"].(string)
				args := compactJSON(payload["arguments"])
				if args == "" {
					args = compactJSON(payload["input"])
				}
				turn := Turn{Role: "assistant", ToolCalls: []ToolCall{{
					Name: name, Args: args, Category: categorize(name),
				}}}
				if strings.EqualFold(name, "skill") || categorize(name) == CatSkill {
					turn.SkillsInvoked = skillNamesFromArgs(args)
				}
				s.Turns = append(s.Turns, turn)
			case "function_call_output", "custom_tool_call_output":
				if len(s.Turns) == 0 {
					return
				}
				last := &s.Turns[len(s.Turns)-1]
				if len(last.ToolCalls) == 0 {
					return
				}
				out := compactJSON(payload["output"])
				if out == "" {
					out = compactJSON(payload)
				}
				last.ToolCalls[len(last.ToolCalls)-1].Result = truncate(out, 500)
				if errStr, ok := payload["error"].(string); ok && errStr != "" {
					last.ToolCalls[len(last.ToolCalls)-1].IsError = true
				}
			case "message":
				role, _ := payload["role"].(string)
				text := extractCodexText(payload)
				if text != "" {
					s.Turns = append(s.Turns, Turn{Role: role, Text: text})
				}
			}
		case "event_msg":
			if payload == nil {
				return
			}
			pt, _ := payload["type"].(string)
			if pt == "mcp_tool_call_end" {
				name, _ := payload["name"].(string)
				if name == "" {
					name, _ = payload["tool"].(string)
				}
				if name == "" {
					name = "mcp"
				}
				s.Turns = append(s.Turns, Turn{Role: "assistant", ToolCalls: []ToolCall{{
					Name: name, Category: CatMCP, IsError: payload["error"] != nil,
				}}})
			}
			if pt == "user_message" {
				text, _ := payload["message"].(string)
				if text == "" {
					text, _ = payload["text"].(string)
				}
				if text != "" {
					s.Turns = append(s.Turns, Turn{Role: "user", Text: text})
				}
			}
		}
	})
	return s, err
}

func extractCodexText(payload map[string]any) string {
	if c, ok := payload["content"].([]any); ok {
		var b strings.Builder
		for _, item := range c {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if t, ok := m["text"].(string); ok {
				b.WriteString(t)
			}
		}
		return b.String()
	}
	if t, ok := payload["text"].(string); ok {
		return t
	}
	return ""
}

func parseOpenCode(s Session) (Session, error) {
	st, err := os.Stat(s.Path)
	if err != nil {
		return s, err
	}
	if st.IsDir() {
		// Older listing treated a project dir as the session. Prefer a ses_*.json inside.
		entries, err := os.ReadDir(s.Path)
		if err != nil {
			return s, err
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
				continue
			}
			s.Path = filepath.Join(s.Path, e.Name())
			break
		}
		st, err = os.Stat(s.Path)
		if err != nil {
			return s, nil
		}
	}
	if st.IsDir() {
		return s, nil
	}

	data, err := os.ReadFile(s.Path)
	if err != nil {
		return s, err
	}
	var meta map[string]any
	if json.Unmarshal(data, &meta) == nil {
		if id, ok := meta["id"].(string); ok && id != "" {
			s.ID = id
		}
		if dir, ok := meta["directory"].(string); ok && dir != "" {
			s.Workspace = dir
		}
		if t, ok := meta["time"].(map[string]any); ok {
			if created, ok := t["created"].(float64); ok {
				s.StartedAt = time.UnixMilli(int64(created))
			}
		}
	}
	if s.ID == "" {
		s.ID = strings.TrimSuffix(filepath.Base(s.Path), filepath.Ext(s.Path))
	}

	storageRoot := filepath.Dir(filepath.Dir(filepath.Dir(s.Path))) // …/storage/session/<proj>/<file>
	if filepath.Base(filepath.Dir(filepath.Dir(s.Path))) != "session" {
		// file dropped next to message/part (tests)
		storageRoot = filepath.Dir(s.Path)
		if filepath.Base(storageRoot) == "session" {
			storageRoot = filepath.Dir(storageRoot)
		}
	}
	msgDir := filepath.Join(storageRoot, "message", s.ID)
	entries, err := os.ReadDir(msgDir)
	if err != nil {
		return s, nil
	}
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(msgDir, e.Name()))
		if err != nil {
			continue
		}
		var msg map[string]any
		if json.Unmarshal(raw, &msg) != nil {
			continue
		}
		role, _ := msg["role"].(string)
		if role == "" {
			role = "assistant"
		}
		if p, ok := msg["path"].(map[string]any); ok {
			if cwd, ok := p["cwd"].(string); ok && s.Workspace == "" {
				s.Workspace = cwd
			}
		}
		turn := Turn{Role: role}
		msgID, _ := msg["id"].(string)
		if msgID == "" {
			msgID = strings.TrimSuffix(e.Name(), ".json")
		}
		appendOpenCodeParts(&turn, filepath.Join(storageRoot, "part", msgID))
		if turn.Text == "" {
			if sum, ok := msg["summary"].(map[string]any); ok {
				turn.Text, _ = sum["title"].(string)
			}
		}
		if turn.Text != "" || len(turn.ToolCalls) > 0 {
			s.Turns = append(s.Turns, turn)
		}
	}
	return s, nil
}

func appendOpenCodeParts(turn *Turn, partDir string) {
	entries, err := os.ReadDir(partDir)
	if err != nil {
		return
	}
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(partDir, e.Name()))
		if err != nil {
			continue
		}
		var part map[string]any
		if json.Unmarshal(raw, &part) != nil {
			continue
		}
		switch part["type"] {
		case "text":
			if t, ok := part["text"].(string); ok && t != "" {
				if turn.Text != "" {
					turn.Text += "\n"
				}
				turn.Text += t
			}
		case "tool":
			name, _ := part["tool"].(string)
			if name == "" {
				name = "tool"
			}
			tc := ToolCall{Name: name, Category: categorize(name)}
			if state, ok := part["state"].(map[string]any); ok {
				tc.Args = compactJSON(state["input"])
				if out, ok := state["output"].(string); ok {
					tc.Result = truncate(out, 500)
				} else {
					tc.Result = truncate(compactJSON(state["output"]), 500)
				}
				if status, ok := state["status"].(string); ok && (status == "error" || status == "failed") {
					tc.IsError = true
				}
			}
			turn.ToolCalls = append(turn.ToolCalls, tc)
			if tc.Category == CatSkill {
				turn.SkillsInvoked = append(turn.SkillsInvoked, skillNamesFromArgs(tc.Args)...)
			}
		}
	}
}

func parseCopilot(s Session) (Session, error) {
	st, err := os.Stat(s.Path)
	if err != nil {
		return s, err
	}
	dir := s.Path
	if !st.IsDir() {
		dir = filepath.Dir(s.Path)
	}
	if data, err := os.ReadFile(filepath.Join(dir, "workspace.yaml")); err == nil {
		if ws := firstLineValue(string(data), "cwd:", "path:", "workspace:"); ws != "" {
			s.Workspace = ws
		}
	}
	eventsPath := filepath.Join(dir, "events.jsonl")
	if _, err := os.Stat(eventsPath); err != nil {
		if !st.IsDir() {
			return parseGenericJSON(s)
		}
		return s, nil
	}
	type pending struct {
		id   string
		turn int
		idx  int
	}
	var lastByID = map[string]pending{}
	err = parseJSONL(eventsPath, func(obj map[string]any) {
		typ, _ := obj["type"].(string)
		data, _ := obj["data"].(map[string]any)
		if data == nil {
			return
		}
		if ts, ok := obj["timestamp"].(string); ok && s.StartedAt.IsZero() {
			if t, err := time.Parse(time.RFC3339Nano, ts); err == nil {
				s.StartedAt = t
			}
		}
		switch typ {
		case "session.start":
			if ctx, ok := data["context"].(map[string]any); ok {
				if cwd, ok := ctx["cwd"].(string); ok && s.Workspace == "" {
					s.Workspace = cwd
				}
			}
			if id, ok := data["sessionId"].(string); ok && id != "" {
				s.ID = id
			}
		case "user.message":
			text, _ := data["content"].(string)
			if text != "" {
				s.Turns = append(s.Turns, Turn{Role: "user", Text: text})
			}
		case "assistant.message":
			text, _ := data["content"].(string)
			turn := Turn{Role: "assistant", Text: text}
			if reqs, ok := data["toolRequests"].([]any); ok {
				for _, r := range reqs {
					m, ok := r.(map[string]any)
					if !ok {
						continue
					}
					name, _ := m["name"].(string)
					if name == "" {
						name, _ = m["toolName"].(string)
					}
					if name != "" {
						turn.ToolCalls = append(turn.ToolCalls, ToolCall{
							Name: name, Args: compactJSON(m["arguments"]), Category: categorize(name),
						})
					}
				}
			}
			if turn.Text != "" || len(turn.ToolCalls) > 0 {
				s.Turns = append(s.Turns, turn)
			}
		case "tool.execution_start":
			name, _ := data["toolName"].(string)
			if name == "" {
				name, _ = data["name"].(string)
			}
			if name == "" {
				return
			}
			tc := ToolCall{Name: name, Args: compactJSON(data["arguments"]), Category: categorize(name)}
			turn := Turn{Role: "assistant", ToolCalls: []ToolCall{tc}}
			s.Turns = append(s.Turns, turn)
			if id, ok := data["toolCallId"].(string); ok && id != "" {
				lastByID[id] = pending{id: id, turn: len(s.Turns) - 1, idx: 0}
			}
		case "tool.execution_complete":
			id, _ := data["toolCallId"].(string)
			if p, ok := lastByID[id]; ok && p.turn < len(s.Turns) && p.idx < len(s.Turns[p.turn].ToolCalls) {
				tc := &s.Turns[p.turn].ToolCalls[p.idx]
				if success, ok := data["success"].(bool); ok && !success {
					tc.IsError = true
				}
				tc.Result = truncate(compactJSON(data["result"]), 500)
			}
		}
	})
	return s, err
}

func parseGemini(s Session) (Session, error) {
	return parseGenericJSON(s)
}

func parseGenericJSON(s Session) (Session, error) {
	data, err := os.ReadFile(s.Path)
	if err != nil {
		return s, err
	}
	var obj map[string]any
	if json.Unmarshal(data, &obj) != nil {
		return s, nil
	}
	if cwd, ok := obj["cwd"].(string); ok {
		s.Workspace = cwd
	}
	return s, nil
}

func firstLineValue(text string, keys ...string) string {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		for _, k := range keys {
			if strings.HasPrefix(line, k) {
				return strings.TrimSpace(strings.Trim(strings.TrimPrefix(line, k), `"'`))
			}
		}
	}
	return ""
}

func appendContent(turn *Turn, content any) {
	switch c := content.(type) {
	case string:
		turn.Text += c
	case []any:
		for _, item := range c {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			typ, _ := m["type"].(string)
			switch typ {
			case "text":
				if t, ok := m["text"].(string); ok {
					if turn.Text != "" {
						turn.Text += "\n"
					}
					turn.Text += t
				}
			case "tool_use":
				name, _ := m["name"].(string)
				args := compactJSON(m["input"])
				tc := ToolCall{Name: name, Args: args, Category: categorize(name)}
				turn.ToolCalls = append(turn.ToolCalls, tc)
				if tc.Category == CatSkill {
					turn.SkillsInvoked = append(turn.SkillsInvoked, skillNamesFromArgs(args)...)
					if name != "Skill" && name != "skill" && !strings.Contains(strings.ToLower(name), "mcp") {
						// tool named like a skill
					}
				}
				if strings.EqualFold(name, "Skill") {
					turn.SkillsInvoked = append(turn.SkillsInvoked, skillNamesFromArgs(args)...)
				}
			case "tool_result":
				isErr, _ := m["is_error"].(bool)
				if isErr && len(turn.ToolCalls) > 0 {
					turn.ToolCalls[len(turn.ToolCalls)-1].IsError = true
				}
			}
		}
	}
}

func skillNamesFromArgs(args string) []string {
	var obj map[string]any
	if json.Unmarshal([]byte(args), &obj) != nil {
		if args != "" && !strings.HasPrefix(args, "{") {
			return []string{args}
		}
		return nil
	}
	for _, key := range []string{"skill", "name", "skill_name"} {
		if n, ok := obj[key].(string); ok && n != "" {
			// claude uses "frontend-design:frontend-design"
			if i := strings.IndexByte(n, ':'); i > 0 {
				n = n[:i]
			}
			return []string{n}
		}
	}
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
