package cmd

import (
	"errors"
	"fmt"
	"io"
	"kubos/libraries/essentials"
	"kubos/libraries/logger"
	"os"
	"os/exec"
	"path"
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
		return essentials.ExecutionResult{Code: essentials.EXECUTION_CLEANUP_DIR_NOEXIST, Context: "Cleaning non sandbox directories inside sandbox/", Message: err.Error()} // Directory doesn't exist yet, nothing to clean
	}

	for _, entry := range entries {
		if entry.IsDir() && !validMap[entry.Name()] {
			sandboxDir := filepath.Join("sandboxes", entry.Name())
			if err := os.RemoveAll(sandboxDir); err != nil {
				// 1. Detect if it is explicitly a permission-denied issue
				if errors.Is(err, os.ErrPermission) {
					logger.LoggedPrint(essentials.EXECUTION_TASK_FAIL, "Failed to clean sandbox directories: Permission denied. Trying to run sudo mode instead..", true)
					res := RunWithTTY(exec.Command("sudo", "rm", "-rf", sandboxDir), "run")
					if res.ExecutionResult.Code != essentials.EXECUTION_TASK_SUCCESS {
						logger.LoggedPrint(essentials.EXECUTION_TASK_FAIL, "Failed to delete the sandbox directory.", true)
						return essentials.ExecutionResult{Code: essentials.EXECUTION_TASK_FAIL, Context: "Trying to delete the sandbox directories but couldn't.", Message: fmt.Sprintf("Deleting sandbox directory failed with error: %v", res.ExecutionResult.Message)}
					}
				} else {
					logger.LoggedPrint(essentials.EXECUTION_TASK_FAIL, "Failed to clean sandbox directories.", true)
					return essentials.ExecutionResult{Code: essentials.EXECUTION_TASK_FAIL, Context: "Trying to delete the sandbox directories but couldn't.", Message: fmt.Sprintf("Deleting sandbox directories failed with error: %v", err.Error())}

				}
			}
		}
	}
	return essentials.ExecutionResult{Code: essentials.EXECUTION_TASK_SUCCESS, Message: ""}
}

// DeBusyPath identifies and kills processes that are accessing the given path.
// This is particularly useful when an unmount fails because the target is busy.
func DeBusyPath(path string, verbose bool) essentials.ExecutionResult {
	logger.LoggedContextedPrint(essentials.LOG_INFO, "DEBUSY", "Checking for busy processes on: "+path, true)

	// fuser returns PIDs to stdout. We use sudo to ensure we see processes from all users.
	// Note: fuser returns a non-zero exit code if no processes are found, which we ignore.
	if verbose {
		logger.VerbosedPrint("Running command: sudo fuser " + path)
	}
	cmd := exec.Command("sudo", "fuser", path)
	output := RunWithTTY(cmd, "output").Output
	// var buf strings.Builder
	// _ = RunWithPTY(cmd, func(line string, w io.Writer) bool {
	// 	logger.ShellOutputPrint(line)
	// 	buf.WriteString(line)
	// 	return true
	// })
	// if res.Code != essentials.EXECUTION_TASK_SUCCESS {
	// 	logger.LoggedPrint(essentials.EXECUTION_TASK_FAIL, fmt.Sprintf("Failed to run and get fuser output. The app won't stop, it's just the debusy app won't do anything. Error: %v", res.Message), true)
	// }
	// if err := cmd.Wait(); err != nil {
	// 	// Jangan langsung anggap gagal, karena exit status 1 di fuser artinya "tidak ada PID yang ditemukan"
	// 	// Kita biarkan regex di bawah yang memastikan apakah ada PID atau tidak
	// }
	// fuser output is typically "path: PID1[suffix] PID2[suffix] ..."
	// We use a regex to extract only the numeric PIDs.
	// fuser output typically separates PIDs by whitespace (e.g., "  1234 5678m ")
	var pids []string
	for _, token := range strings.Fields(output) {
		cleaned := token
		if len(token) > 0 {
			// fuser sometimes appends an access type character to the end of a PID
			// (e.g., 'c' for current directory, 'm' for mapped file).
			lastChar := token[len(token)-1]
			if lastChar < '0' || lastChar > '9' {
				cleaned = token[:len(token)-1]
			}
		}

		// Ensure the token is genuinely a standalone numeric PID
		if _, err := strconv.Atoi(cleaned); err == nil && cleaned != "" {
			pids = append(pids, cleaned)
		}
	}

	if len(pids) == 0 {
		return essentials.ExecutionResult{Code: essentials.EXECUTION_TASK_SUCCESS, Message: ""}
	}

	for _, pid := range pids {
		if verbose {
			logger.VerbosedPrint("Running command: sudo kill -9 " + pid)
		}
		logger.LoggedPrint(essentials.LOG_WARNING, fmt.Sprintf("Killing process %s keeping %s busy", pid, path), true)
		// cmd2 := exec.Command("sudo", "kill", "-9", pid)
		_ = RunWithTTY(exec.Command("sudo", "kill", "-9", pid), "run")
	}
	// Give the kernel a small window to release the file handles
	time.Sleep(200 * time.Millisecond)
	return essentials.ExecutionResult{Code: essentials.EXECUTION_TASK_SUCCESS, Message: ""}
}

// Spawn executes a command within a systemd-nspawn container.
// It handles command parsing and ensures interactive I/O is preserved.
// func Spawn(container string, command string, verbose bool) essentials.ExecutionResult {
// 	cmdParts := strings.Fields(command)
// 	if len(cmdParts) == 0 {
// 		logger.LoggedPrint(essentials.LOG_ERROR, "No command provided to Spawn", true)
// 		return essentials.ExecutionResult{Code: essentials.EXECUTION_TASK_FAIL, Message: "No command provided to Spawn"}
// 	}

// 	containerPath := essentials.GetSandboxPath(container)
// 	if containerPath == "" {
// 		return essentials.ExecutionResult{Code: essentials.EXECUTION_TASK_FAIL, Message: fmt.Sprintf("sandbox %s does not exist", container)}
// 	}

// 	parsedPacmanResult := essentials.ParsePacmanCommand(command)
// 	if !parsedPacmanResult.IsValid {
// 		return essentials.ExecutionResult{Code: essentials.EXECUTION_TASK_FAIL, Message: "The command is not valid pacman command"}
// 	}

// 	if verbose {
// 		logger.VerbosedPrint("Running command: sudo systemd-nspawn -D " + filepath.Join(containerPath, "merged") + strings.Join(cmdParts, " "))
// 	}

// 	args := append([]string{"systemd-nspawn", "-D", filepath.Join(containerPath, "merged")}, cmdParts...)
// 	cmd := exec.Command("sudo", args...)

// 	ptmx, err := pty.Start(cmd)
// 	if err != nil {
// 		logger.LoggedPrint(essentials.LOG_ERROR, "Failed to start process with PTY: "+err.Error(), true)
// 		return essentials.ExecutionResult{Code: essentials.EXECUTION_TASK_FAIL, Message: "Failed to start PTY: " + err.Error()}
// 	}
// 	defer ptmx.Close()

// 	go func() {
// 		_, _ = io.Copy(ptmx, os.Stdin)
// 	}()

// 	conflictingPackages := make(essentials.ConflictingPackages)

// 	if parsedPacmanResult.Action == "install" {
// 		buf := make([]byte, 1024)
// 		lineEndingRegex := regexp.MustCompile(`\r+\n|\r`)

// 		const (
// 			PROCESSLINE_GENERIC_ERROR = iota
// 			PROCESSLINE_MOUNTPOINT_ERROR
// 			PROCESSLINE_SUCCESS
// 		)

// 		processLine := func(line string) int8 {
// 			cleanLine := strings.TrimSpace(essentials.ClearColor(essentials.CleanTerminalEscapeCodes(line)))
// 			if cleanLine == "" {
// 				return PROCESSLINE_GENERIC_ERROR
// 			}

// 			pkgName, conflictedWith, _, status := essentials.ParseConflictingPackages(cleanLine)
// 			if status || strings.Contains(strings.ToLower(cleanLine), "are in conflict") {
// 				conflictingPackages[pkgName] = []string{conflictedWith, strconv.FormatBool(status)}
// 			} else if strings.Contains(cleanLine, "Mount point '/run/systemd/nspawn/unix-export/merged' exists already, refusing.") {
// 				logger.ColoredPrint(color.FgYellow, "Systemd-nspawn mount point error has just happened. Try running 'kubos cleanup' and run the command again.")
// 				return PROCESSLINE_MOUNTPOINT_ERROR
// 			} else {
// 				logger.ShellOutputPrint(line)
// 			}
// 			return PROCESSLINE_SUCCESS
// 		}

// 	loop:
// 		for {
// 			n, err := ptmx.Read(buf)
// 			if n > 0 {
// 				chunk := essentials.CleanTerminalEscapeCodes(string(buf[:n]))
// 				chunk = lineEndingRegex.ReplaceAllString(chunk, "\n")

// 				lines := strings.Split(chunk, "\n")
// 				for _, line := range lines {
// 					if line == "" {
// 						continue
// 					}

// 					// Cek conflict prompt
// 					if strings.Contains(strings.ToLower(line), "are in conflict") && strings.Contains(line, "[y/N]") {
// 						processLine(line)
// 						if _, werr := ptmx.Write([]byte("y\n")); werr != nil {
// 							logger.ColoredPrint(color.FgRed, "Could not write answer into stdin. Please input yes.")
// 							logger.LoggedPrint(essentials.LOG_ERROR, "Could not write answer into stdin. Please input yes.", true)
// 						}
// 						continue
// 					}

// 					switch processLine(line) {
// 					case PROCESSLINE_MOUNTPOINT_ERROR:
// 						return essentials.ExecutionResult{Code: essentials.EXECUTION_TASK_FAIL, Message: "Systemd-nspawn mount point error."}
// 					}
// 				}
// 			}
// 			if err != nil {
// 				if errors.Is(err, io.EOF) || essentials.IsEOF(err) {
// 					break loop
// 				}
// 				logger.ColoredPrint(color.FgRed, "Error reading PTY: "+err.Error())
// 				break loop
// 			}
// 		}
// 	}

// 	if len(conflictingPackages) > 0 {
// 		logger.LoggedPrint(essentials.LOG_WARNING, "Conflicting package found during sandbox installation.", true)
// 		for pkgName, targets := range conflictingPackages {
// 			if verbose {
// 				logger.VerbosedPrint(fmt.Sprintf("%s is conflicting with %s.", pkgName, targets))
// 			}
// 		}
// 		return essentials.ExecutionResult{Code: essentials.EXECUTION_TASK_SUCCESS, Message: conflictingPackages}
// 	}

// 	return essentials.ExecutionResult{Code: essentials.EXECUTION_TASK_SUCCESS, Message: ""}
// }

func Spawn(container string, command string, verbose bool) essentials.ExecutionResult {
	cmdParts := strings.Fields(command)
	if len(cmdParts) == 0 {
		logger.LoggedPrint(essentials.LOG_ERROR, "No command provided to Spawn", true)
		return essentials.ExecutionResult{Code: essentials.EXECUTION_NO_ARGS, Context: "Spawning systemd-nspawn but no argument was sent.", Message: "No command provided to Spawn"}
	}

	containerPath := essentials.GetSandboxPath(container)
	if containerPath == "" {
		return essentials.ExecutionResult{Code: essentials.EXECUTION_SANDBOX_NOT_FOUND, Context: "Spawning systemd-nspawn but no sandbox was found.", Message: fmt.Sprintf("sandbox %s does not exist", container)}
	}

	parsedPacmanResult := essentials.ParsePacmanCommand(command)
	if !parsedPacmanResult.IsValid {
		return essentials.ExecutionResult{Code: essentials.EXECUTION_INVALID_CMD, Context: "Spawning systemd-nspawn but the command was invalid.", Message: "The command is not valid pacman command"}
	}

	if verbose {
		logger.VerbosedPrint("Running command: sudo systemd-nspawn -D " + filepath.Join(containerPath, "merged") + " " + strings.Join(cmdParts, " "))
	}

	args := append([]string{"systemd-nspawn", "-D", filepath.Join(containerPath, "merged")}, cmdParts...)
	cmd := exec.Command("sudo", args...)

	conflictingPackages := make(essentials.ConflictingPackages)
	var mountPointError bool

	if parsedPacmanResult.Action == "install" {
		err := RunWithPTY(cmd, func(line string, w io.Writer) bool {
			cleanLine := essentials.ClearColor(essentials.CleanTerminalEscapeCodes(line))
			if cleanLine == "" {
				return true
			}

			// Cek mountpoint error
			if strings.Contains(cleanLine, "Mount point '/run/systemd/nspawn/unix-export/merged' exists already, refusing.") {
				logger.ColoredPrint(color.FgYellow, "Systemd-nspawn mount point error has just happened. Try running 'kubos cleanup' and run the command again.")
				mountPointError = true
				return false // stop loop
			}

			// Cek conflict prompt
			if strings.Contains(strings.ToLower(cleanLine), "are in conflict") && strings.Contains(cleanLine, "[y/N]") {
				pkgName, conflictedWith, _, status := essentials.ParseConflictingPackages(cleanLine)
				conflictingPackages[pkgName] = []string{conflictedWith, strconv.FormatBool(status)}
				if _, werr := w.Write([]byte("y\n")); werr != nil {
					logger.ColoredPrint(color.FgRed, "Could not write answer into stdin. Please input yes.")
					logger.LoggedPrint(essentials.LOG_ERROR, "Could not write answer into stdin. Please input yes.", true)
				}
				return true
			}

			// Cek conflict line tanpa prompt (informational)
			pkgName, conflictedWith, _, status := essentials.ParseConflictingPackages(cleanLine)
			if status {
				conflictingPackages[pkgName] = []string{conflictedWith, strconv.FormatBool(status)}
				return true
			}

			logger.ShellOutputPrint(line)
			return true
		})

		if mountPointError {
			return essentials.ExecutionResult{Code: essentials.EXECUTION_SYSTEMD_MOUNTERR, Context: "Systemd-nspawn exited with mountpoint error.", Message: "Systemd-nspawn mount point error."}
		}
		if err.Code != essentials.EXECUTION_TASK_SUCCESS {
			return essentials.ExecutionResult{Code: essentials.EXECUTION_TASK_FAIL, Context: "Systemd-nspawn exited with an error.", Message: err.Message}
		}
	}

	if len(conflictingPackages) > 0 {
		logger.LoggedPrint(essentials.LOG_WARNING, "Conflicting package found during sandbox installation.", true)
		for pkgName, targets := range conflictingPackages {
			if verbose {
				logger.VerbosedPrint(fmt.Sprintf("%s is conflicting with %s.", pkgName, targets))
			}
		}
		return essentials.ExecutionResult{Code: essentials.EXECUTION_TASK_SUCCESS, Message: conflictingPackages}
	}

	return essentials.ExecutionResult{Code: essentials.EXECUTION_TASK_SUCCESS, Message: ""}
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
				logger.LoggedPrintNoNewline(essentials.LOG_WARNING, fmt.Sprintf("There is an existing sandbox with name %s. Would you like to use the sandbox? [y/n] ", pkgName), true)
				_, err := fmt.Scan(&ans)
				if err != nil {
					return essentials.ExecutionResult{Code: essentials.EXECUTION_TASK_FAIL, Context: "Trying to scan stdin but failed.", Message: "Failed to scan stdin"}
				}
				if strings.ToLower(ans) == "y" || strings.ToLower(ans) == "yes" || strings.ToLower(ans) == "n" || strings.ToLower(ans) == "no" {
					break
				} else {
					fmt.Println("\nType y/n.")
					continue
				}
			}
		} else {
			return res
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
	res := RunWithPTY(cmd, ptyPrintAndDone)
	if res.Code != essentials.EXECUTION_TASK_SUCCESS {
		logger.LoggedPrint(essentials.LOG_ERROR, fmt.Sprintf("Error happened during running pacman. Error: %v", res.Message), true)
		return res
	}
	// cmd.Stdin = os.Stdin
	// cmd.Stdout = os.Stdout
	// cmd.Stderr = os.Stderr

	// err := cmd.Run()
	// var msg string
	// if err != nil {
	// 	msg = err.Error()
	// }

	return essentials.ExecutionResult{Code: essentials.EXECUTION_TASK_SUCCESS, Message: ""}
}

func SpawnYAY(args []string, verbose bool) essentials.ExecutionResult {
	logger.LoggedContextedPrint(essentials.LOG_INFO, "PACMAN", "Redirecting to YAY..", true)
	if verbose {
		logger.VerbosedPrint("Running command: yay " + strings.Join(args, " "))
	}
	cmd := exec.Command("yay", args...)
	res := RunWithPTY(cmd, ptyPrintAndDone)
	if res.Code != essentials.EXECUTION_TASK_SUCCESS {
		logger.LoggedPrint(essentials.LOG_ERROR, fmt.Sprintf("Error happened during running pacman. Error: %v", res.Message), true)
		return res
	}
	// cmd.Stdin = os.Stdin
	// cmd.Stdout = os.Stdout
	// cmd.Stderr = os.Stderr

	// err := cmd.Run()
	// var msg string
	// if err != nil {
	// 	msg = err.Error()
	// }

	return essentials.ExecutionResult{Code: essentials.EXECUTION_TASK_SUCCESS, Message: ""}
}

func Setup(givenName string, verbose bool) essentials.ExecutionResult {
	logger.LoggedContextedPrint(essentials.LOG_INFO, "SANDBOX", fmt.Sprintf("Setting up sandbox %s...", givenName), true)
	// fmt.Println(essentials.IsValidSandbox(path.Join("sandboxes", givenName)), essentials.DIR_EXIST_INVALID_SANDBOX, essentials.IsValidSandbox(path.Join("sandboxes", givenName)) == essentials.DIR_EXIST_INVALID_SANDBOX)
	if essentials.IsSandboxExists(givenName) {
		logger.ColoredPrint(color.FgYellow, "Sandbox already exists: "+givenName)
		logger.Print(essentials.LOG_WARNING, "Sandbox already exists: "+givenName, true, true)
		return essentials.ExecutionResult{Code: essentials.EXECUTION_TASK_FAIL, Context: "Trying to set up sandbox but sandbox already exist.", Message: "sandbox exists"}

	} else if essentials.IsValidSandbox(path.Join("sandboxes/", givenName)) == essentials.DIR_EXIST_INVALID_SANDBOX {
		logger.LoggedPrint(essentials.LOG_ERROR, "Sandbox directory exists, but it is invalid sandbox. Try running 'kubos sandbox-cleanup "+givenName+"'.", true)
		return essentials.ExecutionResult{Code: essentials.EXECUTION_INVALID_SANDBOX, Context: "Trying to run a sandbox, but it is an invalid sandbox.", Message: "Sandbox is invalid"}

	}
	// Check if the needed directory exists safely
	logger.LoggedPrint(essentials.LOG_INFO, "Checking sandbox path status: sandboxes/"+givenName, true)

	sandboxPath := filepath.Join("sandboxes", givenName)
	info, err := os.Stat(sandboxPath)

	if err != nil {
		if os.IsNotExist(err) {
			// Scenario A: It genuinely doesn't exist. Create it.
			logger.LoggedPrint(essentials.LOG_INFO, ">> Directory does not exist. Creating path: "+sandboxPath, true)
			if err := os.MkdirAll(sandboxPath, 0755); err != nil {
				logger.ColoredPrint(color.FgRed, "Error creating sandbox directory: "+err.Error())
				logger.Print(essentials.LOG_ERROR, err.Error(), true, true)
				return essentials.ExecutionResult{Code: essentials.EXECUTION_TASK_FAIL, Context: "Trying to create a directory but failed.", Message: err.Error()}
			}

			logger.LoggedPrint(essentials.LOG_SUCCESS, "Created directory on path: "+sandboxPath, true)

			// Create the subdirectories for the overlay filesystem
			logger.LoggedPrint(essentials.LOG_INFO, fmt.Sprintf("Trying to create subdirectories in: %s", sandboxPath), true)
			subDirs := []string{"upper", "merged", "work"}
			for _, dir := range subDirs {
				fullPath := filepath.Join(sandboxPath, dir)
				if err := os.MkdirAll(fullPath, 0755); err != nil {
					logger.ColoredPrint(color.FgRed, fmt.Sprintf("Error creating subdirectories %s: %v", fullPath, err))
					logger.Print(essentials.LOG_ERROR, fmt.Sprintf("Error creating subdirectories %s: %v", fullPath, err), false, true)
					return essentials.ExecutionResult{Code: essentials.EXECUTION_TASK_FAIL, Context: "Trying to create subdirectories but failed.", Message: err.Error()}
				}
			}

			// Mount overlay filesystem
			command := "sudo mount -t overlay overlay -o lowerdir=/,upperdir=" + filepath.Join(sandboxPath, "upper") + ",workdir=" + filepath.Join(sandboxPath, "work") + " " + filepath.Join(sandboxPath, "merged")
			logger.LoggedPrint(essentials.LOG_INFO, "Mounting overlay filesystem on path: "+filepath.Join(sandboxPath, "merged"), true)
			logger.VerbosedPrint("Running command: " + command)

			parts := strings.Fields(command)
			res := RunWithTTY(exec.Command(parts[0], parts[1:]...), "run")

			if res.ExecutionResult.Code != essentials.EXECUTION_TASK_SUCCESS && res.ExecutionResult.Message != "" {
				logger.ColoredPrint(color.FgRed, fmt.Sprintf("Failed with output: %v ", res.ExecutionResult.Message))
				logger.Print(essentials.LOG_ERROR, fmt.Sprintf("Failed with output: %v", res.ExecutionResult.Message), true, true)
				return essentials.ExecutionResult{Code: essentials.EXECUTION_TASK_FAIL, Context: "Trying to mount overlayfs but failed.", Message: "failed with error"}
			}

			logger.ColoredPrint(color.FgGreen, "Successfully mounted overlay filesystem on path: "+filepath.Join(sandboxPath, "merged"))
			logger.Print(essentials.LOG_SUCCESS, "Successfully mounted overlay filesystem on path: "+filepath.Join(sandboxPath, "merged"), true, true)
		} else {
			// Scenario B: os.Stat failed due to another error (e.g., Permission Denied)
			logger.LoggedPrint(essentials.LOG_ERROR, "Failed to inspect sandbox path: "+err.Error(), true)
			return essentials.ExecutionResult{Code: essentials.EXECUTION_TASK_FAIL, Context: "Error checking if sandbox path exists.", Message: err.Error()}
		}
	} else if !info.IsDir() {
		// Scenario C: The path exists, but it's a file instead of a folder!
		logger.LoggedPrint(essentials.LOG_ERROR, "Sandbox path exists but is a regular file, not a directory: "+sandboxPath, true)
		return essentials.ExecutionResult{Code: essentials.EXECUTION_INVALID_SANDBOX, Context: "Sandbox path collision with an existing file.", Message: "path exists but is not a directory"}
	}

	// If the directory already exists securely or was successfully set up, return success path
	return essentials.ExecutionResult{Code: essentials.EXECUTION_TASK_SUCCESS, Message: sandboxPath}
	// If the directory already exists or was successfully created,
	// we should return the path to the sandbox.
	// Assuming the function should return the path to the base sandbox directory.

}

// Teardown cleans up the sandbox by unmounting the overlay and removing the directories.
func Teardown(givenName string, verbose bool) essentials.ExecutionResult {
	logger.LoggedContextedPrint(essentials.LOG_INFO, "CLEANUP", fmt.Sprintf("Start tearing down sandbox %s...", givenName), true)
	var isExistDir bool
	if !essentials.IsSandboxExists(givenName) {
		if essentials.IsValidSandbox(path.Join("sandboxes", givenName)) == essentials.DIR_EXIST_INVALID_SANDBOX {
			isExistDir = true
		} else {
			logger.ColoredPrint(color.FgRed, "Sandbox does not exist: "+givenName)

			logger.Print(essentials.LOG_WARNING, "Sandbox does not exist: "+givenName, true, true)
			return essentials.ExecutionResult{Code: essentials.EXECUTION_TASK_FAIL, Context: "Trying to teardown a sandbox that doesn't exist.", Message: fmt.Sprintf("sandbox %s does not exist", givenName)}

		}
	}
	sandboxPath := filepath.Join("sandboxes", givenName)
	mergedPath := filepath.Join(sandboxPath, "merged")

	// 1. Unmount the overlay filesystem
	// We use sudo here because the mount was likely created with sudo/root privileges.
	if !isExistDir {

		logger.LoggedPrint(essentials.LOG_INFO, "Unmounting overlay for "+mergedPath, true)
		DeBusyPath(mergedPath, verbose)
		logger.VerbosedPrint("Running this command: sudo umount " + mergedPath)

		umountCmd := exec.Command("sudo", "umount", mergedPath)
		err := RunWithTTY(umountCmd, "run")
		if err.ExecutionResult.Code != essentials.EXECUTION_TASK_SUCCESS {
			// If it's already unmounted, we might want to continue anyway to clean up files
			logger.ColoredPrint(color.FgRed, fmt.Sprintf("Unmount failed (it might already be unmounted): %s", err.ExecutionResult.Message))

			logger.Print(essentials.LOG_WARNING, fmt.Sprintf("Unmount failed (it might already be unmounted): %s", err.ExecutionResult.Message), true, true)
		} else {
			logger.ColoredPrint(color.FgGreen, "Successfully unmounted overlay for "+givenName)

			logger.Print(essentials.LOG_SUCCESS, "Successfully unmounted overlay for "+givenName, true, true)
		}

	}
	// 2. Remove the sandbox directory structure

	logger.LoggedPrint(essentials.LOG_INFO, "Removing sandbox directories: "+sandboxPath, true)
	res := removeSandboxDir(sandboxPath)
	if res.Code != essentials.EXECUTION_TASK_SUCCESS {
		logger.LoggedPrint(essentials.LOG_ERROR, fmt.Sprintf("Failed to remove sandbox directory, Error: %v", res.Message), true)
		return res
	}
	logger.ColoredPrint(color.FgGreen, "Cleaned up sandbox directories for "+givenName)

	logger.Print(essentials.LOG_SUCCESS, "Cleaned up sandbox directories for "+givenName, true, true)
	return essentials.ExecutionResult{Code: essentials.EXECUTION_TASK_SUCCESS, Message: ""}

}

// LineProcessor adalah callback yang dipanggil untuk setiap baris output
type LineProcessor func(line string, w io.Writer) bool // return false = stop loop
// RunWithPTY menjalankan command dengan PTY dan memanggil processor per baris
func RunWithPTY(cmd *exec.Cmd, processor LineProcessor) essentials.ExecutionResult {
	ptmx, err := pty.Start(cmd)
	if err != nil {
		return essentials.ExecutionResult{Code: essentials.EXECUTION_TASK_FAIL, Context: "Trying to start PTY but failed.", Message: fmt.Sprintf("failed to start PTY: %v", err)}
	}
	defer ptmx.Close()

	go func() {
		_, _ = io.Copy(ptmx, os.Stdin)
	}()

	buf := make([]byte, 1024)
	lineEndingRegex := regexp.MustCompile(`\r+\n|\r`)

loop:
	for {
		n, err := ptmx.Read(buf)
		if n > 0 {
			chunk := essentials.CleanTerminalEscapeCodes(string(buf[:n]))
			chunk = lineEndingRegex.ReplaceAllString(chunk, "\n")

			for _, line := range strings.Split(chunk, "\n") {
				if line == "" {
					continue
				}
				if !processor(line, ptmx) {
					break loop
				}
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) || essentials.IsEOF(err) {
				break loop
			}
			return essentials.ExecutionResult{Code: essentials.EXECUTION_TASK_FAIL, Context: "Trying to read PTY but failed.", Message: fmt.Sprintf("failed reading PTY: %v", err)}
		}
	}

	// NEW CODE: Wait for the process to exit and capture its exit code
	// if err := cmd.Wait(); err != nil {
	// 	var exitErr *exec.ExitError
	// 	if errors.As(err, &exitErr) {
	// 		return essentials.ExecutionResult{
	// 			Code:    essentials.EXECUTION_TASK_FAIL,
	// 			Context: "Command exited with non-zero status.",
	// 			Message: fmt.Sprintf("Command failed with exit code %d", exitErr.ExitCode()),
	// 		}
	// 	}
	// 	return essentials.ExecutionResult{
	// 		Code:    essentials.EXECUTION_TASK_FAIL,
	// 		Context: "Command wait failed.",
	// 		Message: err.Error(),
	// 	}
	// }
	return essentials.ExecutionResult{Code: essentials.EXECUTION_TASK_SUCCESS, Message: ""}
}

// RunWithTTY menjalankan command di TTY langsung.
// Mode:
//   - "run"    → jalankan dan tunggu selesai (seperti cmd.Run())
//   - "output" → jalankan dan kembalikan output sebagai string (seperti cmd.Output())
//   - "start"  → jalankan tanpa tunggu (seperti cmd.Start())

type TTYResult struct {
	essentials.ExecutionResult
	Output string // hanya terisi kalau mode "output"
}

func RunWithTTY(cmd *exec.Cmd, mode string) TTYResult {
	cmd.Stdin = os.Stdin

	var outBuf strings.Builder

	switch mode {
	case "output":
		// Tidak set cmd.Stdout — kita capture sendiri
		cmd.Stdout = &outBuf
		cmd.Stderr = os.Stderr
	default: // "run", "start"
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}

	var runErr error
	switch mode {
	case "start":
		runErr = cmd.Start()
	default: // "run", "output"
		runErr = cmd.Run()
	}

	if runErr != nil {
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			return TTYResult{
				ExecutionResult: essentials.ExecutionResult{
					Code:    essentials.EXECUTION_TASK_FAIL,
					Context: "Command exited with non-zero status.",
					Message: fmt.Sprintf("exit code %d", exitErr.ExitCode()),
				},
			}
		}
		return TTYResult{
			ExecutionResult: essentials.ExecutionResult{
				Code:    essentials.EXECUTION_TASK_FAIL,
				Context: "Command failed to run.",
				Message: runErr.Error(),
			},
		}
	}

	return TTYResult{
		ExecutionResult: essentials.ExecutionResult{Code: essentials.EXECUTION_TASK_SUCCESS},
		Output:          outBuf.String(),
	}
}

func ptyPrintAndDone(line string, w io.Writer) bool {
	logger.ShellOutputPrint(line)
	return true
}

func removeSandboxDir(sandboxPath string) essentials.ExecutionResult {
	err := os.RemoveAll(sandboxPath)
	if err == nil {
		return essentials.ExecutionResult{Code: essentials.EXECUTION_TASK_SUCCESS}
	}

	if errors.Is(err, os.ErrPermission) {
		res := RunWithTTY(exec.Command("sudo", "rm", "-rf", sandboxPath), "run")
		if res.ExecutionResult.Code == essentials.EXECUTION_TASK_SUCCESS {
			return essentials.ExecutionResult{Code: essentials.EXECUTION_TASK_SUCCESS}
		} else {
			return essentials.ExecutionResult{Code: essentials.EXECUTION_TASK_FAIL, Message: fmt.Sprintf("Failed to remove the sandbox directory using the legacy mode. Error: %v", err.Error()), Context: "Trying to remove the sandbox directory using legacy mode but still failed."}
		}
	}

	return essentials.ExecutionResult{
		Code:    essentials.EXECUTION_TASK_FAIL,
		Message: err.Error(),
	}
}
