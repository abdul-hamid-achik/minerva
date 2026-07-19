// Package profile manages agent profiles and system prompts in the
// ~/.agents/agents directory. Each profile defines a model, skills,
// MCP server allowlist, and a custom system prompt.
package profile

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

// Profile represents an agent profile with its configuration.
type Profile struct {
	Name         string   `yaml:"name" json:"name"`
	Description  string   `yaml:"description" json:"description"`
	Model        string   `yaml:"model" json:"model"`
	Skills       []string `yaml:"skills" json:"skills"`
	MCPServers   []string `yaml:"mcp_servers" json:"mcp_servers"`
	SystemPrompt string   `yaml:"system_prompt" json:"system_prompt"`
	UseCases     []string `yaml:"use_cases" json:"use_cases"`
	Path         string   `yaml:"-" json:"path"`
}

// LoadWarning is a non-fatal issue discovered while loading profiles.
type LoadWarning struct {
	Dir     string `json:"dir"`
	Path    string `json:"path,omitempty"`
	Message string `json:"message"`
}

// Manager handles agent profile discovery and management.
type Manager struct {
	mu       sync.RWMutex
	profiles map[string]*Profile
	warnings []LoadWarning
	dir      string
}

// NewManager creates a profile manager for the given agents directory.
func NewManager(dir string) *Manager {
	return &Manager{
		profiles: make(map[string]*Profile),
		dir:      dir,
	}
}

// LoadAll discovers and loads all agent profiles from the agents directory.
// Unreadable or invalid profiles are skipped and recorded via Warnings().
func (m *Manager) LoadAll() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.profiles = make(map[string]*Profile)
	m.warnings = nil

	agentsDir := filepath.Join(m.dir, "agents")
	entries, err := os.ReadDir(agentsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read agents dir: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		var profilePath string
		var data []byte
		for _, name := range []string{"agent.yaml", "agent.md"} {
			candidate := filepath.Join(agentsDir, entry.Name(), name)
			candidateData, readErr := os.ReadFile(candidate)
			if readErr == nil {
				profilePath = candidate
				data = candidateData
				break
			}
			if !os.IsNotExist(readErr) {
				m.warnings = append(m.warnings, LoadWarning{
					Dir:     entry.Name(),
					Path:    candidate,
					Message: fmt.Sprintf("read error: %v", readErr),
				})
				return fmt.Errorf("read agent profile %s: %w", candidate, readErr)
			}
		}
		if profilePath == "" {
			// Directory without agent.yaml/agent.md — skip silently (may be WIP).
			continue
		}

		var profile Profile
		if err := yaml.Unmarshal(data, &profile); err != nil {
			m.warnings = append(m.warnings, LoadWarning{
				Dir:     entry.Name(),
				Path:    profilePath,
				Message: fmt.Sprintf("invalid YAML: %v", err),
			})
			continue
		}

		if profile.Name == "" {
			profile.Name = entry.Name()
		}
		profile.Path = profilePath

		m.profiles[profile.Name] = &profile
	}

	return nil
}

// Warnings returns non-fatal load issues (e.g. unreadable YAML).
func (m *Manager) Warnings() []LoadWarning {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if len(m.warnings) == 0 {
		return nil
	}
	out := make([]LoadWarning, len(m.warnings))
	copy(out, m.warnings)
	return out
}

// All returns all discovered profiles sorted by name.
func (m *Manager) All() []*Profile {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*Profile, 0, len(m.profiles))
	for _, p := range m.profiles {
		result = append(result, p)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})
	return result
}

// Get returns a profile by name.
func (m *Manager) Get(name string) *Profile {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.profiles[name]
}

// Has reports whether a profile exists.
func (m *Manager) Has(name string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.profiles[name]
	return ok
}

// Create creates a new agent profile.
func (m *Manager) Create(profile *Profile) error {
	if profile.Name == "" {
		return fmt.Errorf("profile name is required")
	}
	if m.Has(profile.Name) {
		return fmt.Errorf("profile %q already exists", profile.Name)
	}

	profileDir := filepath.Join(m.dir, "agents", profile.Name)
	if err := os.MkdirAll(profileDir, 0o755); err != nil {
		return fmt.Errorf("create profile directory: %w", err)
	}

	data, err := yaml.Marshal(profile)
	if err != nil {
		return fmt.Errorf("marshal profile: %w", err)
	}

	profilePath := filepath.Join(profileDir, "agent.yaml")
	if err := os.WriteFile(profilePath, data, 0o644); err != nil {
		return fmt.Errorf("write profile: %w", err)
	}

	return m.LoadAll()
}

// Delete removes an agent profile.
func (m *Manager) Delete(name string) error {
	if !m.Has(name) {
		return fmt.Errorf("profile %q not found", name)
	}

	profileDir := filepath.Join(m.dir, "agents", name)
	if err := os.RemoveAll(profileDir); err != nil {
		return fmt.Errorf("delete profile directory: %w", err)
	}

	return m.LoadAll()
}

// UpdateSystemPrompt updates the system prompt for a profile.
func (m *Manager) UpdateSystemPrompt(name, prompt string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	profile, ok := m.profiles[name]
	if !ok {
		return fmt.Errorf("profile %q not found", name)
	}

	profile.SystemPrompt = prompt
	return m.writeProfileLocked(profile)
}

// UpdateSkills replaces the skills list for a profile.
func (m *Manager) UpdateSkills(name string, skills []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	profile, ok := m.profiles[name]
	if !ok {
		return fmt.Errorf("profile %q not found", name)
	}

	profile.Skills = skills
	return m.writeProfileLocked(profile)
}

// AddSkills merges skill names into a profile without dropping existing ones.
func (m *Manager) AddSkills(name string, add []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	profile, ok := m.profiles[name]
	if !ok {
		return fmt.Errorf("profile %q not found", name)
	}

	profile.Skills = mergeUnique(profile.Skills, add)
	return m.writeProfileLocked(profile)
}

// RemoveSkills drops skill names from a profile (no-op for missing names).
func (m *Manager) RemoveSkills(name string, remove []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	profile, ok := m.profiles[name]
	if !ok {
		return fmt.Errorf("profile %q not found", name)
	}

	drop := make(map[string]bool, len(remove))
	for _, s := range remove {
		s = strings.TrimSpace(s)
		if s != "" {
			drop[s] = true
		}
	}
	kept := make([]string, 0, len(profile.Skills))
	for _, s := range profile.Skills {
		s = strings.TrimSpace(s)
		if s == "" || drop[s] {
			continue
		}
		kept = append(kept, s)
	}
	profile.Skills = kept
	return m.writeProfileLocked(profile)
}

// UpdateModel sets the model field for a profile.
func (m *Manager) UpdateModel(name, model string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	profile, ok := m.profiles[name]
	if !ok {
		return fmt.Errorf("profile %q not found", name)
	}
	profile.Model = model
	return m.writeProfileLocked(profile)
}

// UpdateMCPServers replaces the MCP server allowlist for a profile.
func (m *Manager) UpdateMCPServers(name string, servers []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	profile, ok := m.profiles[name]
	if !ok {
		return fmt.Errorf("profile %q not found", name)
	}
	cleaned := make([]string, 0, len(servers))
	seen := make(map[string]bool, len(servers))
	for _, s := range servers {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		cleaned = append(cleaned, s)
	}
	profile.MCPServers = cleaned
	return m.writeProfileLocked(profile)
}

// UpdateDescription sets the description for a profile.
func (m *Manager) UpdateDescription(name, description string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	profile, ok := m.profiles[name]
	if !ok {
		return fmt.Errorf("profile %q not found", name)
	}
	profile.Description = description
	return m.writeProfileLocked(profile)
}

// SystemPromptContent returns the system prompt for a profile, or empty string.
func (m *Manager) SystemPromptContent(name string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	profile, ok := m.profiles[name]
	if !ok {
		return ""
	}
	return strings.TrimSpace(profile.SystemPrompt)
}

// writeProfileLocked marshals and writes a profile. Caller must hold m.mu.
func (m *Manager) writeProfileLocked(profile *Profile) error {
	data, err := yaml.Marshal(profile)
	if err != nil {
		return fmt.Errorf("marshal profile: %w", err)
	}
	if err := os.WriteFile(profile.Path, data, 0o644); err != nil {
		return fmt.Errorf("write profile: %w", err)
	}
	return nil
}

func mergeUnique(existing, add []string) []string {
	seen := make(map[string]bool, len(existing)+len(add))
	merged := make([]string, 0, len(existing)+len(add))
	for _, s := range existing {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		merged = append(merged, s)
	}
	for _, s := range add {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		merged = append(merged, s)
	}
	return merged
}
