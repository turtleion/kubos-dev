package pty

import (
	"errors"
	"fmt"
	"io"
	"kubos/internal/exec/exectypes"
	"kubos/internal/log"
	"kubos/internal/util"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/creack/pty"
)

func HostPTYSpawn(command string, verbose bool) exectypes.ExecutionResult {
	cmdParts := strings.Fields(command)
	if len(cmdParts) == 0 {
		log.LoggedPrint(log.ERROR, "No command provided to Spawn", true)
		return exectypes.ExecutionResult{Code: exectypes.EXECUTION_NO_ARGS, Context: "Spawning systemd-nspawn but no argument was sent.", Message: "No command provided to Spawn"}
	}
	// Run with PTY
	cmd := exec.Command(command)
	if verbose {
		log.VerbosedPrint(fmt.Sprintf("Running command %s", command))
	}
	err := RunWithPTY(cmd, func(line string, w io.Writer) bool {
		log.ShellOutputPrint(line)
		return true
	})
	return err
}

// RunWithPTY menjalankan command dengan PTY dan memanggil processor per baris
func RunWithPTY(cmd *exec.Cmd, processor exectypes.LineProcessor) exectypes.ExecutionResult {
	ptmx, err := pty.Start(cmd)
	if err != nil {
		return exectypes.ExecutionResult{Code: exectypes.EXECUTION_TASK_FAIL, Context: "Trying to start PTY but failed.", Message: fmt.Sprintf("failed to start PTY: %v", err)}
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
			chunk := util.CleanTerminalEscapeCodes(string(buf[:n]))
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
			if errors.Is(err, io.EOF) || util.IsEOF(err) {
				break loop
			}
			return exectypes.ExecutionResult{Code: exectypes.EXECUTION_TASK_FAIL, Context: "Trying to read PTY but failed.", Message: fmt.Sprintf("failed reading PTY: %v", err)}
		}
	}

	// NEW CODE: Wait for the process to exit and capture its exit code
	// if err := cmd.Wait(); err != nil {
	// 	var exitErr *exectypes.ExitError
	// 	if errors.As(err, &exitErr) {
	// 		return exectypes.ExecutionResult{
	// 			Code:    exectypes.EXECUTION_TASK_FAIL,
	// 			Context: "Command exited with non-zero status.",
	// 			Message: fmt.Sprintf("Command failed with exit code %d", exitErr.ExitCode()),
	// 		}
	// 	}
	// 	return exectypes.ExecutionResult{
	// 		Code:    exectypes.EXECUTION_TASK_FAIL,
	// 		Context: "Command wait failed.",
	// 		Message: err.Error(),
	// 	}
	// }
	return exectypes.ExecutionResult{Code: exectypes.EXECUTION_TASK_SUCCESS, Message: ""}
}
