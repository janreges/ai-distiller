package debug

import (
	"fmt"
	"io"
	"log"
	"strings"
	"time"

	"github.com/davecgh/go-spew/spew"
)

// Verbosity levels
const (
	LevelBasic    = 1 // -v: Basic info (file counts, phase transitions)
	LevelDetailed = 2 // -vv: Detailed (AST node counts, processing steps)
	LevelTrace    = 3 // -vvv: Full data dumps of IR structures
)

// Debugger defines the interface for our debugging system
type Debugger interface {
	// Logf conditionally prints a formatted message if the verbosity is high enough
	Logf(level int, format string, args ...any)

	// Dump conditionally prints a detailed view of a data structure
	// Typically used with LevelTrace
	Dump(level int, label string, data any)

	// IsEnabledFor checks if a given verbosity level is active
	// This is a performance optimization to avoid expensive argument preparation
	IsEnabledFor(level int) bool

	// WithSubsystem returns a new Debugger instance scoped to a specific part of the pipeline
	// e.g., "parser", "formatter", "stripper"
	WithSubsystem(name string) Debugger

	// Timing starts a timer and returns a function to stop it and log the duration
	// Usage: defer dbg.Timing(LevelDetailed, "parsing")()
	Timing(level int, operation string) func()

	// SetFormat sets the output format (text or json)
	SetFormat(format string)
}

// cliDebugger is the concrete implementation used by the application
type cliDebugger struct {
	logger    *log.Logger
	verbosity int
	prefix    string
	format    string // "text" or "json"
}

// New creates a new debugger with a given verbosity level
func New(w io.Writer, verbosity int) Debugger {
	return &cliDebugger{
		logger:    log.New(w, "", 0), // No default prefix or flags
		verbosity: verbosity,
		format:    "text",
	}
}

func (d *cliDebugger) IsEnabledFor(level int) bool {
	return d.verbosity >= level
}

func (d *cliDebugger) Logf(level int, format string, args ...any) {
	if !d.IsEnabledFor(level) {
		return
	}

	levelStr := d.levelString(level)
	timestamp := time.Now().Format("15:04:05.000")

	if d.format == "json" {
		// Simple JSON output for now, can be enhanced with zap/zerolog later
		msg := fmt.Sprintf(format, args...)
		jsonMsg := fmt.Sprintf(`{"time":"%s","level":"%s","subsystem":"%s","msg":"%s"}`,
			timestamp, levelStr, strings.TrimSpace(d.prefix), escapeJSON(msg))
		d.logger.Println(jsonMsg)
	} else {
		// Text format with timestamp and level
		msg := fmt.Sprintf(format, args...)
		d.logger.Printf("[%s] %s%s: %s", timestamp, d.prefix, levelStr, msg)
	}
}

func (d *cliDebugger) Dump(level int, label string, data any) {
	if !d.IsEnabledFor(level) {
		return
	}

	timestamp := time.Now().Format("15:04:05.000")

	if d.format == "json" {
		// For JSON, we'll use a simplified dump
		dumpedString := fmt.Sprintf("%+v", data) // Simple for now
		jsonMsg := fmt.Sprintf(`{"time":"%s","level":"DUMP","subsystem":"%s","label":"%s","data":"%s"}`,
			timestamp, strings.TrimSpace(d.prefix), label, escapeJSON(dumpedString))
		d.logger.Println(jsonMsg)
	} else {
		// Use go-spew for human-readable dumps
		d.logger.Printf("[%s] %sDUMP: === %s ===\n", timestamp, d.prefix, label)

		// Configure spew for readable output
		config := spew.ConfigState{
			Indent:                  "  ",
			DisablePointerAddresses: true,
			DisableCapacities:       true,
			SortKeys:                true,
			SpewKeys:                false,
		}
		dumpedString := config.Sdump(data)

		// Add prefix to each line for consistent formatting
		lines := strings.Split(dumpedString, "\n")
		for _, line := range lines {
			if line != "" {
				d.logger.Printf("[%s] %s  %s", timestamp, d.prefix, line)
			}
		}
		d.logger.Printf("[%s] %s=== End %s ===\n", timestamp, d.prefix, label)
	}
}

func (d *cliDebugger) WithSubsystem(name string) Debugger {
	newPrefix := d.prefix
	if d.prefix == "" {
		newPrefix = fmt.Sprintf("[%s] ", name)
	} else {
		// Handle nested subsystems
		newPrefix = strings.TrimSuffix(d.prefix, " ")
		newPrefix = fmt.Sprintf("%s:%s] ", strings.TrimSuffix(newPrefix, "]"), name)
	}

	return &cliDebugger{
		logger:    d.logger,
		verbosity: d.verbosity,
		prefix:    newPrefix,
		format:    d.format,
	}
}

func (d *cliDebugger) Timing(level int, operation string) func() {
	if !d.IsEnabledFor(level) {
		return func() {} // No-op
	}

	start := time.Now()
	d.Logf(level, "Starting %s", operation)

	return func() {
		duration := time.Since(start)
		d.Logf(level, "Completed %s in %v", operation, duration)
	}
}

func (d *cliDebugger) SetFormat(format string) {
	d.format = format
}

func (d *cliDebugger) levelString(level int) string {
	switch level {
	case LevelBasic:
		return "INFO"
	case LevelDetailed:
		return "DEBUG"
	case LevelTrace:
		return "TRACE"
	default:
		return fmt.Sprintf("L%d", level)
	}
}

// escapeJSON escapes a string for JSON output
func escapeJSON(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	s = strings.ReplaceAll(s, "\r", `\r`)
	s = strings.ReplaceAll(s, "\t", `\t`)
	return s
}

// --- No-Op Implementation ---

type noOpDebugger struct{}

func (n *noOpDebugger) Logf(level int, format string, args ...any) {}
func (n *noOpDebugger) Dump(level int, label string, data any)     {}
func (n *noOpDebugger) IsEnabledFor(level int) bool                { return false }
func (n *noOpDebugger) WithSubsystem(name string) Debugger         { return n }
func (n *noOpDebugger) Timing(level int, operation string) func()  { return func() {} }
func (n *noOpDebugger) SetFormat(format string)                    {}

// Silent returns a no-op debugger that does nothing
func Silent() Debugger {
	return &noOpDebugger{}
}
