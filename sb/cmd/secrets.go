package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/marcusguttenplan/sb/internal/desk"
	"github.com/marcusguttenplan/sb/internal/secrets"
	"github.com/marcusguttenplan/sb/internal/state"
)

var (
	secretsProject  string
	secretsProvider string
	secretsOrg      string
	secretsService  string
	secretsStage    string
	secretsOutput   string
	secretsValue    string
)

var secretsCmd = &cobra.Command{
	Use:   "secrets",
	Short: "Manage cloud secrets (list, get, create, tag, generate env files)",
	Long: `Manage secrets from cloud secret managers.

The --project flag is optional. If omitted, the project is inferred from:
  1. Active desk's GCP config
  2. Active GCP config from ~/.saddlebag/state.json
  3. Explicit --project flag

Examples:
  sb secrets list
  sb secrets list --project=my-project --org=courseclear
  sb secrets get my-secret-name
  sb secrets create my-secret --value="s3cr3t" --org=cc --service=api --stage=prod
  sb secrets tag my-secret --org=cc --service=api --stage=prod
  sb secrets env --service=api --stage=prod --output=./
  sb secrets copy my-secret-name`,
}

var secretsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List secrets with their labels",
	RunE: func(cmd *cobra.Command, args []string) error {
		project, err := resolveProject()
		if err != nil {
			return err
		}

		all, err := secrets.ListGCP(project)
		if err != nil {
			return fmt.Errorf("listing secrets: %w", err)
		}

		// Apply filters
		filtered := secrets.FilterSecrets(all, secretsOrg, secretsService, secretsStage)

		if len(filtered) == 0 {
			fmt.Println("No secrets found")
			return nil
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "NAME\tORG\tSERVICE\tSTAGE\tVAR")
		fmt.Fprintln(w, "----\t---\t-------\t-----\t---")
		for _, s := range filtered {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
				s.Name,
				valueOrDash(s.Org()),
				valueOrDash(s.Service()),
				valueOrDash(s.Stage()),
				valueOrDash(s.Var()),
			)
		}
		w.Flush()

		fmt.Fprintf(os.Stderr, "\n%d secret(s) in %s\n", len(filtered), project)
		return nil
	},
}

var secretsGetCmd = &cobra.Command{
	Use:   "get <secret-name>",
	Short: "Print a secret's value to stdout",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		project, err := resolveProject()
		if err != nil {
			return err
		}

		value, err := secrets.GetValueGCP(args[0], project)
		if err != nil {
			return fmt.Errorf("getting secret: %w", err)
		}

		fmt.Print(value)
		return nil
	},
}

var secretsCreateCmd = &cobra.Command{
	Use:   "create <secret-name>",
	Short: "Create a new secret with labels (upserts if exists)",
	Long: `Create a new secret with labels and a value.

If the secret already exists, it updates the labels and adds a new version
with the provided value (upsert behavior).`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		project, err := resolveProject()
		if err != nil {
			return err
		}

		if secretsValue == "" {
			return fmt.Errorf("--value is required")
		}

		labels := make(map[string]string)
		if secretsOrg != "" {
			labels["org"] = strings.ToLower(secretsOrg)
		}
		if secretsService != "" {
			labels["service"] = strings.ToLower(secretsService)
		}
		if secretsStage != "" {
			labels["stage"] = strings.ToLower(secretsStage)
		}

		if err := secrets.CreateGCP(args[0], project, secretsValue, labels); err != nil {
			return fmt.Errorf("creating secret: %w", err)
		}

		fmt.Printf("Created secret %s in %s\n", args[0], project)
		if len(labels) > 0 {
			pairs := make([]string, 0, len(labels))
			for k, v := range labels {
				pairs = append(pairs, k+"="+v)
			}
			fmt.Printf("  labels: %s\n", strings.Join(pairs, ", "))
		}
		return nil
	},
}

var secretsEnvCmd = &cobra.Command{
	Use:   "env",
	Short: "Generate .env files from secrets grouped by service/stage labels",
	Long: `Generate .env files from secrets, using the label convention:
  org      → filter/grouping
  service  → becomes the directory
  stage    → becomes the file suffix (.env.stage)
  var      → becomes the env var name

Output: <output>/<service>/.env.<stage>

Example: sb secrets env --service=api --stage=prod --output=./`,
	RunE: func(cmd *cobra.Command, args []string) error {
		project, err := resolveProject()
		if err != nil {
			return err
		}

		all, err := secrets.ListGCP(project)
		if err != nil {
			return fmt.Errorf("listing secrets: %w", err)
		}

		filtered := secrets.FilterSecrets(all, secretsOrg, secretsService, secretsStage)
		if len(filtered) == 0 {
			fmt.Println("No secrets match the given filters")
			return nil
		}

		output := secretsOutput
		if output == "" {
			output = "."
		}

		written, err := secrets.GenerateEnvFiles(filtered, output, secrets.GetValueGCP)
		if err != nil {
			return fmt.Errorf("generating env files: %w", err)
		}

		if len(written) == 0 {
			fmt.Println("No secrets with service/stage/var labels found")
			return nil
		}

		for _, f := range written {
			fmt.Printf("  wrote %s\n", f)
		}
		fmt.Fprintf(os.Stderr, "\n%d file(s) generated\n", len(written))
		return nil
	},
}

var secretsCopyCmd = &cobra.Command{
	Use:   "copy <secret-name>",
	Short: "Copy a secret's value to the clipboard",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		project, err := resolveProject()
		if err != nil {
			return err
		}

		value, err := secrets.GetValueGCP(args[0], project)
		if err != nil {
			return fmt.Errorf("getting secret: %w", err)
		}

		// Pipe to pbcopy
		pbcopy := exec.Command("pbcopy")
		pbcopy.Stdin = strings.NewReader(value)
		if err := pbcopy.Run(); err != nil {
			return fmt.Errorf("copying to clipboard: %w", err)
		}

		fmt.Fprintf(os.Stderr, "Copied %s to clipboard\n", args[0])
		return nil
	},
}

var secretsTagCmd = &cobra.Command{
	Use:   "tag <secret-name>",
	Short: "Update labels on an existing secret",
	Long: `Update labels on an existing secret. Only specified labels are updated;
existing labels not mentioned are preserved.

Examples:
  sb secrets tag my-secret --org=courseclear --service=api --stage=prod
  sb secrets tag my-secret --stage=staging`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		project, err := resolveProject()
		if err != nil {
			return err
		}

		labels := make(map[string]string)
		if secretsOrg != "" {
			labels["org"] = strings.ToLower(secretsOrg)
		}
		if secretsService != "" {
			labels["service"] = strings.ToLower(secretsService)
		}
		if secretsStage != "" {
			labels["stage"] = strings.ToLower(secretsStage)
		}

		if len(labels) == 0 {
			return fmt.Errorf("at least one label flag is required (--org, --service, --stage)")
		}

		if err := secrets.UpdateLabelsGCP(args[0], project, labels); err != nil {
			return fmt.Errorf("updating labels: %w", err)
		}

		fmt.Printf("Updated labels on %s in %s\n", args[0], project)
		pairs := make([]string, 0, len(labels))
		for k, v := range labels {
			pairs = append(pairs, k+"="+v)
		}
		fmt.Printf("  labels: %s\n", strings.Join(pairs, ", "))
		return nil
	},
}

// resolveProject resolves the GCP project from flags, desk, or state
func resolveProject() (string, error) {
	// 1. Explicit flag
	if secretsProject != "" {
		// Check if it's an alias
		if resolved := resolveAlias(secretsProject); resolved != "" {
			return resolved, nil
		}
		return secretsProject, nil
	}

	// 2. Active desk's GCP config → resolve to project
	if deskID, _, _ := resolveCurrentDesk(); deskID != "" {
		desks, err := desk.LoadAll()
		if err == nil {
			if d, ok := desks[deskID]; ok && d.GCP != nil && d.GCP.Config != "" {
				if proj := gcpConfigProject(d.GCP.Config); proj != "" {
					return proj, nil
				}
			}
		}
	}

	// 3. Active GCP config from state.json
	if s, err := state.Read(); err == nil && s.GCPConfig != "" {
		if proj := gcpConfigProject(s.GCPConfig); proj != "" {
			return proj, nil
		}
	}

	return "", fmt.Errorf("no project specified and none could be inferred\n\nUse --project=PROJECT_ID or set an active desk/GCP config")
}

// resolveAlias checks config.json for project aliases
func resolveAlias(alias string) string {
	data, err := os.ReadFile(configPath())
	if err != nil {
		return ""
	}

	var cfg struct {
		ProjectAliases map[string]string `json:"projectAliases"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil || cfg.ProjectAliases == nil {
		return ""
	}

	return cfg.ProjectAliases[alias]
}

// gcpConfigProject resolves a gcloud config name to its project ID
func gcpConfigProject(configName string) string {
	cmd := exec.Command("gcloud", "config", "configurations", "describe", configName, "--format=value(properties.core.project)")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func configPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = os.Getenv("HOME")
	}
	return home + "/.saddlebag/config.json"
}

func valueOrDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

func init() {
	// Global flags for all secrets subcommands
	secretsCmd.PersistentFlags().StringVar(&secretsProject, "project", "", "GCP project ID (inferred if not set)")
	secretsCmd.PersistentFlags().StringVar(&secretsProvider, "provider", "gcp", "Cloud provider (gcp)")

	// Filter flags
	secretsListCmd.Flags().StringVar(&secretsOrg, "org", "", "Filter by org label")
	secretsListCmd.Flags().StringVar(&secretsService, "service", "", "Filter by service label")
	secretsListCmd.Flags().StringVar(&secretsStage, "stage", "", "Filter by stage label")

	// Create flags
	secretsCreateCmd.Flags().StringVar(&secretsValue, "value", "", "Secret value")
	secretsCreateCmd.Flags().StringVar(&secretsOrg, "org", "", "Org label")
	secretsCreateCmd.Flags().StringVar(&secretsService, "service", "", "Service label")
	secretsCreateCmd.Flags().StringVar(&secretsStage, "stage", "", "Stage label")

	// Env flags
	secretsEnvCmd.Flags().StringVar(&secretsOrg, "org", "", "Filter by org label")
	secretsEnvCmd.Flags().StringVar(&secretsService, "service", "", "Filter by service label")
	secretsEnvCmd.Flags().StringVar(&secretsStage, "stage", "", "Filter by stage label")
	secretsEnvCmd.Flags().StringVar(&secretsOutput, "output", ".", "Output base path")

	// Tag flags
	secretsTagCmd.Flags().StringVar(&secretsOrg, "org", "", "Org label")
	secretsTagCmd.Flags().StringVar(&secretsService, "service", "", "Service label")
	secretsTagCmd.Flags().StringVar(&secretsStage, "stage", "", "Stage label")

	// Wire up
	secretsCmd.AddCommand(secretsListCmd)
	secretsCmd.AddCommand(secretsGetCmd)
	secretsCmd.AddCommand(secretsCreateCmd)
	secretsCmd.AddCommand(secretsTagCmd)
	secretsCmd.AddCommand(secretsEnvCmd)
	secretsCmd.AddCommand(secretsCopyCmd)
	rootCmd.AddCommand(secretsCmd)
}
