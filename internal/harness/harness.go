// Package harness catalogs agent runtimes and their on-disk skill/session roots.
package harness

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// IDs for known harnesses.
const (
	Claude   = "claude"
	Codex    = "codex"
	Cursor   = "cursor"
	OpenCode = "opencode"
	Copilot  = "copilot"
	Gemini   = "gemini"
	Sonar    = "sonar"
	Hermes   = "hermes"
	OMP      = "omp"
)

// Env is the filesystem context Minerva uses to find harness data.
type Env struct {
	Home      string
	AgentsDir string
	Now       time.Time
}

// DefaultEnv reads MINERVA_HOME / MINERVA_AGENTS_DIR / $HOME.
func DefaultEnv() Env {
	home, _ := os.UserHomeDir()
	if h := os.Getenv("MINERVA_HOME"); h != "" {
		home = h
	}
	agents := os.Getenv("MINERVA_AGENTS_DIR")
	if agents == "" && home != "" {
		agents = filepath.Join(home, ".agents")
	}
	return Env{Home: home, AgentsDir: agents, Now: time.Now()}
}

// Harness describes one agent runtime.
type Harness struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
	SkillsDir   string `json:"skills_dir"`
	// SessionGlobs are path globs relative to Home, or absolute.
	SessionGlobs []string `json:"session_globs,omitempty"`
	// LinkSkills is true when Minerva may symlink ~/.agents/skills into this dir.
	LinkSkills bool `json:"link_skills"`
	// SyncWritable is false for harness-owned skill trees (do not overwrite).
	SyncWritable bool `json:"sync_writable"`
	// Native marks a harness that reads the canonical ~/.agents/skills tree
	// directly: there is nothing to sync into it and nothing to diff.
	Native bool `json:"native,omitempty"`
	// PresenceDirs, when set, replaces the SkillsDir check in presence
	// detection. Native harnesses need it: their SkillsDir is the shared
	// canonical tree, which exists whether or not the harness is installed.
	PresenceDirs []string `json:"presence_dirs,omitempty"`
	Present      bool     `json:"present"`
}

// Catalog returns every known harness with paths resolved against env.
func Catalog(env Env) []Harness {
	home := env.Home
	items := []Harness{
		{
			ID: Claude, DisplayName: "Claude Code",
			SkillsDir:    filepath.Join(home, ".claude", "skills"),
			SessionGlobs: []string{filepath.Join(home, ".claude", "projects", "*", "*.jsonl")},
			LinkSkills:   true, SyncWritable: true,
		},
		{
			ID: Codex, DisplayName: "Codex",
			SkillsDir:    filepath.Join(home, ".codex", "skills"),
			SessionGlobs: []string{filepath.Join(home, ".codex", "sessions", "*", "*", "*", "*.jsonl")},
			LinkSkills:   true, SyncWritable: true,
		},
		{
			ID: Cursor, DisplayName: "Cursor",
			SkillsDir: filepath.Join(home, ".cursor", "skills-cursor"),
			SessionGlobs: []string{
				filepath.Join(home, ".cursor", "projects", "*", "agent-transcripts", "*", "*.jsonl"),
			},
			LinkSkills: false, SyncWritable: false,
		},
		{
			ID: OpenCode, DisplayName: "OpenCode",
			SkillsDir:    filepath.Join(home, ".config", "opencode", "skills"),
			SessionGlobs: []string{filepath.Join(home, ".local", "share", "opencode", "storage", "session", "*", "*.json")},
			LinkSkills:   true, SyncWritable: true,
		},
		{
			ID: Copilot, DisplayName: "GitHub Copilot",
			SkillsDir:    filepath.Join(home, ".copilot", "skills"),
			SessionGlobs: []string{filepath.Join(home, ".copilot", "session-state", "*")},
			LinkSkills:   true, SyncWritable: true,
		},
		{
			ID: Gemini, DisplayName: "Gemini CLI / Antigravity",
			SkillsDir: filepath.Join(home, ".gemini", "skills"),
			SessionGlobs: []string{
				filepath.Join(home, ".gemini", "tmp", "*", "chats", "*.jsonl"),
				filepath.Join(home, ".gemini", "tmp", "*", "chats", "*.json"),
				filepath.Join(home, ".gemini", "antigravity", "conversations", "*"),
			},
			LinkSkills: true, SyncWritable: true,
		},
		{
			ID: Sonar, DisplayName: "sonar",
			SkillsDir:    filepath.Join(env.AgentsDir, "skills"),
			SessionGlobs: []string{filepath.Join(home, ".sonar", "logs", "*"), filepath.Join(home, ".sonar", "sonar.db")},
			LinkSkills:   false, SyncWritable: false,
			Native: true, PresenceDirs: []string{filepath.Join(home, ".sonar")},
		},
		{
			// Hermes walks its skills dir with followlinks=True, so symlinks
			// work. The dir also holds Hermes-owned dot-entries and _shared;
			// sync and doctor ignore those (see internal/sync listSkillNames).
			ID: Hermes, DisplayName: "Hermes Agent",
			SkillsDir:    filepath.Join(home, ".hermes", "skills"),
			SessionGlobs: []string{filepath.Join(home, ".hermes", "sessions", "*.jsonl")},
			LinkSkills:   true, SyncWritable: true,
		},
		{
			// omp loads ~/.agents/skills natively; a separate dir would only
			// duplicate every skill.
			ID: OMP, DisplayName: "oh-my-pi (omp)",
			SkillsDir:    filepath.Join(env.AgentsDir, "skills"),
			SessionGlobs: []string{filepath.Join(home, ".omp", "agent", "sessions", "*", "*.jsonl")},
			LinkSkills:   false, SyncWritable: false,
			Native: true, PresenceDirs: []string{filepath.Join(home, ".omp", "agent")},
		},
	}
	for i := range items {
		h := &items[i]
		present := anyGlobExists(h.SessionGlobs)
		if len(h.PresenceDirs) > 0 {
			for _, d := range h.PresenceDirs {
				present = present || dirExists(d)
			}
		} else {
			present = present || dirExists(h.SkillsDir)
		}
		h.Present = present
	}
	return items
}

// Get returns a harness by id, or nil.
func Get(env Env, id string) *Harness {
	id = strings.ToLower(strings.TrimSpace(id))
	for _, h := range Catalog(env) {
		if h.ID == id {
			copy := h
			return &copy
		}
	}
	return nil
}

// IDs returns catalog ids in stable order.
func IDs(env Env) []string {
	cat := Catalog(env)
	out := make([]string, len(cat))
	for i, h := range cat {
		out[i] = h.ID
	}
	sort.Strings(out)
	return out
}

func dirExists(path string) bool {
	if path == "" {
		return false
	}
	st, err := os.Stat(path)
	return err == nil && st.IsDir()
}

func anyGlobExists(globs []string) bool {
	for _, g := range globs {
		matches, _ := filepath.Glob(g)
		if len(matches) > 0 {
			return true
		}
	}
	return false
}
