package main

import (
	"errors"
	"fmt"
	"kubos/cmd"
	"kubos/libraries/essentials"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

func CleanUp() {
	// Get the list of sandboxes that are currently valid/active
	fmt.Printf("Current active: %s", essentials.GetSandboxes())
	validSandboxes := essentials.GetSandboxes()
	validMap := make(map[string]bool)
	for _, s := range validSandboxes {
		validMap[s.Name] = true
	}
	// Read the physical sandboxes directory
	entries, err := os.ReadDir("sandboxes")
	if err != nil {
		return // Directory doesn't exist yet, nothing to clean
	}

	for _, entry := range entries {
		if entry.IsDir() && !validMap[entry.Name()] {
			fmt.Print("Removing entry: ", entry.Name())
			err = os.RemoveAll(filepath.Join("sandboxes", entry.Name()))
			if err != nil {
				if errors.Is(err, syscall.ENOTEMPTY) {
					fmt.Printf("Detected 'Directory not empty' error for: %s\n", entry.Name())
					cmd2 := exec.Command("sudo", "rm", "-rf", "sandboxes/"+entry.Name())
					if output, err := cmd2.CombinedOutput(); err != nil {
						// CombinedOutput returns the error AND the stderr message
						fmt.Printf("Sudo cleanup failed: %v\nOutput: %s\n", err, string(output))
						return
					}
				}
			}
		}
	}
}

func main() {
	fmt.Println("Logger loading")
	cmd.CleanUp()
	// CleanUp()
	sandboxes := essentials.GetSandboxes()
	for _, sandbox := range sandboxes {
		fmt.Println(sandbox.Name)
	}

	cmd.Setup("test")
	cmd.Teardown("test")

	// logger.Print(logger.LOG_ERROR, "Log fail", false, true)
}
