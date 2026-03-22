package state

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/marcusguttenplan/sb/internal/config"
)

// State represents the live shared state written by Saddlebag.app
type State struct {
	ActiveDesk string `json:"activeDesk,omitempty"`
	AWSProfile string `json:"awsProfile,omitempty"`
	GCPConfig  string `json:"gcpConfig,omitempty"`
	GitEmail   string `json:"gitEmail,omitempty"`
	LastUpdated time.Time `json:"lastUpdated"`
}

// Read loads the current state from ~/.saddlebag/state.json
func Read() (*State, error) {
	data, err := os.ReadFile(config.StatePath())
	if err != nil {
		if os.IsNotExist(err) {
			return &State{}, nil
		}
		return nil, fmt.Errorf("reading state: %w", err)
	}

	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parsing state: %w", err)
	}
	return &s, nil
}

// Write persists state to ~/.saddlebag/state.json
func Write(s *State) error {
	s.LastUpdated = time.Now()

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding state: %w", err)
	}

	if err := os.MkdirAll(config.SaddlebagDir(), 0o755); err != nil {
		return fmt.Errorf("creating config dir: %w", err)
	}

	if err := os.WriteFile(config.StatePath(), data, 0o644); err != nil {
		return fmt.Errorf("writing state: %w", err)
	}
	return nil
}
