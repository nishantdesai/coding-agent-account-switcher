package ags

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type tempFile interface {
	Write([]byte) (int, error)
	Chmod(os.FileMode) error
	Close() error
	Name() string
}

var (
	userHomeDir = os.UserHomeDir
	mkdirAll    = os.MkdirAll
	createTemp  = func(dir string, pattern string) (tempFile, error) { return os.CreateTemp(dir, pattern) }
	removePath  = os.Remove
	renamePath  = os.Rename
)

func expandPath(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("path cannot be empty")
	}

	if strings.HasPrefix(path, "~") {
		home, err := userHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolving home directory: %w", err)
		}
		if path == "~" {
			return home, nil
		}
		if strings.HasPrefix(path, "~/") {
			return filepath.Join(home, path[2:]), nil
		}
	}
	return path, nil
}

func atomicWriteFile(path string, raw []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	if err := mkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("creating parent directory: %w", err)
	}

	tmp, err := createTemp(dir, ".ags-*")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer removePath(tmpName)

	if _, err := tmp.Write(raw); err != nil {
		tmp.Close()
		return fmt.Errorf("writing temp file: %w", err)
	}
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return fmt.Errorf("setting file mode: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("closing temp file: %w", err)
	}

	if err := renamePath(tmpName, path); err != nil {
		return fmt.Errorf("replacing file atomically: %w", err)
	}
	return nil
}

func validateJSONObject(raw []byte) error {
	var payload any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return err
	}
	_, ok := payload.(map[string]any)
	if !ok {
		return fmt.Errorf("expected JSON object at top level")
	}
	return nil
}
