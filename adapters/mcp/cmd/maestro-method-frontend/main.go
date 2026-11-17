// maestro-method-frontend is an offline factory, not an installed runtime command.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp/packaging"
)

func run(args []string, output io.Writer) error {
	flags := flag.NewFlagSet("maestro-method-frontend", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	destination := flags.String("destination", "", "new absolute factory output directory")
	version := flags.String("version", "", "public numeric semantic version")
	if err := flags.Parse(args); err != nil || *destination == "" || *version == "" || flags.NArg() != 0 {
		return errors.New("usage: maestro-method-frontend --destination NEW_DIRECTORY --version VERSION")
	}
	report, err := packaging.BuildDesktopMethodFrontend(packaging.DesktopFrontendOptions{Destination: *destination, Version: *version})
	if err != nil {
		return err
	}
	return json.NewEncoder(output).Encode(report)
}

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
}
