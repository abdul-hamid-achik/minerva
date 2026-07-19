package library

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/abdul-hamid-achik/minerva/internal/profile"
	"github.com/abdul-hamid-achik/minerva/internal/skill"
	"github.com/abdul-hamid-achik/minerva/internal/templates"
)

// Severity levels for lint findings.
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
	Profiles int     `json:"profiles"`
	OK       bool    `json:"ok"`
}

// Lint scans skills, profiles, and templates under agentsDir.
func Lint(agentsDir string) (*LintReport, error) {
	rep := &LintReport{}

	skillMgr := skill.NewManagerWithState(agentsDir, filepath.Join(agentsDir, "skills"))
	if err := skillMgr.LoadAll(); err != nil {
		return nil, fmt.Errorf("load skills: %w", err)
	}
	profileMgr := profile.NewManager(agentsDir)
	if err := profileMgr.LoadAll(); err != nil {
		return nil, fmt.Errorf("load profiles: %w", err)
	}

	skills := skillMgr.All()
	profiles := profileMgr.All()
	rep.Skills = len(skills)
	rep.Profiles = len(profiles)

	// Profile load warnings
	for _, w := range profileMgr.Warnings() {
		rep.add(SeverityError, "profile-yaml", w.Dir, w.Message)
	}

	// Skill checks
	for _, s := range skills {
		if strings.TrimSpace(s.Description) == "" {
			rep.add(SeverityWarning, "skill-description", s.Name, "missing description (poor catalog discovery)")
		}
		if len(s.Description) > skill.MaxSkillDescriptionBytes {
			rep.add(SeverityError, "skill-description", s.Name, "description exceeds size limit")
		}
		if strings.TrimSpace(s.Content) == "" {
			rep.add(SeverityWarning, "skill-body", s.Name, "empty body")
		}
		if !utf8.ValidString(s.Content) {
			rep.add(SeverityError, "skill-body", s.Name, "body is not valid UTF-8")
		}
		for _, secret := range secretHits(s.Content) {
			rep.add(SeverityError, "secret", s.Name, "possible secret pattern: "+secret)
		}
		// Orphan: not referenced by any profile
		if !skillInProfiles(s.Name, profiles) {
			rep.add(SeverityInfo, "orphan-skill", s.Name, "not referenced by any profile")
		}
	}

	// Profile checks
	skillOnDisk := map[string]bool{}
	for _, s := range skills {
		skillOnDisk[s.Name] = true
	}
	for _, p := range profiles {
		if strings.TrimSpace(p.SystemPrompt) == "" {
			rep.add(SeverityError, "profile-prompt", p.Name, "empty system_prompt")
		}
		if len(p.Skills) == 0 {
			rep.add(SeverityWarning, "profile-skills", p.Name, "no skills listed")
		}
		for _, sk := range p.Skills {
			if !skillOnDisk[sk] {
				rep.add(SeverityError, "missing-skill", p.Name, fmt.Sprintf("references missing skill %q", sk))
			}
		}
		for _, secret := range secretHits(p.SystemPrompt) {
			rep.add(SeverityError, "secret", "profile:"+p.Name, "possible secret pattern: "+secret)
		}
	}

	// Templates on disk
	tplDir := templates.DefaultDir(agentsDir)
	tpls, err := templates.LoadDir(tplDir)
	if err != nil {
		rep.add(SeverityWarning, "templates", tplDir, err.Error())
	} else {
		for _, t := range tpls {
			if strings.TrimSpace(t.Prompt) == "" {
				rep.add(SeverityError, "template-prompt", t.Name, "empty prompt")
			}
			for _, sk := range t.Skills {
				if !skillOnDisk[sk] {
					rep.add(SeverityWarning, "template-skill", t.Name, fmt.Sprintf("recommends missing skill %q", sk))
				}
			}
		}
	}

	// Empty library
	if rep.Skills == 0 && rep.Profiles == 0 {
		rep.add(SeverityInfo, "empty", agentsDir, "no skills or profiles yet — run minerva init / template apply")
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

func skillInProfiles(name string, profiles []*profile.Profile) bool {
	for _, p := range profiles {
		for _, s := range p.Skills {
			if s == name {
				return true
			}
		}
	}
	return false
}

var secretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(api[_-]?key|secret|password|token)\s*[:=]\s*['"]?[A-Za-z0-9_\-]{16,}`),
	regexp.MustCompile(`(?i)BEGIN (RSA |OPENSSH |EC )?PRIVATE KEY`),
	regexp.MustCompile(`(?i)sk-[A-Za-z0-9]{20,}`),
	regexp.MustCompile(`(?i)ghp_[A-Za-z0-9]{20,}`),
	regexp.MustCompile(`(?i)xox[baprs]-[A-Za-z0-9-]{10,}`),
}

func secretHits(text string) []string {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	var hits []string
	seen := map[string]bool{}
	for _, re := range secretPatterns {
		if m := re.FindString(text); m != "" {
			// Redact mid-string
			label := re.String()
			if len(label) > 40 {
				label = label[:40] + "…"
			}
			if !seen[label] {
				seen[label] = true
				hits = append(hits, label)
			}
		}
	}
	// .env style files should never be skills — flag bare KEY=value long secrets
	if strings.Contains(text, "\n") {
		for _, line := range strings.Split(text, "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "export ") {
				line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
			}
			if i := strings.IndexByte(line, '='); i > 0 {
				key := line[:i]
				val := line[i+1:]
				if secretKeyName(key) && len(val) >= 16 && !strings.Contains(val, " ") {
					label := "env-like " + key
					if !seen[label] {
						seen[label] = true
						hits = append(hits, label)
					}
				}
			}
		}
	}
	return hits
}

func secretKeyName(k string) bool {
	k = strings.ToUpper(k)
	for _, p := range []string{"KEY", "SECRET", "TOKEN", "PASSWORD", "PASSWD", "CREDENTIAL"} {
		if strings.Contains(k, p) {
			return true
		}
	}
	return false
}

// FormatHuman renders a lint report for the terminal.
func FormatHuman(r *LintReport) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Library lint: skills=%d profiles=%d errors=%d warnings=%d infos=%d\n",
		r.Skills, r.Profiles, r.Errors, r.Warnings, r.Infos)
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


