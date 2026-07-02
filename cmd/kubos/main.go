package main

import (
	"fmt"
	"kubos/internal/app"
	"kubos/internal/exec/exectypes"
	"kubos/internal/layout"
	"kubos/internal/log"
	"kubos/internal/parser"

	"os"
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

	if len(os.Args[1:]) == 0 {
		Help()
		os.Exit(0)
	}

	// first := args[0]
	// rest := args[1:]

	// // If the first arg starts with '-', it's a pacman flag → passthrough.
	// if strings.HasPrefix(first, "-") {
	// 	if err := exec.SpawnPassthroughPacman(args, verbose); err.Code != exectypes.EXECUTION_TASK_SUCCESS {
	// 		os.Exit(1)
	// 	}
	// 	return
	// }

	parsed := parser.Parse(os.Args[1:])
	// spew.Dump(parsed)
	layout.Setup(parsed.Flags.Verbose, parsed.Flags.UserOperation)

	// Otherwise, route to kubos custom commands.
	var res exectypes.ExecutionResult
	switch parsed.Command {
	case "install":
		res = app.Install(parsed.Remaining, parsed.Flags.Verbose)
		if res.Code != exectypes.EXECUTION_TASK_SUCCESS {
			log.ParseAndPrintError(res)
		}
	case "sandbox-cleanup":
		res = app.CleanUp(parsed.Remaining, parsed.Flags.Verbose)
	case "show-path":
		app.PrintPath()
	// case "remove":
	// 	err = Remove(rest)
	// case "update":
	// 	err = Update(rest)
	case "help", "--help", "-h", "+h", "+help":
		Help()
	default:
		fmt.Fprintf(os.Stderr, "kubos: unknown command '%s'\nRun 'kubos help' for usage.\n", parsed.Command)
		os.Exit(1)
	}

	if res.Code != exectypes.EXECUTION_TASK_SUCCESS {
		fmt.Fprintf(os.Stderr, "kubos: error: %v\n", res.Message)
		os.Exit(1)
	}
}
