package cmd

import (
	"fmt"
	"kubos/libraries/essentials"
	"kubos/libraries/logger"
)

func installpkg(pkgName string) essentials.ExecutionResult {
	logger.Print(essentials.LOG_INFO, fmt.Sprintf("==> Resolving %s...", pkgName), false, true)
	// 1. Check official repos first (pacman -Si exits 0 if found)
	if IsInPacman(pkgName) {
		logger.Print(essentials.LOG_INFO, fmt.Sprintf("==> Found %s in pacman repos. Installing...", pkgName), false, true)
		return SpawnPacman([]string{"-S", pkgName}, pkgName)
	}

	// 2. Fall back to AUR
	logger.Print(essentials.LOG_INFO, "==> Not in pacman repos. searching in AUR...", false, true)
	found, pkg, err := AURExists(pkgName)
	if err != nil {
		logger.Print(essentials.LOG_ERROR, "AUR Lookup failed", false, true)
		return essentials.ExecutionResult{Code: essentials.EXECUTION_TASK_FAIL, Message: "AUR Lookup failed"}
	}

	if !found {
		logger.Print(essentials.LOG_ERROR, fmt.Sprintf("package '%s' not found in pacman repos or AUR", pkgName), false, true)
		return essentials.ExecutionResult{Code: essentials.EXECUTION_TASK_FAIL, Message: fmt.Sprintf("package '%s' not found in pacman repos or AUR", pkgName)}
	}

	logger.Print(essentials.LOG_SUCCESS, fmt.Sprintf("==> Found '%s' on AUR (v%s, %d votes)\n", pkg.Name, pkg.Version, pkg.NumVotes), false, true)
	return SpawnYAY([]string{"-S", pkgName})

}

func Install(args []string) essentials.ExecutionResult {
	// Default: systemd-nspawn scoping
	if len(args) == 0 {
		logger.Print(essentials.LOG_ERROR, "No argument", false, true)
		return essentials.ExecutionResult{Code: essentials.EXECUTION_TASK_FAIL, Message: "No argument"}
	}

	for _, pkgName := range args {
		if err := installpkg(pkgName); err.Code != essentials.EXECUTION_TASK_SUCCESS {
			return err
		}
	}
	return essentials.ExecutionResult{Code: essentials.EXECUTION_TASK_SUCCESS, Message: ""}

}
