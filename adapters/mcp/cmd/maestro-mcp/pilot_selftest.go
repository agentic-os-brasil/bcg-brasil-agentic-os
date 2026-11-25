package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	product "github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp"
	baseagents "github.com/agentic-os-brasil/bcg-brasil-agentic-os/bundles/base/agents"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/pilotdocument"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// One installed action owns transport, verification and (only when requested)
// synthetic effects. The launcher is derived from the admitted selection, never
// supplied as an executable argument or replaced by an in-process server.
func installedPilotTransport(ctx context.Context, o product.Options, id, manifest, pack string, r *installedPilotReport, functional bool) error {
	if o.ExperimentalSeed == nil || o.ExperimentalSeed.Revalidate() != nil || !pilotWorkspaceID.MatchString(o.ManagementCarrierID) || !pilotKitID.MatchString(manifest) || !pilotKitID.MatchString(pack) || o.InstalledManifestSHA256 != manifest {
		return errors.New("authenticated_installed_carrier_required")
	}
	launcher := filepath.Join(o.ProductRoot, "maestro-install")
	if r.Target.OS == "windows" {
		launcher += ".exe"
	}
	args := []string{"stable-check-launch", "--product-root", o.ProductRoot, "--user-home", o.UserHome, "--workspace", o.WorkspacePath, "--carrier-id", o.ManagementCarrierID}
	open := func(ctx context.Context) (*mcp.ClientSession, error) {
		if err := o.ExperimentalSeed.Revalidate(); err != nil {
			return nil, err
		}
		return installedPilotConnect(ctx, exec.CommandContext(ctx, launcher, args...))
	}
	prepare := func() error { return installedPilotPrepareFixture(ctx, o, id) }
	return installedPilotExercise(ctx, open, prepare, id, manifest, pack, Version, r, functional)
}

func installedPilotConnect(ctx context.Context, cmd *exec.Cmd) (*mcp.ClientSession, error) {
	cmd.Stderr = io.Discard // The shareable report never receives child output.
	connectCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	s, err := mcp.NewClient(&mcp.Implementation{Name: "maestro-installed-pilot-check", Version: "2"}, &mcp.ClientOptions{Capabilities: &mcp.ClientCapabilities{}}).Connect(connectCtx, &mcp.CommandTransport{Command: cmd, TerminateDuration: time.Second}, nil)
	if err != nil {
		return nil, &installedPilotReadbackError{stage: "checker_connect", cause: err}
	}
	return s, nil
}

type installedPilotOpen func(context.Context) (*mcp.ClientSession, error)

func installedPilotExercise(ctx context.Context, open installedPilotOpen, prepare func() error, id, manifest, pack, version string, r *installedPilotReport, functional bool) (err error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Minute)
	defer cancel()
	r.CheckMode, r.FunctionalCheck = "check_only", "not_requested"
	if functional {
		r.CheckMode, r.FunctionalCheck = "functional", "incomplete"
	}
	stage := "connect"
	defer func() {
		if err != nil && functional {
			r.FunctionalCheck = "incomplete"
			r.FunctionalStage = stage
			r.FunctionalReason = "installed_mcp_" + stage + "_failed"
		}
	}()
	session, err := open(ctx)
	if err != nil {
		return err
	}
	defer func() {
		if session != nil {
			if closeErr := session.Close(); err == nil && closeErr != nil {
				stage = "close"
				err = &installedPilotReadbackError{stage: "close", cause: closeErr}
			}
		}
	}()
	checkContext := func(s *mcp.ClientSession) error {
		init := s.InitializeResult()
		if init == nil || init.ServerInfo.Name != "maestro-product" || init.ServerInfo.Version != version || init.Capabilities == nil || init.Capabilities.Tools == nil || init.Capabilities.Resources == nil {
			return errors.New("installed_server_identity_mismatch")
		}
		var got struct {
			WorkspaceID string `json:"workspace_id"`
			State       string `json:"state"`
		}
		if err := installedPilotCall(ctx, s, "context", "maestro_context", map[string]any{}, &got); err != nil {
			return err
		}
		if got.WorkspaceID != id || got.State != "ready" {
			return errors.New("installed_context_mismatch")
		}
		return nil
	}
	stage = "context"
	if err = checkContext(session); err != nil {
		return err
	}
	r.add("mcp_connection", "passed", "actual_authenticated_checker_carrier_sdk_context_verified_not_host_loading", "")
	stage = "canonical_resources"
	if err = installedPilotCatalog(ctx, session); err != nil {
		return err
	}
	r.add("method_catalog", "passed", "complete_canonical_method_inventory_verified_not_native_execution", "")
	r.add("canonical_resources", "passed", "complete_role_and_method_resources_verified_byte_for_byte", "")
	var availability struct {
		State   string                                                       `json:"state"`
		Pack    string                                                       `json:"pack_sha256"`
		Formats []struct{ Extension, State, Route, Parser, Fidelity string } `json:"formats"`
	}
	stage = "content_status"
	if err = installedPilotCall(ctx, session, "content_status", "maestro_query", map[string]any{"operation": "content.status"}, &availability); err != nil {
		return err
	}
	if err = installedConversionObservation(&r.pilotReport, manifest, pack, availability.State, availability.Pack); err != nil {
		return err
	}
	if !functional {
		return nil
	}
	docxReady := false
	docxCount := 0
	for _, f := range availability.Formats {
		if f.Extension == ".docx" {
			docxCount++
			docxReady = f.State == "ready" && f.Route == "docling_local" && f.Parser == "docling.docx" && f.Fidelity == "structured"
		}
	}
	if availability.State != "ready" || availability.Pack != pack || !docxReady || docxCount != 1 {
		return errors.New("functional_converter_unavailable")
	}
	stage = "fixture"
	if err = prepare(); err != nil {
		return err
	}
	r.add("synthetic_fixture", "passed", "canonical_docx_verified_conflicting_files_preserved", "")
	stage = "conversion"
	if err = installedPilotConvert(ctx, session, id, manifest, pack); err != nil {
		return err
	}
	r.add("conversion", "passed", "one_synthetic_docx_conversion_and_complete_verified_readback", "")
	stage = "execution_create"
	var item map[string]any
	call := func(op string, extra map[string]any, out any) error {
		args := map[string]any{"operation": op}
		for k, v := range extra {
			args[k] = v
		}
		return installedPilotCall(ctx, session, "functional_"+op, "maestro_apply", args, out)
	}
	if err = call("execution.create", map[string]any{"objective": "Maestro MCP synthetic self-check", "next_step": "Verify canonical synthetic document", "artifacts": []string{pilotdocument.Name}}, &item); err != nil {
		return err
	}
	itemID, _ := item["item_id"].(string)
	if !regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,95}$`).MatchString(itemID) {
		return errors.New("execution_create_identity_missing")
	}
	r.FunctionalItemID = itemID
	if item["workspace_id"] != id || item["state_revision"] != float64(1) || item["state"] != "ready" {
		return errors.New("execution_create_receipt_mismatch")
	}
	mutate := func(op, attempt string, extra map[string]any) (map[string]any, error) {
		args := map[string]any{"item_id": itemID, "expected_revision": item["state_revision"]}
		if attempt != "" {
			args["attempt_id"] = attempt
		}
		for k, v := range extra {
			args[k] = v
		}
		var next map[string]any
		e := call("execution."+op, args, &next)
		if e == nil {
			priorRevision, ok := item["state_revision"].(float64)
			if !ok || next["item_id"] != itemID || next["workspace_id"] != id || next["state_revision"] != priorRevision+1 {
				return next, errors.New("execution_mutation_receipt_mismatch")
			}
			item = next
		}
		return next, e
	}
	stage = "execution_start"
	if _, err = mutate("start", "", nil); err != nil {
		return err
	}
	firstAttempt, _ := item["attempt_id"].(string)
	if firstAttempt == "" {
		return errors.New("execution_attempt_missing")
	}
	stage = "execution_checkpoint"
	if _, err = mutate("checkpoint", firstAttempt, map[string]any{"summary": "Canonical synthetic document verified", "next_step": "Reconnect and verify preserved checkpoint"}); err != nil {
		return err
	}
	stage = "execution_pause"
	if _, err = mutate("pause", firstAttempt, nil); err != nil {
		return err
	}
	if item["state"] != "paused" {
		return errors.New("execution_not_paused")
	}
	pausedRevision := item["state_revision"]
	stage = "reconnect"
	if err = session.Close(); err != nil {
		session = nil
		return err
	}
	session = nil
	if session, err = open(ctx); err != nil {
		return err
	}
	if err = checkContext(session); err != nil {
		return err
	}
	next := func() (map[string]any, error) {
		var got map[string]any
		e := installedPilotCall(ctx, session, "execution_next", "maestro_query", map[string]any{"operation": "execution.next", "item_id": itemID}, &got)
		if e == nil && got["item_id"] != itemID {
			return nil, errors.New("execution_projection_identity_mismatch")
		}
		return got, e
	}
	stage = "paused_readback"
	observed, e := next()
	if e != nil {
		return e
	}
	if observed["state"] != "paused" || observed["state_revision"] != pausedRevision || observed["summary"] != "Canonical synthetic document verified" || observed["next_step"] != "Reconnect and verify preserved checkpoint" {
		return errors.New("execution_checkpoint_readback_mismatch")
	}
	stage = "execution_resume"
	if _, err = mutate("resume", "", nil); err != nil {
		return err
	}
	attempt, _ := item["attempt_id"].(string)
	if attempt == "" || attempt == firstAttempt || item["state"] != "running" {
		return errors.New("execution_resume_identity_mismatch")
	}
	stage = "stale_attempt_refusal"
	before, e := next()
	if e != nil {
		return e
	}
	stale := map[string]any{"operation": "execution.checkpoint", "item_id": itemID, "expected_revision": item["state_revision"], "attempt_id": firstAttempt, "summary": "Stale synthetic attempt must refuse", "next_step": "Must not replace current checkpoint"}
	if err = installedPilotAttemptRefusal(ctx, session, stale); err != nil {
		return err
	}
	after, e := next()
	if e != nil {
		return e
	}
	if !reflect.DeepEqual(before, after) {
		return errors.New("stale_attempt_changed_projection")
	}
	stage = "execution_evidence"
	var contract struct {
		Criteria []struct{ ID, Path string } `json:"criteria"`
	}
	if err = installedPilotCall(ctx, session, "execution_contract", "maestro_query", map[string]any{"operation": "execution.contract", "item_id": itemID}, &contract); err != nil {
		return err
	}
	if len(contract.Criteria) != 1 || contract.Criteria[0].Path != pilotdocument.Name || contract.Criteria[0].ID == "" {
		return errors.New("synthetic_execution_contract_mismatch")
	}
	if _, err = mutate("evidence", attempt, map[string]any{"criterion_id": contract.Criteria[0].ID}); err != nil {
		return err
	}
	fixture, fixtureErr := pilotdocument.Bytes()
	if fixtureErr != nil || item["outcome"] != "passed" || item["artifact_sha256"] != pilotdocument.SHA256 || item["artifact_bytes"] != float64(len(fixture)) || item["criterion_id"] != contract.Criteria[0].ID || item["attempt_id"] != attempt {
		return errors.New("synthetic_artifact_evidence_mismatch")
	}
	stage = "execution_complete"
	if _, err = mutate("complete", attempt, nil); err != nil {
		return err
	}
	if item["state"] != "completed" {
		return errors.New("synthetic_execution_not_completed")
	}
	observed, e = next()
	if e != nil {
		return e
	}
	if observed["state"] != "completed" || observed["state_revision"] != item["state_revision"] {
		return errors.New("synthetic_completed_readback_mismatch")
	}
	r.add("execution_continuity", "passed", "one_case_checkpoint_pause_same_carrier_resume_stale_refusal_evidence_complete", "")
	r.FunctionalCheck, r.FunctionalStage, r.FunctionalReason = "passed", "completed", "synthetic_mcp_conversion_and_continuity_verified_not_native_host"
	return nil
}

func installedPilotCatalog(ctx context.Context, s *mcp.ClientSession) error {
	tools := map[string]bool{}
	toolCursors := map[string]bool{}
	toolParams := &mcp.ListToolsParams{}
	for page := 0; ; page++ {
		if page >= 8 {
			return errors.New("installed_tool_bound")
		}
		cc, cancel := context.WithTimeout(ctx, 20*time.Second)
		listed, e := s.ListTools(cc, toolParams)
		cancel()
		if e != nil {
			return e
		}
		if listed == nil || len(listed.Tools) > 32 {
			return errors.New("installed_tool_bound")
		}
		for _, tool := range listed.Tools {
			if tool == nil {
				return errors.New("installed_tool_invalid")
			}
			if tools[tool.Name] {
				return errors.New("installed_tool_duplicate")
			}
			tools[tool.Name] = true
		}
		if listed.NextCursor == "" {
			break
		}
		if len(listed.NextCursor) > 4096 || toolCursors[listed.NextCursor] {
			return errors.New("installed_tool_cursor")
		}
		toolCursors[listed.NextCursor] = true
		toolParams.Cursor = listed.NextCursor
	}
	for _, name := range []string{"maestro_context", "maestro_query", "maestro_apply"} {
		if !tools[name] {
			return errors.New("installed_tool_missing")
		}
	}
	methods, err := product.PackagedMethods()
	if err != nil {
		return err
	}
	wantIDs := []string{}
	for _, m := range methods.Methods {
		wantIDs = append(wantIDs, m.ID)
	}
	gotIDs := []string{}
	for offset := 0; ; {
		if offset > 128 {
			return errors.New("installed_method_bound")
		}
		var p struct {
			Items []struct{ ID string } `json:"items"`
			Next  int                   `json:"next_offset"`
			More  bool                  `json:"has_more"`
		}
		if err := installedPilotCall(ctx, s, "methods_list", "maestro_query", map[string]any{"operation": "methods.list", "offset": offset, "limit": 32}, &p); err != nil {
			return err
		}
		if len(p.Items) > 32 || p.Next != offset+len(p.Items) || p.More && p.Next <= offset {
			return errors.New("installed_method_page")
		}
		for _, m := range p.Items {
			gotIDs = append(gotIDs, m.ID)
		}
		if !p.More {
			break
		}
		offset = p.Next
	}
	if !reflect.DeepEqual(gotIDs, wantIDs) {
		return errors.New("installed_method_catalog_mismatch")
	}
	expected := map[string][]byte{}
	for name, b := range methods.Assets {
		if name == "methods-index.json" {
			name = "catalog.json"
		}
		expected["maestro://methods/"+name] = b
	}
	for _, m := range methods.Methods {
		for _, r := range m.Resources {
			expected[r.URI] = m.Files[r.Path]
		}
	}
	agents, err := baseagents.Files()
	if err != nil {
		return err
	}
	for name, b := range agents {
		expected["maestro://agents/"+name] = b
	}
	seen := map[string]bool{}
	cursors := map[string]bool{}
	params := &mcp.ListResourcesParams{}
	for page := 0; ; page++ {
		if page >= 16 {
			return errors.New("installed_resource_bound")
		}
		cc, cancel := context.WithTimeout(ctx, 20*time.Second)
		p, e := s.ListResources(cc, params)
		cancel()
		if e != nil {
			return e
		}
		if p == nil || len(p.Resources) > 512 {
			return errors.New("installed_resource_bound")
		}
		for _, r := range p.Resources {
			if r == nil {
				return errors.New("installed_resource_invalid")
			}
			if !strings.HasPrefix(r.URI, "maestro://methods/") && !strings.HasPrefix(r.URI, "maestro://agents/") {
				continue
			}
			body, ok := expected[r.URI]
			if !ok || seen[r.URI] || r.MIMEType != installedPilotMIME(r.URI) || r.Size != int64(len(body)) || r.Meta["sha256"] != installedPilotSHA(body) {
				return errors.New("installed_resource_inventory_mismatch")
			}
			seen[r.URI] = true
		}
		if p.NextCursor == "" {
			break
		}
		if len(p.NextCursor) > 4096 || cursors[p.NextCursor] {
			return errors.New("installed_resource_cursor")
		}
		cursors[p.NextCursor] = true
		params.Cursor = p.NextCursor
	}
	if len(seen) != len(expected) {
		return errors.New("installed_resource_missing")
	}
	uris := make([]string, 0, len(expected))
	for uri := range expected {
		uris = append(uris, uri)
	}
	sort.Strings(uris)
	for _, uri := range uris {
		cc, cancel := context.WithTimeout(ctx, 20*time.Second)
		read, e := s.ReadResource(cc, &mcp.ReadResourceParams{URI: uri})
		cancel()
		if e != nil {
			return e
		}
		if read == nil || len(read.Contents) != 1 || read.Contents[0] == nil || read.Contents[0].URI != uri || read.Contents[0].MIMEType != installedPilotMIME(uri) || read.Contents[0].Text != string(expected[uri]) || len(read.Contents[0].Blob) != 0 {
			return errors.New("installed_resource_content_mismatch")
		}
	}
	return nil
}

func installedPilotMIME(uri string) string {
	if strings.HasSuffix(uri, ".md") {
		return "text/markdown"
	}
	if strings.HasSuffix(uri, ".json") {
		return "application/json"
	}
	return "text/plain"
}

func installedPilotSHA(b []byte) string { return fmt.Sprintf("%x", sha256.Sum256(b)) }

func installedPilotConvert(ctx context.Context, s *mcp.ClientSession, id, manifest, pack string) error {
	var created struct {
		Result     map[string]any `json:"result"`
		Provenance map[string]any `json:"provenance"`
	}
	if err := installedPilotCall(ctx, s, "content_convert", "maestro_apply", map[string]any{"operation": "content.convert", "source_path": pilotdocument.Name}, &created); err != nil {
		return err
	}
	result, prov := created.Result, created.Provenance
	artifact, _ := result["artifact_ref"].(string)
	for k, want := range map[string]any{"status": "usable", "adapter": "docling", "workspace_id": id, "source_sha256": pilotdocument.SHA256, "source_name": pilotdocument.Name, "route": "docling_local", "parser": "docling.docx", "fidelity": "structured"} {
		if result[k] != want {
			return errors.New("synthetic_conversion_result_mismatch")
		}
	}
	if !pilotKitID.MatchString(artifact) || prov["pack_sha256"] != pack || prov["manifest_sha256"] != manifest || prov["network_denied"] != true {
		return errors.New("synthetic_conversion_provenance_mismatch")
	}
	stored := map[string]any{}
	for k, v := range result {
		stored[k] = v
	}
	delete(stored, "artifact_ref") // The immutable record cannot hash its own content address.
	outputs := map[string][]byte{}
	total := 0
	for _, format := range []string{"markdown", "document"} {
		offset := 0
		var complete []byte
		for page := 0; ; page++ {
			if page >= 128 {
				return errors.New("synthetic_content_page_bound")
			}
			var read struct {
				ID         string         `json:"artifact_id"`
				Format     string         `json:"format"`
				Text       string         `json:"text"`
				More       bool           `json:"has_more"`
				Next       int            `json:"next_offset"`
				Result     map[string]any `json:"result"`
				Provenance map[string]any `json:"provenance"`
			}
			if err := installedPilotCall(ctx, s, "content_read", "maestro_query", map[string]any{"operation": "content.read", "artifact_id": artifact, "format": format, "offset": offset, "max_bytes": 4096}, &read); err != nil {
				return err
			}
			if read.ID != artifact || read.Format != format || !reflect.DeepEqual(stored, read.Result) || !reflect.DeepEqual(prov, read.Provenance) || !utf8.ValidString(read.Text) || len(read.Text) > 4096 {
				return errors.New("synthetic_content_identity_mismatch")
			}
			complete = append(complete, []byte(read.Text)...)
			if len(complete) > 512<<10 {
				return errors.New("synthetic_content_output_bound")
			}
			if read.Next != len(complete) {
				return errors.New("synthetic_content_continuation_mismatch")
			}
			if !read.More {
				break
			}
			if read.Next <= offset || read.Next != len(complete) {
				return errors.New("synthetic_content_continuation_mismatch")
			}
			offset = read.Next
		}
		if prov[format+"_sha256"] != installedPilotSHA(complete) {
			return errors.New("synthetic_content_hash_mismatch")
		}
		outputs[format] = complete
		total += len(complete)
	}
	if result["output_bytes"] != float64(total) {
		return errors.New("synthetic_content_byte_count_mismatch")
	}
	var doc struct {
		Schema string `json:"schema_name"`
		Texts  []struct {
			Text string `json:"text"`
		} `json:"texts"`
		Tables []struct {
			Data struct {
				Cells []struct {
					Text string `json:"text"`
					Row  int    `json:"start_row_offset_idx"`
					Col  int    `json:"start_col_offset_idx"`
				} `json:"table_cells"`
			} `json:"data"`
		} `json:"tables"`
	}
	if json.Unmarshal(outputs["document"], &doc) != nil || doc.Schema != "DoclingDocument" {
		return errors.New("synthetic_document_invalid")
	}
	marker := false
	for _, t := range doc.Texts {
		marker = marker || strings.Contains(t.Text, "MAESTRO_DOCLING_OFFLINE_20261101")
	}
	if !marker || len(doc.Tables) != 1 {
		return errors.New("synthetic_document_content_mismatch")
	}
	cells := map[[2]int]string{}
	for _, c := range doc.Tables[0].Data.Cells {
		cells[[2]int{c.Row, c.Col}] = c.Text
	}
	for pos, want := range map[[2]int]string{{0, 0}: "Item", {0, 1}: "Count", {1, 0}: "alpha", {1, 1}: "7"} {
		if cells[pos] != want {
			return errors.New("synthetic_document_table_mismatch")
		}
	}
	markdown := strings.ReplaceAll(string(outputs["markdown"]), `\_`, "_")
	if !strings.Contains(markdown, "MAESTRO_DOCLING_OFFLINE_20261101") || !strings.Contains(markdown, "alpha") || !strings.Contains(markdown, "7") {
		return errors.New("synthetic_markdown_content_mismatch")
	}
	return nil
}

func installedPilotAttemptRefusal(ctx context.Context, s *mcp.ClientSession, args map[string]any) error {
	cc, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	out, err := s.CallTool(cc, &mcp.CallToolParams{Name: "maestro_apply", Arguments: args})
	if err != nil {
		return err
	}
	if !out.IsError || len(out.Content) != 1 {
		return errors.New("stale_attempt_not_refused")
	}
	text, ok := out.Content[0].(*mcp.TextContent)
	if !ok || len(text.Text) > 4096 {
		return errors.New("stale_attempt_refusal_invalid")
	}
	var value struct {
		Code string `json:"code"`
	}
	if json.Unmarshal([]byte(text.Text), &value) != nil || value.Code != "attempt_conflict" {
		return errors.New("stale_attempt_refusal_wrong_code")
	}
	return nil
}
