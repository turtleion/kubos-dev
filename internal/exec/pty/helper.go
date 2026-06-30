package pty

import (
	"io"
	"kubos/internal/log"
)

func PTYPrintAndDone(line string, w io.Writer) bool {
	log.ShellOutputPrint(line)
	return true
}
