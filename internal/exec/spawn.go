package exec

import (
	"fmt"
	"io"
	"kubos/internal/exec/pty"
	"kubos/internal/log"
	"kubos/internal/util"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"kubos/internal/exec/exectypes"

	"github.com/fatih/color"
)

func Spawn(container string, command string, verbose bool) exectypes.ExecutionResult {
	cmdParts := strings.Fields(command)
	if len(cmdParts) == 0 {
		log.LoggedPrint(log.ERROR, "No command provided to Spawn", true)
		return exectypes.ExecutionResult{Code: exectypes.EXECUTION_NO_ARGS, Context: "Spawning systemd-nspawn but no argument was sent.", Message: "No command provided to Spawn"}
	}

	containerPath := util.GetSandboxPath(container)
	if containerPath == "" {
		return exectypes.ExecutionResult{Code: exectypes.EXECUTION_SANDBOX_NOT_FOUND, Context: "Spawning systemd-nspawn but no sandbox was found.", Message: fmt.Sprintf("sandbox %s does not exist", container)}
	}

	parsedPacmanResult := util.ParsePacmanCommand(command)
	if !parsedPacmanResult.IsValid {
		return exectypes.ExecutionResult{Code: exectypes.EXECUTION_INVALID_CMD, Context: "Spawning systemd-nspawn but the command was invalid.", Message: "The command is not valid pacman command"}
	}

	if verbose {
		log.VerbosedPrint("Running command: sudo systemd-nspawn -D " + filepath.Join(containerPath, "merged") + " " + strings.Join(cmdParts, " "))
	}

	args := append([]string{"systemd-nspawn", "-D", filepath.Join(containerPath, "merged")}, cmdParts...)
	cmd := exec.Command("sudo", args...)

	conflictingPackages := make(exectypes.ConflictingPackages)
	var mountPointError bool

	if parsedPacmanResult.Action == "install" {
		err := pty.RunWithPTY(cmd, func(line string, w io.Writer) bool {
			cleanLine := util.ClearColor(util.CleanTerminalEscapeCodes(line))
			if cleanLine == "" {
				return true
			}

			// Cek mountpoint error
			if strings.Contains(cleanLine, "Mount point '/run/systemd/nspawn/unix-export/merged' exists already, refusing.") {
				log.ColoredPrint(color.FgYellow, "Systemd-nspawn mount point error has just happened. Try running 'kubos cleanup' and run the command again.")
				mountPointError = true
				return false // stop loop
			}

			// Cek conflict prompt
			if strings.Contains(strings.ToLower(cleanLine), "are in conflict") && strings.Contains(cleanLine, "[y/N]") {
				pkgName, conflictedWith, _, status := util.ParseConflictingPackages(cleanLine)
				if !status {
					log.LoggedPrint(log.ERROR, "Failed to parse conflicting packages from pacman output. Skipping conflicting packages feature.", true)
				}
				pkgNameSanitized, _, _, ok := util.ParsePacmanPkgName(pkgName)
				if !ok {
					log.LoggedPrint(log.ERROR, "Package name is not valid. Failed to insert it to conflicting package list.", false)
				}
				conflictingPackages[pkgNameSanitized] = conflictedWith
				if _, werr := w.Write([]byte("y\n")); werr != nil {
					log.ColoredPrint(color.FgRed, "Could not write answer into stdin. Please input yes.")
					log.LoggedPrint(log.ERROR, "Could not write answer into stdin. Please input yes.", true)
				}
				return true
			}

			// Cek conflict line tanpa prompt (informational)
			pkgName, conflictedWith, _, status := util.ParseConflictingPackages(cleanLine)
			if status {
				pkgNameSanitized, _, _, ok := util.ParsePacmanPkgName(pkgName)
				if !ok {
					log.LoggedPrint(log.ERROR, "Package name is not valid. Failed to insert it to conflicting package list.", false)
				} else {
					conflictingPackages[pkgNameSanitized] = conflictedWith

				}
				return true
			}

			log.ShellOutputPrint(line)
			return true
		})

		if mountPointError {
			return exectypes.ExecutionResult{Code: exectypes.EXECUTION_SYSTEMD_MOUNTERR, Context: "Systemd-nspawn exited with mountpoint error.", Message: "Systemd-nspawn mount point error."}
		}
		if err.Code != exectypes.EXECUTION_TASK_SUCCESS {
			return exectypes.ExecutionResult{Code: exectypes.EXECUTION_TASK_FAIL, Context: "Systemd-nspawn exited with an error.", Message: err.Message}
		}
	}

	if len(conflictingPackages) > 0 {
		log.LoggedPrint(log.WARNING, "Conflicting package found during sandbox installation.", true)
		for pkgName, targets := range conflictingPackages {
			if verbose {
				log.VerbosedPrint(fmt.Sprintf("%s is conflicting with %s.", pkgName, targets))
			}
		}
		return exectypes.ExecutionResult{Code: exectypes.EXECUTION_TASK_SUCCESS, Message: conflictingPackages}
	}

	return exectypes.ExecutionResult{Code: exectypes.EXECUTION_TASK_SUCCESS, Message: ""}
}

func SpawnPacman(args []string, pkgName string, verbose bool) exectypes.ExecutionResult {
	log.LoggedContextedPrint(log.INFO, "PACMAN", "Redirecting to KUBOS pacman..", true)
	args = slices.Insert(args, 0, "pacman")
	log.LoggedPrint(log.INFO, "Setting up closed environment (sandbox)..", true)
	var ans string
	res := Setup(pkgName, verbose)

	if res.Code != exectypes.EXECUTION_TASK_SUCCESS { // Ensure sandbox is set up before spawning pacman
		if res.Message == "sandbox exists" {
			for {
				log.LoggedPrintNoNewline(log.WARNING, fmt.Sprintf("There is an existing sandbox with name %s. Would you like to use the sandbox? [y/n] ", pkgName), true)
				_, err := fmt.Scan(&ans)
				if err != nil {
					return exectypes.ExecutionResult{Code: exectypes.EXECUTION_TASK_FAIL, Context: "Trying to scan stdin but failed.", Message: "Failed to scan stdin"}
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

func SpawnPassthroughPacman(args []string, verbose bool) exectypes.ExecutionResult {
	log.LoggedContextedPrint(log.INFO, "PACMAN", "Redirecting to PASSTHROUGH pacman..", true)
	args = slices.Insert(args, 0, "pacman")
	if verbose {
		log.VerbosedPrint("Running command: sudo " + strings.Join(args, " "))
	}
	cmd := exec.Command("sudo", args...)
	res := pty.RunWithPTY(cmd, pty.PTYPrintAndDone)
	if res.Code != exectypes.EXECUTION_TASK_SUCCESS {
		log.LoggedPrint(log.ERROR, fmt.Sprintf("Error happened during running pacman. Error: %v", res.Message), true)
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

	return exectypes.ExecutionResult{Code: exectypes.EXECUTION_TASK_SUCCESS, Message: ""}
}

func SpawnYAY(args []string, verbose bool) exectypes.ExecutionResult {
	log.LoggedContextedPrint(log.INFO, "PACMAN", "Redirecting to YAY..", true)
	if verbose {
		log.VerbosedPrint("Running command: yay " + strings.Join(args, " "))
	}
	cmd := exec.Command("yay", args...)
	res := pty.RunWithPTY(cmd, pty.PTYPrintAndDone)
	if res.Code != exectypes.EXECUTION_TASK_SUCCESS {
		log.LoggedPrint(log.ERROR, fmt.Sprintf("Error happened during running pacman. Error: %v", res.Message), true)
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

	return exectypes.ExecutionResult{Code: exectypes.EXECUTION_TASK_SUCCESS, Message: ""}
}
