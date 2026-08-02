package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/dev/releasepack"
)

func icons(root string, args []string) {
	flags := flag.NewFlagSet("icons", flag.ExitOnError)
	source := flags.String("source", "installers/wizard/assets/maestro-app-icon.svg", "canonical Maestro SVG icon")
	output := flags.String("output", "", "directory for generated native icon assets")
	_ = flags.Parse(args)
	if *output == "" || flags.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "usage: go run ./dev/release icons --source FILE --output DIRECTORY")
		os.Exit(2)
	}
	result, err := releasepack.BuildNativeIcons(
		context.Background(),
		absoluteFromRoot(root, *source),
		absoluteFromRoot(root, *output),
	)
	if err != nil {
		fatal(err)
	}
	result.Source = filepath.Clean(result.Source)
	result.ICNS = filepath.Clean(result.ICNS)
	result.ICO = filepath.Clean(result.ICO)
	result.Manifest = filepath.Clean(result.Manifest)
	if err := json.NewEncoder(os.Stdout).Encode(result); err != nil {
		fatal(err)
	}
}
