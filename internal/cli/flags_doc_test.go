package cli

import (
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/janreges/ai-distiller/internal/ai"
	"github.com/janreges/ai-distiller/internal/aiactions"
)

// documentedFlags returns every flag a user may rely on: defined, not deprecated, not hidden.
func documentedFlags() []string {
	var names []string
	rootCmd.Flags().VisitAll(func(f *pflag.Flag) {
		if f.Deprecated == "" && !f.Hidden {
			names = append(names, f.Name)
		}
	})
	sort.Strings(names)
	return names
}

var flagToken = regexp.MustCompile(`--[a-z][a-z0-9-]+`)

func assertOnlyRealFlags(t *testing.T, label, text string) {
	t.Helper()
	for _, tok := range flagToken.FindAllString(text, -1) {
		assert.NotNil(t, rootCmd.Flags().Lookup(strings.TrimPrefix(tok, "--")), "%s mentions %s, which is not a flag", label, tok)
	}
}

// captureStdout runs fn and returns everything it printed to os.Stdout.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w
	done := make(chan string)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()
	fn()
	w.Close()
	os.Stdout = orig
	return <-done
}

func TestExtendedHelpCoversEveryFlag(t *testing.T) {
	text := getExtendedHelpContent()
	for _, name := range documentedFlags() {
		assert.Contains(t, text, "--"+name, "extended help does not mention --%s", name)
	}
}

func TestHelpTextsNameOnlyRealFlags(t *testing.T) {
	assertOnlyRealFlags(t, "help template", getHelpTemplate())
	assertOnlyRealFlags(t, "extended help", getExtendedHelpContent())
	assertOnlyRealFlags(t, "filtering help", captureStdout(t, showFilteringHelp))
	assertOnlyRealFlags(t, "cheat sheet", captureStdout(t, showCheatSheet))
	assertOnlyRealFlags(t, "git help", captureStdout(t, showGitHelp))
	assertOnlyRealFlags(t, "rootCmd.Long", rootCmd.Long)
}

func TestCommandLineOptionsDocMatchesFlags(t *testing.T) {
	doc, err := os.ReadFile(filepath.Join("..", "..", "docs", "user", "COMMAND-LINE-OPTIONS.md"))
	require.NoError(t, err)
	for _, name := range documentedFlags() {
		assert.Contains(t, string(doc), "--"+name, "COMMAND-LINE-OPTIONS.md does not mention --%s", name)
	}
	assertOnlyRealFlags(t, "COMMAND-LINE-OPTIONS.md", string(doc))
}

func TestCommandLineOptionsDocListsEveryAIAction(t *testing.T) {
	doc, err := os.ReadFile(filepath.Join("..", "..", "docs", "user", "COMMAND-LINE-OPTIONS.md"))
	require.NoError(t, err)
	registry := ai.NewActionRegistry()
	aiactions.Register(registry)
	actions := registry.List()
	require.NotEmpty(t, actions)
	for _, action := range actions {
		assert.Contains(t, string(doc), "`"+action.Name()+"`", "COMMAND-LINE-OPTIONS.md does not list AI action %s", action.Name())
	}
}

// docTexts returns every help text and the option reference, by name.
func docTexts(t *testing.T) map[string]string {
	t.Helper()
	doc, err := os.ReadFile(filepath.Join("..", "..", "docs", "user", "COMMAND-LINE-OPTIONS.md"))
	require.NoError(t, err)
	return map[string]string{
		"help template":           getHelpTemplate(),
		"extended help":           getExtendedHelpContent(),
		"actions help":            captureStdout(t, showAIActionsHelp),
		"filtering help":          captureStdout(t, showFilteringHelp),
		"cheat sheet":             captureStdout(t, showCheatSheet),
		"git help":                captureStdout(t, showGitHelp),
		"COMMAND-LINE-OPTIONS.md": string(doc),
	}
}

// --include-only starts from nothing: a recipe without methods or fields
// prints every class, struct and interface with an empty body.
func TestIncludeOnlyRecipesKeepMembers(t *testing.T) {
	recipe := regexp.MustCompile(`--include-only[ =]((?:public|protected|internal|private|comments|docstrings|implementation|imports|annotations|fields|methods)(?:,[a-z]+)*)`)
	for name, text := range docTexts(t) {
		for _, m := range recipe.FindAllStringSubmatch(text, -1) {
			cats := "," + m[1] + ","
			assert.True(t, strings.Contains(cats, ",methods,") || strings.Contains(cats, ",fields,"),
				"%s: --include-only %s keeps no members", name, m[1])
		}
	}
}

// Outside git mode aid prints nothing without --stdout, so "aid ... > file"
// or "aid ... | cmd" would capture only status text.
func TestRedirectExamplesPrint(t *testing.T) {
	example := regexp.MustCompile(`(?m)^[ \t]*aid [^\n#]*[ \t][>|][ \t][^\n]*`)
	for name, text := range docTexts(t) {
		for _, ex := range example.FindAllString(text, -1) {
			if strings.Contains(ex, ".git") || strings.Contains(ex, "--stdout") {
				continue
			}
			t.Errorf("%s: %q captures stdout but has no --stdout", name, strings.TrimSpace(ex))
		}
	}
}
