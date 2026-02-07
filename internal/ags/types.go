package ags

import "time"

type Tool string

const (
	ToolCodex  Tool = "codex"
	ToolClaude Tool = "claude"
	ToolPi     Tool = "pi"
)

func (t Tool) String() string {
	return string(t)
}

func ParseTool(value string) (Tool, bool) {
	switch Tool(value) {
	case ToolCodex, ToolClaude, ToolPi:
		return Tool(value), true
	default:
		return "", false
	}
}

type AuthInsight struct {
	Status       string
	ExpiresAt    string
	LastRefresh  string
	NeedsRefresh string
	Details      []string
}

type SaveResult struct {
	Tool                 Tool
	Label                string
	SourcePath           string
	SnapshotPath         string
	ChangedSinceLastSave bool
	Insight              AuthInsight
}

type UseResult struct {
	Tool                 Tool
	Label                string
	TargetPath           string
	ChangeSinceLastUse   string
	Insight              AuthInsight
}

type ListItem struct {
	Tool        Tool
	Label       string
	SavedAt     string
	LastUsedAt  string
	Snapshot    string
	AuthInsight AuthInsight
}

type State struct {
	Version int                    `json:"version"`
	Entries map[string]StateEntry  `json:"entries"`
}

type StateEntry struct {
	Tool          string `json:"tool"`
	Label         string `json:"label"`
	SourcePath    string `json:"source_path"`
	SnapshotPath  string `json:"snapshot_path"`
	SHA256        string `json:"sha256"`
	SavedAt       string `json:"saved_at"`
	LastUsedAt    string `json:"last_used_at,omitempty"`
	LastUsedSHA   string `json:"last_used_sha256,omitempty"`
}

type Manager struct {
	rootDir string
	paths   map[Tool]ToolPaths
}

type ToolPaths struct {
	DefaultRuntime string
	SaveCandidates []string
}

func defaultState() State {
	return State{
		Version: 1,
		Entries: map[string]StateEntry{},
	}
}

func nowUTC() time.Time {
	return time.Now().UTC()
}

func nowISO() string {
	return nowUTC().Format(time.RFC3339)
}
