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
