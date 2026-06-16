package cmd

import (
	"fmt"
	"io"
	"kubos/libraries/essentials"
	"kubos/libraries/logger"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/creack/pty"
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
	// Check if there is command provided to Spawn
	if len(cmdParts) == 0 {
		logger.Print(essentials.LOG_ERROR, "No command provided to Spawn", false, true)
		return essentials.ExecutionResult{Code: essentials.EXECUTION_TASK_FAIL, Message: "No command provided to Spawn"}
	}
	// Check if the sandbox 'container' really exists
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

	ptmx, err := pty.Start(cmd)
	if err != nil {
		logger.Print(essentials.LOG_ERROR, "Failed to start process with PTY: "+err.Error(), false, true)
		return essentials.ExecutionResult{Code: essentials.EXECUTION_TASK_FAIL, Message: "Failed to start PTY: " + err.Error()}
	}
	defer ptmx.Close()

	go func() {
		_, _ = io.Copy(ptmx, os.Stdin)
	}()
	// Flag to track if a conflicting package message was detected
	conflictingPackages := make(essentials.ConflictingPackages)
	fmt.Println("\nParsed Pacman Result: ")
	fmt.Println(parsedPacmanResult)
	fmt.Printf("\n")
	if parsedPacmanResult.Action == "install" {
		// Replace your bufio.NewScanner(ptmx) block with a raw byte buffer read loop

		buf := make([]byte, 1024)
		var lineBuffer strings.Builder

		// Helper to process a line for conflicts and logging
		processLine := func(line string) {
			// Clear CSI (colors) and OSC escape codes for reliable regex matching
			cleanLine := strings.TrimSpace(essentials.ClearColor(essentials.CleanTerminalEscapeCodes(line)))
			if cleanLine == "" {
				return
			}

			pkgName, conflictedWith, _, status := essentials.ParseConflictingPackages(cleanLine)
			if status || strings.Contains(strings.ToLower(cleanLine), "are in conflict") {
				//logger.Print(essentials.LOG_ERROR, "Conflicting package detected: "+cleanLine, false, false)
				conflictingPackages[pkgName] = []string{conflictedWith, strconv.FormatBool(status)}
			} else {
				// Normal output log
				logger.Print(essentials.LOG_INFO, line, true, true)
			}
		}

		for {
			n, err := ptmx.Read(buf)
			if n > 0 {
				chunk := essentials.CleanTerminalEscapeCodes(string(buf[:n]))
				fmt.Print(chunk)
				lineBuffer.WriteString(chunk)

				// Cek prompt konflik SEBELUM nunggu newline
				current := lineBuffer.String()
				if strings.Contains(strings.ToLower(current), "are in conflict") && strings.Contains(current, "[y/N]") {
					// Jawab otomatis
					processLine(current)
					_, err := ptmx.Write([]byte("y\n"))
					if err != nil {
						logger.Print(essentials.LOG_ERROR, "Could not write answer into stdin. Please input yes.", false, true)
					}
					lineBuffer.Reset()
				}

				if strings.Contains(lineBuffer.String(), "\n") {
					fullText := lineBuffer.String()
					lines := strings.Split(fullText, "\n")
					lineBuffer.Reset()
					if !strings.HasSuffix(fullText, "\n") {
						lineBuffer.WriteString(lines[len(lines)-1])
						lines = lines[:len(lines)-1]
					}

					for _, line := range lines {
						processLine(line)
					}
				}
			}
			if err != nil {
				if err != io.EOF {
					logger.Print(essentials.LOG_ERROR, "Error reading PTY: "+err.Error(), false, true)
				}
				break
			}
		}
		// Crucial: Process any remaining text (like the [y/N] prompt) after the loop ends
		processLine(lineBuffer.String())

	}

	// // Wait for the command to finish and catch the exit error
	// if err := cmd.Wait(); err != nil {
	// 	if exitErr, ok := err.(*exec.ExitError); ok {
	// 		// This shows the exit code (e.g., 1, 127)
	// 		logger.Print(essentials.LOG_ERROR, fmt.Sprintf("Command failed with exit code %d", exitErr.ExitCode()), false, true)
	// 	} else {
	// 		logger.Print(essentials.LOG_ERROR, "Command execution failed: "+err.Error(), false, true)
	// 	}
	// }

	if len(conflictingPackages) > 0 {
		logger.Print(essentials.LOG_WARNING, "Conflicting package found during sandbox installation.", false, true)
		for pkgName, targets := range conflictingPackages {
			fmt.Printf("%s is conflicting with %s.", pkgName, targets)
		}
		return essentials.ExecutionResult{Code: essentials.EXECUTION_TASK_SUCCESS, Message: conflictingPackages} // If command succeeded and conflict detected
	}

	return essentials.ExecutionResult{Code: essentials.EXECUTION_TASK_SUCCESS, Message: ""} // If command succeeded and no conflict detected
}

func SpawnPacman(args []string, pkgName string) essentials.ExecutionResult {
	logger.Print(essentials.LOG_INFO, "==> Redirecting to KUBOS pacman..", false, true)
	args = slices.Insert(args, 0, "pacman")
	fmt.Printf("Command: %v", args)
	logger.Print(essentials.LOG_INFO, "Setting up closed environment..", false, true)
	var ans string
	res := Setup(pkgName)
	if res.Code != essentials.EXECUTION_TASK_SUCCESS { // Ensure sandbox is set up before spawning pacman
		if res.Message == "sandbox exists" {
			for {
				fmt.Printf("There is an existing sandbox with name %s. Would you like to use the sandbox? [y/n] ", pkgName)
				_, err := fmt.Scan(&ans)
				if err != nil {
					return essentials.ExecutionResult{essentials.EXECUTION_TASK_FAIL, "Failed to scan stdin"}
				}
				if strings.ToLower(ans) == "y" || strings.ToLower(ans) == "yes" || strings.ToLower(ans) == "n" || strings.ToLower(ans) == "no" {
					break
				} else {
					fmt.Println("\nType y/n.")
					continue
				}
			}
		}
	}
	if ans == "n" || ans == "no" {
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

	err := cmd.Run()
	var msg string
	if err != nil {
		msg = err.Error()
	}

	return essentials.ExecutionResult{Code: essentials.EXECUTION_TASK_SUCCESS, Message: msg}
}

func SpawnYAY(args []string) essentials.ExecutionResult {
	fmt.Printf("==> forwarding to YAY: yay %v\n", args)
	cmd := exec.Command("yay", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err := cmd.Run()
	var msg string
	if err != nil {
		msg = err.Error()
	}

	return essentials.ExecutionResult{Code: essentials.EXECUTION_TASK_SUCCESS, Message: msg}
}

func Setup(givenName string) essentials.ExecutionResult {
	if essentials.IsSandboxExists(givenName) {
		logger.Print(essentials.LOG_WARNING, "Sandbox already exists: "+givenName, false, true)
		return essentials.ExecutionResult{Code: essentials.EXECUTION_TASK_FAIL, Message: "sandbox exists"}

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
			logger.Print(essentials.LOG_ERROR, fmt.Sprintf("Failed with output: %s and error: %s", output, err.Error()), false, true)
			return essentials.ExecutionResult{Code: essentials.EXECUTION_TASK_FAIL, Message: "failed with error"}

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
