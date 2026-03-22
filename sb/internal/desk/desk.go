package desk

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"

	"github.com/marcusguttenplan/sb/internal/config"
)

// Desk represents a context bundle loaded from a TOML file
type Desk struct {
	DeskMeta DeskMeta          `toml:"desk"`
	AWS      *AWSConfig        `toml:"aws,omitempty"`
	GCP      *GCPConfig        `toml:"gcp,omitempty"`
	Git      *GitConfig        `toml:"git,omitempty"`
	SSH      *SSHConfig        `toml:"ssh,omitempty"`
	Env      map[string]string `toml:"env,omitempty"`
}

type DeskMeta struct {
	Name string `toml:"name"`
}

type AWSConfig struct {
	Profile string `toml:"profile"`
}

type GCPConfig struct {
	Config string `toml:"config"`
}

type GitConfig struct {
	Email string `toml:"email"`
	Name  string `toml:"name"`
}

type SSHConfig struct {
	Key string `toml:"key"`
}

// ID returns the desk identifier (filename without extension)
func (d *Desk) ID(filename string) string {
	return strings.TrimSuffix(filepath.Base(filename), ".toml")
}

// LoadAll reads all desk definitions from ~/.saddlebag/desks/
func LoadAll() (map[string]*Desk, error) {
	desksDir := config.DesksDir()

	entries, err := os.ReadDir(desksDir)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]*Desk{}, nil
		}
		return nil, fmt.Errorf("reading desks dir: %w", err)
	}

	desks := make(map[string]*Desk)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".toml") {
			continue
		}

		d, err := LoadFile(filepath.Join(desksDir, entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("loading desk %s: %w", entry.Name(), err)
		}

		id := strings.TrimSuffix(entry.Name(), ".toml")
		desks[id] = d
	}

	return desks, nil
}

// LoadFile reads a single desk TOML file
func LoadFile(path string) (*Desk, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}

	var d Desk
	if err := toml.Unmarshal(data, &d); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}

	return &d, nil
}
