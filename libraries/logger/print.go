package logger

import (
	"fmt"
	"kubos/libraries/essentials"
	"strings"

	"github.com/fatih/color"
)

func LoggedContextedPrint(Status essentials.LogStatus, ContextTag string, Message string, WriteToFile bool) {
	fmt.Printf("%s [%s] %s\n", cyan("==>"), bold(ContextTag), bold(Message))
	Print(Status, fmt.Sprintf("==> [%s] %s\n", ContextTag, Message), true, WriteToFile)
}

// 🔴 DIUBAH: Menambahkan 4 spasi sebelum %s (cyan("::"))
func LoggedPrint(Status essentials.LogStatus, Message string, WriteToFile bool) {
	fmt.Printf("    %s %s\n", cyan("::"), Message)
	Print(Status, fmt.Sprintf("    :: %s\n", Message), true, WriteToFile)
}

func LoggedBasicPrint(Status essentials.LogStatus, Message string, WriteToFile bool) {
	fmt.Println(Message)
	Print(Status, fmt.Sprintln(Message), true, WriteToFile)
}

func ContextedPrint(ContextTag string, Message string) {
	fmt.Printf("%s [%s] %s\n", cyan("==>"), bold(ContextTag), bold(Message))
}

// 🔴 DIUBAH: Menambahkan 4 spasi sebelum %s (cyan("::"))
func ColoredPrint(attr color.Attribute, Message string) {
	fmt.Printf("    %s ", cyan("::"))
	color.New(attr).Println(Message)
}

func ContextedColoredPrint(attr color.Attribute, ContextTag string, Message string) {
	fmt.Printf("%s [%s] ", cyan("==>"), bold(ContextTag))
	color.New(attr).Println(Message)
}

func VerbosedPrint(Message string) {
	color.New(color.FgCyan).Printf("    VERBOSE >>  %s\n", Message)
}

// ShellOutputPrint mencetak output mentah dari perintah eksternal (seperti pacman)
// dengan prefix garis vertikal agar terpisah secara visual dari log Kubos.
func ShellOutputPrint(Message string) {
	// 1. Bersihkan spasi kosong aneh di ujung-ujung baris
	cleanMessage := strings.TrimSpace(Message)

	// 2. Jika baris kosong setelah dibersihkan, lewati saja agar tidak buat baris kosong palsu
	if cleanMessage == "" {
		return
	}

	// 3. Tangani masalah Carriage Return (\r) bawaan progress bar pacman
	// Jika pesan mengandung \r, kita bersihkan agar tidak merusak indentasi kita
	cleanMessage = strings.ReplaceAll(cleanMessage, "\r", "")

	// 4. Cetak dengan posisi indentasi yang dikunci aman
	if strings.Contains(cleanMessage, "[sudo] password for") || strings.Contains(cleanMessage, "[Y/n]") {
		fmt.Printf("    %s  %s", cyan("│"), cleanMessage)

	} else {
		fmt.Printf("    %s  %s\n", cyan("│"), cleanMessage)

	}
}
