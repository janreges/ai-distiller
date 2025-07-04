package version

// These variables are populated by ldflags at build time.
//
//nolint:gochecknoglobals // Build info variables need to be global for ldflags
var (
	// Version is the semantic version number.
	Version = "dev"

	// Commit is the git commit hash.
	Commit = "none"

	// Date is the build date.
	Date = "unknown"

	// WebsiteURL is the official website/repository URL.
	WebsiteURL = "https://github.com/janreges/ai-distiller"

	// BuildInfo returns a formatted version string with all build information.
	BuildInfo = func() string {
		return "aid version " + Version
	}
)
