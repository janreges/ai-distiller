package conformance

// knownGaps lists the checks that fail today, keyed "<lang>/<cap>", with the
// reason. A fix deletes its entry: a listed check that passes fails the suite.
var knownGaps = map[string]string{
	"csharp/comment-with-comments": "H5: parser emits no comments",
	"csharp/doc-by-default":        "H5: parser emits no comments",
	"ruby/comment-with-comments":   "H5: parser emits no comments",
	"ruby/doc-by-default":          "H5: parser emits no comments",

	// The fixture is detected as "typescript" before C# is scored, because the
	// TypeScript pattern <\w+> matches <summary>; without that tag the detector
	// returns "c#". Both have to be fixed for the check to pass.
	"csharp/stdin-detect": `H3: detector returns "c#"`,

	// Detected as "typescript": the pattern <\w+> matches "#include <string>".
	"cpp/stdin-detect": "found by the first conformance run",
	// The /** */ docblock above the class is dropped (single- and multi-line).
	"php/doc-by-default": "found by the first conformance run",
	// --comments=1 still drops the standalone line comment.
	"swift/comment-with-comments": "found by the first conformance run",
}
