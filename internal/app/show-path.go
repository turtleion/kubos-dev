package app

import (
	"fmt"
	"kubos/internal/layout"
	"kubos/internal/log"
)

// PrintPath mencetak PATH yang sedang aktif setelah Setup() dipanggil.
func PrintPath() {
	if layout.PATH == (layout.Path{}) {
		log.Print2("PATH is not set yet. Call layout.Setup() first.")
		return
	}

	log.ContextedPrint("PATH", "Current active path layout:")
	fmt.Printf("  Config  : %s\n", layout.PATH.Config)
	fmt.Printf("  Persist : %s\n", layout.PATH.Persist)
	fmt.Printf("  Log     : %s\n", layout.PATH.Log)
}
