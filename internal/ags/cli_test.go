package ags

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestRunNoArgsAndUnknownCommand(t *testing.T) {
	var out bytes.Buffer
	if err := Run(nil, &out, &out); err != nil {
		t.Fatalf("Run no args: %v", err)
	}
	if !strings.Contains(out.String(), "USAGE:") {
		t.Fatalf("expected root help output, got %q", out.String())
	}

	err := Run([]string{"unknown"}, &out, &out)
	if err == nil || !strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("expected unknown command error, got %v", err)
	}
}

func TestRunHelpTopics(t *testing.T) {
	topics := []string{"save", "use", "delete", "list"}
	for _, topic := range topics {
		var out bytes.Buffer
		if err := Run([]string{"help", topic}, &out, &out); err != nil {
			t.Fatalf("help %s: %v", topic, err)
		}
		if !strings.Contains(out.String(), "USAGE:") {
			t.Fatalf("expected usage text for %s, got %q", topic, out.String())
		}
	}

	var out bytes.Buffer
	if err := Run([]string{"help", "wat"}, &out, &out); err == nil {
		t.Fatalf("expected unknown help topic error")
	}
}

func TestSubcommandHelpFlags(t *testing.T) {
	cases := [][]string{
		{"save", "--help"},
		{"use", "--help"},
		{"delete", "--help"},
		{"list", "--help"},
		{"save", "-h"},
	}
	for _, args := range cases {
		var out bytes.Buffer
		if err := Run(args, &out, &out); err != nil {
			t.Fatalf("Run %v: %v", args, err)
		}
		if !strings.Contains(out.String(), "USAGE:") {
			t.Fatalf("expected command help for %v, got %q", args, out.String())
		}
	}
}

func TestCLIEndToEndSaveUseListDelete(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	root := t.TempDir()
	source := root + "/source.json"
	target := root + "/target.json"

	writeFile(t, source, makeCodexAuthJSON(t, time.Now().Add(2*time.Hour)))

	var out bytes.Buffer
	if err := Run([]string{"save", "codex", "work", "--source", source, "--root", root}, &out, &out); err != nil {
		t.Fatalf("save: %v", err)
	}
	if !strings.Contains(out.String(), "Saved codex for work") {
		t.Fatalf("unexpected save output: %q", out.String())
	}

	out.Reset()
	if err := Run([]string{"use", "codex", "work", "--target", target, "--root", root}, &out, &out); err != nil {
		t.Fatalf("use: %v", err)
	}
	if !strings.Contains(out.String(), "Using codex for work") {
		t.Fatalf("unexpected use output: %q", out.String())
	}

	out.Reset()
	if err := Run([]string{"list", "codex", "--verbose", "--root", root}, &out, &out); err != nil {
		t.Fatalf("list verbose: %v", err)
	}
	if !strings.Contains(out.String(), "needs refresh") || !strings.Contains(out.String(), "expires raw=") {
		t.Fatalf("unexpected list output: %q", out.String())
	}

	out.Reset()
	if err := Run([]string{"delete", "codex", "work", "--root", root}, &out, &out); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if !strings.Contains(out.String(), "state: removed") {
		t.Fatalf("unexpected delete output: %q", out.String())
	}

	out.Reset()
	if err := Run([]string{"list", "codex", "--root", root}, &out, &out); err != nil {
		t.Fatalf("list empty: %v", err)
	}
	if !strings.Contains(out.String(), "No saved profiles found.") {
		t.Fatalf("expected empty list message, got %q", out.String())
	}
}

func TestCLISavePiShowsIdentityWhenAvailable(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	root := t.TempDir()
	source := filepath.Join(root, "pi-source.json")

	accessToken := makeJWT(t, map[string]any{
		"exp": time.Now().UTC().Add(2 * time.Hour).Unix(),
		"https://api.openai.com/profile": map[string]any{
			"email": "pi.person@company.com",
		},
		"auth": map[string]any{
			"chatgpt_plan_type": "plus",
		},
	})
	raw := `{"openai-codex":{"type":"oauth","access":"` + accessToken + `","expires":` + strconv.FormatInt(time.Now().UTC().Add(2*time.Hour).UnixMilli(), 10) + `}}`
	writeFile(t, source, []byte(raw))

	var out bytes.Buffer
	if err := Run([]string{"save", "pi", "work", "--source", source, "--root", root}, &out, &out); err != nil {
		t.Fatalf("save pi with identity: %v", err)
	}
	if !strings.Contains(out.String(), "Saved pi.person@company.com (Plus) for work") {
		t.Fatalf("expected pi identity in save output, got %q", out.String())
	}
}

func TestCLIValidationAndParseErrors(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	root := t.TempDir()
	source := root + "/source.json"
	writeFile(t, source, []byte(`{"x":1}`))

	cases := []struct {
		name string
		args []string
		sub  string
	}{
		{"save invalid tool", []string{"save", "bad", "work"}, "invalid tool"},
		{"save missing label", []string{"save", "codex"}, "--label is required"},
		{"save bad label", []string{"save", "codex", "bad label"}, "--label must match"},
		{"save conflict label", []string{"save", "codex", "work", "--label", "personal", "--source", source, "--root", root}, "conflicting labels"},
		{"save too many args", []string{"save", "codex", "work", "extra", "--source", source, "--root", root}, "too many arguments"},
		{"save parse error", []string{"save", "codex", "work", "--bad-flag"}, "flag provided but not defined"},
		{"use invalid tool", []string{"use", "bad", "work"}, "invalid tool"},
		{"delete invalid tool", []string{"delete", "bad", "work"}, "invalid tool"},
		{"list invalid tool", []string{"list", "bad"}, "invalid tool"},
		{"list extra arg", []string{"list", "codex", "x"}, "usage: ags list"},
		{"list parse error", []string{"list", "--bad-flag"}, "flag provided but not defined"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			err := Run(tc.args, &out, &out)
			if err == nil || !strings.Contains(err.Error(), tc.sub) {
				t.Fatalf("expected error containing %q, got %v", tc.sub, err)
			}
		})
	}
}

func TestCLIHelperFunctions(t *testing.T) {
	if !wantsHelp([]string{"x", "--help"}) || !wantsHelp([]string{"-h"}) || wantsHelp([]string{"x"}) {
		t.Fatalf("unexpected wantsHelp behavior")
	}

	label, rest := splitPositionalLabel([]string{"codex", "work", "--root", "/tmp"})
	if label != "work" || len(rest) != 2 {
		t.Fatalf("unexpected split positional label: %q %+v", label, rest)
	}
	label, rest = splitPositionalLabel([]string{"codex", "--root", "/tmp"})
	if label != "" || len(rest) != 2 {
		t.Fatalf("unexpected split without positional label: %q %+v", label, rest)
	}

	if got, err := resolveLabel("work", "", "", nil); err != nil || got != "work" {
		t.Fatalf("resolve long label failed: %q err=%v", got, err)
	}
	if got, err := resolveLabel("", "work", "", nil); err != nil || got != "work" {
		t.Fatalf("resolve short label failed: %q err=%v", got, err)
	}
	if got, err := resolveLabel("", "", "", []string{"work"}); err != nil || got != "work" {
		t.Fatalf("resolve trailing positional failed: %q err=%v", got, err)
	}
	if _, err := resolveLabel("", "", "", []string{"a", "b"}); err == nil {
		t.Fatalf("expected too many args error")
	}
	if _, err := resolveLabel("work", "personal", "", nil); err == nil {
		t.Fatalf("expected conflict error")
	}

	if d := defaultRootDir(); d != "~/.config/ags" {
		t.Fatalf("unexpected default root dir %q", d)
	}
	if orDash("") != "-" || orDash("x") != "x" {
		t.Fatalf("unexpected orDash behavior")
	}
	if got := formatIdentity(AuthInsight{AccountEmail: "xyz.com", AccountPlan: "Plus"}); got != "xyz.com (Plus)" {
		t.Fatalf("expected identity to be shown, got %q", got)
	}
	if got := formatIdentity(AuthInsight{AccountEmail: "real.person@company.com", AccountPlan: "Plus"}); got != "real.person@company.com (Plus)" {
		t.Fatalf("unexpected formatted identity: %q", got)
	}
}

func TestCLITimeFormattingHelpers(t *testing.T) {
	if _, ok := parseISO("bad"); ok {
		t.Fatalf("parseISO should fail for invalid input")
	}
	if got := formatHumanTime("bad"); got != "bad" {
		t.Fatalf("expected passthrough raw value, got %q", got)
	}

	valid := time.Now().UTC().Add(time.Hour).Format(time.RFC3339)
	formatted := formatHumanTime(valid)
	if !strings.Contains(formatted, "(") {
		t.Fatalf("expected formatted absolute/relative time, got %q", formatted)
	}

	if got := humanizeDuration(0); got != "now" {
		t.Fatalf("expected now, got %q", got)
	}
	if got := humanizeDuration(48 * time.Hour); !strings.Contains(got, "in 2 days") {
		t.Fatalf("expected future days, got %q", got)
	}
	if got := humanizeDuration(-48 * time.Hour); !strings.Contains(got, "2 days ago") {
		t.Fatalf("expected past days, got %q", got)
	}
	if got := humanizeDuration(2 * time.Hour); !strings.Contains(got, "2 hours") {
		t.Fatalf("expected hours text, got %q", got)
	}
	if got := humanizeDuration(2 * time.Minute); !strings.Contains(got, "2 minutes") {
		t.Fatalf("expected minutes text, got %q", got)
	}
	if got := humanizeDuration(10 * time.Second); !strings.Contains(got, "less than a minute") {
		t.Fatalf("expected sub-minute text, got %q", got)
	}

	if plural(1, "day") != "1 day" || plural(2, "day") != "2 days" {
		t.Fatalf("unexpected plural formatting")
	}

	if got := formatRelative(time.Now().Add(time.Minute)); !strings.Contains(got, "in") {
		t.Fatalf("expected future relative text, got %q", got)
	}
	if got := formatRelative(time.Now().Add(-time.Minute)); !strings.Contains(got, "ago") {
		t.Fatalf("expected past relative text, got %q", got)
	}
}

func TestCLIPrintFunctionsAndUsageText(t *testing.T) {
	var out bytes.Buffer
	printRootUsage(&out)
	if !strings.Contains(out.String(), "COMMANDS:") {
		t.Fatalf("unexpected root usage: %q", out.String())
	}

	out.Reset()
	printCommandUsage(&out, "save")
	if !strings.Contains(out.String(), "ags save") {
		t.Fatalf("unexpected save usage: %q", out.String())
	}

	if got := commandUsageText("unknown"); got != rootUsageText() {
		t.Fatalf("expected fallback to root usage")
	}

	out.Reset()
	printInsight(&out, AuthInsight{Status: "valid", NeedsRefresh: "no", ExpiresAt: time.Now().UTC().Add(time.Hour).Format(time.RFC3339), LastRefresh: time.Now().UTC().Format(time.RFC3339), Details: []string{"d1"}}, true)
	if !strings.Contains(out.String(), "status") || !strings.Contains(out.String(), "detail: d1") {
		t.Fatalf("unexpected insight output: %q", out.String())
	}

	out.Reset()
	printInsight(&out, AuthInsight{}, true)
	if !strings.Contains(out.String(), "status") {
		t.Fatalf("expected status line for empty insight")
	}
}

func TestRunHelpNoTopic(t *testing.T) {
	var out bytes.Buffer
	if err := Run([]string{"help"}, &out, &out); err != nil {
		t.Fatalf("help with no topic should succeed: %v", err)
	}
	if !strings.Contains(out.String(), "USAGE:") {
		t.Fatalf("expected root usage output, got %q", out.String())
	}
}

func TestRunSaveRunUseRunDeleteErrorBranches(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	root := t.TempDir()
	source := root + "/source.json"
	writeFile(t, source, makeCodexAuthJSON(t, time.Now().Add(2*time.Hour)))

	var out bytes.Buffer

	if err := runSave([]string{}, &out); err == nil {
		t.Fatalf("expected runSave len args usage error")
	}
	if err := runUse([]string{}, &out); err == nil {
		t.Fatalf("expected runUse len args usage error")
	}
	if err := runDelete([]string{}, &out); err == nil {
		t.Fatalf("expected runDelete len args usage error")
	}

	if err := runSave([]string{"codex", "work", "--bad"}, &out); err == nil {
		t.Fatalf("expected runSave parse error")
	}
	if err := runUse([]string{"codex", "work", "--bad"}, &out); err == nil {
		t.Fatalf("expected runUse parse error")
	}
	if err := runDelete([]string{"codex", "work", "--bad"}, &out); err == nil {
		t.Fatalf("expected runDelete parse error")
	}

	if err := runUse([]string{"codex", "--root", root}, &out); err == nil || !strings.Contains(err.Error(), "--label is required") {
		t.Fatalf("expected runUse required label error, got %v", err)
	}
	if err := runUse([]string{"codex", "bad label", "--root", root}, &out); err == nil || !strings.Contains(err.Error(), "--label must match") {
		t.Fatalf("expected runUse label pattern error, got %v", err)
	}
	if err := runDelete([]string{"codex", "--root", root}, &out); err == nil || !strings.Contains(err.Error(), "--label is required") {
		t.Fatalf("expected runDelete required label error, got %v", err)
	}
	if err := runDelete([]string{"codex", "bad label", "--root", root}, &out); err == nil || !strings.Contains(err.Error(), "--label must match") {
		t.Fatalf("expected runDelete label pattern error, got %v", err)
	}

	if err := runSave([]string{"codex", "work", "--source", source, "--root", " "}, &out); err == nil {
		t.Fatalf("expected runSave NewManager error with empty root")
	}
	if err := runUse([]string{"codex", "work", "--root", " "}, &out); err == nil {
		t.Fatalf("expected runUse NewManager error with empty root")
	}
	if err := runDelete([]string{"codex", "work", "--root", " "}, &out); err == nil {
		t.Fatalf("expected runDelete NewManager error with empty root")
	}

	if err := runSave([]string{"codex", "work", "--root", root}, &out); err == nil {
		t.Fatalf("expected runSave manager.Save error when source cannot be resolved")
	}
	if err := runUse([]string{"codex", "work", "--root", root}, &out); err == nil {
		t.Fatalf("expected runUse manager.Use error for missing saved profile")
	}
	if err := runDelete([]string{"codex", "work", "--root", root}, &out); err == nil {
		t.Fatalf("expected runDelete manager.Delete error for missing profile")
	}

	out.Reset()
	if err := runSave([]string{"codex", "work", "--source", source, "--root", root}, &out); err != nil {
		t.Fatalf("runSave setup: %v", err)
	}
	out.Reset()
	if err := runSave([]string{"codex", "work", "--source", source, "--root", root}, &out); err != nil {
		t.Fatalf("runSave second save: %v", err)
	}
	if !strings.Contains(out.String(), "Saved codex for work") {
		t.Fatalf("expected save output, got %q", out.String())
	}
}

func TestRunListErrorAndVerboseBranches(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	root := t.TempDir()
	var out bytes.Buffer

	if err := runList([]string{"--root", " "}, &out); err == nil {
		t.Fatalf("expected runList NewManager error with empty root")
	}

	brokenRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(brokenRoot, "state.json"), 0o700); err != nil {
		t.Fatalf("mkdir state dir: %v", err)
	}
	if err := runList([]string{"--root", brokenRoot}, &out); err == nil {
		t.Fatalf("expected runList manager.List/loadState error")
	}

	source := filepath.Join(root, "source.json")
	writeFile(t, source, []byte(`{"last_refresh":"2026-01-01T00:00:00Z","tokens":{"access_token":"bad"}}`))
	if err := runSave([]string{"codex", "work", "--source", source, "--root", root}, &out); err != nil {
		t.Fatalf("save for list verbose branches: %v", err)
	}
	out.Reset()
	if err := runList([]string{"codex", "--verbose", "--root", root}, &out); err != nil {
		t.Fatalf("list verbose: %v", err)
	}
	if !strings.Contains(out.String(), "last refresh raw=") || !strings.Contains(out.String(), "detail=") {
		t.Fatalf("expected verbose last refresh/detail branches, got %q", out.String())
	}
}

func TestRunUseAndDeleteRemainingBranches(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	root := t.TempDir()
	source := filepath.Join(root, "source.json")
	writeFile(t, source, []byte(`{"x":1}`))
	var out bytes.Buffer

	if err := runSave([]string{"codex", "work", "--source", source, "--root", root}, &out); err != nil {
		t.Fatalf("setup save: %v", err)
	}

	// resolveLabel conflict branch in runUse
	if err := runUse([]string{"codex", "work", "--label", "personal", "--root", root}, &out); err == nil {
		t.Fatalf("expected runUse resolveLabel conflict error")
	}

	// resolveLabel conflict branch in runDelete
	if err := runDelete([]string{"codex", "work", "--label", "personal", "--root", root}, &out); err == nil {
		t.Fatalf("expected runDelete resolveLabel conflict error")
	}

	// snapshot missing output branch in runDelete
	m, err := NewManager(root)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	if err := os.Remove(m.snapshotPath(ToolCodex, "work")); err != nil {
		t.Fatalf("remove snapshot: %v", err)
	}
	out.Reset()
	if err := runDelete([]string{"codex", "work", "--root", root}, &out); err != nil {
		t.Fatalf("runDelete with missing snapshot: %v", err)
	}
	if !strings.Contains(out.String(), "snapshot file: already missing") {
		t.Fatalf("expected missing snapshot branch output, got %q", out.String())
	}
}

func TestRunVersionAndHelpTopic(t *testing.T) {
	oldVersion := Version
	Version = "0.1.0-test"
	defer func() { Version = oldVersion }()

	var out bytes.Buffer
	if err := Run([]string{"version"}, &out, &out); err != nil {
		t.Fatalf("version command: %v", err)
	}
	if !strings.Contains(out.String(), "ags version 0.1.0-test") {
		t.Fatalf("unexpected version output: %q", out.String())
	}

	out.Reset()
	if err := Run([]string{"--version"}, &out, &out); err != nil {
		t.Fatalf("--version command: %v", err)
	}
	if !strings.Contains(out.String(), "ags version 0.1.0-test") {
		t.Fatalf("unexpected --version output: %q", out.String())
	}

	out.Reset()
	if err := Run([]string{"help", "active"}, &out, &out); err != nil {
		t.Fatalf("help active: %v", err)
	}
	if !strings.Contains(out.String(), "ags active") {
		t.Fatalf("unexpected help active output: %q", out.String())
	}

	out.Reset()
	if err := Run([]string{"help", "version"}, &out, &out); err != nil {
		t.Fatalf("help version: %v", err)
	}
	if !strings.Contains(out.String(), "ags version") {
		t.Fatalf("unexpected help version output: %q", out.String())
	}
}

func TestRunActive(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	root := t.TempDir()
	piSrc := filepath.Join(t.TempDir(), "pi.json")
	writeFile(t, piSrc, []byte(`{"openai-codex":{"access":"codex-work"}}`))

	var out bytes.Buffer
	if err := Run([]string{"active", "--help"}, &out, &out); err != nil {
		t.Fatalf("active --help: %v", err)
	}
	if !strings.Contains(out.String(), "ags active") {
		t.Fatalf("expected active usage output, got %q", out.String())
	}

	out.Reset()
	if err := Run([]string{"save", "pi", "work", "--source", piSrc, "--root", root}, &out, &out); err != nil {
		t.Fatalf("save pi for active: %v", err)
	}

	out.Reset()
	if err := Run([]string{"active", "--root", root}, &out, &out); err != nil {
		t.Fatalf("active all: %v", err)
	}
	if !strings.Contains(out.String(), "tool\tactive label\tstatus\truntime") {
		t.Fatalf("unexpected active output header: %q", out.String())
	}

	out.Reset()
	if err := Run([]string{"active", "pi", "--verbose", "--root", root}, &out, &out); err != nil {
		t.Fatalf("active filtered: %v", err)
	}
	if !strings.Contains(out.String(), "pi") {
		t.Fatalf("expected pi row in active output: %q", out.String())
	}

	if err := Run([]string{"active", "bad"}, &out, &out); err == nil {
		t.Fatalf("expected invalid tool error")
	}
	if err := Run([]string{"active", "pi", "extra", "--root", root}, &out, &out); err == nil {
		t.Fatalf("expected active usage error for extra arg")
	}
	if err := Run([]string{"active", "pi", "--bad-flag", "--root", root}, &out, &out); err == nil {
		t.Fatalf("expected active parse error")
	}
	if err := Run([]string{"active", "pi", "--root", " "}, &out, &out); err == nil {
		t.Fatalf("expected active NewManager error")
	}

	brokenRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(brokenRoot, "state.json"), 0o700); err != nil {
		t.Fatalf("mkdir broken state path: %v", err)
	}
	if err := Run([]string{"active", "--root", brokenRoot}, &out, &out); err == nil {
		t.Fatalf("expected active manager error")
	}

	codexSrc := filepath.Join(t.TempDir(), "codex.json")
	writeFile(t, codexSrc, makeCodexAuthJSON(t, time.Now().Add(time.Hour)))
	out.Reset()
	if err := Run([]string{"save", "codex", "work", "--source", codexSrc, "--root", root}, &out, &out); err != nil {
		t.Fatalf("save codex work: %v", err)
	}
	out.Reset()
	if err := Run([]string{"save", "codex", "work-clone", "--source", codexSrc, "--root", root}, &out, &out); err != nil {
		t.Fatalf("save codex work-clone: %v", err)
	}
	out.Reset()
	if err := Run([]string{"use", "codex", "work", "--root", root}, &out, &out); err != nil {
		t.Fatalf("use codex work: %v", err)
	}
	out.Reset()
	if err := Run([]string{"active", "codex", "--verbose", "--root", root}, &out, &out); err != nil {
		t.Fatalf("active codex verbose: %v", err)
	}
	if !strings.Contains(out.String(), "detail=multiple saved labels match current runtime auth") {
		t.Fatalf("expected active verbose detail, got %q", out.String())
	}
}
