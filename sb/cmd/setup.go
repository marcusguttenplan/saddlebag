package cmd

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

const stubFileName = ".zsh.sb"
const sourceLineMarker = "# saddlebag shell integration"

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Install shell integration into ~/.zsh.sb and link from ~/.zshrc",
	Long: `Creates ~/.zsh.sb with the Saddlebag shell integration script,
and adds a source line to ~/.zshrc if not already present.

This replaces the need for 'eval "$(sb init zsh)"' in your shell config.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("getting home dir: %w", err)
		}

		stubPath := filepath.Join(home, stubFileName)
		zshrcPath := filepath.Join(home, ".zshrc.local")

		// 1. Write ~/.zsh.sb
		if err := os.WriteFile(stubPath, []byte(shellScript), 0o644); err != nil {
			return fmt.Errorf("writing %s: %w", stubPath, err)
		}
		fmt.Printf("✓ Wrote %s\n", stubPath)

		// 2. Resolve symlinks for the target rc file
		resolvedPath := zshrcPath
		if target, err := filepath.EvalSymlinks(zshrcPath); err == nil {
			resolvedPath = target
			if resolvedPath != zshrcPath {
				fmt.Printf("  (resolved symlink → %s)\n", resolvedPath)
			}
		}

		// 3. Add source line if not present
		sourceLine := fmt.Sprintf("%s\nsource ~/%s\n", sourceLineMarker, stubFileName)

		existing, err := os.ReadFile(resolvedPath)
		if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("reading %s: %w", resolvedPath, err)
		}

		content := string(existing)
		if strings.Contains(content, sourceLineMarker) {
			fmt.Printf("✓ %s already sources %s\n", resolvedPath, stubFileName)
		} else {
			// Append to the resolved path (not the symlink) — no O_CREATE
			flags := os.O_APPEND | os.O_WRONLY
			if os.IsNotExist(err) {
				flags |= os.O_CREATE
			}
			f, err := os.OpenFile(resolvedPath, flags, 0o644)
			if err != nil {
				return fmt.Errorf("opening %s: %w", resolvedPath, err)
			}
			defer f.Close()

			if len(content) > 0 && !strings.HasSuffix(content, "\n") {
				f.WriteString("\n")
			}
			f.WriteString("\n" + sourceLine)
			fmt.Printf("✓ Added source line to %s\n", resolvedPath)
		}

		fmt.Println("\nRestart your shell or run: source ~/.zshrc")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(setupCmd)
}
