// Package library exports, imports, and lints the shared ~/.agents tree
// (skills, profiles, templates) as a portable bundle.
package library

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	ManifestName    = "manifest.json"
	BundleVersion   = 1
	skillsRel       = "skills"
	agentsRel       = "agents"
	templatesRel    = "templates"
)

// Manifest describes a library bundle.
type Manifest struct {
	Version   int       `json:"version"`
	CreatedAt time.Time `json:"created_at"`
	Source    string    `json:"source,omitempty"`
	Skills    int       `json:"skills"`
	Profiles  int       `json:"profiles"`
	Templates int       `json:"templates"`
	Note      string    `json:"note,omitempty"`
}

// ExportOptions controls what is packaged.
type ExportOptions struct {
	// AgentsDir is the ~/.agents root.
	AgentsDir string
	// Dest is a .tar.gz path or a directory path.
	Dest string
	// IncludeTemplates packs templates/ when present.
	IncludeTemplates bool
	// Note is stored in the manifest.
	Note string
}

// ExportResult summarizes an export.
type ExportResult struct {
	Path      string   `json:"path"`
	Manifest  Manifest `json:"manifest"`
	Format    string   `json:"format"` // tarball | directory
}

// Export packs skills, profiles, and optional templates into dest.
// If dest ends with .tar.gz / .tgz, writes a gzip tarball; otherwise a directory tree.
func Export(opts ExportOptions) (*ExportResult, error) {
	if strings.TrimSpace(opts.AgentsDir) == "" {
		return nil, fmt.Errorf("agents dir is required")
	}
	if strings.TrimSpace(opts.Dest) == "" {
		return nil, fmt.Errorf("destination is required")
	}

	skillsDir := filepath.Join(opts.AgentsDir, skillsRel)
	agentsDir := filepath.Join(opts.AgentsDir, agentsRel)
	templatesDir := filepath.Join(opts.AgentsDir, templatesRel)

	skillCount, _ := countSubdirs(skillsDir)
	profileCount, _ := countSubdirs(agentsDir)
	templateCount := 0
	if opts.IncludeTemplates {
		templateCount, _ = countSubdirs(templatesDir)
	}

	man := Manifest{
		Version:   BundleVersion,
		CreatedAt: time.Now().UTC(),
		Source:    opts.AgentsDir,
		Skills:    skillCount,
		Profiles:  profileCount,
		Templates: templateCount,
		Note:      opts.Note,
	}

	format := "directory"
	if isTarballPath(opts.Dest) {
		format = "tarball"
		if err := exportTarball(opts, man); err != nil {
			return nil, err
		}
	} else {
		if err := exportDirectory(opts, man); err != nil {
			return nil, err
		}
	}
	return &ExportResult{Path: opts.Dest, Manifest: man, Format: format}, nil
}

func isTarballPath(p string) bool {
	lp := strings.ToLower(p)
	return strings.HasSuffix(lp, ".tar.gz") || strings.HasSuffix(lp, ".tgz")
}

func exportDirectory(opts ExportOptions, man Manifest) error {
	if err := os.MkdirAll(opts.Dest, 0o755); err != nil {
		return err
	}
	if err := writeManifest(filepath.Join(opts.Dest, ManifestName), man); err != nil {
		return err
	}
	if err := copyTree(filepath.Join(opts.AgentsDir, skillsRel), filepath.Join(opts.Dest, skillsRel)); err != nil {
		return err
	}
	if err := copyTree(filepath.Join(opts.AgentsDir, agentsRel), filepath.Join(opts.Dest, agentsRel)); err != nil {
		return err
	}
	if opts.IncludeTemplates {
		if err := copyTree(filepath.Join(opts.AgentsDir, templatesRel), filepath.Join(opts.Dest, templatesRel)); err != nil {
			return err
		}
	}
	return nil
}

func exportTarball(opts ExportOptions, man Manifest) error {
	if err := os.MkdirAll(filepath.Dir(opts.Dest), 0o755); err != nil && filepath.Dir(opts.Dest) != "." {
		// dest may be in cwd
		_ = err
	}
	f, err := os.Create(opts.Dest)
	if err != nil {
		return err
	}
	defer f.Close()

	gz := gzip.NewWriter(f)
	defer gz.Close()
	tw := tar.NewWriter(gz)
	defer tw.Close()

	// manifest
	data, err := json.MarshalIndent(man, "", "  ")
	if err != nil {
		return err
	}
	if err := writeTarFile(tw, ManifestName, data); err != nil {
		return err
	}

	if err := addDirToTar(tw, filepath.Join(opts.AgentsDir, skillsRel), skillsRel); err != nil {
		return err
	}
	if err := addDirToTar(tw, filepath.Join(opts.AgentsDir, agentsRel), agentsRel); err != nil {
		return err
	}
	if opts.IncludeTemplates {
		if err := addDirToTar(tw, filepath.Join(opts.AgentsDir, templatesRel), templatesRel); err != nil {
			return err
		}
	}
	return nil
}

// ImportOptions controls merge behavior.
type ImportOptions struct {
	// Source is a .tar.gz path or a directory with manifest.json.
	Source string
	// AgentsDir is the destination ~/.agents root.
	AgentsDir string
	// Force overwrites existing skill/profile/template dirs.
	Force bool
	// IncludeTemplates imports templates when present in the bundle.
	IncludeTemplates bool
}

// ImportResult summarizes import.
type ImportResult struct {
	Skills    int `json:"skills"`
	Profiles  int `json:"profiles"`
	Templates int `json:"templates"`
	Skipped   int `json:"skipped"`
}

// Import merges a bundle into AgentsDir.
func Import(opts ImportOptions) (*ImportResult, error) {
	if strings.TrimSpace(opts.Source) == "" || strings.TrimSpace(opts.AgentsDir) == "" {
		return nil, fmt.Errorf("source and agents dir are required")
	}

	staging, cleanup, err := stageSource(opts.Source)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	res := &ImportResult{}
	for _, item := range []struct {
		rel   string
		count *int
		skip  bool
	}{
		{skillsRel, &res.Skills, false},
		{agentsRel, &res.Profiles, false},
		{templatesRel, &res.Templates, !opts.IncludeTemplates},
	} {
		if item.skip {
			continue
		}
		src := filepath.Join(staging, item.rel)
		dst := filepath.Join(opts.AgentsDir, item.rel)
		n, skipped, err := mergeSubdirs(src, dst, opts.Force)
		if err != nil {
			return nil, err
		}
		*item.count = n
		res.Skipped += skipped
	}
	return res, nil
}

func stageSource(source string) (dir string, cleanup func(), err error) {
	info, err := os.Stat(source)
	if err != nil {
		return "", nil, err
	}
	if info.IsDir() {
		return source, func() {}, nil
	}
	if !isTarballPath(source) {
		return "", nil, fmt.Errorf("source must be a directory or .tar.gz/.tgz")
	}
	tmp, err := os.MkdirTemp("", "minerva-import-*")
	if err != nil {
		return "", nil, err
	}
	if err := extractTarball(source, tmp); err != nil {
		os.RemoveAll(tmp)
		return "", nil, err
	}
	return tmp, func() { os.RemoveAll(tmp) }, nil
}

func extractTarball(path, dest string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		// Prevent path traversal.
		name := filepath.Clean(hdr.Name)
		if strings.HasPrefix(name, "..") || filepath.IsAbs(name) {
			return fmt.Errorf("unsafe path in archive: %s", hdr.Name)
		}
		target := filepath.Join(dest, name)
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(hdr.Mode))
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return err
			}
			out.Close()
		}
	}
	return nil
}

func mergeSubdirs(src, dst string, force bool) (copied, skipped int, err error) {
	entries, err := os.ReadDir(src)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, 0, nil
		}
		return 0, 0, err
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return 0, 0, err
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		from := filepath.Join(src, e.Name())
		to := filepath.Join(dst, e.Name())
		if _, err := os.Stat(to); err == nil && !force {
			skipped++
			continue
		}
		if force {
			_ = os.RemoveAll(to)
		}
		if err := copyTree(from, to); err != nil {
			return copied, skipped, err
		}
		copied++
	}
	return copied, skipped, nil
}

// --- helpers ---

func writeManifest(path string, man Manifest) error {
	data, err := json.MarshalIndent(man, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func writeTarFile(tw *tar.Writer, name string, data []byte) error {
	hdr := &tar.Header{
		Name:    name,
		Mode:    0o644,
		Size:    int64(len(data)),
		ModTime: time.Now(),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	_, err := tw.Write(data)
	return err
}

func addDirToTar(tw *tar.Writer, src, prefix string) error {
	if _, err := os.Stat(src); os.IsNotExist(err) {
		return nil
	}
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		name := filepath.ToSlash(filepath.Join(prefix, rel))
		if info.IsDir() {
			if !strings.HasSuffix(name, "/") {
				name += "/"
			}
			hdr, err := tar.FileInfoHeader(info, "")
			if err != nil {
				return err
			}
			hdr.Name = name
			return tw.WriteHeader(hdr)
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		hdr, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		hdr.Name = name
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = io.Copy(tw, f)
		return err
	})
}

func copyTree(src, dst string) error {
	if _, err := os.Stat(src); os.IsNotExist(err) {
		return nil
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
		if !info.Mode().IsRegular() {
			return nil
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode())
		if err != nil {
			return err
		}
		defer out.Close()
		_, err = io.Copy(out, in)
		return err
	})
}

func countSubdirs(dir string) (int, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	n := 0
	for _, e := range entries {
		if e.IsDir() {
			n++
		}
	}
	return n, nil
}
