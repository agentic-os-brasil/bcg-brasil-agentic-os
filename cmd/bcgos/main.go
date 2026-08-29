package main

import (
	"os"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/cli"
)

var Version = "0.0.0-dev"

func main() {
	cli.Version = Version
	os.Exit(cli.Run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}
