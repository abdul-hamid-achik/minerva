// Package signal extracts deterministic skill-proposal signals from sessions.
package signal

import (
	"sort"
	"strings"

	"github.com/abdul-hamid-achik/minerva/internal/session"
	"github.com/abdul-hamid-achik/minerva/internal/skill"
)

// Kind identifies a signal class.
const (
	KindRepeat     = "repeat_sequence"
	KindRetry      = "retry_loop"
	KindCorrection = "user_correction"
	KindLoadGap    = "load_gap"
	KindLongManual = "long_manual"
	KindShellFam   = "shell_family"
)

// Evidence points at a session that produced a signal.
type Evidence struct {
	Harness   string `json:"harness"`
	SessionID string `json:"session_id"`
	Workspace string `json:"workspace,omitempty"`
	Note      string `json:"note"`
	Count     int    `json:"count,omitempty"`
}

// Signal is one extracted pattern.
type Signal struct {
	Kind     string     `json:"kind"`
	Key      string     `json:"key"`
	Message  string     `json:"message"`
	Weight   int        `json:"weight"`
	Evidence []Evidence `json:"evidence"`
}

var correctionPhrases = []string{
	"no, ", "no,", "otra vez", "do it again", "already told", "ya te dije",
	"that's not", "not what i", "wrong", "try again", "el otro",
}

// Extract walks sessions and returns ranked signals.
func Extract(sessions []session.Session, catalog []*skill.Skill) []Signal {
	byKey := map[string]*Signal{}
	add := func(kind, key, msg string, weight int, ev Evidence) {
		k := kind + "\x00" + key
		if existing, ok := byKey[k]; ok {
			existing.Weight += weight
			for _, prev := range existing.Evidence {
				if prev.Harness == ev.Harness && prev.SessionID == ev.SessionID {
					return
				}
			}
			existing.Evidence = append(existing.Evidence, ev)
			return
		}
		byKey[k] = &Signal{Kind: kind, Key: key, Message: msg, Weight: weight, Evidence: []Evidence{ev}}
	}

	for _, s := range sessions {
		ev := Evidence{Harness: s.Harness, SessionID: s.ID, Workspace: s.Workspace}

		// n-grams of tool names
		var names []string
		for _, t := range s.Turns {
			for _, tc := range t.ToolCalls {
				names = append(names, tc.Name)
			}
		}
		if len(names) >= 3 {
			for i := 0; i+2 < len(names); i++ {
				a, b, c := names[i], names[i+1], names[i+2]
				if a == b && b == c {
					continue
				}
				if browseOnly(a, b, c) {
					continue
				}
				gram := a + " → " + b + " → " + c
				add(KindRepeat, gram, "repeated tool sequence "+gram, 1, ev)
			}
		}

		// retry loops
		var prev *session.ToolCall
		for _, t := range s.Turns {
			for i := range t.ToolCalls {
				tc := t.ToolCalls[i]
				if prev != nil && prev.IsError && prev.Name == tc.Name {
					add(KindRetry, tc.Name, "retry after error: "+tc.Name, 3, ev)
				}
				prev = &t.ToolCalls[i]
			}
		}

		// user corrections
		for _, t := range s.Turns {
			if t.Role != "user" {
				continue
			}
			low := strings.ToLower(t.Text)
			for _, p := range correctionPhrases {
				if strings.Contains(low, p) {
					add(KindCorrection, "correction", "user asked to redo or correct work", 4, ev)
					break
				}
			}
		}

		// long session without skills
		if s.ToolCount >= 8 && len(s.Skills) == 0 {
			add(KindLongManual, s.Harness, "long session with no skill loaded", 2, ev)
		}

		// shell families
		fams := map[string]int{}
		for _, t := range s.Turns {
			for _, tc := range t.ToolCalls {
				if tc.Category != session.CatShell {
					continue
				}
				cmd := firstToken(tc.Args)
				if cmd != "" {
					fams[cmd]++
				}
			}
		}
		for cmd, n := range fams {
			if n >= 3 {
				add(KindShellFam, cmd, "repeated shell family "+cmd, n, ev)
			}
		}

		// load gaps: catalog skill would have applied, was not invoked
		prompt := s.FirstUserPrompt()
		if prompt != "" && len(catalog) > 0 {
			res := skill.Resolve(prompt, catalog)
			invoked := map[string]bool{}
			for _, name := range s.Skills {
				invoked[name] = true
			}
			for _, hit := range res.Hits {
				if hit.Score >= 6 && !invoked[hit.Name] {
					add(KindLoadGap, hit.Name, "skill "+hit.Name+" matched the prompt but was not loaded", 5, Evidence{
						Harness: s.Harness, SessionID: s.ID, Workspace: s.Workspace, Note: hit.Reason,
					})
				}
			}
		}
	}

	out := make([]Signal, 0, len(byKey))
	for _, s := range byKey {
		if s.Kind == KindRepeat && s.Weight < 2 {
			continue // only keep sequences seen more than once
		}
		out = append(out, *s)
	}
	bonus := map[string]int{
		KindLoadGap: 8, KindCorrection: 6, KindRetry: 4,
		KindShellFam: 3, KindLongManual: 2, KindRepeat: 0,
	}
	for i := range out {
		out[i].Weight += bonus[out[i].Kind]
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Weight == out[j].Weight {
			return out[i].Key < out[j].Key
		}
		return out[i].Weight > out[j].Weight
	})
	if len(out) > 40 {
		out = out[:40]
	}
	return out
}

func browseOnly(names ...string) bool {
	if len(names) == 0 {
		return false
	}
	for _, name := range names {
		switch session.CategoryOf(name) {
		case session.CatRead, session.CatSearch:
			continue
		default:
			return false
		}
	}
	return true
}

func firstToken(args string) string {
	low := strings.ToLower(args)
	for _, cmd := range []string{"git", "go", "npm", "pnpm", "yarn", "cargo", "python", "pytest", "vercel", "docker", "kubectl", "task", "make"} {
		if strings.Contains(low, cmd+" ") || strings.Contains(low, `"`+cmd) || strings.Contains(low, cmd+`"`) {
			return cmd
		}
	}
	return ""
}
