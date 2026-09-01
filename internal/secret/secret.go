// Package secret detects secret-like strings in skill bodies and session text.
//
// Hits and Redact share one pattern table so anything lint can flag, the MCP
// surface can also strip. Redact keeps the identifying prefix (key name,
// token family) and replaces only the secret material.
package secret

import (
	"regexp"
	"strings"
)

// pattern is one secret family. re must expose the secret material as the
// last capture group; earlier groups (if any) are kept verbatim by Redact.
type pattern struct {
	label string
	re    *regexp.Regexp
}

var patterns = []pattern{
	// Assignments: api_key: …, SECRET = "…", token=…, password: …
	{"key assignment", regexp.MustCompile(`(?i)((?:api[_-]?key|secret|passw(?:or)?d|token|credential)s?\s*[:=]\s*["']?)([A-Za-z0-9_\-./+=]{16,})`)},
	// Bearer / Basic auth headers
	{"authorization header", regexp.MustCompile(`(?i)(authorization\s*[:=]\s*["']?(?:bearer|basic)\s+)([A-Za-z0-9_\-./+=]{16,})`)},
	// Private key blocks (whole block redacted)
	{"private key", regexp.MustCompile(`(?s)(-----BEGIN [A-Z ]*PRIVATE KEY-----)(.*?)(-----END [A-Z ]*PRIVATE KEY-----)`)},
	// OpenAI / Anthropic style
	{"openai key", regexp.MustCompile(`\b(sk-(?:proj-|ant-)?)([A-Za-z0-9_\-]{20,})`)},
	// Stripe
	{"stripe key", regexp.MustCompile(`\b((?:sk|rk|pk)_(?:live|test)_)([A-Za-z0-9]{16,})`)},
	{"stripe webhook secret", regexp.MustCompile(`\b(whsec_)([A-Za-z0-9]{16,})`)},
	// GitHub
	{"github token", regexp.MustCompile(`\b(gh[pousr]_)([A-Za-z0-9]{20,})`)},
	{"github fine-grained token", regexp.MustCompile(`\b(github_pat_)([A-Za-z0-9_]{20,})`)},
	// Slack
	{"slack token", regexp.MustCompile(`\b(xox[baprs]-)([A-Za-z0-9-]{10,})`)},
	// AWS
	{"aws access key", regexp.MustCompile(`\b((?:AKIA|ASIA))([0-9A-Z]{16})\b`)},
	// Google API key
	{"google api key", regexp.MustCompile(`\b(AIza)([0-9A-Za-z_\-]{35})`)},
	// Vercel / npm / generic vendor prefixes
	{"vercel token", regexp.MustCompile(`\b(vercel_)([A-Za-z0-9]{20,})`)},
	{"npm token", regexp.MustCompile(`\b(npm_)([A-Za-z0-9]{30,})`)},
	// JWT
	{"jwt", regexp.MustCompile(`\b(eyJ[A-Za-z0-9_-]{8,}\.)([A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,})`)},
	// URLs with embedded credentials: scheme://user:pass@host
	{"url credential", regexp.MustCompile(`([a-z][a-z0-9+.-]*://[^/\s:@]+:)([^@\s/]{4,})(@)`)},
}

// Hits returns labels for secret-like patterns in text (nil when clean).
func Hits(text string) []string {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	var hits []string
	seen := map[string]bool{}
	for _, p := range patterns {
		if p.re.MatchString(text) && !seen[p.label] {
			seen[p.label] = true
			hits = append(hits, p.label)
		}
	}
	for _, key := range envKeysWithValues(text) {
		label := "env-like " + key
		if !seen[label] {
			seen[label] = true
			hits = append(hits, label)
		}
	}
	return hits
}

// Redact replaces secret material with [redacted], keeping key names and
// token-family prefixes so the text stays readable.
func Redact(text string) string {
	if text == "" {
		return text
	}
	out := text
	for _, p := range patterns {
		out = p.re.ReplaceAllStringFunc(out, func(m string) string {
			idx := p.re.FindStringSubmatchIndex(m)
			if idx == nil {
				return "[redacted]"
			}
			n := len(idx)/2 - 1 // number of groups
			if n == 0 {
				return "[redacted]"
			}
			// Secret group: last group, except for patterns that end with a
			// closing delimiter group (private key END line, url "@").
			secret := n
			if p.label == "private key" || p.label == "url credential" {
				secret = n - 1
			}
			var b strings.Builder
			for g := 1; g <= n; g++ {
				s, e := idx[2*g], idx[2*g+1]
				if s < 0 {
					continue
				}
				if g == secret {
					b.WriteString("[redacted]")
					continue
				}
				b.WriteString(m[s:e])
			}
			return b.String()
		})
	}
	return redactEnvLines(out)
}

// envKeysWithValues finds KEY=value lines where KEY looks secret-bearing and
// the value is long enough to be one.
func envKeysWithValues(text string) []string {
	var keys []string
	for _, line := range strings.Split(text, "\n") {
		if key, _, ok := splitEnvLine(line); ok {
			keys = append(keys, key)
		}
	}
	return keys
}

func redactEnvLines(text string) string {
	if !strings.Contains(text, "=") {
		return text
	}
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		key, val, ok := splitEnvLine(line)
		if !ok {
			continue
		}
		at := strings.LastIndex(line, val)
		if at < 0 {
			continue
		}
		_ = key
		lines[i] = line[:at] + "[redacted]" + line[at+len(val):]
	}
	return strings.Join(lines, "\n")
}

// splitEnvLine parses `export KEY=value` / `KEY="value"` where KEY names a
// secret and value is 16+ chars without whitespace. Returns the raw value
// (quotes stripped) so callers can locate it in the line.
func splitEnvLine(line string) (key, val string, ok bool) {
	t := strings.TrimSpace(line)
	t = strings.TrimPrefix(t, "export ")
	t = strings.TrimSpace(t)
	i := strings.IndexByte(t, '=')
	if i <= 0 {
		return "", "", false
	}
	key = strings.TrimSpace(t[:i])
	if !isEnvName(key) || !secretKeyName(key) {
		return "", "", false
	}
	val = strings.TrimSpace(t[i+1:])
	val = strings.Trim(val, `"'`)
	if len(val) < 16 || strings.ContainsAny(val, " \t") || val == "[redacted]" {
		return "", "", false
	}
	return key, val, true
}

func isEnvName(k string) bool {
	if k == "" {
		return false
	}
	for _, r := range k {
		if !(r == '_' || (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')) {
			return false
		}
	}
	return true
}

func secretKeyName(k string) bool {
	k = strings.ToUpper(k)
	for _, p := range []string{"KEY", "SECRET", "TOKEN", "PASSWORD", "PASSWD", "CREDENTIAL", "PRIVATE"} {
		if strings.Contains(k, p) {
			return true
		}
	}
	return false
}
