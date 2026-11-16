package project

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindRoot(t *testing.T) {
	// Save original env and cwd
	origEnv := os.Getenv(EnvProjectRoot)
	origCwd, _ := os.Getwd()
	defer func() {
		os.Setenv(EnvProjectRoot, origEnv)
		os.Chdir(origCwd)
		ResetCache()
	}()

	// Create test directory structure
	tempDir := t.TempDir()

	// Create nested structure: tempDir/repo/service/subdir
	repoDir := filepath.Join(tempDir, "repo")
	serviceDir := filepath.Join(repoDir, "service")
	subDir := filepath.Join(serviceDir, "subdir")

	os.MkdirAll(subDir, 0755)

	// Test 1: Environment variable override
	t.Run("EnvVarOverride", func(t *testing.T) {
		ResetCache()
		os.Setenv(EnvProjectRoot, serviceDir)
		os.Chdir(subDir)

		info, err := FindRoot()
		if err != nil {
			t.Fatalf("FindRoot failed: %v", err)
		}

		if info.Path != serviceDir {
			t.Errorf("Expected root %s, got %s", serviceDir, info.Path)
		}
		if info.Marker != "AID_PROJECT_ROOT" {
			t.Errorf("Expected marker AID_PROJECT_ROOT, got %s", info.Marker)
		}
	})

	// Test 2: .aidrc marker (highest priority)
	t.Run("AidrcMarker", func(t *testing.T) {
		ResetCache()
		os.Unsetenv(EnvProjectRoot)

		// Create both .aidrc and .git to test priority
		os.WriteFile(filepath.Join(serviceDir, ".aidrc"), []byte{}, 0644)
		os.Mkdir(filepath.Join(repoDir, ".git"), 0755)

		os.Chdir(subDir)

		info, err := FindRoot()
		if err != nil {
			t.Fatalf("FindRoot failed: %v", err)
		}

		if info.Path != serviceDir {
			t.Errorf("Expected root %s, got %s", serviceDir, info.Path)
		}
		if info.Marker != ".aidrc" {
			t.Errorf("Expected marker .aidrc, got %s", info.Marker)
		}
	})

	// Test 3: Language-specific markers
	t.Run("GoModMarker", func(t *testing.T) {
		ResetCache()
		os.Remove(filepath.Join(serviceDir, ".aidrc"))
		os.WriteFile(filepath.Join(serviceDir, "go.mod"), []byte("module test"), 0644)

		os.Chdir(subDir)

		info, err := FindRoot()
		if err != nil {
			t.Fatalf("FindRoot failed: %v", err)
		}

		if info.Path != serviceDir {
			t.Errorf("Expected root %s, got %s", serviceDir, info.Path)
		}
		if info.Marker != "go.mod" {
			t.Errorf("Expected marker go.mod, got %s", info.Marker)
		}
	})

	// Test 4: Fallback to CWD
	t.Run("FallbackToCWD", func(t *testing.T) {
		ResetCache()
		// Clean up all markers
		os.Remove(filepath.Join(serviceDir, "go.mod"))
		os.RemoveAll(filepath.Join(repoDir, ".git"))

		os.Chdir(subDir)

		info, err := FindRoot()
		if err != nil {
			t.Fatalf("FindRoot failed: %v", err)
		}

		if info.Path != subDir {
			t.Errorf("Expected root %s, got %s", subDir, info.Path)
		}
		if !info.IsFallback {
			t.Error("Expected IsFallback to be true")
		}

		// The cached result keeps IsFallback: a caller must still be able
		// to tell the working directory from a detected root.
		cached, err := FindRoot()
		if err != nil {
			t.Fatalf("FindRoot failed: %v", err)
		}
		if cached.Marker != "cached" || cached.Path != subDir || !cached.IsFallback {
			t.Errorf("Expected the cached fallback root %s, got %+v", subDir, cached)
		}
	})

	// Test 5: Cache functionality
	t.Run("Caching", func(t *testing.T) {
		ResetCache()
		os.WriteFile(filepath.Join(serviceDir, ".aidrc"), []byte{}, 0644)
		os.Chdir(subDir)

		// First call
		info1, _ := FindRoot()

		// Change directory and marker - should still return cached value
		os.Chdir(tempDir)
		os.Remove(filepath.Join(serviceDir, ".aidrc"))

		info2, _ := FindRoot()

		if info1.Path != info2.Path {
			t.Error("Cache not working - paths differ")
		}
		if info2.Marker != "cached" {
			t.Errorf("Expected cached marker, got %s", info2.Marker)
		}
	})
}

func TestGetAidDir(t *testing.T) {
	origCwd, _ := os.Getwd()
	defer func() {
		os.Chdir(origCwd)
		ResetCache()
	}()

	tempDir := t.TempDir()
	os.Chdir(tempDir)

	// Create .aidrc to mark this as root
	os.WriteFile(filepath.Join(tempDir, ".aidrc"), []byte{}, 0644)

	aidDir, err := GetAidDir()
	if err != nil {
		t.Fatalf("GetAidDir failed: %v", err)
	}

	expected := filepath.Join(tempDir, AidDirName)
	if aidDir != expected {
		t.Errorf("Expected aid dir %s, got %s", expected, aidDir)
	}
}

func TestEnsureAidDir(t *testing.T) {
	origCwd, _ := os.Getwd()
	defer func() {
		os.Chdir(origCwd)
		ResetCache()
	}()

	tempDir := t.TempDir()
	os.Chdir(tempDir)

	// Create .aidrc to mark this as root
	os.WriteFile(filepath.Join(tempDir, ".aidrc"), []byte{}, 0644)

	aidDir, err := EnsureAidDir()
	if err != nil {
		t.Fatalf("EnsureAidDir failed: %v", err)
	}

	// Check that directory was created
	if _, err := os.Stat(aidDir); err != nil {
		t.Errorf("Aid directory was not created: %v", err)
	}
}
