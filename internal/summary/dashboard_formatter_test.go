package summary

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestDashboardFormatter(t *testing.T) {
	tests := []struct {
		name     string
		stats    Stats
		noEmoji  bool
		contains []string
	}{
		{
			name: "high compression with all features",
			stats: Stats{
				OriginalBytes:   5242880, // 5MB
				DistilledBytes:  131072,  // 128KB
				OriginalTokens:  1310720,
				DistilledTokens: 32768,
				Duration:        450 * time.Millisecond,
			},
			contains: []string{
				"╔═══ AI Distiller ═══╗",
				"║ Speed:",
				"║ Saved:",
				"97.5%",
				"5.2 MB→131 kB",
				"║ Tokens saved: ~1.3M",
				"╚═════════════════════╝",
			},
		},
		{
			name: "medium compression without emojis",
			stats: Stats{
				OriginalBytes:   1048576, // 1MB
				DistilledBytes:  367001,  // ~358KB
				OriginalTokens:  262144,
				DistilledTokens: 91750,
				Duration:        1200 * time.Millisecond, // > 1 second
			},
			noEmoji: true,
			contains: []string{
				"╔═══ AI Distiller ═══╗",
				"║ Speed:",
				"║ Saved:",
				"65.0%",
				"1.0 MB→367 kB",
				"1.2s",
				"║ Tokens saved: ~170k",
				"╚═════════════════════╝",
			},
		},
		{
			name: "fast processing time",
			stats: Stats{
				OriginalBytes:   10240, // 10KB
				DistilledBytes:  2048,  // 2KB
				OriginalTokens:  2560,
				DistilledTokens: 512,
				Duration:        50 * time.Millisecond, // < 100ms
			},
			contains: []string{
				"80.0%",
				"║ Speed:",
				"50ms",
				"10 kB→2.0 kB",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			formatter := NewDashboardFormatter()
			formatter.NoEmoji = tt.noEmoji

			var buf bytes.Buffer
			err := formatter.Format(&buf, tt.stats)
			if err != nil {
				t.Fatalf("Format failed: %v", err)
			}

			output := buf.String()

			// Check that all expected strings are present
			for _, expected := range tt.contains {
				if !strings.Contains(output, expected) {
					t.Errorf("Expected output to contain %q, got:\n%s", expected, output)
				}
			}

			// Check structure
			lines := strings.Split(strings.TrimSpace(output), "\n")
			if len(lines) < 4 {
				t.Errorf("Expected at least 4 lines, got %d", len(lines))
			}

			// Check box drawing characters
			if !strings.HasPrefix(lines[0], "╔") {
				t.Errorf("Expected first line to start with ╔")
			}
			if !strings.Contains(lines[len(lines)-1], "╚") {
				t.Errorf("Expected last line to contain ╚")
			}
		})
	}
}
