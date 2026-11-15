package debug

import "context"

// key is an unexported type to prevent context key collisions
type key int

var debuggerKey key

// NewContext returns a new context that carries the provided Debugger
func NewContext(ctx context.Context, debugger Debugger) context.Context {
	return context.WithValue(ctx, debuggerKey, debugger)
}

// FromContext retrieves the Debugger from the context
// If no Debugger is found, it returns a no-op debugger that does nothing
// This makes usage safe and eliminates nil checks everywhere
func FromContext(ctx context.Context) Debugger {
	if debugger, ok := ctx.Value(debuggerKey).(Debugger); ok {
		return debugger
	}
	return Silent() // Never return nil!
}

// WithTiming is a helper that executes a function and logs its duration
// Usage: result := debug.WithTiming(ctx, LevelDetailed, "parsing", func() T { ... })
func WithTiming[T any](ctx context.Context, level int, operation string, fn func() T) T {
	dbg := FromContext(ctx)
	if !dbg.IsEnabledFor(level) {
		return fn()
	}

	done := dbg.Timing(level, operation)
	defer done()
	return fn()
}

// Lazy executes a logging function only if the debug level is enabled
// This avoids expensive string formatting when debugging is disabled
// Usage: debug.Lazy(ctx, LevelTrace, func(d Debugger) { d.Logf(LevelTrace, "data: %+v", expensiveData()) })
func Lazy(ctx context.Context, level int, fn func(Debugger)) {
	dbg := FromContext(ctx)
	if dbg.IsEnabledFor(level) {
		fn(dbg)
	}
}
