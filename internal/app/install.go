package app

import (
	"fmt"
	"kubos/internal/aur"
	"kubos/internal/exec"
	"kubos/internal/exec/exectypes"
	"kubos/internal/exec/pty"
	"kubos/internal/exec/teardown"
	"kubos/internal/log"
	"kubos/internal/test"
	"kubos/internal/util"
	"maps"
	"path"
	"slices"

	"github.com/fatih/color"
)

func installpkg(pkgName string, verbose bool) exectypes.ExecutionResult {
	log.LoggedContextedPrint(log.INFO, "KUBOS", fmt.Sprintf("Resolving %s...", pkgName), true)
	// 1. Check official repos first (pacman -Si exits 0 if found)
	if aur.IsInPacman(pkgName) {
		log.LoggedPrint(log.INFO, fmt.Sprintf("Found %s in pacman repos. Installing...", pkgName), true)
		res := exec.SpawnPacman([]string{"-S", pkgName}, pkgName, verbose)
		if res.Code != exectypes.EXECUTION_TASK_SUCCESS {
			if res.Code == exectypes.EXECUTION_INVALID_SANDBOX {
				return res
			}
			log.LoggedPrint(log.ERROR, "Failed to install the package, rolling back and cleaning the sandbox...", true)
			log.LoggedContextedPrint(log.INFO, "CLEANUP", "Cleaning up sandbox directory...", true)
			res2 := teardown.RemoveSandboxDir(path.Join("sandboxes", pkgName))
			if res2.Code != exectypes.EXECUTION_TASK_SUCCESS {
				log.LoggedPrint(log.ERROR, "Failed to clean up sandbox directory", true)
			}
			return res

		}
		var conflicting []string
		var conflictingMap exectypes.ConflictingPackages
		switch msg := res.Message.(type) {
		case exectypes.ConflictingPackages:
			conflicting = slices.Collect(maps.Keys(msg))
			conflictingMap = res.Message.(exectypes.ConflictingPackages)
		case string:
			log.LoggedPrint(log.ERROR, fmt.Sprintf("Conflicting packages detected: %v", conflicting), true)
			log.Print(log.ERROR, fmt.Sprintf("Conflicting packages detected: %v", conflicting), true, true)
		}
		// Sanitized, _, _, ok := essentials.ParsePacmanPkgName(conflicting[0])
		// fmt.Println("WOY! PkgName: ", conflicting[0], " Sannitized Version Name: ", Sanitized, " is Okay? ", ok)

		log.LoggedPrint(log.INFO, fmt.Sprintf("Running test for package %s", pkgName), true)
		report := test.RunTestSuite(path.Join("sandboxes", pkgName, "upper"), path.Join("sandboxes", pkgName, "merged"), pkgName, conflictingMap, conflicting, res)
		test.PrintReport(report)
		choice, err := util.AskSelection("Would you like to commit it to your real machine?", []string{"Yes, I would like to. I'm conscious of any risks.", "Nah, nevermind."})
		if err != nil {
			return exectypes.ExecutionResult{Code: exectypes.EXECUTION_TASK_FAIL, Context: "Trying to ask user but failed", Message: "Failed to ask user due to: " + err.Error()}
		}
		if choice == "Yes, I would like to. I'm conscious of any risks." {
			log.LoggedContextedPrint(log.INFO, "KUBOS", "Installing package "+pkgName+" to the real machine.", true)
			res := pty.HostPTYSpawn("sudo pacman -S "+pkgName, true)
			if res.Code != exectypes.EXECUTION_TASK_SUCCESS {
				return res
			}
		} else {
			fmt.Printf("\n\nAlright alright bro, I will do nothing. :v\n")
			return exectypes.ExecutionResult{Code: exectypes.EXECUTION_TASK_SUCCESS}
		}
		// _, ok = conflictingMap[pkgName]
		// fmt.Println("COnflictMap: ", conflictingMap, " | PkgName: ", pkgName, " | is conflictMap["+pkgName+"] okay? ", ok)
		// fmt.Println(report)
	}

	// 2. Fall back to AUR
	log.LoggedContextedPrint(log.INFO, "KUBOS", "Not in pacman repos. searching in AUR...", true)
	found, pkg, err := aur.AURExists(pkgName)
	if err != nil {
		log.ColoredPrint(color.FgRed, "AUR Lookup failed")
		log.Print(log.ERROR, "AUR Lookup failed", true, true)
		return exectypes.ExecutionResult{Code: exectypes.EXECUTION_TASK_FAIL, Context: "Looking package in AUR but resulted nothing.", Message: "AUR Lookup failed"}
	}

	if !found {
		log.LoggedPrint(log.ERROR, fmt.Sprintf("package '%s' not found in pacman repos or AUR", pkgName), true)
		return exectypes.ExecutionResult{Code: exectypes.EXECUTION_TASK_FAIL, Context: "Looking for a package that doesn't exist.", Message: fmt.Sprintf("package '%s' not found in pacman repos or AUR", pkgName)}
	}

	log.LoggedContextedPrint(log.SUCCESS, "KUBOS", fmt.Sprintf("Found '%s' on AUR (v%s, %d votes)\n", pkg.Name, pkg.Version, pkg.NumVotes), true)
	return exec.SpawnYAY([]string{"-S", pkgName}, verbose)

}

func Install(args []string, verbose bool) exectypes.ExecutionResult {
	// Default: systemd-nspawn scoping
	if len(args) == 0 {
		log.ContextedColoredPrint(color.FgRed, "KUBOS", "No argument detected. Please type the package to install.")
		log.Print(log.ERROR, "No argument", !verbose, true)
		return exectypes.ExecutionResult{Code: exectypes.EXECUTION_TASK_FAIL, Context: "Trying to install but no argument was sent.", Message: "No argument"}
	}

	for _, pkgName := range args {
		if err := installpkg(pkgName, verbose); err.Code != exectypes.EXECUTION_TASK_SUCCESS {
			return err
		}
	}
	return exectypes.ExecutionResult{Code: exectypes.EXECUTION_TASK_SUCCESS, Message: ""}

}
