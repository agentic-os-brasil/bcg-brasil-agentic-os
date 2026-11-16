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
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/execution"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/maintenance"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/processidentity"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspace"
	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Options struct {
	ProductRoot, UserHome, WorkspacePath string
	LookupEnv                            func(string) (string, bool)
	// These constructor inputs come only from the native binary seed, never tool arguments.
	AuthorityRegistryPath, AuthorityRegistrySHA256, CLIVersion string
	// Fixed carrier inputs come from canonical packaging, never MCP arguments.
	InstalledManifestSHA256, InstalledClientInputsSHA256 string
	// Explicit operator startup selection; never admitted from tool arguments.
	OwnerReviewChannel string
	GuardProcess       bool
}
type Bound struct {
	options           Options
	home, workspaceID string
	store             execution.Store
	// Deterministic test seams; neither is exposed as runtime configuration or tool authority.
	afterBrainPrepareEffect    func() error
	observeWorkspaceTransition func(stage, page string)
	localOwnerReview           func(context.Context, string, any) (bool, error)
	maintenanceClock           func() time.Time
	maintenanceActivity        func(context.Context) (maintenance.IdleState, error)
}

func Open(o Options) (*Bound, error) {
	if o.OwnerReviewChannel != "" && o.OwnerReviewChannel != "local-browser" {
		return nil, errors.New("owner_review_channel_unavailable")
	}
	b := &Bound{options: o}
	if err := b.validate(); err != nil {
		return nil, err
	}
	return b, nil
}
func (b *Bound) validate() error {
	if _, err := processidentity.Current(); err != nil {
		return err
	}
	if err := b.validateInstalledSource(); err != nil {
		return err
	}
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
	variants = append(variants, agentIdentityMutations()...)
	return map[string]any{"type": "object", "oneOf": variants}
}

// toolPresentation keeps discovery in ordinary object properties because some
// MCP hosts flatten a top-level oneOf into its first branch. The authoritative
// closed variants still validate every request in handle; this projection never
// supplies admission or replaces their per-operation requirements.
func toolPresentation(authoritative map[string]any) (map[string]any, string) {
	properties := map[string]any{}
	requiredFor := map[string][]string{}
	allowedFor := map[string][]string{}
	constraints := map[string][]string{}
	var operations []string
	for _, entry := range authoritative["oneOf"].([]any) {
		v := entry.(map[string]any)
		fields := v["properties"].(map[string]any)
		op := fields["operation"].(map[string]any)["const"].(string)
		operations = append(operations, op)
		required := map[string]bool{}
		for _, name := range v["required"].([]string) {
			required[name] = true
		}
		for name, definition := range fields {
			if name == "operation" {
				continue
			}
			allowedFor[name] = append(allowedFor[name], op)
			if required[name] {
				requiredFor[name] = append(requiredFor[name], op)
			}
			encoded, _ := json.Marshal(definition)
			constraints[name] = append(constraints[name], op+": "+string(encoded))
			candidate := definition.(map[string]any)
			if current, exists := properties[name]; exists {
				// Shared metadata must not import a different branch's tighter bound.
				common := current.(map[string]any)
				for key, value := range common {
					a, _ := json.Marshal(value)
					b, present := candidate[key]
					encoded, _ := json.Marshal(b)
					if !present || !bytes.Equal(a, encoded) {
						delete(common, key)
					}
				}
			} else {
				copy := map[string]any{}
				for key, value := range candidate {
					copy[key] = value
				}
				properties[name] = copy
			}
		}
	}
	for name, definition := range properties {
		// Hosts may truncate the tool description. Keep each field's complete
		// contract beside that field instead of one duplicated, lossy tool blob.
		definition.(map[string]any)["description"] = "Required for: " + strings.Join(requiredFor[name], ", ") + "\nAllowed for: " + strings.Join(allowedFor[name], ", ") + "\nExact constraints by operation:\n" + strings.Join(constraints[name], "\n")
	}
	properties["operation"] = map[string]any{"type": "string", "enum": operations, "description": "Select one listed operation. Each other field states exactly which operations require or allow it, and its exact per-operation constraints. Omit every field not allowed for the selected operation."}
	return map[string]any{"type": "object", "properties": properties, "required": []string{"operation"}, "additionalProperties": false}, " Read each field's Required for, Allowed for and exact constraint lines before calling. Include every required field and only fields allowed for the selected operation."
}

func (b *Bound) Server() *mcp.Server {
	version := b.options.CLIVersion
	if version == "" {
		version = "unavailable"
	}
	s := mcp.NewServer(&mcp.Implementation{Name: "maestro-product", Version: version}, nil)
	s.AddReceivingMiddleware(func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, request mcp.Request) (mcp.Result, error) {
			switch method {
			case "tools/list", "resources/list", "prompts/list":
				if err := b.validate(); err != nil {
					return nil, errors.New("product_restart_or_source_recovery_required")
				}
			}
			return next(ctx, method, request)
		}
	})
	readonly := &mcp.ToolAnnotations{ReadOnlyHint: true}
	empty := map[string]any{"type": "object", "properties": map[string]any{}, "additionalProperties": false}
	s.AddTool(&mcp.Tool{Name: "maestro_context", Description: "Read admitted binding metadata, canonical display identities and memory capability hints; inspect maestro_query and maestro_apply schemas for operations. No document or facet export.", InputSchema: empty, Annotations: readonly}, b.handle("context", empty))
	query := productQuerySchema()
	queryPresentation, queryContracts := toolPresentation(query)
	apply := productApplySchema()
	applyPresentation, applyContracts := toolPresentation(apply)
	s.AddTool(&mcp.Tool{Name: "maestro_query", Description: "Read immutable methods and requirements, accepted session-readable owner context, owner metadata/questions, execution continuation, eligible memory, authored pages, converted content, named workspace knowledge, maintenance status or private wiki through bounded pages. Full private owner review and reader-restricted text remain protected." + queryContracts, InputSchema: queryPresentation, Annotations: readonly}, b.handle("query", query))
	s.AddTool(&mcp.Tool{Name: "maestro_apply", Description: "Author reviewed workspace pages, convert documents, apply reviewed knowledge/owner/memory changes, qualify and enroll exact maintenance jobs, or run active due work. Maintenance lifecycle changes require trusted exact review; execution uses revision/attempt and artifact checks." + applyContracts, InputSchema: applyPresentation}, b.handle("apply", apply))
	b.registerMethods(s)
	return s
}

func productQuerySchema() map[string]any {
	variants := []any{
		variant("execution.list", map[string]any{"offset": map[string]any{"type": "integer", "minimum": 0, "maximum": 128}, "limit": map[string]any{"type": "integer", "minimum": 1, "maximum": 32}}),
		variant("execution.next", map[string]any{"item_id": strSchema(96)}, "item_id"),
		variant("execution.contract", map[string]any{"item_id": strSchema(96)}, "item_id"),
	}
	for _, group := range [][]any{dailyQueries(), caseSetupQueries(), ownerQueries(), agentIdentityQueries(), memoryQueries(), methodsQueries(), contentQueries(), brainQueries(), maintenanceQueries(), wikiQueries()} {
		variants = append(variants, group...)
	}
	return map[string]any{"type": "object", "oneOf": variants}
}

type input struct {
	DailyScope    string   `json:"daily_scope"`
	CheckpointID  string   `json:"checkpoint_id"`
	SessionID     string   `json:"session_id"`
	CapturedAt    string   `json:"captured_at"`
	LocalDate     string   `json:"local_date"`
	Decisions     []string `json:"decisions"`
	NextActions   []string `json:"next_actions"`
	CheckpointIDs []string `json:"checkpoint_ids"`

	Mode                       string   `json:"mode"`
	ProjectName                string   `json:"project_name"`
	AccountID                  string   `json:"account_id"`
	AccountOwner               string   `json:"account_owner"`
	AccountMandate             string   `json:"account_mandate"`
	ExpectedSetupRevision      string   `json:"expected_setup_revision"`
	IdentityOwnerID            string   `json:"owner_id"`
	IdentityRole               string   `json:"role"`
	IdentityAgentID            string   `json:"agent_id"`
	IdentityDisplayName        string   `json:"display_name"`
	IdentityEmoji              string   `json:"emoji"`
	IdentityQuestionID         string   `json:"question_id"`
	IdentityExpectedRevision   string   `json:"expected_profile_revision"`
	Generation                 string   `json:"generation"`
	Reference                  string   `json:"reference"`
	JobID                      string   `json:"job_id"`
	Timezone                   string   `json:"timezone"`
	MethodID                   string   `json:"method_id"`
	ResourceURI                string   `json:"resource_uri"`
	SourcePath                 string   `json:"source_path"`
	ArtifactID                 string   `json:"artifact_id"`
	Format                     string   `json:"format"`
	Page                       string   `json:"page"`
	Section                    string   `json:"section"`
	Field                      string   `json:"field"`
	Value                      string   `json:"value"`
	Target                     string   `json:"target"`
	Label                      string   `json:"label"`
	ExpectedPageRevision       string   `json:"expected_page_revision"`
	Purpose                    string   `json:"purpose"`
	Pages                      []string `json:"pages"`
	Text                       string   `json:"text"`
	CaptureID                  string   `json:"capture_id"`
	Query                      string   `json:"query"`
	Cycle                      string   `json:"cycle"`
	Date                       string   `json:"date"`
	Cutoff                     string   `json:"cutoff"`
	ExpectedVisibilityRevision uint64   `json:"expected_visibility_revision"`
	MaxBytes                   int      `json:"max_bytes"`
	Operation                  string   `json:"operation"`
	ItemID                     string   `json:"item_id"`
	ExpectedRevision           int      `json:"expected_revision"`
	AttemptID                  string   `json:"attempt_id"`
	Objective                  string   `json:"objective"`
	NextStep                   string   `json:"next_step"`
	Summary                    string   `json:"summary"`
	Artifacts                  []string `json:"artifacts"`
	CriterionID                string   `json:"criterion_id"`
	Offset                     int      `json:"offset"`
	Limit                      int      `json:"limit"`
	Track                      string   `json:"track"`
	ProposalID                 string   `json:"proposal_id"`
	Facet                      string   `json:"facet"`
	Evidence                   string   `json:"evidence"`
	ProposedBody               string   `json:"proposed_body"`
}

func result(value any, err error) *mcp.CallToolResult {
	if err != nil {
		code := "execution_rejected"
		switch {
		case errors.Is(err, processidentity.ErrChanged):
			code = "restart_required"
		case errors.Is(err, ErrSourceRecoveryRequired):
			code = "source_recovery_required"
		case errors.Is(err, execution.ErrRevisionConflict):
			code = "revision_conflict"
		case errors.Is(err, execution.ErrAttemptConflict):
			code = "attempt_conflict"
		case errors.Is(err, execution.ErrCompletionUnsatisfied):
			code = "completion_unsatisfied"
		case errors.Is(err, execution.ErrItemBusy):
			code = "item_busy"
		}
		metadata := map[string]string{"code": code}
		if code == "source_recovery_required" {
			metadata["next_action"] = "Use the independently admitted local maestro-install source-review/source-check recovery route before restarting."
		}
		value = metadata
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
		if len(req.Params.Arguments) > 2<<20 {
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
		if len(req.Params.Arguments) > 128<<10 && in.Operation != "knowledge.create" && in.Operation != "brain.create" && in.Operation != "brain.update" && in.Operation != "owner.atlas.create" && in.Operation != "owner.atlas.update" {
			return result(nil, errors.New("input_size")), nil
		}
		if len(req.Params.Arguments) > 16<<10 && in.Operation != "daily.enqueue" && in.Operation != "owner.propose" && in.Operation != "memory.capture" && in.Operation != "memory.correct" && in.Operation != "knowledge.create" && in.Operation != "knowledge.append" && in.Operation != "brain.append" && in.Operation != "brain.create" && in.Operation != "brain.update" && in.Operation != "owner.atlas.create" && in.Operation != "owner.atlas.update" && in.Operation != "owner.atlas.append" && in.Operation != "owner.atlas.collect" {
			return result(nil, errors.New("input_size")), nil
		}
		if kind == "context" {
			return result(map[string]any{"workspace_id": b.workspaceID, "state": "ready", "evidence": "artifact_snapshot", "agent_identity": b.managedIdentityContext(), "next_max_bytes": 2048, "owner": map[string]any{"context_operation": "owner.context", "reader": "session", "max_page_bytes": 4096, "requires": "current_confirmed_onboarding", "authority": "owner_profile_data_only"}, "memory": map[string]any{"context_operation": "memory.context", "max_page_bytes": 4096, "synthesis": "local_deterministic", "lifetime": "two_l3_generations_and_exact_owner_review"}, "methods": map[string]any{"list_operation": "methods.list", "read_operation": "methods.read", "plan_operation": "methods.plan", "max_page_bytes": 4096, "mode": "methods_not_authority"}}, nil), nil
		}
		if strings.HasPrefix(in.Operation, "daily.") {
			return b.handleDaily(ctx, req), nil
		}
		if strings.HasPrefix(in.Operation, "case.setup.") {
			return b.handleCaseSetup(ctx, req, in), nil
		}
		if strings.HasPrefix(in.Operation, "methods.") {
			return b.handleMethods(in), nil
		}
		if strings.HasPrefix(in.Operation, "brain.") {
			return b.handleBrain(ctx, req, in), nil
		}
		if strings.HasPrefix(in.Operation, "maintenance.") {
			return b.handleMaintenance(ctx, req, in), nil
		}
		if strings.HasPrefix(in.Operation, "wiki.") {
			return b.handleWiki(in), nil
		}
		if strings.HasPrefix(in.Operation, "content.") || strings.HasPrefix(in.Operation, "knowledge.") {
			return b.handleContent(ctx, req, in), nil
		}
		if strings.HasPrefix(in.Operation, "memory.") {
			return b.handleMemory(ctx, req, in), nil
		}
		if strings.HasPrefix(in.Operation, "agent.identity.") {
			return b.handleAgentIdentity(ctx, req, in), nil
		}
		if strings.HasPrefix(in.Operation, "owner.") {
			return b.handleOwner(ctx, req, in), nil
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
