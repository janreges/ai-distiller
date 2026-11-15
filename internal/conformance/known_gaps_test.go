package conformance

// knownGaps lists the checks that fail today, keyed "<lang>/<cap>", with the
// reason. A fix deletes its entry: a listed check that passes fails the suite.
var knownGaps = map[string]string{
	// By default PHP drops the /** */ docblocks of classes, methods and
	// properties, while file and top-level function docblocks print, so
	// --docstrings means something different in PHP than in the other
	// languages.
	"php/doc-by-default": "plan 9 Task 6: class, method and property docblocks are dropped by default; file and top-level function docblocks print",
	// The Swift line parser emits comment nodes only for "///" docs and
	// "// MARK:" lines; other // and /* */ comments produce no node, so
	// --comments=1 adds nothing for them.
	"swift/comment-with-comments": "plan 9 Task 6: the line parser emits no comment node for // and /* */ comments, so --comments=1 adds nothing",
}
