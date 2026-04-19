package appcli

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"mnemosyneos/internal/config"
	"mnemosyneos/internal/runtimeapp"
)

// RunServe starts the HTTP server using the same runtime wiring as the web
// stack. Expected flags (after the subcommand name is stripped by the caller):
//
//	--runtime-root <path>   (default $MNEMOSYNE_RUNTIME_ROOT or ./runtime)
//	--addr <listen>         (default $MNEMOSYNE_ADDR or :8080)
//	--workspace-root <dir> (default current working directory)
//	--ui web|api            (web enables /ui routes; api is REST-only)
func RunServe(flagArgs []string) error {
	_ = config.LoadDefaultLocalEnv()

	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	runtimeRoot := fs.String("runtime-root", "", "runtime root directory")
	addr := fs.String("addr", "", "listen address (e.g. :8080)")
	workspaceRoot := fs.String("workspace-root", "", "workspace root directory")
	ui := fs.String("ui", "web", `web (default) or "api" for REST-only`)
	if err := fs.Parse(flagArgs); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected arguments: %v", fs.Args())
	}

	root := resolveRuntimeRootForServe(*runtimeRoot)
	listen := resolveListenAddrForServe(*addr)
	ws := strings.TrimSpace(*workspaceRoot)
	if ws == "" {
		if v := strings.TrimSpace(os.Getenv("MNEMOSYNE_WORKSPACE_ROOT")); v != "" {
			ws = v
		} else {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			ws = cwd
		}
	}
	wsAbs, err := filepath.Abs(ws)
	if err != nil {
		return err
	}
	enableWeb := strings.EqualFold(strings.TrimSpace(*ui), "web")

	app, err := runtimeapp.Build(runtimeapp.Options{
		Addr:          listen,
		RuntimeRoot:   root,
		WorkspaceRoot: wsAbs,
		EnableWeb:     enableWeb,
	})
	if err != nil {
		return err
	}
	defer app.Shutdown()

	fmt.Fprintf(os.Stderr, "MnemosyneOS listening on %s (runtime=%s web=%v)\n", listen, root, enableWeb)
	return http.ListenAndServe(listen, app.Handler)
}

func resolveRuntimeRootForServe(explicit string) string {
	if strings.TrimSpace(explicit) != "" {
		return strings.TrimSpace(explicit)
	}
	if v := strings.TrimSpace(os.Getenv("MNEMOSYNE_RUNTIME_ROOT")); v != "" {
		return v
	}
	return "runtime"
}

func resolveListenAddrForServe(explicit string) string {
	if strings.TrimSpace(explicit) != "" {
		return strings.TrimSpace(explicit)
	}
	if v := strings.TrimSpace(os.Getenv("MNEMOSYNE_ADDR")); v != "" {
		return v
	}
	return ":8080"
}
