package summary

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestSparklineFormatter(t *testing.T) {
	tests := []struct {
		name     string
		stats    Stats
		noEmoji  bool
		contains []string
	}{
		{
			name: "high compression with sparkline",
			stats: Stats{
				OriginalBytes:   5242880, // 5MB
				DistilledBytes:  131072,  // 128KB
				OriginalTokens:  1310720,
				DistilledTokens: 32768,
				Duration:        450 * time.Millisecond,
			},
			contains: []string{
				"✨ Distilled in 450ms.",
				"📦 5.2 MB → 131 kB (97.5% saved).",
				"🎟️ Tokens: ~1.3M → ~33k.",
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
				"Distilled in 120ms.",
				"1.0 MB → 367 kB (65.0% saved).",
				"Tokens: ~262k → ~92k.",
			},
		},
		{
			name: "low compression without tokens",
			stats: Stats{
				OriginalBytes:  2048, // 2KB
				DistilledBytes: 1536, // 1.5KB
				Duration:       10 * time.Millisecond,
			},
			contains: []string{
				"Distilled in 10ms.",
				"2.0 kB → 1.5 kB (25.0% saved).",
			},
		},
		{
			name: "very fast processing",
			stats: Stats{
				OriginalBytes:   102400, // 100KB
				DistilledBytes:  10240,  // 10KB
				OriginalTokens:  25600,
				DistilledTokens: 2560,
				Duration:        time.Second + 500*time.Millisecond,
			},
			contains: []string{
				"✨ Distilled in 1.5s.", // Shows seconds for >= 1s
				"📦 102 kB → 10 kB (90.0% saved).",
				"🎟️ Tokens: ~26k → ~3k.",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			formatter := NewSparklineFormatter()
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

			// Should be single line ending with newline
			if !strings.HasSuffix(output, "\n") {
				t.Errorf("Expected output to end with newline")
			}

			lines := strings.Split(strings.TrimSpace(output), "\n")
			if len(lines) != 1 {
				t.Errorf("Expected single line output, got %d lines", len(lines))
			}
		})
	}
}
