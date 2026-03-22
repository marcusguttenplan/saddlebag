package desk

import (
	"os"
	"path/filepath"
	"strings"
)

// ResolveTier indicates which tier resolved the active desk
type ResolveTier string

const (
	TierShell  ResolveTier = "shell"
	TierLocal  ResolveTier = "local"
	TierGlobal ResolveTier = "global"
	TierNone   ResolveTier = "none"
)

const DeskFileName = ".desk"

// FindDeskFile walks from startDir up to / looking for a .desk file.
// Returns the desk ID and the path where it was found, or empty strings if not found.
func FindDeskFile(startDir string) (deskID string, foundAt string) {
	dir := startDir
	for {
		candidate := filepath.Join(dir, DeskFileName)
		data, err := os.ReadFile(candidate)
		if err == nil {
			id := strings.TrimSpace(string(data))
			if id != "" {
				return id, candidate
			}
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached filesystem root
			return "", ""
		}
		dir = parent
	}
}

// WriteDeskFile creates a .desk file in the given directory
func WriteDeskFile(dir string, deskID string) error {
	path := filepath.Join(dir, DeskFileName)
	return os.WriteFile(path, []byte(deskID+"\n"), 0o644)
}
