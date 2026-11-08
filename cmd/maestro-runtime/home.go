package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/datahome"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/zipruntime"
)

func inspectHome(args []string, out, diagnostic io.Writer) int {
	if len(args) > 0 && (args[0] == "paths" || args[0] == "init") {
		return homePaths(args, out, diagnostic)
	}
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

func homePaths(args []string, out, diagnostic io.Writer) int {
	flags := flag.NewFlagSet("home "+args[0], flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	product := flags.String("product-root", "", "managed product directory")
	user := flags.String("user-home", "", "OS user home")
	field := ""
	if args[0] == "paths" {
		flags.StringVar(&field, "field", "", "one admitted path or mode")
	}
	if flags.Parse(args[1:]) != nil || *product == "" || flags.NArg() != 0 {
		fmt.Fprintln(diagnostic, "home paths/init requires --product-root ROOT [--user-home USER_HOME]")
		return 2
	}
	if *user == "" {
		var err error
		*user, err = os.UserHomeDir()
		if err != nil {
			fmt.Fprintln(diagnostic, "user home unavailable")
			return 2
		}
	}
	options := zipruntime.Options{UserHome: *user, LookupEnv: os.LookupEnv}
	paths := zipruntime.ResolvePaths(*product, options)
	if args[0] == "init" && paths.Mode == "initialization_required" {
		result, err := datahome.InitializeFresh(datahome.Options{UserHome: *user, ProductRoot: *product, LookupEnv: os.LookupEnv})
		if err != nil {
			paths = zipruntime.Paths{Mode: "blocked", Diagnostic: result.Diagnostic}
		} else {
			paths = zipruntime.ResolvePaths(*product, options)
		}
	} else if args[0] == "init" && paths.Mode == "legacy" {
		paths = zipruntime.Paths{Mode: "blocked", Diagnostic: "legacy-home-conflict"}
	}
	if field != "" {
		values := map[string]string{"home": paths.Home, "brain": paths.BrainRoot, "metadata": paths.MetadataRoot, "state": paths.StateRoot, "owner-scope": paths.OwnerScopeRoot, "mode": paths.Mode}
		value, ok := values[field]
		if !paths.Writable || !ok || value == "" || strings.ContainsAny(value, "\x00\r\n") {
			fmt.Fprintln(diagnostic, "requested HOME field unavailable")
			return 2
		}
		if _, err := fmt.Fprintln(out, value); err != nil {
			return 2
		}
	} else if json.NewEncoder(out).Encode(paths) != nil {
		return 2
	}
	if !paths.Writable {
		return 2
	}
	return 0
}
