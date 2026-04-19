package skills

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestAgentShellToolDisabledByDefault(t *testing.T) {
	t.Setenv("MNEMOSYNE_AGENT_ENABLE_SHELL_TOOL", "")

	reg := NewAgentSkillRegistry()
	RegisterBuiltinAgentSkills(reg, BuiltinSkillOpts{
		WorkspaceRoot: t.TempDir(),
		RuntimeRoot:   t.TempDir(),
	})

	for _, def := range reg.ToolDefinitions() {
		if def.Function.Name == "run_command" {
			t.Fatalf("run_command should not be registered by default")
		}
	}
	if _, err := reg.Execute(context.Background(), "run_command", `{"command":"echo unsafe"}`); err == nil {
		t.Fatalf("expected run_command to be unavailable by default")
	}
}

func TestAgentFileToolsAllowAbsolutePathsOutsideWorkspaceWhenUnrestricted(t *testing.T) {
	t.Setenv("MNEMOSYNE_FILESYSTEM_UNRESTRICTED", "1")
	t.Cleanup(func() { _ = os.Unsetenv("MNEMOSYNE_FILESYSTEM_UNRESTRICTED") })

	workspaceRoot := t.TempDir()
	runtimeRoot := t.TempDir()
	outside := t.TempDir()
	outsideFile := filepath.Join(outside, "wide.txt")
	if err := os.WriteFile(outsideFile, []byte("ok"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	reg := NewAgentSkillRegistry()
	RegisterBuiltinAgentSkills(reg, BuiltinSkillOpts{
		WorkspaceRoot: workspaceRoot,
		RuntimeRoot:   runtimeRoot,
	})
	out, err := reg.Execute(context.Background(), "read_file", `{"path":`+quoteJSON(outsideFile)+`}`)
	if err != nil {
		t.Fatalf("read_file: %v", err)
	}
	if !strings.Contains(out, "ok") {
		t.Fatalf("expected file content, got %q", out)
	}
}

func TestAgentReadToolsAllowPathsOutsideWorkspaceByDefault(t *testing.T) {
	// Default personal-agent policy: read-only tools accept any absolute
	// path the OS already lets us read. This is the OpenClaw-style model
	// the user asked for so "帮我查找 lab 目录" can actually reach ~/.
	t.Setenv("MNEMOSYNE_FILESYSTEM_EXTRA_ROOTS", "")
	t.Setenv("MNEMOSYNE_FILESYSTEM_UNRESTRICTED", "")
	t.Setenv("MNEMOSYNE_FILESYSTEM_READONLY_STRICT", "")

	workspaceRoot := t.TempDir()
	runtimeRoot := t.TempDir()
	outsideRoot := existingSystemDirOutsideAllowedRoots(t)
	outsideFile := existingSystemFileOutsideAllowedRoots(t)

	reg := NewAgentSkillRegistry()
	RegisterBuiltinAgentSkills(reg, BuiltinSkillOpts{
		WorkspaceRoot: workspaceRoot,
		RuntimeRoot:   runtimeRoot,
	})

	if _, err := reg.Execute(context.Background(), "read_file", `{"path":`+quoteJSON(outsideFile)+`}`); err != nil {
		t.Fatalf("read_file outside workspace should be allowed by default: %v", err)
	}
	if _, err := reg.Execute(context.Background(), "list_directory", `{"path":`+quoteJSON(outsideRoot)+`}`); err != nil {
		t.Fatalf("list_directory outside workspace should be allowed by default: %v", err)
	}
	if _, err := reg.Execute(context.Background(), "search_files", `{"pattern":"*.txt","directory":`+quoteJSON(outsideRoot)+`}`); err != nil {
		t.Fatalf("search_files outside workspace should be allowed by default: %v", err)
	}
}

func TestAgentReadToolsRespectReadonlyStrictOptIn(t *testing.T) {
	// Operators who want the old strict policy can opt in via the
	// READONLY_STRICT env flag; under that flag, reads obey the same
	// workspace/runtime/extra-roots rules as writes.
	t.Setenv("MNEMOSYNE_FILESYSTEM_EXTRA_ROOTS", "")
	t.Setenv("MNEMOSYNE_FILESYSTEM_UNRESTRICTED", "")
	t.Setenv("MNEMOSYNE_FILESYSTEM_READONLY_STRICT", "true")

	workspaceRoot := t.TempDir()
	runtimeRoot := t.TempDir()
	outsideRoot := existingSystemDirOutsideAllowedRoots(t)
	outsideFile := existingSystemFileOutsideAllowedRoots(t)

	reg := NewAgentSkillRegistry()
	RegisterBuiltinAgentSkills(reg, BuiltinSkillOpts{
		WorkspaceRoot: workspaceRoot,
		RuntimeRoot:   runtimeRoot,
	})

	if _, err := reg.Execute(context.Background(), "read_file", `{"path":`+quoteJSON(outsideFile)+`}`); err == nil {
		t.Fatalf("expected read_file to fail under READONLY_STRICT")
	}
	if _, err := reg.Execute(context.Background(), "list_directory", `{"path":`+quoteJSON(outsideRoot)+`}`); err == nil {
		t.Fatalf("expected list_directory to fail under READONLY_STRICT")
	}
	if _, err := reg.Execute(context.Background(), "search_files", `{"pattern":"*.txt","directory":`+quoteJSON(outsideRoot)+`}`); err == nil {
		t.Fatalf("expected search_files to fail under READONLY_STRICT")
	}
}

func TestAgentFileToolsAllowWorkspacePaths(t *testing.T) {
	workspaceRoot := t.TempDir()
	runtimeRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspaceRoot, "README.md"), []byte("hello"), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	reg := NewAgentSkillRegistry()
	RegisterBuiltinAgentSkills(reg, BuiltinSkillOpts{
		WorkspaceRoot: workspaceRoot,
		RuntimeRoot:   runtimeRoot,
	})

	result, err := reg.Execute(context.Background(), "read_file", `{"path":"README.md"}`)
	if err != nil {
		t.Fatalf("read_file returned error: %v", err)
	}
	if result != "hello" {
		t.Fatalf("unexpected read_file result: %q", result)
	}
}

func TestAgentFileToolsRejectSymlinkEscapeUnderReadonlyStrict(t *testing.T) {
	// Symlink escape is only a threat model under the legacy strict read
	// policy. When READONLY_STRICT is set, a workspace-local symlink that
	// points outside allowed roots must still be refused.
	t.Setenv("MNEMOSYNE_FILESYSTEM_EXTRA_ROOTS", "")
	t.Setenv("MNEMOSYNE_FILESYSTEM_UNRESTRICTED", "")
	t.Setenv("MNEMOSYNE_FILESYSTEM_READONLY_STRICT", "true")

	workspaceRoot := t.TempDir()
	runtimeRoot := t.TempDir()
	outsideFile := existingSystemFileOutsideAllowedRoots(t)
	if err := os.Symlink(outsideFile, filepath.Join(workspaceRoot, "secret-link.txt")); err != nil {
		t.Skipf("symlink not available: %v", err)
	}

	reg := NewAgentSkillRegistry()
	RegisterBuiltinAgentSkills(reg, BuiltinSkillOpts{
		WorkspaceRoot: workspaceRoot,
		RuntimeRoot:   runtimeRoot,
	})

	if _, err := reg.Execute(context.Background(), "read_file", `{"path":"secret-link.txt"}`); err == nil {
		t.Fatalf("expected read_file through symlink escape to fail under READONLY_STRICT")
	}
}

func TestWebSearchNotConfiguredMessageLocalizedByLocaleArg(t *testing.T) {
	reg := NewAgentSkillRegistry()
	RegisterBuiltinAgentSkills(reg, BuiltinSkillOpts{
		WorkspaceRoot: t.TempDir(),
		RuntimeRoot:   t.TempDir(),
		Connectors:    nil,
	})
	result, err := reg.Execute(context.Background(), "web_search", `{"query":"latest ai news","_locale":"en"}`)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !strings.Contains(result, "Web search is not enabled") {
		t.Fatalf("expected english config hint, got %q", result)
	}
}

func quoteJSON(value string) string {
	return strconv.Quote(value)
}

func existingSystemDirOutsideAllowedRoots(t *testing.T) string {
	t.Helper()
	for _, path := range []string{"/etc", "/usr"} {
		info, err := os.Stat(path)
		if err == nil && info.IsDir() {
			return path
		}
	}
	t.Skip("no stable system directory available outside allowed roots")
	return ""
}

func existingSystemFileOutsideAllowedRoots(t *testing.T) string {
	t.Helper()
	for _, path := range []string{"/etc/hosts", "/etc/passwd"} {
		info, err := os.Stat(path)
		if err == nil && !info.IsDir() {
			return path
		}
	}
	t.Skip("no stable system file available outside allowed roots")
	return ""
}
