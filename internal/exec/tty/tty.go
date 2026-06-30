package tty

import (
	"errors"
	"fmt"
	exectypes "kubos/internal/exec/exectypes"
	"os"
	"os/exec"
	"strings"
)

type TTYResult struct {
	exectypes.ExecutionResult
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
				ExecutionResult: exectypes.ExecutionResult{
					Code:    exectypes.EXECUTION_TASK_FAIL,
					Context: "Command exited with non-zero status.",
					Message: fmt.Sprintf("exit code %d", exitErr.ExitCode()),
				},
			}
		}
		return TTYResult{
			ExecutionResult: exectypes.ExecutionResult{
				Code:    exectypes.EXECUTION_TASK_FAIL,
				Context: "Command failed to run.",
				Message: runErr.Error(),
			},
		}
	}

	return TTYResult{
		ExecutionResult: exectypes.ExecutionResult{Code: exectypes.EXECUTION_TASK_SUCCESS},
		Output:          outBuf.String(),
	}
}
