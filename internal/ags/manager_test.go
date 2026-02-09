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

func makeJWT(t *testing.T, claims map[string]any) string {
	t.Helper()
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`))
	claimsBytes, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("marshal claims: %v", err)
	}
	claimsPart := base64.RawURLEncoding.EncodeToString(claimsBytes)
	return header + "." + claimsPart + ".sig"
}

func makeCodexAuthJSON(t *testing.T, exp time.Time) []byte {
	t.Helper()
	token := makeJWT(t, map[string]any{"exp": exp.Unix()})
	return []byte(`{"tokens":{"access_token":"` + token + `"}}`)
}

func makeCodexAuthJSONWithIdentity(t *testing.T, exp time.Time, accountID string, email string, plan string) []byte {
	t.Helper()
	accessToken := makeJWT(t, map[string]any{"exp": exp.Unix()})
	idClaims := map[string]any{}
	if strings.TrimSpace(email) != "" {
		idClaims["email"] = email
	}
	if strings.TrimSpace(plan) != "" {
		idClaims["auth"] = map[string]any{"chatgpt_plan_type": plan}
	}
	if strings.TrimSpace(accountID) != "" {
		idClaims["account_id"] = accountID
	}
	idToken := makeJWT(t, idClaims)

	payload := map[string]any{
		"tokens": map[string]any{
			"access_token": accessToken,
			"id_token":     idToken,
		},
	}
	if strings.TrimSpace(accountID) != "" {
		payload["tokens"].(map[string]any)["account_id"] = accountID
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal codex auth payload: %v", err)
	}
	return raw
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

func TestManagerCachesIdentityByAccountID(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	root := t.TempDir()

	m, err := NewManager(root)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	accountID := "acct_123"
	sourceWithEmail := filepath.Join(t.TempDir(), "source-with-email.json")
	writeFile(t, sourceWithEmail, makeCodexAuthJSONWithIdentity(t, time.Now().Add(2*time.Hour), accountID, "person@company.com", "plus"))

	savedWithEmail, err := m.Save(ToolCodex, "work", sourceWithEmail)
	if err != nil {
		t.Fatalf("save with email: %v", err)
	}
	if savedWithEmail.Insight.AccountEmail != "person@company.com" || savedWithEmail.Insight.AccountPlan != "Plus" {
		t.Fatalf("expected identity in first save, got %+v", savedWithEmail.Insight)
	}

	state, err := m.loadState()
	if err != nil {
		t.Fatalf("loadState after first save: %v", err)
	}
	cacheItem, ok := state.IdentityCache[accountID]
	if !ok || cacheItem.Email != "person@company.com" || cacheItem.Plan != "Plus" {
		t.Fatalf("expected identity cache entry, got %+v", state.IdentityCache)
	}

	sourceWithoutEmail := filepath.Join(t.TempDir(), "source-without-email.json")
	writeFile(t, sourceWithoutEmail, makeCodexAuthJSONWithIdentity(t, time.Now().Add(3*time.Hour), accountID, "", ""))

	savedWithoutEmail, err := m.Save(ToolCodex, "personal", sourceWithoutEmail)
	if err != nil {
		t.Fatalf("save without email: %v", err)
	}
	if savedWithoutEmail.Insight.AccountEmail != "person@company.com" {
		t.Fatalf("expected cached email fallback, got %+v", savedWithoutEmail.Insight)
	}
	if savedWithoutEmail.Insight.AccountPlan != "Plus" {
		t.Fatalf("expected cached plan fallback, got %+v", savedWithoutEmail.Insight)
	}

	target := filepath.Join(t.TempDir(), "target.json")
	usedWithoutEmail, err := m.Use(ToolCodex, "personal", target)
	if err != nil {
		t.Fatalf("use without email: %v", err)
	}
	if usedWithoutEmail.Insight.AccountEmail != "person@company.com" || usedWithoutEmail.Insight.AccountPlan != "Plus" {
		t.Fatalf("expected cached identity on use, got %+v", usedWithoutEmail.Insight)
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
	oldUnmarshalPIAuthJSON := unmarshalPIAuthJSON
	oldUserHomeDir := userHomeDir
	return func() {
		jsonMarshalIndent = oldJSONMarshalIndent
		unmarshalPIAuthJSON = oldUnmarshalPIAuthJSON
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

func TestMergePIAuthWithTarget(t *testing.T) {
	t.Run("invalid snapshot", func(t *testing.T) {
		if _, err := mergePIAuthWithTarget([]byte("not-json"), filepath.Join(t.TempDir(), "target.json")); err == nil {
			t.Fatalf("expected snapshot parse error")
		}
	})

	t.Run("target missing", func(t *testing.T) {
		snapshot := []byte(`{"openai-codex":{"access":"new"}}`)
		merged, err := mergePIAuthWithTarget(snapshot, filepath.Join(t.TempDir(), "missing.json"))
		if err != nil {
			t.Fatalf("target missing merge should succeed: %v", err)
		}
		if string(merged) != string(snapshot) {
			t.Fatalf("expected snapshot passthrough when target missing, got %q", merged)
		}
	})

	t.Run("target read error", func(t *testing.T) {
		targetDir := filepath.Join(t.TempDir(), "target")
		if err := os.MkdirAll(targetDir, 0o700); err != nil {
			t.Fatalf("mkdir target dir: %v", err)
		}
		if _, err := mergePIAuthWithTarget([]byte(`{"openai-codex":{"access":"new"}}`), targetDir); err == nil {
			t.Fatalf("expected target read error")
		}
	})

	t.Run("target invalid json", func(t *testing.T) {
		target := filepath.Join(t.TempDir(), "target.json")
		writeFile(t, target, []byte("not-json"))
		if _, err := mergePIAuthWithTarget([]byte(`{"openai-codex":{"access":"new"}}`), target); err == nil {
			t.Fatalf("expected target invalid json error")
		}
	})

	t.Run("merge preserves other providers", func(t *testing.T) {
		target := filepath.Join(t.TempDir(), "target.json")
		writeFile(t, target, []byte(`{"anthropic":{"access":"anthro-old"},"openai-codex":{"access":"codex-old"}}`))
		snapshot := []byte(`{"openai-codex":{"access":"codex-new"}}`)

		mergedRaw, err := mergePIAuthWithTarget(snapshot, target)
		if err != nil {
			t.Fatalf("mergePIAuthWithTarget: %v", err)
		}

		var merged map[string]any
		if err := json.Unmarshal(mergedRaw, &merged); err != nil {
			t.Fatalf("unmarshal merged json: %v", err)
		}

		anthropic := merged["anthropic"].(map[string]any)
		if anthropic["access"] != "anthro-old" {
			t.Fatalf("expected anthropic preserved, got %+v", anthropic)
		}
		openai := merged["openai-codex"].(map[string]any)
		if openai["access"] != "codex-new" {
			t.Fatalf("expected openai-codex updated, got %+v", openai)
		}
	})

	t.Run("merge serialize error", func(t *testing.T) {
		restore := restoreManagerSeams()
		defer restore()
		jsonMarshalIndent = func(any, string, string) ([]byte, error) { return nil, os.ErrInvalid }
		target := filepath.Join(t.TempDir(), "target.json")
		writeFile(t, target, []byte(`{"anthropic":{"access":"anthro-old"}}`))
		if _, err := mergePIAuthWithTarget([]byte(`{"openai-codex":{"access":"codex-new"}}`), target); err == nil {
			t.Fatalf("expected merge serialization error")
		}
	})
}

func TestFilterPIAuthProviders(t *testing.T) {
	t.Run("invalid json", func(t *testing.T) {
		if _, err := filterPIAuthProviders([]byte("not-json"), "codex"); err == nil {
			t.Fatalf("expected invalid JSON error")
		}
	})

	t.Run("missing provider", func(t *testing.T) {
		raw := []byte(`{"openai-codex":{"access":"c1"},"anthropic":{"access":"a1"}}`)
		if _, err := filterPIAuthProviders(raw, "missing"); err == nil {
			t.Fatalf("expected provider missing error")
		}
	})

	t.Run("codex alias", func(t *testing.T) {
		raw := []byte(`{"openai-codex":{"access":"c1"},"anthropic":{"access":"a1"}}`)
		filtered, err := filterPIAuthProviders(raw, "codex")
		if err != nil {
			t.Fatalf("filter codex: %v", err)
		}
		var obj map[string]any
		if err := json.Unmarshal(filtered, &obj); err != nil {
			t.Fatalf("unmarshal filtered: %v", err)
		}
		if len(obj) != 1 {
			t.Fatalf("expected single provider after codex filter, got %+v", obj)
		}
		if _, ok := obj["openai-codex"]; !ok {
			t.Fatalf("expected openai-codex key, got %+v", obj)
		}
	})

	t.Run("exact provider case-insensitive", func(t *testing.T) {
		raw := []byte(`{"openai-codex":{"access":"c1"},"anthropic":{"access":"a1"}}`)
		filtered, err := filterPIAuthProviders(raw, "ANTHROPIC")
		if err != nil {
			t.Fatalf("filter anthropic exact: %v", err)
		}
		var obj map[string]any
		if err := json.Unmarshal(filtered, &obj); err != nil {
			t.Fatalf("unmarshal filtered: %v", err)
		}
		if len(obj) != 1 {
			t.Fatalf("expected single provider after exact filter, got %+v", obj)
		}
		if _, ok := obj["anthropic"]; !ok {
			t.Fatalf("expected anthropic key, got %+v", obj)
		}
	})
}

func TestManagerSaveAndUseWithPIProvider(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	root := t.TempDir()
	m, err := NewManager(root)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	source := filepath.Join(t.TempDir(), "pi-source.json")
	writeFile(t, source, []byte(`{"openai-codex":{"access":"codex-work"},"anthropic":{"access":"anthro-work"}}`))

	if _, err := m.SaveWithPIProvider(ToolPi, "codex-only", source, "codex"); err != nil {
		t.Fatalf("save codex-only provider: %v", err)
	}

	state, err := m.loadState()
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	entry, ok := state.Entries[stateKey(ToolPi, "codex-only")]
	if !ok {
		t.Fatalf("expected codex-only entry in state")
	}
	snapshotRaw, err := os.ReadFile(entry.SnapshotPath)
	if err != nil {
		t.Fatalf("read snapshot: %v", err)
	}
	var savedSnapshot map[string]any
	if err := json.Unmarshal(snapshotRaw, &savedSnapshot); err != nil {
		t.Fatalf("unmarshal snapshot: %v", err)
	}
	if len(savedSnapshot) != 1 {
		t.Fatalf("expected single provider in saved snapshot, got %+v", savedSnapshot)
	}
	if _, ok := savedSnapshot["openai-codex"]; !ok {
		t.Fatalf("expected codex provider in saved snapshot, got %+v", savedSnapshot)
	}

	if _, err := m.Save(ToolPi, "full", source); err != nil {
		t.Fatalf("save full pi snapshot: %v", err)
	}

	target := filepath.Join(t.TempDir(), "pi-target.json")
	writeFile(t, target, []byte(`{"openai-codex":{"access":"codex-old"},"anthropic":{"access":"anthro-old"}}`))

	if _, err := m.UseWithPIProvider(ToolPi, "full", target, "anthropic"); err != nil {
		t.Fatalf("use full snapshot with anthropic selector: %v", err)
	}

	mergedRaw, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	var merged map[string]any
	if err := json.Unmarshal(mergedRaw, &merged); err != nil {
		t.Fatalf("unmarshal merged target: %v", err)
	}
	if merged["openai-codex"].(map[string]any)["access"] != "codex-old" {
		t.Fatalf("expected codex untouched by anthropic selector, got %+v", merged)
	}
	if merged["anthropic"].(map[string]any)["access"] != "anthro-work" {
		t.Fatalf("expected anthropic replaced by selector, got %+v", merged)
	}
}

func TestManagerUsePIMergesProviders(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	root := t.TempDir()
	m, err := NewManager(root)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	source := filepath.Join(t.TempDir(), "pi-source.json")
	writeFile(t, source, []byte(`{"openai-codex":{"access":"codex-work","expires":9999999999999}}`))
	if _, err := m.Save(ToolPi, "work", source); err != nil {
		t.Fatalf("save pi snapshot: %v", err)
	}

	target := filepath.Join(t.TempDir(), "pi-target.json")
	writeFile(t, target, []byte(`{"anthropic":{"access":"anthro-personal","expires":1111},"openai-codex":{"access":"codex-personal","expires":2222}}`))

	if _, err := m.Use(ToolPi, "work", target); err != nil {
		t.Fatalf("use pi merge: %v", err)
	}

	raw, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read merged target: %v", err)
	}
	var merged map[string]any
	if err := json.Unmarshal(raw, &merged); err != nil {
		t.Fatalf("unmarshal merged target: %v", err)
	}

	anthropic := merged["anthropic"].(map[string]any)
	if anthropic["access"] != "anthro-personal" {
		t.Fatalf("expected anthropic preserved, got %+v", anthropic)
	}
	openai := merged["openai-codex"].(map[string]any)
	if openai["access"] != "codex-work" {
		t.Fatalf("expected codex provider switched, got %+v", openai)
	}
}

func TestMergePIAuthWithTargetTargetParseErrorViaSeam(t *testing.T) {
	restore := restoreManagerSeams()
	defer restore()
	unmarshalPIAuthJSON = func([]byte, any) error { return os.ErrInvalid }

	target := filepath.Join(t.TempDir(), "target.json")
	writeFile(t, target, []byte(`{"anthropic":{"access":"anthro-old"}}`))
	if _, err := mergePIAuthWithTarget([]byte(`{"openai-codex":{"access":"codex-new"}}`), target); err == nil {
		t.Fatalf("expected target parse error from seam")
	}
}

func TestManagerUsePiMergeErrorPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	root := t.TempDir()
	m, err := NewManager(root)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	source := filepath.Join(t.TempDir(), "pi-source.json")
	writeFile(t, source, []byte(`{"openai-codex":{"access":"codex-work"}}`))
	if _, err := m.Save(ToolPi, "work", source); err != nil {
		t.Fatalf("save pi snapshot: %v", err)
	}

	target := filepath.Join(t.TempDir(), "pi-target.json")
	writeFile(t, target, []byte("not-json"))
	if _, err := m.Use(ToolPi, "work", target); err == nil {
		t.Fatalf("expected pi merge error for invalid target JSON")
	}
}

func TestPiProviderSubsetMatch(t *testing.T) {
	if piProviderSubsetMatch(map[string]any{}, map[string]any{"x": 1}) {
		t.Fatalf("empty snapshot should not match")
	}
	if !piProviderSubsetMatch(
		map[string]any{"openai-codex": map[string]any{"access": "a"}},
		map[string]any{"openai-codex": map[string]any{"access": "a"}, "anthropic": map[string]any{"access": "b"}},
	) {
		t.Fatalf("subset provider match expected")
	}
	if piProviderSubsetMatch(
		map[string]any{"openai-codex": map[string]any{"access": "a"}},
		map[string]any{"anthropic": map[string]any{"access": "b"}},
	) {
		t.Fatalf("missing provider should not match")
	}
	if piProviderSubsetMatch(
		map[string]any{"openai-codex": map[string]any{"access": "a"}},
		map[string]any{"openai-codex": map[string]any{"access": "b"}},
	) {
		t.Fatalf("different auth payload should not match")
	}
}

func TestManagerActive(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	root := t.TempDir()
	m, err := NewManager(root)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	items, err := m.Active(nil)
	if err != nil {
		t.Fatalf("Active no profiles: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 tools, got %+v", items)
	}

	codexSrc := filepath.Join(t.TempDir(), "codex.json")
	writeFile(t, codexSrc, makeCodexAuthJSON(t, time.Now().Add(time.Hour)))
	if _, err := m.Save(ToolCodex, "work", codexSrc); err != nil {
		t.Fatalf("save codex work: %v", err)
	}

	codexTarget := filepath.Join(home, ".codex", "auth.json")
	writeFile(t, codexTarget, makeCodexAuthJSON(t, time.Now().Add(2*time.Hour)))
	filtered := ToolCodex
	items, err = m.Active(&filtered)
	if err != nil {
		t.Fatalf("Active codex no match: %v", err)
	}
	if len(items) != 1 || items[0].Status != "no matching saved profile" {
		t.Fatalf("unexpected active no match result: %+v", items)
	}

	if _, err := m.Use(ToolCodex, "work", ""); err != nil {
		t.Fatalf("use codex work default target: %v", err)
	}
	items, err = m.Active(&filtered)
	if err != nil {
		t.Fatalf("Active codex match: %v", err)
	}
	if len(items) != 1 || items[0].Status != "match" || items[0].ActiveLabel != "work" {
		t.Fatalf("unexpected active match result: %+v", items)
	}

	if _, err := m.Save(ToolCodex, "work-clone", codexSrc); err != nil {
		t.Fatalf("save codex clone: %v", err)
	}
	items, err = m.Active(&filtered)
	if err != nil {
		t.Fatalf("Active codex ambiguous: %v", err)
	}
	if len(items) != 1 || items[0].Status != "ambiguous" {
		t.Fatalf("unexpected ambiguous result: %+v", items)
	}

	piSrc := filepath.Join(t.TempDir(), "pi.json")
	writeFile(t, piSrc, []byte(`{"openai-codex":{"access":"codex-work"}}`))
	if _, err := m.Save(ToolPi, "work", piSrc); err != nil {
		t.Fatalf("save pi work: %v", err)
	}
	piTarget := filepath.Join(home, ".pi", "agent", "auth.json")
	writeFile(t, piTarget, []byte(`{"openai-codex":{"access":"codex-work"},"anthropic":{"access":"anthro"}}`))
	piTool := ToolPi
	items, err = m.Active(&piTool)
	if err != nil {
		t.Fatalf("Active pi match: %v", err)
	}
	if len(items) != 1 || items[0].Status != "match" || items[0].ActiveLabel != "work" {
		t.Fatalf("unexpected pi active result: %+v", items)
	}

	if err := os.Remove(piTarget); err != nil {
		t.Fatalf("remove pi runtime: %v", err)
	}
	items, err = m.Active(&piTool)
	if err != nil {
		t.Fatalf("Active pi missing runtime: %v", err)
	}
	if len(items) != 1 || items[0].Status != "runtime auth file missing" {
		t.Fatalf("unexpected pi missing runtime result: %+v", items)
	}
}

func TestManagerActiveErrors(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	root := t.TempDir()
	m, err := NewManager(root)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	if err := os.MkdirAll(m.statePath(), 0o700); err != nil {
		t.Fatalf("mkdir state path: %v", err)
	}
	if _, err := m.Active(nil); err == nil {
		t.Fatalf("expected active loadState error")
	}

	root2 := t.TempDir()
	m2, err := NewManager(root2)
	if err != nil {
		t.Fatalf("NewManager root2: %v", err)
	}
	piSrc := filepath.Join(t.TempDir(), "pi.json")
	writeFile(t, piSrc, []byte(`{"openai-codex":{"access":"codex-work"}}`))
	if _, err := m2.Save(ToolPi, "work", piSrc); err != nil {
		t.Fatalf("save pi work: %v", err)
	}
	piTarget := filepath.Join(home, ".pi", "agent", "auth.json")
	writeFile(t, piTarget, []byte(`{"openai-codex":{"access":"codex-work"}`))
	piTool := ToolPi
	items, err := m2.Active(&piTool)
	if err != nil {
		t.Fatalf("Active with invalid runtime json should not hard fail: %v", err)
	}
	if len(items) != 1 || items[0].Status != "runtime auth JSON invalid" {
		t.Fatalf("unexpected active invalid runtime result: %+v", items)
	}

	codexSrc := filepath.Join(t.TempDir(), "codex.json")
	writeFile(t, codexSrc, makeCodexAuthJSON(t, time.Now().Add(time.Hour)))
	if _, err := m2.Save(ToolCodex, "work", codexSrc); err != nil {
		t.Fatalf("save codex work: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(home, ".codex", "auth.json"), 0o700); err != nil {
		t.Fatalf("mkdir codex runtime dir: %v", err)
	}
	codexTool := ToolCodex
	if _, err := m2.Active(&codexTool); err == nil {
		t.Fatalf("expected active runtime read error for codex")
	}
}

func TestManagerActivePiRuntimeParseErrorViaSeam(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	root := t.TempDir()
	m, err := NewManager(root)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	piSrc := filepath.Join(t.TempDir(), "pi.json")
	writeFile(t, piSrc, []byte(`{"openai-codex":{"access":"codex-work"}}`))
	if _, err := m.Save(ToolPi, "work", piSrc); err != nil {
		t.Fatalf("save pi work: %v", err)
	}
	piTarget := filepath.Join(home, ".pi", "agent", "auth.json")
	writeFile(t, piTarget, []byte(`{"openai-codex":{"access":"codex-work"},"runtime-only":true}`))

	restore := restoreManagerSeams()
	defer restore()
	unmarshalPIAuthJSON = func([]byte, any) error { return os.ErrInvalid }

	piTool := ToolPi
	if _, err := m.Active(&piTool); err == nil || !strings.Contains(err.Error(), "parsing runtime pi auth JSON") {
		t.Fatalf("expected runtime parse error, got %v", err)
	}
}

func TestManagerActivePiSnapshotScanBranches(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	root := t.TempDir()
	m, err := NewManager(root)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	goodSnap := filepath.Join(t.TempDir(), "good.json")
	writeFile(t, goodSnap, []byte(`{"openai-codex":{"access":"codex-work"}}`))
	if _, err := m.Save(ToolPi, "good", goodSnap); err != nil {
		t.Fatalf("save pi good: %v", err)
	}

	state, err := m.loadState()
	if err != nil {
		t.Fatalf("loadState: %v", err)
	}

	dirSnap := filepath.Join(t.TempDir(), "snap-dir")
	if err := os.MkdirAll(dirSnap, 0o700); err != nil {
		t.Fatalf("mkdir dir snapshot: %v", err)
	}
	state.Entries[stateKey(ToolPi, "dir")] = StateEntry{
		Tool:         ToolPi.String(),
		Label:        "dir",
		SnapshotPath: dirSnap,
		SavedAt:      nowISO(),
	}

	invalidSnap := filepath.Join(t.TempDir(), "invalid.json")
	writeFile(t, invalidSnap, []byte("not-json"))
	state.Entries[stateKey(ToolPi, "invalid")] = StateEntry{
		Tool:         ToolPi.String(),
		Label:        "invalid",
		SnapshotPath: invalidSnap,
		SavedAt:      nowISO(),
	}

	seamSnap := filepath.Join(t.TempDir(), "seam.json")
	writeFile(t, seamSnap, []byte(`{"openai-codex":{"access":"other"}}`))
	state.Entries[stateKey(ToolPi, "seam")] = StateEntry{
		Tool:         ToolPi.String(),
		Label:        "seam",
		SnapshotPath: seamSnap,
		SavedAt:      nowISO(),
	}

	if err := m.saveState(state); err != nil {
		t.Fatalf("saveState: %v", err)
	}

	piTarget := filepath.Join(home, ".pi", "agent", "auth.json")
	writeFile(t, piTarget, []byte(`{"openai-codex":{"access":"codex-work"},"runtime-only":true}`))

	restore := restoreManagerSeams()
	defer restore()
	unmarshalPIAuthJSON = func(raw []byte, v any) error {
		if strings.Contains(string(raw), "\"runtime-only\":true") {
			return json.Unmarshal(raw, v)
		}
		return os.ErrInvalid
	}

	piTool := ToolPi
	items, err := m.Active(&piTool)
	if err != nil {
		t.Fatalf("Active pi with snapshot scan branches: %v", err)
	}
	if len(items) != 1 || items[0].Status != "no matching saved profile" {
		t.Fatalf("unexpected active result: %+v", items)
	}
}

func TestManagerRejectsInvalidToolAndLabel(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	m, err := NewManager(t.TempDir())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	invalidTool := Tool("claude")
	if _, err := m.Save(invalidTool, "work", ""); err == nil || !strings.Contains(err.Error(), "invalid tool") {
		t.Fatalf("expected invalid tool error from Save, got %v", err)
	}
	if _, err := m.Use(invalidTool, "work", filepath.Join(t.TempDir(), "target.json")); err == nil || !strings.Contains(err.Error(), "invalid tool") {
		t.Fatalf("expected invalid tool error from Use, got %v", err)
	}
	if _, err := m.Delete(invalidTool, "work"); err == nil || !strings.Contains(err.Error(), "invalid tool") {
		t.Fatalf("expected invalid tool error from Delete, got %v", err)
	}

	invalidFilter := Tool("not-a-tool")
	if _, err := m.List(&invalidFilter); err == nil || !strings.Contains(err.Error(), "invalid tool") {
		t.Fatalf("expected invalid tool error from List, got %v", err)
	}
	if _, err := m.Active(&invalidFilter); err == nil || !strings.Contains(err.Error(), "invalid tool") {
		t.Fatalf("expected invalid tool error from Active, got %v", err)
	}

	if _, err := m.Save(ToolCodex, "bad label", ""); err == nil || !strings.Contains(err.Error(), "label must match") {
		t.Fatalf("expected invalid label format error from Save, got %v", err)
	}
	if _, err := m.Use(ToolCodex, " ", filepath.Join(t.TempDir(), "target.json")); err == nil || !strings.Contains(err.Error(), "label is required") {
		t.Fatalf("expected required label error from Use, got %v", err)
	}
	if _, err := m.Delete(ToolCodex, "../traversal"); err == nil || !strings.Contains(err.Error(), "label must match") {
		t.Fatalf("expected invalid label format error from Delete, got %v", err)
	}
}

func TestManagerUseRollsBackTargetWhenStateSaveFails(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	root := t.TempDir()
	m, err := NewManager(root)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	source := filepath.Join(t.TempDir(), "source.json")
	writeFile(t, source, makeCodexAuthJSON(t, time.Now().Add(2*time.Hour)))
	if _, err := m.Save(ToolCodex, "work", source); err != nil {
		t.Fatalf("save setup: %v", err)
	}

	t.Run("restores previous file content", func(t *testing.T) {
		restore := restoreManagerSeams()
		defer restore()
		jsonMarshalIndent = func(any, string, string) ([]byte, error) { return nil, os.ErrInvalid }

		target := filepath.Join(t.TempDir(), "target.json")
		originalRaw := []byte(`{"tokens":{"access_token":"old"}}`)
		writeFile(t, target, originalRaw)

		if _, err := m.Use(ToolCodex, "work", target); err == nil || !strings.Contains(err.Error(), "target rolled back") {
			t.Fatalf("expected use saveState rollback error, got %v", err)
		}

		got, err := os.ReadFile(target)
		if err != nil {
			t.Fatalf("read rolled back target: %v", err)
		}
		if string(got) != string(originalRaw) {
			t.Fatalf("expected target rollback to original content, got %q", string(got))
		}
	})

	t.Run("removes newly created file", func(t *testing.T) {
		restore := restoreManagerSeams()
		defer restore()
		jsonMarshalIndent = func(any, string, string) ([]byte, error) { return nil, os.ErrInvalid }

		target := filepath.Join(t.TempDir(), "new-target.json")
		if _, err := m.Use(ToolCodex, "work", target); err == nil || !strings.Contains(err.Error(), "target rolled back") {
			t.Fatalf("expected use saveState rollback error, got %v", err)
		}

		if _, err := os.Stat(target); !os.IsNotExist(err) {
			t.Fatalf("expected new target file to be removed after rollback, got err=%v", err)
		}
	})
}
