package project

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfineRoot(t *testing.T) {
	defer ResetCache()
	tmp, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(tmp, "root")
	for _, dir := range []string{filepath.Join(root, "sub"), filepath.Join(tmp, "outside")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, f := range []string{filepath.Join(root, "sub", "in.py"), filepath.Join(tmp, "outside", "out.py")} {
		if err := os.WriteFile(f, []byte("x = 1\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	links := map[string]string{
		filepath.Join(root, "in-link.py"):  filepath.Join(root, "sub", "in.py"),
		filepath.Join(root, "out-link.py"): filepath.Join(tmp, "outside", "out.py"),
		filepath.Join(tmp, "root-link"):    root,
	}
	for link, target := range links {
		if err := os.Symlink(target, link); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("Unset", func(t *testing.T) {
		t.Setenv(EnvConfineRoot, "")
		ResetCache()
		if got, err := ConfineRoot(); got != "" || err != nil {
			t.Fatalf("ConfineRoot() = %q, %v; want \"\", nil", got, err)
		}
		if !InConfineRoot(filepath.Join(tmp, "outside", "out.py")) {
			t.Error("unset: every path must be allowed")
		}
	})

	t.Run("SymlinkedValueIsResolved", func(t *testing.T) {
		t.Setenv(EnvConfineRoot, filepath.Join(tmp, "root-link"))
		ResetCache()
		if got, err := ConfineRoot(); got != root || err != nil {
			t.Fatalf("ConfineRoot() = %q, %v; want %q, nil", got, err, root)
		}
		for path, want := range map[string]bool{
			filepath.Join(root, "sub", "in.py"):               true,
			filepath.Join(root, "in-link.py"):                 true,
			filepath.Join(tmp, "root-link", "sub", "in.py"):   true,
			filepath.Join(root, "out-link.py"):                false,
			filepath.Join(tmp, "outside", "out.py"):           false,
			filepath.Join(root, "sub", "..", "..", "outside"): false,
			filepath.Join(root, "missing.py"):                 false,
			root + "-sibling":                                 false,
		} {
			if got := InConfineRoot(path); got != want {
				t.Errorf("InConfineRoot(%s) = %v, want %v", path, got, want)
			}
		}
	})

	t.Run("NotADirectory", func(t *testing.T) {
		for _, value := range []string{filepath.Join(tmp, "missing"), filepath.Join(root, "sub", "in.py")} {
			t.Setenv(EnvConfineRoot, value)
			ResetCache()
			_, err := ConfineRoot()
			if err == nil || !strings.Contains(err.Error(), EnvConfineRoot+"="+value) {
				t.Errorf("%s: err = %v, want an error naming %s", value, err, EnvConfineRoot)
			}
			if InConfineRoot(filepath.Join(root, "sub", "in.py")) {
				t.Errorf("%s: an invalid value must allow nothing", value)
			}
		}
	})
}
