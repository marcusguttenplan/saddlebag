package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/marcusguttenplan/sb/internal/config"
	"github.com/marcusguttenplan/sb/internal/desk"
	"github.com/marcusguttenplan/sb/internal/gateway"
	"github.com/marcusguttenplan/sb/internal/ledger"
)

var gatewayCmd = &cobra.Command{
	Use:   "gateway",
	Short: "Manage the Packmule model gateway",
}

var gatewayStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the saddlebag policy proxy (and optionally pm) on localhost",
	Long: `Start the saddlebag policy proxy — a localhost HTTP server that sits in
front of the Packmule gateway (pm serve) and enforces policy.

The proxy:
  - Resolves desk routing metadata and injects it as X-Sb-* headers
  - Performs Cedar authorization (Phase 1a: allow-all stub)
  - Reverse-proxies authorized requests to pm (default: http://localhost:7475)
  - Writes an audit trail row to ~/.saddlebag/ledger/

Use --with-pm to start both pm and the sb proxy together in a single command.
Both processes shut down cleanly when you press Ctrl-C.

Examples:
  sb gateway start                              # proxy only (pm must already run)
  sb gateway start --with-pm                   # start pm + sb proxy together
  sb gateway start --with-pm --with-ollama     # full local stack (pm + Ollama + sb)
  sb gateway start --with-pm --desk myproject  # with a specific desk
  sb gateway start --port 8080                 # custom sb proxy port
  sb gateway start --packmule-url http://...   # non-default pm URL
`,
	RunE: runGatewayStart,
}

var gatewayStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show today's token ledger summary",
	RunE:  runGatewayStatus,
}

var (
	gatewayPort        string
	gatewayDesk        string
	gatewayPackmuleURL string
	gatewayWithPM      bool
	gatewayPMPort      string
	gatewayWithOllama  bool
)

func init() {
	gatewayStartCmd.Flags().StringVar(&gatewayPort, "port", "7474", "Port for the sb policy proxy (localhost only)")
	gatewayStartCmd.Flags().StringVar(&gatewayDesk, "desk", "", "Desk to use (default: resolved from environment)")
	gatewayStartCmd.Flags().StringVar(&gatewayPackmuleURL, "packmule-url", "http://localhost:7475", "Packmule pm serve URL")
	gatewayStartCmd.Flags().BoolVar(&gatewayWithPM, "with-pm", false, "Start pm serve alongside sb gateway (managed subprocess)")
	gatewayStartCmd.Flags().StringVar(&gatewayPMPort, "pm-port", "7475", "Port for pm serve when --with-pm is set")
	gatewayStartCmd.Flags().BoolVar(&gatewayWithOllama, "with-ollama", false, "Pass --with-ollama to pm serve (requires --with-pm)")

	gatewayCmd.AddCommand(gatewayStartCmd)
	gatewayCmd.AddCommand(gatewayStatusCmd)
	rootCmd.AddCommand(gatewayCmd)
}

func runGatewayStart(cmd *cobra.Command, args []string) error {
	logger := log.New(os.Stderr, "[gateway] ", log.LstdFlags)

	// --- Resolve desk ---
	var activeDeskName string
	if gatewayDesk != "" {
		activeDeskName = gatewayDesk
	} else if v := os.Getenv("SB_DESK"); v != "" {
		activeDeskName = v
	}

	var activeDeskConfig *desk.Desk
	if activeDeskName != "" {
		desks, err := desk.LoadAll()
		if err != nil {
			return fmt.Errorf("loading desks: %w", err)
		}
		if d, ok := desks[activeDeskName]; ok {
			activeDeskConfig = d
			logger.Printf("using desk %q", activeDeskConfig.DeskMeta.Name)
		} else {
			logger.Printf("desk %q not found, starting with no desk config", activeDeskName)
		}
	}
	if activeDeskConfig == nil {
		activeDeskConfig = &desk.Desk{}
	}

	// --- Optionally start pm serve as a managed subprocess ---
	var pmProc *os.Process
	pmURL := gatewayPackmuleURL

	if gatewayWithPM {
		var err error
		pmURL, pmProc, err = startPMSubprocess(logger, gatewayPMPort, activeDeskName, gatewayWithOllama)
		if err != nil {
			return err
		}
		defer func() {
			if pmProc != nil {
				logger.Println("stopping pm serve...")
				_ = pmProc.Signal(syscall.SIGTERM)
				// Give pm 5s to exit cleanly, then force-kill.
				done := make(chan struct{})
				go func() {
					pmProc.Wait() //nolint:errcheck
					close(done)
				}()
				select {
				case <-done:
				case <-time.After(5 * time.Second):
					_ = pmProc.Kill()
				}
			}
		}()
	}

	// --- Audit ledger ---
	l, err := ledger.New(config.LedgerDir())
	if err != nil {
		return fmt.Errorf("initializing ledger: %w", err)
	}

	// --- Build proxy server ---
	srv, err := gateway.New(gateway.Config{
		Desk:        activeDeskConfig,
		Ledger:      l,
		Logger:      logger,
		PackmuleURL: pmURL,
	})
	if err != nil {
		return fmt.Errorf("creating gateway: %w", err)
	}

	addr := "127.0.0.1:" + gatewayPort

	// --- Graceful shutdown ---
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	go func() {
		if err := srv.ListenAndServe(addr); err != nil && err != http.ErrServerClosed {
			logger.Printf("fatal: %v", err)
			os.Exit(1)
		}
	}()

	time.Sleep(50 * time.Millisecond)
	printBanner(addr, pmURL, activeDeskConfig, gatewayWithPM, gatewayWithOllama)

	<-stop
	logger.Println("shutting down sb proxy...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(ctx)
}

// startPMSubprocess launches `pm serve` as a child process, waits for it to
// become healthy, and returns its URL and process handle.
// withOllama passes --with-ollama through to pm so pm manages the Ollama lifecycle.
func startPMSubprocess(logger *log.Logger, port, deskName string, withOllama bool) (pmURL string, proc *os.Process, err error) {
	pmBin, err := exec.LookPath("pm")
	if err != nil {
		return "", nil, fmt.Errorf("pm not found in PATH — install pm first (cd packmule/cli && make install): %w", err)
	}

	pmURL = "http://127.0.0.1:" + port

	// If pm is already running on that port, reuse it.
	if pmHealthCheck(pmURL) {
		logger.Printf("pm already running at %s — skipping launch", pmURL)
		return pmURL, nil, nil
	}

	args := []string{"serve", "--port", port}
	if deskName != "" {
		args = append(args, "--desk", deskName)
	} else if v := os.Getenv("SB_DESK"); v != "" {
		args = append(args, "--desk", v)
	}
	if withOllama {
		args = append(args, "--with-ollama")
	}

	pmCmd := exec.Command(pmBin, args...)
	pmCmd.Stdout = os.Stderr // pm logs → same stderr stream as sb
	pmCmd.Stderr = os.Stderr

	if err := pmCmd.Start(); err != nil {
		return "", nil, fmt.Errorf("starting pm serve: %w", err)
	}

	proc = pmCmd.Process
	logger.Printf("started pm serve (pid %d) at %s", proc.Pid, pmURL)

	// Wait up to 10s for pm to become healthy
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if pmHealthCheck(pmURL) {
			return pmURL, proc, nil
		}
		time.Sleep(200 * time.Millisecond)
	}

	// Timed out — kill and bail
	_ = proc.Kill()
	return "", nil, fmt.Errorf("pm serve did not become healthy within 10s — check pm logs")
}

func printBanner(addr, pmURL string, d *desk.Desk, withPM, withOllama bool) {
	fmt.Fprintln(os.Stderr)
	if withPM {
		fmt.Fprintf(os.Stderr, "  ✦ Packmule stack ready")
		if withOllama {
			fmt.Fprintf(os.Stderr, " (+ Ollama)")
		}
		fmt.Fprintln(os.Stderr)
	} else {
		fmt.Fprintf(os.Stderr, "  Saddlebag policy proxy ready\n")
	}
	fmt.Fprintf(os.Stderr, "  Endpoint : http://%s/v1/chat/completions\n", addr)
	fmt.Fprintf(os.Stderr, "  Upstream : %s\n", pmURL)
	fmt.Fprintf(os.Stderr, "  Health   : http://%s/health\n", addr)
	if d.DeskMeta.Name != "" {
		fmt.Fprintf(os.Stderr, "  Desk     : %s\n", d.DeskMeta.Name)
	}
	fmt.Fprintf(os.Stderr, "  Ledger   : %s\n", config.LedgerDir())
	fmt.Fprintln(os.Stderr)
}

func runGatewayStatus(cmd *cobra.Command, args []string) error {
	l, err := ledger.New(config.LedgerDir())
	if err != nil {
		return fmt.Errorf("opening ledger: %w", err)
	}

	s, err := l.TodaySummary()
	if err != nil {
		return fmt.Errorf("reading ledger: %w", err)
	}

	if s.CallCount == 0 {
		fmt.Println("No calls recorded today.")
		return nil
	}

	fmt.Printf("Today's gateway usage (%s)\n", s.Date)
	fmt.Printf("  Calls      : %d (%d errors)\n", s.CallCount, s.ErrorCount)
	fmt.Printf("  Input      : %d tokens\n", s.TotalInputTokens)
	fmt.Printf("  Output     : %d tokens\n", s.TotalOutputTokens)
	fmt.Printf("  Cost       : $%.4f\n", s.TotalCostUSD)

	if len(s.ByModel) > 0 {
		fmt.Println("\nBy model:")
		for _, m := range s.ByModel {
			fmt.Printf("  %-45s  calls=%-3d  in=%-7d  out=%-7d  $%.4f\n",
				m.Provider+"/"+m.Model, m.CallCount, m.InputTokens, m.OutputTokens, m.CostUSD)
		}
	}

	if os.Getenv("SB_JSON") != "" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(s)
	}

	return nil
}
