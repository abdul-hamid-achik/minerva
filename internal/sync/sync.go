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
	canonicalDir := map[string]string{}
	for _, s := range mgr.All() {
		dir := filepath.Dir(s.Path)
		h, _ := FolderHash(dir)
		canonical[s.Name] = h
		if abs, err := filepath.Abs(dir); err == nil {
			canonicalDir[s.Name] = abs
		}
	}

	var out []Finding
	for _, h := range harness.Catalog(env) {
		if h.Native {
			continue // reads ~/.agents/skills directly; nothing to compare
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
			entry := filepath.Join(h.SkillsDir, name)
			if st, err := os.Lstat(entry); err == nil && st.Mode()&os.ModeSymlink != 0 {
				// Minerva-owned link: fine when it points at the canonical dir.
				target, _ := os.Readlink(entry)
				if !filepath.IsAbs(target) {
					target = filepath.Join(h.SkillsDir, target)
				}
				target = filepath.Clean(target)
				if _, err := os.Stat(target); err != nil {
					out = append(out, Finding{Harness: h.ID, Skill: name, Kind: "broken-link", Message: "symlink target missing: " + target})
				} else if canonicalDir[name] != "" && target != canonicalDir[name] {
					out = append(out, Finding{Harness: h.ID, Skill: name, Kind: "drift", Message: "symlink points outside ~/.agents/skills: " + target})
				}
				continue
			}
			hash, _ := FolderHash(entry)
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
	// Force replaces a harness-local directory whose contents differ from
	// the canonical skill. Without it such directories are reported and kept.
	Force bool
	Env   harness.Env
}

// Action is one sync step.
type Action struct {
	Harness string `json:"harness"`
	Skill   string `json:"skill"`
	Method  string `json:"method"` // symlink|copy|skip
	From    string `json:"from"`
	To      string `json:"to"`
	Done    bool   `json:"done"`
	// Reason explains a skip (or, in dry-run, what would be replaced).
	Reason string `json:"reason,omitempty"`
}

// Method values.
const (
	MethodSymlink = "symlink"
	MethodCopy    = "copy"
	MethodSkip    = "skip"
)

// ReasonDiverged marks a harness-local directory that Minerva did not write
// and whose contents differ from ~/.agents/skills.
const ReasonDiverged = "harness copy differs from ~/.agents/skills; re-run with --force to replace"

// Sync links or copies canonical skills into writable harness dirs.
//
// Destinations that are symlinks (Minerva-owned) or byte-identical copies are
// replaced freely. A real directory with different contents is the harness's
// own work: it is skipped with ReasonDiverged unless opts.Force. Dry-run
// touches nothing, not even the harness skills dir.
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
		if !opts.DryRun {
			if err := os.MkdirAll(h.SkillsDir, 0o755); err != nil {
				return nil, err
			}
		}
		for _, s := range mgr.All() {
			src := filepath.Dir(s.Path)
			dst := filepath.Join(h.SkillsDir, s.Name)
			method := MethodCopy
			if h.LinkSkills {
				method = MethodSymlink
			}
			act := Action{Harness: h.ID, Skill: s.Name, Method: method, From: src, To: dst}
			if reason := unsyncable(s, dst, h.SkillsDir); reason != "" {
				act.Method = MethodSkip
				act.Reason = reason
				actions = append(actions, act)
				continue
			}
			state := inspectDest(src, dst)
			if state == destDiverged && !opts.Force {
				act.Method = MethodSkip
				act.Reason = ReasonDiverged
				actions = append(actions, act)
				continue
			}
			if state == destDiverged {
				act.Reason = "replaced diverged harness copy (--force)"
			}
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

// unsyncable explains why s cannot be placed at dst, or returns "". A flat
// <name>.md skill has the whole library as its folder, and a name that is not
// a single path element would place it outside the harness skills dir.
func unsyncable(s *skill.Skill, dst, skillsDir string) string {
	if skill.IsFlat(s) {
		return "flat .md skill; move it to <name>/SKILL.md to sync it"
	}
	if err := skill.ValidateName(s.Name); err != nil {
		return "invalid skill name: " + err.Error()
	}
	if filepath.Dir(dst) != filepath.Clean(skillsDir) {
		return "skill name escapes the harness skills dir"
	}
	return ""
}

type destState int

const (
	destAbsent destState = iota
	destLink
	destIdentical
	destDiverged
)

// inspectDest classifies what is currently at dst relative to src.
func inspectDest(src, dst string) destState {
	st, err := os.Lstat(dst)
	if err != nil {
		return destAbsent
	}
	if st.Mode()&os.ModeSymlink != 0 {
		return destLink
	}
	if !st.IsDir() {
		return destDiverged // a stray file with the skill's name
	}
	want, err1 := FolderHash(src)
	have, err2 := FolderHash(dst)
	if err1 != nil || err2 != nil || want != have {
		return destDiverged
	}
	return destIdentical
}

func place(src, dst, method string) error {
	if _, err := os.Lstat(dst); err == nil {
		if err := os.RemoveAll(dst); err != nil {
			return err
		}
	}
	if method == MethodSymlink {
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
		// Following a link would copy whatever it points at (e.g. a key from
		// the user's home) into the skill and then into every harness.
		if !info.Mode().IsRegular() {
			return fmt.Errorf("refusing to copy %s: not a regular file", path)
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

// Install clones a GitHub repo (or owner/repo/path) into ~/.agents/skills and
// updates the lock file. When the repo holds several SKILL.md files and no
// path narrows the choice, Install fails and lists them. An existing local
// skill that the lock file does not know about is never replaced unless force.
func Install(agentsDir, spec string, force bool) (string, error) {
	spec = strings.TrimSpace(spec)
	spec = strings.TrimPrefix(spec, "https://github.com/")
	spec = strings.TrimSuffix(spec, ".git")
	parts := strings.Split(spec, "/")
	if len(parts) < 2 {
		return "", fmt.Errorf("install spec must be owner/repo or owner/repo/path")
	}
	owner, repo := parts[0], parts[1]
	for _, p := range parts {
		if p == "" || p == "." || p == ".." || strings.Contains(p, `\`) {
			return "", fmt.Errorf("install spec %q has an invalid path element %q", spec, p)
		}
	}
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
	skillRoot, name, skillMD, err := findSkill(search, tmp)
	if err != nil {
		return "", err
	}
	if skillRoot == tmp {
		name = repo // SKILL.md at the repo root: the temp dir name means nothing
	}
	if err := skill.ValidateName(name); err != nil {
		return "", fmt.Errorf("installed skill folder %q: %w", name, err)
	}
	lf, err := LoadLock(agentsDir)
	if err != nil {
		return "", err
	}
	dest := filepath.Join(agentsDir, "skills", name)
	if _, statErr := os.Lstat(dest); statErr == nil {
		if _, tracked := lf.Skills[name]; !tracked && !force {
			return "", fmt.Errorf("skill %q already exists in %s and is not in .skill-lock.json; delete it or re-run with --force", name, filepath.Dir(dest))
		}
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return "", err
	}
	if err := os.RemoveAll(dest); err != nil {
		return "", err
	}
	if err := copyDir(skillRoot, dest); err != nil {
		return "", err
	}
	hash, _ := FolderHash(dest)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	installedAt := now
	if prev, ok := lf.Skills[name]; ok && prev.InstalledAt != "" {
		installedAt = prev.InstalledAt // re-install keeps the original date
	}
	rel := skillMD
	if relPath, err := filepath.Rel(tmp, skillMD); err == nil {
		rel = relPath
	}
	lf.Skills[name] = LockEntry{
		Source: owner + "/" + repo, SourceType: "github", SourceURL: url,
		SkillPath: rel, SkillFolderHash: hash, InstalledAt: installedAt, UpdatedAt: now,
	}
	if err := saveLock(agentsDir, lf); err != nil {
		return "", err
	}
	return name, nil
}

// findSkill locates exactly one SKILL.md under root. repoRoot is used to
// render candidate paths when the choice is ambiguous.
func findSkill(root, repoRoot string) (folder, name, skillMD string, err error) {
	var found []string
	_ = filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if info.IsDir() {
			if base := info.Name(); path != root && (base == ".git" || base == "node_modules") {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.ToLower(info.Name()) == "skill.md" {
			found = append(found, path)
		}
		return nil
	})
	if len(found) == 0 {
		return "", "", "", fmt.Errorf("no SKILL.md under %s", root)
	}
	sort.Strings(found)
	if len(found) > 1 {
		var rels []string
		for _, p := range found {
			rel := filepath.Dir(p)
			if r, err := filepath.Rel(repoRoot, rel); err == nil {
				rel = r
			}
			rels = append(rels, "  owner/repo/"+filepath.ToSlash(rel))
		}
		return "", "", "", fmt.Errorf("%d skills found; pick one by path:\n%s", len(found), strings.Join(rels, "\n"))
	}
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
		if isHarnessOwnedEntry(e.Name()) {
			continue
		}
		switch {
		case e.Type()&os.ModeSymlink != 0:
			// Keep links (even broken ones) so doctor can report them.
			names = append(names, e.Name())
		case e.IsDir() && hasSkillFile(filepath.Join(dir, e.Name())):
			names = append(names, e.Name())
		}
	}
	return names, nil
}

// isHarnessOwnedEntry reports names a harness keeps in its skills dir for its
// own bookkeeping (dot-entries such as .archive or .bundled_manifest, and
// Hermes's _shared). They are never skills and never touched.
func isHarnessOwnedEntry(name string) bool {
	return strings.HasPrefix(name, ".") || name == "_shared"
}

// hasSkillFile reports whether dir directly contains SKILL.md. Category
// folders (Hermes groups skills in them) are not skills themselves.
func hasSkillFile(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !e.IsDir() && strings.EqualFold(e.Name(), "SKILL.md") {
			return true
		}
	}
	return false
}

func dirExists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && st.IsDir()
}
