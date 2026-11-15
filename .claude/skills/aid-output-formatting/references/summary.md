# The summary

The line (or block) aid writes to stderr after a path run in the normal mode:
how much smaller the output is than the source, how long it took, and where
the output went. `internal/cli/root.go#runDistiller` fills
`internal/summary/formatter.go#Stats` and calls
`internal/summary/summary.go#Print`. It is printed when the run names a path
(not `-`) and `--summary-type` is not `off`. Stdin, git mode and the AI
actions print none (`.claude/skills/aid-architecture/SKILL.md`, section 3).
The snapshot suite and the conformance checks pass `--summary-type=off`;
`internal/conformance/cli_test.go` tests the summary itself.

## The numbers

- **Original size** is measured: the sum of the source files' sizes, which
  `internal/processor/processor.go#Processor.ProcessFile` records from
  `os.Stat` on every result
  (`internal/conformance/cli_test.go#TestSummaryReportsMeasuredSize`).
- **Distilled size** is the byte length of the output as written or printed,
  after the empty `<file>` blocks are removed and the
  `--show-ai-agent-instructions` paragraph is added
  (`internal/conformance/cli_test.go#TestSummaryDistilledBytesIsWrittenSize`).
  It is the size of whatever `--format` produced.
- **Tokens** are an estimate, not a count: `internal/summary/summary.go#EstimateTokens`
  divides the bytes by four (integer division). No tokenizer runs; the `json`
  summary says so with `"tokenizer": "estimate (bytes/4)"`.
- **Saved** is `(1 - distilled / original) * 100`.
- **File count** is the number of files formatted, including files whose
  `<file>` block was removed as empty.

## Formats

`internal/summary/summary.go#GetFormatter` picks one by `--summary-type`.
Every line below is real output for
`internal/conformance/testdata/python/sample.py` (454 bytes; distilled 272
bytes) with `NO_COLOR=1`; the duration varies from run to run:

```bash
export CGO_ENABLED=1 CGO_CFLAGS=-w
for t in visual-progress-bar stock-ticker speedometer-dashboard minimalist-sparkline ci-friendly json; do
  NO_COLOR=1 go run ./cmd/aid internal/conformance/testdata/python/sample.py --stdout --summary-type=$t </dev/null 2>&1 >/dev/null
done
```

| `--summary-type` (alias) | Output |
|---|---|
| `visual-progress-bar` (`bar`), the default | `✅ Distilled 1 file [██████░░░░░░░░░] 40.1% (454 B → 272 B) in 0ms` |
| `stock-ticker` (`ticker`) | `📊 AID 40.1% ▼ │ SIZE: 454 B→272 B │ TIME: 0ms │ EST: ~45 tokens saved` |
| `speedometer-dashboard` (`dashboard`) | a five-line box with a speed bar, a saved bar and `Tokens saved: ~45` |
| `minimalist-sparkline` (`sparkline`) | `✨ Distilled in 0ms. 📦 454 B → 272 B (40.1% saved). 🎟️ Tokens: ~113 → ~68.` |
| `ci-friendly` (`ci`) | `[aid] ✓ 40.1% saved \| 454 B → 272 B \| 0ms \| ~45 tokens saved` |
| `json` | an indented object: `original_bytes`, `distilled_bytes`, `savings_pct`, `duration_ms`, `tokens_before`, `tokens_after`, `tokens_saved`, `token_savings_pct`, `file_count`, `output_path` when a file was written, `tokenizer` |
| `off` | nothing |

Details that decide what you see:

- Any other value is not an error: the CI style is used when the `CI`
  environment variable is set or stderr is not a terminal, else the bar.
- The bar shows tokens only when more than 1,000 tokens were saved; the ticker
  when any were saved; the dashboard, sparkline, CI and `json` styles when both
  token estimates are above zero.
- The bar is coloured with ANSI codes unless `NO_COLOR` is set. `--no-emoji`
  drops the emoji of the bar, ticker, dashboard and sparkline styles and of
  their file lines; the CI style keeps its `✓`.
- Sizes are printed with SI units (`1.2 kB`), durations in milliseconds below
  one second, else as `1.2s`.
- When an output file was written, every style except `json` adds the path
  (`💾 Distilled output saved to: ...`, CI: `| saved to: ...`), and, unless the
  output was also printed with `--stdout`, a line telling an AI agent to load
  the whole file (`internal/summary/printed_test.go#TestPrintedOutputIsNotOfferedForLoading`).
