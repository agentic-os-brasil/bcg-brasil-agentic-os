// Synthetic native qualification harness. Never distributed or used as authority.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync/atomic"
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
		var failure placementFailureProjection
		slog.SetDefault(slog.New(&failure))
		registry, err := releaseverify.LoadPinnedAuthorityRegistry(input.Options.AuthorityRegistryPath, input.Options.AuthorityRegistrySHA256, time.Now)
		if err != nil {
			fail("synthetic_registry_unavailable")
		}
		release, err := releaseverify.VerifyDirectory(input.Release, registry)
		if err != nil {
			fail("synthetic_release_unverified")
		}
		if err = docling.PlaceVerified(context.Background(), input.Options, release); err != nil {
			fail(failure.message())
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

const placementLaunchAccessDenied = "MAESTRO_NATIVE_RUNTIME_FAILURE operation=place stage=launch process_status=0xc0000022"

// This fixture observes only the original qualification record's typed fields.
// It never renders log records, child output, paths, errors or document bodies.
type placementFailureProjection struct{ state atomic.Uint32 }

func (*placementFailureProjection) Enabled(context.Context, slog.Level) bool { return true }
func (p *placementFailureProjection) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(attrs) != 0 {
		p.state.Store(2) // Only the original direct record is understood.
	}
	return p
}
func (p *placementFailureProjection) WithGroup(name string) slog.Handler {
	if name != "" {
		p.state.Store(2)
	}
	return p
}
func (p *placementFailureProjection) Handle(_ context.Context, record slog.Record) error {
	if record.Message != "native converter qualification failed" {
		return nil
	}
	stage, status := "", ""
	stageCount, statusCount := 0, 0
	record.Attrs(func(attr slog.Attr) bool {
		if attr.Key == "stage" {
			stageCount++
			if attr.Value.Kind() == slog.KindString {
				stage = attr.Value.String()
			}
		}
		if attr.Key == "process_status" {
			statusCount++
			if attr.Value.Kind() == slog.KindString {
				status = attr.Value.String()
			}
		}
		return true
	})
	if stageCount != 1 || statusCount != 1 || stage != "launch" || len(status) > 256 {
		p.state.Store(2)
		return nil
	}
	var pid, flags uint32
	const format = "confined converter exit 3221225506 (status=0xc0000022 pid=%d creation_flags=0x%08x)"
	count, err := fmt.Sscanf(status, "confined converter exit 3221225506 (status=0xc0000022 pid=%d creation_flags=0x%x)", &pid, &flags)
	if err != nil || count != 2 || pid == 0 || status != fmt.Sprintf(format, pid, flags) || !p.state.CompareAndSwap(0, 1) {
		p.state.Store(2)
	}
	return nil
}
func (p *placementFailureProjection) message() string {
	if p.state.Load() == 1 {
		return placementLaunchAccessDenied
	}
	return "synthetic_place_failed"
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
