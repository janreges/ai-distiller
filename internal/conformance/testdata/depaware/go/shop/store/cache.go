package store

var cacheHits int

// ResetCache forgets the cached rows.
func ResetCache() {
	cacheHits = 0
}
