package parser

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
)

// ResolveError dipakai supaya caller (main.go) bisa bedain error resolusi
// dari error lain, dan nunjukin pesan yang jelas ke user.
type ResolveError struct {
	Msg string
}

func (e *ResolveError) Error() string {
	return e.Msg
}

// currentHomeDir ambil home directory dari sistem (UID lookup),
// BUKAN dari $HOME env var mentah, supaya gak gampang di-spoof
// (mis. `HOME=/home/userB kubos ...`).
func currentHomeDir() (string, error) {
	u, err := user.Current()
	if err != nil {
		return "", fmt.Errorf("gagal mendapatkan info user sistem: %w", err)
	}
	if u.HomeDir == "" {
		return "", fmt.Errorf("home directory user tidak terdeteksi")
	}
	// canonical-kan juga home dir-nya, jaga-jaga kalau home dir itu sendiri symlink
	resolved, err := filepath.EvalSymlinks(u.HomeDir)
	if err != nil {
		// kalau belum exist / gak bisa di-resolve, pakai apa adanya
		resolved = u.HomeDir
	}
	return filepath.Clean(resolved), nil
}

// canonicalize resolve path jadi absolute + hilangkan "..", ".", dan symlink.
// Kalau path belum exist di disk, EvalSymlinks gagal -> fallback ke
// filepath.Abs + Clean saja (tetap ngilangin "..", cuma gak resolve symlink).
func canonicalize(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("path tidak valid: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		return filepath.Clean(resolved), nil
	}
	return filepath.Clean(abs), nil
}

// isWithinHome cek apakah `target` benar-benar berada di bawah `home`,
// dengan boundary check yang benar (bukan cuma string prefix naif,
// biar "/home/userAxyz" gak keanggep bagian dari "/home/userA").
func isWithinHome(target, home string) bool {
	if target == home {
		return true
	}
	return len(target) > len(home) &&
		target[:len(home)] == home &&
		target[len(home)] == os.PathSeparator
}

// ResolvedFlags adalah hasil akhir setelah Flags mentah dari Parse()
// di-resolve & divalidasi. Field DevPath di sini SUDAH final/canonical,
// beda dengan Flags.DevPath yang masih mentah dari args.
type ResolvedFlags struct {
	Flags
	DevPath string // path final yang sudah di-canonicalize, siap dipakai
}

// defaultDevPath dipanggil kalau user gak kasih "+dev:PATH" dan
// config DEV_PATH juga kosong.
func defaultDevPath(home string) string {
	return filepath.Join(home, ".kubos", "dev")
}

// Resolve mengubah Flags mentah (hasil Parse) jadi ResolvedFlags yang
// sudah divalidasi. Ini tahap terpisah dari Parse() karena butuh I/O
// (baca sistem file, resolve symlink) sehingga Parse() sendiri tetap
// pure/syntactic dan gampang di-test.
//
// configDevPath: nilai DEV_PATH dari config file, kosongkan kalau gak diset.
func Resolve(f Flags, configDevPath string) (ResolvedFlags, error) {
	out := ResolvedFlags{Flags: f}

	if !f.DevMode.Enabled {
		// +dev gak dipakai sama sekali, gak ada yang perlu di-resolve
		return out, nil
	}

	home, err := currentHomeDir()
	if err != nil {
		return out, &ResolveError{Msg: fmt.Sprintf("tidak bisa menentukan home directory: %v", err)}
	}

	// tentukan path mentah sebelum di-canonicalize, urutan prioritas:
	// 1. +dev:PATH eksplisit dari user
	// 2. DEV_PATH dari config
	// 3. default ~/.kubos/dev
	rawPath := f.DevMode.Path
	if rawPath == "" {
		rawPath = configDevPath
	}
	if rawPath == "" {
		rawPath = defaultDevPath(home)
	}

	resolved, err := canonicalize(rawPath)
	if err != nil {
		return out, &ResolveError{Msg: fmt.Sprintf("dev path tidak valid: %v", err)}
	}
	out.DevPath = resolved

	// +user membatasi SEMUA operasi (termasuk dev path) ke dalam home
	// user saja. Kalau dev path resolusinya jatuh di luar home -> error,
	// TERMASUK kalau path itu kelihatan seperti "direktori user lain"
	// (mis. /home/userB), karena scope selalu relatif ke current user,
	// bukan pattern "/home/*" secara umum.
	if f.UserOperation && !isWithinHome(resolved, home) {
		return out, &ResolveError{
			Msg: fmt.Sprintf(
				"+user aktif tapi dev path '%s' berada di luar home direktori (%s)",
				resolved, home,
			),
		}
	}

	return out, nil
}
