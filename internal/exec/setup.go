package exec

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"

	"kubos/internal/exec/exectypes"
	"kubos/internal/exec/tty"
	"kubos/internal/log"
	"kubos/internal/util"

	"github.com/fatih/color"
)

func Setup(givenName string, verbose bool) exectypes.ExecutionResult {
	log.LoggedContextedPrint(log.INFO, "SANDBOX", fmt.Sprintf("Setting up sandbox %s...", givenName), true)
	// fmt.Println(essentials.IsValidSandbox(path.Join("sandboxes", givenName)), essentials.DIR_EXIST_INVALID_SANDBOX, essentials.IsValidSandbox(path.Join("sandboxes", givenName)) == essentials.DIR_EXIST_INVALID_SANDBOX)
	if util.IsSandboxExists(givenName) {
		log.ColoredPrint(color.FgYellow, "Sandbox already exists: "+givenName)
		log.Print(log.WARNING, "Sandbox already exists: "+givenName, true, true)
		return exectypes.ExecutionResult{Code: exectypes.EXECUTION_TASK_FAIL, Context: "Trying to set up sandbox but sandbox already exist.", Message: "sandbox exists"}

	} else if util.IsValidSandbox(path.Join("sandboxes/", givenName)) == util.DIR_EXIST_INVALID_SANDBOX {
		log.LoggedPrint(log.ERROR, "Sandbox directory exists, but it is invalid sandbox. Try running 'kubos sandbox-cleanup "+givenName+"'.", true)
		return exectypes.ExecutionResult{Code: exectypes.EXECUTION_INVALID_SANDBOX, Context: "Trying to run a sandbox, but it is an invalid sandbox.", Message: "Sandbox is invalid"}

	}
	// Check if the needed directory exists safely
	log.LoggedPrint(log.INFO, "Checking sandbox path status: sandboxes/"+givenName, true)

	sandboxPath := filepath.Join("sandboxes", givenName)
	info, err := os.Stat(sandboxPath)

	if err != nil {
		if os.IsNotExist(err) {
			// Scenario A: It genuinely doesn't exist. Create it.
			log.LoggedPrint(log.INFO, ">> Directory does not exist. Creating path: "+sandboxPath, true)
			if err := os.MkdirAll(sandboxPath, 0755); err != nil {
				log.ColoredPrint(color.FgRed, "Error creating sandbox directory: "+err.Error())
				log.Print(log.ERROR, err.Error(), true, true)
				return exectypes.ExecutionResult{Code: exectypes.EXECUTION_TASK_FAIL, Context: "Trying to create a directory but failed.", Message: err.Error()}
			}

			log.LoggedPrint(log.SUCCESS, "Created directory on path: "+sandboxPath, true)

			// Create the subdirectories for the overlay filesystem
			log.LoggedPrint(log.INFO, fmt.Sprintf("Trying to create subdirectories in: %s", sandboxPath), true)
			subDirs := []string{"upper", "merged", "work"}
			for _, dir := range subDirs {
				fullPath := filepath.Join(sandboxPath, dir)
				if err := os.MkdirAll(fullPath, 0755); err != nil {
					log.ColoredPrint(color.FgRed, fmt.Sprintf("Error creating subdirectories %s: %v", fullPath, err))
					log.Print(log.ERROR, fmt.Sprintf("Error creating subdirectories %s: %v", fullPath, err), false, true)
					return exectypes.ExecutionResult{Code: exectypes.EXECUTION_TASK_FAIL, Context: "Trying to create subdirectories but failed.", Message: err.Error()}
				}
			}

			// Mount overlay filesystem
			command := "sudo mount -t overlay overlay -o lowerdir=/,upperdir=" + filepath.Join(sandboxPath, "upper") + ",workdir=" + filepath.Join(sandboxPath, "work") + " " + filepath.Join(sandboxPath, "merged")
			log.LoggedPrint(log.INFO, "Mounting overlay filesystem on path: "+filepath.Join(sandboxPath, "merged"), true)
			log.VerbosedPrint("Running command: " + command)

			parts := strings.Fields(command)
			res := tty.RunWithTTY(exec.Command(parts[0], parts[1:]...), "run")

			if res.ExecutionResult.Code != exectypes.EXECUTION_TASK_SUCCESS && res.ExecutionResult.Message != "" {
				log.ColoredPrint(color.FgRed, fmt.Sprintf("Failed with output: %v ", res.ExecutionResult.Message))
				log.Print(log.ERROR, fmt.Sprintf("Failed with output: %v", res.ExecutionResult.Message), true, true)
				return exectypes.ExecutionResult{Code: exectypes.EXECUTION_TASK_FAIL, Context: "Trying to mount overlayfs but failed.", Message: "failed with error"}
			}

			log.ColoredPrint(color.FgGreen, "Successfully mounted overlay filesystem on path: "+filepath.Join(sandboxPath, "merged"))
			log.Print(log.SUCCESS, "Successfully mounted overlay filesystem on path: "+filepath.Join(sandboxPath, "merged"), true, true)
		} else {
			// Scenario B: os.Stat failed due to another error (e.g., Permission Denied)
			log.LoggedPrint(log.ERROR, "Failed to inspect sandbox path: "+err.Error(), true)
			return exectypes.ExecutionResult{Code: exectypes.EXECUTION_TASK_FAIL, Context: "Error checking if sandbox path exists.", Message: err.Error()}
		}
	} else if !info.IsDir() {
		// Scenario C: The path exists, but it's a file instead of a folder!
		log.LoggedPrint(log.ERROR, "Sandbox path exists but is a regular file, not a directory: "+sandboxPath, true)
		return exectypes.ExecutionResult{Code: exectypes.EXECUTION_INVALID_SANDBOX, Context: "Sandbox path collision with an existing file.", Message: "path exists but is not a directory"}
	}

	// If the directory already exists securely or was successfully set up, return success path
	return exectypes.ExecutionResult{Code: exectypes.EXECUTION_TASK_SUCCESS, Message: sandboxPath}
	// If the directory already exists or was successfully created,
	// we should return the path to the sandbox.
	// Assuming the function should return the path to the base sandbox directory.

}
