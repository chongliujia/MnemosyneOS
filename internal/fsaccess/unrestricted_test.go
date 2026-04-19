package fsaccess

import "testing"

func TestUnrestrictedFromEnv(t *testing.T) {
	t.Setenv("MNEMOSYNE_FILESYSTEM_UNRESTRICTED", "")
	if UnrestrictedFromEnv() {
		t.Fatal("expected false when unset")
	}
	t.Setenv("MNEMOSYNE_FILESYSTEM_UNRESTRICTED", "1")
	if !UnrestrictedFromEnv() {
		t.Fatal("expected true for 1")
	}
	t.Setenv("MNEMOSYNE_FILESYSTEM_UNRESTRICTED", "TRUE")
	if !UnrestrictedFromEnv() {
		t.Fatal("expected true for TRUE")
	}
}
