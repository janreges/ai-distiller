package conformance

import (
	"bytes"
	"context"
	"encoding/json"
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

func TestLangAcceptsAlias(t *testing.T) {
	out, stderr, code := aidRun(t, conformanceData(t), strings.NewReader(stdinSnippet), "--lang", "py")
	require.Equal(t, 0, code, stderr)
	assert.Contains(t, out, "from_stdin")
}

const tsxComponents = "import React from \"react\";\n\n" +
	"export const Hello = ({ name }: { name: string }) => <h1>Hello {name}</h1>;\n\n" +
	"export function App({ title }: { title: string }) {\n  return <Hello name={title} />;\n}\n"

// TSX on stdin (--lang tsx, or detected TypeScript with JSX) is parsed with the
// TSX grammar, like a .tsx file; the output path stays "stdin".
func TestStdinTSXMatchesFileRun(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "comp.tsx"), []byte(tsxComponents), 0o644))
	fromFile, stderr, code := aidRun(t, dir, nil, "comp.tsx", "--stdout", "--summary-type=off")
	require.Equal(t, 0, code, stderr)
	require.Contains(t, fromFile, "export function App")
	want := strings.Replace(fromFile, `<file path="comp.tsx">`, `<file path="stdin">`, 1)
	for _, args := range [][]string{{"--lang", "tsx"}, nil} {
		out, stderr, code := aidRun(t, dir, strings.NewReader(tsxComponents), args...)
		require.Equal(t, 0, code, stderr)
		assert.Equal(t, want, out, "stdin %v", args)
	}
}

func TestUndetectedStdinListsLanguages(t *testing.T) {
	_, stderr, code := aidRun(t, conformanceData(t), strings.NewReader("hello world\n"))
	assert.NotEqual(t, 0, code)
	assert.Contains(t, stderr, "could not auto-detect the language of stdin; pass --lang with one of: "+
		"cpp, csharp, go, java, javascript, kotlin, php, python, ruby, rust, swift, typescript (aliases: ")
	assert.Contains(t, stderr, "py, rb, rs, ts, tsx)")
}

func TestEmptyStdinSaysNoInput(t *testing.T) {
	_, stderr, code := aidRun(t, conformanceData(t), strings.NewReader(""))
	assert.NotEqual(t, 0, code)
	assert.Contains(t, stderr, "no input on stdin")
	assert.NotContains(t, stderr, "auto-detect")
}

func TestUnknownLangListsRegisteredNames(t *testing.T) {
	_, stderr, code := aidRun(t, conformanceData(t), strings.NewReader(stdinSnippet), "--lang", "zig")
	assert.NotEqual(t, 0, code)
	assert.Contains(t, stderr, `unknown language "zig"; supported: `+
		"cpp, csharp, go, java, javascript, kotlin, php, python, ruby, rust, swift, typescript")
}

// projectWithSample makes a temp project (marker file .aidrc) holding one Python file.
func projectWithSample(t *testing.T) string {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".aidrc"), nil, 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "m.py"), []byte("def visible():\n    return 1\n"), 0o644))
	return dir
}

func TestStdoutWithOutputWritesBoth(t *testing.T) {
	dir := projectWithSample(t)
	out, _, code := aidRun(t, dir, nil, "m.py", "--stdout", "-o", "out.txt", "--summary-type=off")
	require.Equal(t, 0, code)
	assert.Contains(t, out, "visible")
	written, err := os.ReadFile(filepath.Join(dir, "out.txt"))
	require.NoError(t, err)
	assert.Equal(t, out, string(written))
}

// Stdin output is always printed; -o FILE also writes it to FILE.
func TestStdinWithOutputWritesFileAndPrints(t *testing.T) {
	dir := projectWithSample(t)
	out, stderr, code := aidRun(t, dir, strings.NewReader(stdinSnippet), "--lang", "python", "-o", "out.txt")
	require.Equal(t, 0, code, stderr)
	assert.Contains(t, out, "from_stdin")
	written, err := os.ReadFile(filepath.Join(dir, "out.txt"))
	require.NoError(t, err)
	assert.Equal(t, out, string(written))
}

// An AI action writes to --ai-output, so -o with --ai-action is an error, not ignored.
func TestOutputWithAIActionFails(t *testing.T) {
	dir := projectWithSample(t)
	_, stderr, code := aidRun(t, dir, nil, ".", "--ai-action", "prompt-for-bug-hunting", "-o", "out.md")
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "--ai-output")
	_, err := os.Stat(filepath.Join(dir, ".aid"))
	assert.True(t, os.IsNotExist(err), "the action must not run")
}

func TestStdoutAloneWritesNoFile(t *testing.T) {
	dir := projectWithSample(t)
	out, _, code := aidRun(t, dir, nil, "m.py", "--stdout", "--summary-type=off")
	require.Equal(t, 0, code)
	assert.Contains(t, out, "visible")
	_, err := os.Stat(filepath.Join(dir, ".aid"))
	assert.True(t, os.IsNotExist(err), ".aid must not be created by --stdout")
}

func TestDefaultWritesFileAndPrintsNothing(t *testing.T) {
	dir := projectWithSample(t)
	out, _, code := aidRun(t, dir, nil, "m.py", "--summary-type=off")
	require.Equal(t, 0, code)
	assert.Empty(t, out)
	written, err := os.ReadFile(filepath.Join(dir, ".aid", "aid.m.py.txt"))
	require.NoError(t, err)
	assert.Contains(t, string(written), "visible")
}

func TestSummaryReportsMeasuredSize(t *testing.T) {
	data := conformanceData(t)
	info, err := os.Stat(filepath.Join(data, "python", "sample.py"))
	require.NoError(t, err)
	_, stderr, code := aidRun(t, data, nil, "python/sample.py", "--stdout", "--summary-type=json")
	require.Equal(t, 0, code)
	var s struct {
		OriginalBytes int64  `json:"original_bytes"`
		Tokenizer     string `json:"tokenizer"`
	}
	require.NoError(t, json.Unmarshal([]byte(stderr[strings.Index(stderr, "{"):]), &s))
	assert.Equal(t, info.Size(), s.OriginalBytes)
	assert.Equal(t, "estimate (bytes/4)", s.Tokenizer)
}

func TestStructuredFormatsDoNotLeakMetadata(t *testing.T) {
	for _, format := range []string{"jsonl", "json-structured", "xml", "md"} {
		out, _, code := aidRun(t, conformanceData(t), nil, "python/sample.py", "--stdout", "--summary-type=off", "--format", format)
		require.Equal(t, 0, code, format)
		assert.NotContains(t, out, "size_bytes", format)
		assert.NotContains(t, out, "<metadata", format)
	}
}

func TestSummaryMentionsWrittenFileWithStdout(t *testing.T) {
	dir := projectWithSample(t)
	_, stderr, code := aidRun(t, dir, nil, "m.py", "--stdout", "-o", "out.txt")
	require.Equal(t, 0, code)
	assert.Contains(t, stderr, "out.txt")
}

// distilled_bytes is the size of the text written: empty <file> blocks are
// dropped and --show-ai-agent-instructions is added before the write.
func TestSummaryDistilledBytesIsWrittenSize(t *testing.T) {
	dir := projectWithSample(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "p.py"), []byte("def _hidden():\n    return 1\n"), 0o644))
	for _, extra := range [][]string{nil, {"--show-ai-agent-instructions"}} {
		args := append([]string{".", "-o", "out.txt", "--summary-type=json"}, extra...)
		_, stderr, code := aidRun(t, dir, nil, args...)
		require.Equal(t, 0, code, stderr)
		var s struct {
			DistilledBytes int64 `json:"distilled_bytes"`
		}
		require.NoError(t, json.Unmarshal([]byte(stderr[strings.Index(stderr, "{"):]), &s))
		info, err := os.Stat(filepath.Join(dir, "out.txt"))
		require.NoError(t, err)
		assert.Equal(t, info.Size(), s.DistilledBytes, "extra flags %v", extra)
	}
}

// With -o FILE --stdout the text was printed, so the summary names the file
// but does not tell an AI agent to load it.
func TestSummaryDoesNotAskToLoadPrintedOutput(t *testing.T) {
	dir := projectWithSample(t)
	_, stderr, code := aidRun(t, dir, nil, "m.py", "--stdout", "-o", "out.txt")
	require.Equal(t, 0, code)
	assert.Contains(t, stderr, "out.txt")
	assert.NotContains(t, stderr, "ENTIRE")

	_, stderr, code = aidRun(t, dir, nil, "m.py", "-o", "out.txt")
	require.Equal(t, 0, code)
	assert.Contains(t, stderr, "load this ENTIRE file")
}
