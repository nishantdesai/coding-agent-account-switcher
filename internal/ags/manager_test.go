package ags

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func makeCodexAuthJSON(t *testing.T, exp time.Time) []byte {
	t.Helper()
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`))
	claimsBytes, err := json.Marshal(map[string]any{"exp": exp.Unix()})
	if err != nil {
		t.Fatalf("marshal claims: %v", err)
	}
	claims := base64.RawURLEncoding.EncodeToString(claimsBytes)
	token := header + "." + claims + ".sig"
	return []byte(`{"tokens":{"access_token":"` + token + `"}}`)
}

func writeFile(t *testing.T, path string, raw []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestNewManagerAndPaths(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	m, err := NewManager("~/ags-root")
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	if !strings.HasPrefix(m.rootDir, home) {
		t.Fatalf("expected expanded root under home, got %q", m.rootDir)
	}
	if m.paths[ToolCodex].DefaultRuntime != filepath.Join(home, ".codex", "auth.json") {
		t.Fatalf("unexpected codex runtime path: %q", m.paths[ToolCodex].DefaultRuntime)
	}
}

func TestManagerSaveUseDeleteAndListFlow(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	root := t.TempDir()

	m, err := NewManager(root)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	source1 := filepath.Join(t.TempDir(), "source1.json")
	source2 := filepath.Join(t.TempDir(), "source2.json")

	writeFile(t, source1, makeCodexAuthJSON(t, time.Now().Add(2*time.Hour)))
	writeFile(t, source2, makeCodexAuthJSON(t, time.Now().Add(3*time.Hour)))

	save1, err := m.Save(ToolCodex, "work", source1)
	if err != nil {
		t.Fatalf("save1: %v", err)
	}
	if !save1.ChangedSinceLastSave {
		t.Fatalf("expected first save changed=true")
	}

	save2, err := m.Save(ToolCodex, "work", source1)
	if err != nil {
		t.Fatalf("save2: %v", err)
	}
	if save2.ChangedSinceLastSave {
		t.Fatalf("expected identical save changed=false")
	}

	save3, err := m.Save(ToolCodex, "work", source2)
	if err != nil {
		t.Fatalf("save3: %v", err)
	}
	if !save3.ChangedSinceLastSave {
		t.Fatalf("expected changed save changed=true")
	}

	target := filepath.Join(t.TempDir(), "target-auth.json")
	use1, err := m.Use(ToolCodex, "work", target)
	if err != nil {
		t.Fatalf("use1: %v", err)
	}
	if use1.ChangeSinceLastUse != "first use" {
		t.Fatalf("expected first use signal, got %q", use1.ChangeSinceLastUse)
	}

	use2, err := m.Use(ToolCodex, "work", target)
	if err != nil {
		t.Fatalf("use2: %v", err)
	}
	if use2.ChangeSinceLastUse != "unchanged since last use" {
		t.Fatalf("expected unchanged signal, got %q", use2.ChangeSinceLastUse)
	}

	if _, err := m.Save(ToolCodex, "work", source1); err != nil {
		t.Fatalf("save4: %v", err)
	}
	use3, err := m.Use(ToolCodex, "work", target)
	if err != nil {
		t.Fatalf("use3: %v", err)
	}
	if use3.ChangeSinceLastUse != "changed since last use (likely refreshed)" {
		t.Fatalf("expected changed signal, got %q", use3.ChangeSinceLastUse)
	}

	items, err := m.List(nil)
	if err != nil {
		t.Fatalf("list all: %v", err)
	}
	if len(items) != 1 || items[0].Label != "work" {
		t.Fatalf("unexpected list items: %+v", items)
	}

	filter := ToolCodex
	items, err = m.List(&filter)
	if err != nil {
		t.Fatalf("list filtered: %v", err)
	}
	if len(items) != 1 || items[0].Tool != ToolCodex {
		t.Fatalf("unexpected filtered list: %+v", items)
	}

	del, err := m.Delete(ToolCodex, "work")
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if !del.SnapshotDeleted {
		t.Fatalf("expected snapshot deleted=true")
	}

	items, err = m.List(&filter)
	if err != nil {
		t.Fatalf("list after delete: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected empty list after delete, got %+v", items)
	}
}

func TestManagerErrorBranches(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	m, err := NewManager(t.TempDir())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	if _, err := m.Use(ToolCodex, "missing", filepath.Join(t.TempDir(), "target.json")); err == nil {
		t.Fatalf("expected use missing profile error")
	}
	if _, err := m.Delete(ToolCodex, "missing"); err == nil {
		t.Fatalf("expected delete missing profile error")
	}

	source := filepath.Join(t.TempDir(), "source.json")
	writeFile(t, source, []byte(`{"tokens":{"access_token":"x"}}`))
	res, err := m.Save(ToolCodex, "work", source)
	if err != nil {
		t.Fatalf("save for delete-missing-file case: %v", err)
	}
	if err := os.Remove(res.SnapshotPath); err != nil {
		t.Fatalf("remove snapshot: %v", err)
	}
	del, err := m.Delete(ToolCodex, "work")
	if err != nil {
		t.Fatalf("delete missing snapshot should still succeed: %v", err)
	}
	if del.SnapshotDeleted {
		t.Fatalf("expected SnapshotDeleted=false when file already missing")
	}

	broken := filepath.Join(t.TempDir(), "bad.json")
	writeFile(t, broken, []byte(`not-json`))
	if _, err := m.Save(ToolCodex, "bad", broken); err == nil {
		t.Fatalf("expected save validation error for non-json object")
	}
}

func TestManagerResolveSourcePath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	m, err := NewManager(t.TempDir())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	if _, err := m.resolveSourcePath(ToolCodex, filepath.Join(home, "missing.json")); err == nil {
		t.Fatalf("expected override missing path error")
	}

	override := filepath.Join(home, "override.json")
	writeFile(t, override, []byte(`{"x":1}`))
	got, err := m.resolveSourcePath(ToolCodex, override)
	if err != nil {
		t.Fatalf("override existing path: %v", err)
	}
	if got != override {
		t.Fatalf("expected override path %q got %q", override, got)
	}

	candidate := filepath.Join(home, ".codex", "auth.json")
	writeFile(t, candidate, []byte(`{"x":1}`))
	got, err = m.resolveSourcePath(ToolCodex, "")
	if err != nil {
		t.Fatalf("candidate source path: %v", err)
	}
	if got != candidate {
		t.Fatalf("expected candidate path %q got %q", candidate, got)
	}

	if err := os.Remove(candidate); err != nil {
		t.Fatalf("remove candidate: %v", err)
	}
	if _, err := m.resolveSourcePath(ToolCodex, ""); err == nil {
		t.Fatalf("expected missing candidate error")
	}
}

func TestManagerStateHelpersAndPersistence(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	root := t.TempDir()
	m, err := NewManager(root)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	st, err := m.loadState()
	if err != nil {
		t.Fatalf("loadState missing file: %v", err)
	}
	if st.Version != 1 || len(st.Entries) != 0 {
		t.Fatalf("unexpected default state: %+v", st)
	}

	writeFile(t, m.statePath(), []byte(`{not-json`))
	if _, err := m.loadState(); err == nil {
		t.Fatalf("expected parse error for malformed state")
	}

	writeFile(t, m.statePath(), []byte(`{"version":0}`))
	st, err = m.loadState()
	if err != nil {
		t.Fatalf("loadState version zero: %v", err)
	}
	if st.Version != 1 || st.Entries == nil {
		t.Fatalf("expected state normalization, got %+v", st)
	}

	st.Entries[stateKey(ToolCodex, "w")] = StateEntry{Tool: ToolCodex.String(), Label: "w", SnapshotPath: "x", SavedAt: nowISO()}
	if err := m.saveState(st); err != nil {
		t.Fatalf("saveState: %v", err)
	}

	if stateKey(ToolCodex, "w") != "codex:w" {
		t.Fatalf("unexpected state key")
	}
	if sha256Hex([]byte("x")) == "" {
		t.Fatalf("expected sha256")
	}
	if m.snapshotPath(ToolCodex, "w") == "" || m.statePath() == "" {
		t.Fatalf("expected non-empty paths")
	}
}

func TestManagerLoadStateReadErrorAndSaveStateWriteError(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	root := t.TempDir()
	m, err := NewManager(root)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	if err := os.RemoveAll(m.statePath()); err != nil {
		t.Fatalf("remove state path: %v", err)
	}
	if err := os.MkdirAll(m.statePath(), 0o700); err != nil {
		t.Fatalf("mkdir at state path: %v", err)
	}
	if _, err := m.loadState(); err == nil {
		t.Fatalf("expected read error when state path is directory")
	}

	rootFile := filepath.Join(t.TempDir(), "root-file")
	writeFile(t, rootFile, []byte("x"))
	m2 := &Manager{rootDir: rootFile, paths: m.paths}
	if err := m2.saveState(defaultState()); err == nil {
		t.Fatalf("expected write error when root is a file")
	}
}

func TestManagerListSkipsUnknownToolAndMissingSnapshotInsight(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	root := t.TempDir()
	m, err := NewManager(root)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	state := defaultState()
	state.Entries["unknown:label"] = StateEntry{Tool: "unknown", Label: "label", SnapshotPath: filepath.Join(root, "missing1.json"), SavedAt: nowISO()}
	state.Entries[stateKey(ToolCodex, "a")] = StateEntry{Tool: ToolCodex.String(), Label: "a", SnapshotPath: filepath.Join(root, "missing2.json"), SavedAt: nowISO()}
	if err := m.saveState(state); err != nil {
		t.Fatalf("saveState: %v", err)
	}

	items, err := m.List(nil)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected unknown tool entry skipped, got %+v", items)
	}
	if len(items[0].AuthInsight.Details) == 0 || items[0].AuthInsight.Details[0] != "snapshot missing or unreadable" {
		t.Fatalf("expected missing snapshot detail, got %+v", items[0].AuthInsight)
	}
}

func restoreManagerSeams() func() {
	oldJSONMarshalIndent := jsonMarshalIndent
	oldUserHomeDir := userHomeDir
	return func() {
		jsonMarshalIndent = oldJSONMarshalIndent
		userHomeDir = oldUserHomeDir
	}
}

func TestNewManagerErrorBranches(t *testing.T) {
	if _, err := NewManager("   "); err == nil {
		t.Fatalf("expected expandPath error for empty root")
	}

	restore := restoreManagerSeams()
	defer restore()
	userHomeDir = func() (string, error) { return "", os.ErrNotExist }
	if _, err := NewManager("/tmp/root"); err == nil {
		t.Fatalf("expected home resolution error")
	}
}

func TestManagerSaveErrorPaths(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	root := t.TempDir()
	m, err := NewManager(root)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	if _, err := m.Save(ToolCodex, "work", ""); err == nil {
		t.Fatalf("expected save resolveSourcePath error with empty override and no candidates")
	}

	dirAsSource := filepath.Join(t.TempDir(), "srcdir")
	if err := os.MkdirAll(dirAsSource, 0o700); err != nil {
		t.Fatalf("mkdir source dir: %v", err)
	}
	if _, err := m.Save(ToolCodex, "work", dirAsSource); err == nil {
		t.Fatalf("expected read source auth file error for directory source")
	}

	rootFile := filepath.Join(t.TempDir(), "root-file")
	writeFile(t, rootFile, []byte("x"))
	mBadRoot := &Manager{rootDir: rootFile, paths: m.paths}
	source := filepath.Join(t.TempDir(), "source.json")
	writeFile(t, source, []byte(`{"x":1}`))
	if _, err := mBadRoot.Save(ToolCodex, "work", source); err == nil {
		t.Fatalf("expected snapshot write failure for file root")
	}

	stateDirRoot := t.TempDir()
	mStateDir, err := NewManager(stateDirRoot)
	if err != nil {
		t.Fatalf("NewManager state dir root: %v", err)
	}
	if err := os.MkdirAll(mStateDir.statePath(), 0o700); err != nil {
		t.Fatalf("mkdir at state path: %v", err)
	}
	if _, err := mStateDir.Save(ToolCodex, "work", source); err == nil {
		t.Fatalf("expected save loadState error")
	}

	restore := restoreManagerSeams()
	defer restore()
	jsonMarshalIndent = func(any, string, string) ([]byte, error) { return nil, os.ErrInvalid }
	if _, err := m.Save(ToolCodex, "marshal", source); err == nil {
		t.Fatalf("expected saveState serialization error")
	}
}

func TestManagerUseErrorPaths(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	root := t.TempDir()
	m, err := NewManager(root)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	// loadState error
	if err := os.MkdirAll(m.statePath(), 0o700); err != nil {
		t.Fatalf("mkdir at state path: %v", err)
	}
	if _, err := m.Use(ToolCodex, "work", filepath.Join(t.TempDir(), "target.json")); err == nil {
		t.Fatalf("expected use loadState error")
	}

	// snapshot read error (snapshot path is directory)
	root2 := t.TempDir()
	m2, err := NewManager(root2)
	if err != nil {
		t.Fatalf("NewManager root2: %v", err)
	}
	snapDir := filepath.Join(root2, "snapdir")
	if err := os.MkdirAll(snapDir, 0o700); err != nil {
		t.Fatalf("mkdir snapdir: %v", err)
	}
	state := defaultState()
	state.Entries[stateKey(ToolCodex, "work")] = StateEntry{Tool: ToolCodex.String(), Label: "work", SnapshotPath: snapDir, SavedAt: nowISO()}
	if err := m2.saveState(state); err != nil {
		t.Fatalf("save state: %v", err)
	}
	if _, err := m2.Use(ToolCodex, "work", filepath.Join(t.TempDir(), "target.json")); err == nil {
		t.Fatalf("expected read snapshot error")
	}

	// snapshot invalid json
	root3 := t.TempDir()
	m3, err := NewManager(root3)
	if err != nil {
		t.Fatalf("NewManager root3: %v", err)
	}
	invalidSnap := filepath.Join(root3, "snap.json")
	writeFile(t, invalidSnap, []byte("not-json"))
	state3 := defaultState()
	state3.Entries[stateKey(ToolCodex, "work")] = StateEntry{Tool: ToolCodex.String(), Label: "work", SnapshotPath: invalidSnap, SavedAt: nowISO()}
	if err := m3.saveState(state3); err != nil {
		t.Fatalf("save state3: %v", err)
	}
	if _, err := m3.Use(ToolCodex, "work", filepath.Join(t.TempDir(), "target.json")); err == nil {
		t.Fatalf("expected invalid snapshot json error")
	}

	// default target path branch
	root4 := t.TempDir()
	m4, err := NewManager(root4)
	if err != nil {
		t.Fatalf("NewManager root4: %v", err)
	}
	source := filepath.Join(t.TempDir(), "source.json")
	writeFile(t, source, makeCodexAuthJSON(t, time.Now().Add(time.Hour)))
	if _, err := m4.Save(ToolCodex, "work", source); err != nil {
		t.Fatalf("save before default target use: %v", err)
	}
	if _, err := m4.Use(ToolCodex, "work", ""); err != nil {
		t.Fatalf("use default target path: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".codex", "auth.json")); err != nil {
		t.Fatalf("expected default target file to be written: %v", err)
	}

	// target expand path error with HOME lookup failure
	restore := restoreManagerSeams()
	defer restore()
	userHomeDir = func() (string, error) { return "", os.ErrNotExist }
	if _, err := m4.Use(ToolCodex, "work", "~"); err == nil {
		t.Fatalf("expected target expandPath error")
	}

	// target write error (rename onto dir)
	targetDir := filepath.Join(t.TempDir(), "target-dir")
	if err := os.MkdirAll(targetDir, 0o700); err != nil {
		t.Fatalf("mkdir target dir: %v", err)
	}
	if _, err := m4.Use(ToolCodex, "work", targetDir); err == nil {
		t.Fatalf("expected target write error when target is directory")
	}

	// saveState error after use
	restore2 := restoreManagerSeams()
	defer restore2()
	jsonMarshalIndent = func(any, string, string) ([]byte, error) { return nil, os.ErrInvalid }
	if _, err := m4.Use(ToolCodex, "work", filepath.Join(t.TempDir(), "target.json")); err == nil {
		t.Fatalf("expected use saveState serialization error")
	}
}

func TestManagerDeleteErrorPathsAndListCoverage(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	root := t.TempDir()
	m, err := NewManager(root)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	// delete loadState error
	if err := os.MkdirAll(m.statePath(), 0o700); err != nil {
		t.Fatalf("mkdir at state path: %v", err)
	}
	if _, err := m.Delete(ToolCodex, "work"); err == nil {
		t.Fatalf("expected delete loadState error")
	}

	// delete remove non-ErrNotExist
	root2 := t.TempDir()
	m2, err := NewManager(root2)
	if err != nil {
		t.Fatalf("NewManager root2: %v", err)
	}
	nonEmptyDir := filepath.Join(root2, "snap-dir")
	writeFile(t, filepath.Join(nonEmptyDir, "child"), []byte("x"))
	state2 := defaultState()
	state2.Entries[stateKey(ToolCodex, "work")] = StateEntry{Tool: ToolCodex.String(), Label: "work", SnapshotPath: nonEmptyDir, SavedAt: nowISO()}
	if err := m2.saveState(state2); err != nil {
		t.Fatalf("save state2: %v", err)
	}
	if _, err := m2.Delete(ToolCodex, "work"); err == nil {
		t.Fatalf("expected delete remove error for non-empty directory")
	}

	// delete saveState error
	root3 := t.TempDir()
	m3, err := NewManager(root3)
	if err != nil {
		t.Fatalf("NewManager root3: %v", err)
	}
	source := filepath.Join(t.TempDir(), "source.json")
	writeFile(t, source, []byte(`{"x":1}`))
	if _, err := m3.Save(ToolCodex, "work", source); err != nil {
		t.Fatalf("save before delete saveState error: %v", err)
	}
	restore := restoreManagerSeams()
	jsonMarshalIndent = func(any, string, string) ([]byte, error) { return nil, os.ErrInvalid }
	if _, err := m3.Delete(ToolCodex, "work"); err == nil {
		t.Fatalf("expected delete saveState serialization error")
	}
	restore()

	// list loadState error
	root4 := t.TempDir()
	m4, err := NewManager(root4)
	if err != nil {
		t.Fatalf("NewManager root4: %v", err)
	}
	if err := os.MkdirAll(m4.statePath(), 0o700); err != nil {
		t.Fatalf("mkdir at state path root4: %v", err)
	}
	if _, err := m4.List(nil); err == nil {
		t.Fatalf("expected list loadState error")
	}

	// list toolFilter mismatch and sorting comparator branches
	root5 := t.TempDir()
	m5, err := NewManager(root5)
	if err != nil {
		t.Fatalf("NewManager root5: %v", err)
	}
	s1 := filepath.Join(root5, "s1.json")
	s2 := filepath.Join(root5, "s2.json")
	s3 := filepath.Join(root5, "s3.json")
	writeFile(t, s1, makeCodexAuthJSON(t, time.Now().Add(time.Hour)))
	writeFile(t, s2, makeCodexAuthJSON(t, time.Now().Add(2*time.Hour)))
	writeFile(t, s3, []byte(`{"provider":{"expires":9999999999999}}`))
	state5 := defaultState()
	state5.Entries[stateKey(ToolCodex, "b")] = StateEntry{Tool: ToolCodex.String(), Label: "b", SnapshotPath: s2, SavedAt: nowISO()}
	state5.Entries[stateKey(ToolCodex, "a")] = StateEntry{Tool: ToolCodex.String(), Label: "a", SnapshotPath: s1, SavedAt: nowISO()}
	state5.Entries[stateKey(ToolPi, "p")] = StateEntry{Tool: ToolPi.String(), Label: "p", SnapshotPath: s3, SavedAt: nowISO()}
	if err := m5.saveState(state5); err != nil {
		t.Fatalf("save state5: %v", err)
	}
	allItems, err := m5.List(nil)
	if err != nil {
		t.Fatalf("list all sorted: %v", err)
	}
	if len(allItems) != 3 {
		t.Fatalf("expected three items in all list, got %+v", allItems)
	}

	filter := ToolCodex
	items, err := m5.List(&filter)
	if err != nil {
		t.Fatalf("list filtered/sorted: %v", err)
	}
	if len(items) != 2 || items[0].Label != "a" || items[1].Label != "b" {
		t.Fatalf("expected sorted codex items [a,b], got %+v", items)
	}
}

func TestResolveSourcePathExpandErrorAndSaveStateSerializeError(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	m, err := NewManager(t.TempDir())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	restore := restoreManagerSeams()
	defer restore()
	userHomeDir = func() (string, error) { return "", os.ErrNotExist }
	if _, err := m.resolveSourcePath(ToolCodex, "~"); err == nil {
		t.Fatalf("expected expandPath error in resolveSourcePath")
	}

	jsonMarshalIndent = func(any, string, string) ([]byte, error) { return nil, os.ErrInvalid }
	if err := m.saveState(defaultState()); err == nil {
		t.Fatalf("expected saveState serialization error")
	}
}
