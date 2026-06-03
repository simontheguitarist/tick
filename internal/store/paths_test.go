package store

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestConfigDirHonorsXDG(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/tmp/xdgcfg")
	got, err := configDir()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join("/tmp/xdgcfg", "tick"); got != want {
		t.Fatalf("configDir = %q, want %q", got, want)
	}
}

func TestConfigDirFallsBackToHome(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "") // unset -> ~/.config/tick
	got, err := configDir()
	if err != nil {
		t.Fatal(err)
	}
	home, _ := os.UserHomeDir()
	if want := filepath.Join(home, ".config", "tick"); got != want {
		t.Fatalf("configDir = %q, want %q", got, want)
	}
}

func TestConfigDirIgnoresRelativeXDG(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "relative/path") // must be ignored (not absolute)
	got, err := configDir()
	if err != nil {
		t.Fatal(err)
	}
	home, _ := os.UserHomeDir()
	if want := filepath.Join(home, ".config", "tick"); got != want {
		t.Fatalf("configDir = %q, want %q", got, want)
	}
}

func TestProjectRootFindsGitWalkingUp(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(root, "a", "b", "c")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	got := ProjectRoot(sub)
	want, _ := filepath.EvalSymlinks(root)
	if got != want {
		t.Fatalf("ProjectRoot(%q) = %q, want %q", sub, got, want)
	}
}

func TestProjectRootTreatsGitFileAsMarker(t *testing.T) {
	root := t.TempDir()
	// git worktrees use a .git *file*, not a dir.
	if err := os.WriteFile(filepath.Join(root, ".git"), []byte("gitdir: ../x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := ProjectRoot(root)
	want, _ := filepath.EvalSymlinks(root)
	if got != want {
		t.Fatalf("ProjectRoot = %q, want %q", got, want)
	}
}

func TestProjectRootFallsBackToDir(t *testing.T) {
	dir := t.TempDir() // no .git anywhere up the tree we control
	got := ProjectRoot(dir)
	want, _ := filepath.EvalSymlinks(dir)
	// A .git could theoretically exist above a temp dir; only assert the
	// fallback shape when none was found at/below the temp root.
	if _, err := os.Stat(filepath.Join(want, ".git")); err == nil {
		t.Skip("unexpected .git at temp root")
	}
	if got != want {
		t.Fatalf("ProjectRoot = %q, want %q", got, want)
	}
}

func TestProjectRootResolvesSymlinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks unreliable on windows CI")
	}
	real := t.TempDir()
	if err := os.Mkdir(filepath.Join(real, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	viaReal := ProjectRoot(real)
	viaLink := ProjectRoot(link)
	if viaReal != viaLink {
		t.Fatalf("symlinked paths diverged: %q vs %q", viaReal, viaLink)
	}
}
