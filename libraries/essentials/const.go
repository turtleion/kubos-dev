package essentials

// LogStatus represents the severity level of a log entry.
type LogStatus int8
type ExecutionStatusCode int8

type Sandbox struct {
	Name, Path string
}

type ExecutionResult struct {
	Code    ExecutionStatusCode
	Message string
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
const LOG_SUCCESS = 1
const LOG_WARNING = 2
const LOG_ERROR = 3
const LOG_INFO = 4

// Exec constants
const EXECUTION_TASK_FAIL ExecutionStatusCode = 5
const EXECUTION_TASK_SUCCESS ExecutionStatusCode = 6
const EXECUTION_TASK_COMPLETED ExecutionStatusCode = 7
const EXECUTION_UNKNOWN ExecutionStatusCode = 99
