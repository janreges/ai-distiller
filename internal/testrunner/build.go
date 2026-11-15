package testrunner

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// ProjectRoot walks up from the working directory to the directory holding go.mod.
func ProjectRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("getting working directory: %w", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("go.mod not found above working directory")
		}
		dir = parent
	}
}

// BuildBinary builds ./cmd/aid into a fresh temp directory and returns the binary
// path together with a cleanup function that removes it. If the AID_TEST_BINARY
// environment variable names an existing file, that file is used instead and
// nothing is built.
func BuildBinary(projectRoot string) (binPath string, cleanup func(), err error) {
	if override := os.Getenv("AID_TEST_BINARY"); override != "" {
		if info, statErr := os.Stat(override); statErr == nil && !info.IsDir() {
			return override, func() {}, nil
		}
	}

	tmpDir, err := os.MkdirTemp("", "aid-test-bin-")
	if err != nil {
		return "", nil, fmt.Errorf("creating temp dir: %w", err)
	}
	cleanup = func() { _ = os.RemoveAll(tmpDir) }

	name := "aid"
	if runtime.GOOS == "windows" {
		name = "aid.exe"
	}
	binPath = filepath.Join(tmpDir, name)

	cmd := exec.Command("go", "build", "-o", binPath, "./cmd/aid")
	cmd.Dir = projectRoot
	cmd.Env = append(os.Environ(), "CGO_ENABLED=1", "CGO_CFLAGS=-w")
	if out, buildErr := cmd.CombinedOutput(); buildErr != nil {
		cleanup()
		return "", nil, fmt.Errorf("building aid: %w\n%s", buildErr, out)
	}
	return binPath, cleanup, nil
}
