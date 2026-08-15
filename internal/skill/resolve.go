package skill

import (
	"fmt"
	"sort"
	"strings"

	"github.com/abdul-hamid-achik/minerva/internal/profile"
)

// ResolveHit is one ranked skill for an operator intent.
type ResolveHit struct {
	Name       string   `json:"name"`
	Score      int      `json:"score"`
	Reason     string   `json:"reason"`
	OnProfiles []string `json:"on_profiles,omitempty"`
	Orphan     bool     `json:"orphan"`
	Action     string   `json:"action"`
}

// ResolveResult is the stable skill resolve --json payload.
type ResolveResult struct {
	Query string       `json:"query"`
	Hits  []ResolveHit `json:"hits"`
}

const maxResolveHits = 5

// Resolve ranks catalog skills against a natural-language intent.
func Resolve(query string, skills []*Skill, profiles []*profile.Profile) ResolveResult {
	q := strings.TrimSpace(query)
	out := ResolveResult{Query: q, Hits: []ResolveHit{}}
	if q == "" {
		return out
	}
	terms := tokenize(q)
	if len(terms) == 0 {
		return out
	}

	onProfiles := map[string][]string{}
	for _, p := range profiles {
		if p == nil {
			continue
		}
		for _, name := range p.Skills {
			onProfiles[name] = append(onProfiles[name], p.Name)
		}
	}

	type scored struct {
		hit   ResolveHit
		score int
	}
	var ranked []scored
	for _, s := range skills {
		if s == nil {
			continue
		}
		score, reason := scoreSkill(s, terms, q)
		if score <= 0 {
			continue
		}
		owners := append([]string(nil), onProfiles[s.Name]...)
		sort.Strings(owners)
		hit := ResolveHit{
			Name:       s.Name,
			Score:      score,
			Reason:     reason,
			OnProfiles: owners,
			Orphan:     len(owners) == 0,
			Action:     resolveAction(s.Name, owners),
		}
		ranked = append(ranked, scored{hit: hit, score: score})
	}
	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].score == ranked[j].score {
			return ranked[i].hit.Name < ranked[j].hit.Name
		}
		return ranked[i].score > ranked[j].score
	})
	if len(ranked) > maxResolveHits {
		ranked = ranked[:maxResolveHits]
	}
	for _, r := range ranked {
		out.Hits = append(out.Hits, r.hit)
	}
	return out
}

func resolveAction(name string, owners []string) string {
	if len(owners) > 0 {
		return fmt.Sprintf("load_skill %s  # already on profile %s", name, owners[0])
	}
	return fmt.Sprintf("minerva profile add-skills <workspace-profile> %s", name)
}

func scoreSkill(s *Skill, terms []string, rawQuery string) (int, string) {
	name := strings.ToLower(s.Name)
	desc := strings.ToLower(s.Description)
	bodyHead := strings.ToLower(s.Content)
	if len(bodyHead) > 2000 {
		bodyHead = bodyHead[:2000]
	}
	score := 0
	var matched []string
	for _, term := range terms {
		if term == "" {
			continue
		}
		if name == term || strings.ReplaceAll(name, "-", "") == strings.ReplaceAll(term, "-", "") {
			score += 8
			matched = append(matched, term)
			continue
		}
		if strings.Contains(name, term) {
			score += 5
			matched = append(matched, term)
			continue
		}
		if strings.Contains(desc, term) {
			score += 3
			matched = append(matched, term)
			continue
		}
		if strings.Contains(bodyHead, term) {
			score += 1
			matched = append(matched, term)
		}
	}
	score += synonymBonus(name+" "+desc, rawQuery)
	if score == 0 {
		return 0, ""
	}
	reason := "matched " + strings.Join(unique(matched), ", ")
	if reason == "matched " {
		reason = "synonym match"
	}
	return score, reason
}

func synonymBonus(haystack, rawQuery string) int {
	q := strings.ToLower(rawQuery)
	pairs := []struct {
		needles []string
		skill   []string
		bonus   int
	}{
		{[]string{"pull request", "pr review", "code review", "review this pr"}, []string{"review", "qa-tester", "sharp-edges", "differential"}, 6},
		{[]string{"frontend", "ui", "react", "vue"}, []string{"frontend", "ux-designer", "web-design"}, 6},
		{[]string{"docs", "readme", "documentation"}, []string{"doc-writer", "writing-guidelines"}, 6},
		{[]string{"stripe", "payments", "billing"}, []string{"stripe"}, 6},
		{[]string{"test", "testing", "qa"}, []string{"qa-tester", "webapp-testing"}, 4},
	}
	bonus := 0
	for _, p := range pairs {
		hitQ := false
		for _, n := range p.needles {
			if strings.Contains(q, n) {
				hitQ = true
				break
			}
		}
		if !hitQ {
			continue
		}
		for _, sk := range p.skill {
			if strings.Contains(haystack, sk) {
				bonus += p.bonus
				break
			}
		}
	}
	return bonus
}

func tokenize(q string) []string {
	q = strings.ToLower(q)
	repl := strings.NewReplacer("/", " ", ",", " ", ".", " ", ":", " ", "_", " ", "-", " ")
	q = repl.Replace(q)
	parts := strings.Fields(q)
	stop := map[string]bool{
		"a": true, "an": true, "the": true, "this": true, "that": true,
		"for": true, "and": true, "or": true, "to": true, "of": true,
		"my": true, "our": true, "i": true, "we": true, "please": true,
		"want": true, "need": true, "how": true, "do": true, "me": true,
	}
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if stop[p] || len(p) < 2 {
			continue
		}
		out = append(out, p)
	}
	return out
}

func unique(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}
