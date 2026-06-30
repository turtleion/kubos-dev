package tty

import (
	"errors"
	"fmt"
	exectypes "kubos/internal/exec/exectypes"
	"os"
	"os/exec"
	"strings"
)

type TTYOutput struct {
	Stdout string
	Stderr string
}

type TTYResult struct {
	exectypes.ExecutionResult
	Output TTYOutput // hanya terisi kalau mode "output"
}

// Run shell command with TTY instead of PTY. Each new session will carry data from the previous session (such sudo session)
// You need to describe its mode,
//   - "output" to capture the output.
//   - "start" if only you want to get the error, and
//   - "run" you don't care anything about it
func RunWithTTY(cmd *exec.Cmd, mode string) TTYResult {
	cmd.Stdin = os.Stdin

	var outBuf, errBuf strings.Builder

	switch mode {
	case "output":
		// Tidak set cmd.Stdout — kita capture sendiri
		cmd.Stdout = &outBuf
		cmd.Stderr = &errBuf
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
		Output:          TTYOutput{outBuf.String(), errBuf.String()},
	}
}
