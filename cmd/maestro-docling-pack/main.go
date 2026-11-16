// maestro-docling-pack assembles already acquired, pinned official inputs offline.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ingest/docling"
	"os"
	"strings"
)

func main() {
	inputs := flag.String("inputs", "", "directory containing the exact pinned interpreter and parser distributions")
	destination := flag.String("destination", "", "new unsigned pack directory")
	target := flag.String("target", "darwin/arm64", "pinned runtime target: darwin/arm64 or windows/amd64")
	profile := flag.String("profile", docling.LegacyProfile, "closed runtime profile: policy-formats-v1 or model-free-documents-v1")
	flag.Parse()
	if flag.NArg() != 0 || *inputs == "" || *destination == "" {
		fmt.Fprintln(os.Stderr, "inputs and a new destination are required")
		os.Exit(2)
	}
	platform := strings.Split(*target, "/")
	if len(platform) != 2 {
		fmt.Fprintln(os.Stderr, "target must be OS/architecture")
		os.Exit(2)
	}
	result, err := docling.AssembleProfile(*inputs, *destination, platform[0], platform[1], *profile)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	_ = json.NewEncoder(os.Stdout).Encode(result)
}
