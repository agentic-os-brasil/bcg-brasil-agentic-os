// Package product binds MCP tools to an admitted owner home and registered workspace.
package product

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/execution"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspace"
	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Options struct {
	ProductRoot, UserHome, WorkspacePath string
	LookupEnv                            func(string) (string, bool)
}
type Bound struct {
	options           Options
	home, workspaceID string
	store             execution.Store
}

func Open(o Options) (*Bound, error) {
	b := &Bound{options: o}
	if err := b.validate(); err != nil {
		return nil, err
	}
	return b, nil
}
func (b *Bound) validate() error {
	h, err := resolveBinding(b.options)
	if err != nil || !h.Writable {
		return errors.New("home_unavailable")
	}
	root := filepath.Join(h.Home, "state")
	w, err := workspace.Inspect(b.options.WorkspacePath, root)
	if err != nil || w.MetadataStatus != "valid" || w.State != "ready" {
		return errors.New("workspace_unavailable")
	}
	path, err := workspace.ResolveReference(w.WorkspaceID, root)
	if err != nil || path != b.options.WorkspacePath {
		return errors.New("workspace_unavailable")
	}
	if b.home != "" && (b.home != h.Home || b.workspaceID != w.WorkspaceID) {
		return errors.New("binding_changed")
	}
	s, err := execution.OpenConfinedStore(root)
	if err != nil {
		return errors.New("store_unavailable")
	}
	if b.home == "" {
		b.home, b.workspaceID, b.store = h.Home, w.WorkspaceID, s
	}
	return nil
}
func strSchema(max int) map[string]any {
	return map[string]any{"type": "string", "minLength": 1, "maxLength": max}
}
func variant(op string, fields map[string]any, required ...string) map[string]any {
	fields["operation"] = map[string]any{"type": "string", "const": op}
	return map[string]any{"type": "object", "properties": fields, "required": append([]string{"operation"}, required...), "additionalProperties": false}
}
func mutationSchema() map[string]any {
	var variants []any
	for _, op := range []string{"create", "start", "checkpoint", "pause", "resume", "evidence", "complete"} {
		f := map[string]any{}
		r := []string{}
		if op == "create" {
			f["objective"] = strSchema(4096)
			f["next_step"] = strSchema(768)
			f["artifacts"] = map[string]any{"type": "array", "minItems": 1, "maxItems": 8, "uniqueItems": true, "items": strSchema(256)}
			r = []string{"objective", "next_step", "artifacts"}
		} else {
			f["item_id"] = strSchema(96)
			f["expected_revision"] = map[string]any{"type": "integer", "minimum": 1}
			r = []string{"item_id", "expected_revision"}
			if op != "start" && op != "resume" {
				f["attempt_id"] = strSchema(96)
				r = append(r, "attempt_id")
			}
			if op == "checkpoint" {
				f["summary"] = strSchema(768)
				f["next_step"] = strSchema(768)
				r = append(r, "summary", "next_step")
			}
			if op == "evidence" {
				f["criterion_id"] = strSchema(96)
				r = append(r, "criterion_id")
			}
		}
		variants = append(variants, variant("execution."+op, f, r...))
	}
	return map[string]any{"type": "object", "oneOf": variants}
}
func (b *Bound) Server() *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{Name: "maestro-product", Version: "0.1.0"}, nil)
	readonly := &mcp.ToolAnnotations{ReadOnlyHint: true}
	empty := map[string]any{"type": "object", "properties": map[string]any{}, "additionalProperties": false}
	s.AddTool(&mcp.Tool{Name: "maestro_context", Description: "Read binding metadata and available operations; no content export.", InputSchema: empty, Annotations: readonly}, b.handle("context", empty))
	query := map[string]any{"type": "object", "oneOf": []any{
		variant("execution.list", map[string]any{"offset": map[string]any{"type": "integer", "minimum": 0, "maximum": 128}, "limit": map[string]any{"type": "integer", "minimum": 1, "maximum": 32}}),
		variant("execution.next", map[string]any{"item_id": strSchema(96)}, "item_id"),
		variant("execution.contract", map[string]any{"item_id": strSchema(96)}, "item_id"),
	}}
	s.AddTool(&mcp.Tool{Name: "maestro_query", Description: "Read paged execution metadata or the canonical bounded continuation.", InputSchema: query, Annotations: readonly}, b.handle("query", query))
	s.AddTool(&mcp.Tool{Name: "maestro_apply", Description: "Mutate bound execution with revision and attempt checks. Only artifact snapshot criteria are admitted.", InputSchema: mutationSchema()}, b.handle("apply", mutationSchema()))
	return s
}

type input struct {
	Operation        string   `json:"operation"`
	ItemID           string   `json:"item_id"`
	ExpectedRevision int      `json:"expected_revision"`
	AttemptID        string   `json:"attempt_id"`
	Objective        string   `json:"objective"`
	NextStep         string   `json:"next_step"`
	Summary          string   `json:"summary"`
	Artifacts        []string `json:"artifacts"`
	CriterionID      string   `json:"criterion_id"`
	Offset           int      `json:"offset"`
	Limit            int      `json:"limit"`
}

func result(value any, err error) *mcp.CallToolResult {
	if err != nil {
		code := "execution_rejected"
		switch {
		case errors.Is(err, execution.ErrRevisionConflict):
			code = "revision_conflict"
		case errors.Is(err, execution.ErrAttemptConflict):
			code = "attempt_conflict"
		case errors.Is(err, execution.ErrCompletionUnsatisfied):
			code = "completion_unsatisfied"
		case errors.Is(err, execution.ErrItemBusy):
			code = "item_busy"
		}
		value = map[string]string{"code": code}
	}
	body, _ := json.Marshal(value)
	return &mcp.CallToolResult{IsError: err != nil, Content: []mcp.Content{&mcp.TextContent{Text: string(body)}}}
}
func (b *Bound) handle(kind string, schema map[string]any) mcp.ToolHandler {
	body, _ := json.Marshal(schema)
	var definition jsonschema.Schema
	if err := json.Unmarshal(body, &definition); err != nil {
		panic(err)
	}
	resolved, err := definition.Resolve(nil)
	if err != nil {
		panic(err)
	}
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if len(req.Params.Arguments) > 16<<10 {
			return result(nil, errors.New("input_size")), nil
		}
		var raw any
		if err := json.Unmarshal(req.Params.Arguments, &raw); err != nil {
			return result(nil, err), nil
		}
		if err := resolved.Validate(raw); err != nil {
			return result(nil, errors.New("invalid_input")), nil
		}
		if err := b.validate(); err != nil {
			return result(nil, err), nil
		}
		var in input
		d := json.NewDecoder(bytes.NewReader(req.Params.Arguments))
		d.DisallowUnknownFields()
		if err := d.Decode(&in); err != nil {
			return result(nil, err), nil
		}
		if d.Decode(new(any)) != io.EOF {
			return result(nil, errors.New("trailing_input")), nil
		}
		if kind == "context" {
			return result(map[string]any{"workspace_id": b.workspaceID, "state": "ready", "evidence": "artifact_snapshot", "next_max_bytes": 2048}, nil), nil
		}
		if kind == "query" {
			if in.Operation == "execution.contract" {
				item, err := b.store.Inspect(b.workspaceID, in.ItemID)
				if err != nil {
					return result(nil, err), nil
				}
				artifacts, err := admittedArtifacts(item.Contract)
				return result(map[string]any{"item_id": in.ItemID, "contract_version": item.Contract.ContractVersion, "criteria": artifacts}, err), nil
			}
			if in.Operation == "execution.next" {
				out, err := b.store.Next(b.workspaceID, in.ItemID)
				return result(out, err), nil
			}
			if in.Operation != "execution.list" {
				return result(nil, errors.New("invalid_operation")), nil
			}
			items, err := b.store.List(b.workspaceID)
			if err != nil {
				return result(nil, err), nil
			}
			limit := in.Limit
			if limit == 0 {
				limit = 16
			}
			if in.Offset < 0 || in.Offset > len(items) || limit < 1 || limit > 32 {
				return result(nil, errors.New("invalid_page")), nil
			}
			end := min(in.Offset+limit, len(items))
			return result(map[string]any{"items": items[in.Offset:end], "next_offset": end, "has_more": end < len(items)}, nil), nil
		}
		if in.Operation != "execution.create" {
			contract, err := b.store.Inspect(b.workspaceID, in.ItemID)
			if err != nil {
				return result(nil, err), nil
			}
			if _, err := admittedArtifacts(contract.Contract); err != nil {
				return result(nil, err), nil
			}
		}
		var item execution.Item
		var err error
		switch in.Operation {
		case "execution.create":
			var criteria []execution.Criterion
			var refs []string
			for i, path := range in.Artifacts {
				if err := artifactPath(path); err != nil {
					return result(nil, err), nil
				}
				ref := "bcgos://workspace/" + path
				refs = append(refs, ref)
				criteria = append(criteria, execution.Criterion{ID: fmtCriterion(i), Type: execution.CriterionArtifactSnapshot, TargetRef: ref})
			}
			item, err = b.store.Create(execution.CreateInput{WorkspaceID: b.workspaceID, Objective: in.Objective, InitialNextStep: in.NextStep, Criteria: criteria, AllowedRefs: refs})
		case "execution.start":
			item, err = b.store.Start(b.workspaceID, in.ItemID, in.ExpectedRevision)
		case "execution.resume":
			item, err = b.store.Resume(b.workspaceID, in.ItemID, in.ExpectedRevision)
		case "execution.pause":
			item, err = b.store.Pause(b.workspaceID, in.ItemID, in.ExpectedRevision, in.AttemptID)
		case "execution.checkpoint":
			item, err = b.store.Checkpoint(b.workspaceID, in.ItemID, execution.CheckpointInput{ExpectedRevision: in.ExpectedRevision, AttemptID: in.AttemptID, Summary: in.Summary, NextStep: in.NextStep})
		case "execution.evidence":
			out, e := b.store.CollectEvidence(b.workspaceID, in.ItemID, execution.EvidenceInput{WorkspaceRoot: b.options.WorkspacePath, ExpectedRevision: in.ExpectedRevision, AttemptID: in.AttemptID, CriterionID: in.CriterionID})
			return result(execution.EvidenceMutationReceipt(out), e), nil
		case "execution.complete":
			item, err = b.store.Complete(b.workspaceID, in.ItemID, execution.CompletionInput{WorkspaceRoot: b.options.WorkspacePath, ExpectedRevision: in.ExpectedRevision, AttemptID: in.AttemptID})
		default:
			err = errors.New("invalid_operation")
		}
		return result(execution.Receipt(item), err), nil
	}
}
func overlaps(a, b string) bool {
	a, b = strings.ToLower(a), strings.ToLower(b)
	for _, pair := range [][2]string{{a, b}, {b, a}} {
		rel, err := filepath.Rel(pair[0], pair[1])
		if err == nil && (rel == "." || rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))) {
			return true
		}
	}
	return false
}
func fmtCriterion(i int) string { return "artifact_" + strconv.Itoa(i+1) }
