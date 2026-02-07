package main

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestRunSuccess(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := run([]string{"help"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	if !strings.Contains(stdout.String(), "USAGE:") {
		t.Fatalf("expected help text on stdout, got %q", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}
}

func TestRunError(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := run([]string{"unknown"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("expected exit code 1, got %d", code)
	}
	if !strings.Contains(stderr.String(), "Error:") {
		t.Fatalf("expected error output, got %q", stderr.String())
	}
}

func TestMainCallsExit(t *testing.T) {
	oldArgs := os.Args
	oldExit := osExit
	defer func() {
		os.Args = oldArgs
		osExit = oldExit
	}()

	os.Args = []string{"ags", "help"}

	exitCode := -1
	osExit = func(code int) {
		exitCode = code
		panic("exit called")
	}

	defer func() {
		r := recover()
		if r == nil {
			t.Fatalf("expected panic from fake osExit")
		}
		if exitCode != 0 {
			t.Fatalf("expected exit code 0, got %d", exitCode)
		}
	}()

	main()
}
