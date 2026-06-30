package teardown

import (
	"fmt"
	"os/exec"
	"path"
	"path/filepath"

	exectypes "kubos/internal/exec/exectypes"
	"kubos/internal/exec/tty"
	"kubos/internal/log"
	"kubos/internal/util"

	"github.com/fatih/color"
)

// Teardown cleans up the sandbox by unmounting the overlay and removing the directories.
func Teardown(givenName string, verbose bool) exectypes.ExecutionResult {
	log.LoggedContextedPrint(log.INFO, "CLEANUP", fmt.Sprintf("Start tearing down sandbox %s...", givenName), true)
	var needsUnmount bool
	if !util.IsSandboxExists(givenName) {
		status := util.IsValidSandbox(path.Join("sandboxes", givenName))
		needsUnmount = status == util.VALID_SANDBOX || status == util.DIRMOUNT_EXIST_INVALID_SANDBOX

		if status == util.INVALID_SANDBOX {
			log.ColoredPrint(color.FgRed, "Sandbox does not exist: "+givenName)

			log.Print(log.WARNING, "Sandbox does not exist: "+givenName, true, true)
			return exectypes.ExecutionResult{Code: exectypes.EXECUTION_TASK_FAIL, Context: "Trying to teardown a sandbox that doesn't exist.", Message: fmt.Sprintf("sandbox %s does not exist", givenName)}

		}
	}
	sandboxPath := filepath.Join("sandboxes", givenName)
	mergedPath := filepath.Join(sandboxPath, "merged")

	// 1. Unmount the overlay filesystem
	// We use sudo here because the mount was likely created with sudo/root privileges.
	if needsUnmount {

		log.LoggedPrint(log.INFO, "Unmounting overlay for "+mergedPath, true)
		DeBusyPath(mergedPath, verbose)
		log.VerbosedPrint("Running this command: sudo umount " + mergedPath)

		umountCmd := exec.Command("sudo", "umount", mergedPath)
		err := tty.RunWithTTY(umountCmd, "run")
		if err.ExecutionResult.Code != exectypes.EXECUTION_TASK_SUCCESS {
			// If it's already unmounted, we might want to continue anyway to clean up files
			log.ColoredPrint(color.FgRed, fmt.Sprintf("Unmount failed (it might already be unmounted): %s", err.ExecutionResult.Message))

			log.Print(log.WARNING, fmt.Sprintf("Unmount failed (it might already be unmounted): %s", err.ExecutionResult.Message), true, true)
		} else {
			log.ColoredPrint(color.FgGreen, "Successfully unmounted overlay for "+givenName)

			log.Print(log.SUCCESS, "Successfully unmounted overlay for "+givenName, true, true)
		}

	}
	// 2. Remove the sandbox directory structure

	log.LoggedPrint(log.INFO, "Removing sandbox directories: "+sandboxPath, true)
	res := RemoveSandboxDir(sandboxPath)
	if res.Code != exectypes.EXECUTION_TASK_SUCCESS {
		log.LoggedPrint(log.ERROR, fmt.Sprintf("Failed to remove sandbox directory, Error: %v", res.Message), true)
		return res
	}
	log.ColoredPrint(color.FgGreen, "Cleaned up sandbox directories for "+givenName)

	log.Print(log.SUCCESS, "Cleaned up sandbox directories for "+givenName, true, true)
	return exectypes.ExecutionResult{Code: exectypes.EXECUTION_TASK_SUCCESS, Message: ""}

}
