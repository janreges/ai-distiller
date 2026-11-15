package summary

import (
	"fmt"
	"io"
	"strings"
	"time"
)

// Stats holds the statistics for a distillation operation
type Stats struct {
	OriginalBytes   int64
	DistilledBytes  int64
	OriginalTokens  int64
	DistilledTokens int64
	Duration        time.Duration
	FileCount       int
	OutputPath      string
	IsStdout        bool // no output file was written
	Printed         bool // the output was printed to stdout
}

// Formatter defines the interface for summary formatters
type Formatter interface {
	Format(w io.Writer, stats Stats) error
}

// getCompressionRatio calculates the compression ratio as a percentage
func getCompressionRatio(original, distilled int64) float64 {
	if original == 0 {
		return 0
	}
	return (1 - float64(distilled)/float64(original)) * 100
}

// getEmoji returns an appropriate emoji based on compression ratio
func getEmoji(ratio float64) string {
	switch {
	case ratio >= 90:
		return "🚀"
	case ratio >= 70:
		return "✨"
	case ratio >= 50:
		return "👍"
	default:
		return "✅"
	}
}

// formatDuration formats a duration into a human-readable string
func formatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	return fmt.Sprintf("%.1fs", d.Seconds())
}

// buildProgressBar creates a visual progress bar
func buildProgressBar(ratio float64, width int) string {
	if width <= 0 {
		width = 40
	}
	
	filled := int(float64(width) * ratio / 100)
	if filled > width {
		filled = width
	}
	if filled < 0 {
		filled = 0
	}
	
	bar := strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
	return bar
}