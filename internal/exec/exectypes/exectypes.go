package exectypes

import "io"

type ExecutionStatusCode int8

// LineProcessor adalah callback yang dipanggil untuk setiap baris output
type LineProcessor func(line string, w io.Writer) bool // return false = stop loop

type ExecutionResult struct {
	Code    ExecutionStatusCode
	Context string
	Message any
}

const (
	EXECUTION_TASK_FAIL      ExecutionStatusCode = iota // General fail (bawaanmu)
	EXECUTION_TASK_SUCCESS                              // Sukses (bawaanmu)
	EXECUTION_TASK_COMPLETED                            // Selesai (bawaanmu)

	// --- 🛑 LAYER 1: ERROR RUNTIME INTERNALS ---
	EXECUTION_PTY_ERROR   // Gagal inisialisasi/baca stream PTY
	EXECUTION_SUDO_DENIED // User membatalkan atau salah ketik password sudo
	EXECUTION_CLEANUP_DIR_NOEXIST
	EXECUTION_NO_ARGS
	EXECUTION_INVALID_CMD

	// --- 📦 LAYER 2: ERROR MANAGEMENT SANDBOX (OVERLAYFS / nspawn) ---
	EXECUTION_MOUNT_BUSY        // Gagal umount karena target masih busy
	EXECUTION_MOUNT_ALREADY     // Folder merged sudah di-mount (error nspawn tadi)
	EXECUTION_SANDBOX_NOT_FOUND // Nyari folder sandbox tapi gak ada
	EXECUTION_SYSTEMD_MOUNTERR

	// --- 🦅 LAYER 3: ERROR COMPONENT EXECUTION (PACMAN / AUR) ---
	EXECUTION_PACMAN_CONFLICT   // Konflik paket terdeteksi (seperti kasus mesa-amber)
	EXECUTION_PACKAGE_NOT_FOUND // Paket yang mau di-install gak ada di repo/AUR
	EXECUTION_NETWORK_TIMEOUT   // Koneksi internet putus pas download paket

	EXECUTION_INVALID_SANDBOX

	EXECUTION_UNKNOWN = 99 // Unknown (bawaanmu)
)

var EXECUTION_RESULT_STRING = map[ExecutionStatusCode]string{
	// General Execution Task
	EXECUTION_TASK_FAIL:      "EXECUTION_TASK_FAIL",
	EXECUTION_TASK_SUCCESS:   "EXECUTION_TASK_SUCCESS",
	EXECUTION_TASK_COMPLETED: "EXECUTION_TASK_COMPLETED",

	// LAYER 1: ERROR RUNTIME INTERNALS
	EXECUTION_PTY_ERROR:           "EXECUTION_PTY_ERROR",
	EXECUTION_SUDO_DENIED:         "EXECUTION_SUDO_DENIED",
	EXECUTION_CLEANUP_DIR_NOEXIST: "EXECUTION_CLEANUP_DIR_NOEXIST",
	EXECUTION_NO_ARGS:             "EXECUTION_NO_ARGS",
	EXECUTION_INVALID_CMD:         "EXECUTION_INVALID_CMD",

	// LAYER 2: ERROR MANAGEMENT SANDBOX
	EXECUTION_MOUNT_BUSY:        "EXECUTION_MOUNT_BUSY",
	EXECUTION_MOUNT_ALREADY:     "EXECUTION_MOUNT_ALREADY",
	EXECUTION_SANDBOX_NOT_FOUND: "EXECUTION_SANDBOX_NOT_FOUND",
	EXECUTION_SYSTEMD_MOUNTERR:  "EXECUTION_SYSTEMD_MOUNTERR",

	// LAYER 3: ERROR COMPONENT EXECUTION
	EXECUTION_PACMAN_CONFLICT:   "EXECUTION_PACMAN_CONFLICT",
	EXECUTION_PACKAGE_NOT_FOUND: "EXECUTION_PACKAGE_NOT_FOUND",
	EXECUTION_NETWORK_TIMEOUT:   "EXECUTION_NETWORK_TIMEOUT",

	EXECUTION_INVALID_SANDBOX: "EXECUTION_INVALID_SANDBOX",

	EXECUTION_UNKNOWN: "EXECUTION_UNKNOWN",
}

type ConflictingPackages map[string]string

// Result holds the classified meaning of the parsed pacman command.
type PacmanParserResult struct {
	IsValid   bool
	Action    string   // e.g., "system upgrade", "install", "search", "download only", "remove"
	Scope     string   // "remote" or "local"
	Summary   string   // Plain text description
	Targets   []string // Target packages or files
	Modifiers []string // Individual modifier flags found
}

// ToMap converts the PacmanParserResult into a clean, plain Go map[string]any.
func (r PacmanParserResult) ToMap() map[string]any {
	return map[string]any{
		"isValid":   r.IsValid,
		"action":    r.Action,
		"scope":     r.Scope,
		"summary":   r.Summary,
		"targets":   r.Targets,
		"modifiers": r.Modifiers,
	}
}

type Sandbox struct {
	Name, Path string
}
