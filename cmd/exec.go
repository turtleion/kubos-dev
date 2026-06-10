package cmd

import (
	"bufio"
	"fmt"
	"kubos/libraries/logger"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

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

	// Connect the host's standard I/O to the command so interactivity works
	stdin, _ := cmd.StdinPipe()
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()
	defer stdin.Close()
	defer stdout.Close()
	defer stderr.Close()
	// Search for a pacman conflict
	// Read the output line by line
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := scanner.Text()
		logger.Print(logger.LOG_SUCCESS, line, false, true)

		if scanner.Err() != nil {
			logger.Print(logger.LOG_ERROR, scanner.Err().Error(), false, true)
		}
	}

}

func Setup(givenName string) (string, error) {
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
	sandboxPath := filepath.Join("sandboxes", givenName)
	mergedPath := filepath.Join(sandboxPath, "merged")

	// 1. Unmount the overlay filesystem
	// We use sudo here because the mount was likely created with sudo/root privileges.
	logger.Print(logger.LOG_INFO, "Unmounting overlay for "+mergedPath, false, true)
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
