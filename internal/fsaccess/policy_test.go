package fsaccess

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestExpandHomeDirTilde(t *testing.T) {
	t.Parallel()
	got, err := ExpandHomeDir("~/Documents")
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(got) || filepath.Base(got) != "Documents" {
		t.Fatalf("unexpected expanded path: %q", got)
	}
}

func TestExtraRootsFromEnvSplit(t *testing.T) {
	a := filepath.Join(t.TempDir(), "a")
	b := filepath.Join(t.TempDir(), "b")
	t.Setenv("MNEMOSYNE_FILESYSTEM_EXTRA_ROOTS", a+string(filepath.ListSeparator)+b)

	got := ExtraRootsFromEnv()
	if len(got) != 2 {
		t.Fatalf("expected 2 roots, got %#v", got)
	}
}

func TestIsPathAllowedWithExtraRoot(t *testing.T) {
	ws := t.TempDir()
	out := t.TempDir()
	child := filepath.Join(out, "nested")
	if err := os.MkdirAll(child, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MNEMOSYNE_FILESYSTEM_EXTRA_ROOTS", out)
	t.Setenv("MNEMOSYNE_FILESYSTEM_UNRESTRICTED", "")

	if !IsPathAllowed(child, ws, "") {
		t.Fatalf("expected child under extra root to be allowed, ws=%q out=%q child=%q", ws, out, child)
	}
	outside := "/usr/bin/env"
	if runtime.GOOS == "windows" {
		outside = `C:\Windows\System32\config\systemprofile`
	}
	if IsPathAllowed(outside, ws, "") {
		t.Fatalf("expected path outside workspace and extra roots to be denied: %q", outside)
	}
}

func TestPathUnderRootNonExistentPathStillInsideWorkspace(t *testing.T) {
	ws := t.TempDir()
	child := filepath.Join(ws, "not-yet", "created.txt")
	if !PathUnderRoot(child, ws) {
		t.Fatalf("expected non-existent file path under workspace, ws=%q child=%q", ws, child)
	}
}
