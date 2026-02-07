package ags

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
		ToolClaude: {
			DefaultRuntime: filepath.Join(home, ".claude.json"),
			SaveCandidates: []string{
				filepath.Join(home, ".claude.json"),
				filepath.Join(home, ".claude", "auth.json"),
				filepath.Join(home, ".config", "claude", "auth.json"),
				filepath.Join(home, ".claude.json.backup"),
			},
		},
	}

	return &Manager{
		rootDir: rootExpanded,
		paths:   paths,
	}, nil
}

func (m *Manager) Save(tool Tool, label string, sourceOverride string) (*SaveResult, error) {
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

	insight := inspectAuth(tool, raw)
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

	target := targetOverride
	if strings.TrimSpace(target) == "" {
		target = m.paths[tool].DefaultRuntime
	}
	target, err = expandPath(target)
	if err != nil {
		return nil, err
	}

	rawToWrite := snapshotRaw
	if tool == ToolPi {
		rawToWrite, err = mergePIAuthWithTarget(snapshotRaw, target)
		if err != nil {
			return nil, fmt.Errorf("merging pi auth file: %w", err)
		}
	}

	if err := atomicWriteFile(target, rawToWrite, 0o600); err != nil {
		return nil, fmt.Errorf("writing target auth file: %w", err)
	}

	hash := sha256Hex(snapshotRaw)
	changeSignal := "first use"
	if entry.LastUsedSHA != "" {
		if entry.LastUsedSHA == hash {
			changeSignal = "unchanged since last use"
		} else {
			changeSignal = "changed since last use (likely refreshed)"
		}
	}

	entry.LastUsedAt = nowISO()
	entry.LastUsedSHA = hash
	state.Entries[key] = entry
	if err := m.saveState(state); err != nil {
		return nil, err
	}

	return &UseResult{
		Tool:               tool,
		Label:              label,
		TargetPath:         target,
		ChangeSinceLastUse: changeSignal,
		Insight:            inspectAuth(tool, rawToWrite),
	}, nil
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
