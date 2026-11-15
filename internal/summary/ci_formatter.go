package summary

import (
	"fmt"
	"io"

	"github.com/dustin/go-humanize"
)

// CIFormatter formats summaries for CI/pipeline environments
type CIFormatter struct{}

// NewCIFormatter creates a new CI formatter
func NewCIFormatter() *CIFormatter {
	return &CIFormatter{}
}

// Format outputs a clean, parseable summary line for CI environments
func (f *CIFormatter) Format(w io.Writer, stats Stats) error {
	ratio := getCompressionRatio(stats.OriginalBytes, stats.DistilledBytes)

	// Format: [aid] ✓ 97.6% saved | 5.2MB → 128KB | 450ms | ~1.2M → ~30k tokens
	fmt.Fprintf(w, "[aid] ✓ %.1f%% saved | %s → %s | %s",
		ratio,
		humanize.Bytes(uint64(stats.OriginalBytes)),
		humanize.Bytes(uint64(stats.DistilledBytes)),
		formatDuration(stats.Duration),
	)

	// Add token savings if available
	if stats.OriginalTokens > 0 && stats.DistilledTokens > 0 {
		tokensSaved := stats.OriginalTokens - stats.DistilledTokens
		fmt.Fprintf(w, " | ~%s tokens saved",
			formatTokenCount(tokensSaved),
		)
	}

	// Add output path if not stdout
	if !stats.IsStdout && stats.OutputPath != "" {
		fmt.Fprintf(w, " | saved to: %s", stats.OutputPath)
	}

	fmt.Fprintln(w)

	// Add AI agent recommendation when the output went only to the file
	if !stats.IsStdout && !stats.Printed && stats.OutputPath != "" {
		fmt.Fprintln(w, "AI agent recommendation: Load the ENTIRE output file into your context (don't use grep/tail/head) for a comprehensive code overview.")
	}

	return nil
}

// formatTokenCount formats token counts with appropriate units
func formatTokenCount(tokens int64) string {
	switch {
	case tokens >= 1000000:
		return fmt.Sprintf("%.1fM", float64(tokens)/1000000)
	case tokens >= 1000:
		return fmt.Sprintf("%.0fk", float64(tokens)/1000)
	default:
		return fmt.Sprintf("%d", tokens)
	}
}
