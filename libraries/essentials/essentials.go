package essentials

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// Sandbox represents an active container environment, storing its unique identifier and root filesystem path.

// IsValidSandbox verifies if the specified directory follows the required OverlayFS structure
// (upper, merged, and work directories) and confirms the merged path is an active mount.
func IsValidSandbox(sandboxPath string) bool {
	// OverlayFS requires an 'upper' directory for new files, a 'work' directory for
	// metadata/atomicity, and a 'merged' directory to serve as the mount point.
	subDirs := []string{"upper", "merged", "work"}
	for _, dir := range subDirs {
		if _, err := os.Stat(filepath.Join(sandboxPath, dir)); os.IsNotExist(err) {
			return false
		}
	}

	// Check if the merged directory is an overlay filesystem
	// -n: no headings, -o FSTYPE: only output fstype, -t overlay: filter by type
	mergedPath := filepath.Join(sandboxPath, "merged")
	cmd := exec.Command("findmnt", "-n", "-o", "FSTYPE", "-t", "overlay", mergedPath)
	output, err := cmd.Output()
	if err != nil {
		return false
	}

	return strings.TrimSpace(string(output)) == "overlay"
}

// GetSandboxes scans the local 'sandboxes' root directory and returns a slice
// containing metadata for all active and structurally valid sandboxes.
func GetSandboxes() []Sandbox {
	var sandboxes []Sandbox
	entries, err := os.ReadDir("sandboxes")
	if err != nil {
		return sandboxes
	}

	for _, entry := range entries {
		if entry.IsDir() {
			sandboxPath := filepath.Join("sandboxes", entry.Name())
			if IsValidSandbox(sandboxPath) {
				sandboxes = append(sandboxes, Sandbox{
					Name: entry.Name(),
					Path: sandboxPath,
				})
			}
		}
	}
	return sandboxes
}

// IsSandboxExists determines if a sandbox with the specified name is currently active and valid.
func IsSandboxExists(givenName string) bool {
	if sandboxes := GetSandboxes(); slices.ContainsFunc(sandboxes, func(s Sandbox) bool {
		return s.Name == givenName
	}) {
		return true
	} else {
		return false

	}
}

// ClearColor removes ANSI escape sequences (such as terminal color codes) from a string.
// This ensures that logs written to plain-text files remain clean and readable.
func ClearColor(str string) string {
	// Regex pattern to match standard ANSI CSI (Control Sequence Introducer) sequences.
	const ansiRegexPattern = `\x1b\[[0-9;]*[a-zA-Z]`
	re := regexp.MustCompile(ansiRegexPattern)

	return re.ReplaceAllString(str, "")
}
