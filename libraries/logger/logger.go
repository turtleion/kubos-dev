package logger

import (
	"fmt"
	"kubos/libraries/essentials"
	"os"
	"path/filepath"
	"time"

	"github.com/fatih/color"
)

var green = color.New(color.FgGreen).SprintFunc()
var yellow = color.New(color.FgYellow).SprintFunc()
var red = color.New(color.FgRed).SprintFunc()
var blue = color.New(color.FgBlue).SprintFunc()

// init is called automatically when the package is initialized.
// It records the start of the logging session.
func init() {
	writeToLogFile("-------------------------------------------\nLOGGING START ON " + time.Now().Format("2006-01-02 15:04:05"))
}

// Print outputs a formatted log message.
//
// Parameters:
//   - status: The severity level (e.g., LOG_SUCCESS, LOG_ERROR).
//   - messages: The content of the log message.
//   - silent: If true, suppresses output to the standard output (console).
//   - writetofile: If true, appends the log entry to a daily file in the 'logs' directory.
func Print(status essentials.LogStatus, messages string, silent bool, writetofile bool) {
	defer color.Unset()
	decorator := ""
	switch status {
	case essentials.LOG_SUCCESS:
		decorator = green("[V] ")
	case essentials.LOG_WARNING:
		decorator = yellow("[!] ")
	case essentials.LOG_ERROR:
		decorator = red("[X] ")
	case essentials.LOG_INFO:
		decorator = blue("[i] ")
	}

	// Format the log line: [Level] YYYY-MM-DD HH:MM:SS Message
	text := fmt.Sprint(decorator, time.Now().Format("2006-01-02 15:04:05"), " ", messages)

	if !silent {
		fmt.Println(text)
	}

	if writetofile {
		writeToLogFile(text)
	}
}

// writeToLogFile handles the persistence of logs to the local filesystem.
func writeToLogFile(text string) {
	logDir := "logs"
	// Ensure the log directory exists (drwxr-xr-x).
	if err := os.MkdirAll(logDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Critical Logger Error: Could not create log directory: %v\n", err)
		return
	}

	logFilename := filepath.Join(logDir, time.Now().Format("2006-01-02")+".txt")

	// Check if the file exists before opening to determine if we should write the header.
	_, statErr := os.Stat(logFilename)
	isNewFile := os.IsNotExist(statErr)

	// Open file with Read/Write, Create if missing, and Append mode (-rw-r--r--).
	file, err := os.OpenFile(logFilename, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Critical Logger Error: Could not open log file: %v\n", err)
		return
	}
	defer file.Close()

	if isNewFile {
		initialHeader := fmt.Sprintf("LOG FILE CREATED ON %s\n=============================\n", time.Now().Format("2006-01-02 15:04:05"))
		_, _ = file.WriteString(initialHeader)
	}

	// Strip ANSI color codes before writing to file to ensure log readability in standard text editors.
	_, err = file.WriteString(essentials.ClearColor(text) + "\n")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Critical Logger Error: Could not write to log file: %v\n", err)
	}
}
