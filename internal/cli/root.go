package cli

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
	
	"github.com/spf13/cobra"
	"github.com/janreges/ai-distiller/internal/ai"
	"github.com/janreges/ai-distiller/internal/aiactions"
	"github.com/janreges/ai-distiller/internal/debug"
	"github.com/janreges/ai-distiller/internal/formatter"
	"github.com/janreges/ai-distiller/internal/ir"
	"github.com/janreges/ai-distiller/internal/processor"
	"github.com/janreges/ai-distiller/internal/project"
	"github.com/janreges/ai-distiller/internal/language"
	"github.com/janreges/ai-distiller/internal/version"
	"github.com/janreges/ai-distiller/internal/summary"
	_ "github.com/janreges/ai-distiller/internal/language" // Register language processors
)

var (
	// Version is set by main.go
	Version string

	// Flags
	outputFile       string
	outputToStdout   bool
	outputFormat     string
	stripOptions     []string // Deprecated, kept for backward compatibility
	includeGlob      []string
	excludeGlob      []string
	expandGlob       []string
	recursiveStr     string
	filePathType     string
	relativePathPrefix string
	verbosity        int
	langOverride     string
	
	// New filtering flags
	includePublic         *bool
	includeProtected      *bool
	includeInternal       *bool
	includePrivate        *bool
	includeComments       *bool
	includeDocstrings     *bool
	includeImplementation *bool
	includeImports        *bool
	includeAnnotations    *bool
	
	// Group flags
	includeList           string
	excludeList           string
	
	// Concurrency flags
	workers               int
	
	// Raw mode flag
	rawMode               bool
	
	// Git mode flags
	gitCommitLimit        int
	withAnalysisPrompt    bool
	
	// AI analysis task list flag (deprecated)
	aiAnalysisTaskList    bool
	
	// New AI action system
	aiAction             string
	aiOutput             string
	
	// Summary output flags
	summaryFormat        string
	noEmoji              bool
	
	// AI agent instructions flag
	showAiAgentInstructions bool
	
	// Import filtering flag
	filterImports        *bool
	
	// Fields and methods filtering flags
	fieldsFlag           string
	methodsFlag          string
	
	// Dependency-aware distillation flags
	dependencyAware      bool
	maxDepth            int
)

// rootCmd represents the base command
var rootCmd = &cobra.Command{
	Use:   "aid [path]",
	Short: "AI Distiller - Extract essential code structure for LLMs",
	Long: `AI Distiller (aid) intelligently "distills" source code from any project 
into a compact, structured format, optimized for the context window of 
Large Language Models (LLMs).

Special Git Mode: When you pass a .git directory path, aid switches to git log
mode and outputs formatted commit history instead of processing source files.

═══════════════════════════════════════════════════════════════════════════════

BASIC OPTIONS:
  -o, --output <file>          Output file (default: auto-generated)
  --stdout                     Print to stdout instead of file
  --format <type>              Output format: text|md|jsonl|json-structured|xml
                              (default: text)

PATH & OUTPUT CONTROL:
  --file-path-type <type>      How paths appear in output: relative|absolute
                              (default: relative)
  --relative-path-prefix <str> Custom prefix for relative paths (e.g., "src/")
  -r, --recursive              Process directories recursively
                              0/1 (default: 1)

───────────────────────────────────────────────────────────────────────────────

VISIBILITY FILTERING:
  Control which visibility levels are included in output
  
  --public                     Include public members
                              0/1 (default: 1)
  --protected                  Include protected members
                              0/1 (default: 0)
  --internal                   Include internal/package-private members
                              0/1 (default: 0)
  --private                    Include private members
                              0/1 (default: 0)

CONTENT FILTERING:
  Control what code elements are included
  
  --comments                   Include comments
                              0/1 (default: 0)
  --docstrings                 Include documentation
                              0/1 (default: 1)
  --implementation             Include function/method bodies
                              0/1 (default: 0)
  --imports                    Include import statements
                              0/1 (default: 1)
  --annotations                Include decorators/annotations
                              0/1 (default: 1)

ALTERNATIVE FILTERING:
  --include-only <items>       Include ONLY these categories (comma-separated)
  --exclude-items <items>      Exclude these categories (comma-separated)
                              Categories: public,protected,internal,private,
                              comments,docstrings,implementation,imports,annotations

───────────────────────────────────────────────────────────────────────────────

FILE SELECTION:
  --include <pattern>          Include file patterns (comma-separated)
                              Examples: "*.go", "*.go,*.py", "src/**/*.js"
  --exclude <pattern>          Exclude file patterns (comma-separated)
                              Examples: "*_test.go", "vendor/**", "*/tmp/*"
                              
                              Pattern types:
                              - Simple: "*.go" (matches Go files)
                              - Multiple: "*.go,*.py,*.js" (multiple extensions)
                              - Directory: "vendor/**" (exclude vendor tree)
                              - Path: "*/internal/*" (match path segments)
                              - Complex: "src/**/*.js" (recursive directory)

PROCESSING MODE:
  --raw                        Process all text files without parsing
  --lang <language>            Override language detection
                              Languages: auto|python|typescript|javascript|go|ruby|
                              swift|rust|java|csharp|kotlin|cpp|php
                              (default: auto)
  --tree-sitter                Use tree-sitter parser (experimental)

───────────────────────────────────────────────────────────────────────────────

PERFORMANCE:
  -w, --workers <num>          Number of parallel workers
                              0=auto (80% CPU), 1=serial, N=use N workers
                              (default: 0)

SUMMARY OUTPUT:
  --summary-type <type>        Summary format after processing
                              visual-progress-bar|stock-ticker|speedometer-dashboard|
                              minimalist-sparkline|ci-friendly|json|off
                              (default: visual-progress-bar)
  --no-emoji                   Disable emojis in summary output

DIAGNOSTICS:
  -v, --verbose                Verbose output (use -vv or -vvv for more)
  --strict                     Fail on first syntax error
  --version                    Show version information
  --help                       Show this help message

═══════════════════════════════════════════════════════════════════════════════

EXAMPLES:
  aid                          # Show this help message
  aid .                        # Process current dir, public APIs only
  aid src/ --private=1         # Include private members
  aid --file-path-type=absolute . # Use absolute paths in output
  aid docs/ --raw              # Process text files without parsing
  aid . -w 1                   # Force serial processing
  aid --relative-path-prefix="module/" docs/  # Add custom prefix to paths
  aid .git                     # Show git commit history (special mode)
  aid .git --git-limit=50      # Show latest 50 commits
  
  # File filtering examples:
  aid --exclude "*_test.go"    # Exclude test files
  aid --include "*.go,*.py"    # Only Go and Python files
  aid --exclude "vendor/**,node_modules/**"  # Skip dependency directories
  aid src/ --include "**/*.ts" --exclude "**/*.spec.ts"  # TypeScript without tests`,
	Args: cobra.MaximumNArgs(1),
	RunE: runDistiller,
}

// Execute runs the root command
func Execute() error {
	// Set custom error function for better error display
	rootCmd.SilenceErrors = true
	rootCmd.SilenceUsage = true
	
	if err := rootCmd.Execute(); err != nil {
		// Print error in red color
		red := "\033[31m"
		bold := "\033[1m"
		reset := "\033[0m"
		
		// Check if we should use colors
		useColor := os.Getenv("NO_COLOR") == ""
		if !useColor {
			red = ""
			bold = ""
			reset = ""
		}
		
		fmt.Fprintf(os.Stderr, "%s%sError: %s%s\n", red, bold, err.Error(), reset)
		
		// Show helpful usage for common errors
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "Usage:")
		fmt.Fprintln(os.Stderr, "  aid [path] [flags]")
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "Examples:")
		fmt.Fprintln(os.Stderr, "  aid .                  # Process current directory")
		fmt.Fprintln(os.Stderr, "  aid src/               # Process src directory")
		fmt.Fprintln(os.Stderr, "  aid main.py            # Process single file")
		fmt.Fprintln(os.Stderr, "  aid --help             # Show full help")
		fmt.Fprintln(os.Stderr, "  aid --help-extended    # Show extended documentation")
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "Common flags:")
		fmt.Fprintln(os.Stderr, "  --stdout               # Print to stdout instead of file")
		fmt.Fprintln(os.Stderr, "  --format md            # Output in Markdown format")
		fmt.Fprintln(os.Stderr, "  --private=1            # Include private members")
		fmt.Fprintln(os.Stderr, "  --implementation=1     # Include function bodies")
		
		return err
	}
	return nil
}

func init() {
	initFlags()
	
	// Initialize help system with custom templates and commands
	initializeHelpSystem()
	
	// Register all built-in AI actions
	// This is done here to avoid import cycles
	registerAIActions()
}

func initFlags() {
	// Output flags
	rootCmd.Flags().StringVarP(&outputFile, "output", "o", "", "Output file path (default: .aid.<dir>.[options].txt)")
	rootCmd.Flags().BoolVar(&outputToStdout, "stdout", false, "Print to stdout (in addition to file)")
	rootCmd.Flags().StringVar(&outputFormat, "format", "text", "Output format: md|text|jsonl|json-structured|xml (default: text)")

	// Legacy processing flags (deprecated)
	rootCmd.Flags().StringSliceVar(&stripOptions, "strip", nil, "DEPRECATED: Use individual filtering flags instead")
	rootCmd.Flags().MarkDeprecated("strip", "use individual filtering flags like --public=1, --private=0, etc.")
	
	// File pattern flags
	rootCmd.Flags().StringSliceVar(&includeGlob, "include", nil, "Include file patterns (comma-separated: *.go,*.py or use flag multiple times)")
	rootCmd.Flags().StringSliceVar(&excludeGlob, "exclude", nil, "Exclude file patterns (comma-separated: *.json,*test* or use flag multiple times)")
	rootCmd.Flags().StringSliceVar(&expandGlob, "expand", nil, "Keep full implementation only for symbols whose name matches these globs (comma-separated: GetUser,*Service* or use flag multiple times)")
	rootCmd.Flags().StringVarP(&recursiveStr, "recursive", "r", "1", "Process directories recursively (0/1, default: 1)")
	rootCmd.Flags().StringVar(&filePathType, "file-path-type", "relative", "How paths appear in output: relative|absolute (default: relative)")
	rootCmd.Flags().StringVar(&relativePathPrefix, "relative-path-prefix", "", "Custom prefix for relative paths (e.g., \"src/\")")

	// General flags
	rootCmd.Flags().CountVarP(&verbosity, "verbose", "v", "Verbose output (use -vv or -vvv for more detail)")
	rootCmd.Flags().Bool("version", false, "Show version information")
	rootCmd.Flags().Bool("help", false, "Show help message")
	

	// Language override flag
	rootCmd.Flags().StringVar(&langOverride, "lang", "auto", "Override language detection: auto|python|typescript|javascript|go|ruby|swift|rust|java|csharp|kotlin|cpp|php")
	
	// New filtering flags - visibility
	rootCmd.Flags().String("public", "1", "Include public members (0/1, default: 1)")
	rootCmd.Flags().String("protected", "0", "Include protected members (0/1, default: 0)")
	rootCmd.Flags().String("internal", "0", "Include internal/package-private members (0/1, default: 0)")
	rootCmd.Flags().String("private", "0", "Include private members (0/1, default: 0)")
	
	// New filtering flags - content
	rootCmd.Flags().String("comments", "0", "Include comments (0/1, default: 0)")
	rootCmd.Flags().String("docstrings", "1", "Include documentation comments (0/1, default: 1)")
	rootCmd.Flags().String("implementation", "0", "Include function/method bodies (0/1, default: 0)")
	rootCmd.Flags().String("imports", "1", "Include import statements (0/1, default: 1)")
	rootCmd.Flags().String("annotations", "1", "Include decorators/annotations (0/1, default: 1)")
	rootCmd.Flags().String("fields", "1", "Include fields/properties (0/1, default: 1)")
	rootCmd.Flags().String("methods", "1", "Include methods/functions (0/1, default: 1)")
	
	// Group filtering flags
	rootCmd.Flags().StringVar(&includeList, "include-only", "", "Include only these categories (comma-separated)")
	rootCmd.Flags().StringVar(&excludeList, "exclude-items", "", "Exclude these categories (comma-separated)")
	
	// Concurrency flags
	rootCmd.Flags().IntVarP(&workers, "workers", "w", 0, "Number of parallel workers (0=auto/80% CPU cores, 1=serial, default: 0)")
	
	// Raw mode flag
	rootCmd.Flags().BoolVar(&rawMode, "raw", false, "Raw mode: process all text files without parsing (txt, md, json, yaml, etc.")
	
	// Git mode flags
	rootCmd.Flags().IntVar(&gitCommitLimit, "git-limit", 200, "Limit number of commits in git mode (default: 200, 0=all)")
	rootCmd.Flags().BoolVar(&withAnalysisPrompt, "with-analysis-prompt", false, "Prepend AI analysis prompt to git output")
	
	// AI analysis task list flag (deprecated)
	rootCmd.Flags().BoolVar(&aiAnalysisTaskList, "ai-analysis-task-list", false, "DEPRECATED: Use --ai-action=flow-for-deep-file-to-file-analysis instead")
	rootCmd.Flags().MarkDeprecated("ai-analysis-task-list", "use --ai-action=flow-for-deep-file-to-file-analysis instead")
	
	// New AI action system
	rootCmd.Flags().StringVar(&aiAction, "ai-action", "", "AI action to perform on distilled output")
	rootCmd.Flags().StringVar(&aiOutput, "ai-output", "", "Output path for AI action (default: action-specific)")
	
	// Summary output flags
	rootCmd.Flags().StringVar(&summaryFormat, "summary-type", "visual-progress-bar", "Summary output format: visual-progress-bar|stock-ticker|speedometer-dashboard|minimalist-sparkline|ci-friendly|json|off (default: visual-progress-bar)")
	rootCmd.Flags().BoolVar(&noEmoji, "no-emoji", false, "Disable emojis in summary output")
	
	// AI agent instructions flag
	rootCmd.Flags().BoolVar(&showAiAgentInstructions, "show-ai-agent-instructions", false, "Show AI agent instructions at the beginning of text output")
	
	// Import filtering flag
	filterImports = new(bool)
	*filterImports = true // Default is true for text output
	rootCmd.Flags().BoolVar(filterImports, "filter-imports", true, "Filter unused imports in text output")
	
	// Dependency-aware distillation flags
	rootCmd.Flags().BoolVar(&dependencyAware, "dependency-aware", false, "Enable cross-file dependency analysis and call graph traversal")
	rootCmd.Flags().IntVar(&maxDepth, "max-depth", 2, "Maximum depth for dependency traversal (default: 2)")

	// Handle version flag specially
	rootCmd.PreRun = func(cmd *cobra.Command, args []string) {
		if v, _ := cmd.Flags().GetBool("version"); v {
			fmt.Printf("aid version %s\n", Version)
			if version.Date != "unknown" && version.Date != "" {
				// Parse and format the date
				if t, err := time.Parse(time.RFC3339, version.Date); err == nil {
					fmt.Printf("built: %s\n", t.Format("2006-01-02"))
				}
			}
			fmt.Printf("%s\n", version.WebsiteURL)
			os.Exit(0)
		}
		
		// Parse boolean flags
		parseBoolFlag(cmd, "public", &includePublic)
		parseBoolFlag(cmd, "protected", &includeProtected)
		parseBoolFlag(cmd, "internal", &includeInternal)
		parseBoolFlag(cmd, "private", &includePrivate)
		parseBoolFlag(cmd, "comments", &includeComments)
		parseBoolFlag(cmd, "docstrings", &includeDocstrings)
		parseBoolFlag(cmd, "implementation", &includeImplementation)
		parseBoolFlag(cmd, "imports", &includeImports)
		parseBoolFlag(cmd, "annotations", &includeAnnotations)
		
		// Parse fields and methods flags
		fieldsFlag, _ = cmd.Flags().GetString("fields")
		methodsFlag, _ = cmd.Flags().GetString("methods")
		
		// Validate mutually exclusive flags
		if includeList != "" && excludeList != "" {
			fmt.Fprintf(os.Stderr, "Error: --include-only and --exclude-items are mutually exclusive\n")
			os.Exit(1)
		}
	}
}

func runDistiller(cmd *cobra.Command, args []string) error {
	// Create debugger based on verbosity level
	dbg := debug.New(os.Stderr, verbosity)
	ctx := debug.NewContext(context.Background(), dbg)
	
	// Log startup info
	dbg.Logf(debug.LevelBasic, "AI Distiller %s starting", Version)
	
	// Check if stdin is available (not a TTY) or explicitly requested with "-"
	stdinAvailable := false
	if len(args) > 0 && args[0] == "-" {
		stdinAvailable = true
	} else {
		// Check if stdin is piped
		stat, _ := os.Stdin.Stat()
		if (stat.Mode() & os.ModeCharDevice) == 0 {
			stdinAvailable = true
		}
	}
	
	// Handle stdin input
	if stdinAvailable {
		return processStdinWithContext(ctx)
	}
	
	// If no arguments provided, show help
	if len(args) == 0 {
		return cmd.Help()
	}
	
	// Handle file/directory input
	inputPath := args[0]

	// Resolve absolute path
	absPath, err := filepath.Abs(inputPath)
	if err != nil {
		return fmt.Errorf("failed to resolve path: %w", err)
	}

	// Check if path exists
	if _, err := os.Stat(absPath); err != nil {
		return fmt.Errorf("path does not exist: %s", inputPath)
	}

	// Check if the path is a .git directory
	if filepath.Base(absPath) == ".git" {
		// For git mode, default to stdout unless output file is explicitly specified
		// This is different from regular mode where we auto-generate filenames
		return handleGitMode(ctx, absPath)
	}
	
	// Check if AI analysis task list flag is set (deprecated)
	if aiAnalysisTaskList {
		// Convert to new AI action system
		aiAction = "flow-for-deep-file-to-file-analysis"
		dbg.Logf(debug.LevelBasic, "Using deprecated --ai-analysis-task-list, converting to --ai-action=%s", aiAction)
	}
	
	// Check if AI action is specified
	if aiAction != "" {
		return handleAIAction(ctx, absPath)
	}

	// Generate output filename if not specified and not using stdout
	if outputFile == "" && !outputToStdout {
		outputFile = generateOutputFilename(absPath, stripOptions, outputFormat)
	}

	// Validate output format
	validFormats := []string{"md", "text", "jsonl", "json-structured", "xml"}
	if !contains(validFormats, outputFormat) {
		return fmt.Errorf("invalid output format: %s (valid: %s)", outputFormat, strings.Join(validFormats, ", "))
	}

	// Log configuration using debugger
	dbg.Logf(debug.LevelBasic, "Input: %s", absPath)
	dbg.Logf(debug.LevelBasic, "Output: %s", outputFile)
	dbg.Logf(debug.LevelBasic, "Format: %s", outputFormat)
	
	if len(stripOptions) > 0 {
		dbg.Logf(debug.LevelBasic, "Strip (deprecated): %s", strings.Join(stripOptions, ", "))
	}
	
	// Log detailed configuration at level 2
	dbg.Logf(debug.LevelDetailed, "Visibility: public=%v, protected=%v, internal=%v, private=%v",
		getBoolFlag(includePublic, true),
		getBoolFlag(includeProtected, false),
		getBoolFlag(includeInternal, false),
		getBoolFlag(includePrivate, false))
	dbg.Logf(debug.LevelDetailed, "Content: comments=%v, docstrings=%v, implementation=%v, imports=%v, annotations=%v, fields=%v, methods=%v",
		getBoolFlag(includeComments, false),
		getBoolFlag(includeDocstrings, true),
		getBoolFlag(includeImplementation, false),
		getBoolFlag(includeImports, true),
		getBoolFlag(includeAnnotations, true),
		parseBoolStringFlag(fieldsFlag, true),
		parseBoolStringFlag(methodsFlag, true))

	// Create processor options from flags
	procOpts := createProcessOptionsFromFlags()

	// Create the processor with context
	proc := processor.NewWithContext(ctx)
	

	// Log workers configuration
	actualWorkers := workers
	if actualWorkers == 0 {
		actualWorkers = int(float64(runtime.NumCPU()) * 0.8)
		if actualWorkers < 1 {
			actualWorkers = 1
		}
	}
	if workers != 1 {
		dbg.Logf(debug.LevelBasic, "Using %d parallel workers (%d CPU cores available)", actualWorkers, runtime.NumCPU())
	}

	// Set base path information
	procOpts.BasePath = inputPath
	procOpts.FilePathType = filePathType
	procOpts.RelativePathPrefix = relativePathPrefix
	
	// If user provided absolute path and didn't specify file-path-type, default to absolute
	if filepath.IsAbs(inputPath) && !cmd.Flags().Changed("file-path-type") {
		procOpts.FilePathType = "absolute"
	}
	
	// Track processing time
	startTime := time.Now()
	
	// Process the input
	result, err := proc.ProcessPath(absPath, procOpts)
	if err != nil {
		return fmt.Errorf("failed to process: %w", err)
	}
	if result == nil {
		return fmt.Errorf("no result returned from processing")
	}

	// Create formatter based on format
	formatterOpts := formatter.Options{
		ProcessingOptions: formatter.ProcessingInfo{
			IncludePublic:        procOpts.IncludePublic,
			IncludeProtected:     procOpts.IncludeProtected,
			IncludeInternal:      procOpts.IncludeInternal,
			IncludePrivate:       procOpts.IncludePrivate,
			IncludeImplementation: procOpts.IncludeImplementation,
			IncludeComments:      procOpts.IncludeComments,
			IncludeImports:       procOpts.IncludeImports,
			IncludeDocstrings:    procOpts.IncludeDocstrings,
			IncludeAnnotations:   procOpts.IncludeAnnotations,
			FilterImports:        *filterImports && outputFormat == "text",
		},
		DebugLevel: verbosity,
	}
	outputFormatter, err := formatter.Get(outputFormat, formatterOpts)
	if err != nil {
		return fmt.Errorf("failed to get formatter: %w", err)
	}

	// Write output
	var output strings.Builder
	
	// Log formatting phase
	dbg.Logf(debug.LevelDetailed, "Starting formatting phase with %s formatter", outputFormat)
	defer dbg.Timing(debug.LevelDetailed, fmt.Sprintf("formatting to %s", outputFormat))()
	
	// Handle different result types and count files
	var fileCount int
	switch r := result.(type) {
	case *ir.DistilledFile:
		dbg.Logf(debug.LevelDetailed, "Formatting single file: %s", r.Path)
		
		// Dump IR being formatted at trace level
		debug.Lazy(ctx, debug.LevelTrace, func(d debug.Debugger) {
			d.Dump(debug.LevelTrace, "IR being formatted", r)
		})
		
		if err := outputFormatter.Format(&output, r); err != nil {
			return fmt.Errorf("failed to format output: %w", err)
		}
		fileCount = 1
	case *ir.DistilledDirectory:
		// Extract files from directory
		var files []*ir.DistilledFile
		for _, child := range r.Children {
			if file, ok := child.(*ir.DistilledFile); ok {
				files = append(files, file)
			}
		}
		
		fileCount = len(files)
		dbg.Logf(debug.LevelDetailed, "Formatting %d files from directory", fileCount)
		
		if err := outputFormatter.FormatMultiple(&output, files); err != nil {
			return fmt.Errorf("failed to format output: %w", err)
		}
	default:
		return fmt.Errorf("unexpected result type: %T", result)
	}
	
	dbg.Logf(debug.LevelDetailed, "Formatted output size: %d bytes", output.Len())

	// Clean up empty file tags for text format
	outputStr := output.String()
	dbg.Logf(debug.LevelDetailed, "Output format: %s", outputFormat)
	
	// Add distillation instructions at the beginning of text output if enabled
	if outputFormat == "text" && fileCount > 0 && showAiAgentInstructions {
		instructions := generateDistillationInstructions(procOpts)
		outputStr = instructions + "\n\n" + outputStr
	}
	
	if outputFormat == "text" {
		// Remove empty file tags with only whitespace between them
		emptyFilePattern := regexp.MustCompile(`(?s)<file path="[^"]+">\s*</file>\s*`)
		cleanedStr := emptyFilePattern.ReplaceAllString(outputStr, "")
		dbg.Logf(debug.LevelDetailed, "Regex cleanup: before=%d bytes, after=%d bytes, pattern matches=%d", 
			len(outputStr), len(cleanedStr), len(emptyFilePattern.FindAllString(outputStr, -1)))
		outputStr = cleanedStr
	}

	// Write to file if not stdout-only
	if outputFile != "" && !outputToStdout {
		if err := os.WriteFile(outputFile, []byte(outputStr), 0644); err != nil {
			return fmt.Errorf("failed to write output file: %w", err)
		}
		dbg.Logf(debug.LevelBasic, "Wrote output to %s", outputFile)
	}

	// Write to stdout if requested
	if outputToStdout || outputFile == "" {
		fmt.Print(outputStr)
	}

	// Print advanced summary to stderr (only when not using stdin)
	if len(args) > 0 && args[0] != "-" && summaryFormat != "off" {
		// Calculate processing duration
		duration := time.Since(startTime)
		
		// Get original size
		var originalSize int64
		if rawMode {
			// For raw mode, the original size equals distilled size (no compression)
			originalSize = int64(output.Len())
		} else {
			originalSize = getOriginalSize(result)
		}
		distilledSize := int64(output.Len())
		
		// Create summary stats
		stats := summary.Stats{
			OriginalBytes:   originalSize,
			DistilledBytes:  distilledSize,
			OriginalTokens:  summary.EstimateTokens(originalSize),
			DistilledTokens: summary.EstimateTokens(distilledSize),
			Duration:        duration,
			FileCount:       fileCount,
			OutputPath:      outputFile,
			IsStdout:        outputToStdout || outputFile == "",
		}
		
		// Print summary
		summaryOpts := summary.Options{
			Format:  summaryFormat,
			NoColor: os.Getenv("NO_COLOR") != "",
			NoEmoji: noEmoji,
		}
		
		summary.Print(os.Stderr, stats, summaryOpts)
	}

	return nil
}

func generateOutputFilename(path string, stripOptions []string, format string) string {
	// Get project root and ensure .aid directory exists
	aidDir, err := project.EnsureAidDir()
	if err != nil {
		// Fallback to current directory
		aidDir = ".aid"
		os.MkdirAll(aidDir, 0755)
	}
	
	// Get directory name
	dirName := filepath.Base(path)
	if dirName == "." || dirName == "/" {
		dirName = "current"
	}

	// Build options suffix based on what's excluded from defaults
	var abbrev []string
	
	// Check if using new flag system
	if len(stripOptions) == 0 {
		// New flag system - check what differs from defaults
		if !getBoolFlag(includePublic, true) {
			abbrev = append(abbrev, "npub")
		}
		if getBoolFlag(includeProtected, false) {
			abbrev = append(abbrev, "prot")
		}
		if getBoolFlag(includeInternal, false) {
			abbrev = append(abbrev, "int")
		}
		if getBoolFlag(includePrivate, false) {
			abbrev = append(abbrev, "priv")
		}
		if getBoolFlag(includeComments, false) {
			abbrev = append(abbrev, "com")
		}
		if !getBoolFlag(includeDocstrings, true) {
			abbrev = append(abbrev, "ndoc")
		}
		if getBoolFlag(includeImplementation, false) {
			abbrev = append(abbrev, "impl")
		}
		if !getBoolFlag(includeImports, true) {
			abbrev = append(abbrev, "nimp")
		}
		if !getBoolFlag(includeAnnotations, true) {
			abbrev = append(abbrev, "nann")
		}
		if !parseBoolStringFlag(fieldsFlag, true) {
			abbrev = append(abbrev, "nfld")
		}
		if !parseBoolStringFlag(methodsFlag, true) {
			abbrev = append(abbrev, "nmth")
		}
	} else {
		// Legacy --strip system
		for _, opt := range stripOptions {
			switch opt {
			case "comments":
				abbrev = append(abbrev, "ncom")
			case "imports":
				abbrev = append(abbrev, "nimp")
			case "implementation":
				abbrev = append(abbrev, "nimpl")
			case "non-public":
				abbrev = append(abbrev, "npub")
			case "private":
				abbrev = append(abbrev, "npriv")
			case "protected":
				abbrev = append(abbrev, "nprot")
			}
		}
	}
	
	optionsSuffix := ""
	if len(abbrev) > 0 {
		optionsSuffix = "." + strings.Join(abbrev, ".")
	}

	// Determine file extension based on format
	ext := ".txt"
	switch format {
	case "md":
		ext = ".md"
	case "jsonl":
		ext = ".jsonl"
	case "json-structured":
		ext = ".json"
	case "xml":
		ext = ".xml"
	}
	
	// Generate filename within .aid directory
	filename := fmt.Sprintf("aid.%s%s%s", dirName, optionsSuffix, ext)
	return filepath.Join(aidDir, filename)
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// processStdinWithContext handles input from stdin with debugging context
func processStdinWithContext(ctx context.Context) error {
	dbg := debug.FromContext(ctx).WithSubsystem("stdin")
	
	// Force stdout output when reading from stdin
	outputToStdout = true
	
	dbg.Logf(debug.LevelBasic, "Processing input from stdin")
	
	// Read stdin into buffer for language detection
	var buffer bytes.Buffer
	tee := io.TeeReader(os.Stdin, &buffer)
	
	// Read up to 64kB for detection
	detectBuf := make([]byte, 64*1024)
	n, _ := tee.Read(detectBuf)
	detectBuf = detectBuf[:n]
	
	dbg.Logf(debug.LevelDetailed, "Read %d bytes for language detection", n)
	
	// Determine language
	lang := langOverride
	if lang == "auto" {
		detector := language.NewDetector()
		detectedLang, err := detector.DetectFromReader(bytes.NewReader(detectBuf))
		if err != nil {
			return fmt.Errorf("could not auto-detect language from stdin. Please specify with --lang flag")
		}
		lang = detectedLang
		dbg.Logf(debug.LevelBasic, "Detected language: %s", lang)
	} else {
		dbg.Logf(debug.LevelBasic, "Using specified language: %s", lang)
	}
	
	// Read the rest of stdin
	remainingBytes, _ := io.ReadAll(tee)
	fullContent := append(detectBuf, remainingBytes...)
	
	// Get language processor
	langProc, ok := language.GetProcessor(lang)
	if !ok {
		return fmt.Errorf("no processor found for language: %s", lang)
	}
	
	// Create processor options from flags
	procOpts := createProcessOptionsFromFlags()
	
	// Process the input with our debug-enabled context
	result, err := langProc.ProcessWithOptions(ctx, bytes.NewReader(fullContent), "stdin", procOpts)
	if err != nil {
		return fmt.Errorf("failed to process stdin: %w", err)
	}
	
	// Create formatter based on format
	formatterOpts := formatter.Options{
		ProcessingOptions: formatter.ProcessingInfo{
			IncludePublic:        procOpts.IncludePublic,
			IncludeProtected:     procOpts.IncludeProtected,
			IncludeInternal:      procOpts.IncludeInternal,
			IncludePrivate:       procOpts.IncludePrivate,
			IncludeImplementation: procOpts.IncludeImplementation,
			IncludeComments:      procOpts.IncludeComments,
			IncludeImports:       procOpts.IncludeImports,
			IncludeDocstrings:    procOpts.IncludeDocstrings,
			IncludeAnnotations:   procOpts.IncludeAnnotations,
			FilterImports:        *filterImports && outputFormat == "text",
		},
		DebugLevel: verbosity,
	}
	outputFormatter, err := formatter.Get(outputFormat, formatterOpts)
	if err != nil {
		return fmt.Errorf("failed to get formatter: %w", err)
	}
	
	// Format and output
	var output strings.Builder
	if err := outputFormatter.Format(&output, result); err != nil {
		return fmt.Errorf("failed to format output: %w", err)
	}
	
	// Always write to stdout for stdin input
	fmt.Print(output.String())
	
	return nil
}

// parseBoolFlag parses a string flag as boolean (0/1)
func parseBoolFlag(cmd *cobra.Command, name string, target **bool) {
	if val, err := cmd.Flags().GetString(name); err == nil && val != "" {
		b, err := strconv.ParseBool(val)
		if err != nil {
			// Try parsing as 0/1
			if val == "0" {
				b = false
			} else if val == "1" {
				b = true
			} else {
				fmt.Fprintf(os.Stderr, "Error: --%s must be 0 or 1, got %q\n", name, val)
				os.Exit(1)
			}
		}
		*target = &b
	}
}

// parseBoolStringFlag parses a string flag value to bool with default
func parseBoolStringFlag(value string, defaultValue bool) bool {
	if value == "" {
		return defaultValue
	}
	
	if value == "0" {
		return false
	} else if value == "1" {
		return true
	}
	
	if b, err := strconv.ParseBool(value); err == nil {
		return b
	}
	
	return defaultValue
}

// createProcessOptionsFromFlags creates ProcessOptions from the new flag system
func createProcessOptionsFromFlags() processor.ProcessOptions {
	opts := processor.ProcessOptions{}
	
	// Handle legacy --strip flag if present
	if len(stripOptions) > 0 {
		if verbosity > 0 {
			fmt.Fprintf(os.Stderr, "Warning: --strip is deprecated. Use individual filtering flags instead.\n")
		}
		// Apply legacy strip options
		opts.IncludeComments = !contains(stripOptions, "comments")
		opts.IncludeImports = !contains(stripOptions, "imports")
		opts.IncludeImplementation = !contains(stripOptions, "implementation")
		opts.IncludePrivate = !contains(stripOptions, "non-public")
		opts.RemovePrivateOnly = contains(stripOptions, "private")
		opts.RemoveProtectedOnly = contains(stripOptions, "protected")
		opts.Workers = workers
		opts.RawMode = rawMode
		opts.MaxDepth = maxDepth
		opts.SymbolResolution = dependencyAware
		return opts
	}
	
	// Process include/exclude lists if provided
	if includeList != "" {
		opts = processIncludeList(includeList)
		opts.Workers = workers
		opts.RawMode = rawMode
		opts.Recursive = recursiveStr != "0"
		opts.IncludePatterns = includeGlob
		opts.ExcludePatterns = excludeGlob
		opts.ExpandSymbols = expandGlob
		opts.MaxDepth = maxDepth
		opts.SymbolResolution = dependencyAware
		return opts
	}
	if excludeList != "" {
		opts = processExcludeList(excludeList)
		opts.Workers = workers
		opts.RawMode = rawMode
		opts.Recursive = recursiveStr != "0"
		opts.IncludePatterns = includeGlob
		opts.ExcludePatterns = excludeGlob
		opts.ExpandSymbols = expandGlob
		opts.MaxDepth = maxDepth
		opts.SymbolResolution = dependencyAware
		return opts
	}
	
	// Use individual flags with defaults
	// fmt.Printf("DEBUG: includeComments=%v\n", includeComments)
	opts.IncludeComments = getBoolFlag(includeComments, false)
	opts.IncludeImports = getBoolFlag(includeImports, true)
	opts.IncludeImplementation = getBoolFlag(includeImplementation, false)
	
	// Handle visibility flags
	includePublicVal := getBoolFlag(includePublic, true)
	includeProtectedVal := getBoolFlag(includeProtected, false)
	includeInternalVal := getBoolFlag(includeInternal, false)
	includePrivateVal := getBoolFlag(includePrivate, false)
	
	// Store visibility levels for formatter instructions
	opts.IncludePublic = includePublicVal
	opts.IncludeProtected = includeProtectedVal
	opts.IncludeInternal = includeInternalVal
	
	// Convert to stripper options
	// If only public is included, remove all non-public
	if includePublicVal && !includeProtectedVal && !includeInternalVal && !includePrivateVal {
		opts.IncludePrivate = false
	} else {
		opts.IncludePrivate = includePrivateVal
		// Set specific removal flags based on what's NOT included
		opts.RemovePrivateOnly = !includePrivateVal
		opts.RemoveProtectedOnly = !includeProtectedVal
		opts.RemoveInternalOnly = !includeInternalVal
	}
	
	// Handle docstrings and annotations
	opts.IncludeDocstrings = getBoolFlag(includeDocstrings, true)
	opts.IncludeAnnotations = getBoolFlag(includeAnnotations, true)
	
	// Handle fields and methods flags
	opts.IncludeFields = parseBoolStringFlag(fieldsFlag, true)
	opts.IncludeMethods = parseBoolStringFlag(methodsFlag, true)
	
	// Set workers value
	opts.Workers = workers
	
	// Set raw mode
	opts.RawMode = rawMode
	
	// Set recursive - parse from global recursiveStr
	opts.Recursive = recursiveStr != "0"
	
	// Set include/exclude patterns
	opts.IncludePatterns = includeGlob
	opts.ExcludePatterns = excludeGlob
	opts.ExpandSymbols = expandGlob
	
	// Dependency-aware distillation options
	opts.MaxDepth = maxDepth
	opts.SymbolResolution = dependencyAware
	
	return opts
}

// getBoolFlag returns the value of a bool flag or its default
func getBoolFlag(flag *bool, defaultVal bool) bool {
	if flag == nil {
		return defaultVal
	}
	return *flag
}

// processIncludeList processes the --include-only flag
func processIncludeList(list string) processor.ProcessOptions {
	opts := processor.ProcessOptions{
		// Start with everything excluded
		IncludeComments: false,
		IncludeImports: false,
		IncludeImplementation: false,
		IncludePrivate: false,
		IncludeDocstrings: false,
		IncludeAnnotations: false,
		IncludeFields: false,
		IncludeMethods: false,
	}
	
	// Track which visibility levels are included
	includePublic := false
	includeProtected := false
	includeInternal := false
	includePrivate := false
	
	items := strings.Split(list, ",")
	for _, item := range items {
		switch strings.TrimSpace(item) {
		case "public":
			includePublic = true
		case "protected":
			includeProtected = true
		case "internal":
			includeInternal = true
		case "private":
			includePrivate = true
		case "comments":
			opts.IncludeComments = true
		case "docstrings":
			opts.IncludeDocstrings = true
		case "implementation":
			opts.IncludeImplementation = true
		case "imports":
			opts.IncludeImports = true
		case "annotations":
			opts.IncludeAnnotations = true
		case "fields":
			opts.IncludeFields = true
		case "methods":
			opts.IncludeMethods = true
		}
	}
	
	// Apply visibility logic similar to normal flag processing
	if includePublic && !includeProtected && !includeInternal && !includePrivate {
		// Only public members
		opts.IncludePrivate = false
	} else if includePublic || includeProtected || includeInternal || includePrivate {
		// Some non-public visibility included
		opts.IncludePrivate = true
		opts.RemovePrivateOnly = !includePrivate
		opts.RemoveProtectedOnly = !includeProtected
		opts.RemoveInternalOnly = !includeInternal
		// If none explicitly included, we still need to show public
		if !includePublic && !includeProtected && !includeInternal && !includePrivate {
			opts.IncludePrivate = false
		}
	}
	
	return opts
}

// processExcludeList processes the --exclude-items flag
func processExcludeList(list string) processor.ProcessOptions {
	opts := processor.ProcessOptions{
		// Start with everything included
		IncludeComments: true,
		IncludeImports: true,
		IncludeImplementation: true,
		IncludePrivate: true,
		IncludeDocstrings: true,
		IncludeAnnotations: true,
		IncludeFields: true,
		IncludeMethods: true,
	}
	
	items := strings.Split(list, ",")
	for _, item := range items {
		switch strings.TrimSpace(item) {
		case "private":
			opts.RemovePrivateOnly = true
		case "protected":
			opts.RemoveProtectedOnly = true
		case "internal":
			// Internal is often grouped with private in many languages
			opts.RemovePrivateOnly = true
		case "comments":
			opts.IncludeComments = false
		case "docstrings":
			// TODO: Implement separate docstring handling
			opts.IncludeComments = false
		case "implementation":
			opts.IncludeImplementation = false
		case "imports":
			opts.IncludeImports = false
		case "annotations":
			opts.IncludeAnnotations = false
		case "fields":
			opts.IncludeFields = false
		case "methods":
			opts.IncludeMethods = false
		}
	}
	
	return opts
}

// handleGitMode processes git log when user passes a .git directory
func handleGitMode(ctx context.Context, gitPath string) error {
	dbg := debug.FromContext(ctx).WithSubsystem("git")
	dbg.Logf(debug.LevelBasic, "Git mode activated for: %s", gitPath)
	
	// Get the repository directory (parent of .git)
	repoPath := filepath.Dir(gitPath)
	
	// Build git log command with custom format
	// Using a rare delimiter to avoid conflicts with commit messages
	delimiter := "|||DELIMITER|||"
	// Format: hash | date | author name <email> | subject + body
	formatStr := fmt.Sprintf("--pretty=format:%%h%s%%ai%s%%an <%%ae>%s%%s%%n%%b", delimiter, delimiter, delimiter)
	
	// Build command args
	args := []string{"-C", repoPath, "log", formatStr}
	if gitCommitLimit > 0 {
		args = append(args, fmt.Sprintf("-n%d", gitCommitLimit))
	}
	
	cmd := exec.Command("git", args...)
	
	dbg.Logf(debug.LevelDetailed, "Running git command: %s", strings.Join(cmd.Args, " "))
	
	// Execute the command
	cmdOutput, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return fmt.Errorf("git log failed: %s\nStderr: %s", err, string(exitErr.Stderr))
		}
		return fmt.Errorf("failed to run git log: %w", err)
	}
	
	// Process the output to format it nicely
	lines := strings.Split(string(cmdOutput), "\n")
	var commits []GitCommit
	var currentCommit *GitCommit
	
	for _, line := range lines {
		if line == "" {
			continue
		}
		
		// Check if this is a new commit (contains the delimiter)
		if strings.Contains(line, delimiter) {
			// Save previous commit if exists
			if currentCommit != nil {
				commits = append(commits, *currentCommit)
			}
			
			// Parse new commit
			parts := strings.SplitN(line, delimiter, 4)
			if len(parts) >= 4 {
				currentCommit = &GitCommit{
					Hash:    parts[0],
					Date:    parts[1],
					Author:  parts[2],
					Message: parts[3],
				}
			}
		} else if currentCommit != nil {
			// This is a continuation of the commit message (body)
			currentCommit.Message += "\n" + line
		}
	}
	
	// Don't forget the last commit
	if currentCommit != nil {
		commits = append(commits, *currentCommit)
	}
	
	dbg.Logf(debug.LevelBasic, "Found %d commits", len(commits))
	
	// Format and output the commits
	var output strings.Builder
	
	// Add analysis prompt if requested
	if withAnalysisPrompt {
		output.WriteString(getGitAnalysisPrompt())
		output.WriteString("\n\n=== BEGIN GIT LOG ===\n")
	}
	
	for i, commit := range commits {
		if i > 0 {
			output.WriteString("\n")
		}
		
		// Extract just the date part (without timezone) for cleaner display
		dateParts := strings.Fields(commit.Date)
		cleanDate := commit.Date
		if len(dateParts) >= 2 {
			cleanDate = dateParts[0] + " " + dateParts[1]
		}
		
		// Format the commit header in a clean, single-line format
		// Format: [hash] YYYY-MM-DD HH:MM:SS | author | subject
		message := strings.TrimSpace(commit.Message)
		lines := strings.Split(message, "\n")
		subject := lines[0]
		if len(subject) > 80 {
			subject = subject[:77] + "..."
		}
		
		// Extract author name without email for cleaner display
		author := commit.Author
		if idx := strings.Index(author, " <"); idx > 0 {
			author = author[:idx]
		}
		// Truncate long author names
		if len(author) > 20 {
			author = author[:17] + "..."
		}
		
		fmt.Fprintf(&output, "[%s] %s | %-20s | %s\n", commit.Hash, cleanDate, author, subject)
		
		// Format the rest of the message (body) with proper indentation
		if len(lines) > 1 {
			for i := 1; i < len(lines); i++ {
				line := strings.TrimSpace(lines[i])
				if line != "" {
					fmt.Fprintf(&output, "        %s\n", line)
				}
			}
		}
	}
	
	// Add closing tag if analysis prompt was used
	if withAnalysisPrompt {
		output.WriteString("\n=== END GIT LOG ===\n")
	}
	
	// Write to file if specified
	if outputFile != "" {
		if err := os.WriteFile(outputFile, []byte(output.String()), 0644); err != nil {
			return fmt.Errorf("failed to write output file: %w", err)
		}
		absOutputFile, _ := filepath.Abs(outputFile)
		dbg.Logf(debug.LevelBasic, "Wrote git log to %s", absOutputFile)
		if !outputToStdout {
			fmt.Fprintf(os.Stderr, "Git log written to: %s\n", absOutputFile)
		}
	}
	
	// Write to stdout only if explicitly requested or no output file specified
	if outputToStdout || outputFile == "" {
		fmt.Print(output.String())
	}
	
	return nil
}

// GitCommit represents a single git commit
type GitCommit struct {
	Hash    string
	Date    string
	Author  string
	Message string
}

// getGitAnalysisPrompt returns the AI analysis prompt for git history
func getGitAnalysisPrompt() string {
	// Try to load from template file
	data := aiactions.CreateTemplateData("git-project")
	prompt, err := aiactions.LoadTemplate("git-analysis", data)
	if err != nil {
		// Fallback to embedded prompt if template loading fails
		return `You are a seasoned software archeologist and senior engineer.
Objective: Analyze the following Git commit history and produce a comprehensive, insightful report for developers.

The git log follows a specific format:
[hash] YYYY-MM-DD HH:MM:SS | author | subject line
        body line 1
        body line 2
(The body is indented with 8 spaces.)

Please provide a comprehensive analysis including contributor statistics, commit quality scores, 
development timeline visualization, complexity analysis, and actionable recommendations.`
	}
	return prompt
}

// handleAIAnalysisTaskList generates a comprehensive AI analysis task list for the project
func handleAIAnalysisTaskList(ctx context.Context, projectPath string) error {
	dbg := debug.FromContext(ctx).WithSubsystem("ai-analysis")
	dbg.Logf(debug.LevelBasic, "AI Analysis Task List mode activated for: %s", projectPath)
	
	// Get project basename and current date
	basename := filepath.Base(projectPath)
	currentDate := fmt.Sprintf("%04d-%02d-%02d", 
		time.Now().Year(), time.Now().Month(), time.Now().Day())
	
	// Create .aid directory structure
	aidDir := filepath.Join(projectPath, ".aid")
	analysisDir := filepath.Join(aidDir, fmt.Sprintf("analysis.%s", basename), currentDate)
	
	if err := os.MkdirAll(analysisDir, 0755); err != nil {
		return fmt.Errorf("failed to create .aid directory structure: %w", err)
	}
	
	dbg.Logf(debug.LevelBasic, "Created directory structure: %s", analysisDir)
	
	// Collect all source files
	sourceFiles, err := collectSourceFiles(projectPath)
	if err != nil {
		return fmt.Errorf("failed to collect source files: %w", err)
	}
	
	dbg.Logf(debug.LevelBasic, "Found %d source files to analyze", len(sourceFiles))
	
	// Pre-create all individual report directories to avoid mkdir operations during analysis
	for _, file := range sourceFiles {
		reportDir := filepath.Join(analysisDir, filepath.Dir(file))
		if err := os.MkdirAll(reportDir, 0755); err != nil {
			return fmt.Errorf("failed to create report directory %s: %w", reportDir, err)
		}
	}
	
	dbg.Logf(debug.LevelBasic, "Pre-created all report directories for %d files", len(sourceFiles))
	
	// Generate task list file
	taskListFile := filepath.Join(aidDir, fmt.Sprintf("ANALYSIS-TASK-LIST.%s.%s.md", basename, currentDate))
	if err := generateTaskList(taskListFile, basename, currentDate, sourceFiles); err != nil {
		return fmt.Errorf("failed to generate task list: %w", err)
	}
	
	// Generate summary file with headers
	summaryFile := filepath.Join(aidDir, fmt.Sprintf("ANALYSIS-SUMMARY.%s.%s.md", basename, currentDate))
	if err := generateSummaryFile(summaryFile, basename, currentDate); err != nil {
		return fmt.Errorf("failed to generate summary file: %w", err)
	}
	
	dbg.Logf(debug.LevelBasic, "Generated task list: %s", taskListFile)
	dbg.Logf(debug.LevelBasic, "Generated summary file: %s", summaryFile)
	
	// Output information to user
	fmt.Printf("✅ AI Analysis Task List generated successfully!\n\n")
	fmt.Printf("📋 Task List: %s\n", taskListFile)
	fmt.Printf("📊 Summary File: %s\n", summaryFile)
	fmt.Printf("📁 Analysis Reports Directory: %s\n\n", analysisDir)
	fmt.Printf("🤖 Ready for AI-driven analysis workflow!\n")
	fmt.Printf("   Files to analyze: %d\n", len(sourceFiles))
	fmt.Printf("   Analysis structure created in: %s\n", aidDir)
	
	return nil
}

// collectSourceFiles recursively collects all source files from the project directory
func collectSourceFiles(projectPath string) ([]string, error) {
	var sourceFiles []string
	
	// Load git submodules to skip them
	gitSubmodules, err := loadGitSubmodules(projectPath)
	if err != nil {
		// Not a git repo or no submodules - continue without error
		gitSubmodules = make(map[string]bool)
	}
	
	// Define source file extensions (focus on core programming languages)
	sourceExtensions := map[string]bool{
		// Core programming languages
		".py": true, ".js": true, ".ts": true, ".tsx": true, ".jsx": true,
		".go": true, ".java": true, ".kt": true, ".kts": true,
		".rs": true, ".swift": true, ".rb": true, ".php": true,
		".cpp": true, ".cc": true, ".cxx": true, ".c": true, ".h": true, ".hpp": true,
		".cs": true, ".fs": true, ".vb": true,
		".scala": true, ".clj": true, ".cljs": true,
		
		// Frontend frameworks and templates
		".vue": true, ".svelte": true, ".astro": true,
		".twig": true, ".tpl": true, ".latte": true, ".j2": true, ".jinja": true, ".jinja2": true,
		".hbs": true, ".handlebars": true, ".mustache": true, ".ejs": true, ".pug": true, ".jade": true,
		".blade": true, ".razor": true, ".cshtml": true, ".vbhtml": true,
		
		// Web technologies
		".html": true, ".htm": true, ".xhtml": true,
		".css": true, ".scss": true, ".sass": true, ".less": true, ".styl": true, ".stylus": true,
		".xml": true, ".xsl": true, ".xslt": true, ".svg": true,
		
		// Configuration and data files
		".json": true, ".json5": true, ".jsonc": true,
		".yaml": true, ".yml": true, ".neon": true,
		".toml": true, ".ini": true, ".cfg": true, ".conf": true,
		".env": true, ".properties": true,
		
		// Documentation and markup
		".md": true, ".mdx": true, ".rst": true, ".txt": true, ".asciidoc": true, ".adoc": true,
		
		// Scripts and shell
		".sh": true, ".bash": true, ".zsh": true, ".fish": true, ".bat": true, ".cmd": true, ".ps1": true,
		".awk": true, ".sed": true,
		
		// Database and query languages
		".sql": true, ".psql": true, ".mysql": true, ".sqlite": true, ".graphql": true, ".gql": true,
		
		// Keep all languages - users can control scope via directory selection or --exclude
	}
	
	// Skip these directories
	skipDirs := map[string]bool{
		"node_modules": true, ".git": true, ".svn": true, ".hg": true,
		"__pycache__": true, ".pytest_cache": true,
		"target": true, "build": true, "dist": true, "out": true,
		".aid": true, // Skip our own analysis directory
		"vendor": true, ".vscode": true, ".idea": true,
		"coverage": true, ".coverage": true, ".nyc_output": true,
		"grammars": true, // Skip tree-sitter grammars
		"test-data": true, // Skip test data files
		"testdata": true, // Skip test data files (alternative naming)
		"docs": true, // Skip documentation
		"examples": true, // Skip example code
		"assets": true, // Skip assets
		"static": true, // Skip static files
	}
	
	err = filepath.Walk(projectPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		
		// Skip directories that we don't want to analyze
		if info.IsDir() {
			dirName := filepath.Base(path)
			
			// Check if this is a git submodule directory
			relPath, err := filepath.Rel(projectPath, path)
			if err == nil && gitSubmodules[relPath] {
				return filepath.SkipDir
			}
			
			// Skip directories by name or if they contain "grammars" in their path
			if skipDirs[dirName] || strings.HasPrefix(dirName, ".") && dirName != "." {
				return filepath.SkipDir
			}
			
			// Skip any directory path containing tree-sitter related content
			if err == nil && (strings.Contains(relPath, "/grammars") || 
							  strings.Contains(relPath, "grammars/") ||
							  strings.Contains(relPath, "tree-sitter") ||
							  strings.Contains(relPath, "parser/grammars") ||
							  strings.HasPrefix(relPath, "internal/parser/grammars") ||
							  relPath == "grammars") {
				return filepath.SkipDir
			}
			
			return nil
		}
		
		// Check if it's a source file
		ext := strings.ToLower(filepath.Ext(path))
		if sourceExtensions[ext] {
			// Make path relative to project directory
			relPath, err := filepath.Rel(projectPath, path)
			if err == nil {
				// Skip specific unwanted files
				fileName := filepath.Base(relPath)
				
				// Skip generated files and AI Distiller output files
				if strings.HasPrefix(fileName, ".aid.") ||
				   strings.HasPrefix(fileName, ".") ||
				   strings.Contains(fileName, "generated") ||
				   strings.Contains(fileName, ".generated.") ||
				   strings.Contains(relPath, "/parser.c") ||
				   strings.Contains(relPath, "/scanner.c") ||
				   strings.Contains(relPath, "tree-sitter") ||
				   strings.Contains(relPath, "/grammars/") ||
				   strings.Contains(relPath, "/src/parser.c") ||
				   strings.Contains(relPath, "/src/scanner.c") ||
				   strings.Contains(fileName, "grammar.js") {
					return nil
				}
				
				// Apply include/exclude filters if specified
				if shouldIncludeFile(relPath, includeGlob, excludeGlob) {
					sourceFiles = append(sourceFiles, relPath)
				}
			}
		}
		
		return nil
	})
	
	return sourceFiles, err
}

// generateTaskList creates the main task list file with checkboxes for each source file
func generateTaskList(taskListFile, basename, currentDate string, sourceFiles []string) error {
	var content strings.Builder
	
	// Write header
	content.WriteString(fmt.Sprintf("# AI Distiller – Comprehensive Code Analysis Task List\n\n"))
	content.WriteString(fmt.Sprintf("**Project:** `%s`  \n", basename))
	content.WriteString(fmt.Sprintf("**Analysis Date:** %s  \n", currentDate))
	content.WriteString(fmt.Sprintf("**Total Files:** %d  \n\n", len(sourceFiles)))
	
	// Write comprehensive prompt
	content.WriteString(getAIAnalysisPrompt(basename, currentDate))
	content.WriteString("\n\n")
	
	// Write the task list
	content.WriteString("## 📋 Analysis Task List\n\n")
	content.WriteString("Complete each task in order, checking off items as you finish:\n\n")
	
	// Task 1: Create summary file
	content.WriteString(fmt.Sprintf("- [ ] **1. Initialize Analysis Summary**  \n"))
	content.WriteString(fmt.Sprintf("      Create `./aid/ANALYSIS-SUMMARY.%s.%s.md` with project overview\n\n", basename, currentDate))
	
	// Tasks for each file
	for i, file := range sourceFiles {
		taskNum := i + 2
		content.WriteString(fmt.Sprintf("- [ ] **%d. Analyze `%s`**  \n", taskNum, file))
		content.WriteString(fmt.Sprintf("      → Create report: `./aid/analysis.%s/%s/%s.md`  \n", basename, currentDate, file))
		content.WriteString(fmt.Sprintf("      → Add summary row to ANALYSIS-SUMMARY file\n\n"))
	}
	
	// Final task: Generate conclusion
	finalTaskNum := len(sourceFiles) + 2
	content.WriteString(fmt.Sprintf("- [ ] **%d. Generate Project Conclusion**  \n", finalTaskNum))
	content.WriteString(fmt.Sprintf("      Read completed ANALYSIS-SUMMARY file and write comprehensive conclusion\n\n"))
	
	// Write workflow notes
	content.WriteString("## 🔄 Workflow Notes\n\n")
	content.WriteString("- Check off each task **[x]** only after completing BOTH the individual report AND the summary row\n")
	content.WriteString("- Follow the exact file naming conventions specified\n")
	content.WriteString("- Use the standardized analysis format provided in the prompt\n")
	content.WriteString("- Maintain consistent scoring across all files\n")
	content.WriteString("- The final conclusion should synthesize findings from the entire summary table\n\n")
	content.WriteString("## 💡 Scope Control Tips\n\n")
	content.WriteString("If this task list is too large:\n")
	content.WriteString("- **Analyze specific directories**: `aid internal/cli --ai-analysis-task-list`\n")
	content.WriteString("- **Exclude test files**: `aid --exclude \"*test*,*spec*\" --ai-analysis-task-list`\n")
	content.WriteString("- **Focus on core languages**: `aid --include \"*.go,*.py,*.ts,*.php\" --ai-analysis-task-list`\n")
	content.WriteString("- **Multiple exclusions**: `aid --exclude \"*.json\" --exclude \"*.yml\" --ai-analysis-task-list`\n")
	content.WriteString("- **Only Vue/Svelte**: `aid --include \"*.vue,*.svelte\" --ai-analysis-task-list`\n")
	content.WriteString("- **Template files**: `aid --include \"*.twig,*.latte,*.j2\" --ai-analysis-task-list`\n")
	content.WriteString("- **Skip config files**: `aid --exclude \"*.json,*.yaml,*.yml,*.env,*.ini\" --ai-analysis-task-list`\n")
	content.WriteString("- **Skip large directories**: Use directory selection instead of --exclude for dirs\n\n")
	
	content.WriteString("---\n")
	content.WriteString("*Generated by AI Distiller – Comprehensive Code Analysis System*\n")
	
	return os.WriteFile(taskListFile, []byte(content.String()), 0644)
}

// generateSummaryFile creates the summary file with headers and initial content
func generateSummaryFile(summaryFile, basename, currentDate string) error {
	var content strings.Builder
	
	content.WriteString(fmt.Sprintf("# Project Analysis Summary – %s (%s)\n\n", basename, currentDate))
	
	content.WriteString("## 📊 Overview\n\n")
	content.WriteString("This document provides a comprehensive analysis summary of the entire codebase. ")
	content.WriteString("Each file has been individually analyzed for security, maintainability, performance, ")
	content.WriteString("and readability. The results are compiled in the table below.\n\n")
	
	content.WriteString("## 📈 Analysis Results\n\n")
	content.WriteString("| File | Security % | Maintainability % | Performance % | Readability % | Critical | High | Medium | Low |\n")
	content.WriteString("|------|:----------:|:-----------------:|:-------------:|:-------------:|:--------:|:----:|:------:|:---:|\n")
	
	// Note: Individual file rows will be appended here during analysis
	// The final conclusion section will be added by the last task
	
	return os.WriteFile(summaryFile, []byte(content.String()), 0644)
}

// getAIAnalysisPrompt returns the comprehensive prompt for AI-driven code analysis
func getAIAnalysisPrompt(basename, currentDate string) string {
	infrastructureInfo := "### 🚀 Pre-Created Infrastructure\n" +
		"- **All report directories have been pre-created** - no need to run mkdir commands\n" +
		"- **Individual reports go to**: `.aid/analysis." + basename + "/" + currentDate + "/[file-path].md`\n" +
		"- **Summary table updates**: `.aid/ANALYSIS-SUMMARY." + basename + "." + currentDate + ".md`"

	return fmt.Sprintf(`## 🤖 AI Analysis Instructions

# CRITICAL EXECUTION MANDATE: Unbreakable Sequential Processing

This is a FORMAL PROTOCOL implementing Chain-of-Thought (CoT) analysis with ZERO tolerance for deviations.

## ABSOLUTE PROHIBITIONS ⛔

1. **PROHIBITED**: Batch processing multiple files simultaneously
2. **PROHIBITED**: Using any "time-saving" shortcuts or optimizations  
3. **PROHIBITED**: Skipping individual file analysis for "efficiency"
4. **PROHIBITED**: Marking tasks complete before ALL outputs are verified
5. **PROHIBITED**: Referencing or planning for files not yet in scope
6. **VIOLATION CONSEQUENCE**: Any deviation = IMMEDIATE PROTOCOL FAILURE

You are an **Expert Senior Staff Engineer and Security Auditor** conducting a comprehensive file-by-file analysis of the **%s** project. Follow these instructions precisely:

### 📋 Your Mission
1. **Analyze each file individually** using the standardized template below
2. **Score each file** across 4 dimensions: Security, Maintainability, Performance, Readability
3. **Generate two outputs** for each file:
   - Detailed analysis report (saved as individual .md file)
   - Summary table row (appended to ANALYSIS-SUMMARY file)

%s

### 🎯 Analysis Dimensions & Scoring

**Scoring Scale**: 0-100%% (start at 100%%, deduct points for issues)

**Deduction Guide**:
- **Critical Issue**: -30 points (exposed secrets, clear RCE, unreadable god functions)
- **High Issue**: -15 points (SQL injection risk, complex unmaintainable code)  
- **Medium Issue**: -5 points (inefficient patterns, poor naming, missing docs)
- **Low Issue**: -2 points (minor style issues, trivial improvements)
- **Info**: 0 points (observations, TODO comments)

**Categories**:
1. **Security**: Vulnerabilities, exposed secrets, dangerous patterns
2. **Maintainability**: Code complexity, structure, documentation quality
3. **Performance**: Efficiency, scalability concerns, resource usage
4. **Readability**: Code clarity, naming, organization, comments

### 📝 Required Report Template

For each file, create the corresponding report file:

` + "```markdown" + `
# File Analysis: [FILEPATH]
*Analysis Date: 2025-06-17*  
*Analyst: AI Distiller*

| Metric | Score |
|--------|-------|
| Security (%%) | **[SCORE]** |
| Maintainability (%%) | **[SCORE]** |
| Performance (%%) | **[SCORE]** |
| Readability (%%) | **[SCORE]** |

## 🔍 Executive Summary
One paragraph overview of file purpose and critical findings.

## 🛡️ Security Analysis
List vulnerabilities with severity, line numbers, and mitigations.

## ⚡ Performance Analysis  
Identify bottlenecks, complexity issues, optimization opportunities.

## 🔧 Maintainability Analysis
Code structure, complexity, documentation, technical debt.

## 📖 Readability Analysis
Code clarity, naming conventions, organization, comments.

## 🎯 Recommendations
Prioritized action items with impact assessment.

### Scoring Rationale
Explain how each percentage was calculated.
` + "```" + `

### 📊 Required Summary Row

After completing each file analysis, append ONE row to the ANALYSIS-SUMMARY file:

| filepath | sec%% | maint%% | perf%% | read%% | critical | high | medium | low |

### 🎨 Visual Formatting Guidelines

**For Critical/High Issue Counts** (use HTML spans for color):
- **Critical issues**: ` + "`" + `<span style="color:#ff0000; font-weight: bold">3</span>` + "`" + ` (red + bold)
- **High issues**: ` + "`" + `<span style="color:#ff6600; font-weight: bold">2</span>` + "`" + ` (orange + bold)
- **Medium issues**: ` + "`" + `<span style="color:#ffaa00">1</span>` + "`" + ` (yellow)
- **Low issues**: regular text

**For Low Scores** (< 70%%):
- **Scores < 50%%**: ` + "`" + `<span style="color:#ff0000; font-weight: bold">45</span>` + "`" + ` (red + bold)
- **Scores 50-69%%**: ` + "`" + `<span style="color:#ff6600; font-weight: bold">65</span>` + "`" + ` (orange + bold)
- **Scores 70-89%%**: regular text
- **Scores 90-100%%**: ` + "`" + `<span style="color:#00aa00; font-weight: bold">95</span>` + "`" + ` (green + bold)

**Project-Level Conclusion Section** should use larger fonts and colors:
- ` + "`" + `<h2 style="color:#ff0000;">🚨 CRITICAL ISSUES FOUND</h2>` + "`" + `
- ` + "`" + `<h3 style="color:#00aa00;">✅ Overall Project Health: <span style="font-size: 1.5em; color:#00aa00;">GOOD</span></h3>` + "`" + `

### ⚡ Workflow Rules

1. **Process files in task list order**
2. **Complete BOTH outputs before checking off task**
3. **Use consistent scoring methodology**
4. **All directories are pre-created** - just write files directly
5. **Apply visual formatting** to summary rows for better readability
6. **After all files: write colorful project conclusion in SUMMARY file**

## MANDATORY EXECUTION LOOP WITH ERROR HANDLING

` + "```python" + `
# REQUIRED BEHAVIOR - MUST BE FOLLOWED EXACTLY
current_file_only = True  # INVARIANT: Only process the current file

for task in task_list:
    if task.status in ["completed", "failed"]:
        continue
    
    try:
        # CHECKPOINT 1: Acknowledge current file
        print(f"[CHECKPOINT] Starting analysis for: {task.file_path}")
        update_task_status(task.file_path, "in_progress")
        
        # STEP 1: Read ONLY current file
        file_content = read_file(task.file_path)
        file_hash = compute_sha256(file_content)
        
        # STEP 2: Analyze THIS file comprehensively
        detailed_report = analyze_file_comprehensively(file_content)
        assert detailed_report is not None
        
        # STEP 3: Save detailed report
        save_report(detailed_report, task.report_path)
        print(f"[CHECKPOINT] Saved report: {task.report_path}")
        
        # STEP 4: Extract and append summary
        summary_row = extract_summary_metrics(detailed_report)
        append_to_summary_table(summary_row)
        print(f"[CHECKPOINT] Added summary row for: {task.file_path}")
        
        # STEP 5: Verify outputs exist
        assert file_exists(task.report_path)
        assert summary_row_in_table(task.file_name)
        
        # STEP 6: Mark complete ONLY after verification
        update_task_status(task.file_path, "completed")
        print(f"[CHECKPOINT] Completed: {task.file_path}")
        
        # MANDATORY: Acknowledge completion before next file
        print(f"[CONFIRMATION] File {task.file_path} fully processed.")
        print(f"[CONFIRMATION] Moving to next file in sequence.")
        
    except Exception as e:
        # FAILURE PATH - Log and continue
        log_error(task.file_path, str(e))
        update_task_status(task.file_path, "failed")
        print(f"[ERROR] Failed to process {task.file_path}: {e}")
        continue
` + "```" + `

## PROGRESSIVE VALIDATION REQUIREMENTS

After EACH file, you MUST output:
1. "[CHECKPOINT] Starting analysis for: [filename]"
2. "[CHECKPOINT] Saved report: [path]"
3. "[CHECKPOINT] Added summary row for: [filename]"
4. "[CONFIRMATION] File [filename] fully processed."
5. "[CONFIRMATION] Moving to next file in sequence."

## WHY THIS PROTOCOL EXISTS

Individual sequential analysis ensures:
- Consistent scoring methodology across all files
- Detailed insights not possible in batch processing
- Progressive understanding of codebase patterns
- Traceable audit path for each decision
- Error isolation (one file failure doesn't affect others)

## CONTEXT LIMITATION

You will NOT be shown:
- The full list of remaining files
- Directory listings beyond current scope
- Any information that enables planning ahead

This is BY DESIGN to enforce sequential processing.

### 🏁 Final Task

After analyzing all files, read the complete ANALYSIS-SUMMARY table and write a comprehensive "Project-Level Conclusion" section covering:
- Overall project health scores (averages)
- Top 3-5 highest-risk files requiring immediate attention  
- Recurring patterns and systemic issues
- Strategic recommendations for the development team

---

**Ready to begin comprehensive codebase analysis!**`, basename, infrastructureInfo)
}

// loadGitSubmodules loads git submodules from .gitmodules file to exclude them from analysis
func loadGitSubmodules(projectPath string) (map[string]bool, error) {
	submodules := make(map[string]bool)
	
	gitmodulesPath := filepath.Join(projectPath, ".gitmodules")
	
	// Check if .gitmodules file exists
	if _, err := os.Stat(gitmodulesPath); os.IsNotExist(err) {
		return submodules, nil // No submodules
	}
	
	// Read .gitmodules file
	content, err := os.ReadFile(gitmodulesPath)
	if err != nil {
		return submodules, err
	}
	
	// Parse .gitmodules file to extract submodule paths
	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		
		// Look for path = ... lines
		if strings.HasPrefix(line, "path = ") {
			path := strings.TrimSpace(strings.TrimPrefix(line, "path = "))
			if path != "" {
				submodules[path] = true
			}
		}
	}
	
	return submodules, nil
}

// shouldIncludeFile checks if a file should be included based on include/exclude patterns
func shouldIncludeFile(filePath string, includePatterns, excludePatterns []string) bool {
	// If exclude patterns are specified and any matches, exclude the file
	for _, excludePattern := range excludePatterns {
		if excludePattern != "" {
			matched, err := filepath.Match(excludePattern, filePath)
			if err == nil && matched {
				return false
			}
			// Also check if the pattern matches the filename only
			matched, err = filepath.Match(excludePattern, filepath.Base(filePath))
			if err == nil && matched {
				return false
			}
		}
	}
	
	// If include patterns are specified, only include files that match any pattern
	if len(includePatterns) > 0 {
		for _, includePattern := range includePatterns {
			if includePattern != "" {
				matched, err := filepath.Match(includePattern, filePath)
				if err == nil && matched {
					return true
				}
				// Also check if the pattern matches the filename only
				matched, err = filepath.Match(includePattern, filepath.Base(filePath))
				if err == nil && matched {
					return true
				}
			}
		}
		// If include patterns are specified but none match, exclude
		return false
	}
	
	// No patterns specified or exclude patterns don't match, include the file
	return true
}

// handleAIAction processes AI actions on distilled content
func handleAIAction(ctx context.Context, projectPath string) error {
	dbg := debug.FromContext(ctx).WithSubsystem("ai-action")
	dbg.Logf(debug.LevelBasic, "AI Action mode: %s", aiAction)
	
	// Get the AI action from registry
	registry := ai.GetRegistry()
	action, err := registry.Get(aiAction)
	if err != nil {
		return fmt.Errorf("failed to get AI action: %w", err)
	}
	
	// Validate the action
	if err := action.Validate(); err != nil {
		return fmt.Errorf("action validation failed: %w", err)
	}
	
	// Create action context
	baseName := filepath.Base(projectPath)
	if baseName == "." || baseName == "/" {
		baseName = "project"
	}
	
	actionCtx := &ai.ActionContext{
		ProjectPath:     projectPath,
		BaseName:        baseName,
		Timestamp:       time.Now(),
		IncludePatterns: includeGlob,
		ExcludePatterns: excludeGlob,
		Config: &ai.ActionConfig{
			OutputPath: aiOutput,
		},
	}
	
	// Handle different action types
	switch action.Type() {
	case ai.ActionTypeFlow:
		// Flow actions don't need distilled content
		flowAction, ok := action.(ai.FlowAction)
		if !ok {
			return fmt.Errorf("action %s claims to be flow type but doesn't implement FlowAction interface", aiAction)
		}
		
		return executeFlowAction(ctx, flowAction, actionCtx)
		
	case ai.ActionTypePrompt:
		// Prompt actions need distilled content
		contentAction, ok := action.(ai.ContentAction)
		if !ok {
			return fmt.Errorf("action %s claims to be prompt type but doesn't implement ContentAction interface", aiAction)
		}
		
		// First, distill the content
		distilledContent, err := distillForAction(ctx, projectPath)
		if err != nil {
			return fmt.Errorf("failed to distill content: %w", err)
		}
		
		actionCtx.DistilledContent = distilledContent
		return executeContentAction(ctx, contentAction, actionCtx)
		
	default:
		return fmt.Errorf("unknown action type: %s", action.Type())
	}
}

// distillForAction runs the distiller to get content for AI actions
func distillForAction(ctx context.Context, projectPath string) (string, error) {
	dbg := debug.FromContext(ctx).WithSubsystem("distill-for-action")
	
	// Create processor options from flags
	procOpts := createProcessOptionsFromFlags()
	procOpts.BasePath = projectPath
	procOpts.FilePathType = "relative"
	
	// Create the processor
	proc := processor.NewWithContext(ctx)
	
	// Process the input
	result, err := proc.ProcessPath(projectPath, procOpts)
	if err != nil {
		return "", fmt.Errorf("failed to process: %w", err)
	}
	
	// Always use text format for AI actions
	formatterOpts := formatter.Options{
		ProcessingOptions: formatter.ProcessingInfo{
			IncludePublic:        procOpts.IncludePublic,
			IncludeProtected:     procOpts.IncludeProtected,
			IncludeInternal:      procOpts.IncludeInternal,
			IncludePrivate:       procOpts.IncludePrivate,
			IncludeImplementation: procOpts.IncludeImplementation,
			IncludeComments:      procOpts.IncludeComments,
			IncludeImports:       procOpts.IncludeImports,
			IncludeDocstrings:    procOpts.IncludeDocstrings,
			IncludeAnnotations:   procOpts.IncludeAnnotations,
			FilterImports:        true, // Always filter imports for AI actions
		},
		DebugLevel: verbosity,
	}
	outputFormatter, err := formatter.Get("text", formatterOpts)
	if err != nil {
		return "", fmt.Errorf("failed to get formatter: %w", err)
	}
	
	// Format the output
	var output strings.Builder
	
	switch r := result.(type) {
	case *ir.DistilledFile:
		if err := outputFormatter.Format(&output, r); err != nil {
			return "", fmt.Errorf("failed to format output: %w", err)
		}
	case *ir.DistilledDirectory:
		var files []*ir.DistilledFile
		for _, child := range r.Children {
			if file, ok := child.(*ir.DistilledFile); ok {
				files = append(files, file)
			}
		}
		if err := outputFormatter.FormatMultiple(&output, files); err != nil {
			return "", fmt.Errorf("failed to format output: %w", err)
		}
	default:
		return "", fmt.Errorf("unexpected result type: %T", result)
	}
	
	dbg.Logf(debug.LevelBasic, "Distilled %d bytes of content", output.Len())
	return output.String(), nil
}

// executeFlowAction executes a flow-type AI action
func executeFlowAction(ctx context.Context, action ai.FlowAction, actionCtx *ai.ActionContext) error {
	startTime := time.Now()
	dbg := debug.FromContext(ctx).WithSubsystem("flow-action")
	
	// Determine output path
	outputPath := actionCtx.Config.OutputPath
	if outputPath == "" {
		outputPath = action.DefaultOutput()
	}
	outputPath = ai.ExpandTemplate(outputPath, actionCtx)
	
	// Ensure output path is within project
	if err := ai.ValidateOutputPath(outputPath, actionCtx.ProjectPath); err != nil {
		return fmt.Errorf("invalid output path: %w", err)
	}
	
	// Execute the flow
	result, err := action.ExecuteFlow(actionCtx)
	if err != nil {
		return fmt.Errorf("flow execution failed: %w", err)
	}
	
	// Create output directory
	if err := os.MkdirAll(outputPath, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}
	
	// Write all files and track total size
	var totalSize int64
	fileCount := 0
	for relPath, content := range result.Files {
		fullPath := filepath.Join(outputPath, relPath)
		
		// Create parent directory
		parentDir := filepath.Dir(fullPath)
		if err := os.MkdirAll(parentDir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", parentDir, err)
		}
		
		// Write file
		contentBytes := []byte(content)
		if err := os.WriteFile(fullPath, contentBytes, 0644); err != nil {
			return fmt.Errorf("failed to write file %s: %w", fullPath, err)
		}
		
		totalSize += int64(len(contentBytes))
		fileCount++
		sizeKB := float64(len(contentBytes)) / 1024.0
		dbg.Logf(debug.LevelDetailed, "Wrote file: %s (%.1f kB)", fullPath, sizeKB)
	}
	
	// Calculate duration and total size
	duration := time.Since(startTime)
	totalSizeKB := float64(totalSize) / 1024.0
	
	// Print messages
	for _, msg := range result.Messages {
		fmt.Println(msg)
	}
	
	// Print summary
	fmt.Printf("✅ AI flow action '%s' completed successfully! (%.2fs)\n", action.Name(), duration.Seconds())
	fmt.Printf("📄 Output saved to: %s (%d files, %.1f kB total)\n", outputPath, fileCount, totalSizeKB)
	
	return nil
}

// executeContentAction executes a content-type AI action
func executeContentAction(ctx context.Context, action ai.ContentAction, actionCtx *ai.ActionContext) error {
	startTime := time.Now()
	dbg := debug.FromContext(ctx).WithSubsystem("content-action")
	
	// Generate content
	result, err := action.GenerateContent(actionCtx)
	if err != nil {
		return fmt.Errorf("content generation failed: %w", err)
	}
	
	// Determine output path
	outputPath := actionCtx.Config.OutputPath
	if outputPath == "" {
		outputPath = action.DefaultOutput()
	}
	outputPath = ai.ExpandTemplate(outputPath, actionCtx)
	
	// Ensure output path is within project
	if err := ai.ValidateOutputPath(outputPath, actionCtx.ProjectPath); err != nil {
		return fmt.Errorf("invalid output path: %w", err)
	}
	
	// Build final content
	var finalContent strings.Builder
	finalContent.WriteString(result.ContentBefore)
	finalContent.WriteString(actionCtx.DistilledContent)
	finalContent.WriteString(result.ContentAfter)
	
	content := []byte(finalContent.String())
	
	// If outputToStdout is set, print to stdout
	if outputToStdout {
		fmt.Print(string(content))
		
		// Also save to file unless explicitly disabled
		if outputPath != "" && outputPath != "-" {
			// Create parent directory
			parentDir := filepath.Dir(outputPath)
			if err := os.MkdirAll(parentDir, 0755); err != nil {
				return fmt.Errorf("failed to create directory %s: %w", parentDir, err)
			}
			
			// Write output file
			if err := os.WriteFile(outputPath, content, 0644); err != nil {
				return fmt.Errorf("failed to write output file: %w", err)
			}
			
			// Print info message to stderr so it doesn't mix with stdout content
			fmt.Fprintf(os.Stderr, "\n\n")
			fmt.Fprintf(os.Stderr, "════════════════════════════════════════════════════════════════════════\n")
			fmt.Fprintf(os.Stderr, "✅ AI prompt with distilled code has been generated and saved to:\n")
			fmt.Fprintf(os.Stderr, "📄 %s\n", outputPath)
			fmt.Fprintf(os.Stderr, "\n")
			fmt.Fprintf(os.Stderr, "You can now:\n")
			fmt.Fprintf(os.Stderr, "1. Let your AI agent read and execute this file\n")
			fmt.Fprintf(os.Stderr, "2. Copy the file content to Gemini 2.5 Pro/Flash (supports 1M+ context)\n")
			fmt.Fprintf(os.Stderr, "3. Use with any AI tool that supports large context windows\n")
			fmt.Fprintf(os.Stderr, "\n")
			if len(content) > 500000 { // If larger than 500KB
				fmt.Fprintf(os.Stderr, "⚠️  The generated file is quite large (%.1f MB).\n", float64(len(content))/1024.0/1024.0)
				fmt.Fprintf(os.Stderr, "   Consider analyzing a specific subdirectory for better results.\n")
			}
			fmt.Fprintf(os.Stderr, "════════════════════════════════════════════════════════════════════════\n")
		}
	} else {
		// Original behavior: save to file only
		// Create parent directory
		parentDir := filepath.Dir(outputPath)
		if err := os.MkdirAll(parentDir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", parentDir, err)
		}
		
		// Write output file
		if err := os.WriteFile(outputPath, content, 0644); err != nil {
			return fmt.Errorf("failed to write output file: %w", err)
		}
		
		// Get file info for size
		fileInfo, err := os.Stat(outputPath)
		if err != nil {
			return fmt.Errorf("failed to stat output file: %w", err)
		}
		
		// Calculate size in kB
		sizeKB := float64(fileInfo.Size()) / 1024.0
		sizeMB := sizeKB / 1024.0
		duration := time.Since(startTime)
		
		dbg.Logf(debug.LevelBasic, "Wrote AI action output to %s (%d bytes, %.1f kB) in %v", outputPath, fileInfo.Size(), sizeKB, duration)
		
		fmt.Printf("\n")
		fmt.Printf("════════════════════════════════════════════════════════════════════════\n")
		fmt.Printf("✅ AI action '%s' completed successfully! (%.2fs)\n", action.Name(), duration.Seconds())
		fmt.Printf("📄 AI prompt with distilled code saved to:\n")
		fmt.Printf("💾 %s (%.1f kB)\n", outputPath, sizeKB)
		fmt.Printf("\n")
		fmt.Printf("You can now:\n")
		fmt.Printf("1. Let your AI agent read and execute this file\n")
		fmt.Printf("2. Copy the file content to Gemini 2.5 Pro/Flash (supports 1M+ context)\n")
		fmt.Printf("3. Use with any AI tool that supports large context windows\n")
		
		if sizeMB > 0.5 { // If larger than 500KB
			fmt.Printf("\n")
			fmt.Printf("⚠️  The generated file is quite large (%.1f MB).\n", sizeMB)
			fmt.Printf("   Consider analyzing a specific subdirectory for better results:\n")
			fmt.Printf("   aid <subdirectory> --ai-action=%s\n", action.Name())
		}
		fmt.Printf("════════════════════════════════════════════════════════════════════════\n")
	}
	
	return nil
}

// pluralS returns "s" if count is not 1
func pluralS(count int) string {
	if count == 1 {
		return ""
	}
	return "s"
}

// getOriginalSize calculates the total size of source files in the result
func getOriginalSize(result interface{}) int64 {
	var totalSize int64
	
	switch r := result.(type) {
	case *ir.DistilledFile:
		// For single file, estimate based on content
		// This is a rough estimate - actual file might be larger
		totalSize = estimateOriginalSize(r)
	case *ir.DistilledDirectory:
		// For directory, sum up all files
		for _, child := range r.Children {
			if file, ok := child.(*ir.DistilledFile); ok {
				totalSize += estimateOriginalSize(file)
			}
		}
	}
	
	return totalSize
}

// estimateOriginalSize estimates the original file size based on distilled content
func estimateOriginalSize(file *ir.DistilledFile) int64 {
	// This is a rough estimate based on typical compression ratios
	// In reality, we should track actual file sizes during processing
	
	// Count base structure size
	size := int64(len(file.Path)) + 100 // File overhead
	
	// Process all children recursively
	for _, child := range file.Children {
		size += estimateNodeSize(child)
	}
	
	// Apply a multiplier for typical source file overhead (comments, whitespace, etc.)
	// This is a conservative estimate
	return size * 3
}

func estimateNodeSize(node ir.DistilledNode) int64 {
	if node == nil {
		return 0
	}
	
	var size int64
	
	switch n := node.(type) {
	case *ir.DistilledImport:
		size += int64(len(n.Module)) + 20
		for _, sym := range n.Symbols {
			size += int64(len(sym.Name)) + 5
		}
	case *ir.DistilledClass:
		size += int64(len(n.Name)) + 50
		for _, child := range n.Children {
			size += estimateNodeSize(child)
		}
	case *ir.DistilledFunction:
		size += int64(len(n.Name)) + 30
		for _, param := range n.Parameters {
			size += int64(len(param.Name)) + 10
			size += int64(len(param.Type.Name)) + 5
		}
		// If implementation is included, use its actual size
		if n.Implementation != "" {
			size += int64(len(n.Implementation))
		} else {
			// Estimate typical function body size
			size += 200
		}
	case *ir.DistilledField:
		size += int64(len(n.Name)) + 20
		if n.Type != nil {
			size += int64(len(n.Type.Name)) + 10
		}
	case *ir.DistilledComment:
		size += int64(len(n.Text))
	default:
		// For other node types, estimate based on children
		for _, child := range node.GetChildren() {
			size += estimateNodeSize(child)
		}
	}
	
	return size
}

// generateDistillationInstructions generates dynamic instructions based on processing options
func generateDistillationInstructions(opts processor.ProcessOptions) string {
	// Build list of what's included
	var included []string
	var excluded []string
	
	// Visibility levels - handle case when not all flags are explicitly set
	visibilityIncluded := []string{}
	
	// Default values if not explicitly set (only public is true by default)
	includePublic := true
	includeProtected := false
	includeInternal := false
	includePrivate := false
	
	// Use actual values if they were set
	if opts.IncludePublic || opts.IncludeProtected || opts.IncludeInternal || opts.IncludePrivate {
		includePublic = opts.IncludePublic
		includeProtected = opts.IncludeProtected
		includeInternal = opts.IncludeInternal
		includePrivate = opts.IncludePrivate
	}
	
	if includePublic {
		visibilityIncluded = append(visibilityIncluded, "public")
	}
	if includeProtected {
		visibilityIncluded = append(visibilityIncluded, "protected")
	}
	if includeInternal {
		visibilityIncluded = append(visibilityIncluded, "internal/package-private")
	}
	if includePrivate {
		visibilityIncluded = append(visibilityIncluded, "private")
	}
	
	if len(visibilityIncluded) > 0 {
		if len(visibilityIncluded) == 4 {
			included = append(included, "All visibility levels (public, protected, internal, private)")
		} else {
			included = append(included, strings.Join(visibilityIncluded, ", ")+" members")
		}
	}
	
	// Content types
	if opts.IncludeImports {
		included = append(included, "import statements")
	} else {
		excluded = append(excluded, "import statements")
	}
	
	if opts.IncludeDocstrings {
		included = append(included, "documentation/docstrings")
	}
	
	if opts.IncludeAnnotations {
		included = append(included, "decorators/annotations")
	}
	
	if opts.IncludeImplementation {
		included = append(included, "function/method implementations")
	} else {
		excluded = append(excluded, "function/method bodies")
	}
	
	if opts.IncludeComments {
		included = append(included, "code comments")
	} else {
		excluded = append(excluded, "code comments")
	}
	
	// Build the instructions
	instructions := "⚡ PROJECT ARCHITECTURE OVERVIEW: This distilled code shows "
	
	if len(included) > 0 {
		instructions += strings.Join(included, ", ")
		instructions += " providing a complete map of available classes, methods, functions, data types, interfaces, and their relationships."
	} else {
		instructions += "all public APIs, classes, methods, functions, data types, and interfaces available in this project."
	}
	
	if len(excluded) > 0 {
		instructions += " (Excludes: " + strings.Join(excluded, ", ") + ")"
	}
	
	instructions += " 📋 USE THIS DISTILLATION TO: 1) Understand the project's architecture and available components, 2) See what classes/methods/types exist and how to use them correctly, 3) Find the right APIs and their exact signatures, 4) Understand relationships between components. ✅ TRUST THIS OVERVIEW: When implementing features or fixing bugs, reference the distilled signatures above to use the correct classes, methods, parameters, and types - no need to read source files for information already captured here."
	
	return instructions
}

// registerAIActions registers all built-in AI actions
// This is done in the CLI package to avoid import cycles
func registerAIActions() {
	registry := ai.GetRegistry()
	aiactions.Register(registry)
}