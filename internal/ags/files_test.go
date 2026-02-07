package ags

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

type fakeTempFile struct {
	name     string
	writeErr error
	chmodErr error
	closeErr error
}

func (f *fakeTempFile) Write(_ []byte) (int, error) {
	if f.writeErr != nil {
		return 0, f.writeErr
	}
	return 1, nil
}

func (f *fakeTempFile) Chmod(_ os.FileMode) error {
	return f.chmodErr
}

func (f *fakeTempFile) Close() error {
	return f.closeErr
}

func (f *fakeTempFile) Name() string {
	return f.name
}

func restoreFileSeams() func() {
	oldUserHomeDir := userHomeDir
	oldMkdirAll := mkdirAll
	oldCreateTemp := createTemp
	oldRemovePath := removePath
	oldRenamePath := renamePath
	return func() {
		userHomeDir = oldUserHomeDir
		mkdirAll = oldMkdirAll
		createTemp = oldCreateTemp
		removePath = oldRemovePath
		renamePath = oldRenamePath
	}
}

func TestExpandPath(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		if _, err := expandPath("   "); err == nil {
			t.Fatalf("expected error for empty path")
		}
	})

	t.Run("home lookup error", func(t *testing.T) {
		restore := restoreFileSeams()
		defer restore()
		userHomeDir = func() (string, error) { return "", errors.New("boom") }
		if _, err := expandPath("~"); err == nil {
			t.Fatalf("expected home resolution error")
		}
	})

	t.Run("tilde and subpath", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)

		got, err := expandPath("~")
		if err != nil {
			t.Fatalf("expandPath(~) error: %v", err)
		}
		if got != home {
			t.Fatalf("expected %q got %q", home, got)
		}

		got, err = expandPath("~/foo/bar")
		if err != nil {
			t.Fatalf("expandPath(~/foo/bar) error: %v", err)
		}
		want := filepath.Join(home, "foo", "bar")
		if got != want {
			t.Fatalf("expected %q got %q", want, got)
		}
	})

	t.Run("passthrough", func(t *testing.T) {
		got, err := expandPath("~other/path")
		if err != nil {
			t.Fatalf("expandPath passthrough error: %v", err)
		}
		if got != "~other/path" {
			t.Fatalf("unexpected passthrough result: %q", got)
		}
	})
}

func TestAtomicWriteFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "deep", "file.json")
	content := []byte(`{"ok":true}`)

	if err := atomicWriteFile(path, content, 0o600); err != nil {
		t.Fatalf("atomicWriteFile error: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read written file: %v", err)
	}
	if string(raw) != string(content) {
		t.Fatalf("unexpected content: %q", string(raw))
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat written file: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("expected mode 0600 got %o", info.Mode().Perm())
	}
}

func TestAtomicWriteFileErrorBranches(t *testing.T) {
	t.Run("mkdir parent error", func(t *testing.T) {
		dir := t.TempDir()
		fileParent := filepath.Join(dir, "not-a-dir")
		if err := os.WriteFile(fileParent, []byte("x"), 0o600); err != nil {
			t.Fatalf("prepare file parent: %v", err)
		}
		path := filepath.Join(fileParent, "child.json")
		err := atomicWriteFile(path, []byte("{}"), 0o600)
		if err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("create temp error", func(t *testing.T) {
		restore := restoreFileSeams()
		defer restore()
		createTemp = func(string, string) (tempFile, error) { return nil, errors.New("temp failed") }
		if err := atomicWriteFile(filepath.Join(t.TempDir(), "x.json"), []byte("{}"), 0o600); err == nil {
			t.Fatalf("expected create temp error")
		}
	})

	t.Run("write error", func(t *testing.T) {
		restore := restoreFileSeams()
		defer restore()
		createTemp = func(dir string, _ string) (tempFile, error) {
			return &fakeTempFile{name: filepath.Join(dir, "tmp"), writeErr: errors.New("write failed")}, nil
		}
		if err := atomicWriteFile(filepath.Join(t.TempDir(), "x.json"), []byte("{}"), 0o600); err == nil {
			t.Fatalf("expected write error")
		}
	})

	t.Run("chmod error", func(t *testing.T) {
		restore := restoreFileSeams()
		defer restore()
		createTemp = func(dir string, _ string) (tempFile, error) {
			return &fakeTempFile{name: filepath.Join(dir, "tmp"), chmodErr: errors.New("chmod failed")}, nil
		}
		if err := atomicWriteFile(filepath.Join(t.TempDir(), "x.json"), []byte("{}"), 0o600); err == nil {
			t.Fatalf("expected chmod error")
		}
	})

	t.Run("close error", func(t *testing.T) {
		restore := restoreFileSeams()
		defer restore()
		createTemp = func(dir string, _ string) (tempFile, error) {
			return &fakeTempFile{name: filepath.Join(dir, "tmp"), closeErr: errors.New("close failed")}, nil
		}
		if err := atomicWriteFile(filepath.Join(t.TempDir(), "x.json"), []byte("{}"), 0o600); err == nil {
			t.Fatalf("expected close error")
		}
	})

	t.Run("rename error", func(t *testing.T) {
		restore := restoreFileSeams()
		defer restore()
		createTemp = func(dir string, _ string) (tempFile, error) {
			return &fakeTempFile{name: filepath.Join(dir, "tmp")}, nil
		}
		renamePath = func(string, string) error { return errors.New("rename failed") }
		if err := atomicWriteFile(filepath.Join(t.TempDir(), "x.json"), []byte("{}"), 0o600); err == nil {
			t.Fatalf("expected rename error")
		}
	})
}

func TestValidateJSONObject(t *testing.T) {
	if err := validateJSONObject([]byte(`{"a":1}`)); err != nil {
		t.Fatalf("expected valid object: %v", err)
	}

	if err := validateJSONObject([]byte(`not-json`)); err == nil {
		t.Fatalf("expected parse error")
	}

	if err := validateJSONObject([]byte(`[1,2,3]`)); err == nil {
		t.Fatalf("expected top-level object error")
	}
}
