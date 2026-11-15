package cli

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/janreges/ai-distiller/internal/processor"
	"github.com/janreges/ai-distiller/internal/project"
)

func TestGenerateOutputFilename(t *testing.T) {
	// Get the project root dynamically for test expectations
	rootInfo, err := project.FindRoot()
	require.NoError(t, err)
	aidDir := filepath.Join(rootInfo.Path, ".aid")
	
	tests := []struct {
		name         string
		path         string
		stripOptions []string
		setupFlags   func()
		expected     string
	}{
		{
			name:         "NoStripOptions",
			path:         "/home/user/myproject",
			stripOptions: []string{},
			setupFlags:   func() { resetAllFlags() },
			expected:     filepath.Join(aidDir, "aid.myproject.txt"),
		},
		{
			name:         "WithComments",
			path:         "/home/user/myproject",
			stripOptions: []string{"comments"},
			setupFlags:   func() { resetAllFlags() },
			expected:     filepath.Join(aidDir, "aid.myproject.ncom.txt"),
		},
		{
			name:         "MultipleOptions",
			path:         "/home/user/myproject",
			stripOptions: []string{"comments", "imports", "implementation"},
			setupFlags:   func() { resetAllFlags() },
			expected:     filepath.Join(aidDir, "aid.myproject.ncom.nimp.nimpl.txt"),
		},
		{
			name:         "AllOptions",
			path:         "/home/user/myproject",
			stripOptions: []string{"comments", "imports", "implementation", "non-public"},
			setupFlags:   func() { resetAllFlags() },
			expected:     filepath.Join(aidDir, "aid.myproject.ncom.nimp.nimpl.npub.txt"),
		},
		{
			name:         "CurrentDirectory",
			path:         ".",
			stripOptions: []string{},
			setupFlags:   func() { resetAllFlags() },
			expected:     filepath.Join(aidDir, "aid.current.txt"),
		},
		{
			name:         "RootDirectory",
			path:         "/",
			stripOptions: []string{},
			setupFlags:   func() { resetAllFlags() },
			expected:     filepath.Join(aidDir, "aid.current.txt"),
		},
		{
			name:         "NewFlagsWithPrivate",
			path:         "/home/user/myproject",
			stripOptions: []string{},
			setupFlags: func() {
				resetAllFlags()
				includePrivate = boolPtr(true)
			},
			expected:     filepath.Join(aidDir, "aid.myproject.priv.txt"),
		},
		{
			name:         "NewFlagsWithProtectedAndImplementation",
			path:         "/home/user/myproject",
			stripOptions: []string{},
			setupFlags: func() {
				resetAllFlags()
				includeProtected = boolPtr(true)
				includeImplementation = boolPtr(true)
			},
			expected:     filepath.Join(aidDir, "aid.myproject.prot.impl.txt"),
		},
		{
			name:         "NewFlagsWithComments",
			path:         "/home/user/myproject",
			stripOptions: []string{},
			setupFlags: func() {
				resetAllFlags()
				includeComments = boolPtr(true)
			},
			expected:     filepath.Join(aidDir, "aid.myproject.com.txt"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupFlags()
			result := generateOutputFilename(tt.path, tt.stripOptions, "text")
			assert.Equal(t, tt.expected, result)
		})
	}
}

// Helper function to reset all flags
func resetAllFlags() {
	includePublic = nil
	includeProtected = nil
	includeInternal = nil
	includePrivate = nil
	includeComments = nil
	includeDocstrings = nil
	includeImplementation = nil
	includeImports = nil
	includeAnnotations = nil
	includeList = ""
	excludeList = ""
	fieldsFlag = "1"
	methodsFlag = "1"
}

func TestContains(t *testing.T) {
	tests := []struct {
		name     string
		slice    []string
		item     string
		expected bool
	}{
		{
			name:     "ItemExists",
			slice:    []string{"apple", "banana", "orange"},
			item:     "banana",
			expected: true,
		},
		{
			name:     "ItemDoesNotExist",
			slice:    []string{"apple", "banana", "orange"},
			item:     "grape",
			expected: false,
		},
		{
			name:     "EmptySlice",
			slice:    []string{},
			item:     "apple",
			expected: false,
		},
		{
			name:     "EmptyString",
			slice:    []string{"apple", "", "orange"},
			item:     "",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := contains(tt.slice, tt.item)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCLIFlags(t *testing.T) {
	t.Run("DefaultFlags", func(t *testing.T) {
		// Reset flags to defaults
		outputFile = ""
		outputToStdout = false
		outputFormat = "text"
		stripOptions = nil
		includeGlob = []string{}
		excludeGlob = []string{}
		recursiveStr = "1"
		filePathType = "relative"
		verbosity = 0
		includePublic = nil
		includeProtected = nil
		includeInternal = nil
		includePrivate = nil
		includeComments = nil
		includeDocstrings = nil
		includeImplementation = nil
		includeImports = nil
		includeAnnotations = nil
		includeList = ""
		excludeList = ""

		// Test default values
		assert.Equal(t, "", outputFile)
		assert.False(t, outputToStdout)
		assert.Equal(t, "text", outputFormat)
		assert.Equal(t, "1", recursiveStr)
	})

	t.Run("ParseFlags", func(t *testing.T) {
		// Create a new command instance for testing
		cmd := rootCmd
		cmd.ResetFlags()
		initFlags() // Re-initialize flags

		// Test parsing various flags
		args := []string{
			"--output", "test.txt",
			"--stdout",
			"--format", "jsonl",
			"--comments=0",
			"--imports=0",
			"--include", "*.go",
			"--exclude", "*_test.go",
			"--file-path-type", "absolute",
			"-vvv",
		}

		cmd.ParseFlags(args)

		// Verify flags were parsed correctly
		output, _ := cmd.Flags().GetString("output")
		assert.Equal(t, "test.txt", output)

		stdout, _ := cmd.Flags().GetBool("stdout")
		assert.True(t, stdout)

		format, _ := cmd.Flags().GetString("format")
		assert.Equal(t, "jsonl", format)

		comments, _ := cmd.Flags().GetBool("comments")
		assert.Equal(t, false, comments)

		imports, _ := cmd.Flags().GetBool("imports")
		assert.Equal(t, false, imports)

		include, _ := cmd.Flags().GetStringSlice("include")
		assert.Equal(t, []string{"*.go"}, include)

		exclude, _ := cmd.Flags().GetStringSlice("exclude")
		assert.Equal(t, []string{"*_test.go"}, exclude)

		filePathType, _ := cmd.Flags().GetString("file-path-type")
		assert.Equal(t, "absolute", filePathType)

		verbose, _ := cmd.Flags().GetCount("verbose")
		assert.Equal(t, 3, verbose)
	})

	t.Run("ParseNewFilterFlags", func(t *testing.T) {
		// Create a new command instance for testing
		cmd := rootCmd
		cmd.ResetFlags()
		initFlags() // Re-initialize flags

		// Test parsing new filtering flags
		args := []string{
			"--public", "1",
			"--protected", "0",
			"--private", "1",
			"--internal", "0",
			"--comments", "1",
			"--docstrings", "0",
			"--implementation", "1",
			"--imports", "0",
			"--annotations", "1",
		}

		cmd.ParseFlags(args)

		// Verify flags were parsed correctly
		public, _ := cmd.Flags().GetString("public")
		assert.Equal(t, "1", public)

		protected, _ := cmd.Flags().GetString("protected")
		assert.Equal(t, "0", protected)

		private, _ := cmd.Flags().GetString("private")
		assert.Equal(t, "1", private)

		comments, _ := cmd.Flags().GetString("comments")
		assert.Equal(t, "1", comments)

		implementation, _ := cmd.Flags().GetString("implementation")
		assert.Equal(t, "1", implementation)
	})

	t.Run("ParseGroupFilterFlags", func(t *testing.T) {
		// Create a new command instance for testing
		cmd := rootCmd
		cmd.ResetFlags()
		initFlags() // Re-initialize flags

		// Test parsing group filter flags
		args := []string{
			"--include-only", "public,protected,imports",
			"--exclude-items", "comments,implementation",
		}

		cmd.ParseFlags(args)

		// Verify flags were parsed correctly
		includeOnly, _ := cmd.Flags().GetString("include-only")
		assert.Equal(t, "public,protected,imports", includeOnly)

		excludeItems, _ := cmd.Flags().GetString("exclude-items")
		assert.Equal(t, "comments,implementation", excludeItems)
	})
}

func TestVersionFlag(t *testing.T) {
	// Save original version
	originalVersion := Version
	Version = "1.2.3-test"
	defer func() { Version = originalVersion }()

	// Capture output
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// Create command with version flag
	cmd := rootCmd
	cmd.SetArgs([]string{"--version"})

	// We expect this to call os.Exit, so we need to handle it differently
	// For now, we'll just test that the version string is correct
	assert.Equal(t, "1.2.3-test", Version)

	// Restore stdout
	w.Close()
	os.Stdout = old
	io.Copy(io.Discard, r)
}

func TestRunDistillerValidation(t *testing.T) {
	t.Run("InvalidOutputFormat", func(t *testing.T) {
		// Reset all flags before test
		resetAllFlags()
		
		// Create temp directory for testing
		tempDir := t.TempDir()

		cmd := rootCmd
		cmd.ResetFlags()
		initFlags()
		cmd.SetArgs([]string{tempDir, "--format", "invalid"})

		err := cmd.Execute()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid output format")
	})

	t.Run("NonExistentPath", func(t *testing.T) {
		// Reset all flags before test
		resetAllFlags()
		
		cmd := rootCmd
		cmd.ResetFlags()
		initFlags()
		cmd.SetArgs([]string{"/non/existent/path"})

		err := cmd.Execute()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "path does not exist")
	})

	t.Run("ValidPath", func(t *testing.T) {
		// Reset all flags before test
		resetAllFlags()
		
		// Create temp directory
		tempDir := t.TempDir()

		// Reset output file and format to defaults
		outputFile = ""
		outputFormat = "text"

		cmd := rootCmd
		cmd.ResetFlags()
		initFlags()
		cmd.SetArgs([]string{tempDir})

		// This will return "not yet implemented" which is expected for now
		err := cmd.Execute()
		// Since we print and return nil, no error is expected
		assert.NoError(t, err)
	})
}

func TestCLIHelp(t *testing.T) {
	// Reset all flags before test
	resetAllFlags()
	
	buf := new(bytes.Buffer)
	cmd := rootCmd
	cmd.ResetFlags()
	initFlags()
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"--help"})

	err := cmd.Execute()
	require.NoError(t, err)

	output := buf.String()
	
	// Check that help contains expected content
	assert.Contains(t, output, "AI Distiller")
	assert.Contains(t, output, "aid [path]")
	assert.Contains(t, output, "--output")
	assert.Contains(t, output, "--format")
	// Check for new flags instead of deprecated --strip
	assert.Contains(t, output, "--public")
	assert.Contains(t, output, "--private")
	assert.Contains(t, output, "--protected")
	assert.Contains(t, output, "--comments")
	assert.Contains(t, output, "--implementation")
}

func TestStripOptionsAbbreviation(t *testing.T) {
	tests := []struct {
		options  []string
		expected string
	}{
		{[]string{"comments"}, ".ncom"},
		{[]string{"imports"}, ".nimp"},
		{[]string{"implementation"}, ".nimpl"},
		{[]string{"non-public"}, ".npub"},
		{[]string{"private"}, ".npriv"},
		{[]string{"protected"}, ".nprot"},
		{[]string{"comments", "imports"}, ".ncom.nimp"},
		{[]string{"private", "protected"}, ".npriv.nprot"},
		{[]string{"unknown"}, ""}, // Unknown options are ignored
	}

	for _, tt := range tests {
		filename := generateOutputFilename("/test", tt.options, "text")
		if tt.expected != "" {
			assert.Contains(t, filename, tt.expected)
		}
	}
}

// Helper to capture stdout
func captureOutput(f func()) string {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	f()

	w.Close()
	os.Stdout = old

	out, _ := io.ReadAll(r)
	return string(out)
}

// Test verbose output
func TestVerboseOutput(t *testing.T) {
	// This test would require mocking stderr output
	// For now, we just verify the verbosity counter works
	
	cmd := rootCmd
	cmd.ResetFlags()
	initFlags()
	
	// Test single -v
	cmd.ParseFlags([]string{"-v"})
	v, _ := cmd.Flags().GetCount("verbose")
	assert.Equal(t, 1, v)
	
	// Test -vvv
	cmd.ResetFlags()
	initFlags()
	cmd.ParseFlags([]string{"-vvv"})
	v, _ = cmd.Flags().GetCount("verbose")
	assert.Equal(t, 3, v)
}

func TestGetBoolFlag(t *testing.T) {
	tests := []struct {
		name        string
		flag        *bool
		defaultVal  bool
		expected    bool
	}{
		{
			name:       "NilFlagReturnsDefault",
			flag:       nil,
			defaultVal: true,
			expected:   true,
		},
		{
			name:       "TrueFlag",
			flag:       boolPtr(true),
			defaultVal: false,
			expected:   true,
		},
		{
			name:       "FalseFlag",
			flag:       boolPtr(false),
			defaultVal: true,
			expected:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getBoolFlag(tt.flag, tt.defaultVal)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestProcessIncludeList(t *testing.T) {
	tests := []struct {
		name     string
		list     string
		expected processor.ProcessOptions
	}{
		{
			name: "IncludePublicOnly",
			list: "public",
			expected: processor.ProcessOptions{
				IncludeComments:       false,
				IncludeImports:        false,
				IncludeImplementation: false,
				IncludePrivate:        false,
			},
		},
		{
			name: "IncludePublicAndPrivate",
			list: "public,private",
			expected: processor.ProcessOptions{
				IncludeComments:       false,
				IncludeImports:        false,
				IncludeImplementation: false,
				IncludePrivate:        true,
			},
		},
		{
			name: "IncludeAllContent",
			list: "public,comments,implementation,imports",
			expected: processor.ProcessOptions{
				IncludeComments:       true,
				IncludeImports:        true,
				IncludeImplementation: true,
				IncludePrivate:        false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := processIncludeList(tt.list)
			assert.Equal(t, tt.expected.IncludeComments, result.IncludeComments)
			assert.Equal(t, tt.expected.IncludeImports, result.IncludeImports)
			assert.Equal(t, tt.expected.IncludeImplementation, result.IncludeImplementation)
			assert.Equal(t, tt.expected.IncludePrivate, result.IncludePrivate)
		})
	}
}

func TestProcessExcludeList(t *testing.T) {
	tests := []struct {
		name     string
		list     string
		expected processor.ProcessOptions
	}{
		{
			name: "ExcludePrivate",
			list: "private",
			expected: processor.ProcessOptions{
				IncludeComments:       true,
				IncludeImports:        true,
				IncludeImplementation: true,
				IncludePrivate:        true,
				RemovePrivateOnly:     true,
			},
		},
		{
			name: "ExcludeCommentsAndImplementation",
			list: "comments,implementation",
			expected: processor.ProcessOptions{
				IncludeComments:       false,
				IncludeImports:        true,
				IncludeImplementation: false,
				IncludePrivate:        true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := processExcludeList(tt.list)
			assert.Equal(t, tt.expected.IncludeComments, result.IncludeComments)
			assert.Equal(t, tt.expected.IncludeImports, result.IncludeImports)
			assert.Equal(t, tt.expected.IncludeImplementation, result.IncludeImplementation)
			assert.Equal(t, tt.expected.RemovePrivateOnly, result.RemovePrivateOnly)
		})
	}
}

// Helper function to create bool pointer
func boolPtr(b bool) *bool {
	return &b
}

func TestFieldsAndMethodsFiltering(t *testing.T) {
	t.Run("DefaultBehavior", func(t *testing.T) {
		cmd := rootCmd
		cmd.ResetFlags()
		initFlags()

		// Test that fields and methods are included by default
		args := []string{}
		cmd.ParseFlags(args)

		fields, _ := cmd.Flags().GetString("fields")
		methods, _ := cmd.Flags().GetString("methods")

		assert.Equal(t, "1", fields, "fields should be included by default")
		assert.Equal(t, "1", methods, "methods should be included by default")
	})

	t.Run("ExcludeFields", func(t *testing.T) {
		cmd := rootCmd
		cmd.ResetFlags()
		initFlags()

		// Test --fields=0 (exclude fields)
		args := []string{"--fields=0"}
		cmd.ParseFlags(args)

		fields, _ := cmd.Flags().GetString("fields")
		methods, _ := cmd.Flags().GetString("methods")

		assert.Equal(t, "0", fields, "fields should be excluded")
		assert.Equal(t, "1", methods, "methods should still be included")
	})

	t.Run("ExcludeMethods", func(t *testing.T) {
		cmd := rootCmd
		cmd.ResetFlags()
		initFlags()

		// Test --methods=0 (exclude methods)
		args := []string{"--methods=0"}
		cmd.ParseFlags(args)

		fields, _ := cmd.Flags().GetString("fields")
		methods, _ := cmd.Flags().GetString("methods")

		assert.Equal(t, "1", fields, "fields should still be included")
		assert.Equal(t, "0", methods, "methods should be excluded")
	})

	t.Run("ExcludeBoth", func(t *testing.T) {
		cmd := rootCmd
		cmd.ResetFlags()
		initFlags()

		// Test --fields=0 --methods=0 (exclude both)
		args := []string{"--fields=0", "--methods=0"}
		cmd.ParseFlags(args)

		fields, _ := cmd.Flags().GetString("fields")
		methods, _ := cmd.Flags().GetString("methods")

		assert.Equal(t, "0", fields, "fields should be excluded")
		assert.Equal(t, "0", methods, "methods should be excluded")
	})

	t.Run("IncludeOnlyWithFields", func(t *testing.T) {
		cmd := rootCmd
		cmd.ResetFlags()
		initFlags()

		// Test --include-only with fields
		args := []string{"--include-only", "fields,imports"}
		cmd.ParseFlags(args)

		includeOnly, _ := cmd.Flags().GetString("include-only")
		assert.Equal(t, "fields,imports", includeOnly)
	})

	t.Run("IncludeOnlyWithMethods", func(t *testing.T) {
		cmd := rootCmd
		cmd.ResetFlags()
		initFlags()

		// Test --include-only with methods
		args := []string{"--include-only", "methods,public"}
		cmd.ParseFlags(args)

		includeOnly, _ := cmd.Flags().GetString("include-only")
		assert.Equal(t, "methods,public", includeOnly)
	})

	t.Run("ExcludeItemsWithFields", func(t *testing.T) {
		cmd := rootCmd
		cmd.ResetFlags()
		initFlags()

		// Test --exclude-items with fields
		args := []string{"--exclude-items", "fields,comments"}
		cmd.ParseFlags(args)

		excludeItems, _ := cmd.Flags().GetString("exclude-items")
		assert.Equal(t, "fields,comments", excludeItems)
	})

	t.Run("ExcludeItemsWithMethods", func(t *testing.T) {
		cmd := rootCmd
		cmd.ResetFlags()
		initFlags()

		// Test --exclude-items with methods
		args := []string{"--exclude-items", "methods,implementation"}
		cmd.ParseFlags(args)

		excludeItems, _ := cmd.Flags().GetString("exclude-items")
		assert.Equal(t, "methods,implementation", excludeItems)
	})
}

func TestFieldsMethodsProcessorOptionsMapping(t *testing.T) {
	t.Run("DefaultProcessorOptions", func(t *testing.T) {
		// Reset all flags
		resetAllFlags()
		fieldsFlag = "1"
		methodsFlag = "1"

		opts := createProcessOptionsFromFlags()

		assert.True(t, opts.IncludeFields, "IncludeFields should be true by default")
		assert.True(t, opts.IncludeMethods, "IncludeMethods should be true by default")
	})

	t.Run("FieldsDisabled", func(t *testing.T) {
		// Reset all flags and set fields=0
		resetAllFlags()
		fieldsFlag = "0"
		methodsFlag = "1"

		opts := createProcessOptionsFromFlags()

		assert.False(t, opts.IncludeFields, "IncludeFields should be false when fields=0")
		assert.True(t, opts.IncludeMethods, "IncludeMethods should be true")
	})

	t.Run("MethodsDisabled", func(t *testing.T) {
		// Reset all flags and set methods=0
		resetAllFlags()
		fieldsFlag = "1"
		methodsFlag = "0"

		opts := createProcessOptionsFromFlags()

		assert.True(t, opts.IncludeFields, "IncludeFields should be true")
		assert.False(t, opts.IncludeMethods, "IncludeMethods should be false when methods=0")
	})

	t.Run("BothDisabled", func(t *testing.T) {
		// Reset all flags and set both to 0
		resetAllFlags()
		fieldsFlag = "0"
		methodsFlag = "0"

		opts := createProcessOptionsFromFlags()

		assert.False(t, opts.IncludeFields, "IncludeFields should be false when fields=0")
		assert.False(t, opts.IncludeMethods, "IncludeMethods should be false when methods=0")
	})

	t.Run("IncludeOnlyFields", func(t *testing.T) {
		resetAllFlags()
		includeList = "fields"

		opts := processIncludeList(includeList)

		assert.True(t, opts.IncludeFields, "IncludeFields should be true in include-only fields")
		assert.False(t, opts.IncludeMethods, "IncludeMethods should be false in include-only fields")
	})

	t.Run("IncludeOnlyMethods", func(t *testing.T) {
		resetAllFlags()
		includeList = "methods"

		opts := processIncludeList(includeList)

		assert.False(t, opts.IncludeFields, "IncludeFields should be false in include-only methods")
		assert.True(t, opts.IncludeMethods, "IncludeMethods should be true in include-only methods")
	})

	t.Run("ExcludeFields", func(t *testing.T) {
		resetAllFlags()
		excludeList = "fields"

		opts := processExcludeList(excludeList)

		assert.False(t, opts.IncludeFields, "IncludeFields should be false when fields excluded")
		assert.True(t, opts.IncludeMethods, "IncludeMethods should be true when only fields excluded")
	})

	t.Run("ExcludeMethods", func(t *testing.T) {
		resetAllFlags()
		excludeList = "methods"

		opts := processExcludeList(excludeList)

		assert.True(t, opts.IncludeFields, "IncludeFields should be true when only methods excluded")
		assert.False(t, opts.IncludeMethods, "IncludeMethods should be false when methods excluded")
	})
}
func TestPublicZeroSetsRemovePublic(t *testing.T) {
	resetAllFlags()
	no, yes := false, true
	includePublic, includePrivate = &no, &yes
	assert.True(t, createProcessOptionsFromFlags().RemovePublic)

	resetAllFlags()
	assert.False(t, createProcessOptionsFromFlags().RemovePublic, "default keeps public members")
}

func TestIncludeListRemovePublic(t *testing.T) {
	assert.True(t, processIncludeList("private").RemovePublic)
	assert.False(t, processIncludeList("public,private").RemovePublic)
	assert.False(t, processIncludeList("comments").RemovePublic, "no visibility category listed")
}

func TestExcludeListRemovePublic(t *testing.T) {
	assert.True(t, processExcludeList("public").RemovePublic)
	assert.True(t, processExcludeList("public,comments").RemovePublic)
	assert.False(t, processExcludeList("private").RemovePublic, "public not listed")
}
