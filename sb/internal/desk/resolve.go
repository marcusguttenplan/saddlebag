package desk

import (
	"os"
	"path/filepath"
	"strings"
)

// ResolveTier indicates which tier resolved the active desk
type ResolveTier string

const (
	TierShell   ResolveTier = "shell"
	TierLocal   ResolveTier = "local"
	TierWorkdir ResolveTier = "workdir"
	TierGlobal  ResolveTier = "global"
	TierNone    ResolveTier = "none"
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

// FindDeskByWorkingDir checks if the given directory is under any desk's configured workingDir.
// Returns the desk ID if found, empty string otherwise.
func FindDeskByWorkingDir(dir string) string {
	desks, err := LoadAll()
	if err != nil {
		return ""
	}

	// Normalize the search directory
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return ""
	}

	for id, d := range desks {
		wd := d.DeskMeta.WorkingDir
		if wd == "" {
			continue
		}
		// Expand ~ in workingDir
		if strings.HasPrefix(wd, "~/") {
			if home, err := os.UserHomeDir(); err == nil {
				wd = filepath.Join(home, wd[2:])
			}
		}
		absWD, err := filepath.Abs(wd)
		if err != nil {
			continue
		}
		// Check if dir is the workingDir or a subdirectory of it
		if absDir == absWD || strings.HasPrefix(absDir, absWD+string(filepath.Separator)) {
			return id
		}
	}
	return ""
}

// WriteDeskFile creates a .desk file in the given directory
func WriteDeskFile(dir string, deskID string) error {
	path := filepath.Join(dir, DeskFileName)
	return os.WriteFile(path, []byte(deskID+"\n"), 0o644)
}
