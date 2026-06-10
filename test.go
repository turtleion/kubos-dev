package main

import (
	"fmt"
	"kubos/cmd"
)

func main() {
	fmt.Println("Logger loading")
	cmd.Teardown("test")
	// logger.Print(logger.LOG_ERROR, "Log fail", false, true)
}
