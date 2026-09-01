// Package signal extracts deterministic skill-proposal signals from sessions.
package signal

import (
	"regexp"
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

// Thresholds. Exported so tests and docs can reference the same numbers.
const (
	// MaxSignals caps the ranked output.
	MaxSignals = 40
	// LongManualTools is the tool-call count that makes a skill-less session notable.
	LongManualTools = 8
	// ShellFamilyMin is how many times one executable must appear in a session.
	ShellFamilyMin = 3
	// LoadGapScore is the resolve score a catalog skill needs to count as a miss.
	LoadGapScore = 6
	// RepeatMinSessions or RepeatMinOccurrences makes a 3-gram a pattern.
	RepeatMinSessions    = 2
	RepeatMinOccurrences = 3
	// PerSessionCap bounds how much one session can push a repeat/shell signal.
	// Breadth (many sessions) should outrank depth (one very long session).
	PerSessionCap = 5
	// LoadGapMaxHits is how many resolve hits per prompt may become load gaps.
	LoadGapMaxHits = 2
)

// strongCorrections match anywhere in a user turn, on word boundaries.
// Each entry is a whole phrase, so "wrong" inside "wrongly configured" or
// "no" inside "nope, ship it" do not fire.
var strongCorrections = compilePhrases(
	// English
	"do it again", "try again", "redo that", "redo it", "start over",
	"that's not what i", "that is not what i", "not what i asked", "not what i meant",
	"not what i wanted", "already told you", "as i said", "like i said",
	"wrong file", "wrong place", "wrong branch", "the wrong", "undo that", "revert that",
	"roll that back", "you broke", "that broke", "still broken", "still not working",
	"didn't work", "did not work", "still fails", "still failing",
	// Spanish
	"otra vez", "de nuevo", "ya te dije", "te dije que", "eso no es", "no era eso",
	"no es eso", "no fue eso", "así no", "asi no", "estaba mal", "está mal", "esta mal",
	"sigue sin funcionar", "sigue fallando", "no funcionó", "no funciono", "rompiste",
	"deshaz eso", "revierte eso", "el otro archivo", "en el otro",
)

// leadingCorrections must open the message (after punctuation/whitespace).
// "no, use the other one" counts; "no problem, thanks" does not because the
// next token is checked against a small stop set.
var leadingCorrections = compilePhrases("no", "nope", "wrong", "incorrect", "nah", "hmm no", "mal", "incorrecto")

// leadingFalsePositives are words that follow a leading "no" in non-corrections.
var leadingFalsePositives = map[string]bool{
	"no": true, "problem": true, "worries": true, "thanks": true, "thank": true, "need": true,
	"rush": true, "hurry": true, "idea": true, "way": true, "issue": true, "issues": true,
	"tests": true, "test": true, "changes": true, "change": true, "error": true, "errors": true,
	"hay": true, "gracias": true, "problema": true, "pasa": true, "importa": true, "se": true,
	"te": true, "más": true, "mas": true, "sé": true, "worry": true, "questions": true,
}

// Extract walks sessions and returns ranked signals.
func Extract(sessions []session.Session, catalog []*skill.Skill) []Signal {
	byKey := map[string]*Signal{}
	add := func(kind, key, msg string, weight int, ev Evidence) {
		k := kind + "\x00" + key
		if existing, ok := byKey[k]; ok {
			existing.Weight += weight
			for i, prev := range existing.Evidence {
				if prev.Harness == ev.Harness && prev.SessionID == ev.SessionID {
					existing.Evidence[i].Count += weight
					return
				}
			}
			ev.Count = weight
			existing.Evidence = append(existing.Evidence, ev)
			return
		}
		ev.Count = weight
		byKey[k] = &Signal{Kind: kind, Key: key, Message: msg, Weight: weight, Evidence: []Evidence{ev}}
	}

	for _, s := range sessions {
		ev := Evidence{Harness: s.Harness, SessionID: s.ID, Workspace: s.Workspace}

		// n-grams of tool tokens. Shell calls carry their command so
		// "Shell(go) → Read → StrReplace" (test-fix loop) is distinct from
		// "Shell(git) → Read → StrReplace" (review-fix loop).
		var toks []gramToken
		for _, t := range s.Turns {
			for _, tc := range t.ToolCalls {
				toks = append(toks, tokenOf(tc))
			}
		}
		if len(toks) >= 3 {
			for i := 0; i+2 < len(toks); i++ {
				a, b, c := toks[i], toks[i+1], toks[i+2]
				if a.label == b.label && b.label == c.label {
					continue
				}
				if !distinctive(a, b, c) {
					continue
				}
				gram := a.label + " → " + b.label + " → " + c.label
				add(KindRepeat, gram, "repeated tool sequence "+gram, 1, ev)
			}
		}

		// retry loops: same tool right after it errored
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

		// user corrections: only after the assistant has done something
		assistantActed := false
		for _, t := range s.Turns {
			if t.Role != "user" {
				if t.Role == "assistant" && (t.Text != "" || len(t.ToolCalls) > 0) {
					assistantActed = true
				}
				continue
			}
			if !assistantActed {
				continue
			}
			if phrase := correctionPhrase(t.Text); phrase != "" {
				cev := ev
				cev.Note = "matched " + strings.TrimSpace(phrase)
				add(KindCorrection, "correction", "user asked to redo or correct work", 4, cev)
			}
		}

		// long session without skills
		if s.ToolCount >= LongManualTools && len(s.Skills) == 0 {
			add(KindLongManual, s.Harness, "long session with no skill loaded", 2, ev)
		}

		// shell families: leading executable per shell call
		fams := map[string]int{}
		for _, t := range s.Turns {
			for _, tc := range t.ToolCalls {
				if tc.Category != session.CatShell {
					continue
				}
				if cmd := commandOf(tc); cmd != "" {
					fams[cmd]++
				}
			}
		}
		for cmd, n := range fams {
			if n >= ShellFamilyMin {
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
			gaps := 0
			for _, hit := range res.Hits {
				if gaps >= LoadGapMaxHits {
					break
				}
				// One exact name hit (8) or two distinct terms: a loose single
				// description word is not a miss.
				if hit.Score < LoadGapScore || (hit.Terms < 2 && hit.Score < 8) || invoked[hit.Name] {
					continue
				}
				gaps++
				add(KindLoadGap, hit.Name, "skill "+hit.Name+" matched the prompt but was not loaded", 5, Evidence{
					Harness: s.Harness, SessionID: s.ID, Workspace: s.Workspace, Note: hit.Reason,
				})
			}
		}
	}

	out := make([]Signal, 0, len(byKey))
	for _, s := range byKey {
		if s.Kind == KindRepeat && len(s.Evidence) < RepeatMinSessions && s.Weight < RepeatMinOccurrences {
			continue // one session, twice: coincidence, not a pattern
		}
		out = append(out, *s)
	}
	bonus := map[string]int{
		KindLoadGap: 8, KindCorrection: 6, KindRetry: 4,
		KindShellFam: 3, KindLongManual: 2, KindRepeat: 0,
	}
	for i := range out {
		sig := &out[i]
		if sig.Kind == KindRepeat || sig.Kind == KindShellFam {
			// Re-derive weight from capped per-session counts so one marathon
			// session cannot outrank a pattern seen across many sessions.
			w := 0
			for _, e := range sig.Evidence {
				if e.Count > PerSessionCap {
					w += PerSessionCap
				} else {
					w += e.Count
				}
			}
			sig.Weight = w
		}
		sig.Weight += bonus[sig.Kind]
		// Cross-session repetition beats within-session repetition.
		if len(sig.Evidence) > 1 {
			sig.Weight += 2 * (len(sig.Evidence) - 1)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Weight == out[j].Weight {
			return out[i].Key < out[j].Key
		}
		return out[i].Weight > out[j].Weight
	})
	if len(out) > MaxSignals {
		out = out[:MaxSignals]
	}
	return out
}

// gramToken is one n-gram position: a display label plus its category.
type gramToken struct {
	label string
	cat   session.Category
	cmd   string
}

func tokenOf(tc session.ToolCall) gramToken {
	cat := tc.Category
	if cat == "" {
		cat = session.CategoryOf(tc.Name)
	}
	tok := gramToken{label: tc.Name, cat: cat}
	if cat == session.CatShell {
		if cmd := commandOf(tc); cmd != "" {
			tok.cmd = cmd
			tok.label = tc.Name + "(" + cmd + ")"
		}
	}
	return tok
}

// distinctive reports whether a 3-gram says something a generic coding loop
// does not. Pure read/search is browsing; pure read/edit is "editing"; a
// bare shell call with no recognizable command is noise. A skill candidate
// needs at least one anchor: a named shell command, an MCP tool, a subagent,
// or a skill load.
func distinctive(toks ...gramToken) bool {
	for _, t := range toks {
		switch t.cat {
		case session.CatShell:
			if t.cmd != "" {
				return true
			}
		case session.CatMCP, session.CatSubagent, session.CatSkill:
			return true
		}
	}
	return false
}

// commandOf prefers the parser-populated Command, falling back to extraction
// for sessions built in memory (tests, future adapters).
func commandOf(tc session.ToolCall) string {
	if tc.Command != "" {
		return tc.Command
	}
	return session.ShellCommand(tc.Args)
}

// correctionPhrase returns the phrase that marks a user turn as a correction,
// or "" when the turn is not one.
func correctionPhrase(text string) string {
	low := normalizeText(text)
	if low == "" {
		return ""
	}
	for _, re := range strongCorrections {
		if m := re.FindStringSubmatch(low); len(m) > 1 {
			return m[1]
		}
	}
	// Leading markers: first word(s) of the message.
	lead := strings.TrimLeft(low, " \t\"'“”‘’(-—–.,!?¿¡:;")
	for _, re := range leadingCorrections {
		idx := re.FindStringSubmatchIndex(lead)
		if idx == nil || idx[2] != 0 {
			continue
		}
		rest := strings.TrimLeft(lead[idx[3]:], " ,.:;!—–-")
		if leadingFalsePositives[firstWord(rest)] {
			continue
		}
		return lead[idx[2]:idx[3]]
	}
	return ""
}

func normalizeText(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, "’", "'")
	s = strings.Join(strings.Fields(s), " ")
	return s
}

func firstWord(s string) string {
	for i, r := range s {
		if r == ' ' || r == ',' || r == '.' || r == '!' || r == '?' {
			return s[:i]
		}
	}
	return s
}

func compilePhrases(phrases ...string) []*regexp.Regexp {
	out := make([]*regexp.Regexp, 0, len(phrases))
	for _, p := range phrases {
		// Word boundaries that also work for accented letters: not preceded
		// or followed by a letter/digit.
		out = append(out, regexp.MustCompile(`(?:^|[^\pL\pN])(`+regexp.QuoteMeta(p)+`)(?:$|[^\pL\pN])`))
	}
	return out
}
