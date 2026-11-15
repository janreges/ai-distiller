package ignore

import (
	"os"
	"path/filepath"
	"testing"
)

// TestBinObjIgnoreWithSlash tests bin/ and obj/ patterns with trailing slash
func TestBinObjIgnoreWithSlash(t *testing.T) {
	// Create a temporary directory structure mimicking a .NET project
	tmpDir := t.TempDir()

	// Create directory structure like a typical .NET project
	createDotNetProject(t, tmpDir)

	// Create .aidignore file with bin/ and obj/ patterns like user would have
	aidignoreContent := `# Standard .NET ignores
bin/
obj/
*.dll
*.exe
*.pdb

# Visual Studio files
.vs/
*.user
*.suo

# User specific files
temp/
logs/
`

	err := os.WriteFile(filepath.Join(tmpDir, ".aidignore"), []byte(aidignoreContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create .aidignore: %v", err)
	}

	// Create matcher
	matcher, err := New(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create matcher: %v", err)
	}

	tests := []struct {
		path     string
		expected bool
		desc     string
	}{
		// Files that should be KEPT (not ignored)
		{"Program.cs", false, "main source file should be kept"},
		{"src/Controllers/HomeController.cs", false, "controller source should be kept"},
		{"src/Models/User.cs", false, "model source should be kept"},
		{"tests/Unit/UserTest.cs", false, "unit test should be kept"},
		{"MyProject.csproj", false, "project file should be kept"},
		{"README.md", false, "readme should be kept"},

		// Files that should be IGNORED
		{"bin", true, "bin directory itself should be ignored"},
		{"bin/Debug", true, "bin/Debug directory should be ignored"},
		{"bin/Release", true, "bin/Release directory should be ignored"},
		{"bin/Debug/MyProject.exe", true, "executable in bin/Debug should be ignored"},
		{"bin/Release/MyProject.exe", true, "executable in bin/Release should be ignored"},
		{"bin/Debug/MyProject.dll", true, "dll in bin/Debug should be ignored"},
		{"bin/Release/MyProject.dll", true, "dll in bin/Release should be ignored"},
		{"bin/Debug/MyProject.pdb", true, "pdb in bin/Debug should be ignored"},
		{"bin/any/deep/nested/file.dll", true, "any nested file in bin should be ignored"},

		{"obj", true, "obj directory itself should be ignored"},
		{"obj/Debug", true, "obj/Debug directory should be ignored"},
		{"obj/Release", true, "obj/Release directory should be ignored"},
		{"obj/Debug/MyProject.csproj.FileListAbsolute.txt", true, "build file in obj/Debug should be ignored"},
		{"obj/Release/MyProject.dll", true, "dll in obj/Release should be ignored"},
		{"obj/Debug/MyProject.pdb", true, "pdb in obj/Debug should be ignored"},
		{"obj/any/deep/nested/build.cache", true, "any nested file in obj should be ignored"},

		// Additional ignores from .aidignore
		{"temp", true, "temp directory should be ignored"},
		{"temp/cache.tmp", true, "temp file should be ignored"},
		{"logs", true, "logs directory should be ignored"},
		{"logs/app.log", true, "log file should be ignored"},
		{".vs", true, ".vs directory should be ignored"},
		{".vs/MyProject/v16/.suo", true, "VS user file should be ignored"},

		// Binary files anywhere should be ignored by extension
		{"some/path/library.dll", true, "dll anywhere should be ignored"},
		{"root.exe", true, "exe in root should be ignored"},
		{"debug.pdb", true, "pdb anywhere should be ignored"},
	}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			fullPath := filepath.Join(tmpDir, test.path)
			result := matcher.ShouldIgnore(fullPath)
			if result != test.expected {
				t.Errorf("ShouldIgnore(%s) = %v, want %v (%s)", test.path, result, test.expected, test.desc)
			}
		})
	}
}

// createDotNetProject creates a realistic .NET project structure
func createDotNetProject(t *testing.T, root string) {
	t.Helper()

	// Create directories
	dirs := []string{
		"src/Controllers",
		"src/Models",
		"tests/Unit",
		"tests/Integration",
		"bin/Debug",
		"bin/Release",
		"obj/Debug",
		"obj/Release",
		"temp",
		"logs",
		".vs/MyProject/v16",
	}

	for _, dir := range dirs {
		err := os.MkdirAll(filepath.Join(root, dir), 0755)
		if err != nil {
			t.Fatalf("Failed to create directory %s: %v", dir, err)
		}
	}

	// Create source files (should NOT be ignored)
	sourceFiles := []string{
		"Program.cs",
		"src/Controllers/HomeController.cs",
		"src/Models/User.cs",
		"tests/Unit/UserTest.cs",
		"MyProject.csproj",
		"README.md",
	}

	for _, file := range sourceFiles {
		createFile(t, root, file)
	}

	// Create binary/build files (should be ignored)
	binaryFiles := []string{
		"bin/Debug/MyProject.exe",
		"bin/Release/MyProject.exe",
		"bin/Debug/MyProject.dll",
		"bin/Release/MyProject.dll",
		"bin/Debug/MyProject.pdb",
		"bin/Release/MyProject.pdb",
		"obj/Debug/MyProject.csproj.FileListAbsolute.txt",
		"obj/Release/MyProject.dll",
		"obj/Debug/MyProject.pdb",
		"temp/cache.tmp",
		"logs/app.log",
		".vs/MyProject/v16/.suo",
		"some/path/library.dll",
		"root.exe",
		"debug.pdb",
	}

	for _, file := range binaryFiles {
		// Create directory structure for nested files
		dir := filepath.Dir(filepath.Join(root, file))
		os.MkdirAll(dir, 0755)
		createFile(t, root, file)
	}
}

// TestBinObjIgnoreWithoutSlash tests bin and obj patterns WITHOUT trailing slash
// This tests the specific issue reported by the user
func TestBinObjIgnoreWithoutSlash(t *testing.T) {
	// Create a temporary directory structure mimicking a .NET project
	tmpDir := t.TempDir()

	// Create directory structure like a typical .NET project
	createDotNetProject(t, tmpDir)

	// Create .aidignore file WITHOUT trailing slashes (user's issue)
	aidignoreContent := `# Standard .NET ignores - NO trailing slashes
bin
obj
*.dll
*.exe
*.pdb

# Visual Studio files  
.vs
*.user
*.suo

# User specific files
temp
logs
`

	err := os.WriteFile(filepath.Join(tmpDir, ".aidignore"), []byte(aidignoreContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create .aidignore: %v", err)
	}

	// Create matcher
	matcher, err := New(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create matcher: %v", err)
	}

	tests := []struct {
		path     string
		expected bool
		desc     string
	}{
		// Files that should be KEPT (not ignored)
		{"Program.cs", false, "main source file should be kept"},
		{"src/Controllers/HomeController.cs", false, "controller source should be kept"},
		{"src/Models/User.cs", false, "model source should be kept"},
		{"tests/Unit/UserTest.cs", false, "unit test should be kept"},

		// THE KEY TESTS - directories without slash should still exclude contents
		{"bin", true, "bin directory itself should be ignored (no slash)"},
		{"bin/Debug", true, "bin/Debug directory should be ignored (no slash)"},
		{"bin/Release", true, "bin/Release directory should be ignored (no slash)"},
		{"bin/Debug/MyProject.exe", true, "executable in bin/Debug should be ignored (no slash)"},
		{"bin/Release/MyProject.dll", true, "dll in bin/Release should be ignored (no slash)"},
		{"bin/any/deep/nested/file.cs", true, "source file in bin should be ignored (no slash)"},

		{"obj", true, "obj directory itself should be ignored (no slash)"},
		{"obj/Debug", true, "obj/Debug directory should be ignored (no slash)"},
		{"obj/Release", true, "obj/Release directory should be ignored (no slash)"},
		{"obj/Debug/build.cache", true, "build file in obj/Debug should be ignored (no slash)"},
		{"obj/any/deep/nested/file.cs", true, "source file in obj should be ignored (no slash)"},

		// Additional directory patterns without slash
		{"temp", true, "temp directory should be ignored (no slash)"},
		{"temp/cache.tmp", true, "temp file should be ignored (no slash)"},
		{"logs", true, "logs directory should be ignored (no slash)"},
		{"logs/app.log", true, "log file should be ignored (no slash)"},
		{".vs", true, ".vs directory should be ignored (no slash)"},
		{".vs/config.json", true, "VS config should be ignored (no slash)"},
	}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			fullPath := filepath.Join(tmpDir, test.path)
			result := matcher.ShouldIgnore(fullPath)
			if result != test.expected {
				t.Errorf("ShouldIgnore(%s) = %v, want %v (%s)", test.path, result, test.expected, test.desc)
			}
		})
	}
}
