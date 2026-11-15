package summary

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestTickerFormatter(t *testing.T) {
	tests := []struct {
		name     string
		stats    Stats
		noEmoji  bool
		contains []string
	}{
		{
			name: "high compression with cost savings",
			stats: Stats{
				OriginalBytes:   5242880, // 5MB
				DistilledBytes:  131072,  // 128KB
				OriginalTokens:  1310720,
				DistilledTokens: 32768,
				Duration:        450 * time.Millisecond,
			},
			contains: []string{
				"📊 AID 97.5% ▲",
				"SIZE: 5.2 MB→131 kB",
				"TIME: 450ms",
				"EST: ~1.3M tokens saved",
			},
		},
		{
			name: "medium compression without emojis",
			stats: Stats{
				OriginalBytes:   1048576, // 1MB
				DistilledBytes:  367001,  // ~358KB
				OriginalTokens:  262144,
				DistilledTokens: 91750,
				Duration:        120 * time.Millisecond,
			},
			noEmoji: true,
			contains: []string{
				"[AID] AID 65.0% ▲",
				"SIZE: 1.0 MB→367 kB",
				"TIME: 120ms",
				"EST: ~170k tokens saved",
			},
		},
		{
			name: "low compression",
			stats: Stats{
				OriginalBytes:   2048, // 2KB
				DistilledBytes:  1536, // 1.5KB
				OriginalTokens:  512,
				DistilledTokens: 384,
				Duration:        10 * time.Millisecond,
			},
			contains: []string{
				"📊 AID 25.0% ▼", // Down arrow for low compression
				"SIZE: 2.0 kB→1.5 kB",
				"TIME: 10ms",
				"EST: ~128 tokens saved",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			formatter := NewTickerFormatter()
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
					t.Errorf("Expected output to contain %q, got: %s", expected, output)
				}
			}

			// Should end with newline
			if !strings.HasSuffix(output, "\n") {
				t.Errorf("Expected output to end with newline")
			}
		})
	}
}

func TestTickerFormatterEdgeCases(t *testing.T) {
	// Test with zero values
	formatter := NewTickerFormatter()
	var buf bytes.Buffer

	err := formatter.Format(&buf, Stats{})
	if err != nil {
		t.Fatalf("Format failed with zero stats: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "📊 AID 0.0% ▼") {
		t.Errorf("Expected zero compression to show 0.0%% with down arrow, got: %s", output)
	}
}
