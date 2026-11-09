package product

import (
	"context"
	"sort"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ingest"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ingest/docling"
)

type contentFormatAlternative struct {
	ingest.FormatRoute
	Condition string `json:"condition"`
}

type contentFormatCoverage struct {
	ingest.FormatRoute
	Extension    string                     `json:"extension"`
	State        string                     `json:"state"`
	Reason       string                     `json:"reason,omitempty"`
	Alternatives []contentFormatAlternative `json:"alternatives,omitempty"`
}

type contentAvailability struct {
	docling.Availability
	Formats []contentFormatCoverage `json:"formats"`
}

func (b *Bound) contentStatus(ctx context.Context) contentAvailability {
	// Ready authenticates the exact embedded bridge, lock, complete parser inputs
	// and installed inventory. Policy alone is never installed availability.
	availability := b.DoclingRuntime().Status(ctx)
	out := contentAvailability{Availability: availability}
	var extensions []string
	for extension, admitted := range ingest.DefaultPolicy().AllowedExts {
		if admitted {
			extensions = append(extensions, extension)
		}
	}
	sort.Strings(extensions)
	for _, extension := range extensions {
		route, err := ingest.SelectFormat(extension, false)
		coverage := contentFormatCoverage{FormatRoute: route, Extension: extension, State: availability.State, Reason: availability.Reason}
		if err != nil {
			coverage.State, coverage.Reason = "unavailable", "format_unsupported"
		}
		if extension == ".json" {
			export, err := ingest.SelectFormat(extension, true)
			if err == nil {
				coverage.Alternatives = []contentFormatAlternative{{FormatRoute: export, Condition: "validated_docling_document"}}
			}
		}
		out.Formats = append(out.Formats, coverage)
	}
	return out
}
