package summary

import (
	"encoding/json"
	"io"
)

// JSONFormatter formats summaries as JSON for machine processing
type JSONFormatter struct{}

// NewJSONFormatter creates a new JSON formatter
func NewJSONFormatter() *JSONFormatter {
	return &JSONFormatter{}
}

// JSONOutput represents the JSON structure for summary output
type JSONOutput struct {
	OriginalBytes   int64   `json:"original_bytes"`
	DistilledBytes  int64   `json:"distilled_bytes"`
	SavingsPercent  float64 `json:"savings_pct"`
	DurationMS      int64   `json:"duration_ms"`
	TokensBefore    int64   `json:"tokens_before,omitempty"`
	TokensAfter     int64   `json:"tokens_after,omitempty"`
	TokensSaved     int64   `json:"tokens_saved,omitempty"`
	TokenSavingsPct float64 `json:"token_savings_pct,omitempty"`
	FileCount       int     `json:"file_count"`
	OutputPath      string  `json:"output_path,omitempty"`
	Tokenizer       string  `json:"tokenizer,omitempty"`
}

// Format outputs the summary as JSON
func (f *JSONFormatter) Format(w io.Writer, stats Stats) error {
	output := JSONOutput{
		OriginalBytes:  stats.OriginalBytes,
		DistilledBytes: stats.DistilledBytes,
		SavingsPercent: getCompressionRatio(stats.OriginalBytes, stats.DistilledBytes),
		DurationMS:     stats.Duration.Milliseconds(),
		FileCount:      stats.FileCount,
		OutputPath:     stats.OutputPath,
	}

	if stats.OriginalTokens > 0 && stats.DistilledTokens > 0 {
		output.TokensBefore = stats.OriginalTokens
		output.TokensAfter = stats.DistilledTokens
		output.TokensSaved = stats.OriginalTokens - stats.DistilledTokens
		output.TokenSavingsPct = getCompressionRatio(stats.OriginalTokens, stats.DistilledTokens)
		output.Tokenizer = "estimate (bytes/4)"
	}

	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(output)
}
