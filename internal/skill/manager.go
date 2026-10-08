// Package skill manages Agent Skills (SKILL.md) under ~/.agents/skills.
package skill

import (
	"bufio"
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
	MaxSkillDescriptionBytes = 4096
	LintDescriptionWarnBytes = 2048
)

// Skill is a loadable skill definition.
type Skill struct {
	Name        string `yaml:"name" json:"name"`
	Description string `yaml:"description" json:"description"`
	Content     string `yaml:"-" json:"-"`
	Path        string `yaml:"-" json:"path"`
}

// CatalogEntry is the bounded, model-safe projection of a discovered skill.
type CatalogEntry struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// Manager handles skill discovery, loading, and CRUD.
type Manager struct {
	mu       sync.RWMutex
	skills   []*Skill
	problems []error
	dirs     []string
}

// NewManager creates a skill manager for explicit search directories.
func NewManager(dirs ...string) *Manager {
	return &Manager{dirs: dirs}
}

// ForAgents discovers skills under agentsDir/skills.
func ForAgents(agentsDir string) *Manager {
	return NewManager(filepath.Join(agentsDir, "skills"))
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

// LoadAll discovers and loads all skill files from the search directories.
// A skill that cannot be read or parsed, has an invalid name, or repeats a
// name already loaded is skipped and recorded in Problems, so one bad
// SKILL.md never hides the rest of the library. Only a search dir that
// cannot be used is an error.
func (m *Manager) LoadAll() error {
	m.mu.RLock()
	dirs := append([]string(nil), m.dirs...)
	m.mu.RUnlock()

	discovered := make([]*Skill, 0)
	var problems []error
	byName := make(map[string]string)
	for _, dir := range dirs {
		sk, probs, err := loadSkillsFromDir(dir)
		if err != nil {
			return err
		}
		problems = append(problems, probs...)
		for _, candidate := range sk {
			if previous, duplicate := byName[candidate.Name]; duplicate {
				problems = append(problems, fmt.Errorf("duplicate skill name %q: %s is skipped, %s is used", candidate.Name, candidate.Path, previous))
				continue
			}
			byName[candidate.Name] = candidate.Path
			discovered = append(discovered, candidate)
		}
	}
	sort.Slice(discovered, func(i, j int) bool {
		return discovered[i].Name < discovered[j].Name
	})

	m.mu.Lock()
	m.skills = discovered
	m.problems = problems
	m.mu.Unlock()
	return nil
}

// Problems lists the skills the last LoadAll skipped, and why.
func (m *Manager) Problems() []error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append([]error(nil), m.problems...)
}

func loadSkillsFromDir(dir string) (loaded []*Skill, problems []error, err error) {
	if dir == "" {
		return nil, nil, nil
	}

	dirInfo, err := os.Lstat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, nil
		}
		return nil, nil, fmt.Errorf("inspect skills dir: %w", err)
	}
	if dirInfo.Mode()&os.ModeSymlink != 0 {
		return nil, nil, fmt.Errorf("skills dir is a symlink: %s", dir)
	}
	if !dirInfo.IsDir() {
		return nil, nil, fmt.Errorf("skills path is not a directory: %s", dir)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, nil, fmt.Errorf("read skills dir: %w", err)
	}

	loaded = make([]*Skill, 0)
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		skill, err := loadSkillEntry(dir, entry)
		if err != nil {
			problems = append(problems, err)
			continue
		}
		if skill != nil {
			loaded = append(loaded, skill)
		}
	}

	return loaded, problems, nil
}

// loadSkillEntry loads the skill an entry of a skills dir holds. It returns
// nil, nil for entries that are not skills (other files, symlinks, folders
// without a SKILL.md).
func loadSkillEntry(dir string, entry os.DirEntry) (*Skill, error) {
	entryInfo, err := entry.Info()
	if err != nil {
		return nil, fmt.Errorf("inspect skill entry %s: %w", filepath.Join(dir, entry.Name()), err)
	}
	if entryInfo.Mode()&os.ModeSymlink != 0 {
		return nil, nil
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
		return nil, nil
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
		return nil, nil
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
	return skill, nil
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
	case name == "." || name == "..":
		return fmt.Errorf("name %q is not allowed", name)
	case strings.HasPrefix(name, "."):
		return errors.New("name starts with a dot")
	case strings.ContainsAny(name, `/\`):
		return errors.New("name contains a path separator")
	}
	for _, r := range name {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return fmt.Errorf("name contains disallowed Unicode character %U", r)
		}
	}
	return nil
}

// ValidateName reports whether name is usable as a skill name. A valid name
// is a single path element, so joining it to a directory can never address
// anything outside that directory.
func ValidateName(name string) error {
	return validateSkillName(name)
}

// IsFlat reports whether s was loaded from a bare <name>.md file in the
// library root rather than from <folder>/SKILL.md.
func IsFlat(s *Skill) bool {
	return !strings.EqualFold(filepath.Base(s.Path), "SKILL.md")
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

// Get returns a skill by name.
func (m *Manager) Get(name string) *Skill {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, s := range m.skills {
		if s.Name == name {
			copy := *s
			return &copy
		}
	}
	return nil
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

// Create creates a new skill file in the given directory.
func (m *Manager) Create(dir, name, description, content string) error {
	if err := validateSkillName(name); err != nil {
		return fmt.Errorf("invalid skill name: %w", err)
	}
	if m.Has(name) {
		return fmt.Errorf("skill %q already exists", name)
	}

	skillDir := filepath.Join(dir, name)
	// Has only knows frontmatter names: a folder whose SKILL.md declares a
	// different name, or one differing only in case on a case-insensitive
	// filesystem, must not be overwritten either.
	if _, err := os.Lstat(skillDir); err == nil {
		return fmt.Errorf("skill directory %s already exists", skillDir)
	}
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
	if err := rewriteSkillFile(path, name, description, body); err != nil {
		return err
	}
	return m.LoadAll()
}

// rewriteSkillFile replaces the body of an existing SKILL.md and, when
// description is set, its description. Every other frontmatter key
// (allowed-tools, license, metadata, …) and its comments are kept. A file
// without frontmatter gets a fresh one.
func rewriteSkillFile(path, name string, description *string, body string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read skill file: %w", err)
	}
	fm, ok := splitFrontmatter(string(raw))
	if !ok {
		desc := ""
		if description != nil {
			desc = *description
		}
		return writeSkillFile(path, name, desc, body)
	}
	if description != nil {
		if fm, err = setFrontmatterKey(fm, "description", *description); err != nil {
			return fmt.Errorf("update frontmatter: %w", err)
		}
	}
	return writeSkill(path, fm, body)
}

func writeSkillFile(path, name, description, content string) error {
	var b strings.Builder
	fmt.Fprintf(&b, "name: %q\n", name)
	if description != "" {
		fmt.Fprintf(&b, "description: %q\n", description)
	}
	return writeSkill(path, b.String(), content)
}

// splitFrontmatter returns the YAML between a leading "---" line and the
// next "---" line, or false when the file has no complete frontmatter.
func splitFrontmatter(data string) (string, bool) {
	lines := strings.SplitAfter(data, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return "", false
	}
	var fm strings.Builder
	for _, line := range lines[1:] {
		if strings.TrimSpace(line) == "---" {
			return fm.String(), true
		}
		fm.WriteString(line)
	}
	return "", false
}

// setFrontmatterKey sets key to a string value in frontmatter YAML, keeping
// every other key, their order and comments. An empty value removes key.
func setFrontmatterKey(fm, key, value string) (string, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(fm), &doc); err != nil {
		return "", err
	}
	if doc.Kind == 0 {
		doc = yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{{Kind: yaml.MappingNode, Tag: "!!map"}}}
	}
	m := doc.Content[0]
	if m.Kind != yaml.MappingNode {
		return "", fmt.Errorf("frontmatter is not a mapping")
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value != key {
			continue
		}
		if value == "" {
			m.Content = append(m.Content[:i], m.Content[i+2:]...)
		} else {
			m.Content[i+1] = &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value, Style: yaml.DoubleQuotedStyle}
		}
		return encodeYAML(&doc)
	}
	if value != "" {
		m.Content = append(m.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value, Style: yaml.DoubleQuotedStyle})
	}
	return encodeYAML(&doc)
}

func encodeYAML(doc *yaml.Node) (string, error) {
	var b strings.Builder
	enc := yaml.NewEncoder(&b)
	enc.SetIndent(2)
	if err := enc.Encode(doc); err != nil {
		return "", err
	}
	if err := enc.Close(); err != nil {
		return "", err
	}
	return b.String(), nil
}

// WriteFileAtomic writes data to a temp file next to path and renames it
// over path, so a reader never sees a half-written file and a symlink at
// path is replaced rather than written through.
func WriteFileAtomic(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-"+filepath.Base(path)+"-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // no-op once renamed
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// writeSkill writes "---", the frontmatter (YAML ending in a newline), "---",
// a blank line and the body.
func writeSkill(path, frontmatter, body string) error {
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString(frontmatter)
	b.WriteString("---\n\n")
	b.WriteString(body)
	if !strings.HasSuffix(body, "\n") {
		b.WriteString("\n")
	}
	if err := WriteFileAtomic(path, []byte(b.String())); err != nil {
		return fmt.Errorf("write skill file: %w", err)
	}
	return nil
}

// WriteSkillFile writes a SKILL.md with quoted frontmatter (exported for propose/sync).
func WriteSkillFile(path, name, description, content string) error {
	return writeSkillFile(path, name, description, content)
}

// Delete removes a skill from dir: its folder, or the file of a flat
// <name>.md skill. The location comes from where the skill was loaded, not
// from its name, so a frontmatter name that differs from the folder still
// deletes the right thing, and nothing outside dir is ever removed.
func (m *Manager) Delete(dir, name string) error {
	s := m.Get(name)
	if s == nil {
		return fmt.Errorf("skill %q not found", name)
	}
	root := filepath.Clean(dir)
	target := filepath.Dir(s.Path)
	if IsFlat(s) {
		target = s.Path
	}
	if filepath.Dir(target) != root {
		return fmt.Errorf("skill %q is not in %s", name, root)
	}
	if err := os.RemoveAll(target); err != nil {
		return fmt.Errorf("delete skill: %w", err)
	}

	return m.LoadAll()
}
