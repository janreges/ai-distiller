package conformance

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/janreges/ai-distiller/internal/testrunner"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const aidRunTimeout = 20 * time.Second

var (
	aidBinOnce    sync.Once
	aidBinPath    string
	aidBinCleanup func()
	aidBinErr     error
)

// TestMain removes the binary built by aidBinary, if any test built one.
func TestMain(m *testing.M) {
	code := m.Run()
	if aidBinCleanup != nil {
		aidBinCleanup()
	}
	os.Exit(code)
}

// aidBinary builds the aid binary once per test binary.
func aidBinary(t *testing.T) string {
	t.Helper()
	aidBinOnce.Do(func() {
		root, err := testrunner.ProjectRoot()
		if err != nil {
			aidBinErr = err
			return
		}
		aidBinPath, aidBinCleanup, aidBinErr = testrunner.BuildBinary(root)
	})
	require.NoError(t, aidBinErr)
	return aidBinPath
}

// conformanceData returns <project root>/internal/conformance/testdata.
func conformanceData(t *testing.T) string {
	t.Helper()
	root, err := testrunner.ProjectRoot()
	require.NoError(t, err)
	return filepath.Join(root, "internal", "conformance", "testdata")
}

// aidRun runs the built binary in dir. stdin may be nil. It fails the test on timeout.
func aidRun(t *testing.T, dir string, stdin io.Reader, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()
	bin := aidBinary(t)

	ctx, cancel := context.WithTimeout(context.Background(), aidRunTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = dir
	// A non-file stdin would be copied by an exec goroutine that cmd.Wait waits
	// for even after the child exits, and that goroutine can block forever on a
	// reader that is never written or closed. Hand the child a real OS pipe and
	// feed it from our own goroutine instead; a nil stdin means /dev/null.
	if stdin != nil {
		pr, pw, err := os.Pipe()
		require.NoError(t, err)
		defer pr.Close()
		cmd.Stdin = pr
		go func() {
			_, _ = io.Copy(pw, stdin)
			_ = pw.Close()
		}()
		defer pw.Close()
	}
	// Bound Wait if a grandchild keeps stdout/stderr open after the child exits.
	cmd.WaitDelay = 2 * time.Second
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	runErr := cmd.Run()
	if runErr != nil && errors.Is(ctx.Err(), context.DeadlineExceeded) {
		t.Fatalf("aid %s did not finish within %s and was killed (stdout=%q stderr=%q)",
			strings.Join(args, " "), aidRunTimeout, outBuf.String(), errBuf.String())
	}
	if cmd.ProcessState == nil {
		require.NoError(t, runErr, "starting aid")
	}
	if runErr != nil && !errors.Is(runErr, exec.ErrWaitDelay) {
		var exitErr *exec.ExitError
		require.True(t, errors.As(runErr, &exitErr), "running aid: %v", runErr)
	}
	return outBuf.String(), errBuf.String(), cmd.ProcessState.ExitCode()
}

const stdinSnippet = "def from_stdin():\n    return 1\n"

func TestPathArgumentWinsOverPipedStdin(t *testing.T) {
	out, _, code := aidRun(t, conformanceData(t), strings.NewReader(stdinSnippet), "python/sample.py", "--stdout", "--summary-type=off")
	require.Equal(t, 0, code)
	assert.Contains(t, out, "ConfWidget")
	assert.NotContains(t, out, "from_stdin")
}

func TestPathArgumentWithOpenPipeDoesNotHang(t *testing.T) {
	r, w := io.Pipe() // never written, never closed until the test ends
	defer w.Close()
	out, _, code := aidRun(t, conformanceData(t), r, "python/sample.py", "--stdout", "--summary-type=off")
	require.Equal(t, 0, code)
	assert.Contains(t, out, "ConfWidget")
}

func TestDashReadsStdin(t *testing.T) {
	out, _, code := aidRun(t, conformanceData(t), strings.NewReader(stdinSnippet), "-", "--lang", "python")
	require.Equal(t, 0, code)
	assert.Contains(t, out, "from_stdin")
}

func TestNoArgumentReadsPipedStdin(t *testing.T) {
	out, _, code := aidRun(t, conformanceData(t), strings.NewReader(stdinSnippet), "--lang", "python")
	require.Equal(t, 0, code)
	assert.Contains(t, out, "from_stdin")
}
