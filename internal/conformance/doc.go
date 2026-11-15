// Package conformance checks that every supported language honours the same
// basic aid contract: public declarations shown, private ones hidden, bodies,
// comments and doc comments controlled by their flags, --expand selecting one
// body, and stdin language detection.
//
// The package has no production code. Its tests run the real aid binary
// against one small fixture per language in testdata/ and compare the output
// with a list of capability checks. Checks that fail today are listed with a
// reason in knownGaps; a fix must delete its entry, because a listed check that
// starts passing fails the suite.
package conformance
