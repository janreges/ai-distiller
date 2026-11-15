package summary

import (
	"bytes"
	"strings"
	"testing"
)

// When the output was also printed to stdout, no formatter tells an AI agent
// to load the written file; it still names the file.
func TestPrintedOutputIsNotOfferedForLoading(t *testing.T) {
	for _, format := range []string{"visual-progress-bar", "stock-ticker", "speedometer-dashboard", "minimalist-sparkline", "ci-friendly"} {
		for _, printed := range []bool{false, true} {
			var buf bytes.Buffer
			stats := Stats{OriginalBytes: 4000, DistilledBytes: 400, FileCount: 1, OutputPath: "out.txt", Printed: printed}
			if err := Print(&buf, stats, Options{Format: format, NoColor: true}); err != nil {
				t.Fatal(err)
			}
			out := buf.String()
			if !strings.Contains(out, "out.txt") {
				t.Errorf("%s printed=%v: output path missing: %q", format, printed, out)
			}
			if got := strings.Contains(out, "ENTIRE"); got == printed {
				t.Errorf("%s printed=%v: load-the-file advice shown = %v: %q", format, printed, got, out)
			}
		}
	}
}
