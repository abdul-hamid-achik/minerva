// Package secret detects secret-like strings in skill bodies and session text.
package secret

import (
	"regexp"
	"strings"
)

var patterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(api[_-]?key|secret|password|token)\s*[:=]\s*['"]?[A-Za-z0-9_\-]{16,}`),
	regexp.MustCompile(`(?i)BEGIN (RSA |OPENSSH |EC )?PRIVATE KEY`),
	regexp.MustCompile(`(?i)sk-[A-Za-z0-9]{20,}`),
	regexp.MustCompile(`(?i)ghp_[A-Za-z0-9]{20,}`),
	regexp.MustCompile(`(?i)xox[baprs]-[A-Za-z0-9-]{10,}`),
}

// Hits returns redacted labels for secret-like patterns in text.
func Hits(text string) []string {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	var hits []string
	seen := map[string]bool{}
	for _, re := range patterns {
		if m := re.FindString(text); m != "" {
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

var redactPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(sk-[A-Za-z0-9]{20,})`),
	regexp.MustCompile(`(?i)(ghp_[A-Za-z0-9]{20,})`),
	regexp.MustCompile(`(?i)(xox[baprs]-[A-Za-z0-9-]{10,})`),
	regexp.MustCompile(`(?i)((?:api[_-]?key|secret|password|token)\s*[:=]\s*['"]?)([A-Za-z0-9_\-]{16,})`),
}

// Redact replaces secret-like values with [redacted].
func Redact(text string) string {
	if text == "" {
		return text
	}
	out := text
	for _, re := range redactPatterns {
		out = re.ReplaceAllString(out, "${1}[redacted]")
	}
	return out
}
