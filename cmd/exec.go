package cmd

import (
	"bufio"
	"fmt"
	"kubos/libraries/essentials"
	"kubos/libraries/logger"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

func CleanUp() {
	// Get the list of sandboxes that are currently valid/active
	validSandboxes := essentials.GetSandboxes()
	validMap := make(map[string]bool)
	for _, s := range validSandboxes {
		validMap[s.Name] = true
	}

	// Read the physical sandboxes directory
	entries, err := os.ReadDir("sandboxes")
	if err != nil {
		return // Directory doesn't exist yet, nothing to clean
	}

	for _, entry := range entries {
		if entry.IsDir() && !validMap[entry.Name()] {
			_ = os.RemoveAll(filepath.Join("sandboxes", entry.Name()))
		}
	}
}

// DeBusyPath identifies and kills processes that are accessing the given path.
// This is particularly useful when an unmount fails because the target is busy.
func DeBusyPath(path string) {
	logger.Print(logger.LOG_INFO, "Checking for busy processes on: "+path, false, true)

	// fuser returns PIDs to stdout. We use sudo to ensure we see processes from all users.
	// Note: fuser returns a non-zero exit code if no processes are found, which we ignore.
	cmd := exec.Command("sudo", "fuser", path)
	output, _ := cmd.Output()

	// fuser output is typically "path: PID1[suffix] PID2[suffix] ..."
	// We use a regex to extract only the numeric PIDs.
	re := regexp.MustCompile(`\d+`)
	pids := re.FindAllString(string(output), -1)

	if len(pids) == 0 {
		return
	}

	for _, pid := range pids {
		logger.Print(logger.LOG_WARNING, fmt.Sprintf("Killing process %s keeping %s busy", pid, path), false, true)
		_ = exec.Command("sudo", "kill", "-9", pid).Run()
	}
	// Give the kernel a small window to release the file handles
	time.Sleep(200 * time.Millisecond)
}

// Spawn executes a command within a systemd-nspawn container.
// It handles command parsing and ensures interactive I/O is preserved.
func Spawn(containerPath string, command string) {
	// Split the command string into separate arguments (e.g., ["pacman", "-S", "hello"])
	cmdParts := strings.Fields(command)
	if len(cmdParts) == 0 {
		logger.Print(logger.LOG_ERROR, "No command provided to Spawn", false, true)
		return
	}

	// Combine nspawn flags with the split command
	args := append([]string{"-D", containerPath}, cmdParts...)
	cmd := exec.Command("systemd-nspawn", args...)

	// Pipe stderr to the host so we see error messages in real-time
	cmd.Stderr = os.Stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		logger.Print(logger.LOG_ERROR, "Failed to create stdout pipe: "+err.Error(), false, true)
		return
	}

	// Start the command
	if err := cmd.Start(); err != nil {
		logger.Print(logger.LOG_ERROR, "Failed to start process: "+err.Error(), false, true)
		return
	}

	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := scanner.Text()
		logger.Print(logger.LOG_SUCCESS, line, false, true)
	}

	// Wait for the command to finish and catch the exit error
	if err := cmd.Wait(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			// This shows the exit code (e.g., 1, 127)
			logger.Print(logger.LOG_ERROR, fmt.Sprintf("Command failed with exit code %d", exitErr.ExitCode()), false, true)
		} else {
			logger.Print(logger.LOG_ERROR, "Command execution failed: "+err.Error(), false, true)
		}
	}
}

func Setup(givenName string) (string, error) {
	if essentials.IsSandboxExists(givenName) {
		logger.Print(logger.LOG_WARNING, "Sandbox already exists: "+givenName, false, true)
		return "", fmt.Errorf("sandbox %s already exists", givenName)
	}
	// Check if the needed directory exist
	logger.Print(logger.LOG_INFO, "Trying to create a directory on path: sandboxes/"+givenName, false, true)
	if _, err := os.Stat(filepath.Join("sandboxes", givenName)); os.IsNotExist(err) {
		err := os.MkdirAll(filepath.Join("sandboxes", givenName), 0755)
		if err != nil {
			logger.Print(logger.LOG_ERROR, err.Error(), false, true)
			return "", err
		}
		logger.Print(logger.LOG_SUCCESS, "Found desired directory on path: sandboxes/"+givenName, false, true)

		// Now create the subdirectories for the overlay filesystem
		logger.Print(logger.LOG_INFO, "Trying to create directories on path: sandboxes/"+givenName+"/upper, sandboxes/"+givenName+"/merged, sandboxes/"+givenName+"/work", false, true)

		subDirs := []string{"upper", "merged", "work"}
		for _, dir := range subDirs {
			fullPath := filepath.Join("sandboxes", givenName, dir)
			err := os.MkdirAll(fullPath, 0755)
			if err != nil {
				logger.Print(logger.LOG_ERROR, fmt.Sprintf("Error creating subdirectory %s: %v", fullPath, err), false, true)
				return "", err
			}
		}
		logger.Print(logger.LOG_SUCCESS, "Successfully created directories on path: sandboxes/"+givenName+"/upper, sandboxes/"+givenName+"/merged, sandboxes/"+givenName+"/work", false, true)

		logger.Print(logger.LOG_INFO, "Mounting overlay filesystem on path: sandboxes/"+givenName+"/merged", false, true)

		parts := strings.Fields("sudo mount -t overlay overlay -o lowerdir=/,upperdir=" + filepath.Join("sandboxes", givenName, "upper") + ",workdir=" + filepath.Join("sandboxes", givenName, "work") + " " + filepath.Join("sandboxes", givenName, "merged"))
		cmd := exec.Command(parts[0], parts[1:]...)

		// Run the command and capture both Standard Output and Standard Error
		output, err := cmd.CombinedOutput()

		// Check if the command succeeded
		if err != nil {
			// Command failed (returned non-zero exit code)
			logger.Print(logger.LOG_ERROR, string(output), false, true)
			return string(output), fmt.Errorf("command failed: %w", err)
		}

		logger.Print(logger.LOG_SUCCESS, "Successfully mounted overlay filesystem on path: sandboxes/"+givenName+"/merged", false, true)

	}
	// If the directory already exists or was successfully created,
	// we should return the path to the sandbox.
	// Assuming the function should return the path to the base sandbox directory.
	return filepath.Join("sandboxes", givenName), nil
}

// Teardown cleans up the sandbox by unmounting the overlay and removing the directories.
func Teardown(givenName string) error {
	if !essentials.IsSandboxExists(givenName) {
		logger.Print(logger.LOG_WARNING, "Sandbox does not exist: "+givenName, false, true)
		return fmt.Errorf("sandbox %s does not exist", givenName)
	}
	sandboxPath := filepath.Join("sandboxes", givenName)
	mergedPath := filepath.Join(sandboxPath, "merged")

	// 1. Unmount the overlay filesystem
	// We use sudo here because the mount was likely created with sudo/root privileges.
	logger.Print(logger.LOG_INFO, "Unmounting overlay for "+mergedPath, false, true)

	DeBusyPath(mergedPath)

	umountCmd := exec.Command("sudo", "umount", mergedPath)
	output, err := umountCmd.CombinedOutput()
	if err != nil {
		// If it's already unmounted, we might want to continue anyway to clean up files
		logger.Print(logger.LOG_WARNING, fmt.Sprintf("Unmount failed (it might already be unmounted): %s", string(output)), false, true)
	} else {
		logger.Print(logger.LOG_SUCCESS, "Successfully unmounted overlay for "+givenName, false, true)
	}

	// 2. Remove the sandbox directory structure
	logger.Print(logger.LOG_INFO, "Removing sandbox directories: "+sandboxPath, false, true)
	err = os.RemoveAll(sandboxPath)
	if err != nil {
		logger.Print(logger.LOG_ERROR, "Failed to remove sandbox directory: "+err.Error(), false, true)
		return err
	}

	logger.Print(logger.LOG_SUCCESS, "Cleaned up sandbox directories for "+givenName, false, true)
	return nil
}
