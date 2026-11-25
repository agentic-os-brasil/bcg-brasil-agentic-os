package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	product "github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp"
	baseagents "github.com/agentic-os-brasil/bcg-brasil-agentic-os/bundles/base/agents"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/datahome"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/pilotdocument"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspace"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

// The existing CLI recognizes explicit synthetic writes but cannot turn an
// unavailable installation into authority or discard the reusable diagnostic.
func TestInstalledPilotFunctionalCLIReportsUnavailableWithoutPreparing(t *testing.T) {
	root := t.TempDir()
	args := []string{"--product-root", filepath.Join(root, "product"), "--user-home", filepath.Join(root, "owner"), "--workspace", filepath.Join(root, "workspace"), "--carrier-id", strings.Repeat("a", 32), "--installer-version", "0.1.99", "--experimental-seed-sha256", strings.Repeat("b", 64), "--experimental-installed-root", filepath.Join(root, "product"), "--functional"}
	var output bytes.Buffer
	if err := runInstalledPilotCheck(context.Background(), args, &output); err == nil {
		t.Fatal("missing installed authority accepted")
	}
	var r struct {
		CheckMode  string       `json:"check_mode"`
		Functional string       `json:"functional_check"`
		Checks     []pilotCheck `json:"checks"`
		Markdown   string       `json:"markdown"`
	}
	if err := json.Unmarshal(output.Bytes(), &r); err != nil {
		t.Fatal(err)
	}
	if r.CheckMode != "functional" || r.Functional != "incomplete" || len(r.Checks) < 2 || r.Checks[len(r.Checks)-1].Name != "experimental_admission" || r.Markdown == "" {
		t.Fatalf("functional opt-in was lost or falsely succeeded: %+v", r)
	}
}

type installedPilotChildConfig struct{ Product, Home, Workspace, Mutation, ReadyMarker string }

const installedPilotChildReady = "installed-check-child-ready\n"

// The isolated child uses the actual product resource/tool implementation over
// stdio. It is a source transport fixture, not installed seed or client proof.
func TestInstalledPilotChildFixture(t *testing.T) {
	raw := os.Getenv("MAESTRO_INSTALLED_CHECK_TEST_CHILD")
	if raw == "" {
		return
	}
	var f installedPilotChildConfig
	if err := json.Unmarshal([]byte(raw), &f); err != nil {
		panic(err)
	}
	if f.Mutation == "hang" {
		if f.ReadyMarker != "" {
			marker, err := os.OpenFile(f.ReadyMarker, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
			if err != nil {
				panic(err)
			}
			_, writeErr := marker.WriteString(installedPilotChildReady)
			closeErr := marker.Close()
			if writeErr != nil || closeErr != nil {
				panic(fmt.Errorf("fixture readiness: write=%v close=%v", writeErr, closeErr))
			}
		}
		<-time.After(time.Minute)
		os.Exit(0)
	}
	b, err := product.OpenContext(context.Background(), product.Options{ProductRoot: f.Product, UserHome: f.Home, WorkspacePath: f.Workspace, CLIVersion: "0.1.99", LookupEnv: func(string) (string, bool) { return "", false }})
	if err != nil {
		panic(err)
	}
	server := b.Server()
	if f.Mutation == "method" || f.Mutation == "role" || f.Mutation == "method_body" || f.Mutation == "role_body" {
		uri := "maestro://agents/maestro/AGENT.md"
		if f.Mutation == "method" || f.Mutation == "method_body" {
			uri = "maestro://methods/maestro-doctor/SKILL.md"
		}
		metadata := &mcp.Resource{URI: uri, Size: 1, Meta: mcp.Meta{"sha256": "tampered"}}
		if strings.HasSuffix(f.Mutation, "_body") {
			var canonical []byte
			if f.Mutation == "method_body" {
				methods, _ := product.PackagedMethods()
				for _, m := range methods.Methods {
					if m.ID == "maestro-doctor" {
						canonical = m.Files["SKILL.md"]
					}
				}
			} else {
				canonical, _ = baseagents.Definition("maestro")
			}
			metadata.Size = int64(len(canonical))
			metadata.Meta["sha256"] = installedPilotSHA(canonical)
			metadata.MIMEType = "text/markdown"
		}
		server.AddResource(metadata, func(context.Context, *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
			return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{URI: uri, Text: "tampered"}}}, nil
		})
	}
	if f.Mutation != "" && f.Mutation != "method" && f.Mutation != "role" && f.Mutation != "method_body" && f.Mutation != "role_body" {
		server = installedFunctionalProtocolFixture(t, server, f)
	}
	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		panic(err)
	}
	os.Exit(0)
}

func TestInstalledPilotStdioCatalogIntegrityAndReadOnlyEffects(t *testing.T) {
	for _, mutation := range []string{"", "method", "role", "method_body", "role_body"} {
		t.Run(mutation, func(t *testing.T) {
			root, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			f := installedPilotChildConfig{Product: filepath.Join(root, "product"), Home: filepath.Join(root, "owner"), Workspace: filepath.Join(root, "workspace"), Mutation: mutation}
			for _, p := range []string{f.Product, f.Home} {
				if err := os.Mkdir(p, 0700); err != nil {
					t.Fatal(err)
				}
			}
			h, err := datahome.InitializeFresh(datahome.Options{ProductRoot: f.Product, UserHome: f.Home, LookupEnv: func(string) (string, bool) { return "", false }})
			if err != nil {
				t.Fatal(err)
			}
			w, err := workspace.Initialize(workspace.Options{WorkspacePath: f.Workspace, DataRoot: filepath.Join(h.Home, "state")})
			if err != nil {
				t.Fatal(err)
			}
			snapshot := func() map[string]string {
				m := map[string]string{}
				err := filepath.WalkDir(root, func(p string, d fs.DirEntry, e error) error {
					if e != nil {
						return e
					}
					if !d.IsDir() {
						b, e := os.ReadFile(p)
						if e != nil {
							return e
						}
						m[p] = fmt.Sprintf("%x", sha256.Sum256(b))
					}
					return nil
				})
				if err != nil {
					t.Fatal(err)
				}
				return m
			}
			before := snapshot()
			body, _ := json.Marshal(f)
			opens := 0
			open := func(ctx context.Context) (*mcp.ClientSession, error) {
				opens++
				cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestInstalledPilotChildFixture$")
				cmd.Env = append(os.Environ(), "MAESTRO_INSTALLED_CHECK_TEST_CHILD="+string(body))
				return installedPilotConnect(ctx, cmd)
			}
			prepared := false
			r := installedPilotReport{pilotReport: pilotReport{SchemaVersion: 2, Status: "partial"}}
			err = installedPilotExercise(t.Context(), open, func() error { prepared = true; return nil }, w.WorkspaceID, strings.Repeat("b", 64), strings.Repeat("a", 64), "0.1.99", &r, false)
			if mutation == "" {
				if err != nil {
					t.Fatal(err)
				}
				if len(r.Checks) < 4 || r.Checks[0].Reason != "actual_authenticated_checker_carrier_sdk_context_verified_not_host_loading" {
					t.Fatalf("missing actual transport/integrity evidence: %+v", r.Checks)
				}
			} else if err == nil {
				t.Fatal("changed canonical resource admitted")
			}
			if prepared || opens != 1 || !reflect.DeepEqual(before, snapshot()) {
				t.Fatal("read-only carrier check prepared or mutated participant state")
			}
		})
	}
}

// Only this test adapter supplies synthetic parser output. Execution, discovery
// and resource reads still use the real product behind the outer stdio child.
// This proves caller contracts and never claims actual converter qualification.
func installedFunctionalProtocolFixture(t *testing.T, original *mcp.Server, f installedPilotChildConfig) *mcp.Server {
	ctx, cancel := context.WithCancel(context.Background())
	a, z := mcp.NewInMemoryTransports()
	done := make(chan error, 1)
	go func() { done <- original.Run(ctx, a) }()
	inner, err := mcp.NewClient(&mcp.Implementation{Name: "self-check-inner-test", Version: "1"}, nil).Connect(ctx, z, nil)
	if err != nil {
		panic(err)
	}
	t.Cleanup(func() { inner.Close(); cancel(); <-done })
	server := mcp.NewServer(&mcp.Implementation{Name: "maestro-product", Version: "0.1.99"}, nil)
	methods, err := inner.ListTools(ctx, nil)
	if err != nil {
		panic(err)
	}
	md := "# MAESTRO\\_DOCLING\\_OFFLINE\\_20261101\n\n| Item | Count |\n| --- | --- |\n| alpha | 7 |\n"
	document := map[string]any{"schema_name": "DoclingDocument", "texts": []any{map[string]any{"text": "MAESTRO_DOCLING_OFFLINE_20261101"}}, "tables": []any{map[string]any{"data": map[string]any{"table_cells": []any{map[string]any{"text": "Item", "start_row_offset_idx": 0, "start_col_offset_idx": 0}, map[string]any{"text": "Count", "start_row_offset_idx": 0, "start_col_offset_idx": 1}, map[string]any{"text": "alpha", "start_row_offset_idx": 1, "start_col_offset_idx": 0}, map[string]any{"text": "7", "start_row_offset_idx": 1, "start_col_offset_idx": 1}}}}}, "synthetic_padding": strings.Repeat("ação", 1400)}
	doc, _ := json.Marshal(document)
	outputs := map[string]string{"markdown": md, "document": string(doc)}
	prov := map[string]any{"network_denied": true, "pack_sha256": strings.Repeat("a", 64), "manifest_sha256": strings.Repeat("b", 64), "lock_sha256": strings.Repeat("c", 64), "markdown_sha256": installedPilotSHA([]byte(md)), "document_sha256": installedPilotSHA(doc)}
	var binding struct {
		WorkspaceID string `json:"workspace_id"`
	}
	if err := pilotCall(ctx, inner, "maestro_context", map[string]any{}, &binding); err != nil {
		panic(err)
	}
	stored := map[string]any{"schema_version": 1, "status": "usable", "adapter": "docling", "workspace_id": binding.WorkspaceID, "source_sha256": pilotDocumentSHA256, "source_name": pilotDocumentName, "route": "docling_local", "parser": "docling.docx", "fidelity": "structured", "output_bytes": len(md) + len(doc)}
	artifact := strings.Repeat("d", 64)
	result := func(v any, failed bool) *mcp.CallToolResult {
		b, _ := json.Marshal(v)
		return &mcp.CallToolResult{IsError: failed, Content: []mcp.Content{&mcp.TextContent{Text: string(b)}}}
	}
	for _, tool := range methods.Tools {
		tool := tool
		server.AddTool(tool, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			var args map[string]any
			if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
				return nil, err
			}
			op, _ := args["operation"].(string)
			log, err := os.OpenFile(filepath.Join(filepath.Dir(f.Home), "calls.jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
			if err != nil {
				return nil, err
			}
			json.NewEncoder(log).Encode(map[string]any{"tool": tool.Name, "args": args})
			log.Close()
			switch op {
			case "content.status":
				if f.Mutation == "unavailable" {
					return result(map[string]any{"state": "unavailable"}, false), nil
				}
				return result(map[string]any{"state": "ready", "pack_sha256": strings.Repeat("a", 64), "formats": []any{map[string]any{"extension": ".docx", "state": "ready", "route": "docling_local", "parser": "docling.docx", "fidelity": "structured"}}}, false), nil
			case "content.convert":
				creation := map[string]any{}
				for k, v := range stored {
					creation[k] = v
				}
				creation["artifact_ref"] = artifact
				return result(map[string]any{"result": creation, "provenance": prov}, false), nil
			case "content.read":
				format, _ := args["format"].(string)
				body := outputs[format]
				offset := int(args["offset"].(float64))
				limit := int(args["max_bytes"].(float64))
				end := offset + limit
				if end > len(body) {
					end = len(body)
				}
				for !utf8.ValidString(body[offset:end]) {
					end--
				}
				readResult := map[string]any{}
				for k, v := range stored {
					readResult[k] = v
				}
				if f.Mutation == "read_changed" {
					readResult["source_name"] = "foreign.docx"
				}
				return result(map[string]any{"artifact_id": artifact, "format": format, "text": body[offset:end], "has_more": end < len(body), "next_offset": end, "result": readResult, "provenance": prov}, false), nil
			case "execution.checkpoint":
				if f.Mutation == "after_start" {
					return result(map[string]any{"code": "execution_rejected", "private": "PRIVATE_SYNTHETIC_CAUSE"}, true), nil
				}
				if f.Mutation == "wrong_stale" && args["summary"] == "Stale synthetic attempt must refuse" {
					return result(map[string]any{"code": "revision_conflict"}, true), nil
				}
			}
			return inner.CallTool(ctx, &mcp.CallToolParams{Name: tool.Name, Arguments: args})
		})
	}
	params := &mcp.ListResourcesParams{}
	for {
		page, err := inner.ListResources(ctx, params)
		if err != nil {
			panic(err)
		}
		for _, r := range page.Resources {
			r := r
			server.AddResource(r, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
				return inner.ReadResource(ctx, req.Params)
			})
		}
		if page.NextCursor == "" {
			break
		}
		params.Cursor = page.NextCursor
	}
	return server
}

func TestInstalledPilotFunctionalStdioCompletesOnlyOwnCaseOrRetainsFirstFailure(t *testing.T) {
	for _, mode := range []string{"functional", "read_changed", "after_start", "wrong_stale", "unavailable"} {
		t.Run(mode, func(t *testing.T) {
			root, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			f := installedPilotChildConfig{Product: filepath.Join(root, "product"), Home: filepath.Join(root, "owner"), Workspace: filepath.Join(root, "workspace"), Mutation: mode}
			for _, p := range []string{f.Product, f.Home} {
				if err := os.Mkdir(p, 0700); err != nil {
					t.Fatal(err)
				}
			}
			h, err := datahome.InitializeFresh(datahome.Options{ProductRoot: f.Product, UserHome: f.Home, LookupEnv: func(string) (string, bool) { return "", false }})
			if err != nil {
				t.Fatal(err)
			}
			w, err := workspace.Initialize(workspace.Options{WorkspacePath: f.Workspace, DataRoot: filepath.Join(h.Home, "state")})
			if err != nil {
				t.Fatal(err)
			}
			body, _ := json.Marshal(f)
			opens := 0
			open := func(ctx context.Context) (*mcp.ClientSession, error) {
				opens++
				cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestInstalledPilotChildFixture$")
				cmd.Env = append(os.Environ(), "MAESTRO_INSTALLED_CHECK_TEST_CHILD="+string(body))
				return installedPilotConnect(ctx, cmd)
			}
			// A real participant may already have unrelated work. It must survive and
			// must not make the self-check require an empty global ledger.
			prior, err := open(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			var existing map[string]any
			if err := pilotCall(t.Context(), prior, "maestro_apply", map[string]any{"operation": "execution.create", "objective": "Preserve unrelated participant work", "next_step": "Unrelated next step", "artifacts": []string{"unrelated.txt"}}, &existing); err != nil {
				t.Fatal(err)
			}
			var running map[string]any
			if err := pilotCall(t.Context(), prior, "maestro_apply", map[string]any{"operation": "execution.start", "item_id": existing["item_id"], "expected_revision": existing["state_revision"]}, &running); err != nil {
				t.Fatal(err)
			}
			var before map[string]any
			if err := pilotCall(t.Context(), prior, "maestro_query", map[string]any{"operation": "execution.next", "item_id": existing["item_id"]}, &before); err != nil {
				t.Fatal(err)
			}
			prior.Close()
			opens = 0
			prepared := 0
			r := installedPilotReport{pilotReport: pilotReport{SchemaVersion: 2, Status: "partial"}}
			prepare := func() error {
				prepared++
				b, e := pilotdocument.Bytes()
				if e != nil {
					return e
				}
				return os.WriteFile(filepath.Join(f.Workspace, pilotdocument.Name), b, 0600)
			}
			err = installedPilotExercise(t.Context(), open, prepare, w.WorkspaceID, strings.Repeat("b", 64), strings.Repeat("a", 64), "0.1.99", &r, true)
			if mode == "functional" {
				if err != nil || r.FunctionalCheck != "passed" || r.FunctionalItemID == "" || opens != 2 || prepared != 1 {
					t.Fatalf("bounded synthetic journey: %+v %v opens=%d", r, err, opens)
				}
			} else {
				if err == nil || r.FunctionalCheck != "incomplete" {
					t.Fatalf("failed stage hidden: %+v %v", r, err)
				}
				if (mode == "after_start" || mode == "wrong_stale") && r.FunctionalItemID == "" {
					t.Fatal("unfinished item identity discarded")
				}
				if mode == "unavailable" && prepared != 0 {
					t.Fatal("unavailable runtime prepared a fixture")
				}
			}
			calls, err := os.ReadFile(filepath.Join(root, "calls.jsonl"))
			if err != nil {
				t.Fatal(err)
			}
			creates, converts, resumes := 0, 0, 0
			for _, line := range bytes.Split(bytes.TrimSpace(calls), []byte("\n")) {
				var call struct {
					Tool string         `json:"tool"`
					Args map[string]any `json:"args"`
				}
				if json.Unmarshal(line, &call) != nil {
					t.Fatal("bad trace")
				}
				op, _ := call.Args["operation"].(string)
				switch op {
				case "execution.create":
					creates++
				case "content.convert":
					converts++
				case "execution.resume":
					resumes++
				}
				for _, forbidden := range []string{"brain.", "knowledge.", "memory.", "maintenance.", "owner."} {
					if strings.HasPrefix(op, forbidden) {
						t.Fatal("unrelated authority invoked")
					}
				}
			}
			if converts > 1 || creates > 2 || resumes > 1 {
				t.Fatalf("ambiguous mutation replayed: creates=%d converts=%d resumes=%d", creates, converts, resumes)
			}
			last, err := open(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			defer last.Close()
			var after map[string]any
			if err := pilotCall(t.Context(), last, "maestro_query", map[string]any{"operation": "execution.next", "item_id": existing["item_id"]}, &after); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(before, after) {
				t.Fatal("unrelated participant case changed")
			}
			if mode == "functional" {
				var finished map[string]any
				if err := pilotCall(t.Context(), last, "maestro_query", map[string]any{"operation": "execution.next", "item_id": r.FunctionalItemID}, &finished); err != nil {
					t.Fatal(err)
				}
				if finished["state"] != "completed" {
					t.Fatal("own case remained active")
				}
			}
		})
	}
}

func TestInstalledPilotFunctionalModeIsExplicitAndReportIsShareable(t *testing.T) {
	root := t.TempDir()
	base := []string{"--product-root", filepath.Join(root, "product"), "--user-home", filepath.Join(root, "owner"), "--workspace", filepath.Join(root, "workspace"), "--carrier-id", strings.Repeat("a", 32), "--installer-version", "0.1.99", "--experimental-seed-sha256", strings.Repeat("b", 64), "--experimental-installed-root", filepath.Join(root, "product")}
	for _, extra := range [][]string{nil, {"--check-only"}, {"--functional"}, {"--functional=true"}} {
		s, err := parseInstalledPilotSelection(append(append([]string{}, base...), extra...))
		if err != nil || s.Functional != (len(extra) > 0 && strings.HasPrefix(extra[0], "--functional")) {
			t.Fatalf("wrong opt-in: %+v %v", s, err)
		}
	}
	for _, extra := range [][]string{{"--check-only", "--functional"}, {"--functional=false"}, {"--check-only=false"}, {"--functional", "--functional"}, {"--functional", "--source-path", "arbitrary.docx"}} {
		if _, err := parseInstalledPilotSelection(append(append([]string{}, base...), extra...)); err == nil {
			t.Fatal("unclosed or ambiguous effect mode admitted")
		}
	}
	r := installedPilotReport{pilotReport: pilotReport{SchemaVersion: 2, Status: "partial", WorkspaceID: strings.Repeat("a", 32)}, CheckMode: "functional", FunctionalCheck: "incomplete", FunctionalStage: "execution_checkpoint", FunctionalReason: "installed_mcp_execution_checkpoint_failed", FunctionalItemID: "synthetic-known-id", PrivateDiagnostic: "PRIVATE_OWNER_BODY PRIVATE_PROMPT CREDENTIAL"}
	var b bytes.Buffer
	if err := finishInstalledPilotReport(&r, &b); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(r.Markdown, "PRIVATE_") || strings.Contains(r.Markdown, "CREDENTIAL") || !strings.Contains(r.Markdown, "synthetic-known-id") || strings.Contains(r.Markdown, "faz apenas leituras locais") {
		t.Fatal("shareable report leaked private details or misrepresented synthetic effects")
	}
}

func TestInstalledPilotChildConnectionHonorsCallerCancellation(t *testing.T) {
	f := installedPilotChildConfig{Mutation: "hang"}
	body, _ := json.Marshal(f)
	ctx, cancel := context.WithTimeout(t.Context(), 40*time.Millisecond)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestInstalledPilotChildFixture$")
	cmd.Env = append(os.Environ(), "MAESTRO_INSTALLED_CHECK_TEST_CHILD="+string(body))
	start := time.Now()
	s, err := installedPilotConnect(ctx, cmd)
	if s != nil {
		defer s.Close()
	}
	if err == nil || !errors.Is(err, context.DeadlineExceeded) && ctx.Err() == nil || time.Since(start) > 3*time.Second {
		t.Fatalf("child transport ignored finite caller cancellation: %v elapsed=%s", err, time.Since(start))
	}
}
