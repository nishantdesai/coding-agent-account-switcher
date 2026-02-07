package ags

import "time"

type Tool string

const (
	ToolCodex Tool = "codex"
	ToolPi    Tool = "pi"
)

func (t Tool) String() string {
	return string(t)
}

func ParseTool(value string) (Tool, bool) {
	switch Tool(value) {
	case ToolCodex, ToolPi:
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
	AccountEmail string
	AccountPlan  string
	AccountID    string
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
	Tool               Tool
	Label              string
	TargetPath         string
	ChangeSinceLastUse string
	Insight            AuthInsight
}

type DeleteResult struct {
	Tool            Tool
	Label           string
	SnapshotPath    string
	SnapshotDeleted bool
}

type ListItem struct {
	Tool        Tool
	Label       string
	SavedAt     string
	LastUsedAt  string
	Snapshot    string
	AuthInsight AuthInsight
}

type ActiveItem struct {
	Tool        Tool
	ActiveLabel string
	Status      string
	RuntimePath string
	Details     []string
}

type State struct {
	Version       int                          `json:"version"`
	Entries       map[string]StateEntry        `json:"entries"`
	IdentityCache map[string]IdentityCacheItem `json:"identity_cache,omitempty"`
}

type StateEntry struct {
	Tool         string `json:"tool"`
	Label        string `json:"label"`
	SourcePath   string `json:"source_path"`
	SnapshotPath string `json:"snapshot_path"`
	SHA256       string `json:"sha256"`
	SavedAt      string `json:"saved_at"`
	LastUsedAt   string `json:"last_used_at,omitempty"`
	LastUsedSHA  string `json:"last_used_sha256,omitempty"`
}

type IdentityCacheItem struct {
	Email     string `json:"email"`
	Plan      string `json:"plan,omitempty"`
	UpdatedAt string `json:"updated_at"`
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
		Version:       1,
		Entries:       map[string]StateEntry{},
		IdentityCache: map[string]IdentityCacheItem{},
	}
}

func nowUTC() time.Time {
	return time.Now().UTC()
}

func nowISO() string {
	return nowUTC().Format(time.RFC3339)
}
