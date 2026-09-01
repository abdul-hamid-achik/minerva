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
	mixed := session.Session{
		Harness: "claude", ID: "m1",
		Turns: []session.Turn{{
			Role: "assistant",
			ToolCalls: []session.ToolCall{
				{Name: "Read", Category: session.CatRead},
				{Name: "Edit", Category: session.CatEdit},
				{Name: "Bash", Category: session.CatShell},
				{Name: "Read", Category: session.CatRead},
				{Name: "Edit", Category: session.CatEdit},
				{Name: "Bash", Category: session.CatShell},
			},
		}},
	}
	sigs := Extract([]session.Session{browse, mixed}, nil)
	for _, s := range sigs {
		if s.Kind == KindRepeat && strings.Contains(strings.ToLower(s.Key), "grep") && !strings.Contains(strings.ToLower(s.Key), "edit") && !strings.Contains(strings.ToLower(s.Key), "bash") {
			t.Fatalf("browse-only n-gram leaked: %#v", s)
		}
	}
	found := false
	for _, s := range sigs {
		if s.Kind == KindRepeat && strings.Contains(s.Key, "Edit") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected mixed sequence, got %#v", sigs)
	}
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
