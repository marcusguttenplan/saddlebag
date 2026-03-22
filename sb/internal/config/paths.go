package config

import (
	"os"
	"path/filepath"
)

// SaddlebagDir returns the path to ~/.saddlebag/
func SaddlebagDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.Getenv("HOME"), ".saddlebag")
	}
	return filepath.Join(home, ".saddlebag")
}

// StatePath returns the path to ~/.saddlebag/state.json
func StatePath() string {
	return filepath.Join(SaddlebagDir(), "state.json")
}

// ConfigPath returns the path to ~/.saddlebag/config.json
func ConfigPath() string {
	return filepath.Join(SaddlebagDir(), "config.json")
}

// DesksDir returns the path to ~/.saddlebag/desks/
func DesksDir() string {
	return filepath.Join(SaddlebagDir(), "desks")
}

// SocketPath returns the Unix socket path for IPC with Saddlebag.app
func SocketPath() string {
	return "/tmp/saddlebag.sock"
}

// AWSConfigPath returns the path to ~/.aws/config
func AWSConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.Getenv("HOME"), ".aws", "config")
	}
	return filepath.Join(home, ".aws", "config")
}
