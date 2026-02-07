package main

import (
	"fmt"
	"os"

	"github.com/nishantdesai/coding-agent-account-switcher/internal/ags"
)

func main() {
	if err := ags.Run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}
