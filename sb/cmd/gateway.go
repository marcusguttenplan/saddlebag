package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
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
	Short: "Start the model gateway on localhost",
	Long: `Start the Packmule model gateway — a localhost OpenAI-compatible HTTP proxy.

The gateway routes requests to the correct provider based on the active desk's
[llm] configuration. It records all calls to the token ledger and enforces the
daily budget limit.

The active desk is resolved via the normal desk resolution order:
  $SB_DESK env var → .desk file → working_dir match → default

API keys are resolved from environment variables:
  ANTHROPIC_API_KEY, OPENAI_API_KEY, GOOGLE_API_KEY

Ollama requires no key (local).

Examples:
  sb gateway start                    # start on default port (7474)
  sb gateway start --port 8080        # start on custom port
  sb gateway start --desk myproject   # use specific desk
`,
	RunE: runGatewayStart,
}

var gatewayStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show today's token ledger summary",
	RunE:  runGatewayStatus,
}

var (
	gatewayPort      string
	gatewayDesk      string
	gatewayWithOllama bool
)

func init() {
	gatewayStartCmd.Flags().StringVar(&gatewayPort, "port", "7474", "Port to listen on (localhost only)")
	gatewayStartCmd.Flags().StringVar(&gatewayDesk, "desk", "", "Desk to use (default: resolved from environment)")
	gatewayStartCmd.Flags().BoolVar(&gatewayWithOllama, "with-ollama", false, "Start ollama serve if not already running")

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

	// --- Ollama ---
	var ollamaMgr *gateway.OllamaManager
	if gatewayWithOllama {
		ollamaURL := "http://localhost:11434"
		if activeDeskConfig.LLM != nil && activeDeskConfig.LLM.Providers != nil {
			if cfg, ok := activeDeskConfig.LLM.Providers["ollama"]; ok && cfg.BaseURL != "" {
				ollamaURL = cfg.BaseURL
			}
		}
		ollamaMgr = gateway.NewOllamaManager(ollamaURL)
		ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
		defer cancel()
		if err := ollamaMgr.EnsureRunning(ctx); err != nil {
			return fmt.Errorf("ensuring ollama is running: %w", err)
		}
		if ollamaMgr.Managed() {
			logger.Printf("started ollama serve (managed subprocess)")
		} else {
			logger.Printf("ollama already running — not managing lifecycle")
		}
	}

	// --- Ledger ---
	l, err := ledger.New(config.LedgerDir())
	if err != nil {
		return fmt.Errorf("initializing ledger: %w", err)
	}

	// --- Secrets from environment ---
	secrets := &gateway.MapSecretResolver{
		Keys: map[string]string{
			"anthropic": os.Getenv("ANTHROPIC_API_KEY"),
			"openai":    os.Getenv("OPENAI_API_KEY"),
			"google":    os.Getenv("GOOGLE_API_KEY"),
		},
	}

	// --- Build server ---
	srv, err := gateway.New(gateway.Config{
		Desk:    activeDeskConfig,
		Ledger:  l,
		Secrets: secrets,
		Logger:  logger,
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

	// Print ready message after a brief startup delay
	time.Sleep(50 * time.Millisecond)
	fmt.Fprintf(os.Stderr, "\n  Gateway ready\n")
	fmt.Fprintf(os.Stderr, "  Endpoint : http://%s/v1/chat/completions\n", addr)
	if activeDeskConfig.DeskMeta.Name != "" {
		fmt.Fprintf(os.Stderr, "  Desk     : %s\n", activeDeskConfig.DeskMeta.Name)
	}
	if activeDeskConfig.LLM != nil {
		fmt.Fprintf(os.Stderr, "  Default  : %s/%s\n", activeDeskConfig.LLM.DefaultProvider, activeDeskConfig.LLM.DefaultModel)
		if activeDeskConfig.LLM.Budget != nil && activeDeskConfig.LLM.Budget.DailyLimitUSD > 0 {
			fmt.Fprintf(os.Stderr, "  Budget   : $%.2f/day\n", activeDeskConfig.LLM.Budget.DailyLimitUSD)
		}
		if activeDeskConfig.LLM.Providers != nil {
			if ollamaCfg, ok := activeDeskConfig.LLM.Providers["ollama"]; ok {
				fmt.Fprintf(os.Stderr, "  Ollama   : %s (models: %s)\n", ollamaCfg.BaseURL, strings.Join(ollamaCfg.Models, ", "))
			}
		}
	}
	// Ollama status line in banner
	if gatewayWithOllama && ollamaMgr != nil {
		if ollamaMgr.Managed() {
			fmt.Fprintf(os.Stderr, "  Ollama   : managed (started by sb)\n")
		} else {
			fmt.Fprintf(os.Stderr, "  Ollama   : external (already running)\n")
		}
	}
	fmt.Fprintf(os.Stderr, "  Ledger   : %s\n", config.LedgerDir())
	fmt.Fprintln(os.Stderr)

	<-stop
	logger.Println("shutting down...")
	if ollamaMgr != nil && ollamaMgr.Managed() {
		logger.Println("stopping ollama serve...")
		ollamaMgr.Stop()
	}
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
