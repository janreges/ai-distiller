package summary

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/dustin/go-humanize"
	"golang.org/x/term"
)

// BarFormatter formats summaries with visual progress bars for TTY environments
type BarFormatter struct {
	NoColor bool
	NoEmoji bool
}

// NewBarFormatter creates a new bar formatter
func NewBarFormatter() *BarFormatter {
	return &BarFormatter{
		NoColor: os.Getenv("NO_COLOR") != "",
		NoEmoji: false,
	}
}

// Format outputs a visually appealing summary with progress bar
func (f *BarFormatter) Format(w io.Writer, stats Stats) error {
	ratio := getCompressionRatio(stats.OriginalBytes, stats.DistilledBytes)

	// Get terminal width for responsive bar sizing
	barWidth := 15
	if fd, ok := w.(*os.File); ok {
		if width, _, err := term.GetSize(int(fd.Fd())); err == nil && width > 80 {
			// Use up to 20 chars for bar on wide terminals
			barWidth = min(20, width-100)
		}
	}

	// Build the components
	emoji := ""
	if !f.NoEmoji {
		emoji = getEmoji(ratio) + " "
	}

	bar := f.buildColoredProgressBar(ratio, barWidth)

	// Determine what was processed
	subject := "Distilled"
	if stats.FileCount > 0 {
		if stats.FileCount == 1 {
			subject = "Distilled 1 file"
		} else {
			subject = fmt.Sprintf("Distilled %d files", stats.FileCount)
		}
	}

	// Format the line - no decimals for high compression ratios
	ratioFormat := "%.1f%%"
	if ratio >= 80 {
		ratioFormat = "%.0f%%"
	}

	fmt.Fprintf(w, "%s%s [%s] "+ratioFormat+" (%s → %s) in %s",
		emoji,
		subject,
		bar,
		ratio,
		humanize.Bytes(uint64(stats.OriginalBytes)),
		humanize.Bytes(uint64(stats.DistilledBytes)),
		formatDuration(stats.Duration),
	)

	// Add token savings if significant
	tokensSaved := stats.OriginalTokens - stats.DistilledTokens
	if tokensSaved > 1000 {
		fmt.Fprintf(w, " 💰 ~%s tokens saved (~%s remaining)",
			formatTokenCount(tokensSaved),
			formatTokenCount(stats.DistilledTokens),
		)
	}

	fmt.Fprintln(w)

	// Add output path if not stdout on a new line
	if !stats.IsStdout && stats.OutputPath != "" {
		fileEmoji := "💾"
		if f.NoEmoji {
			fileEmoji = "→"
		}
		fmt.Fprintf(w, "%s Distilled output saved to: %s\n", fileEmoji, stats.OutputPath)

		// Add AI agent recommendation, unless the same output was printed
		if !stats.Printed {
			aiEmoji := "🤖"
			if f.NoEmoji {
				aiEmoji = "→"
			}
			fmt.Fprintf(w, "%s If you're an AI agent, load this ENTIRE file into your context (don't use grep/tail/head) for a comprehensive code overview.\n", aiEmoji)
		}
	}
	return nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// buildColoredProgressBar creates a visual progress bar with colors
func (f *BarFormatter) buildColoredProgressBar(ratio float64, width int) string {
	if width <= 0 {
		width = 15
	}

	filled := int(float64(width) * ratio / 100)
	if filled > width {
		filled = width
	}
	if filled < 0 {
		filled = 0
	}

	// ANSI color codes
	green := "\033[32m" // Green for saved portion
	red := "\033[31m"   // Red for remaining portion
	reset := "\033[0m"  // Reset color

	// If colors are disabled, use solid blocks for saved and dots for remaining
	if f.NoColor {
		return strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
	}

	// Build colored bar: green dots for saved, red dots for remaining
	bar := ""
	if filled > 0 {
		bar = green + strings.Repeat("░", filled) + reset
	}
	if width-filled > 0 {
		bar += red + strings.Repeat("░", width-filled) + reset
	}

	return bar
}
