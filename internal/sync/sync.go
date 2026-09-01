// Package sync installs and reconciles skills across harness directories.
package sync

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/abdul-hamid-achik/minerva/internal/harness"
	"github.com/abdul-hamid-achik/minerva/internal/skill"
)

// LockFile is ~/.agents/.skill-lock.json v3.
type LockFile struct {
	Version int                  `json:"version"`
	Skills  map[string]LockEntry `json:"skills"`
}

// LockEntry matches the vercel-style skill lock schema.
type LockEntry struct {
	Source          string `json:"source"`
	SourceType      string `json:"sourceType"`
	SourceURL       string `json:"sourceUrl"`
	SkillPath       string `json:"skillPath"`
	SkillFolderHash string `json:"skillFolderHash"`
	InstalledAt     string `json:"installedAt"`
	UpdatedAt       string `json:"updatedAt"`
}

func lockPath(agentsDir string) string {
	return filepath.Join(agentsDir, ".skill-lock.json")
}

// LoadLock reads the lock file (empty if missing).
func LoadLock(agentsDir string) (LockFile, error) {
	lf := LockFile{Version: 3, Skills: map[string]LockEntry{}}
	data, err := os.ReadFile(lockPath(agentsDir))
	if err != nil {
		if os.IsNotExist(err) {
			return lf, nil
		}
		return lf, err
	}
	if err := json.Unmarshal(data, &lf); err != nil {
		return lf, err
	}
	if lf.Skills == nil {
		lf.Skills = map[string]LockEntry{}
	}
	return lf, nil
}

func saveLock(agentsDir string, lf LockFile) error {
	lf.Version = 3
	data, err := json.MarshalIndent(lf, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(lockPath(agentsDir), data, 0o644)
}

// Finding is one doctor issue.
type Finding struct {
	Harness string `json:"harness"`
	Skill   string `json:"skill,omitempty"`
	Kind    string `json:"kind"`
	Message string `json:"message"`
}

// Doctor compares ~/.agents/skills with each harness skills dir.
func Doctor(env harness.Env) ([]Finding, error) {
	mgr := skill.ForAgents(env.AgentsDir)
	if err := mgr.LoadAll(); err != nil {
		return nil, err
	}
	canonical := map[string]string{}
	for _, s := range mgr.All() {
		dir := filepath.Dir(s.Path)
		h, _ := FolderHash(dir)
		canonical[s.Name] = h
	}

	var out []Finding
	for _, h := range harness.Catalog(env) {
		if h.ID == harness.Sonar {
			continue
		}
		if !dirExists(h.SkillsDir) {
			if h.Present {
				out = append(out, Finding{Harness: h.ID, Kind: "missing-dir", Message: "skills dir missing: " + h.SkillsDir})
			}
			continue
		}
		names, err := listSkillNames(h.SkillsDir)
		if err != nil {
			out = append(out, Finding{Harness: h.ID, Kind: "error", Message: err.Error()})
			continue
		}
		have := map[string]bool{}
		for _, name := range names {
			have[name] = true
			if _, ok := canonical[name]; !ok {
				out = append(out, Finding{Harness: h.ID, Skill: name, Kind: "extra", Message: "present in harness but not in ~/.agents/skills"})
				continue
			}
			hash, _ := FolderHash(filepath.Join(h.SkillsDir, name))
			if canonical[name] != "" && hash != "" && hash != canonical[name] {
				out = append(out, Finding{Harness: h.ID, Skill: name, Kind: "drift", Message: "folder hash differs from ~/.agents/skills"})
			}
		}
		for name := range canonical {
			if !have[name] && h.SyncWritable {
				out = append(out, Finding{Harness: h.ID, Skill: name, Kind: "missing", Message: "not linked/copied into harness"})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Harness == out[j].Harness {
			return out[i].Skill < out[j].Skill
		}
		return out[i].Harness < out[j].Harness
	})
	return out, nil
}

// SyncOptions controls skill sync.
type SyncOptions struct {
	To     []string
	DryRun bool
	Env    harness.Env
}

// Action is one sync step.
type Action struct {
	Harness string `json:"harness"`
	Skill   string `json:"skill"`
	Method  string `json:"method"` // symlink|copy|skip
	From    string `json:"from"`
	To      string `json:"to"`
	Done    bool   `json:"done"`
}

// Sync links or copies canonical skills into writable harness dirs.
func Sync(opts SyncOptions) ([]Action, error) {
	mgr := skill.ForAgents(opts.Env.AgentsDir)
	if err := mgr.LoadAll(); err != nil {
		return nil, err
	}
	want := map[string]bool{}
	for _, id := range opts.To {
		want[strings.ToLower(id)] = true
	}

	var actions []Action
	for _, h := range harness.Catalog(opts.Env) {
		if !h.SyncWritable {
			continue
		}
		if len(want) > 0 && !want[h.ID] {
			continue
		}
		if err := os.MkdirAll(h.SkillsDir, 0o755); err != nil && !opts.DryRun {
			return nil, err
		}
		for _, s := range mgr.All() {
			src := filepath.Dir(s.Path)
			dst := filepath.Join(h.SkillsDir, s.Name)
			method := "copy"
			if h.LinkSkills {
				method = "symlink"
			}
			act := Action{Harness: h.ID, Skill: s.Name, Method: method, From: src, To: dst}
			if opts.DryRun {
				actions = append(actions, act)
				continue
			}
			if err := place(src, dst, method); err != nil {
				return actions, err
			}
			act.Done = true
			actions = append(actions, act)
		}
	}
	return actions, nil
}

func place(src, dst, method string) error {
	if st, err := os.Lstat(dst); err == nil {
		if st.Mode()&os.ModeSymlink != 0 || method == "symlink" {
			_ = os.RemoveAll(dst)
		} else {
			_ = os.RemoveAll(dst)
		}
	}
	if method == "symlink" {
		abs, err := filepath.Abs(src)
		if err != nil {
			return err
		}
		return os.Symlink(abs, dst)
	}
	return copyDir(src, dst)
}

func copyDir(src, dst string) error {
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		return copyFile(path, target)
	})
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

// Install clones a GitHub repo (or owner/repo/path) into ~/.agents/skills and updates the lock file.
func Install(agentsDir, spec string) (string, error) {
	spec = strings.TrimSpace(spec)
	spec = strings.TrimPrefix(spec, "https://github.com/")
	spec = strings.TrimSuffix(spec, ".git")
	parts := strings.Split(spec, "/")
	if len(parts) < 2 {
		return "", fmt.Errorf("install spec must be owner/repo or owner/repo/path")
	}
	owner, repo := parts[0], parts[1]
	sub := ""
	if len(parts) > 2 {
		sub = strings.Join(parts[2:], "/")
	}
	tmp, err := os.MkdirTemp("", "minerva-install-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmp)
	url := fmt.Sprintf("https://github.com/%s/%s.git", owner, repo)
	cmd := exec.Command("git", "clone", "--depth", "1", url, tmp)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("git clone: %w\n%s", err, out)
	}
	search := tmp
	if sub != "" {
		search = filepath.Join(tmp, sub)
	}
	skillRoot, name, skillMD, err := findSkill(search)
	if err != nil {
		return "", err
	}
	dest := filepath.Join(agentsDir, "skills", name)
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return "", err
	}
	_ = os.RemoveAll(dest)
	if err := copyDir(skillRoot, dest); err != nil {
		return "", err
	}
	hash, _ := FolderHash(dest)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	lf, err := LoadLock(agentsDir)
	if err != nil {
		return "", err
	}
	rel := skillMD
	if relPath, err := filepath.Rel(tmp, skillMD); err == nil {
		rel = relPath
	}
	lf.Skills[name] = LockEntry{
		Source: owner + "/" + repo, SourceType: "github", SourceURL: url,
		SkillPath: rel, SkillFolderHash: hash, InstalledAt: now, UpdatedAt: now,
	}
	if err := saveLock(agentsDir, lf); err != nil {
		return "", err
	}
	return name, nil
}

func findSkill(root string) (folder, name, skillMD string, err error) {
	var found []string
	_ = filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil || info.IsDir() {
			return nil
		}
		base := strings.ToLower(info.Name())
		if base == "skill.md" {
			found = append(found, path)
		}
		return nil
	})
	if len(found) == 0 {
		return "", "", "", fmt.Errorf("no SKILL.md under %s", root)
	}
	sort.Strings(found)
	skillMD = found[0]
	folder = filepath.Dir(skillMD)
	name = filepath.Base(folder)
	return folder, name, skillMD, nil
}

// FolderHash is a stable hash of file paths + contents.
func FolderHash(dir string) (string, error) {
	h := sha256.New()
	err := filepath.Walk(dir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		_, _ = io.WriteString(h, rel)
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		_, _ = h.Write(data)
		return nil
	})
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func listSkillNames(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		if e.IsDir() || (e.Type()&os.ModeSymlink != 0) {
			names = append(names, e.Name())
		}
	}
	return names, nil
}

func dirExists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && st.IsDir()
}
