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
	"slices"
	"strings"
	"time"
)

func CleanUp() essentials.ExecutionResult {
	// Get the list of sandboxes that are currently valid/active
	validSandboxes := essentials.GetSandboxes()
	validMap := make(map[string]bool)
	for _, s := range validSandboxes {
		validMap[s.Name] = true
	}

	// Read the physical sandboxes directory
	entries, err := os.ReadDir("sandboxes")
	if err != nil {
		return essentials.ExecutionResult{Code: essentials.EXECUTION_TASK_FAIL, Message: err.Error()} // Directory doesn't exist yet, nothing to clean
	}

	for _, entry := range entries {
		if entry.IsDir() && !validMap[entry.Name()] {
			_ = os.RemoveAll(filepath.Join("sandboxes", entry.Name()))
		}
	}
	return essentials.ExecutionResult{Code: essentials.EXECUTION_TASK_SUCCESS, Message: ""}
}

// DeBusyPath identifies and kills processes that are accessing the given path.
// This is particularly useful when an unmount fails because the target is busy.
func DeBusyPath(path string) essentials.ExecutionResult {
	logger.Print(essentials.LOG_INFO, "Checking for busy processes on: "+path, false, true)

	// fuser returns PIDs to stdout. We use sudo to ensure we see processes from all users.
	// Note: fuser returns a non-zero exit code if no processes are found, which we ignore.
	cmd := exec.Command("sudo", "fuser", path)
	output, _ := cmd.Output()

	// fuser output is typically "path: PID1[suffix] PID2[suffix] ..."
	// We use a regex to extract only the numeric PIDs.
	re := regexp.MustCompile(`\d+`)
	pids := re.FindAllString(string(output), -1)

	if len(pids) == 0 {
		return essentials.ExecutionResult{Code: essentials.EXECUTION_TASK_COMPLETED, Message: ""}
	}

	for _, pid := range pids {
		logger.Print(essentials.LOG_WARNING, fmt.Sprintf("Killing process %s keeping %s busy", pid, path), false, true)
		_ = exec.Command("sudo", "kill", "-9", pid).Run()
	}
	// Give the kernel a small window to release the file handles
	time.Sleep(200 * time.Millisecond)
	return essentials.ExecutionResult{Code: essentials.EXECUTION_TASK_SUCCESS, Message: ""}
}

// Spawn executes a command within a systemd-nspawn container.
// It handles command parsing and ensures interactive I/O is preserved.
func Spawn(container string, command string) essentials.ExecutionResult {
	// Split the command string into separate arguments (e.g., ["pacman", "-S", "hello"])
	cmdParts := strings.Fields(command)
	if len(cmdParts) == 0 {
		logger.Print(essentials.LOG_ERROR, "No command provided to Spawn", false, true)
		return essentials.ExecutionResult{Code: essentials.EXECUTION_TASK_FAIL, Message: "No command provided to Spawn"}
	}

	containerPath := essentials.GetSandboxPath(container)
	if containerPath == "" {
		return essentials.ExecutionResult{Code: essentials.EXECUTION_TASK_FAIL, Message: fmt.Sprintf("sandbox %s does not exist", container)}
	}

	// Identify the command
	parsedPacmanResult := essentials.ParsePacmanCommand(command)
	if parsedPacmanResult.IsValid == false {
		return essentials.ExecutionResult{Code: essentials.EXECUTION_TASK_FAIL, Message: "The command is not valid pacman command"}
	}

	// Combine nspawn flags with the split command
	args := append([]string{"systemd-nspawn", "-D", filepath.Join(containerPath, "merged")}, cmdParts...)
	cmd := exec.Command("sudo", args...)

	// Pipe stderr to the host so we see error messages in real-time
	cmd.Stderr = os.Stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		logger.Print(essentials.LOG_ERROR, "Failed to create stdout pipe: "+err.Error(), false, true)
		return essentials.ExecutionResult{Code: essentials.EXECUTION_TASK_FAIL, Message: "Failed to create stdout pipe: " + err.Error()}
	}

	// Start the command
	if err := cmd.Start(); err != nil {
		logger.Print(essentials.LOG_ERROR, "Failed to start process: "+err.Error(), false, true)
		return essentials.ExecutionResult{Code: essentials.EXECUTION_TASK_FAIL, Message: "Failed to start process: " + err.Error()}
	}

	// Flag to track if a conflicting package message was detected
	conflictDetected := false

	if parsedPacmanResult.Action == "install" {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := scanner.Text()
			// Check for conflicting packages
			if strings.Contains(strings.ToLower(line), "are in conflict") {
				logger.Print(essentials.LOG_ERROR, "Conflicting package detected: "+line, false, true)
				conflictDetected = true
			} else {
				// Log general output as info during installation
				logger.Print(essentials.LOG_INFO, line, false, true)
			}
		}
		// Check for any errors that occurred during scanning
		if err := scanner.Err(); err != nil {
			logger.Print(essentials.LOG_ERROR, "Error reading command output: "+err.Error(), false, true)
			return essentials.ExecutionResult{Code: essentials.EXECUTION_TASK_FAIL, Message: "Error reading command output: " + err.Error()}
		}
	}

	// Wait for the command to finish and catch the exit error
	if err := cmd.Wait(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			// This shows the exit code (e.g., 1, 127)
			logger.Print(essentials.LOG_ERROR, fmt.Sprintf("Command failed with exit code %d", exitErr.ExitCode()), false, true)
		} else {
			logger.Print(essentials.LOG_ERROR, "Command execution failed: "+err.Error(), false, true)
		}
	}

	if conflictDetected {
		logger.Print(essentials.LOG_WARNING, "Conflicting package found during sandbox installation. Would you like to commit change to your real system?", false, true)
	}
	return essentials.ExecutionResult{Code: essentials.EXECUTION_TASK_SUCCESS, Message: ""} // If command succeeded and no conflict detected
}

func SpawnPacman(args []string, pkgName string) essentials.ExecutionResult {
	logger.Print(essentials.LOG_INFO, "==> Redirecting to KUBOS pacman..", false, true)
	args = slices.Insert(args, 0, "pacman")
	logger.Print(essentials.LOG_INFO, "Setting up closed environment..", false, true)
	if res := Setup(pkgName); res.Code != essentials.EXECUTION_TASK_SUCCESS { // Ensure sandbox is set up before spawning pacman
		return res
	}
	return Spawn(pkgName, strings.Join(args, " ")) // Correctly return the result of the Spawn call
}

func SpawnPassthroughPacman(args []string) essentials.ExecutionResult {
	logger.Print(essentials.LOG_INFO, "==> Redirecting to PASSTHROUGH pacman..", false, true)
	args = slices.Insert(args, 0, "pacman")
	cmd := exec.Command("sudo", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return essentials.ExecutionResult{Code: essentials.EXECUTION_TASK_SUCCESS, Message: cmd.Run().Error()}
}

func SpawnYAY(args []string) essentials.ExecutionResult {
	fmt.Printf("==> forwarding to YAY: yay %v\n", args)
	cmd := exec.Command("yay", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return essentials.ExecutionResult{Code: essentials.EXECUTION_TASK_SUCCESS, Message: cmd.Run().Error()}
}

func Setup(givenName string) essentials.ExecutionResult {
	if essentials.IsSandboxExists(givenName) {
		logger.Print(essentials.LOG_WARNING, "Sandbox already exists: "+givenName, false, true)
		return essentials.ExecutionResult{Code: essentials.EXECUTION_TASK_FAIL, Message: fmt.Sprintf("sandbox %s already exists", givenName)}

	}
	// Check if the needed directory exist
	logger.Print(essentials.LOG_INFO, "Trying to create a directory on path: sandboxes/"+givenName, false, true)
	if _, err := os.Stat(filepath.Join("sandboxes", givenName)); os.IsNotExist(err) {
		err := os.MkdirAll(filepath.Join("sandboxes", givenName), 0755)
		if err != nil {
			logger.Print(essentials.LOG_ERROR, err.Error(), false, true)
			return essentials.ExecutionResult{Code: essentials.EXECUTION_TASK_FAIL, Message: err.Error()}

		}
		logger.Print(essentials.LOG_SUCCESS, "Found desired directory on path: sandboxes/"+givenName, false, true)

		// Now create the subdirectories for the overlay filesystem
		logger.Print(essentials.LOG_INFO, "Trying to create directories on path: sandboxes/"+givenName+"/upper, sandboxes/"+givenName+"/merged, sandboxes/"+givenName+"/work", false, true)

		subDirs := []string{"upper", "merged", "work"}
		for _, dir := range subDirs {
			fullPath := filepath.Join("sandboxes", givenName, dir)
			err := os.MkdirAll(fullPath, 0755)
			if err != nil {
				logger.Print(essentials.LOG_ERROR, fmt.Sprintf("Error creating subdirectory %s: %v", fullPath, err), false, true)
				return essentials.ExecutionResult{Code: essentials.EXECUTION_TASK_FAIL, Message: err.Error()}
			}
		}
		logger.Print(essentials.LOG_SUCCESS, "Successfully created directories on path: sandboxes/"+givenName+"/upper, sandboxes/"+givenName+"/merged, sandboxes/"+givenName+"/work", false, true)

		logger.Print(essentials.LOG_INFO, "Mounting overlay filesystem on path: sandboxes/"+givenName+"/merged", false, true)

		parts := strings.Fields("sudo mount -t overlay overlay -o lowerdir=/,upperdir=" + filepath.Join("sandboxes", givenName, "upper") + ",workdir=" + filepath.Join("sandboxes", givenName, "work") + " " + filepath.Join("sandboxes", givenName, "merged"))
		cmd := exec.Command(parts[0], parts[1:]...)

		// Run the command and capture both Standard Output and Standard Error
		output, err := cmd.CombinedOutput()

		// Check if the command succeeded
		if err != nil {
			// Command failed (returned non-zero exit code)
			logger.Print(essentials.LOG_ERROR, string(output), false, true)
			return essentials.ExecutionResult{Code: essentials.EXECUTION_TASK_FAIL, Message: fmt.Sprintf("Failed with output: %s and error: %s", output, err.Error())}

		}

		logger.Print(essentials.LOG_SUCCESS, "Successfully mounted overlay filesystem on path: sandboxes/"+givenName+"/merged", false, true)

	}
	// If the directory already exists or was successfully created,
	// we should return the path to the sandbox.
	// Assuming the function should return the path to the base sandbox directory.
	return essentials.ExecutionResult{Code: essentials.EXECUTION_TASK_SUCCESS, Message: filepath.Join("sandboxes", givenName)}

}

// Teardown cleans up the sandbox by unmounting the overlay and removing the directories.
func Teardown(givenName string) essentials.ExecutionResult {
	if !essentials.IsSandboxExists(givenName) {
		logger.Print(essentials.LOG_WARNING, "Sandbox does not exist: "+givenName, false, true)
		return essentials.ExecutionResult{Code: essentials.EXECUTION_TASK_FAIL, Message: fmt.Sprintf("sandbox %s does not exist", givenName)}
	}
	sandboxPath := filepath.Join("sandboxes", givenName)
	mergedPath := filepath.Join(sandboxPath, "merged")

	// 1. Unmount the overlay filesystem
	// We use sudo here because the mount was likely created with sudo/root privileges.
	logger.Print(essentials.LOG_INFO, "Unmounting overlay for "+mergedPath, false, true)

	DeBusyPath(mergedPath)

	umountCmd := exec.Command("sudo", "umount", mergedPath)
	output, err := umountCmd.CombinedOutput()
	if err != nil {
		// If it's already unmounted, we might want to continue anyway to clean up files
		logger.Print(essentials.LOG_WARNING, fmt.Sprintf("Unmount failed (it might already be unmounted): %s", string(output)), false, true)
	} else {
		logger.Print(essentials.LOG_SUCCESS, "Successfully unmounted overlay for "+givenName, false, true)
	}

	// 2. Remove the sandbox directory structure
	logger.Print(essentials.LOG_INFO, "Removing sandbox directories: "+sandboxPath, false, true)
	err = os.RemoveAll(sandboxPath)
	if err != nil {
		logger.Print(essentials.LOG_ERROR, "Failed to remove sandbox directory: "+err.Error(), false, true)
		return essentials.ExecutionResult{Code: essentials.EXECUTION_TASK_FAIL, Message: "Failed to remove sandbox directory: " + err.Error()}
	}

	logger.Print(essentials.LOG_SUCCESS, "Cleaned up sandbox directories for "+givenName, false, true)
	return essentials.ExecutionResult{Code: essentials.EXECUTION_TASK_SUCCESS, Message: ""}

}
