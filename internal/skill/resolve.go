package skill

import (
	"fmt"
	"sort"
	"strings"
	"unicode"

	"github.com/abdul-hamid-achik/minerva/internal/secret"
)

// ResolveHit is one ranked skill for an operator intent.
type ResolveHit struct {
	Name   string `json:"name"`
	Score  int    `json:"score"`
	Terms  int    `json:"terms"`
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
	// The query is echoed back; a pasted prompt can carry a secret.
	out := ResolveResult{Query: secret.Redact(q), Hits: []ResolveHit{}}
	if q == "" {
		return out
	}
	terms := dropCommonTerms(tokenize(q), skills, namedTerms(tokenize(q), skills))
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
		score, matched := scoreSkill(s, terms)
		if score <= 0 {
			continue
		}
		hit := ResolveHit{
			Name:   s.Name,
			Score:  score,
			Terms:  len(matched),
			Reason: "matched " + strings.Join(matched, ", "),
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

// commonTermMinCatalog is the catalog size from which catalog-wide term
// frequency is trusted enough to drop terms.
const commonTermMinCatalog = 6

// longDescriptionBytes marks descriptions that list many triggers; a single
// word hit inside one is weaker evidence than the same hit in a one-liner.
const longDescriptionBytes = 400

// dropCommonTerms removes query terms that appear in the name or description
// of a third or more of the catalog. "code", "test", "app" carry no signal
// when every skill mentions them; a small catalog keeps every term.
func dropCommonTerms(terms []string, skills []*Skill, keep map[string]bool) []string {
	terms = unique(terms)
	if len(skills) < commonTermMinCatalog {
		return terms
	}
	out := make([]string, 0, len(terms))
	for _, term := range terms {
		if keep[term] {
			out = append(out, term)
			continue
		}
		df := 0
		for _, s := range skills {
			if s == nil {
				continue
			}
			if containsWord(strings.ToLower(s.Name), term) || containsWord(strings.ToLower(s.Description), term) {
				df++
			}
		}
		if df*3 >= len(skills) {
			continue
		}
		out = append(out, term)
	}
	return out
}

// namedTerms returns the query terms that spell out a whole skill name
// ("docker-workflow" or "docker workflow" for skill docker-workflow). They
// are never dropped as common: naming a skill is the strongest signal there
// is, even when many skills share its words.
func namedTerms(terms []string, skills []*Skill) map[string]bool {
	inQuery := map[string]bool{}
	for _, t := range terms {
		inQuery[t] = true
	}
	keep := map[string]bool{}
	for _, s := range skills {
		if s == nil {
			continue
		}
		parts := tokenize(s.Name)
		all := len(parts) > 0
		for _, p := range parts {
			all = all && inQuery[p]
		}
		if all {
			for _, p := range parts {
				keep[p] = true
			}
		}
	}
	return keep
}

// scoreSkill returns a score and the distinct query terms that hit. A skill
// with zero term hits scores zero: description style alone ("Use when …")
// never makes it a candidate.
func scoreSkill(s *Skill, terms []string) (int, []string) {
	name := strings.ToLower(s.Name)
	desc := strings.ToLower(s.Description)
	bodyHead := strings.ToLower(s.Content)
	if len(bodyHead) > 2000 {
		bodyHead = bodyHead[:2000]
	}
	descHit := 3
	if len(desc) > longDescriptionBytes {
		descHit = 2
	}
	score := 0
	var matched []string
	for _, term := range unique(terms) {
		if term == "" {
			continue
		}
		st := stem(term)
		switch {
		case name == term || strings.ReplaceAll(name, "-", "") == term:
			score += 8
		case containsWord(name, term) || containsWord(name, st):
			score += 5
		case containsWord(desc, term) || containsWord(desc, st):
			score += descHit
		case containsWord(bodyHead, term) || containsWord(bodyHead, st):
			score++
		default:
			continue
		}
		matched = append(matched, term)
	}
	if len(matched) == 0 {
		return 0, nil
	}
	if strings.Contains(desc, "use when") {
		score += 2
	}
	return score, matched
}

// stem strips a common English suffix so "testing" reaches "test" and
// "reviews" reaches "review". Short results fall back to the input.
func stem(term string) string {
	for _, suf := range []string{"ing", "ers", "ies", "es", "ed", "er", "s"} {
		if strings.HasSuffix(term, suf) {
			s := strings.TrimSuffix(term, suf)
			if suf == "ies" {
				s += "y"
			}
			if len(s) >= 4 {
				return s
			}
		}
	}
	return term
}

// containsWord matches term as a whole token (stem-tolerant: "test" hits
// "tests" and "testing" but not "attest" or "contest").
func containsWord(text, term string) bool {
	idx := 0
	for {
		i := strings.Index(text[idx:], term)
		if i < 0 {
			return false
		}
		start := idx + i
		end := start + len(term)
		before := start == 0 || !isWordByte(text[start-1])
		if before {
			// allow common suffixes after the term
			rest := text[end:]
			if rest == "" || !isWordByte(rest[0]) {
				return true
			}
			for _, suf := range []string{"s", "es", "d", "ed", "ing", "r", "er", "rs", "ers", "ion", "ions"} {
				if strings.HasPrefix(rest, suf) && (len(rest) == len(suf) || !isWordByte(rest[len(suf)])) {
					return true
				}
			}
		}
		idx = end
		if idx >= len(text) {
			return false
		}
	}
}

func isWordByte(b byte) bool {
	return b == '_' || (b >= 'a' && b <= 'z') || (b >= '0' && b <= '9') || b >= 0x80
}

// maxQueryTerms bounds how many distinct terms a prompt contributes. Long
// pasted prompts would otherwise match every skill on incidental words.
const maxQueryTerms = 40

// stopWords are function words in English and Spanish plus markdown noise.
var stopWords = map[string]bool{
	// en
	"a": true, "an": true, "the": true, "this": true, "that": true, "these": true, "those": true,
	"for": true, "and": true, "or": true, "to": true, "of": true, "in": true, "on": true, "at": true,
	"by": true, "as": true, "is": true, "are": true, "was": true, "were": true, "be": true, "been": true,
	"it": true, "its": true, "my": true, "our": true, "your": true, "their": true, "his": true, "her": true,
	"i": true, "we": true, "you": true, "he": true, "she": true, "they": true, "me": true, "us": true, "them": true,
	"please": true, "want": true, "need": true, "how": true, "do": true, "does": true, "did": true, "done": true,
	"can": true, "could": true, "should": true, "would": true, "will": true, "shall": true, "may": true, "might": true,
	"not": true, "no": true, "yes": true, "but": true, "if": true, "then": true, "else": true, "so": true,
	"with": true, "without": true, "from": true, "into": true, "over": true, "under": true, "through": true,
	"all": true, "any": true, "some": true, "each": true, "every": true, "only": true, "also": true, "just": true,
	"what": true, "which": true, "who": true, "when": true, "where": true, "why": true, "here": true, "there": true,
	"have": true, "has": true, "had": true, "make": true, "made": true, "get": true, "got": true, "let": true,
	"use": true, "using": true, "used": true, "like": true, "than": true, "more": true, "most": true, "very": true,
	"one": true, "two": true, "first": true, "last": true, "new": true, "same": true, "other": true, "before": true,
	"after": true, "now": true, "already": true, "still": true, "again": true, "out": true, "up": true, "down": true,
	"about": true, "give": true, "take": true, "see": true, "look": true, "run": true, "check": true, "list": true,
	"file": true, "files": true, "code": true, "thing": true, "things": true, "way": true, "work": true, "works": true,
	"true": true, "false": true, "null": true, "none": true, "etc": true, "via": true, "per": true,
	"add": true, "fix": true, "set": true, "put": true, "help": true, "show": true, "tell": true,
	// es
	"el": true, "la": true, "los": true, "las": true, "un": true, "una": true, "unos": true, "unas": true,
	"de": true, "del": true, "al": true, "y": true, "o": true, "que": true, "en": true, "con": true, "sin": true,
	"por": true, "para": true, "es": true, "son": true, "ser": true, "esta": true, "está": true, "estan": true,
	"este": true, "esta_": true, "esto": true, "ese": true, "esa": true, "eso": true, "mi": true, "tu": true, "su": true,
	"lo": true, "le": true, "les": true, "se": true, "si": true, "sí": true, "ya": true, "muy": true, "mas": true, "más": true,
	"pero": true, "como": true, "cómo": true, "cuando": true, "donde": true, "porque": true, "podrias": true, "podrías": true,
	"puedes": true, "quiero": true, "necesito": true, "favor": true, "gracias": true, "chat": true, "también": true, "tambien": true,
	"todo": true, "toda": true, "todos": true, "todas": true, "hacer": true, "haz": true, "hay": true, "bien": true,
}

func tokenize(q string) []string {
	q = strings.ToLower(q)
	parts := strings.FieldsFunc(q, func(r rune) bool {
		return !(unicode.IsLetter(r) || unicode.IsDigit(r))
	})
	out := make([]string, 0, len(parts))
	seen := map[string]bool{}
	for _, p := range parts {
		if len(p) < 2 || stopWords[p] || seen[p] {
			continue
		}
		if isNumeric(p) {
			continue
		}
		seen[p] = true
		out = append(out, p)
		if len(out) >= maxQueryTerms {
			break
		}
	}
	return out
}

func isNumeric(s string) bool {
	for _, r := range s {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return true
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
