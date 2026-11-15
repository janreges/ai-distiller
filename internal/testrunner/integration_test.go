package testrunner_test

import (
	"path/filepath"
	"testing"

	"github.com/janreges/ai-distiller/internal/testrunner"
)

func TestIntegration(t *testing.T) {
	// Skip if in short mode
	if testing.Short() {
		t.Skip("skipping integration tests in short mode")
	}

	root, err := testrunner.ProjectRoot()
	if err != nil {
		t.Fatalf("could not find project root: %v", err)
	}

	bin, cleanup, err := testrunner.BuildBinary(root)
	if err != nil {
		t.Fatalf("building aid: %v", err)
	}
	defer cleanup()

	testrunner.New(filepath.Join(root, "testdata"), bin).RunTests(t)
}
