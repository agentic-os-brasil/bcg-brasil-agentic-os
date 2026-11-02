// Package docling runs the authenticated, offline DOCX runtime. It does not
// acquire dependencies or persist document artifacts.
package docling

import (
	"context"
	"encoding/json"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ingest"
)

type Options struct {
	DataRoot                string
	AuthorityRegistryPath   string
	AuthorityRegistrySHA256 string
	CLIVersion              string
}
type Runtime struct{ options Options }
type Availability struct {
	State      string `json:"state"`
	Reason     string `json:"reason,omitempty"`
	PackSHA256 string `json:"pack_sha256,omitempty"`
}
type Provenance struct {
	PackSHA256     string `json:"pack_sha256"`
	ManifestSHA256 string `json:"manifest_sha256"`
	MarkdownSHA256 string `json:"markdown_sha256"`
	DocumentSHA256 string `json:"document_sha256"`
	LockSHA256     string `json:"lock_sha256"`
	Python         string `json:"python"`
	Docling        string `json:"docling"`
	NetworkDenied  bool   `json:"network_denied"`
}
type Conversion struct {
	Result     ingest.Result   `json:"result"`
	Markdown   []byte          `json:"-"`
	Document   json.RawMessage `json:"-"`
	Provenance Provenance      `json:"provenance"`
}

func New(options Options) *Runtime { return &Runtime{options: options} }
func (r *Runtime) Status(ctx context.Context) Availability {
	pack, reason := r.verify(ctx)
	if reason != "" {
		return Availability{State: "unavailable", Reason: reason}
	}
	return Availability{State: "ready", PackSHA256: pack.sha}
}
