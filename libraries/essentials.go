package libraries

import (
	"regexp"
)

// ClearColor is a function to clear all color stuff in a string.
// ClearColor needs only one argument. It's so self explanatory
func ClearColor(str string) string {
	const ansiRegexPattern = `\x1b\[[0-9;]*[a-zA-Z]`
	re := regexp.MustCompile(ansiRegexPattern)

	return re.ReplaceAllString(str, "")
}
