package test

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// LdconfigError represents a library issue found by ldconfig
type LdconfigError struct {
	Line   string
	IsHard bool // true = exit non-zero or "ERROR:", false = warning
}

func (e LdconfigError) String() string {
	kind := "warning"
	if e.IsHard {
		kind = "error"
	}
	return fmt.Sprintf("[ldconfig %s] %s", kind, e.Line)
}

// RunLdconfigInNspawn runs ldconfig inside the nspawn container and
// returns any library errors/warnings found.
func RunLdconfigInNspawn(root string) ([]LdconfigError, error) {
	var stdout, stderr bytes.Buffer

	cmd := exec.Command(
		"sudo",
		"systemd-nspawn",
		"--link-journal=no",
		"-D", root, // container root
		"ldconfig", "-v", // -v makes it print what it's processing
	)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	exitCode := 0
	if exitErr, ok := err.(*exec.ExitError); ok {
		exitCode = exitErr.ExitCode()
	} else if err != nil {
		return nil, fmt.Errorf("failed to launch nspawn: %w", err)
	}

	issues := parseIdconfigOutput(stdout.String(), stderr.String(), exitCode != 0)
	// fmt.Println("LDCONF ISSUE: ", stdout.String())
	return issues, nil
}

// parseIdconfigOutput scans ldconfig's stderr for errors and warnings,
// filtering out nspawn's own noise lines.
func parseIdconfigOutput(rawStdout string, rawStderr string, hardFail bool) []LdconfigError {
	var issues []LdconfigError

	process := func(line string) {
		line = strings.TrimSpace(line)
		if line == "" {
			return
		}

		// Filter nspawn metadata noise
		if isNspawnNoise(line) {
			return
		}

		issue := LdconfigError{Line: line}

		switch {
		case hardFail:
			issue.IsHard = true
		case isLdconfigWarning(line):
			issue.IsHard = false
		case isLdconfigError(line):
			issue.IsHard = true

		default:
			return // informational line, skip
		}

		issues = append(issues, issue)
	}

	for _, line := range strings.Split(rawStderr, "\n") {
		process(line)
	}

	for _, line := range strings.Split(rawStdout, "\n") {
		process(line)
	}

	return issues
}

// isLdconfigError detects hard error patterns in ldconfig output
func isLdconfigError(line string) bool {
	hardPrefixes := []string{
		"ERROR:",
		"ldconfig: Can't",
		"ldconfig: Cannot",
		"ldconfig: Fatal",
		"ldconfig: /", // path errors like "ldconfig: /usr/lib/foo.so is not a symbolic link"
		"error while loading shared libraries",
		"is empty, not checked", // CRITICAL: Corrupted/zero-byte files
		"is truncated",          // CRITICAL: Broken file transfers
		"too small to contain",  // CRITICAL: Malformed ELF headers
		"permission denied",     // CRITICAL: Severe ACL/chmod issues
	}
	lower := strings.ToLower(line)
	for _, p := range hardPrefixes {
		if strings.Contains(lower, strings.ToLower(p)) {
			return true
		}
	}
	return false
}

// isLdconfigWarning detects non-fatal warning patterns
func isLdconfigWarning(line string) bool {
	warnPrefixes := []string{
		"warning:",
		"not a symlink",
		"is not a symbolic link",
		"ignoring",
		"skipping",
		"is a relative path", // Configuration warnings
	}
	lower := strings.ToLower(line)
	for _, p := range warnPrefixes {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
}

// isNspawnNoise filters systemd-nspawn status lines
// (fallback if --quiet doesn't suppress everything)
func isNspawnNoise(line string) bool {
	noisePrefixes := []string{
		"Spawning container",
		"Press ^] three times",
		"Machine ",
		"Detected virtualization",
		"Detected architecture",
		"Set hostname",
		"systemd-nspawn",
	}
	for _, p := range noisePrefixes {
		if strings.HasPrefix(line, p) {
			return true
		}
	}
	return false
}
