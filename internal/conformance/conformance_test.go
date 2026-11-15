package conformance

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/janreges/ai-distiller/internal/testrunner"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConformance(t *testing.T) {
	root, err := testrunner.ProjectRoot()
	require.NoError(t, err)
	bin, cleanup, err := testrunner.BuildBinary(root)
	require.NoError(t, err)
	defer cleanup()
	dataDir := filepath.Join(root, "internal", "conformance", "testdata")

	for _, fx := range fixtures {
		for _, c := range fx.Checks {
			key := fx.Lang + "/" + c.Cap
			t.Run(key, func(t *testing.T) {
				out, runErr := runAid(bin, dataDir, fx, c)
				problems := verify(out, runErr, c)
				reason, gap := knownGaps[key]
				switch {
				case len(problems) > 0 && !gap:
					t.Errorf("%s\noutput:\n%s", strings.Join(problems, "\n"), out)
				case len(problems) > 0 && gap:
					t.Logf("known gap (%s): %s", reason, strings.Join(problems, "; "))
				case len(problems) == 0 && gap:
					t.Errorf("%s passes now: delete it from knownGaps in known_gaps_test.go", key)
				}
			})
		}
	}
}

func TestKnownGapsAreLive(t *testing.T) {
	live := map[string]bool{}
	for _, fx := range fixtures {
		for _, c := range fx.Checks {
			live[fx.Lang+"/"+c.Cap] = true
		}
	}
	for key := range knownGaps {
		assert.True(t, live[key], "knownGaps key %q matches no check", key)
	}
}

// runAid runs the aid binary for one check and returns its stdout. In path
// mode the fixture path is passed relative to dataDir and stdin stays
// detached; in stdin mode the fixture bytes are piped in without a path or
// --lang, so aid has to detect the language itself. A non-zero exit returns an
// error that carries stderr.
func runAid(bin, dataDir string, fx fixture, c check) (string, error) {
	var args []string
	var stdin []byte
	if c.ViaStdin {
		src, err := os.ReadFile(filepath.Join(dataDir, fx.File))
		if err != nil {
			return "", fmt.Errorf("reading fixture: %w", err)
		}
		stdin = src
		args = append(append(args, c.Flags...), "--format", "text")
	} else {
		args = append(append([]string{fx.File}, c.Flags...), "--format", "text", "--stdout", "--summary-type=off")
	}

	cmd := exec.Command(bin, args...)
	cmd.Dir = dataDir
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return stdout.String(), fmt.Errorf("aid %s: %w\nstderr:\n%s", strings.Join(args, " "), err, stderr.String())
	}
	return stdout.String(), nil
}

// verify returns one message per assertion of c that out violates. A run
// error is itself a violation.
func verify(out string, runErr error, c check) []string {
	var problems []string
	if runErr != nil {
		problems = append(problems, fmt.Sprintf("run failed: %v", runErr))
	}
	for _, s := range c.Contains {
		if !strings.Contains(out, s) {
			problems = append(problems, fmt.Sprintf("missing %q", s))
		}
	}
	for _, s := range c.NotContains {
		if strings.Contains(out, s) {
			problems = append(problems, fmt.Sprintf("unexpected %q", s))
		}
	}
	for _, s := range c.Once {
		if n := strings.Count(out, s); n != 1 {
			problems = append(problems, fmt.Sprintf("%q occurs %d times, want exactly once", s, n))
		}
	}
	return problems
}
