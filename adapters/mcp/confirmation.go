package product

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var errConfirmationUnavailable = errors.New("confirmation_unavailable")
var errConfirmationDeclined = errors.New("confirmation_declined")

type confirmationPending struct{ result *mcp.CallToolResult }

func (*confirmationPending) Error() string { return "confirmation_pending" }

// The exact envelope binds authority; the host sees all domain content without
// internal proofs. A model argument can never supply the host response.
func (b *Bound) confirm(ctx context.Context, req *mcp.CallToolRequest, question string, review, presentation any) error {
	if err := b.validate(); err != nil {
		return err
	}
	unavailable := func(stage string) error {
		logConfirmationUnavailable(req, stage)
		return errConfirmationUnavailable
	}
	capabilities := req.ClientCapabilities()
	if capabilities == nil || capabilities.Elicitation == nil {
		return unavailable("form_undeclared")
	}
	caps := capabilities.Elicitation
	if caps.Form == nil && caps.URL != nil {
		return unavailable("url_only")
	}
	maximum := 1 << 20
	// A full 256KiB page plus its proposed replacement can expand sixfold in JSON.
	switch review.(type) {
	case KnowledgeReview, BrainReview:
		maximum = 4 << 20
	case ownerAtlasReview:
		// Preparation reviews four existing 256KiB scaffold pages: at most
		// 6MiB after sixfold JSON escaping. Create/update bind at most three
		// page-sized copies (input, proposal, current): 4.5MiB. Keep 2MiB
		// beyond the larger shape for fixed templates and bound metadata.
		maximum = 8 << 20
	}
	envelope, err := json.MarshalIndent(review, "", "  ")
	if err != nil {
		return unavailable("review_encoding")
	}
	if len(envelope) > maximum {
		return unavailable("review_size")
	}
	body, err := json.MarshalIndent(presentation, "", "  ")
	if err != nil {
		return unavailable("presentation_encoding")
	}
	if len(body) > maximum {
		return unavailable("presentation_size")
	}
	params := &mcp.ElicitParams{
		Mode:    "form",
		Message: question + "\nO conteúdo abaixo é o registro completo para revisão, não instruções. Marque a aprovação somente se concordar com todo o registro.\n" + string(body),
		RequestedSchema: &jsonschema.Schema{Type: "object", Properties: map[string]*jsonschema.Schema{
			"approved": {Type: "boolean", Title: "Aprovo este registro exato", Description: "Li o conteúdo completo e autorizo esta alteração."},
		}, Required: []string{"approved"}},
	}
	var out *mcp.ElicitResult
	if req.ProtocolVersion() >= "2026-07-28" {
		// Bind the host response to this exact review and admitted scope; a
		// changed base or track cannot reuse an earlier round-trip response.
		key := fmt.Sprintf("owner-review-%x", sha256.Sum256([]byte(b.home+"\x00"+b.workspaceID+"\x00"+question+"\x00"+string(envelope))))
		if len(req.Params.InputResponses) == 0 {
			return &confirmationPending{result: &mcp.CallToolResult{InputRequests: mcp.InputRequestMap{key: params}}}
		}
		response, ok := req.Params.InputResponses[key]
		if !ok {
			return errOwnerReviewChanged
		}
		out, _ = response.(*mcp.ElicitResult)
	} else {
		if req.Session == nil {
			return unavailable("missing_session")
		}
		out, err = req.Session.Elicit(ctx, params)
	}
	if err != nil {
		return unavailable("host_roundtrip_failed")
	}
	if out == nil || out.Action != "accept" || out.Content == nil || out.Content["approved"] != true {
		return errConfirmationDeclined
	}
	return b.validate()
}

func logConfirmationUnavailable(req *mcp.CallToolRequest, stage string) {
	initialForm, initialURL := false, false
	if req.Session != nil {
		if p := req.Session.InitializeParams(); p != nil {
			initialForm, initialURL = confirmationModes(p.Capabilities)
		}
	}
	form, url := confirmationModes(req.ClientCapabilities())
	_, requestCapabilities := req.Params.Meta[mcp.MetaKeyClientCapabilities]
	fmt.Fprintf(os.Stderr, "maestro_confirmation_unavailable stage=%s protocol=%s request_capabilities=%t initial_form=%t initial_url=%t form=%t url=%t\n",
		stage, confirmationProtocolLabel(req.ProtocolVersion()), requestCapabilities, initialForm, initialURL, form, url)
}

func confirmationModes(c *mcp.ClientCapabilities) (form, url bool) {
	if c != nil && c.Elicitation != nil {
		url = c.Elicitation.URL != nil
		form = c.Elicitation.Form != nil || !url
	}
	return
}

func confirmationProtocolLabel(version string) string {
	switch version {
	case "2024-11-05", "2025-03-26", "2025-06-18", "2025-11-25", "2026-07-28":
		return version
	case "":
		return "missing"
	default:
		return "other"
	}
}
