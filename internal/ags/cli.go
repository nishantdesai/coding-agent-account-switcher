package ags

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"
)

var labelPattern = regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)

func Run(args []string, stdout io.Writer, stderr io.Writer) error {
	_ = stderr
	if len(args) == 0 {
		printRootUsage(stdout)
		return nil
	}

	command := args[0]
	switch command {
	case "save":
		return runSave(args[1:], stdout)
	case "use":
		return runUse(args[1:], stdout)
	case "delete":
		return runDelete(args[1:], stdout)
	case "list":
		return runList(args[1:], stdout)
	case "active":
		return runActive(args[1:], stdout)
	case "version", "--version", "-V":
		return runVersion(stdout)
	case "help", "--help", "-h":
		return runHelp(args[1:], stdout)
	default:
		return fmt.Errorf("unknown command %q\n\n%s", command, rootUsageText())
	}
}

func runHelp(args []string, stdout io.Writer) error {
	if len(args) == 0 {
		printRootUsage(stdout)
		return nil
	}

	command := strings.ToLower(args[0])
	switch command {
	case "save", "use", "delete", "list", "active", "version":
		printCommandUsage(stdout, command)
		return nil
	default:
		return fmt.Errorf("unknown help topic %q\n\n%s", command, rootUsageText())
	}
}

func runSave(args []string, stdout io.Writer) error {
	if wantsHelp(args) {
		printCommandUsage(stdout, "save")
		return nil
	}
	if len(args) == 0 {
		return errors.New("usage: ags save <tool> <label> [--source <path>] [--provider <id>] [--root <path>] OR ags save <tool> --label <name> [--source <path>] [--provider <id>] [--root <path>]")
	}
	tool, ok := ParseTool(strings.ToLower(args[0]))
	if !ok {
		return fmt.Errorf("invalid tool %q. expected one of: codex, pi", args[0])
	}

	positionalLabel, parseArgs := splitPositionalLabel(args)

	fs := flag.NewFlagSet("save", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	label := fs.String("label", "", "Profile label name, e.g. work")
	labelShort := fs.String("l", "", "Profile label name, e.g. work")
	source := fs.String("source", "", "Override source auth path for this save")
	provider := fs.String("provider", "", "For pi only: save just one provider (codex, anthropic, or provider key)")
	root := fs.String("root", defaultRootDir(), "AGS data root directory")
	verbose := fs.Bool("verbose", false, "Print additional detail lines")

	if err := fs.Parse(parseArgs); err != nil {
		return err
	}

	resolvedLabel, err := resolveLabel(*label, *labelShort, positionalLabel, fs.Args())
	if err != nil {
		return err
	}
	if strings.TrimSpace(resolvedLabel) == "" {
		return errors.New("--label is required")
	}
	if !labelPattern.MatchString(resolvedLabel) {
		return errors.New("--label must match [a-zA-Z0-9._-]+")
	}
	if strings.TrimSpace(*provider) != "" && tool != ToolPi {
		return errors.New("--provider is only supported for tool=pi")
	}

	manager, err := NewManager(*root)
	if err != nil {
		return err
	}
	result, err := manager.SaveWithPIProvider(tool, resolvedLabel, *source, strings.TrimSpace(*provider))
	if err != nil {
		return err
	}

	identity := formatIdentity(result.Insight)
	if identity != "" {
		fmt.Fprintf(stdout, "Saved %s for %s\n", identity, result.Label)
	} else {
		fmt.Fprintf(stdout, "Saved %s for %s\n", result.Tool, result.Label)
	}

	if *verbose {
		fmt.Fprintf(stdout, "- source: %s\n", result.SourcePath)
		fmt.Fprintf(stdout, "- snapshot: %s\n", result.SnapshotPath)
		if result.ChangedSinceLastSave {
			fmt.Fprintln(stdout, "- change: changed since last save (new auth snapshot)")
		} else {
			fmt.Fprintln(stdout, "- change: unchanged since last save")
		}
		printInsight(stdout, result.Insight, true)
	}
	return nil
}

func runUse(args []string, stdout io.Writer) error {
	if wantsHelp(args) {
		printCommandUsage(stdout, "use")
		return nil
	}
	if len(args) == 0 {
		return errors.New("usage: ags use <tool> <label> [--target <path>] [--provider <id>] [--root <path>] OR ags use <tool> --label <name> [--target <path>] [--provider <id>] [--root <path>]")
	}
	tool, ok := ParseTool(strings.ToLower(args[0]))
	if !ok {
		return fmt.Errorf("invalid tool %q. expected one of: codex, pi", args[0])
	}

	positionalLabel, parseArgs := splitPositionalLabel(args)

	fs := flag.NewFlagSet("use", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	label := fs.String("label", "", "Profile label name, e.g. work")
	labelShort := fs.String("l", "", "Profile label name, e.g. work")
	target := fs.String("target", "", "Override runtime target path for this use")
	provider := fs.String("provider", "", "For pi only: apply just one provider (codex, anthropic, or provider key)")
	root := fs.String("root", defaultRootDir(), "AGS data root directory")
	verbose := fs.Bool("verbose", false, "Print additional detail lines")

	if err := fs.Parse(parseArgs); err != nil {
		return err
	}

	resolvedLabel, err := resolveLabel(*label, *labelShort, positionalLabel, fs.Args())
	if err != nil {
		return err
	}
	if strings.TrimSpace(resolvedLabel) == "" {
		return errors.New("--label is required")
	}
	if !labelPattern.MatchString(resolvedLabel) {
		return errors.New("--label must match [a-zA-Z0-9._-]+")
	}
	if strings.TrimSpace(*provider) != "" && tool != ToolPi {
		return errors.New("--provider is only supported for tool=pi")
	}

	manager, err := NewManager(*root)
	if err != nil {
		return err
	}
	result, err := manager.UseWithPIProvider(tool, resolvedLabel, *target, strings.TrimSpace(*provider))
	if err != nil {
		return err
	}

	identity := formatIdentity(result.Insight)
	if identity != "" {
		fmt.Fprintf(stdout, "Using %s for %s\n", identity, result.Label)
	} else {
		fmt.Fprintf(stdout, "Using %s for %s\n", result.Tool, result.Label)
	}

	if *verbose {
		fmt.Fprintf(stdout, "- target: %s\n", result.TargetPath)
		fmt.Fprintf(stdout, "- refresh signal: %s\n", result.ChangeSinceLastUse)
		printInsight(stdout, result.Insight, true)
	}
	return nil
}

func runDelete(args []string, stdout io.Writer) error {
	if wantsHelp(args) {
		printCommandUsage(stdout, "delete")
		return nil
	}
	if len(args) == 0 {
		return errors.New("usage: ags delete <tool> <label> [--root <path>] OR ags delete <tool> --label <name> [--root <path>]")
	}
	tool, ok := ParseTool(strings.ToLower(args[0]))
	if !ok {
		return fmt.Errorf("invalid tool %q. expected one of: codex, pi", args[0])
	}

	positionalLabel, parseArgs := splitPositionalLabel(args)

	fs := flag.NewFlagSet("delete", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	label := fs.String("label", "", "Profile label name, e.g. work")
	labelShort := fs.String("l", "", "Profile label name, e.g. work")
	root := fs.String("root", defaultRootDir(), "AGS data root directory")

	if err := fs.Parse(parseArgs); err != nil {
		return err
	}

	resolvedLabel, err := resolveLabel(*label, *labelShort, positionalLabel, fs.Args())
	if err != nil {
		return err
	}
	if strings.TrimSpace(resolvedLabel) == "" {
		return errors.New("--label is required")
	}
	if !labelPattern.MatchString(resolvedLabel) {
		return errors.New("--label must match [a-zA-Z0-9._-]+")
	}

	manager, err := NewManager(*root)
	if err != nil {
		return err
	}
	result, err := manager.Delete(tool, resolvedLabel)
	if err != nil {
		return err
	}

	fmt.Fprintf(stdout, "Deleted %s label=%s\n", result.Tool, result.Label)
	fmt.Fprintf(stdout, "- snapshot: %s\n", result.SnapshotPath)
	if result.SnapshotDeleted {
		fmt.Fprintln(stdout, "- snapshot file: removed")
	} else {
		fmt.Fprintln(stdout, "- snapshot file: already missing")
	}
	fmt.Fprintln(stdout, "- state: removed")
	return nil
}

func runList(args []string, stdout io.Writer) error {
	if wantsHelp(args) {
		printCommandUsage(stdout, "list")
		return nil
	}

	var toolFilter *Tool
	var flagArgs []string

	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		tool, ok := ParseTool(strings.ToLower(args[0]))
		if !ok {
			return fmt.Errorf("invalid tool %q. expected one of: codex, pi", args[0])
		}
		toolFilter = &tool
		flagArgs = args[1:]
	} else {
		flagArgs = args
	}

	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	root := fs.String("root", defaultRootDir(), "AGS data root directory")
	verbose := fs.Bool("verbose", false, "Print additional detail lines")
	if err := fs.Parse(flagArgs); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return errors.New("usage: ags list [tool] [--verbose] [--root <path>]")
	}

	manager, err := NewManager(*root)
	if err != nil {
		return err
	}

	items, err := manager.List(toolFilter)
	if err != nil {
		return err
	}
	if len(items) == 0 {
		fmt.Fprintln(stdout, "No saved profiles found.")
		return nil
	}

	fmt.Fprintln(stdout, "Saved profiles:")
	currentTool := Tool("")
	for i, item := range items {
		if item.Tool != currentTool {
			if i > 0 {
				fmt.Fprintln(stdout)
			}
			currentTool = item.Tool
			fmt.Fprintf(stdout, "%s\n", currentTool)
		}

		fmt.Fprintf(
			stdout,
			"  %-18s status=%-13s refresh=%-7s expires=%s\n",
			item.Label,
			orDash(item.AuthInsight.Status),
			orDash(item.AuthInsight.NeedsRefresh),
			summarizeExpiry(item.AuthInsight.ExpiresAt),
		)

		if *verbose {
			if identity := formatIdentity(item.AuthInsight); identity != "" {
				fmt.Fprintf(stdout, "    account: %s\n", identity)
			}
			if item.AuthInsight.LastRefresh != "" {
				fmt.Fprintf(stdout, "    last refresh: %s\n", formatHumanTime(item.AuthInsight.LastRefresh))
			}
			fmt.Fprintf(stdout, "    saved: %s\n", formatHumanTime(item.SavedAt))
			if item.LastUsedAt != "" {
				fmt.Fprintf(stdout, "    last used: %s\n", formatHumanTime(item.LastUsedAt))
			}
			fmt.Fprintf(stdout, "    snapshot: %s\n", item.Snapshot)
			for _, detail := range item.AuthInsight.Details {
				fmt.Fprintf(stdout, "    detail: %s\n", detail)
			}
		}
	}
	return nil
}

func runVersion(stdout io.Writer) error {
	fmt.Fprintf(stdout, "ags version %s\n", Version)
	return nil
}

func runActive(args []string, stdout io.Writer) error {
	if wantsHelp(args) {
		printCommandUsage(stdout, "active")
		return nil
	}

	var toolFilter *Tool
	var flagArgs []string

	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		tool, ok := ParseTool(strings.ToLower(args[0]))
		if !ok {
			return fmt.Errorf("invalid tool %q. expected one of: codex, pi", args[0])
		}
		toolFilter = &tool
		flagArgs = args[1:]
	} else {
		flagArgs = args
	}

	fs := flag.NewFlagSet("active", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	root := fs.String("root", defaultRootDir(), "AGS data root directory")
	verbose := fs.Bool("verbose", false, "Print additional detail lines")
	if err := fs.Parse(flagArgs); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return errors.New("usage: ags active [tool] [--verbose] [--root <path>]")
	}

	manager, err := NewManager(*root)
	if err != nil {
		return err
	}

	items, err := manager.Active(toolFilter)
	if err != nil {
		return err
	}

	fmt.Fprintln(stdout, "tool\tactive label\tstatus\truntime")
	for _, item := range items {
		fmt.Fprintf(stdout, "%s\t%s\t%s\t%s\n", item.Tool, orDash(item.ActiveLabel), item.Status, item.RuntimePath)
		if *verbose {
			for _, detail := range item.Details {
				fmt.Fprintf(stdout, "  detail=%s\n", detail)
			}
		}
	}
	return nil
}

func wantsHelp(args []string) bool {
	for _, arg := range args {
		if arg == "-h" || arg == "--help" {
			return true
		}
	}
	return false
}

func splitPositionalLabel(args []string) (string, []string) {
	if len(args) > 1 && !strings.HasPrefix(args[1], "-") {
		return args[1], args[2:]
	}
	return "", args[1:]
}

func resolveLabel(longLabel string, shortLabel string, positional string, trailingArgs []string) (string, error) {
	longLabel = strings.TrimSpace(longLabel)
	shortLabel = strings.TrimSpace(shortLabel)
	positional = strings.TrimSpace(positional)

	if positional == "" && len(trailingArgs) == 1 {
		positional = strings.TrimSpace(trailingArgs[0])
	}
	if len(trailingArgs) > 1 {
		return "", errors.New("too many arguments; provide exactly one label")
	}

	labels := make([]string, 0, 3)
	for _, candidate := range []string{longLabel, shortLabel, positional} {
		if candidate == "" {
			continue
		}
		labels = append(labels, candidate)
	}
	if len(labels) == 0 {
		return "", nil
	}

	label := labels[0]
	for _, candidate := range labels[1:] {
		if candidate != label {
			return "", errors.New("conflicting labels provided via positional and flag values")
		}
	}
	return label, nil
}

func defaultRootDir() string {
	return "~/.config/ags"
}

func orDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "-"
	}
	return s
}

func printInsight(out io.Writer, insight AuthInsight, verbose bool) {
	fmt.Fprintf(out, "- status: %s\n", orDash(insight.Status))
	fmt.Fprintf(out, "- needs refresh: %s\n", orDash(insight.NeedsRefresh))
	if insight.ExpiresAt != "" {
		fmt.Fprintf(out, "- expires: %s\n", formatHumanTime(insight.ExpiresAt))
	}
	if insight.LastRefresh != "" {
		fmt.Fprintf(out, "- last refresh: %s\n", formatHumanTime(insight.LastRefresh))
	}
	if !verbose {
		return
	}
	if insight.AccountID != "" {
		fmt.Fprintf(out, "- account id: %s\n", insight.AccountID)
	}
	for _, detail := range insight.Details {
		fmt.Fprintf(out, "- detail: %s\n", detail)
	}
}

func formatIdentity(insight AuthInsight) string {
	email := strings.TrimSpace(insight.AccountEmail)
	plan := strings.TrimSpace(insight.AccountPlan)
	if email == "" {
		return ""
	}
	if plan == "" {
		return email
	}
	return fmt.Sprintf("%s (%s)", email, plan)
}

func formatHumanTime(raw string) string {
	t, ok := parseISO(raw)
	if !ok {
		return raw
	}
	return fmt.Sprintf("%s (%s)", formatRelative(t), t.UTC().Format("Mon, Jan 2, 2006, 3:04 PM MST"))
}

func summarizeExpiry(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "-"
	}
	t, ok := parseISO(raw)
	if !ok {
		return raw
	}
	return formatRelative(t)
}

func parseISO(raw string) (time.Time, bool) {
	t, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return time.Time{}, false
	}
	return t.UTC(), true
}

func formatRelative(t time.Time) string {
	return humanizeDuration(time.Until(t))
}

func humanizeDuration(delta time.Duration) string {
	if delta == 0 {
		return "now"
	}

	future := delta > 0
	if delta < 0 {
		delta = -delta
	}

	days := int(delta / (24 * time.Hour))
	delta %= 24 * time.Hour
	hours := int(delta / time.Hour)
	delta %= time.Hour
	minutes := int(delta / time.Minute)

	parts := make([]string, 0, 2)
	if days > 0 {
		parts = append(parts, plural(days, "day"))
	}
	if hours > 0 && len(parts) < 2 {
		parts = append(parts, plural(hours, "hour"))
	}
	if days == 0 && minutes > 0 && len(parts) < 2 {
		parts = append(parts, plural(minutes, "minute"))
	}
	if len(parts) == 0 {
		parts = append(parts, "less than a minute")
	}

	text := strings.Join(parts, " ")
	if future {
		return "in " + text
	}
	return text + " ago"
}

func plural(n int, unit string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, unit)
	}
	return fmt.Sprintf("%d %ss", n, unit)
}

func printRootUsage(out io.Writer) {
	fmt.Fprint(out, rootUsageText())
}

func printCommandUsage(out io.Writer, command string) {
	fmt.Fprint(out, commandUsageText(command))
}

func rootUsageText() string {
	return `ags - Coding Agent Account Switcher

USAGE:
  ags <command> [arguments] [flags]

COMMANDS:
  save      Save current tool auth JSON as a labeled snapshot.
  use       Activate a saved labeled snapshot for a tool.
  delete    Remove a saved labeled snapshot and its metadata.
  list      List saved snapshots with status and refresh signals.
  active    Show which saved profile is currently active.
  version   Show CLI version.
  help      Show detailed help. Use "ags help <command>".

TOOLS:
  codex, pi

GLOBAL NOTES:
  - Labels must match [a-zA-Z0-9._-]+.
  - Auth files must be strict JSON objects.
  - Default AGS data root: ~/.config/ags

QUICK START:
  ags save codex work
  ags use codex work
  ags active codex
  ags list --verbose

DETAIL:
  ags help save
  ags help use
  ags help delete
  ags help list
  ags help active
  ags version
`
}

func commandUsageText(command string) string {
	switch command {
	case "save":
		return `ags save - store a labeled auth snapshot

USAGE:
  ags save <tool> <label> [--source <path>] [--root <path>]
  ags save <tool> --label <name> [--source <path>] [--root <path>]

FLAGS:
  --label, -l <name> Required profile label (example: work, personal)
  --source <path>   Optional override source auth file path
  --provider <id>   For pi only: save just one provider (codex, anthropic, or key)
  --root <path>     Optional AGS data root (default: ~/.config/ags)
  --verbose         Show additional detail lines

EXAMPLES:
  ags save codex work
  ags save pi personal
  ags save pi codex-work --provider codex
  ags save pi --label work --source ~/.pi/agent/auth.json
`
	case "use":
		return `ags use - activate a labeled auth snapshot

USAGE:
  ags use <tool> <label> [--target <path>] [--root <path>]
  ags use <tool> --label <name> [--target <path>] [--root <path>]

FLAGS:
  --label, -l <name> Required profile label to activate
  --target <path>   Optional override runtime auth destination
  --provider <id>   For pi only: apply just one provider (codex, anthropic, or key)
  --root <path>     Optional AGS data root (default: ~/.config/ags)
  --verbose         Show additional detail lines

BEHAVIOR:
  - Writes the saved snapshot into the tool runtime auth path.
  - For pi, merges only providers present in the saved snapshot into the existing runtime auth JSON.
  - Prints refresh signal: first use / unchanged / changed since last use.

EXAMPLES:
  ags use codex work
  ags use pi personal
  ags use pi codex-work --provider codex
`
	case "delete":
		return `ags delete - remove a labeled auth snapshot

USAGE:
  ags delete <tool> <label> [--root <path>]
  ags delete <tool> --label <name> [--root <path>]

FLAGS:
  --label, -l <name> Required profile label to delete
  --root <path>     Optional AGS data root (default: ~/.config/ags)

BEHAVIOR:
  - Deletes snapshot file from ~/.config/ags/snapshots/<tool>/<label>.json
  - Removes matching entry from ~/.config/ags/state.json
  - Does NOT modify current runtime auth file used by the tool

EXAMPLES:
  ags delete codex work
  ags delete pi personal
`
	case "list":
		return `ags list - inspect saved profiles

USAGE:
  ags list [tool] [--verbose] [--root <path>]

FLAGS:
  --verbose         Show account, timestamps, snapshot path, and details
  --root <path>     Optional AGS data root (default: ~/.config/ags)

OUTPUT:
  Grouped by tool with one concise line per label.
  Use --verbose for additional metadata.

EXAMPLES:
  ags list
  ags list codex
  ags list pi --verbose
`
	case "active":
		return `ags active - show active saved profile

USAGE:
  ags active [tool] [--verbose] [--root <path>]

FLAGS:
  --verbose         Show additional detail lines
  --root <path>     Optional AGS data root (default: ~/.config/ags)

OUTPUT COLUMNS:
  tool, active label, status, runtime

EXAMPLES:
  ags active
  ags active codex
  ags active pi --verbose
`
	case "version":
		return `ags version - show CLI version

USAGE:
  ags version

EXAMPLES:
  ags version
`
	default:
		return rootUsageText()
	}
}
