package cmd

import (
	"fmt"
	"kubos/libraries/essentials"
	"kubos/libraries/logger"
	"maps"
	"slices"

	"github.com/fatih/color"
)

func installpkg(pkgName string, verbose bool) essentials.ExecutionResult {
	logger.LoggedContextedPrint(essentials.LOG_INFO, "KUBOS", fmt.Sprintf("Resolving %s...", pkgName), true)
	// 1. Check official repos first (pacman -Si exits 0 if found)
	if IsInPacman(pkgName) {
		logger.LoggedPrint(essentials.LOG_INFO, fmt.Sprintf("Found %s in pacman repos. Installing...", pkgName), true)
		res := SpawnPacman([]string{"-S", pkgName}, pkgName, verbose)
		if res.Code != essentials.EXECUTION_TASK_SUCCESS {
			return res
		}
		var conflicting []string
		switch msg := res.Message.(type) {
		case essentials.ConflictingPackages:
			conflicting = slices.Collect(maps.Keys(msg))
			logger.LoggedPrint(essentials.LOG_ERROR, fmt.Sprintf("Conflicting packages detected: %v", conflicting), true)
			logger.Print(essentials.LOG_ERROR, fmt.Sprintf("Conflicting packages detected: %v", conflicting), true, true)
		}

		logger.LoggedPrint(essentials.LOG_INFO, fmt.Sprintf("Running test for package %s", pkgName), true)
	}

	// 2. Fall back to AUR
	logger.LoggedContextedPrint(essentials.LOG_INFO, "KUBOS", "Not in pacman repos. searching in AUR...", true)
	found, pkg, err := AURExists(pkgName)
	if err != nil {
		logger.ColoredPrint(color.FgRed, "AUR Lookup failed")
		logger.Print(essentials.LOG_ERROR, "AUR Lookup failed", true, true)
		return essentials.ExecutionResult{Code: essentials.EXECUTION_TASK_FAIL, Context: "Looking package in AUR but resulted nothing.", Message: "AUR Lookup failed"}
	}

	if !found {
		logger.LoggedPrint(essentials.LOG_ERROR, fmt.Sprintf("package '%s' not found in pacman repos or AUR", pkgName), true)
		return essentials.ExecutionResult{Code: essentials.EXECUTION_TASK_FAIL, Context: "Looking for a package that doesn't exist.", Message: fmt.Sprintf("package '%s' not found in pacman repos or AUR", pkgName)}
	}

	logger.LoggedContextedPrint(essentials.LOG_SUCCESS, "KUBOS", fmt.Sprintf("Found '%s' on AUR (v%s, %d votes)\n", pkg.Name, pkg.Version, pkg.NumVotes), true)
	return SpawnYAY([]string{"-S", pkgName}, verbose)

}

func cleanup(pkgName string, verbose bool) essentials.ExecutionResult {
	logger.LoggedContextedPrint(essentials.LOG_INFO, "KUBOS", "Cleaning up...", true)
	err := Teardown(pkgName, verbose)
	if err.Code != essentials.EXECUTION_TASK_SUCCESS {
		logger.ColoredPrint(color.FgRed, "Teardown failed")
		logger.Print(essentials.LOG_ERROR, "Teardown failed", true, true)
	} else {
		logger.ColoredPrint(color.FgGreen, "Teardown succeeded")

		logger.Print(essentials.LOG_SUCCESS, "Teardown succeeded", true, true)
		logger.LoggedPrint(essentials.LOG_INFO, "Cleaning up sandbox directory", true)
		// err2 := cleanUp()
		// if err2.Code != essentials.EXECUTION_TASK_SUCCESS {
		// 	logger.ColoredPrint(color.FgRed, "Clean up failed")
		// 	logger.Print(essentials.LOG_ERROR, "Clean up failed", true, true)
		// } else {
		// 	logger.ColoredPrint(color.FgGreen, "Clean up succeeded")

		// 	logger.Print(essentials.LOG_SUCCESS, "Clean up succeeded", true, true)
		// }
	}
	return err
}

func Install(args []string, verbose bool) essentials.ExecutionResult {
	// Default: systemd-nspawn scoping
	if len(args) == 0 {
		logger.ContextedColoredPrint(color.FgRed, "KUBOS", "No argument detected. Please type the package to install.")
		logger.Print(essentials.LOG_ERROR, "No argument", !verbose, true)
		return essentials.ExecutionResult{Code: essentials.EXECUTION_TASK_FAIL, Context: "Trying to install but no argument was sent.", Message: "No argument"}
	}

	for _, pkgName := range args {
		if err := installpkg(pkgName, verbose); err.Code != essentials.EXECUTION_TASK_SUCCESS {
			return err
		}
	}
	return essentials.ExecutionResult{Code: essentials.EXECUTION_TASK_SUCCESS, Message: ""}

}

func CleanUp(args []string, verbose bool) essentials.ExecutionResult {
	// Default: systemd-nspawn scoping
	if len(args) == 0 {
		logger.ContextedColoredPrint(color.FgRed, "KUBOS", "No argument detected. Please type the package to install.")
		logger.Print(essentials.LOG_ERROR, "No argument", !verbose, true)
		return essentials.ExecutionResult{Code: essentials.EXECUTION_TASK_FAIL, Context: "Trying to clean up but no argument was sent.", Message: "No argument"}
	}

	for _, pkgName := range args {
		if err := cleanup(pkgName, verbose); err.Code != essentials.EXECUTION_TASK_SUCCESS {
			return err
		}
	}
	return essentials.ExecutionResult{Code: essentials.EXECUTION_TASK_COMPLETED, Message: ""}
}
