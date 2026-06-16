package main

import (
	"fmt"
	"kubos/libraries/essentials"
)

func main() {
	fmt.Println(essentials.ParsePacmanCommand("sudo pacman -Syy gnome"))
}
