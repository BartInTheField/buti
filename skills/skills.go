// Package skills embeds the agent skills in this directory, so `buti skill install` can copy them into a coding
// agent's skills directory. Each skill is a directory with a SKILL.md, as in the Agent Skills format.
package skills

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
)

//go:embed buti-*
var files embed.FS

// Names returns the embedded skills, sorted.
func Names() []string {
	entries, _ := files.ReadDir(".")
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	slices.Sort(names)
	return names
}

// Read returns the SKILL.md of the named skill.
func Read(name string) ([]byte, error) {
	if !slices.Contains(Names(), name) {
		return nil, fmt.Errorf("no skill %q", name)
	}
	return files.ReadFile(path.Join(name, "SKILL.md"))
}

// Install copies the named skill into dir/<name>, replacing the files an earlier install wrote, and returns the
// directory it wrote.
func Install(name, dir string) (string, error) {
	if !slices.Contains(Names(), name) {
		return "", fmt.Errorf("no skill %q", name)
	}
	dest := filepath.Join(dir, name)
	err := fs.WalkDir(files, name, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		target := filepath.Join(dir, filepath.FromSlash(p))
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := files.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
	if err != nil {
		return "", err
	}
	return dest, nil
}
