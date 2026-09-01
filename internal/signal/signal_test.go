package signal

import (
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/minerva/internal/session"
	"github.com/abdul-hamid-achik/minerva/internal/skill"
)

func TestExtract_LoadGapAndCorrection(t *testing.T) {
	sess := session.Session{
		Harness: "claude",
		ID:      "s1",
		Turns: []session.Turn{
			{Role: "user", Text: "write the readme documentation"},
			{Role: "assistant", ToolCalls: []session.ToolCall{
				{Name: "Read", Category: session.CatRead},
				{Name: "Write", Category: session.CatEdit},
			}},
			{Role: "user", Text: "no, do it again the other way"},
		},
		ToolCount: 2,
	}
	catalog := []*skill.Skill{{
		Name: "doc-writer", Description: "Use when writing or editing READMEs and API docs", Content: "docs",
	}}
	sigs := Extract([]session.Session{sess}, catalog)
	kinds := map[string]bool{}
	for _, s := range sigs {
		kinds[s.Kind] = true
	}
	if !kinds[KindCorrection] {
		t.Fatalf("expected correction, got %#v", sigs)
	}
	if !kinds[KindLoadGap] {
		t.Fatalf("expected load gap, got %#v", sigs)
	}
}

func TestExtract_SkipsBrowseOnlyNgrams(t *testing.T) {
	browse := session.Session{
		Harness: "claude", ID: "b1",
		Turns: []session.Turn{{
			Role: "assistant",
			ToolCalls: []session.ToolCall{
				{Name: "Read", Category: session.CatRead},
				{Name: "Grep", Category: session.CatSearch},
				{Name: "Read", Category: session.CatRead},
				{Name: "Read", Category: session.CatRead},
				{Name: "Grep", Category: session.CatSearch},
				{Name: "Read", Category: session.CatRead},
			},
		}},
	}
	mixedTurn := session.Turn{
		Role: "assistant",
		ToolCalls: []session.ToolCall{
			{Name: "Read", Category: session.CatRead},
			{Name: "Edit", Category: session.CatEdit},
			{Name: "Bash", Category: session.CatShell, Args: `{"command":"go test ./..."}`},
		},
	}
	mixed := session.Session{Harness: "claude", ID: "m1", Turns: []session.Turn{mixedTurn}}
	mixed2 := session.Session{Harness: "codex", ID: "m2", Turns: []session.Turn{mixedTurn}}
	sigs := Extract([]session.Session{browse, mixed, mixed2}, nil)
	for _, s := range sigs {
		if s.Kind == KindRepeat && strings.Contains(strings.ToLower(s.Key), "grep") && !strings.Contains(strings.ToLower(s.Key), "edit") && !strings.Contains(strings.ToLower(s.Key), "bash") {
			t.Fatalf("browse-only n-gram leaked: %#v", s)
		}
	}
	found := false
	for _, s := range sigs {
		if s.Kind == KindRepeat && s.Key == "Read → Edit → Bash(go)" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected mixed sequence with shell command token, got %#v", sigs)
	}
}

func TestExtract_EditOnlyNgramsAreNotPatterns(t *testing.T) {
	seq := []session.ToolCall{
		{Name: "Read", Category: session.CatRead},
		{Name: "StrReplace", Category: session.CatEdit},
		{Name: "StrReplace", Category: session.CatEdit},
	}
	bareShell := []session.ToolCall{
		{Name: "Shell", Category: session.CatShell, Args: `{"command":"ls -la"}`},
		{Name: "Read", Category: session.CatRead},
		{Name: "Read", Category: session.CatRead},
	}
	var sessions []session.Session
	for i := 0; i < 4; i++ {
		sessions = append(sessions, session.Session{Harness: "cursor", ID: string(rune('a' + i)), Turns: []session.Turn{
			{Role: "assistant", ToolCalls: seq}, {Role: "assistant", ToolCalls: seq}, {Role: "assistant", ToolCalls: bareShell},
		}})
	}
	if sigs := Extract(sessions, nil); countKind(sigs, KindRepeat) != 0 {
		t.Fatalf("read/edit and noise-shell n-grams must not be patterns: %#v", sigs)
	}
}

func TestExtract_MarathonSessionDoesNotOutrankBreadth(t *testing.T) {
	loop := []session.ToolCall{
		{Name: "Bash", Category: session.CatShell, Args: `{"command":"go test ./..."}`},
		{Name: "Read", Category: session.CatRead},
		{Name: "Edit", Category: session.CatEdit},
	}
	other := []session.ToolCall{
		{Name: "Bash", Category: session.CatShell, Args: `{"command":"npm test"}`},
		{Name: "Read", Category: session.CatRead},
		{Name: "Edit", Category: session.CatEdit},
	}
	var marathon session.Session
	marathon.Harness, marathon.ID = "claude", "long"
	for i := 0; i < 100; i++ {
		marathon.Turns = append(marathon.Turns, session.Turn{Role: "assistant", ToolCalls: loop})
	}
	var breadth []session.Session
	for i := 0; i < 4; i++ {
		breadth = append(breadth, session.Session{Harness: "codex", ID: string(rune('a' + i)), Turns: []session.Turn{{Role: "assistant", ToolCalls: other}}})
	}
	sigs := Extract(append(breadth, marathon), nil)
	var first *Signal
	for i := range sigs {
		if sigs[i].Kind == KindRepeat {
			first = &sigs[i]
			break
		}
	}
	if first == nil || !strings.HasPrefix(first.Key, "Bash(npm)") {
		t.Fatalf("expected the 4-session pattern first, got %#v", sigs)
	}
}

func TestExtract_RepeatNeedsTwoSessionsOrThreeHits(t *testing.T) {
	seq := []session.ToolCall{
		{Name: "Read", Category: session.CatRead},
		{Name: "Edit", Category: session.CatEdit},
		{Name: "Bash", Category: session.CatShell, Args: `{"command":"go test ./..."}`},
	}
	twice := session.Session{Harness: "claude", ID: "t1", Turns: []session.Turn{
		{Role: "assistant", ToolCalls: seq}, {Role: "assistant", ToolCalls: seq},
	}}
	if sigs := Extract([]session.Session{twice}, nil); countKind(sigs, KindRepeat) != 0 {
		t.Fatalf("two hits in one session should not be a pattern: %#v", sigs)
	}
	thrice := session.Session{Harness: "claude", ID: "t2", Turns: []session.Turn{
		{Role: "assistant", ToolCalls: seq}, {Role: "assistant", ToolCalls: seq}, {Role: "assistant", ToolCalls: seq},
	}}
	if sigs := Extract([]session.Session{thrice}, nil); countKind(sigs, KindRepeat) == 0 {
		t.Fatalf("three hits in one session should be a pattern: %#v", sigs)
	}
}

func TestExtract_CrossSessionRanksHigher(t *testing.T) {
	seqA := []session.ToolCall{{Name: "Read"}, {Name: "Edit"}, {Name: "Bash", Args: `{"command":"go build ./..."}`}}
	seqB := []session.ToolCall{{Name: "Edit"}, {Name: "Bash", Args: `{"command":"git diff"}`}, {Name: "Write"}}
	one := session.Session{Harness: "claude", ID: "a", Turns: []session.Turn{
		{Role: "assistant", ToolCalls: seqA}, {Role: "assistant", ToolCalls: seqA}, {Role: "assistant", ToolCalls: seqA},
	}}
	two := session.Session{Harness: "claude", ID: "b", Turns: []session.Turn{{Role: "assistant", ToolCalls: seqB}}}
	three := session.Session{Harness: "codex", ID: "c", Turns: []session.Turn{{Role: "assistant", ToolCalls: seqB}}}
	four := session.Session{Harness: "cursor", ID: "d", Turns: []session.Turn{{Role: "assistant", ToolCalls: seqB}}}
	sigs := Extract([]session.Session{one, two, three, four}, nil)
	var first *Signal
	for i := range sigs {
		if sigs[i].Kind == KindRepeat {
			first = &sigs[i]
			break
		}
	}
	if first == nil || !strings.HasPrefix(first.Key, "Edit → Bash(git) → Write") {
		t.Fatalf("expected cross-session pattern first, got %#v", sigs)
	}
	if len(first.Evidence) != 3 {
		t.Fatalf("evidence=%d", len(first.Evidence))
	}
}

func TestCorrectionPhrase(t *testing.T) {
	yes := []string{
		"no, use the other file",
		"No. Do it again but in Go.",
		"that's not what I asked for",
		"You edited the wrong file",
		"otra vez, pero sin tests",
		"ya te dije que no toques main",
		"Wrong — revert that",
		"hmm no, the other one",
		"it still fails on CI",
	}
	for _, s := range yes {
		if correctionPhrase(s) == "" {
			t.Errorf("expected correction: %q", s)
		}
	}
	no := []string{
		"no problem, thanks",
		"No worries, ship it",
		"there are no tests yet, add some",
		"the login flow is wrongly named but fine",
		"Nope, no errors in the logs — continue",
		"please remove the unknown flag handling",
		"no hay problema, sigue",
		"I know the config is a bit wrong but leave it for now",
		"Add a note about the /no-cache header",
	}
	for _, s := range no {
		if p := correctionPhrase(s); p != "" {
			t.Errorf("false positive %q on %q", p, s)
		}
	}
}

func TestExtract_CorrectionOnlyAfterAssistantActs(t *testing.T) {
	sess := session.Session{Harness: "claude", ID: "c1", Turns: []session.Turn{
		{Role: "user", Text: "no, the other repo — start over from the docs"},
	}}
	if sigs := Extract([]session.Session{sess}, nil); countKind(sigs, KindCorrection) != 0 {
		t.Fatalf("first user turn cannot be a correction: %#v", sigs)
	}
}

func TestExtract_ShellFamilyFromCommand(t *testing.T) {
	git := session.ToolCall{Name: "Bash", Category: session.CatShell, Args: `{"command":"cd /tmp/x && git status --short"}`}
	noise := session.ToolCall{Name: "Bash", Category: session.CatShell, Args: `{"command":"ls -la | head"}`}
	// "python" mentioned in a description must not count as a python family.
	mention := session.ToolCall{Name: "Bash", Category: session.CatShell, Args: `{"command":"echo 'python is nice'","description":"python note"}`}
	sess := session.Session{Harness: "claude", ID: "sh", Turns: []session.Turn{{
		Role: "assistant", ToolCalls: []session.ToolCall{git, git, git, noise, noise, noise, mention, mention, mention},
	}}}
	sigs := Extract([]session.Session{sess}, nil)
	keys := map[string]bool{}
	for _, s := range sigs {
		if s.Kind == KindShellFam {
			keys[s.Key] = true
		}
	}
	if !keys["git"] {
		t.Fatalf("expected git family: %#v", sigs)
	}
	if keys["ls"] || keys["python"] || keys["echo"] {
		t.Fatalf("noise leaked into shell families: %#v", keys)
	}
}

func countKind(sigs []Signal, kind string) int {
	n := 0
	for _, s := range sigs {
		if s.Kind == kind {
			n++
		}
	}
	return n
}

func TestExtract_Retry(t *testing.T) {
	sess := session.Session{
		Harness: "codex",
		ID:      "s2",
		Turns: []session.Turn{{
			Role: "assistant",
			ToolCalls: []session.ToolCall{
				{Name: "exec_command", IsError: true, Category: session.CatShell},
				{Name: "exec_command", Category: session.CatShell},
			},
		}},
	}
	sigs := Extract([]session.Session{sess}, nil)
	found := false
	for _, s := range sigs {
		if s.Kind == KindRetry {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected retry: %#v", sigs)
	}
}
