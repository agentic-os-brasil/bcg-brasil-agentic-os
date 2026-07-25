package main

import (
	"os"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/cli"
)

func main() {
	os.Exit(cli.RunWithInput(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}
