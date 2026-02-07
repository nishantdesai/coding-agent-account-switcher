package main

import (
	"fmt"
	"io"
	"os"

	"github.com/nishantdesai/coding-agent-account-switcher/internal/ags"
)

var osExit = os.Exit

func run(args []string, stdout io.Writer, stderr io.Writer) int {
	if err := ags.Run(args, stdout, stderr); err != nil {
		fmt.Fprintln(stderr, "Error:", err)
		return 1
	}
	return 0
}

func main() {
	osExit(run(os.Args[1:], os.Stdout, os.Stderr))
}
