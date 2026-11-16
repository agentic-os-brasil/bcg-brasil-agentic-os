package product_test

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	product "github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/atlasops"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ingest/docling"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspace"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const ownerKnowledgeFixtureLabel = "SYNTHETIC_ADAPTER_NOT_REAL_CONVERSION"

// This seeds an adapter artifact, not an installed converter or conversion receipt.
// Every source/output/fixture-pack digest is computed from the named synthetic bytes.
func ownerKnowledgeArtifactFixture(t *testing.T, f ownerFixture, cs *mcp.ClientSession, source string) (string, string) {
	t.Helper()
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("existing source revalidation is unavailable; no native document-knowledge qualification")
	}
	t.Log(ownerKnowledgeFixtureLabel)
	meta := contentCall(t, cs, "maestro_context", map[string]any{}, false)
	id := meta["workspace_id"].(string)
	raw := []byte("# " + ownerKnowledgeFixtureLabel + "\n\nExplicit internal professional evidence.\n")
	putContentFile(t, filepath.Join(f.options.WorkspacePath, filepath.FromSlash(source)), raw)
	sum := func(body []byte) string { return fmt.Sprintf("%x", sha256.Sum256(body)) }
	fixtureRoot := filepath.Join(filepath.Dir(f.options.UserHome), "synthetic-adapter-fixture")
	values := map[string][]byte{"pack": []byte(ownerKnowledgeFixtureLabel + ":not-an-executable-runtime-pack"), "manifest": []byte(ownerKnowledgeFixtureLabel + ":not-a-release-manifest"), "lock": []byte(ownerKnowledgeFixtureLabel + ":not-a-runtime-lock")}
	for name, body := range values {
		putContentFile(t, filepath.Join(fixtureRoot, name), body)
	}
	document, _ := json.Marshal(map[string]string{"fixture": ownerKnowledgeFixtureLabel, "text": string(raw)})
	artifact := map[string]any{"scope": filepath.Dir(f.root), "workspace_id": id, "workspace_path": f.options.WorkspacePath, "source_path": source,
		"result":     map[string]any{"schema_version": 1, "adapter": "docling", "route": ownerKnowledgeFixtureLabel, "status": "usable", "fidelity": "textual", "source_name": filepath.Base(source), "source_sha256": sum(raw), "format": ".md", "workspace_id": id, "output_bytes": len(raw) + len(document)},
		"provenance": map[string]any{"pack_sha256": sum(values["pack"]), "manifest_sha256": sum(values["manifest"]), "lock_sha256": sum(values["lock"]), "markdown_sha256": sum(raw), "document_sha256": sum(document), "python": ownerKnowledgeFixtureLabel, "docling": ownerKnowledgeFixtureLabel, "network_denied": true}, "markdown": raw, "document": document}
	body, err := json.Marshal(artifact)
	if err != nil {
		t.Fatal(err)
	}
	artifactID := sum(body)
	putContentFile(t, filepath.Join(filepath.Dir(f.root), "state", "content", id, "artifacts", artifactID+".json"), body)
	read := contentCall(t, cs, "maestro_query", map[string]any{"operation": "content.read", "artifact_id": artifactID, "format": "markdown"}, false)
	if read["text"] != string(raw) {
		t.Fatal("synthetic adapter artifact failed unchanged canonical reader")
	}
	return artifactID, id
}
func prepareOwnerKnowledgeSetup(t *testing.T, cs *mcp.ClientSession, mode string) map[string]any {
	t.Helper()
	fields := map[string]any{"operation": "case.setup.prepare", "mode": mode, "project_name": "Synthetic owner knowledge", "expected_setup_revision": "absent"}
	if mode == "client" {
		fields["account_id"] = "synthetic"
		fields["account_owner"] = "owner"
		fields["account_mandate"] = "Preserve synthetic client scope"
	}
	prepared := contentCall(t, cs, "maestro_apply", fields, false)
	setup := prepared["setup"].(map[string]any)
	id := prepared["workspace_id"].(string)
	page := setup["brief_page"].(string)
	text := authoredBrief(id, page, "Synthetic owner knowledge setup")
	if mode == "client" {
		text = strings.Replace(strings.Replace(text, "scope: workspace/"+id, "scope: account/synthetic/case/"+id, 1), "sensitivity: internal", "sensitivity: client-confidential", 1)
	}
	created := brainCall(t, cs, "create", map[string]any{"page": page, "text": text}, false)
	return contentCall(t, cs, "maestro_apply", map[string]any{"operation": "case.setup.finish", "expected_setup_revision": prepared["setup_revision"], "expected_page_revision": created["revision"]}, false)
}
func TestOwnerKnowledgePublicSourceBoundJourney(t *testing.T) {
	for _, stdio := range []bool{false, true} {
		t.Run(fmt.Sprint(stdio), func(t *testing.T) {
			f := newOwnerFixture(t, stdio)
			knowledgeReviews := 0
			accept := func(_ context.Context, req *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
				if strings.Contains(req.Params.Message, "source-bound.md") {
					knowledgeReviews++
					for _, field := range []string{"Owner-global private", "Source document", "Source SHA-256", "Converted artifact", "Runtime pack SHA-256", "Source workspace", "original source and converted artifact remain", "Exact proposed text"} {
						if !strings.Contains(req.Params.Message, field) {
							t.Errorf("full promotion review missing %s", field)
						}
					}
				}
				return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"approved": true}}, nil
			}
			cs := f.connect(t, accept)
			prepareOwnerKnowledgeSetup(t, cs, "internal")
			contentCall(t, cs, "maestro_apply", map[string]any{"operation": "owner.atlas.prepare"}, false)
			artifactID, workspaceID := ownerKnowledgeArtifactFixture(t, f, cs, "materials/case-study.md")
			sourcePath := filepath.Join(f.options.WorkspacePath, "materials/case-study.md")
			sourceBefore := readContentFile(t, sourcePath)
			artifactPath := filepath.Join(filepath.Dir(f.root), "state/content", workspaceID, "artifacts", artifactID+".json")
			artifactBefore := readContentFile(t, artifactPath)
			body := "# Synthetic source-bound knowledge\n\n## Evidence\n\nReviewed adapter fixture.\n"
			create := map[string]any{"operation": "knowledge.create", "knowledge_destination": "owner", "artifact_id": artifactID, "page": "concepts/source-bound.md", "text": body}
			if !stdio {
				bound, e := product.Open(f.options)
				if e != nil {
					t.Fatal(e)
				}
				mutation := product.KnowledgeMutation{Operation: "knowledge.create", KnowledgeDestination: "owner", ArtifactID: artifactID, Page: "concepts/source-bound.md", Text: body}
				ownerReview, e := bound.KnowledgeReview(mutation)
				if e != nil {
					t.Fatal(e)
				}
				retargeted := mutation
				retargeted.KnowledgeDestination = "workspace"
				if _, e = bound.ApplyKnowledgeReview(retargeted, ownerReview.ReviewDigest, true); e == nil {
					t.Fatal("owner promotion review authorized workspace retarget")
				}
			}
			before := methodTreeSnapshot(t, f.options.UserHome)
			out, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "maestro_apply", Arguments: create})
			if err != nil {
				t.Fatal(err)
			}
			if out.IsError {
				after := methodTreeSnapshot(t, f.options.UserHome)
				t.Fatalf("public owner destination refused; existing owner state preserved=%v: %s", reflect.DeepEqual(before, after), out.Content[0].(*mcp.TextContent).Text)
			}
			var written map[string]any
			if err = json.Unmarshal([]byte(out.Content[0].(*mcp.TextContent).Text), &written); err != nil {
				t.Fatal(err)
			}
			if written["state"] != "written" {
				t.Fatal("owner source-bound page not written", written)
			}
			if !strings.Contains(written["provenance"].(map[string]any)["occurrence_id"].(string), workspaceID) {
				t.Fatal("durable provenance lost original workspace")
			}
			appendArgs := map[string]any{"operation": "knowledge.append", "knowledge_destination": "owner", "artifact_id": artifactID, "page": "concepts/source-bound.md", "section": "## Evidence", "text": "- Exactly once source-bound addition.", "expected_page_revision": written["revision"]}
			appended := contentCall(t, cs, "maestro_apply", appendArgs, false)
			replay := contentCall(t, cs, "maestro_apply", appendArgs, false)
			if replay["operation_id"] != appended["operation_id"] || replay["revision"] != appended["revision"] {
				t.Fatal("exact owner append replay diverged", replay)
			}
			stale := map[string]any{}
			for key, value := range appendArgs {
				stale[key] = value
			}
			stale["text"] = "stale entry must not be saved"
			stale["expected_page_revision"] = strings.Repeat("0", 64)
			rejected := contentCall(t, cs, "maestro_apply", stale, false)
			if rejected["state"] != "proposed" {
				t.Fatal("stale owner append changed the page", rejected)
			}
			if err = cs.Close(); err != nil {
				t.Fatal(err)
			}
			cs = f.connect(t, accept)
			restartedReplay := contentCall(t, cs, "maestro_apply", appendArgs, false)
			if restartedReplay["operation_id"] != appended["operation_id"] {
				t.Fatal("restart lost owner append identity")
			}
			read := contentCall(t, cs, "maestro_query", map[string]any{"operation": "owner.atlas.collect", "purpose": "Read the exact reviewed owner document knowledge", "pages": []string{"concepts/source-bound.md"}}, false)
			page := read["pages"].([]any)[0].(map[string]any)
			text := page["content"].(string)
			if page["revision"] != appended["revision"] || strings.Count(text, "Exactly once source-bound addition.") != 1 || strings.Contains(text, "stale entry") {
				t.Fatal("owner restarted readback differs", read)
			}
			if _, err = os.Stat(filepath.Join(f.options.WorkspacePath, "brain", "concepts", "source-bound.md")); !os.IsNotExist(err) {
				t.Fatal("owner destination wrote workspace page", err)
			}
			if string(readContentFile(t, sourcePath)) != string(sourceBefore) || string(readContentFile(t, artifactPath)) != string(artifactBefore) {
				t.Fatal("promotion changed source or moved/rebound artifact")
			}
			if knowledgeReviews < 4 {
				t.Fatal("owner effects omitted exact native review", knowledgeReviews)
			}
		})
	}
}

func TestOwnerKnowledgeRejectsUnadmittedPromotion(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("source reader unavailable on this platform")
	}
	for _, mode := range []string{"no-setup", "pending", "client", "selection-conflict", "owner-unprepared", "account-source", "client-source", "private-facet", "generated-memory", "invalid-destination"} {
		t.Run(mode, func(t *testing.T) {
			f := newOwnerFixture(t, false)
			reviews := 0
			accept := func(context.Context, *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
				reviews++
				return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"approved": true}}, nil
			}
			cs := f.connect(t, accept)
			switch mode {
			case "no-setup":
			case "pending":
				contentCall(t, cs, "maestro_apply", map[string]any{"operation": "case.setup.prepare", "mode": "internal", "project_name": "Pending internal setup", "expected_setup_revision": "absent"}, false)
			case "client":
				prepareOwnerKnowledgeSetup(t, cs, "client")
			default:
				prepareOwnerKnowledgeSetup(t, cs, "internal")
			}
			if mode != "owner-unprepared" {
				contentCall(t, cs, "maestro_apply", map[string]any{"operation": "owner.atlas.prepare"}, false)
			}
			source := "materials/case-study.md"
			switch mode {
			case "account-source":
				source = "brain/accounts/legacy/internal-note.md"
			case "client-source":
				source = "brain/clients/legacy.md"
			}
			artifactID, _ := ownerKnowledgeArtifactFixture(t, f, cs, source)
			if mode == "owner-unprepared" {
				queryBefore := methodTreeSnapshot(t, f.options.UserHome)
				status := contentCall(t, cs, "maestro_query", map[string]any{"operation": "owner.atlas.status"}, false)
				if status["prepared"] != false {
					t.Fatal("absent owner atlas appears prepared")
				}
				contentCall(t, cs, "maestro_query", map[string]any{"operation": "owner.atlas.collect", "purpose": "Inspect absent owner corpus without preparation", "pages": []string{"concepts/absent.md"}}, true)
				if !reflect.DeepEqual(queryBefore, methodTreeSnapshot(t, f.options.UserHome)) {
					t.Fatal("owner corpus query initialized state")
				}
			}
			if mode == "selection-conflict" {
				putContentFile(t, filepath.Join(f.options.WorkspacePath, "brain/accounts/.pending"), []byte("foreign/case\n"))
			}
			page := "concepts/refused.md"
			destination := "owner"
			switch mode {
			case "private-facet":
				page = "owner/self/working-boundaries.md"
			case "generated-memory":
				page = "memory/current.md"
			case "invalid-destination":
				destination = "owner-self"
			}
			before := methodTreeSnapshot(t, f.options.UserHome)
			reviewCount := reviews
			contentCall(t, cs, "maestro_apply", map[string]any{"operation": "knowledge.create", "knowledge_destination": destination, "artifact_id": artifactID, "page": page, "text": "# Must not be promoted\n"}, true)
			if reviews != reviewCount || !reflect.DeepEqual(before, methodTreeSnapshot(t, f.options.UserHome)) {
				t.Fatal("unadmitted promotion elicited approval or changed owner state")
			}
		})
	}
}

func TestOwnerKnowledgePostReviewChangesAndRefusalsPreserveState(t *testing.T) {
	for _, mode := range []string{"declined", "unavailable", "source-changed", "artifact-changed", "scope-changed", "page-changed"} {
		t.Run(mode, func(t *testing.T) {
			f := newOwnerFixture(t, false)
			accept := func(context.Context, *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
				return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"approved": true}}, nil
			}
			setupClient := f.connect(t, accept)
			prepareOwnerKnowledgeSetup(t, setupClient, "internal")
			contentCall(t, setupClient, "maestro_apply", map[string]any{"operation": "owner.atlas.prepare"}, false)
			artifactID, workspaceID := ownerKnowledgeArtifactFixture(t, f, setupClient, "materials/case-study.md")
			initial := contentCall(t, setupClient, "maestro_apply", map[string]any{"operation": "knowledge.create", "knowledge_destination": "owner", "artifact_id": artifactID, "page": "concepts/preserved.md", "text": "# Preserved owner knowledge\n\n## Evidence\n\nOriginal reviewed body.\n"}, false)
			ownerPath := filepath.Join(filepath.Dir(f.root), "atlas/owner/concepts/preserved.md")
			before := readContentFile(t, ownerPath)
			expected := string(before)
			var handler func(context.Context, *mcp.ElicitRequest) (*mcp.ElicitResult, error)
			if mode != "unavailable" {
				handler = func(context.Context, *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
					switch mode {
					case "declined":
						return &mcp.ElicitResult{Action: "decline"}, nil
					case "source-changed":
						putContentFile(t, filepath.Join(f.options.WorkspacePath, "materials/case-study.md"), []byte("changed selected source"))
					case "artifact-changed":
						path := filepath.Join(filepath.Dir(f.root), "state/content", workspaceID, "artifacts", artifactID+".json")
						putContentFile(t, path, append(readContentFile(t, path), ' '))
					case "scope-changed":
						putContentFile(t, filepath.Join(f.options.WorkspacePath, "brain/accounts/.active"), []byte("foreign/case\n"))
					case "page-changed":
						expected += "\nOWNER NEWER EDIT\n"
						putContentFile(t, ownerPath, []byte(expected))
					}
					return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"approved": true}}, nil
				}
			}
			cs := f.connect(t, handler)
			contentCall(t, cs, "maestro_apply", map[string]any{"operation": "knowledge.append", "knowledge_destination": "owner", "artifact_id": artifactID, "page": "concepts/preserved.md", "section": "## Evidence", "text": "REJECTED APPEND", "expected_page_revision": initial["revision"]}, true)
			after := string(readContentFile(t, ownerPath))
			if after != expected || strings.Contains(after, "REJECTED APPEND") {
				t.Fatal("refused or changed review lost owner work")
			}
		})
	}
}

func TestOwnerKnowledgeWorkspaceDefaultsRetainRequestIdentity(t *testing.T) {
	f := newOwnerFixture(t, false)
	cs := f.connect(t, func(context.Context, *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"approved": true}}, nil
	})
	artifactID, _ := ownerKnowledgeArtifactFixture(t, f, cs, "materials/legacy.md")
	in := product.KnowledgeMutation{Operation: "knowledge.create", ArtifactID: artifactID, Page: "knowledge/legacy.md", Text: "# Legacy exact workspace request\n"}
	type legacyMutation struct {
		Operation            string `json:"operation"`
		ArtifactID           string `json:"artifact_id"`
		Page                 string `json:"page"`
		Section              string `json:"section,omitempty"`
		Text                 string `json:"text"`
		ExpectedPageRevision string `json:"expected_page_revision,omitempty"`
	}
	old := legacyMutation{Operation: in.Operation, ArtifactID: in.ArtifactID, Page: in.Page, Text: in.Text}
	expected, _ := json.Marshal(old)
	actual, _ := json.Marshal(in)
	if string(expected) != string(actual) {
		t.Fatal("omitted destination changed legacy mutation bytes")
	}
	bound, err := product.Open(f.options)
	if err != nil {
		t.Fatal(err)
	}
	reviewed, err := bound.KnowledgeReview(in)
	if err != nil {
		t.Fatal(err)
	}
	type legacyReview struct {
		Scope        string                   `json:"scope"`
		WorkspaceID  string                   `json:"workspace_id"`
		Mutation     legacyMutation           `json:"mutation"`
		SourcePath   string                   `json:"source_path"`
		SourceSHA256 string                   `json:"source_sha256"`
		Conversion   docling.Provenance       `json:"conversion"`
		Current      *atlasops.PageProjection `json:"current,omitempty"`
		Provenance   atlasops.Provenance      `json:"provenance"`
		ReviewDigest string                   `json:"review_digest"`
	}
	legacyIntent := legacyReview{Scope: reviewed.Scope, WorkspaceID: reviewed.WorkspaceID, Mutation: old, SourcePath: reviewed.SourcePath, SourceSHA256: reviewed.SourceSHA256, Conversion: reviewed.Conversion}
	oldIntent, _ := json.Marshal(legacyIntent)
	oldID := fmt.Sprintf("%x", sha256.Sum256(oldIntent))
	if reviewed.Provenance.IdempotencyKey != oldID || reviewed.Provenance.SessionID != "content-"+oldID || reviewed.Provenance.Ritual != "content:"+artifactID || reviewed.Provenance.OccurrenceID != "" {
		t.Fatal("workspace legacy intent/provenance identity changed")
	}
	explicit := in
	explicit.KnowledgeDestination = "workspace"
	explicitReview, err := bound.KnowledgeReview(explicit)
	if err != nil || !reflect.DeepEqual(explicitReview, reviewed) {
		t.Fatal("explicit workspace differs from omitted destination", err)
	}
	args := map[string]any{"operation": in.Operation, "artifact_id": artifactID, "page": in.Page, "text": in.Text}
	written := contentCall(t, cs, "maestro_apply", args, false)
	args["knowledge_destination"] = "workspace"
	replayed := contentCall(t, cs, "maestro_apply", args, false)
	if written["operation_id"] != replayed["operation_id"] || written["revision"] != replayed["revision"] {
		t.Fatal("workspace normalization broke canonical replay")
	}
	if _, err = os.Stat(filepath.Join(filepath.Dir(f.root), "atlas/owner")); !os.IsNotExist(err) {
		t.Fatal("default workspace created owner atlas", err)
	}
	foreign := f
	foreign.options.WorkspacePath = filepath.Join(filepath.Dir(f.options.WorkspacePath), "foreign-workspace")
	if _, err = workspace.Initialize(workspace.Options{WorkspacePath: foreign.options.WorkspacePath, DataRoot: filepath.Join(filepath.Dir(f.root), "state")}); err != nil {
		t.Fatal(err)
	}
	foreignClient := foreign.connect(t, nil)
	contentCall(t, foreignClient, "maestro_apply", map[string]any{"operation": "knowledge.create", "knowledge_destination": "owner", "artifact_id": artifactID, "page": "concepts/foreign.md", "text": "# Foreign artifact must remain unavailable\n"}, true)
	// Copying identical artifact/source bytes cannot transfer the opaque workspace identity.
	foreignPrepared := foreign.connect(t, func(context.Context, *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"approved": true}}, nil
	})
	prepareOwnerKnowledgeSetup(t, foreignPrepared, "internal")
	contentCall(t, foreignPrepared, "maestro_apply", map[string]any{"operation": "owner.atlas.prepare"}, false)
	foreignMeta := contentCall(t, foreignPrepared, "maestro_context", map[string]any{}, false)
	originalMeta := contentCall(t, cs, "maestro_context", map[string]any{}, false)
	originalArtifact := filepath.Join(filepath.Dir(f.root), "state/content", originalMeta["workspace_id"].(string), "artifacts", artifactID+".json")
	putContentFile(t, filepath.Join(filepath.Dir(f.root), "state/content", foreignMeta["workspace_id"].(string), "artifacts", artifactID+".json"), readContentFile(t, originalArtifact))
	putContentFile(t, filepath.Join(foreign.options.WorkspacePath, "materials/legacy.md"), readContentFile(t, filepath.Join(f.options.WorkspacePath, "materials/legacy.md")))
	ownerBefore := methodTreeSnapshot(t, filepath.Join(filepath.Dir(f.root), "atlas/owner"))
	contentCall(t, foreignPrepared, "maestro_apply", map[string]any{"operation": "knowledge.create", "knowledge_destination": "owner", "artifact_id": artifactID, "page": "concepts/foreign.md", "text": "# Copied artifact remains foreign\n"}, true)
	if !reflect.DeepEqual(ownerBefore, methodTreeSnapshot(t, filepath.Join(filepath.Dir(f.root), "atlas/owner"))) {
		t.Fatal("copied artifact gained owner effect authority")
	}
}

func TestOwnerKnowledgeRejectsUnattestedArtifactAndModelAssertions(t *testing.T) {
	f := newOwnerFixture(t, false)
	reviews := 0
	cs := f.connect(t, func(context.Context, *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		reviews++
		return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"approved": true}}, nil
	})
	prepareOwnerKnowledgeSetup(t, cs, "internal")
	contentCall(t, cs, "maestro_apply", map[string]any{"operation": "owner.atlas.prepare"}, false)
	artifactID, workspaceID := ownerKnowledgeArtifactFixture(t, f, cs, "materials/internal.md")
	dir := filepath.Join(filepath.Dir(f.root), "state/content", workspaceID, "artifacts")
	raw := readContentFile(t, filepath.Join(dir, artifactID+".json"))
	for _, field := range []string{"pack", "network", "adapter", "status", "workspace"} {
		t.Run(field, func(t *testing.T) {
			var artifact map[string]any
			if err := json.Unmarshal(raw, &artifact); err != nil {
				t.Fatal(err)
			}
			switch field {
			case "pack":
				artifact["provenance"].(map[string]any)["pack_sha256"] = "unattested"
			case "network":
				artifact["provenance"].(map[string]any)["network_denied"] = false
			case "adapter":
				artifact["result"].(map[string]any)["adapter"] = "unknown"
			case "status":
				artifact["result"].(map[string]any)["status"] = "partial"
			case "workspace":
				artifact["workspace_id"] = strings.Repeat("0", 32)
			}
			body, err := json.Marshal(artifact)
			if err != nil {
				t.Fatal(err)
			}
			id := fmt.Sprintf("%x", sha256.Sum256(body))
			putContentFile(t, filepath.Join(dir, id+".json"), body)
			before := methodTreeSnapshot(t, filepath.Join(filepath.Dir(f.root), "atlas/owner"))
			count := reviews
			contentCall(t, cs, "maestro_apply", map[string]any{"operation": "knowledge.create", "knowledge_destination": "owner", "artifact_id": id, "page": "concepts/rejected-artifact.md", "text": "# Unattested artifact must remain rejected\n"}, true)
			if count != reviews || !reflect.DeepEqual(before, methodTreeSnapshot(t, filepath.Join(filepath.Dir(f.root), "atlas/owner"))) {
				t.Fatal("invalid artifact crossed canonical admission")
			}
		})
	}
	contentCall(t, cs, "maestro_apply", map[string]any{"operation": "knowledge.create", "knowledge_destination": "owner", "artifact_id": artifactID, "page": "concepts/model-assertion.md", "text": "# A model boolean is not authority\n", "no_client_data": true}, true)
}
