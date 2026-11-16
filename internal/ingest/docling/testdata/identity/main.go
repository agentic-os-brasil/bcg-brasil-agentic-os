//go:build windows

// Synthetic module-compatibility helper; never distributed or used for authority.
package main

import (
	"fmt"
	runtimeidentity "github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal"
	_ "github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ingest/docling"
	"os"
)

func main() {
	identity, err := runtimeidentity.Docling()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println(identity)
}
