package conformance

// knownGaps lists the checks that fail today, keyed "<lang>/<cap>", with the
// reason. A fix deletes its entry: a listed check that passes fails the suite.
var knownGaps = map[string]string{}
