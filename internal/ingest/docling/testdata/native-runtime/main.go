// Synthetic native qualification harness. Never distributed or used as authority.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
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
		fail("synthetic_input_invalid")
	}
	runtime := docling.New(input.Options)
	switch input.Action {
	case "place":
		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug})))
		registry, err := releaseverify.LoadPinnedAuthorityRegistry(input.Options.AuthorityRegistryPath, input.Options.AuthorityRegistrySHA256, time.Now)
		if err != nil {
			fail("synthetic_registry_unavailable")
		}
		release, err := releaseverify.VerifyDirectory(input.Release, registry)
		if err != nil {
			fail("synthetic_release_unverified")
		}
		if err = docling.PlaceVerified(context.Background(), input.Options, release); err != nil {
			fail("synthetic_place_failed")
		}
		emit(runtime.Status(context.Background()))
	case "status":
		emit(runtime.Status(context.Background()))
	case "convert", "export_fixture":
		conversion, err := runtime.Convert(context.Background(), input.Source, input.WorkspaceID)
		if err != nil {
			fail("synthetic_conversion_failed")
		}
		if input.Action == "export_fixture" {
			if conversion.Result.Status != ingest.StatusUsable {
				fail("synthetic_export_source_unusable")
			}
			if err := os.WriteFile(filepath.Join(input.Source.WorkspacePath, "validated-export.json"), conversion.Document, 0600); err != nil {
				fail("synthetic_export_write_failed")
			}
		}
		emit(conversion)
	default:
		fail("synthetic_action_unknown")
	}
}

// Fixture-only transport. Production Conversion intentionally keeps bodies out
// of JSON; both slices below preserve the exact synthetic bytes in this private
// child-parent pipe, including original JSON whitespace and escaping.
type conversionWire struct {
	Metadata docling.Conversion `json:"metadata"`
	Markdown []byte             `json:"markdown"`
	Document []byte             `json:"document"`
}

func emit(value any) {
	if conversion, ok := value.(docling.Conversion); ok {
		value = conversionWire{conversion, conversion.Markdown, []byte(conversion.Document)}
	}
	if err := json.NewEncoder(os.Stdout).Encode(value); err != nil {
		fail("synthetic_wire_encode_failed")
	}
}
func fail(class string) { fmt.Fprintln(os.Stderr, class); os.Exit(1) }
