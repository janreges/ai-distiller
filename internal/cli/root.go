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
	"sync"
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
		fmt.Fprintln(os.Stderr, "  --stdout               # Print to stdout; no file is written unless -o is also given; AI actions always write to --ai-output and reject -o")
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
	rootCmd.Flags().StringVarP(&outputFile, "output", "o", "", "Output file (default: .aid/aid.<name>[.<options>].<ext> in the project root); AI actions always write to --ai-output and reject -o")
	rootCmd.Flags().BoolVar(&outputToStdout, "stdout", false, "Print to stdout; no file is written unless -o is also given; AI actions always write to --ai-output and reject -o")
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
	rootCmd.Flags().StringVar(&langOverride, "lang", "auto", "Language of code read from stdin, ignored for files and directories: auto|python|typescript|javascript|go|ruby|swift|rust|java|csharp|kotlin|cpp|php (aliases such as py, ts, c++, c# are accepted)")
	
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

	if verbosity >= 1 {
		// Report each skipped node kind once per language, not once per node.
		var reportedMu sync.Mutex
		reported := map[string]bool{}
		formatter.OnUnhandledNode = func(lang string, node ir.DistilledNode) {
			key := lang + "/" + string(node.GetNodeKind())
			reportedMu.Lock()
			defer reportedMu.Unlock()
			if reported[key] {
				return
			}
			reported[key] = true
			dbg.Logf(debug.LevelBasic, "formatter: no text rendering for %s in %s", node.GetNodeKind(), lang)
		}
	}

	// Log startup info
	dbg.Logf(debug.LevelBasic, "AI Distiller %s starting", Version)
	
	// Stdin is read in exactly two cases: the argument is "-", or there is no
	// argument and stdin is not a character device. With any other argument
	// stdin is never inspected, so an open but unwritten pipe cannot hijack or
	// block a run that names a path.
	stdinAvailable := false
	switch {
	case len(args) > 0:
		stdinAvailable = args[0] == "-"
	default:
		if stat, err := os.Stdin.Stat(); err == nil && (stat.Mode()&os.ModeCharDevice) == 0 {
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
		if outputFile != "" {
			return fmt.Errorf("-o/--output does not apply to --ai-action; use --ai-output to choose where the action writes")
		}
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

	// Write the file whenever an output file is set: the auto-generated name
	// (only computed without --stdout) or an explicit -o, with or without --stdout.
	if outputFile != "" {
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
		originalSize := getOriginalSize(result)
		distilledSize := int64(len(outputStr)) // what was written or printed
		
		// Create summary stats
		stats := summary.Stats{
			OriginalBytes:   originalSize,
			DistilledBytes:  distilledSize,
			OriginalTokens:  summary.EstimateTokens(originalSize),
			DistilledTokens: summary.EstimateTokens(distilledSize),
			Duration:        duration,
			FileCount:       fileCount,
			OutputPath:      outputFile,
			IsStdout:        outputFile == "",
			Printed:         outputToStdout || outputFile == "",
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

	// Read all of stdin; detection looks at the first 64kB
	fullContent, err := io.ReadAll(os.Stdin)
	if err != nil {
		return fmt.Errorf("failed to read stdin: %w", err)
	}

	dbg.Logf(debug.LevelDetailed, "Read %d bytes from stdin", len(fullContent))

	// Determine language
	lang := langOverride
	if lang == "auto" {
		if len(bytes.TrimSpace(fullContent)) == 0 {
			return fmt.Errorf("no input on stdin")
		}
		detector := language.NewDetector()
		detectedLang, err := detector.DetectFromReader(bytes.NewReader(fullContent))
		if err != nil {
			return fmt.Errorf("could not auto-detect the language of stdin; pass --lang with one of: %s", supportedLanguages())
		}
		lang = detectedLang
		dbg.Logf(debug.LevelBasic, "Detected language: %s", lang)
	} else {
		dbg.Logf(debug.LevelBasic, "Using specified language: %s", lang)
	}

	// Get language processor; --lang accepts aliases such as "py" or "c++"
	canonical := language.Canonical(lang)
	langProc, ok := language.GetProcessor(canonical)
	if !ok {
		return fmt.Errorf("unknown language %q; supported: %s", lang, supportedLanguages())
	}

	// The TypeScript processor takes its TSX grammar from a .tsx file name
	parseName := "stdin"
	if canonical == "typescript" && (strings.EqualFold(langOverride, "tsx") || langOverride == "auto" && language.HasJSX(fullContent)) {
		parseName = "stdin.tsx"
	}

	// Create processor options from flags
	procOpts := createProcessOptionsFromFlags()

	// Process the input with our debug-enabled context
	result, err := langProc.ProcessWithOptions(ctx, bytes.NewReader(fullContent), parseName, procOpts)
	if err != nil {
		return fmt.Errorf("failed to process stdin: %w", err)
	}
	result.Path = "stdin"

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
	
	// Stdin output is always printed; -o FILE also writes it to FILE
	if outputFile != "" {
		if err := os.WriteFile(outputFile, []byte(output.String()), 0644); err != nil {
			return fmt.Errorf("failed to write output file: %w", err)
		}
	}
	fmt.Print(output.String())

	return nil
}

// supportedLanguages lists the names and aliases --lang accepts.
func supportedLanguages() string {
	return strings.Join(language.RegisteredNames(), ", ") + " (aliases: " + strings.Join(language.Aliases(), ", ") + ")"
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
	opts.RemovePublic = !includePublicVal
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
	
	// Listing visibility categories without "public" asks for non-public members only
	opts.RemovePublic = (includeProtected || includeInternal || includePrivate) && !includePublic

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
		case "public":
			opts.RemovePublic = true
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

// getOriginalSize sums the measured source sizes recorded on the processed files.
func getOriginalSize(result interface{}) int64 {
	switch r := result.(type) {
	case *ir.DistilledFile:
		if r.Metadata != nil {
			return r.Metadata.Size
		}
	case *ir.DistilledDirectory:
		var total int64
		for _, child := range r.Children {
			total += getOriginalSize(child)
		}
		return total
	}
	return 0
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