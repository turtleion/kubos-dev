package layout

import (
	"fmt"
	"kubos/internal/log"
	"os"
	"strings"
)

var (
	SystemLayout = map[string]string{
		"persist": "/var/lib/kubos",
		"log":     "/var/log/kubos",
		"config":  "/etc/kubos",
	}
	UserLayout = map[string]string{
		"persist": "(HOME)/.local/share/kubos",
		"log":     "(HOME)/.local/state/kubos",
		"config":  "(HOME)/.config/kubos",
	}
)

type Path struct {
	Persist string
	Log     string
	Config  string
}

type LayoutResult struct {
	Persist bool
	Log     bool
	Config  bool
	Overall bool
}

type LayoutCheckRes struct {
	Result LayoutResult
	Errors map[string]error
}

func getAllFalseResult() LayoutResult {
	return LayoutResult{Persist: false, Log: false, Config: false}
}

func CheckLayout(layout map[string]string) LayoutCheckRes {
	res := LayoutResult{}
	helper := func(l *LayoutResult, key string, ok bool) {
		switch key {
		case "persist":
			l.Persist = ok
		case "log":
			l.Log = ok
		case "config":
			l.Config = ok

		}

	}
	errs := make(map[string]error)
	for key, path := range layout {
		finfo, err := os.Stat(path)
		if err != nil {
			errs[key] = err
			helper(&res, key, false)
			continue
		}

		helper(&res, key, finfo.IsDir())

	}
	res.Overall = res.Persist && res.Config && res.Log
	return LayoutCheckRes{Result: res, Errors: errs}
}

func CheckSystemLayout() LayoutCheckRes {
	return CheckLayout(SystemLayout)
}
func CheckUserLayout() LayoutCheckRes {
	return CheckLayout(UserLayout)
}

var PATH = Path{}

func Setup(Verbose bool, ForceUser bool) {
	home := resolveHomeDir()

	log.ContextedPrint("PATH", "Setting up path for kubos to use. This and future messages will not be recorded in logs.")
	if home == "" {
		log.Print2("Failed to get HOME directory. User layout check will not work.")
	}

	for key, path := range UserLayout {
		UserLayout[key] = strings.ReplaceAll(path, "(HOME)", home)
	}

	errhelper := func(res LayoutCheckRes) {
		if !Verbose {
			return
		}
		for key, err := range res.Errors {
			log.VerbosedPrint(fmt.Sprintf("  \"%s\": %s", key, err.Error()))
		}
	}

	applyLayout := func(layout map[string]string) {
		PATH.Persist = layout["persist"]
		PATH.Log = layout["log"]
		PATH.Config = layout["config"]
	}

	failUser := func(cusr LayoutCheckRes) {
		log.Print2("Cannot use user path. Add verbose flag to show error.")
		errhelper(cusr)
		fmt.Printf("\n")
		if ForceUser {
			log.ErrorPanelPrint("LAYOUT_NOT_FOUND", "There is no valid layout detected.", "Trying to set path for kubos, but found no layout.",
				&log.HintBanner{
					Message: "User layout is not detected. You should try to regenerate kubos layout by",
					Title:   "Run this command:",
					Command: []string{"kubos repair-layout +user (if you want only the user layout)", "kubos repair-layout (if you want both system and user)"},
					Footer:  "After that you can run this command again.",
				})
		} else {
			log.ErrorPanelPrint("LAYOUT_NOT_FOUND", "There is no valid layout detected.", "Trying to set path for kubos, but found no layout.",
				&log.HintBanner{
					Message: "You should try to regenerate kubos layout by",
					Title:   "Running this command",
					Command: []string{"kubos repair-layout"},
					Footer:  "After that you can run this command again.",
				})
		}
	}

	// --user aktif: skip system layout sepenuhnya
	if ForceUser {
		log.Print2("--user flag detected. Skipping system layout check.")
		if cusr := CheckUserLayout(); cusr.Result.Overall {
			applyLayout(UserLayout)
		} else {
			failUser(cusr)
		}
		return
	}

	// Coba system layout dulu; hanya cek user layout kalau system gagal
	if csys := CheckSystemLayout(); csys.Result.Overall {
		applyLayout(SystemLayout)
		return
	} else {
		log.Print2("Cannot use system path. Add verbose flag to show error.")
		errhelper(csys)
		log.Print2("Switching to user path...")
	}

	if cusr := CheckUserLayout(); cusr.Result.Overall {
		applyLayout(UserLayout)
	} else {
		failUser(cusr)
	}
}

func resolveHomeDir() string {
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return home
	}
	if home := os.Getenv("HOME"); home != "" {
		return home
	}
	if home := os.Getenv("USERPROFILE"); home != "" {
		return home
	}
	if drive, path := os.Getenv("HOMEDRIVE"), os.Getenv("HOMEPATH"); drive != "" || path != "" {
		return drive + path
	}
	return ""
}
