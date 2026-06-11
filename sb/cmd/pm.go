package cmd

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/marcusguttenplan/sb/internal/config"
)

// pmCmd is the parent for all `sb pm` subcommands.
var pmCmd = &cobra.Command{
	Use:   "pm",
	Short: "Manage the Packmule (pm) model gateway process",
	Long: `sb pm manages the lifecycle of the Packmule (pm) model gateway binary.

pm is the LLM router that sb's policy proxy delegates to. It runs as a
background process on localhost:7475.

Commands:
  sb pm install   Check that pm is installed and print installation hints
  sb pm start     Start pm serve as a background daemon
  sb pm stop      Stop the running pm process
  sb pm status    Show pm process status and health
  sb pm logs      Tail pm's log file`,
}

var pmInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Check pm is installed",
	RunE:  runPMInstall,
}

var pmStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start pm serve as a background daemon",
	RunE:  runPMStart,
}

var pmStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the running pm process",
	RunE:  runPMStop,
}

var pmStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show pm process status and health",
	RunE:  runPMStatus,
}

var pmLogsCmd = &cobra.Command{
	Use:   "logs",
	Short: "Tail pm's log file",
	RunE:  runPMLogs,
}

var (
	pmStartPort  string
	pmStartDesk  string
	pmLogsFollow bool
	pmLogsN      int
)

func init() {
	pmStartCmd.Flags().StringVar(&pmStartPort, "port", "7475", "Port for pm serve")
	pmStartCmd.Flags().StringVar(&pmStartDesk, "desk", "", "Desk to pass to pm serve")

	pmLogsCmd.Flags().BoolVarP(&pmLogsFollow, "follow", "f", true, "Follow log output")
	pmLogsCmd.Flags().IntVarP(&pmLogsN, "lines", "n", 50, "Number of recent lines to show")

	pmCmd.AddCommand(pmInstallCmd)
	pmCmd.AddCommand(pmStartCmd)
	pmCmd.AddCommand(pmStopCmd)
	pmCmd.AddCommand(pmStatusCmd)
	pmCmd.AddCommand(pmLogsCmd)
	rootCmd.AddCommand(pmCmd)
}

// ---------------------------------------------------------------------------
// sb pm install
// ---------------------------------------------------------------------------

func runPMInstall(_ *cobra.Command, _ []string) error {
	bin, err := exec.LookPath("pm")
	if err != nil {
		fmt.Println("pm is not installed or not in PATH.")
		fmt.Println()
		fmt.Println("To install pm, build it from source:")
		fmt.Println("  cd packmule/cli && make install")
		fmt.Println()
		fmt.Println("Or add its bin/ directory to your PATH.")
		return nil
	}
	fmt.Printf("pm found at: %s\n", bin)

	// Print version if pm responds to --version
	out, err := exec.Command(bin, "--version").Output()
	if err == nil {
		fmt.Printf("version: %s", string(out))
	}
	return nil
}

// ---------------------------------------------------------------------------
// sb pm start
// ---------------------------------------------------------------------------

func runPMStart(_ *cobra.Command, _ []string) error {
	bin, err := exec.LookPath("pm")
	if err != nil {
		return fmt.Errorf("pm not found in PATH — run `sb pm install` for instructions")
	}

	// If already running, report and exit cleanly.
	if pid, running := pmRunningPID(); running {
		fmt.Printf("pm is already running (pid %d)\n", pid)
		return nil
	}

	// Build pm serve args
	args := []string{"serve", "--port", pmStartPort}
	if pmStartDesk != "" {
		args = append(args, "--desk", pmStartDesk)
	} else if v := os.Getenv("SB_DESK"); v != "" {
		args = append(args, "--desk", v)
	}

	logPath := config.PMLogFile()
	pidPath := config.PMPIDFile()

	if err := os.MkdirAll(filepath.Dir(logPath), 0o750); err != nil {
		return fmt.Errorf("creating pm log dir: %w", err)
	}

	logFile, err := os.OpenFile(logPath, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o640)
	if err != nil {
		return fmt.Errorf("opening pm log file: %w", err)
	}
	defer logFile.Close()

	cmd := exec.Command(bin, args...)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	// SetsID detaches from the terminal so pm survives shell exit.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting pm: %w", err)
	}

	pid := cmd.Process.Pid

	// Write PID file
	if err := os.WriteFile(pidPath, []byte(strconv.Itoa(pid)+"\n"), 0o644); err != nil {
		return fmt.Errorf("writing PID file: %w", err)
	}

	// Release so the child process is not waited on by sb.
	_ = cmd.Process.Release()

	fmt.Printf("pm started (pid %d)\n", pid)
	fmt.Printf("  Logs : %s\n", logPath)
	fmt.Printf("  PID  : %s\n", pidPath)

	// Wait briefly for pm to become healthy.
	pmURL := "http://127.0.0.1:" + pmStartPort
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if pmHealthCheck(pmURL) {
			fmt.Printf("  URL  : %s/v1/chat/completions\n", pmURL)
			fmt.Println("  Status: healthy ✓")
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}

	fmt.Println("  Status: not yet healthy (still starting up — check logs)")
	return nil
}

// ---------------------------------------------------------------------------
// sb pm stop
// ---------------------------------------------------------------------------

func runPMStop(_ *cobra.Command, _ []string) error {
	pid, running := pmRunningPID()
	if !running {
		fmt.Println("pm is not running")
		return nil
	}

	proc, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("finding pm process (pid %d): %w", pid, err)
	}

	if err := proc.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("sending SIGTERM to pm (pid %d): %w", pid, err)
	}

	// Wait up to 5s for process to exit.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if err := proc.Signal(syscall.Signal(0)); err != nil {
			// Process no longer exists — clean up PID file.
			_ = os.Remove(config.PMPIDFile())
			fmt.Printf("pm stopped (was pid %d)\n", pid)
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}

	// Force kill if SIGTERM didn't work.
	_ = proc.Kill()
	_ = os.Remove(config.PMPIDFile())
	fmt.Printf("pm force-stopped (was pid %d)\n", pid)
	return nil
}

// ---------------------------------------------------------------------------
// sb pm status
// ---------------------------------------------------------------------------

func runPMStatus(_ *cobra.Command, _ []string) error {
	pid, running := pmRunningPID()
	if !running {
		fmt.Println("pm: not running")
		fmt.Printf("  PID file: %s\n", config.PMPIDFile())
		fmt.Println()
		fmt.Println("Start with: sb pm start")
		return nil
	}

	fmt.Printf("pm: running (pid %d)\n", pid)

	pmURL := "http://127.0.0.1:7475"
	if pmHealthCheck(pmURL) {
		fmt.Printf("  Health : %s/health — ok\n", pmURL)
	} else {
		fmt.Printf("  Health : %s/health — not responding\n", pmURL)
	}
	fmt.Printf("  Logs   : %s\n", config.PMLogFile())
	fmt.Printf("  PID    : %s\n", config.PMPIDFile())
	return nil
}

// ---------------------------------------------------------------------------
// sb pm logs
// ---------------------------------------------------------------------------

func runPMLogs(_ *cobra.Command, _ []string) error {
	logPath := config.PMLogFile()
	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		fmt.Printf("no log file found at %s\n", logPath)
		fmt.Println("Start pm first: sb pm start")
		return nil
	}

	if pmLogsFollow {
		// Tail -f: open file, seek to end, print new lines as they arrive.
		f, err := os.Open(logPath)
		if err != nil {
			return fmt.Errorf("opening log: %w", err)
		}
		defer f.Close()

		// First, show last pmLogsN lines.
		lines, err := lastNLines(f, pmLogsN)
		if err == nil {
			for _, l := range lines {
				fmt.Println(l)
			}
		}

		// Then follow.
		if _, err := f.Seek(0, io.SeekEnd); err != nil {
			return err
		}
		scanner := bufio.NewScanner(f)
		for {
			for scanner.Scan() {
				fmt.Println(scanner.Text())
			}
			time.Sleep(200 * time.Millisecond)
		}
	}

	// No follow: just print last n lines.
	f, err := os.Open(logPath)
	if err != nil {
		return fmt.Errorf("opening log: %w", err)
	}
	defer f.Close()
	lines, err := lastNLines(f, pmLogsN)
	if err != nil {
		return err
	}
	for _, l := range lines {
		fmt.Println(l)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// pmRunningPID reads the PID file and checks if the process is alive.
// Returns (pid, true) if alive, (0, false) otherwise.
func pmRunningPID() (int, bool) {
	data, err := os.ReadFile(config.PMPIDFile())
	if err != nil {
		return 0, false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || pid <= 0 {
		return 0, false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return 0, false
	}
	// Signal 0 checks if the process exists without affecting it.
	if err := proc.Signal(syscall.Signal(0)); err != nil {
		return 0, false
	}
	return pid, true
}

// pmHealthCheck returns true if pm's /health endpoint responds with 200.
func pmHealthCheck(baseURL string) bool {
	client := &http.Client{Timeout: 500 * time.Millisecond}
	resp, err := client.Get(baseURL + "/health")
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// lastNLines returns the last n lines from the given reader.
func lastNLines(r io.ReadSeeker, n int) ([]string, error) {
	if _, err := r.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	scanner := bufio.NewScanner(r)
	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
		if len(lines) > n*2 {
			lines = lines[len(lines)-n:]
		}
	}
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return lines, scanner.Err()
}
