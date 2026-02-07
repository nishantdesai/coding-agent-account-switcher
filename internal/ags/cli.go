package ags

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"regexp"
	"strings"
)

var labelPattern = regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)

func Run(args []string, stdout io.Writer, stderr io.Writer) error {
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
	case "list":
		return runList(args[1:], stdout)
	case "help", "--help", "-h":
		printRootUsage(stdout)
		return nil
	default:
		return fmt.Errorf("unknown command %q\n\n%s", command, rootUsageText())
	}
}

func runSave(args []string, stdout io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: ags save <tool> --label <name> [--source <path>] [--root <path>]")
	}
	tool, ok := ParseTool(strings.ToLower(args[0]))
	if !ok {
		return fmt.Errorf("invalid tool %q. expected one of: codex, claude, pi", args[0])
	}

	fs := flag.NewFlagSet("save", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	label := fs.String("label", "", "Profile label name, e.g. work")
	source := fs.String("source", "", "Override source auth path for this save")
	root := fs.String("root", defaultRootDir(), "AGS data root directory")

	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("usage: ags save <tool> --label <name> [--source <path>] [--root <path>]")
	}
	if strings.TrimSpace(*label) == "" {
		return errors.New("--label is required")
	}
	if !labelPattern.MatchString(*label) {
		return errors.New("--label must match [a-zA-Z0-9._-]+")
	}

	manager, err := NewManager(*root)
	if err != nil {
		return err
	}
	result, err := manager.Save(tool, *label, *source)
	if err != nil {
		return err
	}

	fmt.Fprintf(stdout, "Saved %s label=%s\n", result.Tool, result.Label)
	fmt.Fprintf(stdout, "- source: %s\n", result.SourcePath)
	fmt.Fprintf(stdout, "- snapshot: %s\n", result.SnapshotPath)
	if result.ChangedSinceLastSave {
		fmt.Fprintln(stdout, "- change: changed since last save (new auth snapshot)")
	} else {
		fmt.Fprintln(stdout, "- change: unchanged since last save")
	}
	printInsight(stdout, result.Insight)
	return nil
}

func runUse(args []string, stdout io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: ags use <tool> --label <name> [--target <path>] [--root <path>]")
	}
	tool, ok := ParseTool(strings.ToLower(args[0]))
	if !ok {
		return fmt.Errorf("invalid tool %q. expected one of: codex, claude, pi", args[0])
	}

	fs := flag.NewFlagSet("use", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	label := fs.String("label", "", "Profile label name, e.g. work")
	target := fs.String("target", "", "Override runtime target path for this use")
	root := fs.String("root", defaultRootDir(), "AGS data root directory")

	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("usage: ags use <tool> --label <name> [--target <path>] [--root <path>]")
	}
	if strings.TrimSpace(*label) == "" {
		return errors.New("--label is required")
	}
	if !labelPattern.MatchString(*label) {
		return errors.New("--label must match [a-zA-Z0-9._-]+")
	}

	manager, err := NewManager(*root)
	if err != nil {
		return err
	}
	result, err := manager.Use(tool, *label, *target)
	if err != nil {
		return err
	}

	fmt.Fprintf(stdout, "Using %s label=%s\n", result.Tool, result.Label)
	fmt.Fprintf(stdout, "- target: %s\n", result.TargetPath)
	fmt.Fprintf(stdout, "- refresh signal: %s\n", result.ChangeSinceLastUse)
	printInsight(stdout, result.Insight)
	return nil
}

func runList(args []string, stdout io.Writer) error {
	var toolFilter *Tool
	var flagArgs []string

	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		tool, ok := ParseTool(strings.ToLower(args[0]))
		if !ok {
			return fmt.Errorf("invalid tool %q. expected one of: codex, claude, pi", args[0])
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

	fmt.Fprintln(stdout, "tool\tlabel\tstatus\tneeds_refresh\texpires_at\tlast_saved\tlast_used")
	for _, item := range items {
		fmt.Fprintf(
			stdout,
			"%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			item.Tool,
			item.Label,
			orDash(item.AuthInsight.Status),
			orDash(item.AuthInsight.NeedsRefresh),
			orDash(item.AuthInsight.ExpiresAt),
			orDash(item.SavedAt),
			orDash(item.LastUsedAt),
		)
		if *verbose {
			fmt.Fprintf(stdout, "  snapshot=%s\n", item.Snapshot)
			if item.AuthInsight.LastRefresh != "" {
				fmt.Fprintf(stdout, "  last_refresh=%s\n", item.AuthInsight.LastRefresh)
			}
			for _, detail := range item.AuthInsight.Details {
				fmt.Fprintf(stdout, "  detail=%s\n", detail)
			}
		}
	}
	return nil
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

func printInsight(out io.Writer, insight AuthInsight) {
	fmt.Fprintf(out, "- status: %s\n", orDash(insight.Status))
	fmt.Fprintf(out, "- needs_refresh: %s\n", orDash(insight.NeedsRefresh))
	if insight.ExpiresAt != "" {
		fmt.Fprintf(out, "- expires_at: %s\n", insight.ExpiresAt)
	}
	if insight.LastRefresh != "" {
		fmt.Fprintf(out, "- last_refresh: %s\n", insight.LastRefresh)
	}
	for _, detail := range insight.Details {
		fmt.Fprintf(out, "- detail: %s\n", detail)
	}
}

func printRootUsage(out io.Writer) {
	fmt.Fprint(out, rootUsageText())
}

func rootUsageText() string {
	return `ags - Coding agent account switcher

Commands:
  ags save <tool> --label <name> [--source <path>]
  ags use <tool> --label <name> [--target <path>]
  ags list [tool] [--verbose]

Tools:
  codex | claude | pi

Examples:
  ags save codex --label work
  ags save pi --label personal
  ags use codex --label work
  ags list
  ags list codex --verbose
`
}
