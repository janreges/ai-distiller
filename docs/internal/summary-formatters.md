# AI Distiller Summary Formatters

AI Distiller provides multiple summary output formats to suit different environments and preferences. After each distillation, a summary showing compression efficiency, processing time, and token savings is displayed to stderr.

## Available Formatters

### 1. Visual Progress Bar (Default)
**Flag:** `--summary-type=visual-progress-bar`

The default formatter for interactive terminal sessions. Shows a visual progress bar representing the compression ratio.

```
✨ Distilled 11 files [░░░░░░░░░░░░░░░] 87% (38 kB → 5.0 kB) in 1ms 💰 ~8k tokens saved (~1.3k remaining)
```

**Features:**
- Visual progress bar with colored dots (green dots for saved space, red dots for remaining)
- Dynamic emoji based on compression ratio
- Shows token savings when significant (>1000 tokens) with remaining token count
- No decimal places for compression ratios ≥80% for cleaner display
- Adapts bar width to terminal size (15 chars default, up to 20 on wide terminals)
- Color support disabled automatically when NO_COLOR is set or output is piped

### 2. Stock Ticker
**Flag:** `--summary-type=stock-ticker`

Displays results in a stock market ticker style format.

```
📊 AID 86.1% ▲ │ SIZE: 3.2 kB→444 B │ TIME: 0ms │ EST: ~686 tokens saved
```

**Features:**
- Compact single-line format
- Up/down arrow based on compression percentage
- Pipe-separated values for easy parsing
- Always shows token savings estimate

### 3. Speedometer Dashboard
**Flag:** `--summary-type=speedometer-dashboard`

A multi-line dashboard format with dual progress bars.

```
╔═══ AI Distiller ═══╗
║ Speed: ██████████ 100% ║ 0ms
║ Saved: ████████░░ 85.4% ║ 3.3 kB→482 B
║ Tokens saved: ~703      ║
╚═════════════════════╝
```

**Features:**
- Speed meter (based on processing time)
- Savings meter (compression ratio)
- Box-drawing characters for structure
- Token savings on separate line

### 4. Minimalist Sparkline
**Flag:** `--summary-type=minimalist-sparkline`

A compact single-line format with essential information.

```
✨ Distilled in 0ms. 📦 3.3 kB → 484 B (85.3% saved). 🎟️ Tokens: ~825 → ~121.
```

**Features:**
- Sentence-like structure
- Full token count information (before/after)
- Minimal visual clutter
- Emojis for visual appeal

### 5. CI-Friendly
**Flag:** `--summary-type=ci-friendly`

Optimized for CI/CD pipelines and log parsing.

```
[aid] ✓ 86.2% saved | 3.1 kB → 424 B | 0ms | ~662 tokens saved
```

**Features:**
- Clean, parseable format
- Pipe separators for easy field extraction
- No multi-line output
- Minimal Unicode characters

### 6. JSON
**Flag:** `--summary-type=json`

Machine-readable JSON format for programmatic processing.

```json
{
  "original_bytes": 3213,
  "distilled_bytes": 506,
  "savings_pct": 84.25,
  "duration_ms": 0,
  "tokens_before": 803,
  "tokens_after": 126,
  "tokens_saved": 677,
  "token_savings_pct": 84.31,
  "file_count": 1,
  "output_path": "/path/to/output.txt",
  "tokenizer": "estimate (bytes/4)"
}
```

**Features:**
- Complete metrics in structured format
- Includes tokenizer information
- Pretty-printed with indentation
- All percentage values as floats

### 7. Off
**Flag:** `--summary-type=off`

Disables summary output entirely.

## Automatic Selection

When `--summary-type` is not specified, AI Distiller automatically selects the most appropriate formatter:

1. **CI Environment** (`CI` env var set) → CI-Friendly formatter
2. **Non-TTY output** (piped/redirected) → CI-Friendly formatter  
3. **Interactive Terminal** → Visual Progress Bar formatter

## Additional Options

### Disable Emojis
**Flag:** `--no-emoji`

Removes all emoji characters from summary output (applies to all formatters except JSON).

```bash
# With emojis (default)
aid src/ --summary-type=stock-ticker
📊 AID 86.1% ▲ │ SIZE: 3.2 kB→444 B │ TIME: 0ms │ EST: ~686 tokens saved

# Without emojis
aid src/ --summary-type=stock-ticker --no-emoji
[AID] AID 86.1% ▲ │ SIZE: 3.2 kB→444 B │ TIME: 0ms │ EST: ~686 tokens saved
```

## Token Estimation

All formatters use a plain estimate, not a real tokenizer:
- Rough formula: ~1 token per 4 bytes for code
- Always prefixed with `~` to indicate estimation
- Tokenizer name included in JSON output

## Examples

```bash
# Default (visual progress bar)
aid ./src

# For CI/CD pipelines
aid ./src --summary-type=ci-friendly

# For data analysis
aid ./src --summary-type=json > metrics.json

# For terminals without emoji support
aid ./src --no-emoji

# Disable summary entirely
aid ./src --summary-type=off
```

## Implementation Details

The summary system is implemented in the `internal/summary` package with the following components:

- **Formatter Interface**: Common interface for all formatters
- **Stats Structure**: Contains all metrics (bytes, tokens, duration, etc.)
- **Auto-detection**: Environment-based formatter selection
- **Token Estimation**: Conservative estimate using bytes/4 ratio

Each formatter implements the same `Formatter` interface, ensuring consistent behavior and easy extensibility for future formats.