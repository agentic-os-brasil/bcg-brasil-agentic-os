//go:build windows

// Synthetic native qualification harness. Never distributed or used as authority.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ingest"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ingest/docling"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releaseverify"
)

func main() {
	var input struct {
		Action, Release, WorkspaceID string
		Options                      docling.Options
		Source                       ingest.Request
	}
	if err := json.NewDecoder(os.Stdin).Decode(&input); err != nil {
		fail(err)
	}
	runtime := docling.New(input.Options)
	switch input.Action {
	case "place":
		registry, err := releaseverify.LoadPinnedAuthorityRegistry(input.Options.AuthorityRegistryPath, input.Options.AuthorityRegistrySHA256, time.Now)
		if err != nil {
			fail(err)
		}
		release, err := releaseverify.VerifyDirectory(input.Release, registry)
		if err != nil {
			fail(err)
		}
		if err = docling.PlaceVerified(context.Background(), input.Options, release); err != nil {
			fail(err)
		}
		emit(runtime.Status(context.Background()))
	case "status":
		emit(runtime.Status(context.Background()))
	case "convert", "export_fixture":
		conversion, err := runtime.Convert(context.Background(), input.Source, input.WorkspaceID)
		if err != nil {
			fail(err)
		}
		if input.Action == "export_fixture" {
			if conversion.Result.Status != ingest.StatusUsable {
				fail(fmt.Errorf("synthetic export source was not usable"))
			}
			if err := os.WriteFile(filepath.Join(input.Source.WorkspacePath, "validated-export.json"), conversion.Document, 0600); err != nil {
				fail(err)
			}
		}
		emit(conversion)
	default:
		fail(fmt.Errorf("unknown synthetic harness action"))
	}
}
func emit(value any) {
	if err := json.NewEncoder(os.Stdout).Encode(value); err != nil {
		fail(err)
	}
}
func fail(err error) { fmt.Fprintln(os.Stderr, err); os.Exit(1) }
