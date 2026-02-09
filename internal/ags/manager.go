package ags

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
)

var (
	jsonMarshalIndent   = json.MarshalIndent
	unmarshalPIAuthJSON = json.Unmarshal
)

func NewManager(rootDir string) (*Manager, error) {
	rootExpanded, err := expandPath(rootDir)
	if err != nil {
		return nil, err
	}

	home, err := userHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolving home directory: %w", err)
	}

	paths := map[Tool]ToolPaths{
		ToolCodex: {
			DefaultRuntime: filepath.Join(home, ".codex", "auth.json"),
			SaveCandidates: []string{
				filepath.Join(home, ".codex", "auth.json"),
			},
		},
		ToolPi: {
			DefaultRuntime: filepath.Join(home, ".pi", "agent", "auth.json"),
			SaveCandidates: []string{
				filepath.Join(home, ".pi", "agent", "auth.json"),
			},
		},
	}

	return &Manager{
		rootDir: rootExpanded,
		paths:   paths,
	}, nil
}

func (m *Manager) Save(tool Tool, label string, sourceOverride string) (*SaveResult, error) {
	return m.save(tool, label, sourceOverride, "")
}

func (m *Manager) SaveWithPIProvider(tool Tool, label string, sourceOverride string, provider string) (*SaveResult, error) {
	return m.save(tool, label, sourceOverride, provider)
}

func (m *Manager) save(tool Tool, label string, sourceOverride string, piProvider string) (*SaveResult, error) {
	if err := validateManagerToolAndLabel(tool, label); err != nil {
		return nil, err
	}

	sourcePath, err := m.resolveSourcePath(tool, sourceOverride)
	if err != nil {
		return nil, err
	}

	raw, err := os.ReadFile(sourcePath)
	if err != nil {
		return nil, fmt.Errorf("reading source auth file: %w", err)
	}
	if err := validateJSONObject(raw); err != nil {
		return nil, fmt.Errorf("source is not valid JSON object: %w", err)
	}
	if tool == ToolPi && strings.TrimSpace(piProvider) != "" {
		raw, err = filterPIAuthProviders(raw, piProvider)
		if err != nil {
			return nil, err
		}
	}

	snapshotPath := m.snapshotPath(tool, label)
	if err := atomicWriteFile(snapshotPath, raw, 0o600); err != nil {
		return nil, fmt.Errorf("writing snapshot: %w", err)
	}

	hash := sha256Hex(raw)
	state, err := m.loadState()
	if err != nil {
		return nil, err
	}
	key := stateKey(tool, label)
	prev, hadPrev := state.Entries[key]
	changed := !hadPrev || prev.SHA256 != hash

	insight := inspectAuth(tool, raw)
	hydrateIdentityFromCache(&insight, state)
	rememberIdentity(&state, insight)

	state.Entries[key] = StateEntry{
		Tool:         tool.String(),
		Label:        label,
		SourcePath:   sourcePath,
		SnapshotPath: snapshotPath,
		SHA256:       hash,
		SavedAt:      nowISO(),
		LastUsedAt:   prev.LastUsedAt,
		LastUsedSHA:  prev.LastUsedSHA,
	}

	if err := m.saveState(state); err != nil {
		return nil, err
	}

	return &SaveResult{
		Tool:                 tool,
		Label:                label,
		SourcePath:           sourcePath,
		SnapshotPath:         snapshotPath,
		ChangedSinceLastSave: changed,
		Insight:              insight,
	}, nil
}

func (m *Manager) Use(tool Tool, label string, targetOverride string) (*UseResult, error) {
	return m.use(tool, label, targetOverride, "")
}

func (m *Manager) UseWithPIProvider(tool Tool, label string, targetOverride string, provider string) (*UseResult, error) {
	return m.use(tool, label, targetOverride, provider)
}

func (m *Manager) use(tool Tool, label string, targetOverride string, piProvider string) (*UseResult, error) {
	if err := validateManagerToolAndLabel(tool, label); err != nil {
		return nil, err
	}

	state, err := m.loadState()
	if err != nil {
		return nil, err
	}

	key := stateKey(tool, label)
	entry, ok := state.Entries[key]
	if !ok {
		return nil, fmt.Errorf("no saved profile for %s label=%q; run `ags save %s --label %s` first", tool, label, tool, label)
	}

	snapshotRaw, err := os.ReadFile(entry.SnapshotPath)
	if err != nil {
		return nil, fmt.Errorf("reading snapshot file: %w", err)
	}
	if err := validateJSONObject(snapshotRaw); err != nil {
		return nil, fmt.Errorf("snapshot JSON invalid: %w", err)
	}
	snapshotToApply := snapshotRaw
	if tool == ToolPi && strings.TrimSpace(piProvider) != "" {
		snapshotToApply, err = filterPIAuthProviders(snapshotRaw, piProvider)
		if err != nil {
			return nil, err
		}
	}

	target := targetOverride
	if strings.TrimSpace(target) == "" {
		target = m.paths[tool].DefaultRuntime
	}
	target, err = expandPath(target)
	if err != nil {
		return nil, err
	}
	previousTargetRaw, hadPreviousTarget, err := readOptionalFile(target)
	if err != nil {
		return nil, fmt.Errorf("reading existing target auth file: %w", err)
	}

	rawToWrite := snapshotToApply
	if tool == ToolPi {
		rawToWrite, err = mergePIAuthWithTarget(snapshotToApply, target)
		if err != nil {
			return nil, fmt.Errorf("merging pi auth file: %w", err)
		}
	}

	if err := atomicWriteFile(target, rawToWrite, 0o600); err != nil {
		return nil, fmt.Errorf("writing target auth file: %w", err)
	}

	hash := sha256Hex(snapshotToApply)
	changeSignal := "first use"
	if entry.LastUsedSHA != "" {
		if entry.LastUsedSHA == hash {
			changeSignal = "unchanged since last use"
		} else {
			changeSignal = "changed since last use (likely refreshed)"
		}
	}

	insight := inspectAuth(tool, snapshotToApply)
	hydrateIdentityFromCache(&insight, state)
	rememberIdentity(&state, insight)

	entry.LastUsedAt = nowISO()
	entry.LastUsedSHA = hash
	state.Entries[key] = entry
	if err := m.saveState(state); err != nil {
		rollbackErr := rollbackUseTargetWrite(target, previousTargetRaw, hadPreviousTarget)
		if rollbackErr != nil {
			return nil, fmt.Errorf("saving state after writing target: %w (rollback failed: %v)", err, rollbackErr)
		}
		return nil, fmt.Errorf("saving state after writing target: %w (target rolled back)", err)
	}

	return &UseResult{
		Tool:               tool,
		Label:              label,
		TargetPath:         target,
		ChangeSinceLastUse: changeSignal,
		Insight:            insight,
	}, nil
}

func filterPIAuthProviders(raw []byte, selector string) ([]byte, error) {
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("pi auth JSON invalid: %w", err)
	}
	keys, err := resolvePIProviderKeys(payload, selector)
	if err != nil {
		return nil, err
	}

	filtered := make(map[string]any, len(keys))
	for _, key := range keys {
		filtered[key] = payload[key]
	}

	out, err := jsonMarshalIndent(filtered, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("serializing filtered pi auth: %w", err)
	}
	out = append(out, '\n')
	return out, nil
}

func resolvePIProviderKeys(payload map[string]any, selector string) ([]string, error) {
	selector = strings.TrimSpace(strings.ToLower(selector))
	if selector == "" {
		return nil, errors.New("pi provider selector is required")
	}

	matches := []string{}
	switch selector {
	case "codex":
		for key := range payload {
			lower := strings.ToLower(key)
			if strings.Contains(lower, "codex") || strings.Contains(lower, "openai") {
				matches = append(matches, key)
			}
		}
	case "anthropic":
		for key := range payload {
			lower := strings.ToLower(key)
			if strings.Contains(lower, "anthropic") {
				matches = append(matches, key)
			}
		}
	default:
		for key := range payload {
			if key == selector || strings.EqualFold(key, selector) {
				matches = append(matches, key)
			}
		}
	}
	sort.Strings(matches)
	if len(matches) > 0 {
		return matches, nil
	}

	available := make([]string, 0, len(payload))
	for key := range payload {
		available = append(available, key)
	}
	sort.Strings(available)
	return nil, fmt.Errorf("pi provider %q not found in source/snapshot. available providers: %s", selector, strings.Join(available, ", "))
}

func mergePIAuthWithTarget(snapshotRaw []byte, targetPath string) ([]byte, error) {
	var snapshot map[string]any
	if err := json.Unmarshal(snapshotRaw, &snapshot); err != nil {
		return nil, fmt.Errorf("snapshot JSON invalid: %w", err)
	}

	targetRaw, err := os.ReadFile(targetPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return snapshotRaw, nil
		}
		return nil, fmt.Errorf("reading target auth file: %w", err)
	}
	if err := validateJSONObject(targetRaw); err != nil {
		return nil, fmt.Errorf("target auth JSON invalid: %w", err)
	}

	var target map[string]any
	if err := unmarshalPIAuthJSON(targetRaw, &target); err != nil {
		return nil, fmt.Errorf("parsing target auth JSON: %w", err)
	}

	for provider, auth := range snapshot {
		target[provider] = auth
	}

	merged, err := jsonMarshalIndent(target, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("serializing merged pi auth: %w", err)
	}
	merged = append(merged, '\n')
	return merged, nil
}

func (m *Manager) Delete(tool Tool, label string) (*DeleteResult, error) {
	if err := validateManagerToolAndLabel(tool, label); err != nil {
		return nil, err
	}

	state, err := m.loadState()
	if err != nil {
		return nil, err
	}

	key := stateKey(tool, label)
	entry, ok := state.Entries[key]
	if !ok {
		return nil, fmt.Errorf("no saved snapshot for %s label=%q", tool, label)
	}

	snapshotDeleted := false
	if err := os.Remove(entry.SnapshotPath); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("deleting snapshot file: %w", err)
		}
	} else {
		snapshotDeleted = true
	}

	delete(state.Entries, key)
	if err := m.saveState(state); err != nil {
		return nil, err
	}

	return &DeleteResult{
		Tool:            tool,
		Label:           label,
		SnapshotPath:    entry.SnapshotPath,
		SnapshotDeleted: snapshotDeleted,
	}, nil
}

func (m *Manager) List(toolFilter *Tool) ([]ListItem, error) {
	if toolFilter != nil {
		if err := validateManagerTool(*toolFilter); err != nil {
			return nil, err
		}
	}

	state, err := m.loadState()
	if err != nil {
		return nil, err
	}

	items := make([]ListItem, 0, len(state.Entries))
	for _, entry := range state.Entries {
		tool, ok := ParseTool(entry.Tool)
		if !ok {
			continue
		}
		if toolFilter != nil && *toolFilter != tool {
			continue
		}

		raw, err := os.ReadFile(entry.SnapshotPath)
		insight := AuthInsight{
			Status:       "unknown",
			NeedsRefresh: "unknown",
			Details:      []string{"snapshot missing or unreadable"},
		}
		if err == nil {
			insight = inspectAuth(tool, raw)
		}

		items = append(items, ListItem{
			Tool:        tool,
			Label:       entry.Label,
			SavedAt:     entry.SavedAt,
			LastUsedAt:  entry.LastUsedAt,
			Snapshot:    entry.SnapshotPath,
			AuthInsight: insight,
		})
	}

	sort.Slice(items, func(i, j int) bool {
		if items[i].Tool == items[j].Tool {
			return items[i].Label < items[j].Label
		}
		return items[i].Tool < items[j].Tool
	})

	return items, nil
}

func (m *Manager) Active(toolFilter *Tool) ([]ActiveItem, error) {
	if toolFilter != nil {
		if err := validateManagerTool(*toolFilter); err != nil {
			return nil, err
		}
	}

	state, err := m.loadState()
	if err != nil {
		return nil, err
	}

	tools := []Tool{ToolCodex, ToolPi}
	if toolFilter != nil {
		tools = []Tool{*toolFilter}
	}

	items := make([]ActiveItem, 0, len(tools))
	for _, tool := range tools {
		runtimePath := m.paths[tool].DefaultRuntime
		toolEntries := make([]StateEntry, 0)
		for _, entry := range state.Entries {
			parsedTool, ok := ParseTool(entry.Tool)
			if ok && parsedTool == tool {
				toolEntries = append(toolEntries, entry)
			}
		}

		if len(toolEntries) == 0 {
			items = append(items, ActiveItem{
				Tool:        tool,
				Status:      "no saved profiles",
				RuntimePath: runtimePath,
			})
			continue
		}

		runtimeRaw, err := os.ReadFile(runtimePath)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				items = append(items, ActiveItem{
					Tool:        tool,
					Status:      "runtime auth file missing",
					RuntimePath: runtimePath,
				})
				continue
			}
			return nil, fmt.Errorf("reading runtime auth file for %s: %w", tool, err)
		}
		if err := validateJSONObject(runtimeRaw); err != nil {
			items = append(items, ActiveItem{
				Tool:        tool,
				Status:      "runtime auth JSON invalid",
				RuntimePath: runtimePath,
			})
			continue
		}

		matchedLabels := make([]string, 0)
		switch tool {
		case ToolPi:
			var runtimeObj map[string]any
			if err := unmarshalPIAuthJSON(runtimeRaw, &runtimeObj); err != nil {
				return nil, fmt.Errorf("parsing runtime pi auth JSON: %w", err)
			}
			for _, entry := range toolEntries {
				snapshotRaw, err := os.ReadFile(entry.SnapshotPath)
				if err != nil {
					continue
				}
				if err := validateJSONObject(snapshotRaw); err != nil {
					continue
				}
				var snapshotObj map[string]any
				if err := unmarshalPIAuthJSON(snapshotRaw, &snapshotObj); err != nil {
					continue
				}
				if piProviderSubsetMatch(snapshotObj, runtimeObj) {
					matchedLabels = append(matchedLabels, entry.Label)
				}
			}
		default:
			runtimeHash := sha256Hex(runtimeRaw)
			for _, entry := range toolEntries {
				if entry.SHA256 == runtimeHash {
					matchedLabels = append(matchedLabels, entry.Label)
				}
			}
		}

		sort.Strings(matchedLabels)
		switch len(matchedLabels) {
		case 0:
			items = append(items, ActiveItem{
				Tool:        tool,
				Status:      "no matching saved profile",
				RuntimePath: runtimePath,
			})
		case 1:
			items = append(items, ActiveItem{
				Tool:        tool,
				ActiveLabel: matchedLabels[0],
				Status:      "match",
				RuntimePath: runtimePath,
			})
		default:
			items = append(items, ActiveItem{
				Tool:        tool,
				ActiveLabel: strings.Join(matchedLabels, ","),
				Status:      "ambiguous",
				RuntimePath: runtimePath,
				Details:     []string{"multiple saved labels match current runtime auth"},
			})
		}
	}

	return items, nil
}

func piProviderSubsetMatch(snapshotObj map[string]any, runtimeObj map[string]any) bool {
	if len(snapshotObj) == 0 {
		return false
	}
	for provider, snapshotAuth := range snapshotObj {
		runtimeAuth, ok := runtimeObj[provider]
		if !ok {
			return false
		}
		if !reflect.DeepEqual(snapshotAuth, runtimeAuth) {
			return false
		}
	}
	return true
}

func hydrateIdentityFromCache(insight *AuthInsight, state State) {
	if insight == nil {
		return
	}

	insight.AccountID = strings.TrimSpace(insight.AccountID)
	insight.AccountEmail = strings.TrimSpace(insight.AccountEmail)
	insight.AccountPlan = strings.TrimSpace(insight.AccountPlan)

	if insight.AccountID == "" {
		return
	}
	if insight.AccountEmail != "" {
		return
	}

	cacheItem, ok := state.IdentityCache[insight.AccountID]
	if !ok {
		return
	}
	if strings.TrimSpace(cacheItem.Email) != "" {
		insight.AccountEmail = strings.TrimSpace(cacheItem.Email)
	}
	if insight.AccountPlan == "" && strings.TrimSpace(cacheItem.Plan) != "" {
		insight.AccountPlan = strings.TrimSpace(cacheItem.Plan)
	}
}

func rememberIdentity(state *State, insight AuthInsight) {
	if state == nil {
		return
	}

	accountID := strings.TrimSpace(insight.AccountID)
	email := strings.TrimSpace(insight.AccountEmail)
	plan := strings.TrimSpace(insight.AccountPlan)
	if accountID == "" || email == "" {
		return
	}

	if state.IdentityCache == nil {
		state.IdentityCache = map[string]IdentityCacheItem{}
	}

	state.IdentityCache[accountID] = IdentityCacheItem{
		Email:     email,
		Plan:      plan,
		UpdatedAt: nowISO(),
	}
}

func validateManagerToolAndLabel(tool Tool, label string) error {
	if err := validateManagerTool(tool); err != nil {
		return err
	}
	return validateManagerLabel(label)
}

func validateManagerTool(tool Tool) error {
	if _, ok := ParseTool(tool.String()); !ok {
		return fmt.Errorf("invalid tool %q. expected one of: codex, pi", tool)
	}
	return nil
}

func validateManagerLabel(label string) error {
	label = strings.TrimSpace(label)
	if label == "" {
		return errors.New("label is required")
	}
	if !labelPattern.MatchString(label) {
		return errors.New("label must match [a-zA-Z0-9._-]+")
	}
	return nil
}

func readOptionalFile(path string) ([]byte, bool, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return raw, true, nil
}

func rollbackUseTargetWrite(target string, previousRaw []byte, hadPrevious bool) error {
	if hadPrevious {
		if err := atomicWriteFile(target, previousRaw, 0o600); err != nil {
			return fmt.Errorf("restoring previous target content: %w", err)
		}
		return nil
	}
	if err := os.Remove(target); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("removing new target file: %w", err)
	}
	return nil
}

func (m *Manager) resolveSourcePath(tool Tool, sourceOverride string) (string, error) {
	if strings.TrimSpace(sourceOverride) != "" {
		p, err := expandPath(sourceOverride)
		if err != nil {
			return "", err
		}
		if _, err := os.Stat(p); err != nil {
			return "", fmt.Errorf("source path does not exist: %s", p)
		}
		return p, nil
	}

	candidates := m.paths[tool].SaveCandidates
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("could not find %s auth file. tried: %s. pass --source <path>", tool, strings.Join(candidates, ", "))
}

func (m *Manager) snapshotPath(tool Tool, label string) string {
	return filepath.Join(m.rootDir, "snapshots", tool.String(), label+".json")
}

func (m *Manager) statePath() string {
	return filepath.Join(m.rootDir, "state.json")
}

func (m *Manager) loadState() (State, error) {
	path := m.statePath()
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return defaultState(), nil
		}
		return State{}, fmt.Errorf("reading state: %w", err)
	}

	var state State
	if err := json.Unmarshal(raw, &state); err != nil {
		return State{}, fmt.Errorf("parsing state: %w", err)
	}
	if state.Entries == nil {
		state.Entries = map[string]StateEntry{}
	}
	if state.IdentityCache == nil {
		state.IdentityCache = map[string]IdentityCacheItem{}
	}
	if state.Version == 0 {
		state.Version = 1
	}
	return state, nil
}

func (m *Manager) saveState(state State) error {
	raw, err := jsonMarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("serializing state: %w", err)
	}
	raw = append(raw, '\n')
	return atomicWriteFile(m.statePath(), raw, 0o600)
}

func stateKey(tool Tool, label string) string {
	return tool.String() + ":" + label
}

func sha256Hex(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
