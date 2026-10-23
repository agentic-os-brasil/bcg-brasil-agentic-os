// build-plugin packages a local synthetic experiment, never a production release.
package main

import (
	"flag"
	"fmt"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/dev/spikes/maestromcp/packaging"
	"io"
	"os"
	"path/filepath"
)

func run(args []string, out, diagnostic io.Writer) int {
	flags := flag.NewFlagSet("build-plugin", flag.ContinueOnError)
	flags.SetOutput(diagnostic)
	destination := flags.String("destination", "", "new plugin directory (existing parent required)")
	server := flags.String("server", "", "native server binary to copy")
	state := flags.String("state-root", "", "absolute existing synthetic state root, outside plugin")
	workspace := flags.String("workspace", "", "fixed server workspace: case-a or case-b")
	version := flags.String("version", "0.0.1", "numeric semantic version")
	name := flags.String("name", "maestro-architecture-spike", "experimental package identity")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 {
		fmt.Fprintln(diagnostic, "build-plugin: unexpected positional arguments")
		return 2
	}
	if err := packaging.BuildPlugin(packaging.Options{Destination: *destination, Server: *server, StateRoot: *state, Workspace: *workspace, Version: *version, Name: *name}); err != nil {
		fmt.Fprintln(diagnostic, "build-plugin:", err)
		return 2
	}
	absolute, _ := filepath.Abs(*destination)
	fmt.Fprintln(out, absolute)
	return 0
}

func build(destination, server, state, workspace, version string) error {
	return packaging.BuildPlugin(packaging.Options{Destination: destination, Server: server, StateRoot: state, Workspace: workspace, Version: version})
}

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }
