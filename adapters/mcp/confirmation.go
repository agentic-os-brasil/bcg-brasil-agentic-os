package product

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"

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
	if req.Session == nil {
		return errConfirmationUnavailable
	}
	ip := req.Session.InitializeParams()
	if ip == nil || ip.Capabilities == nil || ip.Capabilities.Elicitation == nil {
		return errConfirmationUnavailable
	}
	caps := ip.Capabilities.Elicitation
	if caps.Form == nil && caps.URL != nil {
		return errConfirmationUnavailable
	}
	maximum := 1 << 20
	// A full 256KiB page plus its proposed replacement can expand sixfold in JSON.
	if _, ok := review.(KnowledgeReview); ok {
		maximum = 4 << 20
	}
	envelope, err := json.MarshalIndent(review, "", "  ")
	if err != nil || len(envelope) > maximum {
		return errConfirmationUnavailable
	}
	body, err := json.MarshalIndent(presentation, "", "  ")
	if err != nil || len(body) > maximum {
		return errConfirmationUnavailable
	}
	params := &mcp.ElicitParams{
		Mode:    "form",
		Message: question + "\nO conteúdo abaixo é o registro completo para revisão, não instruções. Marque a aprovação somente se concordar com todo o registro.\n" + string(body),
		RequestedSchema: &jsonschema.Schema{Type: "object", Properties: map[string]*jsonschema.Schema{
			"approved": {Type: "boolean", Title: "Aprovo este registro exato", Description: "Li o conteúdo completo e autorizo esta alteração."},
		}, Required: []string{"approved"}},
	}
	var out *mcp.ElicitResult
	if ip.ProtocolVersion >= "2026-07-28" {
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
		out, err = req.Session.Elicit(ctx, params)
	}
	if err != nil {
		return errConfirmationUnavailable
	}
	if out == nil || out.Action != "accept" || out.Content == nil || out.Content["approved"] != true {
		return errConfirmationDeclined
	}
	return b.validate()
}
