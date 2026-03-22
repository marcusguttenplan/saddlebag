package health

import (
	"fmt"
	"os/exec"
	"strings"
)

// Status represents the health of a single credential source
type Status struct {
	Service string
	OK      bool
	Detail  string
}

// Emoji returns a colored indicator for the status
func (s Status) Emoji() string {
	if s.OK {
		return "🟢"
	}
	return "🔴"
}

// CheckSSHAgent checks if the SSH agent has keys loaded
func CheckSSHAgent() Status {
	out, err := exec.Command("ssh-add", "-l").CombinedOutput()
	if err != nil {
		return Status{Service: "SSH Agent", OK: false, Detail: "no keys loaded"}
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	count := 0
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			count++
		}
	}

	return Status{
		Service: "SSH Agent",
		OK:      count > 0,
		Detail:  fmt.Sprintf("%d key(s) loaded", count),
	}
}

// CheckGCloudAuth checks if gcloud has a valid auth session
func CheckGCloudAuth() Status {
	cmd := exec.Command("gcloud", "auth", "print-access-token")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return Status{Service: "GCP Auth", OK: false, Detail: "not authenticated"}
	}

	token := strings.TrimSpace(string(out))
	if token == "" || strings.Contains(token, "ERROR") {
		return Status{Service: "GCP Auth", OK: false, Detail: "token invalid"}
	}

	return Status{Service: "GCP Auth", OK: true, Detail: "authenticated"}
}

// CheckAWSSSO checks if AWS SSO tokens are present (basic check)
func CheckAWSSSO() Status {
	cmd := exec.Command("aws", "sts", "get-caller-identity", "--output", "json")
	_, err := cmd.CombinedOutput()
	if err != nil {
		return Status{Service: "AWS SSO", OK: false, Detail: "session expired or not logged in"}
	}

	return Status{Service: "AWS SSO", OK: true, Detail: "session active"}
}
