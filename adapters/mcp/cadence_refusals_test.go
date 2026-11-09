package product_test

import (
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	product "github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/maintenance"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func cadenceApprove(context.Context, *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
	return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"approved": true}}, nil
}

func TestMaintenanceConfirmationRefusals(t *testing.T) {
	for _, protocol := range []string{"", "2025-11-25"} {
		t.Run(protocol, func(t *testing.T) {
			f := newOwnerFixture(t, false)
			for _, tc := range []struct {
				name, action, code                           string
				content                                      map[string]any
				unavailable, transportError, invalidResponse bool
			}{
				{name: "decline", action: "decline", content: map[string]any{"approved": true}, code: "confirmation_declined"},
				{name: "cancel", action: "cancel", content: map[string]any{"approved": true}, code: "confirmation_declined"},
				{name: "false", action: "accept", content: map[string]any{"approved": false}, code: "confirmation_declined"},
				{name: "missing", action: "accept", content: map[string]any{}, invalidResponse: true},
				{name: "no-form", unavailable: true, code: "confirmation_unavailable"},
				{name: "transport-error", transportError: true},
			} {
				t.Run(tc.name, func(t *testing.T) {
					var handler func(context.Context, *mcp.ElicitRequest) (*mcp.ElicitResult, error)
					if !tc.unavailable {
						handler = func(context.Context, *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
							if tc.transportError {
								return nil, errors.New("synthetic host failure")
							}
							return &mcp.ElicitResult{Action: tc.action, Content: tc.content}, nil
						}
					}
					cs := f.connectProtocol(t, handler, protocol)
					before := cadenceTree(t, f)
					out, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "maestro_apply", Arguments: map[string]any{"operation": "maintenance.qualify", "job_id": maintenance.MemoryLightDreamJobID, "expected_revision": 0}})
					if err != nil {
						if !tc.transportError && !tc.invalidResponse {
							t.Fatal(err)
						}
					} else {
						if !out.IsError {
							t.Fatalf("refusal accepted: %+v", out)
						}
						if tc.code != "" {
							var value map[string]any
							if err := json.Unmarshal([]byte(out.Content[0].(*mcp.TextContent).Text), &value); err != nil {
								t.Fatal(err)
							}
							if value["code"] != tc.code {
								t.Fatal(value)
							}
							if tc.unavailable && !strings.Contains(value["next_action"].(string), "maintenance-review") {
								t.Fatal(value)
							}
						}
					}
					if cadenceTree(t, f) != before {
						t.Fatal("declined/unavailable review wrote state or keys")
					}
				})
			}
		})
	}
}

func TestMaintenanceLegacyQualification(t *testing.T) {
	f := newOwnerFixture(t, false)
	prepareMemoryFixture(t, f)
	cs := f.connectProtocol(t, cadenceApprove, "2025-11-25")
	memoryCall(t, cs, "memory.capture", map[string]any{"text": "private-maintenance-text: legacy reviewed capture"}, false)
	got := cadenceCall(t, cs, "qualify", map[string]any{"job_id": maintenance.MemoryLightDreamJobID, "expected_revision": 0}, false)
	if got["qualification_state"] != "qualified" || got["state"] != "inactive" {
		t.Fatal(got)
	}
}

func TestMaintenanceChangedSourceDuringReview(t *testing.T) {
	for _, protocol := range []string{"", "2025-11-25"} {
		t.Run(protocol, func(t *testing.T) {
			f := newOwnerFixture(t, false)
			prepareMemoryFixture(t, f)
			owner := f.connect(t, cadenceApprove)
			memoryCall(t, owner, "memory.capture", map[string]any{"text": "private-maintenance-text: initial source"}, false)
			called := false
			cs := f.connectProtocol(t, func(_ context.Context, req *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
				called = true
				if !strings.Contains(req.Params.Message, "source_state") || !strings.Contains(req.Params.Message, "implementation") || !strings.Contains(req.Params.Message, "memory-light-dream") {
					t.Fatal("incomplete trusted review")
				}
				memoryCall(t, owner, "memory.capture", map[string]any{"text": "private-maintenance-text: concurrently accepted source"}, false)
				return cadenceApprove(context.Background(), req)
			}, protocol)
			got := cadenceCall(t, cs, "qualify", map[string]any{"job_id": maintenance.MemoryLightDreamJobID, "expected_revision": 0}, true)
			if !called || got["code"] != "revision_conflict" {
				t.Fatal(got)
			}
			status := cadenceCall(t, owner, "status", nil, false)
			job := cadenceJob(t, status, maintenance.MemoryLightDreamJobID)
			if job["revision"] != float64(0) || job["qualification_state"] != "unqualified" {
				t.Fatal(job)
			}
		})
	}
}

func TestMaintenanceClosedSchemas(t *testing.T) {
	f := newOwnerFixture(t, false)
	cs := f.connect(t, cadenceApprove)
	before := cadenceTree(t, f)
	for _, field := range []string{"root", "workspace", "workspace_id", "executable", "qualification", "qualification_digest", "adapter", "handler", "approved", "confirmed", "review_digest", "now", "scheduled_for"} {
		cadenceCall(t, cs, "qualify", map[string]any{"job_id": maintenance.MemoryLightDreamJobID, "expected_revision": 0, field: "untrusted"}, true)
	}
	for _, fields := range []map[string]any{
		{"job_id": maintenance.MemoryLightDreamJobID},
		{"job_id": maintenance.MemoryLightDreamJobID, "expected_revision": -1},
		{"job_id": "arbitrary-job", "expected_revision": 0},
		{"job_id": maintenance.MemoryLightDreamJobID, "expected_revision": 0, "timezone": "UTC"},
	} {
		cadenceCall(t, cs, "qualify", fields, true)
	}
	cadenceCall(t, cs, "enroll", map[string]any{"job_id": maintenance.MemoryLightDreamJobID, "expected_revision": 0}, true)
	cadenceCall(t, cs, "run", map[string]any{"job_id": maintenance.MemoryLightDreamJobID}, true)
	cadenceCall(t, cs, "status", map[string]any{"expected_revision": 0}, true)
	if cadenceTree(t, f) != before {
		t.Fatal("invalid authority/schema initialized state")
	}
}

func TestMaintenanceLocalFallback(t *testing.T) {
	f := newOwnerFixture(t, true)
	owner := f.connect(t, cadenceApprove)
	memoryCall(t, owner, "memory.capture", map[string]any{"text": "private-maintenance-text: stdin qualification source"}, false)
	request := map[string]any{"operation": "maintenance.qualify", "job_id": maintenance.MemoryLightDreamJobID, "expected_revision": 0}
	before := cadenceTree(t, f)
	cadenceCall(t, f.connect(t, nil), "qualify", map[string]any{"job_id": maintenance.MemoryLightDreamJobID, "expected_revision": 0}, true)
	review := memoryCLI(t, f, "maintenance-review", request, nil, false)
	if cadenceTree(t, f) != before {
		t.Fatal("no-form/local review initialized maintenance")
	}
	args := []string{"--review-digest", review["review_digest"].(string)}
	memoryCLI(t, f, "maintenance-apply", request, args, true)
	memoryCLI(t, f, "maintenance-apply", request, []string{"--review-digest", strings.Repeat("0", 64), "--confirm"}, true)
	if cadenceTree(t, f) != before {
		t.Fatal("missing/mismatched local approval wrote state")
	}
	// Changed source invalidates a real review even at the same control revision.
	memoryCall(t, owner, "memory.capture", map[string]any{"text": "private-maintenance-text: changed stdin source"}, false)
	before = cadenceTree(t, f)
	memoryCLI(t, f, "maintenance-apply", request, append(args, "--confirm"), true)
	if cadenceTree(t, f) != before {
		t.Fatal("stale source review wrote maintenance")
	}
	review = memoryCLI(t, f, "maintenance-review", request, nil, false)
	args = []string{"--review-digest", review["review_digest"].(string), "--confirm"}
	// The same reviewed intent cannot authorize a different admitted workspace.
	foreign := newOwnerFixture(t, false)
	prepareMemoryFixture(t, foreign)
	memoryCall(t, foreign.connect(t, cadenceApprove), "memory.capture", map[string]any{"text": "private-maintenance-text: foreign source"}, false)
	foreign.binary = f.binary
	foreignBefore := cadenceTree(t, foreign)
	memoryCLI(t, foreign, "maintenance-apply", request, args, true)
	if cadenceTree(t, foreign) != foreignBefore {
		t.Fatal("cross-workspace replay wrote state")
	}
	qualified := memoryCLI(t, f, "maintenance-apply", request, args, false)
	if qualified["qualification_state"] != "qualified" {
		t.Fatal(qualified)
	}
	memoryCLI(t, f, "maintenance-apply", request, args, true)
	for _, field := range []string{"confirmed", "review_digest", "workspace_id", "now"} {
		memoryCLI(t, f, "maintenance-review", map[string]any{"operation": "maintenance.qualify", "job_id": maintenance.MemoryLightDreamJobID, "expected_revision": 0, field: true}, nil, true)
	}
	enroll := map[string]any{"operation": "maintenance.enroll", "job_id": maintenance.MemoryLightDreamJobID, "expected_revision": qualified["revision"], "timezone": "UTC"}
	review = memoryCLI(t, f, "maintenance-review", enroll, nil, false)
	enrolled := memoryCLI(t, f, "maintenance-apply", enroll, []string{"--review-digest", review["review_digest"].(string), "--confirm"}, false)
	if enrolled["state"] != "active" {
		t.Fatal(enrolled)
	}
	// A genuinely different rebuilt executable cannot reuse qualification.
	replacement := filepath.Join(filepath.Dir(f.binary), "maestro-mcp-changed"+filepath.Ext(productTestBinaryName()))
	build := exec.Command("go", "build", "-ldflags", "-X main.AuthorityRegistrySHA256=synthetic-changed-build", "-o", replacement, "./cmd/maestro-mcp")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("rebuild: %s %v", out, err)
	}
	f.binary = replacement
	var status map[string]any
	if err := json.Unmarshal([]byte(f.cli(t, "maintenance-status", nil, false)), &status); err != nil {
		t.Fatal(err)
	}
	if cadenceJob(t, status, maintenance.MemoryLightDreamJobID)["qualification_state"] != "stale" {
		t.Fatal(status)
	}
	f.cli(t, "maintenance-run", nil, true)
	for _, command := range []string{"serve", "maintenance-status", "maintenance-run", "maintenance-review"} {
		f.cli(t, command, []string{"--confirm"}, true)
	}
}

func TestMaintenanceURLOnlyHostCannotApprove(t *testing.T) {
	f := newOwnerFixture(t, false)
	bound, err := product.Open(f.options)
	if err != nil {
		t.Fatal(err)
	}
	a, z := mcp.NewInMemoryTransports()
	server, err := bound.Server().Connect(context.Background(), a, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { server.Close() })
	called := false
	client := mcp.NewClient(&mcp.Implementation{Name: "url-only", Version: "1"}, &mcp.ClientOptions{
		Capabilities: &mcp.ClientCapabilities{Elicitation: &mcp.ElicitationCapabilities{URL: &mcp.URLElicitationCapabilities{}}},
		ElicitationHandler: func(ctx context.Context, req *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
			called = true
			return cadenceApprove(ctx, req)
		},
	})
	cs, err := client.Connect(context.Background(), z, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cs.Close() })
	before := cadenceTree(t, f)
	got := cadenceCall(t, cs, "qualify", map[string]any{"job_id": maintenance.MemoryLightDreamJobID, "expected_revision": 0}, true)
	if got["code"] != "confirmation_unavailable" || called || cadenceTree(t, f) != before {
		t.Fatal(got)
	}
}
