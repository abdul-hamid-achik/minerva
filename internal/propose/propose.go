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

	"github.com/abdul-hamid-achik/minerva/internal/session"
	"github.com/abdul-hamid-achik/minerva/internal/signal"
	"github.com/abdul-hamid-achik/minerva/internal/skill"
)

const (
	KindNew     = "new_skill"
	KindUpdate  = "update_skill"
	KindLoadGap = "load_gap"
)

// ObservedHeading is the Minerva-owned section that update proposals merge into.
// Everything else in a SKILL.md body is left untouched.
const ObservedHeading = "## Observed patterns"

// legacyObservedHeading was appended verbatim by earlier apply runs; it is
// migrated into ObservedHeading on the next update.
const legacyObservedHeading = "## Observed later"

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

// MaxStored bounds how many proposals the store keeps across runs.
const MaxStored = 100

// Store holds the proposals of recent propose runs, newest first, so apply
// can find a draft by the id an earlier run printed.
type Store struct {
	GeneratedAt time.Time  `json:"generated_at"`
	Proposals   []Proposal `json:"proposals"`
}

func storePath(agentsDir string) string {
	return filepath.Join(agentsDir, ".minerva", "proposals.json")
}

// Save adds proposals to agentsDir/.minerva/proposals.json. A proposal
// replaces the stored one with the same id; others are kept (up to
// MaxStored), so a narrower or empty run never drops an id printed before.
// The file is replaced atomically, so a concurrent apply never reads half of
// it.
func Save(agentsDir string, proposals []Proposal) error {
	dir := filepath.Join(agentsDir, ".minerva")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	merged := append([]Proposal(nil), proposals...)
	seen := map[string]bool{}
	for _, p := range proposals {
		seen[p.ID] = true
	}
	if prev, err := LoadStore(agentsDir); err == nil {
		for _, p := range prev.Proposals {
			if !seen[p.ID] {
				seen[p.ID] = true
				merged = append(merged, p)
			}
		}
	}
	if len(merged) > MaxStored {
		merged = merged[:MaxStored]
	}
	st := Store{GeneratedAt: time.Now().UTC(), Proposals: merged}
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	return skill.WriteFileAtomic(storePath(agentsDir), data)
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
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("proposal %q not found — no proposals saved yet, run minerva propose first", id)
		}
		return nil, fmt.Errorf("read proposals store: %w", err)
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
		case signal.KindLongManual, signal.KindCorrection:
			// Diagnostic signals: they say a skill was missing, not which one.
			// analyze reports them; propose has nothing concrete to draft.
			continue
		case signal.KindRepeat, signal.KindRetry:
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
		if p.Name == "" || seen[p.Kind+p.Name] || trivialName(p.Name) || browseOnlyKey(sig.Key) {
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
		Action:      "minerva propose apply " + proposalID(KindUpdate, name) + " (merges into " + ObservedHeading + ")",
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
	// update: merge into the Minerva-owned "Observed patterns" section
	existing := mgr.Get(p.Name)
	if existing == nil {
		return "", fmt.Errorf("skill %q not found", p.Name)
	}
	body := MergeObserved(existing.Content, p)
	if body == existing.Content {
		return existing.Path, nil
	}
	if err := mgr.Update(p.Name, nil, &body); err != nil {
		return "", err
	}
	return existing.Path, nil
}

// MergeObserved returns body with p recorded as one bullet under
// ObservedHeading. Re-applying the same proposal updates that bullet's
// session count instead of appending again. A legacy "## Observed later"
// section is folded in. Content outside the section is byte-for-byte intact.
func MergeObserved(body string, p Proposal) string {
	head, bullets, tail := splitObserved(body)
	key := observedKey(p.Description)
	line := observedBullet(p)
	replaced := false
	for i, b := range bullets {
		if observedKey(b) == key {
			bullets[i] = line
			replaced = true
			break
		}
	}
	if !replaced {
		bullets = append(bullets, line)
	}
	var out strings.Builder
	out.WriteString(strings.TrimRight(head, "\n"))
	if out.Len() > 0 {
		out.WriteString("\n\n")
	}
	out.WriteString(ObservedHeading)
	out.WriteString("\n\n")
	for _, b := range bullets {
		out.WriteString(b)
		out.WriteString("\n")
	}
	if tail = strings.TrimSpace(tail); tail != "" {
		out.WriteString("\n")
		out.WriteString(tail)
		out.WriteString("\n")
	}
	return strings.TrimRight(out.String(), "\n")
}

// observedBullet renders one merged entry: description plus evidence summary.
func observedBullet(p Proposal) string {
	desc := strings.TrimSpace(p.Description)
	if len(p.Evidence) == 0 {
		return "- " + desc
	}
	n := len(p.Evidence)
	last := p.Evidence[n-1]
	unit := "sessions"
	if n == 1 {
		unit = "session"
	}
	ref := last.Harness
	if last.SessionID != "" {
		ref += " " + shortID(last.SessionID)
	}
	return fmt.Sprintf("- %s (%d %s; last %s)", desc, n, unit, strings.TrimSpace(ref))
}

// observedKey is the comparable part of a bullet: the description without
// the trailing evidence parenthetical or list marker.
func observedKey(line string) string {
	s := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "-"))
	if i := strings.LastIndex(s, " ("); i > 0 && strings.HasSuffix(s, ")") {
		s = s[:i]
	}
	return strings.ToLower(strings.TrimSpace(s))
}

// splitObserved returns the body before the observed section, its bullets,
// and everything after it. Legacy "## Observed later" paragraphs are
// converted to bullets. When no section exists, head is the whole body.
func splitObserved(body string) (head string, bullets []string, tail string) {
	lines := strings.Split(body, "\n")
	start := -1
	for i, l := range lines {
		t := strings.TrimSpace(l)
		if t == ObservedHeading || t == legacyObservedHeading {
			start = i
			break
		}
	}
	if start < 0 {
		return body, nil, ""
	}
	end := len(lines)
	for i := start + 1; i < len(lines); i++ {
		t := strings.TrimSpace(lines[i])
		if t == legacyObservedHeading {
			continue // several legacy appends in a row: absorb them all
		}
		if strings.HasPrefix(t, "#") {
			end = i
			break
		}
	}
	head = strings.Join(lines[:start], "\n")
	tail = strings.Join(lines[end:], "\n")
	seen := map[string]bool{}
	for _, l := range lines[start+1 : end] {
		t := strings.TrimSpace(l)
		if t == "" || t == legacyObservedHeading {
			continue
		}
		if !strings.HasPrefix(t, "-") {
			t = "- " + t
		}
		k := observedKey(t)
		if seen[k] {
			continue
		}
		seen[k] = true
		bullets = append(bullets, t)
	}
	return head, bullets, tail
}

func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
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

// trivialName rejects names like "read-read" where every part is the same tool.
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

// browseOnlyKey is a safety net for repeat keys ("A → B → C") made only of
// read/search tools. signal already drops these; proposals built from older
// stores or hand-written signals still get filtered here.
func browseOnlyKey(key string) bool {
	if !strings.Contains(key, "→") {
		return false
	}
	var names []string
	for _, p := range strings.Split(key, "→") {
		if p = strings.TrimSpace(p); p != "" {
			names = append(names, p)
		}
	}
	if len(names) < 2 {
		return false
	}
	return session.BrowseOnly(names...)
}

func canonicalTools(key string) string {
	parts := strings.Split(key, " → ")
	seen := map[string]bool{}
	var out []string
	for _, p := range parts {
		n := strings.ToLower(strings.TrimSpace(p))
		// "Bash(go)" → "go": the command is the meaningful part.
		if i := strings.IndexByte(n, '('); i >= 0 {
			n = strings.TrimSuffix(n[i+1:], ")")
		}
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
