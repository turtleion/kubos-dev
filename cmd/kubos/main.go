package main

import (
	"fmt"
	"kubos/internal/app"
	"kubos/internal/exec"
	"kubos/internal/exec/exectypes"
	"kubos/internal/log"

	"os"
	"strings"
)

// Help prints kubos usage.
func Help() {
	fmt.Println(`kubos - a pacman wrapper

USAGE:
  kubos <command> [args...]

CUSTOM COMMANDS:
  install <pkg>   Install a package (with kubos hooks)
  remove  <pkg>   Remove a package (uses -Rns)
  update          Full system upgrade (-Syu)
  help            Show this help

PACMAN PASSTHROUGH:
  Any argument starting with '-' is forwarded directly to pacman.
  Examples:
    kubos -S pkg        -> pacman -S pkg
    kubos -Qi pkg       -> pacman -Qi pkg
    kubos -Rns pkg      -> pacman -Rns pkg
`)
}

func main() {
	verbose := true
	args := os.Args[1:]

	if len(args) == 0 {
		Help()
		os.Exit(0)
	}

	first := args[0]
	rest := args[1:]

	// If the first arg starts with '-', it's a pacman flag → passthrough.
	if strings.HasPrefix(first, "-") {
		if err := exec.SpawnPassthroughPacman(args, verbose); err.Code != exectypes.EXECUTION_TASK_SUCCESS {
			os.Exit(1)
		}
		return
	}

	// Otherwise, route to kubos custom commands.
	var res exectypes.ExecutionResult
	switch first {
	case "install":
		res = app.Install(rest, verbose)
		if res.Code != exectypes.EXECUTION_TASK_SUCCESS {
			log.ParseAndPrintError(res)
		}
	case "sandbox-cleanup":
		res = app.CleanUp(rest, verbose)
	// case "remove":
	// 	err = Remove(rest)
	// case "update":
	// 	err = Update(rest)
	case "help", "--help", "-h":
		Help()
	default:
		fmt.Fprintf(os.Stderr, "kubos: unknown command '%s'\nRun 'kubos help' for usage.\n", first)
		os.Exit(1)
	}

	if res.Code != exectypes.EXECUTION_TASK_SUCCESS {
		fmt.Fprintf(os.Stderr, "kubos: error: %v\n", res.Message)
		os.Exit(1)
	}
}
