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
	"path/filepath"
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

// StashEntry is a minimal fcheap list row.
type StashEntry struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Tool      string   `json:"tool"`
	Tags      []string `json:"tags"`
	CreatedAt string   `json:"created_at"`
	// Parsed from tags for suggest.
	Skills   []string `json:"skills,omitempty"`
	Profiles []string `json:"profiles,omitempty"`
}

// ListByTags runs `fcheap list --json` with AND tag filters.
func ListByTags(ctx context.Context, tags ...string) ([]StashEntry, error) {
	if _, err := exec.LookPath("fcheap"); err != nil {
		return nil, fmt.Errorf("fcheap not found in PATH")
	}
	if ctx == nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
	}
	args := []string{"list", "--json"}
	for _, t := range tags {
		t = strings.TrimSpace(t)
		if t != "" {
			args = append(args, "--tag", t)
		}
	}
	cmd := exec.CommandContext(ctx, "fcheap", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("fcheap list: %w\n%s", err, bytes.TrimSpace(out))
	}
	raw := bytes.TrimSpace(out)
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var entries []StashEntry
	if err := json.Unmarshal(raw, &entries); err != nil {
		return nil, fmt.Errorf("parse fcheap list: %w", err)
	}
	for i := range entries {
		entries[i].Skills, entries[i].Profiles = parseSkillProfileTags(entries[i].Tags)
	}
	return entries, nil
}

// ListOutcomeFails returns minerva stashes tagged outcome:fail (includes closed).
func ListOutcomeFails(ctx context.Context) ([]StashEntry, error) {
	return ListByTags(ctx, TagMinerva, TagOutcomeFail)
}

// ListEvalFails returns eval stashes that failed.
func ListEvalFails(ctx context.Context) ([]StashEntry, error) {
	return ListByTags(ctx, TagMinerva, TagEval, TagOutcomeFail)
}

// TagClosesPrefix marks a close-receipt that resolves a prior fail stash.
const TagClosesPrefix = "closes:"

// TagOutcomeClosed is applied to close receipts (in addition to outcome:pass).
const TagOutcomeClosed = "outcome:closed"

// CloseRequest closes an open fail by writing a pass/closed receipt stash.
// fcheap has no in-place re-tag; Minerva links receipts via closes:<id>.
type CloseRequest struct {
	StashID string // original fail stash id
	Note    string // optional operator note
	Kind    string // eval|stack|incident|other (default: eval)
}

// CloseResult is the close receipt result.
type CloseResult struct {
	ClosedID  string      `json:"closed_id"`
	ReceiptID string      `json:"receipt_id,omitempty"`
	Save      *SaveResult `json:"save,omitempty"`
}

// Close writes a durable close receipt for a fail stash.
// Suggest / status treat fails with a matching closes:<id> receipt as resolved.
func Close(ctx context.Context, req CloseRequest) (*CloseResult, error) {
	id := strings.TrimSpace(req.StashID)
	if id == "" {
		return nil, fmt.Errorf("stash id is required")
	}
	kind := strings.TrimSpace(req.Kind)
	if kind == "" {
		kind = "eval"
	}

	// Best-effort: pull attribution tags from the original stash.
	extra := []string{TagClosesPrefix + id, TagOutcomeClosed}
	if info, err := Info(ctx, id); err == nil {
		skills, profiles := parseSkillProfileTags(info.Tags)
		for _, s := range skills {
			extra = append(extra, "skill:"+s)
		}
		for _, p := range profiles {
			extra = append(extra, "profile:"+p)
		}
	}

	payload := map[string]any{
		"action":    "close",
		"closed_id": id,
		"note":      req.Note,
		"at":        time.Now().UTC().Format(time.RFC3339),
	}
	res, err := SaveJSON(ctx, "close-"+id, kind, "pass", extra, payload)
	if err != nil {
		return nil, err
	}
	out := &CloseResult{ClosedID: id, Save: res}
	if res != nil {
		out.ReceiptID = res.ID
	}
	return out, nil
}

// Info fetches stash metadata via fcheap info --json.
func Info(ctx context.Context, stashID string) (*StashEntry, error) {
	if _, err := exec.LookPath("fcheap"); err != nil {
		return nil, fmt.Errorf("fcheap not found in PATH")
	}
	if strings.TrimSpace(stashID) == "" {
		return nil, fmt.Errorf("stash id is required")
	}
	if ctx == nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
	}
	cmd := exec.CommandContext(ctx, "fcheap", "info", stashID, "--json")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("fcheap info: %w\n%s", err, bytes.TrimSpace(out))
	}
	raw := bytes.TrimSpace(out)
	var entry StashEntry
	if err := json.Unmarshal(raw, &entry); err != nil {
		// Tolerate envelope shapes.
		var env struct {
			OK   bool       `json:"ok"`
			Data StashEntry `json:"data"`
		}
		if err2 := json.Unmarshal(raw, &env); err2 != nil || env.Data.ID == "" {
			return nil, fmt.Errorf("parse fcheap info: %w", err)
		}
		entry = env.Data
	}
	if entry.ID == "" {
		entry.ID = stashID
	}
	entry.Skills, entry.Profiles = parseSkillProfileTags(entry.Tags)
	return &entry, nil
}

// ListClosedIDs returns stash ids that have a Minerva close receipt.
func ListClosedIDs(ctx context.Context) (map[string]bool, error) {
	// Close receipts carry outcome:closed + closes:<id>.
	entries, err := ListByTags(ctx, TagMinerva, TagOutcomeClosed)
	if err != nil {
		// Fallback: pass receipts may still have closes: tags if older.
		entries, err = ListByTags(ctx, TagMinerva, TagOutcomePass)
		if err != nil {
			return nil, err
		}
	}
	closed := make(map[string]bool)
	for _, e := range entries {
		for _, t := range e.Tags {
			if strings.HasPrefix(t, TagClosesPrefix) {
				id := strings.TrimSpace(strings.TrimPrefix(t, TagClosesPrefix))
				if id != "" {
					closed[id] = true
				}
			}
		}
	}
	return closed, nil
}

// ListOpenOutcomeFails returns fail stashes without a close receipt.
func ListOpenOutcomeFails(ctx context.Context) ([]StashEntry, error) {
	fails, err := ListOutcomeFails(ctx)
	if err != nil {
		return nil, err
	}
	if len(fails) == 0 {
		return nil, nil
	}
	closed, err := ListClosedIDs(ctx)
	if err != nil {
		// If we cannot list closes, surface all fails (honest degradation).
		return fails, nil
	}
	open := make([]StashEntry, 0, len(fails))
	for _, f := range fails {
		if closed[f.ID] {
			continue
		}
		open = append(open, f)
	}
	return open, nil
}

func parseSkillProfileTags(tags []string) (skills, profiles []string) {
	for _, t := range tags {
		t = strings.TrimSpace(t)
		switch {
		case strings.HasPrefix(t, "skill:"):
			if s := strings.TrimSpace(strings.TrimPrefix(t, "skill:")); s != "" {
				skills = append(skills, s)
			}
		case strings.HasPrefix(t, "profile:"):
			if s := strings.TrimSpace(strings.TrimPrefix(t, "profile:")); s != "" {
				profiles = append(profiles, s)
			}
		}
	}
	return skills, profiles
}

// SaveJSON writes v as JSON to a temp file and stashes it via fcheap.
// Caller owns cleanup of nothing — temp file is removed after save attempt.
func SaveJSON(ctx context.Context, name, kind, outcome string, extraTags []string, v any) (*SaveResult, error) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, err
	}
	dir, err := os.MkdirTemp("", "minerva-evidence-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)

	filename := "payload.json"
	if name != "" {
		// sanitize lightly for filesystem
		safe := strings.Map(func(r rune) rune {
			if r == '/' || r == '\\' || r == 0 {
				return '_'
			}
			return r
		}, name)
		filename = safe + ".json"
	}
	path := filepath.Join(dir, filename)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return nil, err
	}
	ttl := "30d"
	return Save(ctx, SaveRequest{
		Path:    path,
		Name:    name,
		Tags:    extraTags,
		Kind:    kind,
		Outcome: outcome,
		TTL:     ttl,
		Index:   true,
	})
}

// Docs returns human-readable convention notes.
func Docs() string {
	return `Minerva evidence conventions (via fcheap)

Tags always include "minerva". Kind adds one of:
  minerva-eval | minerva-suggest | minerva-stack | minerva-incident

Optional outcome tags:
  outcome:pass | outcome:fail | outcome:skip | outcome:closed

Close loop (fcheap has no in-place re-tag):
  minerva evidence close <fail-stash-id> [--note "..."]
  → saves a pass receipt tagged closes:<id> + outcome:closed
  → open fails = outcome:fail without a matching closes: receipt

Examples:
  minerva evidence save ./run-artifacts --kind eval --outcome pass --tag skill:qa-tester --index
  minerva evidence close abc123 --note "fixed index"
  minerva stack deep --stash
  fcheap list --tag minerva --tag outcome:fail --json
  fcheap list --tag minerva --tag outcome:closed --json

Attribution tags (optional, enable evidence-backed suggest):
  skill:<name>    profile:<name>

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
