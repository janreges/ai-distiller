package log

import "os"

// Info writes msg to stderr.
func Info(msg string) {
	os.Stderr.WriteString(msg + "\n")
}
