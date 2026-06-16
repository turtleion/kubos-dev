package cmd

import (
	"errors"
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
	"github.com/fatih/color"
)

func cleanUp() essentials.ExecutionResult {
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
func DeBusyPath(path string, verbose bool) essentials.ExecutionResult {
	logger.LoggedContextedPrint(essentials.LOG_INFO, "DeBusy-INFO", "Checking for busy processes on: "+path, true)

	// fuser returns PIDs to stdout. We use sudo to ensure we see processes from all users.
	// Note: fuser returns a non-zero exit code if no processes are found, which we ignore.
	if verbose {
		logger.VerbosedPrint("Running command: sudo fuser " + path)
	}
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
		if verbose {
			logger.VerbosedPrint("Running command: sudo kill -9 " + pid)
		}
		logger.LoggedPrint(essentials.LOG_WARNING, fmt.Sprintf("Killing process %s keeping %s busy", pid, path), true)
		_ = exec.Command("sudo", "kill", "-9", pid).Run()
	}
	// Give the kernel a small window to release the file handles
	time.Sleep(200 * time.Millisecond)
	return essentials.ExecutionResult{Code: essentials.EXECUTION_TASK_SUCCESS, Message: ""}
}

// Spawn executes a command within a systemd-nspawn container.
// It handles command parsing and ensures interactive I/O is preserved.
func Spawn(container string, command string, verbose bool) essentials.ExecutionResult {
	// Split the command string into separate arguments (e.g., ["pacman", "-S", "hello"])
	cmdParts := strings.Fields(command)
	// Check if there is command provided to Spawn
	if len(cmdParts) == 0 {
		logger.LoggedPrint(essentials.LOG_ERROR, "No command provided to Spawn", true)
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
	if verbose {
		logger.VerbosedPrint("Running command: sudo systemd-nspawn -D " + filepath.Join(containerPath, "merged") + strings.Join(cmdParts, " "))
	}
	args := append([]string{"systemd-nspawn", "-D", filepath.Join(containerPath, "merged")}, cmdParts...)
	cmd := exec.Command("sudo", args...)

	ptmx, err := pty.Start(cmd)
	if err != nil {
		logger.LoggedPrint(essentials.LOG_ERROR, "Failed to start process with PTY: "+err.Error(), true)
		return essentials.ExecutionResult{Code: essentials.EXECUTION_TASK_FAIL, Message: "Failed to start PTY: " + err.Error()}
	}
	defer ptmx.Close()

	go func() {
		_, _ = io.Copy(ptmx, os.Stdin)
	}()
	// Get conflicting packages detail
	conflictingPackages := make(essentials.ConflictingPackages)

	if parsedPacmanResult.Action == "install" {
		// Replace your bufio.NewScanner(ptmx) block with a raw byte buffer read loop

		buf := make([]byte, 1024)
		var lineBuffer strings.Builder
		const (
			PROCESSLINE_GENERIC_ERROR = iota
			PROCESSLINE_MOUNTPOINT_ERROR
			PROCESSLINE_SUCCESS
		)
		// Helper to process a line for conflicts and logging
		processLine := func(line string) int8 {
			// Clear CSI (colors) and OSC escape codes for reliable regex matching
			cleanLine := strings.TrimSpace(essentials.ClearColor(essentials.CleanTerminalEscapeCodes(line)))
			if cleanLine == "" {
				return PROCESSLINE_GENERIC_ERROR
			}

			pkgName, conflictedWith, _, status := essentials.ParseConflictingPackages(cleanLine)
			if status || strings.Contains(strings.ToLower(cleanLine), "are in conflict") {
				//logger.Print(essentials.LOG_ERROR, "Conflicting package detected: "+cleanLine, false, false)
				conflictingPackages[pkgName] = []string{conflictedWith, strconv.FormatBool(status)}
			} else if strings.Contains(strings.ToLower(cleanLine), "Mount point '/run/systemd/nspawn/unix-export/merged' exists already, refusing.") {
				logger.ColoredPrint(color.FgYellow, "Systemd-nspawn mount point error has just happened. Try running 'kubos cleanup' and run the command again.")
				return PROCESSLINE_MOUNTPOINT_ERROR
			} else {
				// Normal output log
				logger.Print(essentials.LOG_INFO, line, true, true)
			}
			return PROCESSLINE_SUCCESS
		}

		for {
			n, err := ptmx.Read(buf)
			if n > 0 {
				chunk := essentials.CleanTerminalEscapeCodes(string(buf[:n]))
				var lineEndingRegex = regexp.MustCompile(`\r+\n|\r`)

				// logger.ShellOutputPrint(lineEndingRegex.ReplaceAllString(chunk, "\n"))
				// fmt.Printf("RAW CHUNK: %q\n", lineEndingRegex.ReplaceAllString(chunk, "\n")) // %q supaya \r \n keliatan literal
				chunk = lineEndingRegex.ReplaceAllString(chunk, "\n")
				lineBuffer.WriteString(chunk)

				// Cek prompt konflik SEBELUM nunggu newline
				current := lineBuffer.String()
				if strings.Contains(strings.ToLower(current), "are in conflict") && strings.Contains(current, "[y/N]") {
					// Jawab otomatis
					processLine(current)
					_, err := ptmx.Write([]byte("y\n"))
					if err != nil {
						logger.ColoredPrint(color.FgRed, "Could not write answer into stdin. Please input yes.")
						logger.Print(essentials.LOG_ERROR, "Could not write answer into stdin. Please input yes.", true, true)
					}
					lineBuffer.Reset()
				}

				if strings.Contains(lineBuffer.String(), "\n") {
					fullText := lineBuffer.String()
					lines := strings.Split(fullText, "\n")
					lineBuffer.Reset()

					// Jika potongan terakhir tidak diakhiri \n, simpan kembali ke buffer
					if !strings.HasSuffix(fullText, "\n") {
						lineBuffer.WriteString(lines[len(lines)-1])
						lines = lines[:len(lines)-1]
					}

					// Cetak dan proses HANYA baris yang sudah utuh sempurna!
					for _, line := range lines {
						if line == "" {
							continue
						}

						// Jalankan pengecekan error/konflik
						statusResult := processLine(line)

						switch statusResult {
						case PROCESSLINE_MOUNTPOINT_ERROR:
							return essentials.ExecutionResult{Code: essentials.EXECUTION_TASK_FAIL, Message: "Systemd-nspawn mount point error."}
						}
					}
				}
			}
			if err != nil {
				// EIO = slave side PTY ditutup = proses selesai, bukan error
				if errors.Is(err, io.EOF) || essentials.IsEOF(err) {
					break
				}
				logger.ColoredPrint(color.FgRed, "Error reading PTY: "+err.Error())
				break
			}
		}
		// Crucial: Process any remaining text (like the [y/N] prompt) after the loop ends
		remains := lineBuffer.String()
		if remains != "" {
			if status := processLine(remains); status == PROCESSLINE_SUCCESS {
				switch status {
				case PROCESSLINE_MOUNTPOINT_ERROR:
					return essentials.ExecutionResult{Code: essentials.EXECUTION_TASK_FAIL, Message: "Systemd-nspawn mount point error."}
				}
			}
		}

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
		logger.LoggedPrint(essentials.LOG_WARNING, "Conflicting package found during sandbox installation.", true)
		for pkgName, targets := range conflictingPackages {
			if verbose {
				logger.VerbosedPrint(fmt.Sprintf("%s is conflicting with %s.", pkgName, targets))
			}
		}
		return essentials.ExecutionResult{Code: essentials.EXECUTION_TASK_SUCCESS, Message: conflictingPackages} // If command succeeded and conflict detected
	}

	return essentials.ExecutionResult{Code: essentials.EXECUTION_TASK_SUCCESS, Message: ""} // If command succeeded and no conflict detected
}

func SpawnPacman(args []string, pkgName string, verbose bool) essentials.ExecutionResult {
	logger.LoggedContextedPrint(essentials.LOG_INFO, "PACMAN", "Redirecting to KUBOS pacman..", true)
	args = slices.Insert(args, 0, "pacman")
	logger.LoggedPrint(essentials.LOG_INFO, "Setting up closed environment (sandbox)..", true)
	var ans string
	res := Setup(pkgName, verbose)
	if res.Code != essentials.EXECUTION_TASK_SUCCESS { // Ensure sandbox is set up before spawning pacman
		if res.Message == "sandbox exists" {
			for {
				logger.LoggedPrint(essentials.LOG_WARNING, fmt.Sprintf("There is an existing sandbox with name %s. Would you like to use the sandbox? [y/n] ", pkgName), true)
				_, err := fmt.Scan(&ans)
				if err != nil {
					return essentials.ExecutionResult{Code: essentials.EXECUTION_TASK_FAIL, Message: "Failed to scan stdin"}
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
	return Spawn(pkgName, strings.Join(args, " "), verbose) // Correctly return the result of the Spawn call
}

func SpawnPassthroughPacman(args []string, verbose bool) essentials.ExecutionResult {
	logger.LoggedContextedPrint(essentials.LOG_INFO, "PACMAN", "Redirecting to PASSTHROUGH pacman..", true)
	args = slices.Insert(args, 0, "pacman")
	if verbose {
		logger.VerbosedPrint("Running command: sudo " + strings.Join(args, " "))
	}
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

func SpawnYAY(args []string, verbose bool) essentials.ExecutionResult {
	logger.LoggedContextedPrint(essentials.LOG_INFO, "PACMAN", "Redirecting to YAY..", true)
	if verbose {
		logger.VerbosedPrint("Running command: yay " + strings.Join(args, " "))
	}
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

func Setup(givenName string, verbose bool) essentials.ExecutionResult {
	logger.LoggedContextedPrint(essentials.LOG_INFO, "SANDBOX", fmt.Sprintf("Setting up sandbox %s...", givenName), true)
	if essentials.IsSandboxExists(givenName) {
		logger.ColoredPrint(color.FgYellow, "Sandbox already exists: "+givenName)
		logger.Print(essentials.LOG_WARNING, "Sandbox already exists: "+givenName, true, true)
		return essentials.ExecutionResult{Code: essentials.EXECUTION_TASK_FAIL, Message: "sandbox exists"}

	}
	// Check if the needed directory exist
	logger.LoggedPrint(essentials.LOG_INFO, "Trying to create a directory on path: sandboxes/"+givenName, true)
	if _, err := os.Stat(filepath.Join("sandboxes", givenName)); os.IsNotExist(err) {
		err := os.MkdirAll(filepath.Join("sandboxes", givenName), 0755)
		if err != nil {
			logger.ColoredPrint(color.FgRed, "Error creating sandbox directory: "+err.Error())

			logger.Print(essentials.LOG_ERROR, err.Error(), true, true)
			return essentials.ExecutionResult{Code: essentials.EXECUTION_TASK_FAIL, Message: err.Error()}

		}
		logger.LoggedPrint(essentials.LOG_SUCCESS, "Found desired directory on path: sandboxes/"+givenName, true)

		// Now create the subdirectories for the overlay filesystem
		logger.LoggedPrint(essentials.LOG_INFO, "Trying to create directories on path: sandboxes/"+givenName+"/upper, sandboxes/"+givenName+"/merged, sandboxes/"+givenName+"/work", true)

		subDirs := []string{"upper", "merged", "work"}
		for _, dir := range subDirs {
			fullPath := filepath.Join("sandboxes", givenName, dir)
			err := os.MkdirAll(fullPath, 0755)
			if err != nil {
				logger.ColoredPrint(color.FgRed, fmt.Sprintf("Error creating subdirectory %s: %v", fullPath, err))

				logger.Print(essentials.LOG_ERROR, fmt.Sprintf("Error creating subdirectory %s: %v", fullPath, err), false, true)
				return essentials.ExecutionResult{Code: essentials.EXECUTION_TASK_FAIL, Message: err.Error()}
			}
		}
		command := "sudo mount -t overlay overlay -o lowerdir=/,upperdir=" + filepath.Join("sandboxes", givenName, "upper") + ",workdir=" + filepath.Join("sandboxes", givenName, "work") + " " + filepath.Join("sandboxes", givenName, "merged")

		logger.LoggedPrint(essentials.LOG_SUCCESS, "Successfully created directories on path: sandboxes/"+givenName+"/upper, sandboxes/"+givenName+"/merged, sandboxes/"+givenName+"/work", true)

		logger.LoggedPrint(essentials.LOG_INFO, "Mounting overlay filesystem on path: sandboxes/"+givenName+"/merged", true)
		logger.VerbosedPrint("Running command: " + command)

		parts := strings.Fields(command)
		cmd := exec.Command(parts[0], parts[1:]...)

		// Run the command and capture both Standard Output and Standard Error
		output, err := cmd.CombinedOutput()

		// Check if the command succeeded
		if err != nil {
			// Command failed (returned non-zero exit code)
			logger.ColoredPrint(color.FgRed, fmt.Sprintf("Failed with output: %s and error: %s", output, err.Error()))

			logger.Print(essentials.LOG_ERROR, fmt.Sprintf("Failed with output: %s and error: %s", output, err.Error()), true, true)
			return essentials.ExecutionResult{Code: essentials.EXECUTION_TASK_FAIL, Message: "failed with error"}

		}

		logger.ColoredPrint(color.FgGreen, "Successfully mounted overlay filesystem on path: sandboxes/"+givenName+"/merged")

		logger.Print(essentials.LOG_SUCCESS, "Successfully mounted overlay filesystem on path: sandboxes/"+givenName+"/merged", true, true)

	}
	// If the directory already exists or was successfully created,
	// we should return the path to the sandbox.
	// Assuming the function should return the path to the base sandbox directory.
	return essentials.ExecutionResult{Code: essentials.EXECUTION_TASK_SUCCESS, Message: filepath.Join("sandboxes", givenName)}

}

// Teardown cleans up the sandbox by unmounting the overlay and removing the directories.
func Teardown(givenName string, verbose bool) essentials.ExecutionResult {
	logger.LoggedContextedPrint(essentials.LOG_INFO, "CLEANUP", fmt.Sprintf("Start tearing down sandbox %s...", givenName), true)
	if !essentials.IsSandboxExists(givenName) {
		logger.ColoredPrint(color.FgRed, "Sandbox does not exist: "+givenName)

		logger.Print(essentials.LOG_WARNING, "Sandbox does not exist: "+givenName, true, true)
		return essentials.ExecutionResult{Code: essentials.EXECUTION_TASK_FAIL, Message: fmt.Sprintf("sandbox %s does not exist", givenName)}
	}
	sandboxPath := filepath.Join("sandboxes", givenName)
	mergedPath := filepath.Join(sandboxPath, "merged")

	// 1. Unmount the overlay filesystem
	// We use sudo here because the mount was likely created with sudo/root privileges.
	logger.LoggedPrint(essentials.LOG_INFO, "Unmounting overlay for "+mergedPath, true)
	logger.VerbosedPrint("Will run this command after debusy: sudo umount " + mergedPath)
	DeBusyPath(mergedPath, verbose)

	umountCmd := exec.Command("sudo", "umount", mergedPath)
	output, err := umountCmd.CombinedOutput()
	if err != nil {
		// If it's already unmounted, we might want to continue anyway to clean up files
		logger.ColoredPrint(color.FgRed, fmt.Sprintf("Unmount failed (it might already be unmounted): %s", string(output)))

		logger.Print(essentials.LOG_WARNING, fmt.Sprintf("Unmount failed (it might already be unmounted): %s", string(output)), true, true)
	} else {
		logger.ColoredPrint(color.FgGreen, "Successfully unmounted overlay for "+givenName)

		logger.Print(essentials.LOG_SUCCESS, "Successfully unmounted overlay for "+givenName, true, true)
	}

	// 2. Remove the sandbox directory structure

	logger.LoggedPrint(essentials.LOG_INFO, "Removing sandbox directories: "+sandboxPath, true)

	err = os.RemoveAll(sandboxPath)
	if err != nil {
		logger.ColoredPrint(color.FgRed, "Failed to remove sandbox directory: "+err.Error())

		logger.Print(essentials.LOG_ERROR, "Failed to remove sandbox directory: "+err.Error(), true, true)
		return essentials.ExecutionResult{Code: essentials.EXECUTION_TASK_FAIL, Message: "Failed to remove sandbox directory: " + err.Error()}
	}
	logger.ColoredPrint(color.FgGreen, "Cleaned up sandbox directories for "+givenName)

	logger.Print(essentials.LOG_SUCCESS, "Cleaned up sandbox directories for "+givenName, true, true)
	return essentials.ExecutionResult{Code: essentials.EXECUTION_TASK_SUCCESS, Message: ""}

}
