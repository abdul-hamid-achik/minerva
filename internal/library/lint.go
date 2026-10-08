// Package library lints the shared skill library under ~/.agents.
package library

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/abdul-hamid-achik/minerva/internal/secret"
	"github.com/abdul-hamid-achik/minerva/internal/skill"
)

const (
	SeverityError   = "error"
	SeverityWarning = "warning"
	SeverityInfo    = "info"
)

// Issue is one lint finding.
type Issue struct {
	Severity string `json:"severity"`
	Kind     string `json:"kind"`
	Target   string `json:"target"`
	Message  string `json:"message"`
}

// LintReport aggregates library health.
type LintReport struct {
	Issues   []Issue `json:"issues"`
	Errors   int     `json:"errors"`
	Warnings int     `json:"warnings"`
	Infos    int     `json:"infos"`
	Skills   int     `json:"skills"`
	OK       bool    `json:"ok"`
}

// Lint scans skills under agentsDir.
func Lint(agentsDir string) (*LintReport, error) {
	rep := &LintReport{}

	skillMgr := skill.ForAgents(agentsDir)
	if err := skillMgr.LoadAll(); err != nil {
		return nil, fmt.Errorf("load skills: %w", err)
	}

	skills := skillMgr.All()
	rep.Skills = len(skills)
	for _, p := range skillMgr.Problems() {
		rep.add(SeverityError, "skill-load", agentsDir, "skipped: "+p.Error())
	}

	for _, s := range skills {
		if strings.TrimSpace(s.Description) == "" {
			rep.add(SeverityWarning, "skill-description", s.Name, "missing description (poor catalog discovery)")
		}
		if len(s.Description) > skill.MaxSkillDescriptionBytes {
			rep.add(SeverityError, "skill-description", s.Name, "description exceeds write limit")
		} else if len(s.Description) > skill.LintDescriptionWarnBytes {
			rep.add(SeverityWarning, "skill-description", s.Name, "description is long; fine for harness discovery, consider tightening")
		}
		if strings.TrimSpace(s.Content) == "" {
			rep.add(SeverityWarning, "skill-body", s.Name, "empty body")
		}
		if !utf8.ValidString(s.Content) {
			rep.add(SeverityError, "skill-body", s.Name, "body is not valid UTF-8")
		}
		for _, hit := range secret.Hits(s.Content) {
			rep.add(SeverityError, "secret", s.Name, "possible secret pattern: "+hit)
		}
	}

	if rep.Skills == 0 {
		rep.add(SeverityInfo, "empty", agentsDir, "no skills yet — run minerva init / skill create")
	}

	rep.finalize()
	return rep, nil
}

func (r *LintReport) add(sev, kind, target, msg string) {
	r.Issues = append(r.Issues, Issue{Severity: sev, Kind: kind, Target: target, Message: msg})
}

func (r *LintReport) finalize() {
	for _, i := range r.Issues {
		switch i.Severity {
		case SeverityError:
			r.Errors++
		case SeverityWarning:
			r.Warnings++
		default:
			r.Infos++
		}
	}
	r.OK = r.Errors == 0
}

// FormatHuman renders a lint report for the terminal.
func FormatHuman(r *LintReport) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Library lint: skills=%d errors=%d warnings=%d infos=%d\n",
		r.Skills, r.Errors, r.Warnings, r.Infos)
	if len(r.Issues) == 0 {
		b.WriteString("  (no issues)\n")
		return b.String()
	}
	for _, i := range r.Issues {
		fmt.Fprintf(&b, "  [%s] %s %s — %s\n", strings.ToUpper(i.Severity), i.Kind, i.Target, i.Message)
	}
	if r.OK {
		b.WriteString("ok (no errors)\n")
	} else {
		b.WriteString("failed (errors present)\n")
	}
	return b.String()
}
