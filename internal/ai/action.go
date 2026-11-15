package ai

import (
	"time"
)

// ActionType represents the type of AI action
type ActionType string

const (
	ActionTypePrompt ActionType = "prompt"
	ActionTypeFlow   ActionType = "flow"
)

// AIAction is the core interface that all AI actions must implement
type AIAction interface {
	// Name returns the unique name used in CLI (e.g., 'prompt-for-refactoring-suggestion')
	Name() string

	// Description returns help text for the --help command
	Description() string

	// Type returns the action type (prompt or flow)
	Type() ActionType

	// DefaultOutput returns the default output path template
	DefaultOutput() string

	// Validate checks if the action can be executed
	Validate() error
}

// ContentAction is the interface for actions that generate content to wrap around distilled output
type ContentAction interface {
	AIAction

	// GenerateContent generates content to be placed before/after distilled output
	GenerateContent(ctx *ActionContext) (*ContentResult, error)
}

// FlowAction is the interface for complex workflow actions that may create multiple files
type FlowAction interface {
	AIAction

	// ExecuteFlow executes complex workflow, can create multiple files
	ExecuteFlow(ctx *ActionContext) (*FlowResult, error)
}

// ActionContext provides context to all actions
type ActionContext struct {
	DistilledContent string
	ProjectPath      string
	BaseName         string
	Timestamp        time.Time
	Config           *ActionConfig
	IncludePatterns  []string
	ExcludePatterns  []string
}

// ActionConfig contains configuration for action execution
type ActionConfig struct {
	OutputPath string
	// Future: Add more config options as needed
}

// ContentResult is returned by content-generating actions
type ContentResult struct {
	ContentBefore string
	ContentAfter  string
}

// FlowResult is returned by flow actions
type FlowResult struct {
	Files    map[string]string // path -> content mapping
	Messages []string          // informational messages for user
}
