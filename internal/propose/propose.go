// Package propose turns session signals into skill drafts.
package propose

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/abdul-hamid-achik/minerva/internal/signal"
	"github.com/abdul-hamid-achik/minerva/internal/skill"
)

const (
	KindNew     = "new_skill"
	KindUpdate  = "update_skill"
	KindLoadGap = "load_gap"
	KindMerge   = "merge"
)

// Proposal is one actionable skill recommendation.
type Proposal struct {
	ID          string            `json:"id"`
	Kind        string            `json:"kind"`
	Name        string            `json:"name"`
	Title       string            `json:"title"`
	Description string            `json:"description"`
	Draft       string            `json:"draft,omitempty"`
	Priority    int               `json:"priority"`
	Evidence    []signal.Evidence `json:"evidence"`
	Action      string            `json:"action"`
}

// Store is the last propose run, persisted so apply can find drafts.
type Store struct {
	GeneratedAt time.Time  `json:"generated_at"`
	Proposals   []Proposal `json:"proposals"`
}

func storePath(agentsDir string) string {
	return filepath.Join(agentsDir, ".minerva", "proposals.json")
}

// Save writes proposals to agentsDir/.minerva/proposals.json.
func Save(agentsDir string, proposals []Proposal) error {
	dir := filepath.Join(agentsDir, ".minerva")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	st := Store{GeneratedAt: time.Now().UTC(), Proposals: proposals}
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(storePath(agentsDir), data, 0o644)
}

// LoadStore reads the last propose run.
func LoadStore(agentsDir string) (Store, error) {
	var st Store
	data, err := os.ReadFile(storePath(agentsDir))
	if err != nil {
		return st, err
	}
	err = json.Unmarshal(data, &st)
	return st, err
}

// Find returns a proposal by id from the last store.
func Find(agentsDir, id string) (*Proposal, error) {
	st, err := LoadStore(agentsDir)
	if err != nil {
		return nil, err
	}
	for i := range st.Proposals {
		if st.Proposals[i].ID == id {
			return &st.Proposals[i], nil
		}
	}
	return nil, fmt.Errorf("proposal %q not found — run minerva propose first", id)
}

// FromSignals converts signals into a bounded proposal list.
func FromSignals(sigs []signal.Signal, catalog []*skill.Skill) []Proposal {
	onDisk := map[string]*skill.Skill{}
	for _, s := range catalog {
		onDisk[s.Name] = s
	}

	var out []Proposal
	seen := map[string]bool{}
	for _, sig := range sigs {
		var p Proposal
		switch sig.Kind {
		case signal.KindLoadGap:
			p = Proposal{
				Kind: KindLoadGap, Name: sig.Key,
				Title:       "Load existing skill " + sig.Key,
				Description: sig.Message,
				Priority:    1,
				Evidence:    sig.Evidence,
				Action:      "load skill " + sig.Key + " in the harness (or minerva skill show " + sig.Key + ")",
			}
		case signal.KindShellFam:
			name := slug(sig.Key + "-workflow")
			if onDisk[name] != nil {
				p = updateProposal(name, sig)
			} else {
				p = newSkillProposal(name, "Use when running repeated "+sig.Key+" commands in an agent session.", sig)
			}
		case signal.KindLongManual, signal.KindRepeat, signal.KindRetry, signal.KindCorrection:
			name := slug(guessName(sig))
			if onDisk[name] != nil {
				p = updateProposal(name, sig)
			} else if existing := closest(catalog, sig); existing != "" {
				p = updateProposal(existing, sig)
			} else {
				p = newSkillProposal(name, "Use when this session pattern repeats: "+sig.Message, sig)
			}
		default:
			continue
		}
		if p.Name == "" || seen[p.Kind+p.Name] || lowValueName(p.Name) || browseOnlyKey(sig.Key) {
			continue
		}
		p.ID = proposalID(p.Kind, p.Name)
		seen[p.Kind+p.Name] = true
		out = append(out, p)
		if len(out) >= 15 {
			break
		}
	}
	return out
}

func newSkillProposal(name, desc string, sig signal.Signal) Proposal {
	draft := RenderDraft(name, desc, sig)
	return Proposal{
		Kind: KindNew, Name: name, Title: "Create skill " + name,
		Description: desc, Draft: draft, Priority: 2, Evidence: sig.Evidence,
		Action: "minerva propose apply " + proposalID(KindNew, name),
	}
}

func updateProposal(name string, sig signal.Signal) Proposal {
	return Proposal{
		Kind: KindUpdate, Name: name,
		Title:       "Update skill " + name,
		Description: sig.Message,
		Priority:    3,
		Evidence:    sig.Evidence,
		Action:      "minerva skill show " + name,
	}
}

// RenderDraft writes a SKILL.md body from a signal.
func RenderDraft(name, description string, sig signal.Signal) string {
	var b strings.Builder
	b.WriteString("---\n")
	fmt.Fprintf(&b, "name: %q\n", name)
	fmt.Fprintf(&b, "description: %q\n", description)
	b.WriteString("---\n\n")
	fmt.Fprintf(&b, "# %s\n\n", titleize(name))
	b.WriteString("## When\n\n")
	fmt.Fprintf(&b, "%s\n\n", description)
	b.WriteString("## Observed pattern\n\n")
	fmt.Fprintf(&b, "- %s (`%s`)\n", sig.Message, sig.Kind)
	if len(sig.Evidence) > 0 {
		b.WriteString("\n## Evidence\n\n")
		limit := len(sig.Evidence)
		if limit > 5 {
			limit = 5
		}
		for _, ev := range sig.Evidence[:limit] {
			fmt.Fprintf(&b, "- %s session `%s`", ev.Harness, ev.SessionID)
			if ev.Workspace != "" {
				fmt.Fprintf(&b, " (%s)", ev.Workspace)
			}
			b.WriteString("\n")
		}
	}
	b.WriteString("\n## Steps\n\n")
	b.WriteString("1. Restate the user goal.\n")
	b.WriteString("2. Follow the observed tool sequence instead of rediscovering it.\n")
	b.WriteString("3. Stop when the original request is verified.\n")
	return b.String()
}

// Apply writes a new_skill proposal to the skills directory.
func Apply(agentsDir string, p Proposal) (string, error) {
	if p.Kind != KindNew && p.Kind != KindUpdate {
		return "", fmt.Errorf("proposal %s is not applyable (kind %s)", p.ID, p.Kind)
	}
	if strings.TrimSpace(p.Draft) == "" && p.Kind == KindNew {
		return "", fmt.Errorf("proposal %s has no draft", p.ID)
	}
	mgr := skill.ForAgents(agentsDir)
	if err := mgr.LoadAll(); err != nil {
		return "", err
	}
	skillsDir := filepath.Join(agentsDir, "skills")
	if p.Kind == KindNew {
		if mgr.Has(p.Name) {
			return "", fmt.Errorf("skill %q already exists", p.Name)
		}
		if err := mgr.Create(skillsDir, p.Name, p.Description, bodyOf(p.Draft)); err != nil {
			return "", err
		}
		return filepath.Join(skillsDir, p.Name, "SKILL.md"), nil
	}
	// update: append an Observed pattern section if draft present
	existing := mgr.Get(p.Name)
	if existing == nil {
		return "", fmt.Errorf("skill %q not found", p.Name)
	}
	body := existing.Content
	if !strings.Contains(body, p.Description) {
		body = body + "\n\n## Observed later\n\n" + p.Description + "\n"
	}
	if err := mgr.Update(p.Name, nil, &body); err != nil {
		return "", err
	}
	return existing.Path, nil
}

func bodyOf(draft string) string {
	if i := strings.LastIndex(draft, "---\n"); i >= 0 {
		rest := draft[i+4:]
		return strings.TrimSpace(rest)
	}
	return draft
}

func proposalID(kind, name string) string {
	sum := sha256.Sum256([]byte(kind + ":" + name))
	return kind + "-" + hex.EncodeToString(sum[:4])
}

func slug(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	prevDash := false
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			prevDash = false
		default:
			if !prevDash && b.Len() > 0 {
				b.WriteByte('-')
				prevDash = true
			}
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "session-pattern"
	}
	if len(out) > 48 {
		out = out[:48]
	}
	return out
}

func titleize(slug string) string {
	parts := strings.Split(slug, "-")
	for i, p := range parts {
		if p == "" {
			continue
		}
		parts[i] = strings.ToUpper(p[:1]) + p[1:]
	}
	return strings.Join(parts, " ")
}

func guessName(sig signal.Signal) string {
	switch sig.Kind {
	case signal.KindCorrection:
		return "recovery-loop"
	case signal.KindRetry:
		return slug(sig.Key + "-retry")
	case signal.KindLongManual:
		return slug(sig.Key + "-session")
	case signal.KindRepeat:
		return slug(canonicalTools(sig.Key) + "-loop")
	default:
		if sig.Key != "" {
			return sig.Key
		}
		return "session-pattern"
	}
}

func lowValueName(name string) bool {
	return trivialName(name) || browseOnlyName(name)
}

func trivialName(name string) bool {
	parts := strings.Split(name, "-")
	if len(parts) < 2 {
		return false
	}
	first := parts[0]
	for _, p := range parts {
		if p != first {
			return false
		}
	}
	return true
}

func browseOnlyName(name string) bool {
	name = strings.TrimSuffix(name, "-loop")
	parts := strings.Split(name, "-")
	if len(parts) < 2 {
		return false
	}
	browse := map[string]bool{
		"read": true, "grep": true, "glob": true, "list": true, "search": true,
		"view": true, "find": true, "cat": true, "ls": true,
	}
	for _, p := range parts {
		if !browse[p] {
			return false
		}
	}
	return true
}

func browseOnlyKey(key string) bool {
	if !strings.Contains(key, "→") {
		return false
	}
	var names []string
	for _, p := range strings.Split(key, "→") {
		p = strings.TrimSpace(p)
		if p != "" {
			names = append(names, p)
		}
	}
	if len(names) < 2 {
		return false
	}
	browse := map[string]bool{
		"read": true, "grep": true, "glob": true, "list": true, "search": true,
		"view": true, "find": true, "cat": true, "ls": true,
	}
	for _, n := range names {
		n = strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(n, "_", ""), "-", ""))
		if !browse[n] {
			return false
		}
	}
	return true
}

func canonicalTools(key string) string {
	parts := strings.Split(key, " → ")
	seen := map[string]bool{}
	var out []string
	for _, p := range parts {
		n := strings.ToLower(strings.TrimSpace(p))
		n = strings.ReplaceAll(n, "_", "")
		n = strings.ReplaceAll(n, "-", "")
		if n == "" || seen[n] {
			continue
		}
		seen[n] = true
		out = append(out, n)
	}
	sort.Strings(out)
	if len(out) == 0 {
		return "session-pattern"
	}
	return strings.Join(out, "-")
}

func closest(catalog []*skill.Skill, sig signal.Signal) string {
	key := strings.ToLower(sig.Key + " " + sig.Message)
	for _, s := range catalog {
		if strings.Contains(key, strings.ToLower(s.Name)) {
			return s.Name
		}
	}
	return ""
}
