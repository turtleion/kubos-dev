package essentials

import (
	"fmt"
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

func GetSandboxPath(givenName string) string {
	if !IsSandboxExists(givenName) {
		return ""
	}
	return filepath.Join("sandboxes", givenName)
}

// ClearColor removes ANSI escape sequences (such as terminal color codes) from a string.
// This ensures that logs written to plain-text files remain clean and readable.
func ClearColor(str string) string {
	// Regex pattern to match standard ANSI CSI (Control Sequence Introducer) sequences.
	const ansiRegexPattern = `\x1b\[[0-9;]*[a-zA-Z]`
	re := regexp.MustCompile(ansiRegexPattern)

	return re.ReplaceAllString(str, "")
}

// Parse takes a raw pacman command string and decodes its core intent.
func ParsePacmanCommand(command string) PacmanParserResult {
	var res PacmanParserResult
	res.Targets = []string{}
	res.Modifiers = []string{}

	tokens := strings.Fields(strings.TrimSpace(command))
	if len(tokens) > 0 && tokens[0] == "sudo" {
		tokens = tokens[1:]
	}

	if len(tokens) == 0 || tokens[0] != "pacman" {
		res.IsValid = false
		res.Summary = "Not a valid pacman command string."
		return res
	}

	res.IsValid = true
	var flagGroup string

	// Separate option flags from the targets
	for _, token := range tokens[1:] {
		if strings.HasPrefix(token, "--") {
			// Abaikan long-options atau masukkan ke penampung lain jika ingin diproses
			continue
		} else if strings.HasPrefix(token, "-") {
			// Menggunakan penggabungan (concatenation) agar flag sebelumnya tidak hilang
			flagGroup += strings.TrimLeft(token, "-")
		} else {
			res.Targets = append(res.Targets, token)
		}
	}

	var mainOp rune
	var modStr string
	for _, r := range flagGroup {
		if r >= 'A' && r <= 'Z' {
			mainOp = r
		} else if r >= 'a' && r <= 'z' {
			modStr += string(r)
			res.Modifiers = append(res.Modifiers, string(r))
		}
	}

	// Route the logic into clean, specific action terms
	switch mainOp {
	case 'S':
		res.Scope = "remote"
		switch {
		case strings.Contains(modStr, "y") && strings.Contains(modStr, "u"):
			res.Action = "system upgrade"
			res.Summary = "Refreshes package databases and upgrades all outdated system packages."
		case strings.Contains(modStr, "s"):
			res.Action = "search"
			res.Summary = "Searches online repositories for matching keywords."
		case strings.Contains(modStr, "w"):
			res.Action = "download only"
			res.Summary = "Downloads packages to the local cache without installing them."
		case strings.Contains(modStr, "i"):
			res.Action = "view info"
			res.Summary = "Displays information about packages in remote repositories."
		default:
			res.Action = "install"
			res.Summary = "Downloads and installs specified packages."
		}

	case 'R':
		res.Scope = "local"
		switch {
		case strings.Contains(modStr, "n") && strings.Contains(modStr, "s"):
			res.Action = "purge remove"
			res.Summary = "Removes packages, their unneeded dependencies, and configuration files."
		case strings.Contains(modStr, "s"):
			res.Action = "remove entire dependencies" // matches your 'remove entire dependencies' requirement
			res.Summary = "Removes packages and any unused dependencies."
		default:
			res.Action = "remove package"
			res.Summary = "Removes specified packages while keeping configurations and dependencies."
		}

	case 'Q':
		res.Scope = "local"
		switch {
		case strings.Contains(modStr, "d") && strings.Contains(modStr, "t"):
			res.Action = "list orphans"
			res.Summary = "Lists installed packages no longer required as dependencies."
		case strings.Contains(modStr, "e"):
			res.Action = "list explicit"
			res.Summary = "Lists all packages explicitly installed by the user."
		case strings.Contains(modStr, "l"):
			res.Action = "list files"
			res.Summary = "Lists every file installed on the system by the target package."
		default:
			res.Action = "query"
			res.Summary = "Queries the local package database."
		}

	default:
		res.IsValid = false
		res.Action = "unknown"
		res.Summary = fmt.Sprintf("Unsupported or unknown pacman base option: -%c", mainOp)
	}

	return res
}

// ToMap converts the PacmanParserResult into a clean, plain Go map[string]any.
func (r PacmanParserResult) ToMap() map[string]any {
	return map[string]any{
		"isValid":   r.IsValid,
		"action":    r.Action,
		"scope":     r.Scope,
		"summary":   r.Summary,
		"targets":   r.Targets,
		"modifiers": r.Modifiers,
	}
}

var conflictRe = regexp.MustCompile(
	`:: (\S+) and (\S+) are in conflict\. Remove (\S+)\? \[y/N\]`,
)

func ParseConflictingPackages(line string) (pkg1, pkg2, removeTarget string, ok bool) {
	m := conflictRe.FindStringSubmatch(line)
	if m == nil {
		return "", "", "", false
	}
	return m[1], m[2], m[3], true
}

// Regex untuk menangkap OSC sequence (dimulai dengan ESC ], diikuti teks, diakhiri ESC \)
var oscRegex = regexp.MustCompile(`\x1b\][^\x1b]*\x1b\\`)

func CleanTerminalEscapeCodes(input string) string {
	// Menghapus semua kode OSC sequence dari teks
	return oscRegex.ReplaceAllString(input, "")
}
