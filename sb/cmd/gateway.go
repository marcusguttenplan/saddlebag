package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
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
	Short: "Start the saddlebag policy proxy on localhost",
	Long: `Start the saddlebag policy proxy — a localhost HTTP server that sits in
front of the Packmule gateway (pm serve) and enforces policy.

The proxy:
  - Resolves desk routing metadata and injects it as X-Sb-* headers
  - Performs Cedar authorization (Phase 1a: allow-all stub)
  - Reverse-proxies authorized requests to pm (default: http://localhost:7475)
  - Writes an audit trail row to ~/.saddlebag/ledger/

Start pm serve first, then start sb gateway:
  pm serve &
  sb gateway start

Or point at an existing pm instance:
  sb gateway start --packmule-url http://localhost:7475

Examples:
  sb gateway start                              # default :7474 → pm :7475
  sb gateway start --port 8080                  # custom sb port
  sb gateway start --desk myproject             # use specific desk
  sb gateway start --packmule-url http://...    # non-default pm URL
`,
	RunE: runGatewayStart,
}

var gatewayStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show today's token ledger summary",
	RunE:  runGatewayStatus,
}

var (
	gatewayPort       string
	gatewayDesk       string
	gatewayPackmuleURL string
)

func init() {
	gatewayStartCmd.Flags().StringVar(&gatewayPort, "port", "7474", "Port to listen on (localhost only)")
	gatewayStartCmd.Flags().StringVar(&gatewayDesk, "desk", "", "Desk to use (default: resolved from environment)")
	gatewayStartCmd.Flags().StringVar(&gatewayPackmuleURL, "packmule-url", "http://localhost:7475", "Packmule pm serve URL")

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
		PackmuleURL: gatewayPackmuleURL,
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
	fmt.Fprintf(os.Stderr, "\n  Saddlebag policy proxy ready\n")
	fmt.Fprintf(os.Stderr, "  Endpoint : http://%s/v1/chat/completions\n", addr)
	fmt.Fprintf(os.Stderr, "  Upstream : %s\n", gatewayPackmuleURL)
	fmt.Fprintf(os.Stderr, "  Health   : http://%s/health\n", addr)
	if activeDeskConfig.DeskMeta.Name != "" {
		fmt.Fprintf(os.Stderr, "  Desk     : %s\n", activeDeskConfig.DeskMeta.Name)
	}
	fmt.Fprintf(os.Stderr, "  Ledger   : %s\n", config.LedgerDir())
	fmt.Fprintln(os.Stderr)

	<-stop
	logger.Println("shutting down...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(ctx)
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

	// Pretty-print JSON too for piping
	if os.Getenv("SB_JSON") != "" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(s)
	}

	return nil
}
