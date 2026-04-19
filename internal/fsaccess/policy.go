package fsaccess

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ExpandHomeDir replaces a leading "~" or "~/" with the current user's home
// directory. Unsupported forms like "~other/file" return an error.
func ExpandHomeDir(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" || !strings.HasPrefix(path, "~") {
		return path, nil
	}
	if len(path) > 1 && path[1] != '/' && path[1] != filepath.Separator {
		return "", fmt.Errorf("unsupported home-relative path %q", path)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	if path == "~" {
		return home, nil
	}
	trim := strings.TrimPrefix(path, "~")
	trim = strings.TrimLeft(trim, `/\`+string(filepath.Separator))
	return filepath.Join(home, trim), nil
}

// ExtraRootsFromEnv parses MNEMOSYNE_FILESYSTEM_EXTRA_ROOTS using
// filepath.SplitList (':' on Unix, ';' on Windows). Each entry may start with
// "~" for the current user's home. Non-absolute paths after expansion are skipped.
func ExtraRootsFromEnv() []string {
	raw := strings.TrimSpace(os.Getenv("MNEMOSYNE_FILESYSTEM_EXTRA_ROOTS"))
	if raw == "" {
		return nil
	}
	parts := filepath.SplitList(raw)
	seen := make(map[string]struct{})
	var out []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		expanded, err := ExpandHomeDir(p)
		if err != nil {
			continue
		}
		if !filepath.IsAbs(expanded) {
			continue
		}
		abs, err := filepath.Abs(expanded)
		if err != nil {
			continue
		}
		if clean, err := filepath.EvalSymlinks(abs); err == nil {
			abs = clean
		}
		if _, ok := seen[abs]; ok {
			continue
		}
		seen[abs] = struct{}{}
		out = append(out, abs)
	}
	return out
}

// evalSymlinksInExistingPrefix walks up from path until EvalSymlinks succeeds on
// an existing prefix, then re-attaches the trailing suffix. This keeps workspace
// containment checks correct for not-yet-created files on hosts where the
// workspace path and EvalSymlinks disagree (for example /var vs /private/var on
// macOS) while still expanding symlinks on paths that fully exist.
func evalSymlinksInExistingPrefix(path string) string {
	path = filepath.Clean(path)
	if path == "" || path == "." {
		return path
	}
	p := path
	for {
		resolved, err := filepath.EvalSymlinks(p)
		if err == nil {
			if p == path {
				return resolved
			}
			rel, err := filepath.Rel(p, path)
			if err != nil {
				return filepath.Clean(path)
			}
			if rel == "." {
				return resolved
			}
			return filepath.Join(resolved, rel)
		}
		parent := filepath.Dir(p)
		if parent == p {
			return filepath.Clean(path)
		}
		p = parent
	}
}

// PathUnderRoot reports whether path is equal to root or strictly inside root.
// Both paths should be absolute; root is cleaned and symlink-resolved when possible.
func PathUnderRoot(path, root string) bool {
	root = strings.TrimSpace(root)
	if root == "" {
		return false
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return false
	}
	if resolvedRoot, err := filepath.EvalSymlinks(absRoot); err == nil {
		absRoot = resolvedRoot
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	checkPath := evalSymlinksInExistingPrefix(absPath)
	rel, err := filepath.Rel(absRoot, checkPath)
	if err != nil {
		return false
	}
	return rel == "." || (!strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != "..")
}

// IsPathAllowed reports whether resolvedPath (absolute, ideally clean) is
// allowed when the filesystem is restricted: under workspace, runtime, temp,
// or any MNEMOSYNE_FILESYSTEM_EXTRA_ROOTS entry. Unrestricted mode allows all paths.
func IsPathAllowed(resolvedPath, workspaceRoot, runtimeRoot string) bool {
	if UnrestrictedFromEnv() {
		return true
	}
	for _, root := range []string{workspaceRoot, runtimeRoot, os.TempDir()} {
		if PathUnderRoot(resolvedPath, root) {
			return true
		}
	}
	for _, root := range ExtraRootsFromEnv() {
		if PathUnderRoot(resolvedPath, root) {
			return true
		}
	}
	return false
}

// IsReadPathAllowed is the read-only variant of IsPathAllowed. Reads are
// strictly less dangerous than writes — an agent that can read any file the
// OS already lets the user read cannot actually leak secrets the user hasn't
// chosen to expose (POSIX file permissions still apply). We therefore default
// to "any absolute path" for read-only tools like list_directory /
// search_files / read_file, matching the OpenClaw personal-agent model, and
// keep IsPathAllowed strict for anything that mutates state.
//
// MNEMOSYNE_FILESYSTEM_READONLY_STRICT=true lets an operator opt back into
// the legacy workspace-only read policy without having to micro-manage
// MNEMOSYNE_FILESYSTEM_EXTRA_ROOTS. A path is always rejected if it isn't
// absolute, and IsPathAllowed always wins when it is satisfied (so write
// roots remain readable in strict mode too).
func IsReadPathAllowed(resolvedPath, workspaceRoot, runtimeRoot string) bool {
	if IsPathAllowed(resolvedPath, workspaceRoot, runtimeRoot) {
		return true
	}
	if readOnlyStrictFromEnv() {
		return false
	}
	return filepath.IsAbs(resolvedPath)
}

func readOnlyStrictFromEnv() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("MNEMOSYNE_FILESYSTEM_READONLY_STRICT")))
	switch v {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
