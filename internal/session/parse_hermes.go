package session

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"time"
)

// parseHermes reads a Hermes Agent gateway transcript
// (~/.hermes/sessions/<id>.jsonl): OpenAI-style chat messages, one per line.
//
//	{"role":"session_meta","model":…,"platform":…,"timestamp":…}   (optional)
//	{"role":"user","content":"…"}
//	{"role":"assistant","content":"…","tool_calls":[{"id":…,"function":{"name":…,"arguments":"{…}"}}]}
//	{"role":"tool","tool_call_id":…,"content":"…"}
//
// Tool results are attached to the call with the same id. The transcript
// carries no working directory, so Workspace stays empty.
func parseHermes(s Session) (Session, error) {
	type loc struct{ turn, call int }
	calls := map[string]loc{}
	err := parseJSONL(s.Path, func(obj map[string]any) {
		role, _ := obj["role"].(string)
		if s.StartedAt.IsZero() {
			s.StartedAt = hermesTime(obj["timestamp"])
		}
		switch role {
		case "session_meta":
			if m, ok := obj["model"].(string); ok && m != "" {
				s.Model = m
			}
		case "user":
			if text, _ := obj["content"].(string); text != "" {
				s.Turns = append(s.Turns, Turn{Role: "user", Text: text})
			}
		case "assistant":
			turn := Turn{Role: "assistant"}
			turn.Text, _ = obj["content"].(string)
			list, _ := obj["tool_calls"].([]any)
			for _, c := range list {
				cm, _ := c.(map[string]any)
				fn, _ := cm["function"].(map[string]any)
				name, _ := fn["name"].(string)
				if name == "" {
					continue
				}
				tc := ToolCall{Name: name, Args: compactJSON(fn["arguments"]), Category: categorize(name)}
				if id, _ := cm["id"].(string); id != "" {
					calls[id] = loc{len(s.Turns), len(turn.ToolCalls)}
				}
				turn.ToolCalls = append(turn.ToolCalls, tc)
				if tc.Category == CatSkill {
					turn.SkillsInvoked = append(turn.SkillsInvoked, skillNamesFromArgs(tc.Args)...)
				}
			}
			if turn.Text != "" || len(turn.ToolCalls) > 0 {
				s.Turns = append(s.Turns, turn)
			}
		case "tool":
			id, _ := obj["tool_call_id"].(string)
			at, ok := calls[id]
			if !ok {
				return
			}
			out, _ := obj["content"].(string)
			tc := &s.Turns[at.turn].ToolCalls[at.call]
			tc.Result = truncate(out, 500)
			tc.IsError = hermesResultFailed(out)
		}
	})
	if s.ID == "" {
		s.ID = strings.TrimSuffix(filepath.Base(s.Path), filepath.Ext(s.Path))
	}
	return s, err
}

// hermesResultFailed reads Hermes's JSON tool-result envelope: a non-zero
// exit_code, a non-empty error, or success=false marks a failed call.
func hermesResultFailed(content string) bool {
	var res struct {
		ExitCode *float64 `json:"exit_code"`
		Error    any      `json:"error"`
		Success  *bool    `json:"success"`
	}
	if !strings.HasPrefix(strings.TrimSpace(content), "{") || json.Unmarshal([]byte(content), &res) != nil {
		return false
	}
	if res.ExitCode != nil && *res.ExitCode != 0 {
		return true
	}
	if s, ok := res.Error.(string); ok && s != "" {
		return true
	}
	return res.Success != nil && !*res.Success
}

// hermesTime reads a Hermes timestamp: a zone-less local ISO string
// ("2026-03-23T13:51:42.123456"), an RFC 3339 string, or Unix seconds.
func hermesTime(v any) time.Time {
	switch t := v.(type) {
	case string:
		if p, err := time.Parse(time.RFC3339Nano, t); err == nil {
			return p
		}
		if p, err := time.ParseInLocation("2006-01-02T15:04:05.999999999", t, time.Local); err == nil {
			return p
		}
	case float64:
		if t > 0 {
			return time.Unix(int64(t), 0)
		}
	}
	return time.Time{}
}
