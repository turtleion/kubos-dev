package log

// LogStatus represents the severity level of a log entry.
type LogStatusCode int8

// Log severity levels used to categorize and color-code output.
const (
	SUCCESS LogStatusCode = iota
	INFO
	WARNING
	ERROR
	UNKNOWN
	CRITICAL
)

var STATUS_STRING = map[LogStatusCode]string{
	SUCCESS:  "SUCCESS",
	INFO:     "INFO",
	WARNING:  "WARNING",
	ERROR:    "ERROR",
	UNKNOWN:  "UNKNOWN",
	CRITICAL: "CRITICAL",
}
