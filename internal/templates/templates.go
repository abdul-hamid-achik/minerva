// Package templates provides role prompt templates: built-in seeds plus
// optional on-disk templates under ~/.agents/templates/<name>/template.yaml.
// Disk templates override builtins with the same name.
package templates

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Template is a pre-built system prompt for a specific agent role.
type Template struct {
	Name        string   `json:"name" yaml:"name"`
	Description string   `json:"description" yaml:"description"`
	Role        string   `json:"role" yaml:"role"`
	Skills      []string `json:"skills" yaml:"skills"`
	Prompt      string   `json:"prompt" yaml:"prompt"`
	// Source is "builtin" or "disk".
	Source string `json:"source,omitempty" yaml:"-"`
	// Path is set for disk-backed templates.
	Path string `json:"path,omitempty" yaml:"-"`
}

// Builtins returns the embedded role templates (source=builtin).
func Builtins() []Template {
	out := []Template{
		{
			Name:        "code-reviewer",
			Description: "Thorough code reviewer focused on correctness, security, and maintainability",
			Role:        "reviewer",
			Skills:      []string{"go-review", "software-architect", "qa-tester"},
			Prompt: `You are a thorough code reviewer. Your job is to find issues before they reach production.

## Review Priorities
1. **Correctness** — logic errors, edge cases, off-by-one, nil/null handling
2. **Security** — injection, auth bypass, secret exposure, input validation
3. **Concurrency** — race conditions, deadlocks, goroutine leaks, missing cancellation
4. **Error handling** — swallowed errors, missing context, unclear messages
5. **Performance** — unnecessary allocations, N+1 queries, blocking operations
6. **Maintainability** — unclear names, missing comments, deep nesting, god objects

## Process
- Read the diff completely before commenting
- Flag critical issues first, then style/naming
- Suggest concrete fixes, not just problems
- Approve only when all critical issues are resolved`,
		},
		{
			Name:        "architect",
			Description: "System architect focused on design decisions, trade-offs, and ADRs",
			Role:        "architect",
			Skills:      []string{"software-architect", "fullstack-dev"},
			Prompt: `You are a system architect. You evaluate design decisions, identify trade-offs, and document architecture.

## Approach
1. **Understand the context** — what problem is being solved, what constraints exist
2. **Evaluate options** — what are the alternatives, what are their trade-offs
3. **Make recommendations** — which option fits best and why
4. **Document decisions** — write clear ADRs with context, decision, consequences

## Principles
- Prefer simplicity over flexibility you don't need yet
- Design for change, but don't over-engineer
- Consider operational concerns: deployment, monitoring, rollback
- Think about the team that will maintain this`,
		},
		{
			Name:        "frontend-builder",
			Description: "Frontend specialist focused on UI implementation, accessibility, and animations",
			Role:        "developer",
			Skills:      []string{"frontend-dev", "ux-designer"},
			Prompt: `You are a frontend specialist. You build polished, accessible, performant user interfaces.

## Priorities
1. **Accessibility** — semantic HTML, ARIA labels, keyboard navigation, screen reader support
2. **Performance** — lazy loading, code splitting, image optimization, bundle analysis
3. **Responsiveness** — mobile-first, touch targets, viewport awareness
4. **Visual quality** — consistent spacing, typography, color contrast, motion design
5. **State management** — loading, empty, error, and edge case states

## Process
- Start with the component's purpose and states
- Build the markup first, then style, then animate
- Test on multiple viewport sizes
- Ensure every interactive element is keyboard-accessible`,
		},
		{
			Name:        "backend-builder",
			Description: "Backend specialist focused on APIs, data modeling, and reliability",
			Role:        "developer",
			Skills:      []string{"fullstack-dev", "software-architect"},
			Prompt: `You are a backend specialist. You build reliable, observable, well-tested APIs and services.

## Priorities
1. **API design** — consistent REST/GraphQL, proper status codes, versioning strategy
2. **Data modeling** — normalized schemas, migration safety, query performance
3. **Error handling** — typed errors, retry policies, circuit breakers, graceful degradation
4. **Observability** — structured logging, metrics, traces, health checks
5. **Testing** — unit, integration, contract, and load tests

## Process
- Define the API contract before implementation
- Model the data before writing queries
- Handle every error path explicitly
- Add observability from the first endpoint`,
		},
		{
			Name:        "qa-engineer",
			Description: "QA specialist focused on test strategy, coverage, and bug prevention",
			Role:        "reviewer",
			Skills:      []string{"qa-tester"},
			Prompt: `You are a QA engineer. You design test strategies, write test cases, and prevent regressions.

## Approach
1. **Risk-based testing** — focus on what would hurt most if it broke
2. **Boundary analysis** — test edges, not just happy paths
3. **Regression prevention** — every bug gets a test before it gets a fix
4. **Coverage quality** — branch coverage over line coverage, meaningful assertions

## Test Design
- Happy path: the primary use case works
- Error paths: every error condition is handled
- Edge cases: empty, null, max, min, concurrent
- Integration: services interact correctly
- Contract: API responses match the schema`,
		},
		{
			Name:        "documentation-writer",
			Description: "Technical writer focused on clear, comprehensive documentation",
			Role:        "writer",
			Skills:      []string{"doc-writer"},
			Prompt: `You are a technical writer. You create clear, comprehensive documentation that helps users succeed.

## Principles
1. **Start with why** — what problem does this solve, who is it for
2. **Show, don't tell** — code examples over prose descriptions
3. **Progressive disclosure** — quick start first, deep dive later
4. **Anticipate questions** — what will confuse a new user
5. **Keep it current** — stale docs are worse than no docs

## Document Types
- README: what, why, quick start, contributing
- API docs: endpoint, parameters, responses, errors, examples
- Guides: step-by-step with expected outputs
- ADRs: context, decision, consequences, alternatives considered`,
		},
	}
	for i := range out {
		out[i].Source = "builtin"
	}
	return out
}

// LoadDir loads templates from <dir>/<name>/template.yaml (or template.yml / agent-style).
func LoadDir(dir string) ([]Template, error) {
	if strings.TrimSpace(dir) == "" {
		return nil, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read templates dir: %w", err)
	}
	var out []Template
	for _, e := range entries {
		if !e.IsDir() {
			// Also allow flat template.yaml files named <name>.yaml
			name := e.Name()
			if strings.HasSuffix(name, ".yaml") || strings.HasSuffix(name, ".yml") {
				path := filepath.Join(dir, name)
				t, err := loadFile(path)
				if err != nil {
					return nil, err
				}
				if t.Name == "" {
					t.Name = strings.TrimSuffix(strings.TrimSuffix(name, ".yaml"), ".yml")
				}
				t.Source = "disk"
				t.Path = path
				out = append(out, *t)
			}
			continue
		}
		for _, fname := range []string{"template.yaml", "template.yml"} {
			path := filepath.Join(dir, e.Name(), fname)
			data, err := os.ReadFile(path)
			if err != nil {
				if os.IsNotExist(err) {
					continue
				}
				return nil, fmt.Errorf("read %s: %w", path, err)
			}
			t, err := parseYAML(data)
			if err != nil {
				return nil, fmt.Errorf("parse %s: %w", path, err)
			}
			if t.Name == "" {
				t.Name = e.Name()
			}
			t.Source = "disk"
			t.Path = path
			out = append(out, *t)
			break
		}
	}
	return out, nil
}

func loadFile(path string) (*Template, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return parseYAML(data)
}

func parseYAML(data []byte) (*Template, error) {
	var t Template
	if err := yaml.Unmarshal(data, &t); err != nil {
		return nil, err
	}
	if strings.TrimSpace(t.Name) == "" && strings.TrimSpace(t.Prompt) == "" {
		return nil, fmt.Errorf("template missing name and prompt")
	}
	return &t, nil
}

// Catalog merges builtins with disk templates. Disk wins on name collision.
// dirs are searched in order; later dirs override earlier ones.
func Catalog(dirs ...string) ([]Template, error) {
	byName := map[string]Template{}
	for _, t := range Builtins() {
		byName[t.Name] = t
	}
	for _, dir := range dirs {
		loaded, err := LoadDir(dir)
		if err != nil {
			return nil, err
		}
		for _, t := range loaded {
			byName[t.Name] = t
		}
	}
	out := make([]Template, 0, len(byName))
	for _, t := range byName {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// All is Catalog with no disk dirs (builtins only). Prefer Catalog for full resolution.
func All() []Template {
	out, _ := Catalog()
	return out
}

// Get returns a template by name from builtins only.
// Prefer GetFrom for disk-aware lookup.
func Get(name string) *Template {
	return GetFrom(name)
}

// GetFrom looks up a template by name (disk dirs optional).
func GetFrom(name string, dirs ...string) *Template {
	all, err := Catalog(dirs...)
	if err != nil {
		return nil
	}
	for i := range all {
		if all[i].Name == name {
			t := all[i]
			return &t
		}
	}
	return nil
}

// Names returns all template names from builtins only.
func Names() []string {
	all := All()
	names := make([]string, len(all))
	for i, t := range all {
		names[i] = t.Name
	}
	return names
}

// NamesFrom returns names including disk templates.
func NamesFrom(dirs ...string) []string {
	all, err := Catalog(dirs...)
	if err != nil {
		return Names()
	}
	names := make([]string, len(all))
	for i, t := range all {
		names[i] = t.Name
	}
	return names
}

// Save writes a template to dir/<name>/template.yaml.
func Save(dir string, t Template) error {
	if strings.TrimSpace(t.Name) == "" {
		return fmt.Errorf("template name is required")
	}
	if strings.TrimSpace(t.Prompt) == "" {
		return fmt.Errorf("template prompt is required")
	}
	pathDir := filepath.Join(dir, t.Name)
	if err := os.MkdirAll(pathDir, 0o755); err != nil {
		return err
	}
	// Persist only file fields.
	payload := struct {
		Name        string   `yaml:"name"`
		Description string   `yaml:"description,omitempty"`
		Role        string   `yaml:"role,omitempty"`
		Skills      []string `yaml:"skills,omitempty"`
		Prompt      string   `yaml:"prompt"`
	}{
		Name: t.Name, Description: t.Description, Role: t.Role,
		Skills: t.Skills, Prompt: t.Prompt,
	}
	data, err := yaml.Marshal(&payload)
	if err != nil {
		return err
	}
	path := filepath.Join(pathDir, "template.yaml")
	return os.WriteFile(path, data, 0o644)
}

// InstallBuiltin copies a builtin template to disk (overwrites).
func InstallBuiltin(dir, name string) (*Template, error) {
	var found *Template
	for _, t := range Builtins() {
		if t.Name == name {
			copy := t
			found = &copy
			break
		}
	}
	if found == nil {
		return nil, fmt.Errorf("builtin template %q not found", name)
	}
	if err := Save(dir, *found); err != nil {
		return nil, err
	}
	found.Source = "disk"
	found.Path = filepath.Join(dir, name, "template.yaml")
	return found, nil
}

// DefaultDir returns <agentsDir>/templates.
func DefaultDir(agentsDir string) string {
	return filepath.Join(agentsDir, "templates")
}
