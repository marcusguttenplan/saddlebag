package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/marcusguttenplan/sb/internal/config"
)

var installHooks bool

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check setup and optionally install git hooks",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("🩺 Saddlebag Doctor\n")

		// Check ~/.saddlebag directory
		checkDir("Config directory", config.SaddlebagDir())
		checkDir("Desks directory", config.DesksDir())
		checkFile("State file", config.StatePath())
		checkFile("Config file", config.ConfigPath())

		// Check external tools
		checkBinary("aws")
		checkBinary("gcloud")
		checkBinary("git")
		checkBinary("ssh-add")

		// Check socket
		if _, err := os.Stat(config.SocketPath()); err == nil {
			fmt.Printf("✅  IPC socket      %s\n", config.SocketPath())
		} else {
			fmt.Printf("⚠️  IPC socket      %s (Saddlebag.app not running?)\n", config.SocketPath())
		}

		// Install hooks if requested
		if installHooks {
			fmt.Println("\n📎 Installing git hooks...")
			return installGitHooks()
		}

		fmt.Println("\nRun with --install-hooks to set up pre-commit git hooks")
		return nil
	},
}

func checkDir(label, path string) {
	if info, err := os.Stat(path); err == nil && info.IsDir() {
		fmt.Printf("✅  %-16s %s\n", label, path)
	} else {
		fmt.Printf("❌  %-16s %s (missing)\n", label, path)
	}
}

func checkFile(label, path string) {
	if _, err := os.Stat(path); err == nil {
		fmt.Printf("✅  %-16s %s\n", label, path)
	} else {
		fmt.Printf("⚠️  %-16s %s (not created yet)\n", label, path)
	}
}

func checkBinary(name string) {
	if path, err := exec.LookPath(name); err == nil {
		fmt.Printf("✅  %-16s %s\n", name, path)
	} else {
		fmt.Printf("❌  %-16s not found in PATH\n", name)
	}
}

func installGitHooks() error {
	// Find the git root
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").CombinedOutput()
	if err != nil {
		return fmt.Errorf("not in a git repository")
	}

	gitRoot := filepath.Join(string(out[:len(out)-1]), ".git", "hooks")
	hookPath := filepath.Join(gitRoot, "pre-commit")

	hookScript := `#!/bin/sh
# Saddlebag git identity guard
sb git-check || exit 1
`

	// Check if pre-commit hook already exists
	if _, err := os.Stat(hookPath); err == nil {
		// Read existing hook to check if it already has our line
		existing, _ := os.ReadFile(hookPath)
		if contains(string(existing), "sb git-check") {
			fmt.Printf("✅  pre-commit hook already installed at %s\n", hookPath)
			return nil
		}
		// Append to existing hook
		f, err := os.OpenFile(hookPath, os.O_APPEND|os.O_WRONLY, 0o755)
		if err != nil {
			return fmt.Errorf("appending to hook: %w", err)
		}
		defer f.Close()
		_, err = f.WriteString("\n# Saddlebag git identity guard\nsb git-check || exit 1\n")
		if err != nil {
			return fmt.Errorf("writing hook: %w", err)
		}
		fmt.Printf("📎  Appended to existing pre-commit hook at %s\n", hookPath)
	} else {
		// Create new hook
		if err := os.MkdirAll(gitRoot, 0o755); err != nil {
			return fmt.Errorf("creating hooks dir: %w", err)
		}
		if err := os.WriteFile(hookPath, []byte(hookScript), 0o755); err != nil {
			return fmt.Errorf("writing hook: %w", err)
		}
		fmt.Printf("📎  Created pre-commit hook at %s\n", hookPath)
	}

	return nil
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func init() {
	doctorCmd.Flags().BoolVar(&installHooks, "install-hooks", false, "Install pre-commit git hooks")
	rootCmd.AddCommand(doctorCmd)
}
