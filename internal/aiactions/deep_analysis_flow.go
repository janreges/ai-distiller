package aiactions

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/janreges/ai-distiller/internal/ai"
)

// DeepAnalysisFlowAction implements the existing --ai-analysis-task-list functionality
type DeepAnalysisFlowAction struct{}

// Ensure DeepAnalysisFlowAction implements FlowAction
var _ ai.FlowAction = (*DeepAnalysisFlowAction)(nil)

func (a *DeepAnalysisFlowAction) Name() string {
	return "flow-for-deep-file-to-file-analysis"
}

func (a *DeepAnalysisFlowAction) Description() string {
	return "Generate structured task list for comprehensive file-by-file AI analysis"
}

func (a *DeepAnalysisFlowAction) Type() ai.ActionType {
	return ai.ActionTypeFlow
}

func (a *DeepAnalysisFlowAction) DefaultOutput() string {
	return "./.aid"
}

func (a *DeepAnalysisFlowAction) Validate() error {
	return nil
}

func (a *DeepAnalysisFlowAction) ExecuteFlow(ctx *ai.ActionContext) (*ai.FlowResult, error) {
	// Get project basename and current date
	basename := ctx.BaseName
	currentDate := fmt.Sprintf("%04d-%02d-%02d",
		ctx.Timestamp.Year(), ctx.Timestamp.Month(), ctx.Timestamp.Day())

	// Collect source files from the project
	sourceFiles, err := collectSourceFiles(ctx.ProjectPath, ctx.IncludePatterns, ctx.ExcludePatterns)
	if err != nil {
		return nil, fmt.Errorf("failed to collect source files: %w", err)
	}

	// Generate file paths
	taskListPath := fmt.Sprintf("ANALYSIS-TASK-LIST.%s.%s.md", basename, currentDate)
	summaryPath := fmt.Sprintf("ANALYSIS-SUMMARY.%s.%s.md", basename, currentDate)
	analysisDir := fmt.Sprintf("analysis.%s/%s", basename, currentDate)

	// Create result with all files to be created
	result := &ai.FlowResult{
		Files: map[string]string{
			taskListPath: a.generateTaskList(basename, currentDate, sourceFiles, analysisDir),
			summaryPath:  a.generateSummaryFile(basename, currentDate),
		},
		Messages: []string{
			fmt.Sprintf("\n✅ AI Analysis Task List generated successfully!"),
			fmt.Sprintf("📋 Task List: .aid/%s", taskListPath),
			fmt.Sprintf("📊 Summary File: .aid/%s", summaryPath),
			fmt.Sprintf("📁 Analysis Reports Directory: .aid/%s", analysisDir),
			fmt.Sprintf("🤖 Ready for AI-driven analysis workflow!"),
			fmt.Sprintf("📂 Files to analyze: %d\n", len(sourceFiles)),
			fmt.Sprintf("💡 If you are an AI agent, please read the Task List above and carefully follow all instructions to systematically analyze each file."),
		},
	}

	// Pre-create directory structure for report files
	for _, file := range sourceFiles {
		reportDir := filepath.Join(analysisDir, filepath.Dir(file))
		// Add a marker file to ensure directory creation
		markerPath := filepath.Join(reportDir, ".aid-created")
		result.Files[markerPath] = "# Directory created by AI Distiller\n"
	}

	return result, nil
}

// generateTaskList creates the task list content
func (a *DeepAnalysisFlowAction) generateTaskList(basename, currentDate string, sourceFiles []string, analysisDir string) string {
	var sb strings.Builder

	// Write header
	sb.WriteString(fmt.Sprintf("# AI Distiller – Comprehensive Code Analysis Task List\n\n"))
	sb.WriteString(fmt.Sprintf("**Project:** `%s`  \n", basename))
	sb.WriteString(fmt.Sprintf("**Analysis Date:** %s  \n", currentDate))
	sb.WriteString(fmt.Sprintf("**Total Files:** %d  \n\n", len(sourceFiles)))

	// Write comprehensive prompt
	sb.WriteString(a.getAIAnalysisPrompt(basename, currentDate))
	sb.WriteString("\n\n")

	// Write the task list
	sb.WriteString("## 📋 Analysis Task List\n\n")
	sb.WriteString("Complete each task in order, checking off items as you finish:\n\n")

	// Task 1: Create summary file
	sb.WriteString(fmt.Sprintf("- [ ] **1. Initialize Analysis Summary**  \n"))
	sb.WriteString(fmt.Sprintf("      Create `./aid/ANALYSIS-SUMMARY.%s.%s.md` with project overview\n\n", basename, currentDate))

	// Tasks for each file
	for i, file := range sourceFiles {
		taskNum := i + 2
		sb.WriteString(fmt.Sprintf("- [ ] **%d. Analyze `%s`**  \n", taskNum, file))
		sb.WriteString(fmt.Sprintf("      → Create report: `./aid/%s/%s.md`  \n", analysisDir, file))
		sb.WriteString(fmt.Sprintf("      → Add summary row to ANALYSIS-SUMMARY file\n\n"))
	}

	// Final task: Generate conclusion
	finalTaskNum := len(sourceFiles) + 2
	sb.WriteString(fmt.Sprintf("- [ ] **%d. Generate Project Conclusion**  \n", finalTaskNum))
	sb.WriteString(fmt.Sprintf("      Read completed ANALYSIS-SUMMARY file and write comprehensive conclusion\n\n"))

	// Write workflow notes
	sb.WriteString("## 🔄 Workflow Notes\n\n")
	sb.WriteString("- Check off each task **[x]** only after completing BOTH the individual report AND the summary row\n")
	sb.WriteString("- Follow the exact file naming conventions specified\n")
	sb.WriteString("- Use the standardized analysis format provided in the prompt\n")
	sb.WriteString("- Maintain consistent scoring across all files\n")
	sb.WriteString("- The final conclusion should synthesize findings from the entire summary table\n\n")

	// Add scope control tips
	sb.WriteString("## 💡 Scope Control Tips\n\n")
	sb.WriteString("If this task list is too large:\n")
	sb.WriteString("- **Analyze specific directories**: `aid internal/cli --ai-action=flow-for-deep-file-to-file-analysis`\n")
	sb.WriteString("- **Exclude test files**: `aid --exclude \"*test*,*spec*\" --ai-action=flow-for-deep-file-to-file-analysis`\n")
	sb.WriteString("- **Focus on core languages**: `aid --include \"*.go,*.py,*.ts,*.php\" --ai-action=flow-for-deep-file-to-file-analysis`\n\n")

	sb.WriteString("---\n")
	sb.WriteString("*Generated by AI Distiller – Comprehensive Code Analysis System*\n")

	return sb.String()
}

// generateSummaryFile creates the summary file template
func (a *DeepAnalysisFlowAction) generateSummaryFile(basename, currentDate string) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("# Project Analysis Summary – %s (%s)\n\n", basename, currentDate))

	sb.WriteString("## 📊 Overview\n\n")
	sb.WriteString("This document provides a comprehensive analysis summary of the entire codebase. ")
	sb.WriteString("Each file has been individually analyzed for security, maintainability, performance, ")
	sb.WriteString("and readability. The results are compiled in the table below.\n\n")

	sb.WriteString("## 📈 Analysis Results\n\n")
	sb.WriteString("| File | Security % | Maintainability % | Performance % | Readability % | Critical | High | Medium | Low |\n")
	sb.WriteString("|------|:----------:|:-----------------:|:-------------:|:-------------:|:--------:|:----:|:------:|:---:|\n")

	// Note: Individual file rows will be appended here during analysis

	return sb.String()
}

// getAIAnalysisPrompt returns the comprehensive prompt for AI analysis
func (a *DeepAnalysisFlowAction) getAIAnalysisPrompt(basename, currentDate string) string {
	infrastructureInfo := fmt.Sprintf("### 🚀 Pre-Created Infrastructure\n"+
		"- **All report directories have been pre-created** - no need to run mkdir commands\n"+
		"- **Individual reports go to**: `.aid/analysis.%s/%s/[file-path].md`\n"+
		"- **Summary table updates**: `.aid/ANALYSIS-SUMMARY.%s.%s.md`", basename, currentDate, basename, currentDate)

	// Return the full prompt (abbreviated here for space)
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

You are an **Expert Senior Staff Engineer and Security Auditor** conducting a comprehensive file-by-file analysis of the **%s** project.

%s

[Rest of the original prompt content...]`, basename, infrastructureInfo)
}
