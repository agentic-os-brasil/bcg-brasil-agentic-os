package product_test

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	product "github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/atlas"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/atlasops"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ingest"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ingest/docling"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestAuthoredPageMovePreservesIdentityAcrossRestart(t *testing.T) {
	for _, tc := range []struct {
		name  string
		stdio bool
	}{{"sdk", false}, {"stdio", true}} {
		t.Run(tc.name, func(t *testing.T) {
			f := newOwnerFixture(t, tc.stdio)
			cs := f.connect(t, cadenceApprove)
			workspace := contentCall(t, cs, "maestro_context", map[string]any{}, false)["workspace_id"].(string)
			brainCall(t, cs, "prepare", nil, false)
			const source, target = "projects/lifecycle-first.md", "projects/lifecycle-current.md"
			body := authoredBrief(workspace, source, "Lifecycle proof")
			written := brainCall(t, cs, "create", map[string]any{"page": source, "text": body}, false)
			moved := contentCall(t, cs, "maestro_apply", map[string]any{"operation": "brain.move", "page": source, "target": target, "expected_lifecycle_revision": 0, "expected_page_revision": written["revision"]}, false)
			if moved["state"] != "written" || moved["stable_id"] != "projects/lifecycle-first" || moved["location"] != target {
				t.Fatal("reviewed move did not preserve identity and current location", moved)
			}
			if err := cs.Close(); err != nil {
				t.Fatal(err)
			}
			cs = f.connect(t, nil)
			for _, name := range []string{source, target} {
				read := brainCall(t, cs, "read", map[string]any{"page": name, "purpose": "Continue the moved authored page"}, false)
				if read["text"] != body || read["stable_id"] != "projects/lifecycle-first" || read["page"] != target {
					t.Fatal("restarted reader lost stable identity or current location", read)
				}
			}
		})
	}
}

func TestAuthoredPageRetirementHidesMovedIdentityAcrossRestart(t *testing.T) {
	for _, tc := range []struct {
		name  string
		stdio bool
	}{{"sdk", false}, {"stdio", true}} {
		t.Run(tc.name, func(t *testing.T) {
			f := newOwnerFixture(t, tc.stdio)
			cs := f.connect(t, cadenceApprove)
			workspace := contentCall(t, cs, "maestro_context", map[string]any{}, false)["workspace_id"].(string)
			brainCall(t, cs, "prepare", nil, false)
			const source, target = "projects/lifecycle-archive-original.md", "projects/lifecycle-archive-current.md"
			body := authoredBrief(workspace, source, "Archived lifecycle proof")
			written := brainCall(t, cs, "create", map[string]any{"page": source, "text": body}, false)
			moved := contentCall(t, cs, "maestro_apply", map[string]any{"operation": "brain.move", "page": source, "target": target, "expected_lifecycle_revision": 0, "expected_page_revision": written["revision"]}, false)
			retired := contentCall(t, cs, "maestro_apply", map[string]any{"operation": "brain.retire", "page": target, "expected_lifecycle_revision": moved["lifecycle_revision"], "expected_page_revision": moved["revision"]}, false)
			if retired["state"] != "written" || retired["stable_id"] != "projects/lifecycle-archive-original" || retired["location"] != target || retired["page_state"] != "retired" {
				t.Fatal("reviewed retirement lost identity or final archived state", retired)
			}
			if err := cs.Close(); err != nil {
				t.Fatal(err)
			}
			cs = f.connect(t, cadenceApprove)
			if runtime := contentCall(t, cs, "maestro_query", map[string]any{"operation": "content.status"}, false); runtime["state"] != "unavailable" {
				t.Fatal("source-revocation proof requires the fresh unavailable runtime, with no provider execution", runtime)
			}
			for _, name := range []string{source, target} {
				brainCall(t, cs, "read", map[string]any{"page": name, "purpose": "Verify retired content is unavailable after restart"}, true)
				contentCall(t, cs, "maestro_query", map[string]any{"operation": "knowledge.collect", "purpose": "Verify retired named knowledge stays unavailable", "pages": []string{name}}, true)
				brainCall(t, cs, "create", map[string]any{"page": name, "text": authoredBrief(workspace, name, "Reserved retired name")}, true)
				path := filepath.Join(f.options.WorkspacePath, "brain", filepath.FromSlash(name))
				putContentFile(t, path, []byte(body))
				brainCall(t, cs, "read", map[string]any{"page": name, "purpose": "Recreated bytes cannot renew retired authored authority"}, true)
				contentCall(t, cs, "maestro_apply", map[string]any{"operation": "content.convert", "source_path": "brain/" + name}, true)
				if string(readContentFile(t, path)) != body {
					t.Fatal("source revocation modified externally recreated bytes")
				}
			}
		})
	}
}

func TestAuthoredLifecyclePendingHasSafeNextAction(t *testing.T) {
	f := newOwnerFixture(t, false)
	cs := f.connect(t, cadenceApprove)
	workspace := contentCall(t, cs, "maestro_context", map[string]any{}, false)["workspace_id"].(string)
	brainCall(t, cs, "prepare", nil, false)
	putContentFile(t, filepath.Join(f.options.WorkspacePath, "brain", "accounts", ".active"), []byte("a/selected\n"))
	const source, target = "projects/pending.md", "accounts/a/cases/selected/pending.md"
	body := authoredBrief(workspace, source, "Private pending body must stay redacted")
	created := brainCall(t, cs, "create", map[string]any{"page": source, "text": body}, false)
	var engine *atlasops.Engine
	interrupted := errors.New("synthetic host admission failure after durable barrier")
	var err error
	engine, err = atlasops.OpenWorkspaceWithPolicy(atlas.Options{DataRoot: filepath.Join(filepath.Dir(f.root), "state"), WorkspacePath: f.options.WorkspacePath, WorkspaceID: workspace}, atlasops.WorkspacePolicy{BeforeMutation: func() error {
		revision, err := engine.LifecycleRevision()
		if err != nil {
			return err
		}
		if revision == 1 {
			return interrupted
		}
		return nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	request := atlasops.MovePageRequest{Page: source, Target: target, ExpectedRevision: created["revision"].(string), Provenance: atlasops.Provenance{Origin: atlasops.OriginAttended, SessionID: "pending-diagnostic", IdempotencyKey: "pending-diagnostic"}}
	shown, err := engine.ReviewMove(request)
	if err != nil {
		t.Fatal(err)
	}
	request.ReferencesDigest = shown.ReferencesDigest
	if _, err := engine.MovePage(request); !errors.Is(err, interrupted) {
		t.Fatalf("did not reach the real lifecycle barrier: %v", err)
	}
	for _, name := range []string{source, target} {
		pending := brainCall(t, cs, "read", map[string]any{"page": name, "purpose": "Inspect incomplete move without disclosing or repairing it"}, true)
		if pending["code"] != "brain_lifecycle_incomplete" || pending["state"] != "incomplete" || pending["lifecycle_revision"] != float64(1) || pending["next_action"] == "" || pending["next_action"] == nil {
			t.Fatal("pending lifecycle lost its safe metadata and attended next action", pending)
		}
		if strings.Contains(fmt.Sprint(pending), "Private pending body") || strings.Contains(fmt.Sprint(pending), f.options.WorkspacePath) {
			t.Fatal("pending diagnostic disclosed body or private filesystem details")
		}
	}
	putContentFile(t, filepath.Join(f.options.WorkspacePath, "brain", "accounts", ".active"), []byte("a/other\n"))
	unadmitted := brainCall(t, cs, "read", map[string]any{"page": source, "purpose": "Verify lost destination admission hides pending diagnostics"}, true)
	if unadmitted["state"] == "incomplete" || unadmitted["lifecycle_revision"] != nil || unadmitted["next_action"] != nil {
		t.Fatal("pending details disclosed after current case admission was lost", unadmitted)
	}
}

func TestAuthoredLifecycleScopeNarrowingReviewsExactReferences(t *testing.T) {
	f := newOwnerFixture(t, false)
	var moveReview map[string]any
	var intervene func()
	cs := f.connect(t, func(ctx context.Context, req *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		if strings.Contains(req.Params.Message, `"operation": "brain.move"`) {
			if err := json.Unmarshal([]byte(req.Params.Message[strings.Index(req.Params.Message, "{"):]), &moveReview); err != nil {
				t.Fatal(err)
			}
			if intervene != nil {
				intervene()
			}
		}
		return cadenceApprove(ctx, req)
	})
	workspace := contentCall(t, cs, "maestro_context", map[string]any{}, false)["workspace_id"].(string)
	brainCall(t, cs, "prepare", nil, false)
	marker := filepath.Join(f.options.WorkspacePath, "brain", "accounts", ".active")
	putContentFile(t, marker, []byte("a/selected\n"))
	const source, target, referring = "projects/scoped.md", "accounts/a/cases/selected/scoped.md", "projects/referring.md"
	body := strings.ReplaceAll(strings.Replace(authoredBrief(workspace, source, "Exact narrowing"), "updated:", "custom: preserve this optional field\nupdated:", 1), "\n", "\r\n")
	created := brainCall(t, cs, "create", map[string]any{"page": source, "text": body}, false)
	refBody := authoredBrief(workspace, referring, "Incoming reference") + "\n[Moved](scoped.md)\n"
	brainCall(t, cs, "create", map[string]any{"page": referring, "text": refBody}, false)
	foreign := filepath.Join(f.options.WorkspacePath, "brain", "accounts", "a", "cases", "foreign", "opaque.md")
	const foreignBody = "# Foreign case is excluded from the reference corpus\n\n[[opaque foreign syntax]]\n"
	putContentFile(t, foreign, []byte(foreignBody))
	moved := contentCall(t, cs, "maestro_apply", map[string]any{"operation": "brain.move", "page": source, "target": target, "expected_lifecycle_revision": 0, "expected_page_revision": created["revision"]}, false)
	proposed := strings.Replace(strings.Replace(body, "scope: workspace/"+workspace, `scope: "account/a/case/selected"`, 1), "sensitivity: internal", "sensitivity: client-confidential", 1)
	lifecycle := moveReview["lifecycle"].(map[string]any)
	if lifecycle["proposed"] != proposed || lifecycle["source"].(map[string]any)["content"] != body {
		t.Fatal("host review lost exact current/proposed bytes, scope or optional fields", moveReview)
	}
	found := false
	for _, value := range lifecycle["affected_references"].([]any) {
		ref := value.(map[string]any)
		found = found || ref["page"] == referring && ref["target"] == source
	}
	if !found || strings.Contains(fmt.Sprint(lifecycle), "opaque foreign syntax") {
		t.Fatal("reference review omitted admitted incoming link or read a foreign case", lifecycle)
	}
	current := brainCall(t, cs, "read", map[string]any{"page": source, "purpose": "Read exact narrowed page through its original identity"}, false)
	if current["text"] != proposed || current["page"] != target || current["stable_id"] != "projects/scoped" || string(readContentFile(t, foreign)) != foreignBody || string(readContentFile(t, filepath.Join(f.options.WorkspacePath, "brain", referring))) != refBody {
		t.Fatal("scope narrowing changed optional/prose/reference/outside bytes or identity", current)
	}
	for _, destination := range []string{"projects/widened.md", "accounts/a/cases/foreign/scoped.md", "accounts/b/cases/selected/scoped.md"} {
		contentCall(t, cs, "maestro_apply", map[string]any{"operation": "brain.move", "page": target, "target": destination, "expected_lifecycle_revision": moved["lifecycle_revision"], "expected_page_revision": moved["revision"]}, true)
	}
	const race = "projects/reference-race.md"
	raceCreated := brainCall(t, cs, "create", map[string]any{"page": race, "text": authoredBrief(workspace, race, "Reference race")}, false)
	refBody += "[Race](reference-race.md)\n"
	putContentFile(t, filepath.Join(f.options.WorkspacePath, "brain", referring), []byte(refBody))
	intervene = func() {
		putContentFile(t, filepath.Join(f.options.WorkspacePath, "brain", referring), []byte(refBody+"Changed after the exact host review.\n"))
	}
	conflict := contentCall(t, cs, "maestro_apply", map[string]any{"operation": "brain.move", "page": race, "target": "projects/reference-raced.md", "expected_lifecycle_revision": moved["lifecycle_revision"], "expected_page_revision": raceCreated["revision"]}, true)
	if conflict["code"] != "revision_conflict" || conflict["next_action"] == nil || conflict["next_action"] == "" {
		t.Fatal("changed references lost the conflict and safe next attended action", conflict)
	}
	if current := brainCall(t, cs, "read", map[string]any{"page": race, "purpose": "Preserve source when references change after review"}, false); current["revision"] != raceCreated["revision"] {
		t.Fatal("reference conflict moved or overwrote source", current)
	}
	putContentFile(t, marker, []byte("a/other\n"))
	brainCall(t, cs, "read", map[string]any{"page": source, "purpose": "Old alias cannot bypass current case admission"}, true)
}

func TestAuthoredLifecycleAliasesFenceABAAndPreserveRefusals(t *testing.T) {
	f := newOwnerFixture(t, false)
	approve := true
	cs := f.connect(t, func(ctx context.Context, req *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		if !approve {
			return &mcp.ElicitResult{Action: "decline"}, nil
		}
		return cadenceApprove(ctx, req)
	})
	workspace := contentCall(t, cs, "maestro_context", map[string]any{}, false)["workspace_id"].(string)
	brainCall(t, cs, "prepare", nil, false)
	const source, target, decomposed = "projects/café.md", "projects/current.md", "projects/café.md"
	body := authoredBrief(workspace, source, "Reserved normalized identity")
	created := brainCall(t, cs, "create", map[string]any{"page": source, "text": body}, false)
	moved := contentCall(t, cs, "maestro_apply", map[string]any{"operation": "brain.move", "page": source, "target": target, "expected_lifecycle_revision": 0, "expected_page_revision": created["revision"]}, false)
	for _, name := range []string{source, decomposed, "PROJECTS/CAFÉ.md"} {
		read := brainCall(t, cs, "read", map[string]any{"page": name, "purpose": "Resolve NFC and case aliases to the immutable identity"}, false)
		if read["page"] != target || read["stable_id"] != "projects/café" || read["text"] != body {
			t.Fatal("normalized alias changed identity or content", read)
		}
	}
	brainCall(t, cs, "create", map[string]any{"page": decomposed, "text": authoredBrief(workspace, decomposed, "Cannot reuse a reserved identity")}, true)
	returned := contentCall(t, cs, "maestro_apply", map[string]any{"operation": "brain.move", "page": target, "target": source, "expected_lifecycle_revision": moved["lifecycle_revision"], "expected_page_revision": moved["revision"]}, false)
	if returned["revision"] != created["revision"] || returned["lifecycle_revision"] != float64(4) {
		t.Fatal("return to an owned former location lost lifecycle ABA evidence", returned)
	}
	stale := contentCall(t, cs, "maestro_apply", map[string]any{"operation": "brain.move", "page": source, "target": "projects/stale.md", "expected_lifecycle_revision": 0, "expected_page_revision": created["revision"]}, true)
	if stale["code"] != "revision_conflict" {
		t.Fatal("same-content ABA bypassed lifecycle intention fence", stale)
	}
	const occupied = "projects/occupied.md"
	occupiedBody := authoredBrief(workspace, occupied, "Occupied destination")
	brainCall(t, cs, "create", map[string]any{"page": occupied, "text": occupiedBody}, false)
	contentCall(t, cs, "maestro_apply", map[string]any{"operation": "brain.move", "page": source, "target": occupied, "expected_lifecycle_revision": returned["lifecycle_revision"], "expected_page_revision": returned["revision"]}, true)
	anchor := brainCall(t, cs, "read", map[string]any{"page": "README.md", "purpose": "Verify registered orientation remains fixed"}, false)
	for _, operation := range []string{"brain.move", "brain.retire"} {
		args := map[string]any{"operation": operation, "page": "README.md", "expected_lifecycle_revision": returned["lifecycle_revision"], "expected_page_revision": anchor["revision"]}
		if operation == "brain.move" {
			args["target"] = "projects/README.md"
		}
		contentCall(t, cs, "maestro_apply", args, true)
	}
	approve = false
	declined := contentCall(t, cs, "maestro_apply", map[string]any{"operation": "brain.retire", "page": source, "expected_lifecycle_revision": returned["lifecycle_revision"], "expected_page_revision": returned["revision"]}, true)
	if declined["code"] != "confirmation_declined" {
		t.Fatal("declined archive did not preserve domain refusal", declined)
	}
	approve = true
	if current := brainCall(t, cs, "read", map[string]any{"page": source, "purpose": "Verify every refused operation preserved current source"}, false); current["text"] != body || current["lifecycle_revision"] != float64(4) {
		t.Fatal("a refusal changed source or lifecycle", current)
	}
	if current := brainCall(t, cs, "read", map[string]any{"page": occupied, "purpose": "Verify occupied destination was never overwritten"}, false); current["text"] != occupiedBody {
		t.Fatal("occupied target was overwritten", current)
	}
}

func TestAuthoredLifecycleInvalidatesConvertedArtifactAfterABA(t *testing.T) {
	f := newOwnerFixture(t, false)
	cs := f.connect(t, cadenceApprove)
	workspace := contentCall(t, cs, "maestro_context", map[string]any{}, false)["workspace_id"].(string)
	brainCall(t, cs, "prepare", nil, false)
	const source, target = "projects/artifact-source.md", "projects/artifact-moved.md"
	body := authoredBrief(workspace, source, "Synthetic conversion authority fixture")
	created := brainCall(t, cs, "create", map[string]any{"page": source, "text": body}, false)
	bound, err := product.Open(f.options)
	if err != nil {
		t.Fatal(err)
	}
	markdown, document := []byte("# Synthetic retained conversion; no converter execution\n"), []byte(`{"fixture":"not-runtime-evidence"}`)
	digest := func(body []byte) string { return fmt.Sprintf("%x", sha256.Sum256(body)) }
	conversion := docling.Conversion{Result: ingest.Result{SchemaVersion: 1, Adapter: "docling", Route: "synthetic-test-value", Status: ingest.StatusUsable, Fidelity: ingest.FidelityTextual, SourceName: "artifact-source.md", SourceSHA256: digest([]byte(body)), Format: "md", WorkspaceID: workspace, OutputBytes: int64(len(markdown) + len(document))}, Markdown: markdown, Document: document, Provenance: docling.Provenance{PackSHA256: strings.Repeat("1", 64), ManifestSHA256: strings.Repeat("2", 64), LockSHA256: strings.Repeat("3", 64), MarkdownSHA256: digest(markdown), DocumentSHA256: digest(document), Python: "synthetic-fixture-not-runtime", Docling: "synthetic-fixture-not-runtime", NetworkDenied: true}}
	artifact, err := product.PublishContentForTest(bound, "brain/"+source, conversion)
	if err != nil {
		t.Fatal("canonical synthetic artifact publication refused", err)
	}
	store := filepath.Join(filepath.Dir(f.root), "state", "content", workspace, "artifacts")
	var legacy map[string]any
	if err := json.Unmarshal(readContentFile(t, filepath.Join(store, artifact+".json")), &legacy); err != nil {
		t.Fatal(err)
	}
	delete(legacy, "source_lifecycle_revision")
	legacyBody, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	legacyID := digest(legacyBody)
	putContentFile(t, filepath.Join(store, legacyID+".json"), legacyBody)
	for _, id := range []string{artifact, legacyID} {
		positive := contentCall(t, cs, "maestro_query", map[string]any{"operation": "content.read", "artifact_id": id, "format": "markdown"}, false)
		if positive["text"] != string(markdown) {
			t.Fatal("fixture artifact lacked positive pre-ABA authority", positive)
		}
	}
	create := map[string]any{"operation": "knowledge.create", "artifact_id": artifact, "page": "knowledge/artifact-evidence.md", "text": "# Independently reviewed local knowledge\n\n## Evidence\n"}
	written := contentCall(t, cs, "maestro_apply", create, false)
	appendArgs := map[string]any{"operation": "knowledge.append", "artifact_id": artifact, "page": "knowledge/artifact-evidence.md", "section": "## Evidence", "text": "- Exactly one reviewed artifact excerpt.", "expected_page_revision": written["revision"]}
	contentCall(t, cs, "maestro_apply", appendArgs, false)
	moved := contentCall(t, cs, "maestro_apply", map[string]any{"operation": "brain.move", "page": source, "target": target, "expected_lifecycle_revision": 0, "expected_page_revision": created["revision"]}, false)
	returned := contentCall(t, cs, "maestro_apply", map[string]any{"operation": "brain.move", "page": target, "target": source, "expected_lifecycle_revision": moved["lifecycle_revision"], "expected_page_revision": moved["revision"]}, false)
	if returned["revision"] != created["revision"] {
		t.Fatal("ABA fixture did not restore identical source bytes")
	}
	for _, id := range []string{artifact, legacyID} {
		contentCall(t, cs, "maestro_query", map[string]any{"operation": "content.read", "artifact_id": id, "format": "markdown"}, true)
	}
	contentCall(t, cs, "maestro_apply", appendArgs, true)
	create["page"] = "knowledge/stale-artifact.md"
	contentCall(t, cs, "maestro_apply", create, true)
	fresh, err := product.PublishContentForTest(bound, "brain/"+source, conversion)
	if err != nil {
		t.Fatal("fresh current-cohort synthetic publication refused", err)
	}
	var produced map[string]any
	if err := json.Unmarshal(readContentFile(t, filepath.Join(store, fresh+".json")), &produced); err != nil {
		t.Fatal(err)
	}
	if produced["source_lifecycle_revision"] != float64(4) {
		t.Fatal("canonical artifact producer did not bind its authored cohort", produced)
	}
	contentCall(t, cs, "maestro_query", map[string]any{"operation": "content.read", "artifact_id": fresh, "format": "markdown"}, false)
	preserved := contentCall(t, cs, "maestro_query", map[string]any{"operation": "knowledge.collect", "purpose": "Preserve independently reviewed authored knowledge", "pages": []string{"knowledge/artifact-evidence.md"}}, false)
	if !strings.Contains(fmt.Sprint(preserved["pages"]), "Exactly one reviewed artifact excerpt.") {
		t.Fatal("artifact invalidation revoked independently reviewed knowledge", preserved)
	}
}
