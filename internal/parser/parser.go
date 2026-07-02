package parser

import "strings"

type Flags struct {
	UserOperation bool
	Verbose       bool
	UseSnapshot   bool
	DevMode       struct {
		Enabled bool
		Path    string
	} // kosong = belum ditentukan, resolve dari config/default nanti
}

type ParsedArgs struct {
	Flags     Flags
	Command   string
	Remaining []string
}

// setter menerima value (bisa string kosong kalau flag dipakai tanpa ":value")
type flagSetter func(f *Flags, value string)

// flag kubos sendiri: prefix "+", posisi bebas, opsional ":value"
var knownFlags = map[string]flagSetter{
	"+user": func(f *Flags, _ string) {
		f.UserOperation = true
	},
	"+verbose": func(f *Flags, _ string) {
		f.Verbose = true
	},
	"+use-snapshot": func(f *Flags, _ string) {
		f.UseSnapshot = true
	},
	"+dev": func(f *Flags, value string) {
		f.DevMode.Enabled = true
		f.DevMode.Path = value // kosong kalau cuma "+dev" tanpa value
	},
}

// command custom kubos
var knownCommands = map[string]bool{
	"install":         true,
	"remove":          true,
	"update":          true,
	"help":            true,
	"sandbox-cleanup": true,
}

// splitFlag memecah token "+dev:PATH" menjadi ("+dev", "PATH").
// Kalau gak ada ":", value-nya string kosong: "+dev" -> ("+dev", "").
func splitFlag(token string) (name string, value string) {
	if idx := strings.Index(token, ":"); idx != -1 {
		return token[:idx], token[idx+1:]
	}
	return token, ""
}

func Parse(args []string) ParsedArgs {
	var out ParsedArgs
	commandSet := false

	for _, a := range args {
		if strings.HasPrefix(a, "+") {
			name, value := splitFlag(a)
			if setter, ok := knownFlags[name]; ok {
				setter(&out.Flags, value)
				continue
			}
			// "+xxx" gak dikenal -> tetap dianggap token milik kubos yang gagal match,
			// bukan positional arg. Diteruskan ke Remaining biar kelihatan & bisa
			// di-flag sebagai error di lapisan atas (main.go), bukan didiamkan.
			out.Remaining = append(out.Remaining, a)
			continue
		}

		if !commandSet {
			// token non-flag pertama = command, apapun isinya (termasuk typo)
			out.Command = a
			commandSet = true
			continue
		}

		out.Remaining = append(out.Remaining, a)
	}
	return out
}
