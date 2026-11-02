// maestro-docling-pack assembles already acquired, pinned official inputs offline.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ingest/docling"
	"os"
)

func main() {
	inputs := flag.String("inputs", "", "directory containing the exact pinned Python archive and wheels")
	destination := flag.String("destination", "", "new unsigned pack directory")
	flag.Parse()
	if flag.NArg() != 0 || *inputs == "" || *destination == "" {
		fmt.Fprintln(os.Stderr, "inputs and a new destination are required")
		os.Exit(2)
	}
	result, err := docling.Assemble(*inputs, *destination)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	_ = json.NewEncoder(os.Stdout).Encode(result)
}
