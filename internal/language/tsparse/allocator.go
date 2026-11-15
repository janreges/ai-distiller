package tsparse

/*
// ts_set_allocator is declared here and not through the binding's headers so
// this file needs no include path of its own; the symbol is the one in the
// tree-sitter C library the binding compiles.
extern void ts_set_allocator(
	void *(*new_malloc)(size_t),
	void *(*new_calloc)(size_t, size_t),
	void *(*new_realloc)(void *, size_t),
	void (*new_free)(void *));

static void use_libc_allocator(void) {
	ts_set_allocator(NULL, NULL, NULL, NULL);
}
*/
import "C"

import (
	// The binding installs its allocator in its own init, which runs before
	// this package's init because this package imports it.
	_ "github.com/tree-sitter/go-tree-sitter"
)

// go-tree-sitter v0.25.0 points the C runtime's allocator at Go callbacks
// (allocator.go), so every malloc and free inside the C parser crosses from C
// into Go and back to libc; that cost 13 to 17 percent of parse CPU. Its
// default Go allocator is only C.malloc, C.calloc, C.realloc and C.free, and
// the binding never frees runtime memory any other way, so switching back to
// libc (NULL for all four functions) is safe, also for memory allocated
// before the switch. The binding has no public call for this: its
// SetAllocator(nil, ...) keeps the Go callbacks. This argument holds for
// go-tree-sitter v0.25.0 only: on every runtime upgrade re-read the binding's
// allocator.go and every place it frees memory.
func init() {
	C.use_libc_allocator()
}
