package util

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"syscall"

	"kubos/internal/exec/exectypes"
)

// Sandbox represents an active container environment, storing its unique identifier and root filesystem path.

// IsValidSandbox verifies if the specified directory follows the required OverlayFS structure
// (upper, merged, and work directories) and confirms the merged path is an active mount.
type ValidSandboxCode int8

const (
	VALID_SANDBOX ValidSandboxCode = iota
	DIR_EXIST_INVALID_SANDBOX
	INVALID_SANDBOX
	DIRMOUNT_EXIST_INVALID_SANDBOX
)

func IsValidSandbox(sandboxPath string) ValidSandboxCode {
	info, err := os.Stat(sandboxPath)
	if os.IsNotExist(err) {
		return INVALID_SANDBOX
	}
	if err != nil || !info.IsDir() {
		return INVALID_SANDBOX
	}

	dirsOK := true
	for _, dir := range []string{"upper", "work", "merged"} {
		p := filepath.Join(sandboxPath, dir)
		info, err := os.Stat(p)
		if err != nil || !info.IsDir() {
			dirsOK = false
		}
	}

	mergedPath := filepath.Join(sandboxPath, "merged")
	out, err := exec.Command("findmnt", "-n", "-o", "FSTYPE", mergedPath).Output()
	isOverlay := err == nil && strings.TrimSpace(string(out)) == "overlay"

	switch {
	case dirsOK && isOverlay:
		return VALID_SANDBOX
	case isOverlay:
		// dirs incomplete but mount is still live — caller MUST unmount before cleanup
		return DIRMOUNT_EXIST_INVALID_SANDBOX
	case dirsOK:
		return DIR_EXIST_INVALID_SANDBOX // dirs fine, mount missing/wrong fstype
	default:
		return INVALID_SANDBOX
	}
}

// GetSandboxes scans the local 'sandboxes' root directory and returns a slice
// containing metadata for all active and structurally valid sandboxes.
func GetSandboxes() []exectypes.Sandbox {
	var sandboxes []exectypes.Sandbox
	entries, err := os.ReadDir("sandboxes")
	if err != nil {
		return sandboxes
	}

	for _, entry := range entries {
		if entry.IsDir() {
			sandboxPath := filepath.Join("sandboxes", entry.Name())
			if IsValidSandbox(sandboxPath) == VALID_SANDBOX {
				sandboxes = append(sandboxes, exectypes.Sandbox{
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
	if sandboxes := GetSandboxes(); slices.ContainsFunc(sandboxes, func(s exectypes.Sandbox) bool {
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
func ParsePacmanCommand(command string) exectypes.PacmanParserResult {
	var res exectypes.PacmanParserResult
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

// isEOF cek syscall.EIO karena PTY return EIO bukan io.EOF saat proses mati
func IsEOF(err error) bool {
	var errno syscall.Errno
	if errors.As(err, &errno) {
		return errno == syscall.EIO
	}
	return false
}

func ParsePacmanPkgName(s string) (name, version, release string, ok bool) {
	last := strings.LastIndex(s, "-")
	if last == -1 {
		return "", "", "", false
	}

	release = s[last+1:]
	left := s[:last]

	prev := strings.LastIndex(left, "-")
	if prev == -1 {
		return "", "", "", false
	}

	name = left[:prev]
	version = left[prev+1:]

	if name == "" || version == "" || release == "" {
		return "", "", "", false
	}
	fmt.Println("SANITIZED NAME: ", name)
	return name, version, release, true
}
