package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/datahome"
)

func inspectHome(args []string, out, diagnostic io.Writer) int {
	if len(args) == 0 || args[0] != "inspect" {
		fmt.Fprintln(diagnostic, "usage: home inspect --product-root ROOT [--source-root ROOT]")
		return 2
	}
	flags := flag.NewFlagSet("home inspect", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	product := flags.String("product-root", "", "absolute managed product directory")
	source := flags.String("source-root", "", "explicit prior ZIP directory")
	if err := flags.Parse(args[1:]); err != nil {
		fmt.Fprintln(diagnostic, "invalid home inspect arguments")
		return 2
	}
	if *product == "" || flags.NArg() != 0 {
		fmt.Fprintln(diagnostic, "home inspect requires --product-root and no positional arguments")
		return 2
	}
	user, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintln(diagnostic, "user home unavailable")
		return 2
	}
	result, err := datahome.Resolve(datahome.Options{UserHome: user, ProductRoot: *product, SourceRoot: *source, LookupEnv: os.LookupEnv})
	if err != nil {
		fmt.Fprintln(diagnostic, "home inspection failed:", result.Diagnostic)
		return 2
	}
	if err := json.NewEncoder(out).Encode(result); err != nil {
		fmt.Fprintln(diagnostic, "home inspection output unavailable")
		return 2
	}
	return 0
}
