package essentials

// LogStatus represents the severity level of a log entry.
type LogStatus int8
type ExecutionStatusCode int8

type ConflictingPackages map[string][]string

type Sandbox struct {
	Name, Path string
}

type ExecutionResult struct {
	Code    ExecutionStatusCode
	Context string
	Message any
}

// Result holds the classified meaning of the parsed pacman command.
type PacmanParserResult struct {
	IsValid   bool
	Action    string   // e.g., "system upgrade", "install", "search", "download only", "remove"
	Scope     string   // "remote" or "local"
	Summary   string   // Plain text description
	Targets   []string // Target packages or files
	Modifiers []string // Individual modifier flags found
}

// Package represents a single AUR package result.
type AURPackage struct {
	Name        string  `json:"Name"`
	Version     string  `json:"Version"`
	Description string  `json:"Description"`
	URL         string  `json:"URL"`
	NumVotes    int     `json:"NumVotes"`
	Popularity  float64 `json:"Popularity"`
}

type RPCResponse struct {
	ResultCount int          `json:"resultcount"`
	Results     []AURPackage `json:"results"`
	Error       string       `json:"error"`
}

// Log severity levels used to categorize and color-code output.
const (
	LOG_SUCCESS = iota
	LOG_INFO
	LOG_WARNING
	LOG_ERROR
	EXECUTION_TASK_FAIL      // General fail (bawaanmu)
	EXECUTION_TASK_SUCCESS   // Sukses (bawaanmu)
	EXECUTION_TASK_COMPLETED // Selesai (bawaanmu)

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
	// Log severity levels
	LOG_SUCCESS: "LOG_SUCCESS",
	LOG_INFO:    "LOG_INFO",
	LOG_WARNING: "LOG_WARNING",
	LOG_ERROR:   "LOG_ERROR",

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
