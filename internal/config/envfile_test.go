package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDefaultLocalEnvUsesDotenvPath(t *testing.T) {
	_ = os.Unsetenv("MNEMOSYNE_FROM_TEST")
	root := t.TempDir()
	p := filepath.Join(root, "custom.env")
	if err := os.WriteFile(p, []byte("MNEMOSYNE_FROM_TEST=hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MNEMOSYNE_DOTENV_PATH", p)
	if err := LoadDefaultLocalEnv(); err != nil {
		t.Fatalf("LoadDefaultLocalEnv: %v", err)
	}
	if got := os.Getenv("MNEMOSYNE_FROM_TEST"); got != "hello" {
		t.Fatalf("got %q", got)
	}
}
