package store

import (
	"os"
	"path/filepath"
)

// configDir returns tick's config directory, honoring XDG_CONFIG_HOME on all
// platforms. We resolve it explicitly rather than using os.UserConfigDir
// because on macOS that returns ~/Library/Application Support and deliberately
// ignores XDG_CONFIG_HOME (golang/go#76320).
func configDir() (string, error) {
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" && filepath.IsAbs(x) {
		return filepath.Join(x, "tick"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "tick"), nil
}

// ProjectRoot resolves the project root for a starting directory: it walks up
// looking for a .git entry (a dir or a file, the latter covering git worktrees)
// and falls back to the start directory itself when none is found, so any
// directory can be a "project". The result is absolute and symlink-resolved so
// the same repo reached via different paths maps to a single store entry.
func ProjectRoot(start string) string {
	abs, err := filepath.Abs(start)
	if err != nil {
		abs = start
	}
	root := abs // fallback: the start dir itself is the project
	for dir := abs; ; {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			root = dir
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir { // reached the filesystem root
			break
		}
		dir = parent
	}
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = resolved
	}
	return root
}
