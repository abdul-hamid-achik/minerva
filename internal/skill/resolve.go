package skill

import (
	"fmt"
	"sort"
	"strings"
)

// ResolveHit is one ranked skill for an operator intent.
type ResolveHit struct {
	Name   string `json:"name"`
	Score  int    `json:"score"`
	Reason string `json:"reason"`
	Action string `json:"action"`
}

// ResolveResult is the stable skill resolve --json payload.
type ResolveResult struct {
	Query string       `json:"query"`
	Hits  []ResolveHit `json:"hits"`
}

const maxResolveHits = 5

// Resolve ranks catalog skills against a natural-language intent.
func Resolve(query string, skills []*Skill) ResolveResult {
	q := strings.TrimSpace(query)
	out := ResolveResult{Query: q, Hits: []ResolveHit{}}
	if q == "" {
		return out
	}
	terms := tokenize(q)
	if len(terms) == 0 {
		return out
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
		hit := ResolveHit{
			Name:   s.Name,
			Score:  score,
			Reason: reason,
			Action: fmt.Sprintf("load skill %s in the current harness (or minerva skill show %s)", s.Name, s.Name),
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
	if strings.Contains(desc, "use when") {
		score += 2
	}
	if score == 0 {
		return 0, ""
	}
	reason := "matched " + strings.Join(unique(matched), ", ")
	if reason == "matched " {
		reason = "description match"
	}
	_ = rawQuery
	return score, reason
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
