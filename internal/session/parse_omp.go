package session

import (
	"strings"
	"time"
)

// parseOMP reads an oh-my-pi session (~/.omp/agent/sessions/<slug>/*.jsonl).
// Each line is a typed entry:
//
//	{"type":"session","id":…,"cwd":…,"timestamp":…}
//	{"type":"message","message":{"role":"user|assistant|toolResult",…}}
//
// Assistant content is a list of text / thinking / toolCall{id,name,arguments}
// parts; a toolResult message carries toolCallId, isError and text parts.
// Other entry types (model_change, title, model_usage, …) are ignored.
func parseOMP(s Session) (Session, error) {
	calls := callIndex{}
	err := parseJSONL(s.Path, &s.SkippedLines, func(obj map[string]any) {
		typ, _ := obj["type"].(string)
		switch typ {
		case "session":
			if cwd, _ := obj["cwd"].(string); cwd != "" && s.Workspace == "" {
				s.Workspace = cwd
			}
			if id, _ := obj["id"].(string); id != "" {
				s.ID = id
			}
			if ts, _ := obj["timestamp"].(string); ts != "" && s.StartedAt.IsZero() {
				if t, err := time.Parse(time.RFC3339Nano, ts); err == nil {
					s.StartedAt = t
				}
			}
		case "message":
			msg, _ := obj["message"].(map[string]any)
			role, _ := msg["role"].(string)
			switch role {
			case "user":
				if text := ompText(msg["content"]); text != "" {
					s.Turns = append(s.Turns, Turn{Role: "user", Text: text})
				}
			case "assistant":
				if m, _ := msg["model"].(string); m != "" {
					s.Model = m
				}
				turn := Turn{Role: "assistant", Text: ompText(msg["content"])}
				parts, _ := msg["content"].([]any)
				for _, p := range parts {
					pm, _ := p.(map[string]any)
					if pm["type"] != "toolCall" {
						continue
					}
					name, _ := pm["name"].(string)
					if name == "" {
						continue
					}
					id, _ := pm["id"].(string)
					if calls.has(id) {
						continue // a retried message can repeat a call it already sent
					}
					tc := ToolCall{Name: name, Args: compactJSON(pm["arguments"]), Category: categorize(name)}
					calls.add(id, len(s.Turns), len(turn.ToolCalls))
					turn.ToolCalls = append(turn.ToolCalls, tc)
					if tc.Category == CatSkill {
						turn.SkillsInvoked = append(turn.SkillsInvoked, skillNamesFromArgs(tc.Args)...)
					}
				}
				if turn.Text != "" || len(turn.ToolCalls) > 0 {
					s.Turns = append(s.Turns, turn)
				}
			case "toolResult":
				id, _ := msg["toolCallId"].(string)
				tc := calls.lookup(&s, id)
				if tc == nil {
					return
				}
				tc.Result = truncate(ompText(msg["content"]), 500)
				tc.IsError, _ = msg["isError"].(bool)
			}
		}
	})
	return s, err
}

// ompText joins the text parts of an omp content value (string or part list).
func ompText(content any) string {
	switch c := content.(type) {
	case string:
		return c
	case []any:
		var parts []string
		for _, p := range c {
			pm, _ := p.(map[string]any)
			if pm["type"] != "text" {
				continue
			}
			if t, _ := pm["text"].(string); t != "" {
				parts = append(parts, t)
			}
		}
		return strings.Join(parts, "\n")
	}
	return ""
}
