package ai

import (
	"fmt"
	"sort"
	"sync"
)

// ActionRegistry manages all registered AI actions
type ActionRegistry struct {
	mu      sync.RWMutex
	actions map[string]AIAction
}

// NewActionRegistry creates a new action registry
func NewActionRegistry() *ActionRegistry {
	return &ActionRegistry{
		actions: make(map[string]AIAction),
	}
}

// Register adds a new action to the registry
func (r *ActionRegistry) Register(action AIAction) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.actions[action.Name()]; exists {
		return fmt.Errorf("action %q already registered", action.Name())
	}

	r.actions[action.Name()] = action
	return nil
}

// Get retrieves an action by name
func (r *ActionRegistry) Get(name string) (AIAction, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	action, ok := r.actions[name]
	if !ok {
		available := r.getAvailableNames()
		return nil, fmt.Errorf("unknown action %q. Available actions: %v", name, available)
	}

	return action, nil
}

// List returns all registered actions sorted by name
func (r *ActionRegistry) List() []AIAction {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var list []AIAction
	for _, action := range r.actions {
		list = append(list, action)
	}

	// Sort by name for consistent output
	sort.Slice(list, func(i, j int) bool {
		return list[i].Name() < list[j].Name()
	})

	return list
}

// GetNames returns all registered action names sorted
func (r *ActionRegistry) GetNames() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.getAvailableNames()
}

// getAvailableNames is an internal helper that assumes lock is held
func (r *ActionRegistry) getAvailableNames() []string {
	var names []string
	for name := range r.actions {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
