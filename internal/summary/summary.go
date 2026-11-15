package summary

import (
	"io"
	"os"

	"github.com/mattn/go-isatty"
)

// Options configures summary output behavior
type Options struct {
	Format  string // "auto", "ci", "bar", "json", "off"
	NoColor bool
	NoEmoji bool
}

// GetFormatter returns the appropriate formatter based on options and environment
func GetFormatter(opts Options) Formatter {
	// Handle explicit format selection
	switch opts.Format {
	case "off":
		return nil
	case "json":
		return NewJSONFormatter()
	case "ci", "ci-friendly":
		return NewCIFormatter()
	case "bar", "visual-progress-bar":
		formatter := NewBarFormatter()
		formatter.NoColor = opts.NoColor
		formatter.NoEmoji = opts.NoEmoji
		return formatter
	case "ticker", "stock-ticker":
		formatter := NewTickerFormatter()
		formatter.NoEmoji = opts.NoEmoji
		return formatter
	case "dashboard", "speedometer-dashboard":
		formatter := NewDashboardFormatter()
		formatter.NoEmoji = opts.NoEmoji
		return formatter
	case "sparkline", "minimalist-sparkline":
		formatter := NewSparklineFormatter()
		formatter.NoEmoji = opts.NoEmoji
		return formatter
	}

	// Auto-detection (default when using old name like "auto")
	// Check if we're in a CI environment or output is not a TTY
	if os.Getenv("CI") != "" || !isatty.IsTerminal(os.Stderr.Fd()) {
		return NewCIFormatter()
	}

	// Interactive terminal - use bar formatter (visual-progress-bar)
	formatter := NewBarFormatter()
	formatter.NoColor = opts.NoColor || os.Getenv("NO_COLOR") != ""
	formatter.NoEmoji = opts.NoEmoji
	return formatter
}

// Print outputs the summary using the specified formatter
func Print(w io.Writer, stats Stats, opts Options) error {
	formatter := GetFormatter(opts)
	if formatter == nil {
		return nil // "off" format
	}

	return formatter.Format(w, stats)
}

// EstimateTokens provides a rough estimate of token count from byte size:
// ~1 token per 4 bytes for code. No tokenizer is run.
func EstimateTokens(bytes int64) int64 {
	if bytes == 0 {
		return 0
	}
	// For code, the ratio is approximately 1 token per 4 bytes
	// This is a conservative estimate that works well for most programming languages
	return bytes / 4
}
