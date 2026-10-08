package session

import (
	"encoding/json"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// callIndex remembers where each tool call sits in a session, so a result
// that arrives in a later message (Claude tool_result, Codex *_output, Copilot
// tool.execution_complete, Hermes role:tool, omp toolResult) lands on the call
// that produced it rather than on whichever call happens to be last.
type callIndex map[string]callLoc

type callLoc struct{ turn, call int }

// add records a call that will sit at s.Turns[turn].ToolCalls[call].
func (ix callIndex) add(id string, turn, call int) {
	if id != "" {
		ix[id] = callLoc{turn, call}
	}
}

func (ix callIndex) has(id string) bool {
	_, ok := ix[id]
	return id != "" && ok
}

// lookup returns the call recorded for id, or nil.
func (ix callIndex) lookup(s *Session, id string) *ToolCall {
	at, ok := ix[id]
	if !ok || at.turn >= len(s.Turns) || at.call >= len(s.Turns[at.turn].ToolCalls) {
		return nil
	}
	return &s.Turns[at.turn].ToolCalls[at.call]
}

// lastCall returns the most recent tool call in s, or nil.
func lastCall(s *Session) *ToolCall {
	for i := len(s.Turns) - 1; i >= 0; i-- {
		if n := len(s.Turns[i].ToolCalls); n > 0 {
			return &s.Turns[i].ToolCalls[n-1]
		}
	}
	return nil
}

// exitCodeRe reads the status line a shell tool opens its result with:
// "Exit code: 1" or "Process exited with code 2". It is anchored to the start
// of the result; the same words further down are the command's own output.
var exitCodeRe = regexp.MustCompile(`^(?:Exit code:\s*|Process exited with code\s+)(-?\d+)`)

// jsonExitCodeRe finds "exit_code": N in a JSON envelope that no longer
// parses (a note appended, or cut off).
var jsonExitCodeRe = regexp.MustCompile(`"exit_code"\s*:\s*(-?\d+)`)

// outputFailed reports whether a tool result's text says the call failed.
// Only the tool's own status counts: Codex exec opens with "Script failed" or
// "Script completed", and whatever the script printed after that (including
// exit codes of commands it ran and handled) is not the call's status.
func outputFailed(text string) bool {
	t := strings.TrimSpace(text)
	switch {
	case strings.HasPrefix(t, "Script failed"):
		return true
	case strings.HasPrefix(t, "Script completed"):
		return false
	}
	if strings.HasPrefix(t, "{") {
		var env struct {
			ExitCode *float64 `json:"exit_code"`
			Metadata struct {
				ExitCode *float64 `json:"exit_code"`
			} `json:"metadata"`
		}
		if json.Unmarshal([]byte(t), &env) == nil {
			if env.ExitCode != nil {
				return *env.ExitCode != 0
			}
			if env.Metadata.ExitCode != nil {
				return *env.Metadata.ExitCode != 0
			}
		}
	}
	return nonZero(exitCodeRe.FindStringSubmatch(t))
}

// nonZero reports whether a regexp match captured a non-zero exit code.
func nonZero(m []string) bool {
	if m == nil {
		return false
	}
	code, err := strconv.Atoi(m[1])
	return err == nil && code != 0
}

// bareSkillName drops a plugin namespace: "vercel:nextjs" names the skill
// "nextjs", which is how it appears in the skill library.
func bareSkillName(n string) string {
	if i := strings.LastIndexByte(n, ':'); i >= 0 && i < len(n)-1 {
		return n[i+1:]
	}
	return n
}

// skillFromRead returns the skill a read-style call loads, or "". Harnesses
// without a dedicated skill tool load a skill by reading it: omp reads
// skill://<name>, Codex and Cursor read <...>/skills/<name>/SKILL.md.
func skillFromRead(args string) string {
	var obj map[string]any
	if json.Unmarshal([]byte(args), &obj) != nil {
		return ""
	}
	for _, key := range []string{"path", "file_path", "filePath", "file", "target_file", "absolute_path", "uri"} {
		p, _ := obj[key].(string)
		if p == "" {
			continue
		}
		if rest, ok := strings.CutPrefix(p, "skill://"); ok {
			name, _, _ := strings.Cut(rest, "/")
			return name
		}
		p = filepath.ToSlash(p)
		if strings.EqualFold(filepath.Base(p), "SKILL.md") {
			dir := filepath.Dir(p)
			if filepath.Base(filepath.Dir(dir)) == "skills" {
				return filepath.Base(dir)
			}
		}
	}
	return ""
}
