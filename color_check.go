package main

import (
	"fmt"
	"os"
)

// SupportsTrueColor checks if the current terminal environment supports 24-bit color.
func SupportsTrueColor() bool {
	colorTerm := os.Getenv("COLORTERM")
	return colorTerm == "truecolor" || colorTerm == "24bit"
}

// PrintRGB prints text in a specific RGB color.
func PrintRGB(r, g, b int, text string) {
	if SupportsTrueColor() {
		// ANSI Foreground: \x1b[38;2;R;G;Bm
		fmt.Printf("\x1b[38;2;%d;%d;%dm%s\x1b[0m\n", r, g, b, text)
	} else {
		// Fallback to plain text if not supported
		fmt.Println(text)
	}
}

func main() {
	if SupportsTrueColor() {
		fmt.Println("TrueColor is supported!")
		PrintRGB(255, 165, 0, "This is orange in TrueColor!")
		PrintRGB(123, 237, 159, "This is a custom mint green!")
	} else {
		fmt.Println("TrueColor is not supported by this terminal.")
	}
}
