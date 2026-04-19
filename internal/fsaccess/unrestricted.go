package fsaccess

import (
	"os"
	"strings"
)

// UnrestrictedFromEnv reports whether MNEMOSYNE_FILESYSTEM_UNRESTRICTED is enabled.
// When true, the execution plane and agent file tools allow absolute paths outside
// the configured workspace (similar to a fully trusted local agent). Default is off.
//
// For a narrower escape hatch, set MNEMOSYNE_FILESYSTEM_EXTRA_ROOTS to an OS-specific
// list (same separators as PATH) of absolute directories that are also allowed,
// e.g. "$HOME" or "/Users/you/projects" on Unix. See ExtraRootsFromEnv and IsPathAllowed.
func UnrestrictedFromEnv() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("MNEMOSYNE_FILESYSTEM_UNRESTRICTED")))
	switch v {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
