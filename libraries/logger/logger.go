package logger

import (
	"fmt"
	"kubos/libraries"
	"os"
	"path/filepath"
	"time"

	"github.com/fatih/color"
)

// Set logger constants and some stuffs
const LOG_SUCCESS = 1
const LOG_WARNING = 2
const LOG_ERROR = 3
const LOG_INFO = 4

var green = color.New(color.FgGreen).SprintFunc()
var yellow = color.New(color.FgYellow).SprintFunc()
var red = color.New(color.FgRed).SprintFunc()
var blue = color.New(color.FgBlue).SprintFunc()

type LogStatus int8

// Define the methods.
/**

Print is used to print log to the stdout, or to a file, or even both.
Print needs 4 arguments,
- status LogStatus: An alias of int8
- messages string: Message to show/log
- silent boolean: indicating if the log should be printed on screen or not
- writetofile boolean: indicating if the log should be printed on file or not.

*/
func Print(status LogStatus, messages string, silent bool, writetofile bool) {
	defer color.Unset()
	decorator := ""
	switch status {
	case LOG_SUCCESS:
		decorator = green("[V] ")
	case LOG_WARNING:
		decorator = yellow("[!] ")
	case LOG_ERROR:
		decorator = red("[X] ")
	case LOG_INFO:
		decorator = blue("[i] ")
	}

	text := fmt.Sprint(decorator, time.Now().Format("2006-01-02 15:04:05"), " ", messages)
	if !silent {
		fmt.Println(text)
	}
	if writetofile {
		logDir := "logs"                 // This assumes 'logs' is relative to the directory where the program is executed
		err := os.MkdirAll(logDir, 0755) // Create directory with rwx for owner, rx for others
		if err != nil {
			Print(LOG_ERROR, "(This message won't be written to file) Error creating log directory: "+err.Error(), true, false)
			return
		}

		logFilename := filepath.Join(logDir, time.Now().Format("2006-01-02")+".txt")
		file, err := os.OpenFile(logFilename, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0644)
		if err != nil {
			Print(LOG_ERROR, "(This message won't be written to file) Error opening file "+err.Error(), true, false)
			return
		}
		defer file.Close()

		_, err = file.WriteString(libraries.ClearColor(text) + "\n")
		if err != nil {
			Print(LOG_ERROR, "(This message won't be written to file) Error writing file"+err.Error(), true, false)

		}
	}

}
