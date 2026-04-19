package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"mnemosyneos/internal/config"
)

type daemonInfo struct {
	PID       int    `json:"pid"`
	Addr      string `json:"addr"`
	StartedAt string `json:"started_at"`
}

func daemonPIDPath(runtimeRoot string) string {
	return filepath.Join(runtimeRoot, "state", "daemon.json")
}

func readDaemonInfo(runtimeRoot string) (*daemonInfo, error) {
	data, err := os.ReadFile(daemonPIDPath(runtimeRoot))
	if err != nil {
		return nil, err
	}
	var info daemonInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return nil, err
	}
	return &info, nil
}

func writeDaemonInfo(runtimeRoot string, info daemonInfo) error {
	if err := os.MkdirAll(filepath.Join(runtimeRoot, "state"), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(daemonPIDPath(runtimeRoot), data, 0o644)
}

func removeDaemonInfo(runtimeRoot string) {
	_ = os.Remove(daemonPIDPath(runtimeRoot))
}

// isDaemonAlive checks whether the daemon process is still running AND
// actually responding to HTTP health checks.
func isDaemonAlive(runtimeRoot string) (*daemonInfo, bool) {
	info, err := readDaemonInfo(runtimeRoot)
	if err != nil {
		return nil, false
	}
	if !isProcessAlive(info.PID) {
		removeDaemonInfo(runtimeRoot)
		return nil, false
	}
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get("http://" + normalizeAddr(info.Addr) + "/health")
	if err != nil {
		return info, false
	}
	resp.Body.Close()
	return info, resp.StatusCode == http.StatusOK
}

func isProcessAlive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	if runtime.GOOS == "windows" {
		return true
	}
	return proc.Signal(syscall.Signal(0)) == nil
}

func normalizeAddr(addr string) string {
	if strings.HasPrefix(addr, ":") {
		return "127.0.0.1" + addr
	}
	return addr
}

// startDaemon launches `mnemosynectl serve` as a detached background process.
// It returns the daemon info on success.
func startDaemon(runtimeRoot, listenAddr string) (*daemonInfo, error) {
	execPath, err := os.Executable()
	if err != nil {
		execPath, err = exec.LookPath(os.Args[0])
		if err != nil {
			return nil, fmt.Errorf("cannot find mnemosynectl executable: %w", err)
		}
	}

	cwd, _ := os.Getwd()
	args := []string{"serve", "--runtime-root", runtimeRoot, "--addr", listenAddr}
	if cwd != "" {
		args = append(args, "--workspace-root", cwd)
	}

	logDir := filepath.Join(runtimeRoot, "state")
	_ = os.MkdirAll(logDir, 0o755)
	logPath := filepath.Join(logDir, "daemon.log")

	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open daemon log: %w", err)
	}

	cmd := exec.Command(execPath, args...)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.Dir = "."
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	if err := cmd.Start(); err != nil {
		logFile.Close()
		return nil, fmt.Errorf("start daemon: %w", err)
	}
	logFile.Close()

	info := daemonInfo{
		PID:       cmd.Process.Pid,
		Addr:      listenAddr,
		StartedAt: time.Now().UTC().Format(time.RFC3339),
	}
	if err := writeDaemonInfo(runtimeRoot, info); err != nil {
		return nil, err
	}

	// Detach from the child so it doesn't become a zombie.
	_ = cmd.Process.Release()

	return &info, nil
}

func stopDaemon(runtimeRoot string) error {
	info, err := readDaemonInfo(runtimeRoot)
	if err != nil {
		return fmt.Errorf("no daemon info found (is it running?)")
	}
	if !isProcessAlive(info.PID) {
		removeDaemonInfo(runtimeRoot)
		return fmt.Errorf("daemon (PID %d) is not running (stale PID file removed)", info.PID)
	}
	proc, err := os.FindProcess(info.PID)
	if err != nil {
		return err
	}
	if err := proc.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("send SIGTERM to PID %d: %w", info.PID, err)
	}
	for i := 0; i < 30; i++ {
		time.Sleep(100 * time.Millisecond)
		if !isProcessAlive(info.PID) {
			removeDaemonInfo(runtimeRoot)
			return nil
		}
	}
	_ = proc.Signal(syscall.SIGKILL)
	removeDaemonInfo(runtimeRoot)
	return nil
}

// waitForHealthy polls the server until it responds to /health or timeout.
func waitForHealthy(addr string, timeout time.Duration) bool {
	client := &http.Client{Timeout: 2 * time.Second}
	deadline := time.Now().Add(timeout)
	url := "http://" + normalizeAddr(addr) + "/health"
	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return true
			}
		}
		time.Sleep(150 * time.Millisecond)
	}
	return false
}

func launchNewDaemon(runtimeRoot, listenAddr string) (*daemonInfo, error) {
	bootstrapIfNeeded(runtimeRoot)
	return startDaemon(runtimeRoot, listenAddr)
}

func printDaemonReadyBanner(w io.Writer, runtimeRoot string, info *daemonInfo) {
	base := "http://" + normalizeAddr(info.Addr)
	fmt.Fprintf(w, "\n  %s%sMnemosyneOS%s %sv%s%s\n", cBold, cCyan, cReset, cDim, version, cReset)
	fmt.Fprintf(w, "  %s[OK]%s Daemon running  PID %d\n", cGreen, cReset, info.PID)
	fmt.Fprintf(w, "  %s[OK]%s API             %s%s%s\n", cGreen, cReset, cCyan, base, cReset)
	fmt.Fprintf(w, "  %s[OK]%s Web UI          %s%s/ui/chat%s\n", cGreen, cReset, cCyan, base, cReset)
	fmt.Fprintf(w, "  %s[OK]%s Logs            %s\n\n", cGreen, cReset, filepath.Join(runtimeRoot, "state", "daemon.log"))
}

// autoStartAfterInit starts the background API when init finishes, so the default
// no-subcommand chat can reach localhost without a separate `start` step.
// Failures are warnings only: init files are already written.
func autoStartAfterInit(w io.Writer, runtimeRoot string) {
	_ = loadEnv()
	if info, alive := isDaemonAlive(runtimeRoot); alive {
		fmt.Fprintf(w, "\n  %s[OK]%s MnemosyneOS is already running (PID %d) at %s%s%s\n\n",
			cGreen, cReset, info.PID, cCyan, "http://"+normalizeAddr(info.Addr), cReset)
		return
	}
	listenAddr := resolveListenAddr("")
	fmt.Fprintf(w, "\n  %s⠿ Starting MnemosyneOS in the background…%s\n", cDim, cReset)
	info, err := launchNewDaemon(runtimeRoot, listenAddr)
	if err != nil {
		fmt.Fprintf(w, "  %s[WARN]%s Could not start the API automatically: %v\n", cYellow, cReset, err)
		fmt.Fprintf(w, "         Start it manually: mnemosynectl start\n\n")
		return
	}
	if !waitForHealthy(info.Addr, 10*time.Second) {
		fmt.Fprintf(w, "  %s[WARN]%s Daemon started (PID %d) but health check timed out.\n", cYellow, cReset, info.PID)
		fmt.Fprintf(w, "         Check logs: %s\n\n", filepath.Join(runtimeRoot, "state", "daemon.log"))
		return
	}
	printDaemonReadyBanner(w, runtimeRoot, info)
}

// ── CLI commands ─────────────────────────────────────────────────────

func cmdStart(args []string) {
	fs := flag.NewFlagSet("start", flag.ExitOnError)
	addr := fs.String("addr", "", "listen address (default :8080)")
	runtimeRoot := fs.String("runtime-root", "", "runtime root directory")
	fs.Parse(args)

	_ = loadEnv()
	root := resolveRuntimeRoot(*runtimeRoot)

	if info, alive := isDaemonAlive(root); alive {
		fmt.Fprintf(os.Stderr, "  %s[OK]%s MnemosyneOS is already running (PID %d) at %s%s%s\n\n",
			cGreen, cReset, info.PID, cCyan, "http://"+normalizeAddr(info.Addr), cReset)
		return
	}

	listenAddr := resolveListenAddr(*addr)

	fmt.Fprintf(os.Stderr, "  %s⠿ Starting MnemosyneOS daemon...%s\n", cDim, cReset)
	info, err := launchNewDaemon(root, listenAddr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "  %s[ERROR]%s %v\n", cRed, cReset, err)
		os.Exit(1)
	}

	if !waitForHealthy(info.Addr, 10*time.Second) {
		fmt.Fprintf(os.Stderr, "  %s[WARN]%s Daemon started (PID %d) but health check timed out.\n", cYellow, cReset, info.PID)
		fmt.Fprintf(os.Stderr, "         Check logs: %s\n\n", filepath.Join(root, "state", "daemon.log"))
		return
	}

	printDaemonReadyBanner(os.Stderr, root, info)
}

func cmdStop(args []string) {
	fs := flag.NewFlagSet("stop", flag.ExitOnError)
	runtimeRoot := fs.String("runtime-root", "", "runtime root directory")
	fs.Parse(args)

	_ = loadEnv()
	root := resolveRuntimeRoot(*runtimeRoot)

	info, alive := isDaemonAlive(root)
	if !alive {
		if info != nil {
			removeDaemonInfo(root)
			fmt.Fprintf(os.Stderr, "  %s[OK]%s Stale PID file removed.\n\n", cGreen, cReset)
		} else {
			fmt.Fprintf(os.Stderr, "  %sMnemosyneOS is not running.%s\n\n", cDim, cReset)
		}
		return
	}

	fmt.Fprintf(os.Stderr, "  %s⠿ Stopping MnemosyneOS (PID %d)...%s\n", cDim, info.PID, cReset)
	if err := stopDaemon(root); err != nil {
		fmt.Fprintf(os.Stderr, "  %s[ERROR]%s %v\n", cRed, cReset, err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "  %s[OK]%s Stopped.\n\n", cGreen, cReset)
}

func cmdRestart(args []string) {
	fs := flag.NewFlagSet("restart", flag.ExitOnError)
	addr := fs.String("addr", "", "listen address (default :8080)")
	runtimeRoot := fs.String("runtime-root", "", "runtime root directory")
	fs.Parse(args)

	_ = loadEnv()
	root := resolveRuntimeRoot(*runtimeRoot)

	if info, alive := isDaemonAlive(root); alive {
		fmt.Fprintf(os.Stderr, "  %s⠿ Stopping current daemon (PID %d)...%s\n", cDim, info.PID, cReset)
		_ = stopDaemon(root)
	}

	cmdStart([]string{"--addr", resolveListenAddr(*addr), "--runtime-root", root})
}

func cmdLogs(args []string) {
	fs := flag.NewFlagSet("logs", flag.ExitOnError)
	runtimeRoot := fs.String("runtime-root", "", "runtime root directory")
	lines := fs.Int("n", 50, "number of lines to show")
	fs.Parse(args)

	_ = loadEnv()
	root := resolveRuntimeRoot(*runtimeRoot)

	logPath := filepath.Join(root, "state", "daemon.log")
	data, err := os.ReadFile(logPath)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "  %sNo daemon log found.%s\n\n", cDim, cReset)
		} else {
			fmt.Fprintf(os.Stderr, "  %s[ERROR]%s %v\n", cRed, cReset, err)
		}
		return
	}

	allLines := strings.Split(string(data), "\n")
	start := len(allLines) - *lines
	if start < 0 {
		start = 0
	}
	for _, line := range allLines[start:] {
		fmt.Println(line)
	}
}

// ── helpers shared by daemon commands ─────────────────────────────────

func loadEnv() error {
	return config.LoadDefaultLocalEnv()
}

func resolveListenAddr(explicit string) string {
	if strings.TrimSpace(explicit) != "" {
		return strings.TrimSpace(explicit)
	}
	if v := strings.TrimSpace(os.Getenv("MNEMOSYNE_ADDR")); v != "" {
		return v
	}
	return ":8080"
}

func bootstrapIfNeeded(root string) {
	if _, err := os.Stat(root); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "  %s⠿ Bootstrapping runtime at %s%s\n", cDim, root, cReset)
		if _, initErr := runInit(initOptions{RuntimeRoot: root}); initErr != nil {
			fmt.Fprintf(os.Stderr, "  %s[ERROR]%s Bootstrap failed: %v\n", cRed, cReset, initErr)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "  %s[OK]%s Runtime initialized.\n\n", cGreen, cReset)
	}
}

func daemonStatusLine(runtimeRoot string) string {
	info, alive := isDaemonAlive(runtimeRoot)
	if !alive {
		return "stopped"
	}
	return fmt.Sprintf("running (PID %d, %s)", info.PID, "http://"+normalizeAddr(info.Addr))
}

// parseTailLines is used by cmdLogs; exported for testing if needed.
func parseTailLines(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 {
		return 50
	}
	return n
}
