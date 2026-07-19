// Package skill manages agent skills in the ~/.agents/skills directory.
// Skills are markdown files with YAML frontmatter that provide specialized
// instructions to coding agents. Minerva can create, list, activate,
// deactivate, and load skills.
package skill

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

const (
	MaxSkillBodyBytes        = 1 << 20 // 1 MiB
	MaxSkillNameBytes        = 128
	MaxSkillDescriptionBytes = 512
	stateFileName            = ".minerva-skills.json"
)

// Skill represents a loadable skill definition.
type Skill struct {
	Name        string `yaml:"name" json:"name"`
	Description string `yaml:"description" json:"description"`
	Active      bool   `yaml:"-" json:"active"`
	Content     string `yaml:"-" json:"-"`
	Path        string `yaml:"-" json:"path"`
}

// CatalogEntry is the bounded, model-safe projection of a discovered skill.
type CatalogEntry struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Active      bool   `json:"active"`
}

// Manager handles skill discovery, loading, and activation.
type Manager struct {
	mu        sync.RWMutex
	skills    []*Skill
	dirs      []string
	statePath string
}

// NewManager creates a skill manager for explicit search directories.
func NewManager(dirs ...string) *Manager {
	return &Manager{dirs: dirs}
}

// NewManagerWithState creates a skill manager that persists activation state.
func NewManagerWithState(stateDir string, dirs ...string) *Manager {
	return &Manager{
		dirs:      dirs,
		statePath: filepath.Join(stateDir, stateFileName),
	}
}

// AddSearchPath adds a directory to search for skills.
func (m *Manager) AddSearchPath(dir string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, d := range m.dirs {
		if d == dir {
			return
		}
	}
	m.dirs = append(m.dirs, dir)
}

// LoadAll discovers and loads all .md skill files from the skills directories.
func (m *Manager) LoadAll() error {
	m.mu.RLock()
	dirs := append([]string(nil), m.dirs...)
	m.mu.RUnlock()

	discovered := make([]*Skill, 0)
	byName := make(map[string]string)
	for _, dir := range dirs {
		sk, err := loadSkillsFromDir(dir)
		if err != nil {
			return err
		}
		for _, candidate := range sk {
			if previous, duplicate := byName[candidate.Name]; duplicate {
				return fmt.Errorf("duplicate skill name %q: %s and %s", candidate.Name, previous, candidate.Path)
			}
			byName[candidate.Name] = candidate.Path
			discovered = append(discovered, candidate)
		}
	}
	sort.Slice(discovered, func(i, j int) bool {
		return discovered[i].Name < discovered[j].Name
	})

	// Load persisted activation state.
	activeSet := m.loadState()

	m.mu.Lock()
	for _, candidate := range discovered {
		candidate.Active = activeSet[candidate.Name]
	}
	m.skills = discovered
	m.mu.Unlock()
	return nil
}

func (m *Manager) loadState() map[string]bool {
	if m.statePath == "" {
		return nil
	}
	data, err := os.ReadFile(m.statePath)
	if err != nil {
		return nil
	}
	var active []string
	if err := json.Unmarshal(data, &active); err != nil {
		return nil
	}
	set := make(map[string]bool, len(active))
	for _, name := range active {
		set[name] = true
	}
	return set
}

func (m *Manager) saveState() {
	if m.statePath == "" {
		return
	}
	active := make([]string, 0)
	for _, s := range m.skills {
		if s.Active {
			active = append(active, s.Name)
		}
	}
	sort.Strings(active)
	data, err := json.Marshal(active) // empty slice → [] not null
	if err != nil {
		return
	}
	_ = os.WriteFile(m.statePath, data, 0o644)
}

func loadSkillsFromDir(dir string) ([]*Skill, error) {
	if dir == "" {
		return nil, nil
	}

	dirInfo, err := os.Lstat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("inspect skills dir: %w", err)
	}
	if dirInfo.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("skills dir is a symlink: %s", dir)
	}
	if !dirInfo.IsDir() {
		return nil, fmt.Errorf("skills path is not a directory: %s", dir)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read skills dir: %w", err)
	}

	loaded := make([]*Skill, 0)
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		entryInfo, err := entry.Info()
		if err != nil {
			return nil, fmt.Errorf("inspect skill entry %s: %w", filepath.Join(dir, entry.Name()), err)
		}
		if entryInfo.Mode()&os.ModeSymlink != 0 {
			continue
		}

		var candidatePaths []string
		var fallbackName string
		switch {
		case entryInfo.IsDir():
			candidatePaths = []string{
				filepath.Join(dir, entry.Name(), "SKILL.md"),
				filepath.Join(dir, entry.Name(), "skill.md"),
			}
			fallbackName = entry.Name()
		case strings.HasSuffix(entry.Name(), ".md"):
			candidatePaths = []string{filepath.Join(dir, entry.Name())}
			fallbackName = strings.TrimSuffix(entry.Name(), ".md")
		default:
			continue
		}

		var data []byte
		path := ""
		for _, candidatePath := range candidatePaths {
			data, err = os.ReadFile(candidatePath)
			if err == nil {
				path = candidatePath
				break
			}
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, fmt.Errorf("read skill %s: %w", candidatePath, err)
		}
		if path == "" {
			continue
		}

		if !utf8.Valid(data) {
			return nil, fmt.Errorf("parse skill %s: content is not valid UTF-8", path)
		}
		skill, err := parseFrontmatter(string(data))
		if err != nil {
			return nil, fmt.Errorf("parse skill %s: %w", path, err)
		}

		skill.Path = path
		if skill.Name == "" {
			skill.Name = fallbackName
		}
		if err := validateSkillName(skill.Name); err != nil {
			return nil, fmt.Errorf("parse skill %s: invalid skill name: %w", path, err)
		}
		loaded = append(loaded, skill)
	}

	return loaded, nil
}

func parseFrontmatter(data string) (*Skill, error) {
	scanner := bufio.NewScanner(strings.NewReader(data))
	scanner.Buffer(make([]byte, 64*1024), MaxSkillBodyBytes+1)

	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return nil, err
		}
		return &Skill{Content: data}, nil
	}
	if strings.TrimSpace(scanner.Text()) != "---" {
		return &Skill{Content: data}, nil
	}

	var yamlBuf strings.Builder
	foundEnd := false
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "---" {
			foundEnd = true
			break
		}
		yamlBuf.WriteString(line)
		yamlBuf.WriteString("\n")
	}

	if !foundEnd {
		if err := scanner.Err(); err != nil {
			return nil, err
		}
		return &Skill{Content: data}, nil
	}

	metadata := struct {
		Name        string `yaml:"name"`
		Description string `yaml:"description"`
	}{}
	decoder := yaml.NewDecoder(strings.NewReader(yamlBuf.String()))
	if err := decoder.Decode(&metadata); err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}

	s := &Skill{
		Name:        metadata.Name,
		Description: metadata.Description,
	}

	var bodyBuf strings.Builder
	for scanner.Scan() {
		if bodyBuf.Len() > 0 {
			bodyBuf.WriteString("\n")
		}
		bodyBuf.WriteString(scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	s.Content = strings.TrimSpace(bodyBuf.String())

	return s, nil
}

func validateSkillName(name string) error {
	switch {
	case name == "":
		return errors.New("name is blank")
	case name != strings.TrimSpace(name):
		return errors.New("name has leading or trailing whitespace")
	case !utf8.ValidString(name):
		return errors.New("name is not valid UTF-8")
	case len(name) > MaxSkillNameBytes:
		return fmt.Errorf("name exceeds %d bytes", MaxSkillNameBytes)
	}
	for _, r := range name {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return fmt.Errorf("name contains disallowed Unicode character %U", r)
		}
	}
	return nil
}

// All returns all discovered skills.
func (m *Manager) All() []*Skill {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]*Skill, len(m.skills))
	for i, s := range m.skills {
		copy := *s
		result[i] = &copy
	}
	return result
}

// Catalog returns a deterministic metadata-only snapshot of all skills.
func (m *Manager) Catalog() []CatalogEntry {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]CatalogEntry, 0, len(m.skills))
	for _, s := range m.skills {
		result = append(result, CatalogEntry{
			Name:        s.Name,
			Description: s.Description,
			Active:      s.Active,
		})
	}
	return result
}

// Load returns the already-discovered body for an exact skill name.
func (m *Manager) Load(name string) (string, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, s := range m.skills {
		if s.Name == name {
			return s.Content, true
		}
	}
	return "", false
}

// Has reports whether a skill name is available.
func (m *Manager) Has(name string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, s := range m.skills {
		if s.Name == name {
			return true
		}
	}
	return false
}

// IsActive reports whether a skill is currently active.
func (m *Manager) IsActive(name string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, s := range m.skills {
		if s.Name == name {
			return s.Active
		}
	}
	return false
}

// Activate enables a skill by name and persists the state.
func (m *Manager) Activate(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, s := range m.skills {
		if s.Name == name {
			s.Active = true
			m.saveState()
			return nil
		}
	}
	return fmt.Errorf("skill not found: %s", name)
}

// Deactivate disables a skill by name and persists the state.
func (m *Manager) Deactivate(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, s := range m.skills {
		if s.Name == name {
			s.Active = false
			m.saveState()
			return nil
		}
	}
	return fmt.Errorf("skill not found: %s", name)
}

// ActiveContent returns the combined markdown content of all active skills.
func (m *Manager) ActiveContent() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var parts []string
	for _, s := range m.skills {
		if s.Active && s.Content != "" {
			parts = append(parts, fmt.Sprintf("### %s\n%s", s.Name, s.Content))
		}
	}
	return strings.Join(parts, "\n\n")
}

// Create creates a new skill file in the given directory.
func (m *Manager) Create(dir, name, description, content string) error {
	if err := validateSkillName(name); err != nil {
		return fmt.Errorf("invalid skill name: %w", err)
	}
	if m.Has(name) {
		return fmt.Errorf("skill %q already exists", name)
	}

	skillDir := filepath.Join(dir, name)
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		return fmt.Errorf("create skill directory: %w", err)
	}

	skillPath := filepath.Join(skillDir, "SKILL.md")
	if err := writeSkillFile(skillPath, name, description, content); err != nil {
		return err
	}

	return m.LoadAll()
}

// Update rewrites an existing skill's description and/or body.
// Empty description or content arguments keep the current value when
// updateDescription/updateContent are false.
func (m *Manager) Update(name string, description *string, content *string) error {
	if err := validateSkillName(name); err != nil {
		return fmt.Errorf("invalid skill name: %w", err)
	}

	m.mu.RLock()
	var existing *Skill
	for _, s := range m.skills {
		if s.Name == name {
			copy := *s
			existing = &copy
			break
		}
	}
	m.mu.RUnlock()

	if existing == nil {
		return fmt.Errorf("skill %q not found", name)
	}
	if description == nil && content == nil {
		return fmt.Errorf("nothing to update: provide description and/or content")
	}

	desc := existing.Description
	body := existing.Content
	if description != nil {
		desc = *description
		if len(desc) > MaxSkillDescriptionBytes {
			return fmt.Errorf("description exceeds %d bytes", MaxSkillDescriptionBytes)
		}
	}
	if content != nil {
		body = *content
		if len(body) > MaxSkillBodyBytes {
			return fmt.Errorf("content exceeds %d bytes", MaxSkillBodyBytes)
		}
	}

	path := existing.Path
	if path == "" {
		return fmt.Errorf("skill %q has no path on disk", name)
	}
	if err := writeSkillFile(path, name, desc, body); err != nil {
		return err
	}
	return m.LoadAll()
}

func writeSkillFile(path, name, description, content string) error {
	var b strings.Builder
	b.WriteString("---\n")
	// Quote scalars so descriptions with ":" or newlines cannot break YAML.
	fmt.Fprintf(&b, "name: %q\n", name)
	if description != "" {
		fmt.Fprintf(&b, "description: %q\n", description)
	}
	b.WriteString("---\n\n")
	b.WriteString(content)
	if !strings.HasSuffix(content, "\n") {
		b.WriteString("\n")
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		return fmt.Errorf("write skill file: %w", err)
	}
	return nil
}

// Delete removes a skill directory.
func (m *Manager) Delete(dir, name string) error {
	if !m.Has(name) {
		return fmt.Errorf("skill %q not found", name)
	}

	skillDir := filepath.Join(dir, name)
	if err := os.RemoveAll(skillDir); err != nil {
		return fmt.Errorf("delete skill directory: %w", err)
	}

	return m.LoadAll()
}
