package testrunner

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildBinary_PathOutsideRepo(t *testing.T) {
	root, err := ProjectRoot()
	require.NoError(t, err)
	bin, cleanup, err := BuildBinary(root)
	require.NoError(t, err)
	defer cleanup()
	assert.False(t, strings.HasPrefix(bin, root+string(os.PathSeparator)), "binary must not live in the repo")
	out, err := exec.Command(bin, "--version").CombinedOutput()
	require.NoError(t, err, string(out))
	assert.Contains(t, string(out), "aid version")
}

func TestBuildBinary_HonorsEnvOverride(t *testing.T) {
	root, err := ProjectRoot()
	require.NoError(t, err)
	real, cleanup, err := BuildBinary(root)
	require.NoError(t, err)
	defer cleanup()
	t.Setenv("AID_TEST_BINARY", real)
	got, cleanup2, err := BuildBinary(root)
	require.NoError(t, err)
	defer cleanup2()
	assert.Equal(t, real, got)
}

func TestFormatMismatch_ShowsDiffAndCommand(t *testing.T) {
	tc := TestCase{Language: "go", ScenarioName: "01_basic", ExpectedFile: "/x/testdata/go/01_basic/expected/default.txt"}
	msg := formatMismatch(tc, []string{"go/01_basic/source.go", "--format", "text"}, []byte("a\nb\nc\n"), []byte("a\nB\nc\n"))
	assert.Contains(t, msg, "aid go/01_basic/source.go --format text")
	assert.Contains(t, msg, "-b")
	assert.Contains(t, msg, "+B")
	assert.NotContains(t, msg, "EXPECTED:")
}
