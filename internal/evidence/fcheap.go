// Package evidence provides conventions and helpers for durable Minerva
// outcomes stored via fcheap (file.cheap). Minerva does not reimplement the
// vault — it shells fcheap with standard tags for later search/suggest.
package evidence

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Standard tags for Minerva-related stashes.
const (
	TagMinerva     = "minerva"
	TagEval        = "minerva-eval"
	TagSuggest     = "minerva-suggest"
	TagStack       = "minerva-stack"
	TagIncident    = "minerva-incident"
	TagOutcomePass = "outcome:pass"
	TagOutcomeFail = "outcome:fail"
	TagOutcomeSkip = "outcome:skip"
)

// SaveRequest is a structured fcheap save for Minerva evidence.
type SaveRequest struct {
	Path    string   // file or directory to stash
	Name    string   // display name
	Tags    []string // extra tags (standard tags are added)
	Kind    string   // eval|suggest|stack|incident|other
	Outcome string   // pass|fail|skip|""
	TTL     string   // e.g. 30d
	Index   bool
}

// SaveResult is the fcheap save response (subset).
type SaveResult struct {
	OK      bool   `json:"ok"`
	ID      string `json:"id"`
	StashID string `json:"stash_id"`
	Error   string `json:"error"`
	Raw     string `json:"raw,omitempty"`
}

// StandardTags builds the tag set for a Minerva evidence stash.
func StandardTags(kind, outcome string, extra []string) []string {
	tags := []string{TagMinerva}
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "eval":
		tags = append(tags, TagEval)
	case "suggest":
		tags = append(tags, TagSuggest)
	case "stack":
		tags = append(tags, TagStack)
	case "incident":
		tags = append(tags, TagIncident)
	}
	switch strings.ToLower(strings.TrimSpace(outcome)) {
	case "pass":
		tags = append(tags, TagOutcomePass)
	case "fail":
		tags = append(tags, TagOutcomeFail)
	case "skip":
		tags = append(tags, TagOutcomeSkip)
	}
	for _, t := range extra {
		t = strings.TrimSpace(t)
		if t != "" {
			tags = append(tags, t)
		}
	}
	return dedupe(tags)
}

// Save shells `fcheap save` with Minerva tag conventions.
func Save(ctx context.Context, req SaveRequest) (*SaveResult, error) {
	if _, err := exec.LookPath("fcheap"); err != nil {
		return nil, fmt.Errorf("fcheap not found in PATH — install file.cheap")
	}
	if strings.TrimSpace(req.Path) == "" {
		return nil, fmt.Errorf("path is required")
	}
	if _, err := os.Stat(req.Path); err != nil {
		return nil, fmt.Errorf("path: %w", err)
	}

	tags := StandardTags(req.Kind, req.Outcome, req.Tags)
	args := []string{"save", req.Path, "--tool", "minerva", "--json"}
	if req.Name != "" {
		args = append(args, "--name", req.Name)
	}
	if req.TTL != "" {
		args = append(args, "--ttl", req.TTL)
	}
	if req.Index {
		args = append(args, "--index")
	}
	for _, t := range tags {
		args = append(args, "--tag", t)
	}

	if ctx == nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
	}

	cmd := exec.CommandContext(ctx, "fcheap", args...)
	out, err := cmd.CombinedOutput()
	res := &SaveResult{Raw: strings.TrimSpace(string(out))}
	if len(out) > 0 {
		_ = json.Unmarshal(bytes.TrimSpace(out), res)
		// Tolerate alternate id field names.
		if res.ID == "" && res.StashID != "" {
			res.ID = res.StashID
		}
		if res.ID == "" {
			var loose map[string]any
			if json.Unmarshal(bytes.TrimSpace(out), &loose) == nil {
				for _, k := range []string{"id", "stash_id", "stashId"} {
					if v, ok := loose[k].(string); ok && v != "" {
						res.ID = v
						break
					}
				}
				if data, ok := loose["data"].(map[string]any); ok {
					for _, k := range []string{"id", "stash_id"} {
						if v, ok := data[k].(string); ok && v != "" {
							res.ID = v
							break
						}
					}
				}
			}
		}
	}
	if err != nil {
		if res.Error == "" {
			res.Error = err.Error()
		}
		return res, fmt.Errorf("fcheap save: %w\n%s", err, res.Raw)
	}
	res.OK = true
	return res, nil
}

// SearchMinerva runs fcheap search limited by minerva tag when possible.
func SearchMinerva(ctx context.Context, query string) (string, error) {
	if _, err := exec.LookPath("fcheap"); err != nil {
		return "", fmt.Errorf("fcheap not found in PATH")
	}
	if ctx == nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
	}
	q := strings.TrimSpace(query)
	if q == "" {
		q = TagMinerva
	}
	cmd := exec.CommandContext(ctx, "fcheap", "search", q, "--json")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("fcheap search: %w", err)
	}
	return string(out), nil
}

// Docs returns human-readable convention notes.
func Docs() string {
	return `Minerva evidence conventions (via fcheap)

Tags always include "minerva". Kind adds one of:
  minerva-eval | minerva-suggest | minerva-stack | minerva-incident

Optional outcome tags:
  outcome:pass | outcome:fail | outcome:skip

Examples:
  minerva evidence save ./run-artifacts --kind eval --outcome pass --index
  minerva evidence save ./stack.json --kind stack --name "stack-deep"
  fcheap search minerva-eval --json
  fcheap list --tag minerva

Minerva never stores secret values. Do not stash .env or vault dumps.
`
}

func dedupe(in []string) []string {
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}
