package ai

// globalRegistry is the singleton instance of the action registry
var globalRegistry = NewActionRegistry()

// GetRegistry returns the global action registry
func GetRegistry() *ActionRegistry {
	return globalRegistry
}
