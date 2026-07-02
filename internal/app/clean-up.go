package app

import (
	"kubos/internal/exec/exectypes"
	"kubos/internal/exec/teardown"
	"kubos/internal/log"

	"github.com/fatih/color"
)

func cleanup(pkgName string, verbose bool) exectypes.ExecutionResult {
	log.LoggedContextedPrint(log.INFO, "KUBOS", "Cleaning up...", true)
	err := teardown.Teardown(pkgName, verbose)
	if err.Code != exectypes.EXECUTION_TASK_SUCCESS {
		log.ColoredPrint(color.FgRed, "Teardown failed")
		log.Print(log.ERROR, "Teardown failed", true, true)
	} else {
		log.ColoredPrint(color.FgGreen, "Teardown succeeded")

		log.Print(log.SUCCESS, "Teardown succeeded", true, true)
		log.LoggedPrint(log.INFO, "Cleaning up sandbox directory", true)
		// err2 := cleanUp()
		// if err2.Code != exectypes.EXECUTION_TASK_SUCCESS {
		// 	log.ColoredPrint(color.FgRed, "Clean up failed")
		// 	log.Print(log.ERROR, "Clean up failed", true, true)
		// } else {
		// 	log.ColoredPrint(color.FgGreen, "Clean up succeeded")

		// 	log.Print(log.SUCCESS, "Clean up succeeded", true, true)
		// }
	}
	return err
}

func CleanUp(args []string, verbose bool) exectypes.ExecutionResult {
	// Default: systemd-nspawn scoping
	if len(args) == 0 {
		log.ContextedColoredPrint(color.FgRed, "KUBOS", "No argument detected. Please type the package to install.")
		log.Print(log.ERROR, "No argument", !verbose, true)
		return exectypes.ExecutionResult{Code: exectypes.EXECUTION_TASK_FAIL, Context: "Trying to clean up but no argument was sent.", Message: "No argument"}
	}

	for _, pkgName := range args {
		if err := cleanup(pkgName, verbose); err.Code != exectypes.EXECUTION_TASK_SUCCESS {
			return err
		}
	}
	return exectypes.ExecutionResult{Code: exectypes.EXECUTION_TASK_COMPLETED, Message: ""}
}
