package secrets

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Secret represents a secret from a cloud secret manager
type Secret struct {
	Name      string            `json:"name"`
	Project   string            `json:"project"`
	Labels    map[string]string `json:"labels"`
	CreatedAt *time.Time        `json:"createTime,omitempty"`
}

// Label convention accessors
func (s *Secret) Org() string     { return s.Labels["org"] }
func (s *Secret) Service() string { return s.Labels["service"] }
func (s *Secret) Stage() string   { return s.Labels["stage"] }
func (s *Secret) Var() string     { return s.Labels["var"] }

// gcpSecretJSON matches the JSON output of `gcloud secrets list --format=json`
type gcpSecretJSON struct {
	Name       string            `json:"name"` // "projects/123/secrets/my-secret"
	CreateTime string            `json:"createTime,omitempty"`
	Labels     map[string]string `json:"labels,omitempty"`
}

func (g *gcpSecretJSON) secretName() string {
	parts := strings.Split(g.Name, "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return g.Name
}

// ListGCP lists all secrets in a GCP project
func ListGCP(project string) ([]Secret, error) {
	cmd := exec.Command("gcloud", "secrets", "list",
		"--project="+project,
		"--format=json",
	)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("gcloud secrets list: %w", err)
	}

	var raw []gcpSecretJSON
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("parsing secrets list: %w", err)
	}

	secrets := make([]Secret, 0, len(raw))
	for _, r := range raw {
		s := Secret{
			Name:    r.secretName(),
			Project: project,
			Labels:  r.Labels,
		}
		if r.CreateTime != "" {
			if t, err := time.Parse(time.RFC3339Nano, r.CreateTime); err == nil {
				s.CreatedAt = &t
			}
		}
		if s.Labels == nil {
			s.Labels = make(map[string]string)
		}
		secrets = append(secrets, s)
	}

	sort.Slice(secrets, func(i, j int) bool {
		return secrets[i].Name < secrets[j].Name
	})

	return secrets, nil
}

// GetValueGCP retrieves the latest value of a secret
func GetValueGCP(name, project string) (string, error) {
	cmd := exec.Command("gcloud", "secrets", "versions", "access", "latest",
		"--secret="+name,
		"--project="+project,
	)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("gcloud secrets versions access: %w", err)
	}
	return string(out), nil
}

// CreateGCP creates a new secret with labels and an initial value.
// If the secret already exists, it updates labels and adds a new version (upsert).
func CreateGCP(name, project, value string, labels map[string]string) error {
	// Build create command
	args := []string{"secrets", "create", name, "--project=" + project}
	if len(labels) > 0 {
		pairs := make([]string, 0, len(labels))
		for k, v := range labels {
			pairs = append(pairs, k+"="+v)
		}
		sort.Strings(pairs)
		args = append(args, "--labels="+strings.Join(pairs, ","))
	}

	createCmd := exec.Command("gcloud", args...)
	if out, err := createCmd.CombinedOutput(); err != nil {
		outStr := string(out)
		if strings.Contains(outStr, "already exists") {
			// Secret exists — update labels and add new version
			fmt.Fprintf(os.Stderr, "Secret %s already exists, updating...\n", name)
			if len(labels) > 0 {
				if err := UpdateLabelsGCP(name, project, labels); err != nil {
					return fmt.Errorf("updating labels: %w", err)
				}
			}
			if value != "" {
				return addVersionGCP(name, project, value)
			}
			return nil
		}
		return fmt.Errorf("creating secret: %s: %w", outStr, err)
	}

	// Add initial version
	return addVersionGCP(name, project, value)
}

// UpdateLabelsGCP updates labels on an existing secret
func UpdateLabelsGCP(name, project string, labels map[string]string) error {
	if len(labels) == 0 {
		return nil
	}

	pairs := make([]string, 0, len(labels))
	for k, v := range labels {
		pairs = append(pairs, k+"="+v)
	}
	sort.Strings(pairs)

	cmd := exec.Command("gcloud", "secrets", "update", name,
		"--project="+project,
		"--update-labels="+strings.Join(pairs, ","),
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("updating labels: %s: %w", string(out), err)
	}
	return nil
}

func addVersionGCP(name, project, value string) error {
	addCmd := exec.Command("gcloud", "secrets", "versions", "add", name,
		"--project="+project,
		"--data-file=-",
	)
	addCmd.Stdin = strings.NewReader(value)
	if out, err := addCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("adding secret version: %s: %w", string(out), err)
	}
	return nil
}

// FilterSecrets filters secrets by label values
func FilterSecrets(secrets []Secret, org, service, stage string) []Secret {
	var filtered []Secret
	for _, s := range secrets {
		if org != "" && s.Org() != org {
			continue
		}
		if service != "" && s.Service() != service {
			continue
		}
		if stage != "" && s.Stage() != stage {
			continue
		}
		filtered = append(filtered, s)
	}
	return filtered
}

// envFileGroup groups secrets by (service, stage) for env file generation
type envFileGroup struct {
	Service string
	Stage   string
	Vars    []envVar
}

type envVar struct {
	Name  string
	Value string
}

// GenerateEnvFiles writes .env files grouped by (service, stage)
// Output: basePath/service/.env.stage
func GenerateEnvFiles(secrets []Secret, basePath string, fetcher func(string, string) (string, error)) ([]string, error) {
	// Group by (service, stage)
	groups := make(map[string]*envFileGroup)
	for _, s := range secrets {
		svc := s.Service()
		stg := s.Stage()
		if svc == "" || stg == "" {
			continue
		}

		key := svc + ":" + stg
		if _, ok := groups[key]; !ok {
			groups[key] = &envFileGroup{Service: svc, Stage: stg}
		}

		value, err := fetcher(s.Name, s.Project)
		if err != nil {
			return nil, fmt.Errorf("fetching %s: %w", s.Name, err)
		}

		groups[key].Vars = append(groups[key].Vars, envVar{Name: s.Name, Value: value})
	}

	// Write files
	var written []string
	for _, g := range sortedGroups(groups) {
		dirPath := filepath.Join(basePath, g.Service)
		if err := os.MkdirAll(dirPath, 0o755); err != nil {
			return nil, fmt.Errorf("creating directory %s: %w", dirPath, err)
		}

		filePath := filepath.Join(dirPath, ".env."+g.Stage)

		sort.Slice(g.Vars, func(i, j int) bool {
			return g.Vars[i].Name < g.Vars[j].Name
		})

		var lines []string
		lines = append(lines, "# Generated by Saddlebag — do not edit")
		lines = append(lines, fmt.Sprintf("# service=%s stage=%s", g.Service, g.Stage))
		lines = append(lines, "")
		for _, v := range g.Vars {
			lines = append(lines, fmt.Sprintf("%s=%q", v.Name, v.Value))
		}
		lines = append(lines, "")

		if err := os.WriteFile(filePath, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
			return nil, fmt.Errorf("writing %s: %w", filePath, err)
		}
		written = append(written, filePath)
	}

	return written, nil
}

// FormatAsEnvFiles formats secrets as grouped env content (for clipboard / stdout)
func FormatAsEnvFiles(secrets []Secret, fetcher func(string, string) (string, error)) (string, error) {
	groups := make(map[string]*envFileGroup)
	for _, s := range secrets {
		svc := s.Service()
		stg := s.Stage()
		if svc == "" || stg == "" {
			continue
		}

		key := svc + ":" + stg
		if _, ok := groups[key]; !ok {
			groups[key] = &envFileGroup{Service: svc, Stage: stg}
		}

		value, err := fetcher(s.Name, s.Project)
		if err != nil {
			return "", fmt.Errorf("fetching %s: %w", s.Name, err)
		}

		groups[key].Vars = append(groups[key].Vars, envVar{Name: s.Name, Value: value})
	}

	var sections []string
	for _, g := range sortedGroups(groups) {
		sort.Slice(g.Vars, func(i, j int) bool {
			return g.Vars[i].Name < g.Vars[j].Name
		})

		var lines []string
		lines = append(lines, fmt.Sprintf("# --- %s/.env.%s ---", g.Service, g.Stage))
		for _, v := range g.Vars {
			lines = append(lines, fmt.Sprintf("%s=%q", v.Name, v.Value))
		}
		sections = append(sections, strings.Join(lines, "\n"))
	}

	return strings.Join(sections, "\n\n"), nil
}

func sortedGroups(groups map[string]*envFileGroup) []*envFileGroup {
	keys := make([]string, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	result := make([]*envFileGroup, 0, len(keys))
	for _, k := range keys {
		result = append(result, groups[k])
	}
	return result
}
