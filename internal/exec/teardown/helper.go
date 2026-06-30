package teardown

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	exectypes "kubos/internal/exec/exectypes"
	"kubos/internal/exec/tty"
	"kubos/internal/log"
	"kubos/internal/util"
)

func cleanUp() exectypes.ExecutionResult {
	// Get the list of sandboxes that are currently valid/active
	validSandboxes := util.GetSandboxes()
	validMap := make(map[string]bool)
	for _, s := range validSandboxes {
		validMap[s.Name] = true
	}

	// Read the physical sandboxes directory
	entries, err := os.ReadDir("sandboxes")
	if err != nil {
		return exectypes.ExecutionResult{Code: exectypes.EXECUTION_CLEANUP_DIR_NOEXIST, Context: "Cleaning non sandbox directories inside sandbox/", Message: err.Error()} // Directory doesn't exist yet, nothing to clean
	}

	for _, entry := range entries {
		if entry.IsDir() && !validMap[entry.Name()] {
			sandboxDir := filepath.Join("sandboxes", entry.Name())
			if err := os.RemoveAll(sandboxDir); err != nil {
				// 1. Detect if it is explicitly a permission-denied issue
				if errors.Is(err, os.ErrPermission) {
					log.LoggedPrint(log.ERROR, "Failed to clean sandbox directories: Permission denied. Trying to run sudo mode instead..", true)
					res := tty.RunWithTTY(exec.Command("sudo", "rm", "-rf", sandboxDir), "run")
					if res.ExecutionResult.Code != exectypes.EXECUTION_TASK_SUCCESS {
						log.LoggedPrint(log.ERROR, "Failed to delete the sandbox directory.", true)
						return exectypes.ExecutionResult{Code: exectypes.EXECUTION_TASK_FAIL, Context: "Trying to delete the sandbox directories but couldn't.", Message: fmt.Sprintf("Deleting sandbox directory failed with error: %v", res.ExecutionResult.Message)}
					}
				} else {
					log.LoggedPrint(log.ERROR, "Failed to clean sandbox directories.", true)
					return exectypes.ExecutionResult{Code: exectypes.EXECUTION_TASK_FAIL, Context: "Trying to delete the sandbox directories but couldn't.", Message: fmt.Sprintf("Deleting sandbox directories failed with error: %v", err.Error())}

				}
			}
		}
	}
	return exectypes.ExecutionResult{Code: exectypes.EXECUTION_TASK_SUCCESS, Message: ""}
}

// DeBusyPath identifies and kills processes that are accessing the given path.
// This is particularly useful when an unmount fails because the target is busy.
func DeBusyPath(path string, verbose bool) exectypes.ExecutionResult {
	log.LoggedContextedPrint(log.INFO, "DEBUSY", "Checking for busy processes on: "+path, true)

	// fuser returns PIDs to stdout. We use sudo to ensure we see processes from all users.
	// Note: fuser returns a non-zero exit code if no processes are found, which we ignore.
	if verbose {
		log.VerbosedPrint("Running command: sudo fuser " + path)
	}
	cmd := exec.Command("sudo", "fuser", path)
	output := tty.RunWithTTY(cmd, "output").Output
	// var buf strings.Builder
	// _ = RunWithPTY(cmd, func(line string, w io.Writer) bool {
	// 	log.ShellOutputPrint(line)
	// 	buf.WriteString(line)
	// 	return true
	// })
	// if res.Code != exectypes.EXECUTION_TASK_SUCCESS {
	// 	log.LoggedPrint(exectypes.EXECUTION_TASK_FAIL, fmt.Sprintf("Failed to run and get fuser output. The app won't stop, it's just the debusy app won't do anything. Error: %v", res.Message), true)
	// }
	// if err := cmd.Wait(); err != nil {
	// 	// Jangan langsung anggap gagal, karena exit status 1 di fuser artinya "tidak ada PID yang ditemukan"
	// 	// Kita biarkan regex di bawah yang memastikan apakah ada PID atau tidak
	// }
	// fuser output is typically "path: PID1[suffix] PID2[suffix] ..."
	// We use a regex to extract only the numeric PIDs.
	// fuser output typically separates PIDs by whitespace (e.g., "  1234 5678m ")
	var pids []string
	for _, token := range strings.Fields(output.Stdout) {
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
		return exectypes.ExecutionResult{Code: exectypes.EXECUTION_TASK_SUCCESS, Message: ""}
	}

	for _, pid := range pids {
		if verbose {
			log.VerbosedPrint("Running command: sudo kill -9 " + pid)
		}
		log.LoggedPrint(log.WARNING, fmt.Sprintf("Killing process %s keeping %s busy", pid, path), true)
		// cmd2 := exec.Command("sudo", "kill", "-9", pid)
		_ = tty.RunWithTTY(exec.Command("sudo", "kill", "-9", pid), "run")
	}
	// Give the kernel a small window to release the file handles
	time.Sleep(200 * time.Millisecond)
	return exectypes.ExecutionResult{Code: exectypes.EXECUTION_TASK_SUCCESS, Message: ""}
}

func RemoveSandboxDir(sandboxPath string) exectypes.ExecutionResult {
	err := os.RemoveAll(sandboxPath)
	if err == nil {
		return exectypes.ExecutionResult{Code: exectypes.EXECUTION_TASK_SUCCESS}
	}

	log.LoggedPrint(log.INFO, "Failed to remove using default mode, falling back to legacy mode (rm -rf)..", true)
	if sandboxPath == "" || sandboxPath == "/" || len(sandboxPath) < 5 {
		log.LoggedPrint(log.WARNING, "Deletion is aborted. The path given is dangerous or invalid.", true)
		return exectypes.ExecutionResult{Code: exectypes.EXECUTION_INVALID_CMD, Context: "Trying to remove sandbox dir using legacy mode", Message: "Operation aborted due to dangerous or invalid path given"}
	}
	res := tty.RunWithTTY(exec.Command("sudo", "rm", "-rf", sandboxPath), "run")
	if res.ExecutionResult.Code == exectypes.EXECUTION_TASK_SUCCESS {
		return exectypes.ExecutionResult{Code: exectypes.EXECUTION_TASK_SUCCESS}
	} else {
		return exectypes.ExecutionResult{Code: exectypes.EXECUTION_TASK_FAIL, Message: fmt.Sprintf("Failed to remove the sandbox directory using the legacy mode. Error: %v", err.Error()), Context: "Trying to remove the sandbox directory using legacy mode but still failed."}
	}

}
