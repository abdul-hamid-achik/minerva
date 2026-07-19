// Package analytics tracks skill and profile usage to inform suggestions.
// Events are append-safe: every Record loads existing history first.
package analytics

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

const analyticsFileName = ".minerva-analytics.json"

// Event is one usage event.
type Event struct {
	Timestamp time.Time `json:"timestamp"`
	Kind      string    `json:"kind"`
	Target    string    `json:"target"`
	Detail    string    `json:"detail"`
}

// Summary is aggregate analytics.
// Note: this is Minerva-local only (CLI/MCP mutations). It does not observe
// local-agent session skill loads or harness tool calls (see mcphub/cortex).
type Summary struct {
	TotalEvents        int            `json:"total_events"`
	SkillActivations   map[string]int `json:"skill_activations"`
	ProfileSwitches    map[string]int `json:"profile_switches"`
	SuggestionsApplied int            `json:"suggestions_applied"`
	LastActivity       time.Time      `json:"last_activity"`
	TopSkills          []string       `json:"top_skills"`
	TopProfiles        []string       `json:"top_profiles"`
	// Note is always set so operators do not confuse this with harness telemetry.
	Note string `json:"note,omitempty"`
}

// TopSkillEntry is a skill with activation count.
type TopSkillEntry struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// Store persists usage analytics to disk.
type Store struct {
	mu     sync.Mutex
	path   string
	events []Event
	loaded bool
}

// NewStore creates an analytics store in the given directory.
func NewStore(dir string) *Store {
	return &Store{
		path: filepath.Join(dir, analyticsFileName),
	}
}

// Load reads persisted events from disk.
func (s *Store) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadLocked()
}

func (s *Store) loadLocked() error {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			s.events = nil
			s.loaded = true
			return nil
		}
		return fmt.Errorf("read analytics: %w", err)
	}
	var events []Event
	if len(data) == 0 || string(data) == "null" {
		s.events = nil
		s.loaded = true
		return nil
	}
	if err := json.Unmarshal(data, &events); err != nil {
		return fmt.Errorf("parse analytics: %w", err)
	}
	s.events = events
	s.loaded = true
	return nil
}

// ensureLoaded loads from disk if this process has not yet.
func (s *Store) ensureLoaded() error {
	if s.loaded {
		return nil
	}
	return s.loadLocked()
}

// Record adds an event and persists atomically. Always merges with on-disk history.
func (s *Store) Record(kind, target, detail string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Re-read disk so concurrent CLI processes do not wipe each other.
	if err := s.loadLocked(); err != nil {
		return err
	}

	s.events = append(s.events, Event{
		Timestamp: time.Now(),
		Kind:      kind,
		Target:    target,
		Detail:    detail,
	})

	if len(s.events) > 1000 {
		s.events = s.events[len(s.events)-1000:]
	}

	data, err := json.MarshalIndent(s.events, "", "  ")
	if err != nil {
		return err
	}

	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("analytics dir: %w", err)
	}

	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write analytics tmp: %w", err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("commit analytics: %w", err)
	}
	return nil
}

// Summarize returns aggregate analytics.
func (s *Store) Summarize() Summary {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = s.ensureLoaded()

	summary := Summary{
		SkillActivations: make(map[string]int),
		ProfileSwitches:  make(map[string]int),
		Note:             "Minerva-local events only (not local-agent session usage or mcphub tool_calls)",
	}

	for _, e := range s.events {
		summary.TotalEvents++
		switch e.Kind {
		case "skill_activate":
			summary.SkillActivations[e.Target]++
		case "profile_switch", "profile_create":
			summary.ProfileSwitches[e.Target]++
		case "suggestion_applied", "suggest_apply":
			summary.SuggestionsApplied++
		}
		if e.Timestamp.After(summary.LastActivity) {
			summary.LastActivity = e.Timestamp
		}
	}

	summary.TopSkills = topN(summary.SkillActivations, 5)
	summary.TopProfiles = topN(summary.ProfileSwitches, 5)
	return summary
}

// Summary is an alias for Summarize.
func (s *Store) Summary() Summary {
	return s.Summarize()
}

// TopSkills returns the top N skills by activation count.
func (s *Store) TopSkills(n int) []TopSkillEntry {
	summary := s.Summarize()
	entries := make([]TopSkillEntry, 0, n)
	for _, name := range summary.TopSkills {
		if len(entries) >= n {
			break
		}
		entries = append(entries, TopSkillEntry{Name: name, Count: summary.SkillActivations[name]})
	}
	return entries
}

func topN(counts map[string]int, n int) []string {
	type entry struct {
		name  string
		count int
	}
	entries := make([]entry, 0, len(counts))
	for name, count := range counts {
		entries = append(entries, entry{name, count})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].count == entries[j].count {
			return entries[i].name < entries[j].name
		}
		return entries[i].count > entries[j].count
	})
	result := make([]string, 0, n)
	for i, e := range entries {
		if i >= n {
			break
		}
		result = append(result, e.name)
	}
	return result
}
