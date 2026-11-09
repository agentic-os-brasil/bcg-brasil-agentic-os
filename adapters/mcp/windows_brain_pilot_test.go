//go:build windows

package product_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/atlas"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/atlasops"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspace"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// The existing credentialed driver supplies the actual shipping server and
// current-user private temp. This family never substitutes an elevated token.
func TestWindowsAuthoredBrainPilot(t *testing.T) {
	windowsPrivateOrdinaryUser(t)
	root := windowsPrivateRoot(t)
	product, home, workspacePath := filepath.Join(root, "product"), filepath.Join(root, "owner"), filepath.Join(root, "workspace")
	for _, p := range []string{product, home} {
		if err := os.Mkdir(p, 0700); err != nil {
			t.Fatal(err)
		}
	}
	server := os.Getenv("MAESTRO_WINDOWS_CORE_SERVER")
	args := []string{"--product-root", product, "--user-home", home, "--workspace", workspacePath}
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
	defer cancel()
	if out, err := exec.CommandContext(ctx, server, append([]string{"init"}, args...)...).CombinedOutput(); err != nil {
		t.Fatalf("public authored native setup: %s %v", out, err)
	}
	managed := windowsCoreTree(t, product)
	approved := true
	var reviews atomic.Int32
	var intervene func() error
	connect := func() *mcp.ClientSession {
		client := mcp.NewClient(&mcp.Implementation{Name: "windows-authored-brain-proof", Version: "1"}, &mcp.ClientOptions{ElicitationHandler: func(context.Context, *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
			reviews.Add(1)
			if intervene != nil {
				if err := intervene(); err != nil {
					return nil, err
				}
				intervene = nil
			}
			if approved {
				return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"approved": true}}, nil
			}
			return &mcp.ElicitResult{Action: "decline"}, nil
		}})
		cs, err := client.Connect(ctx, &mcp.CommandTransport{Command: exec.CommandContext(ctx, server, append([]string{"serve"}, args...)...), TerminateDuration: time.Second}, nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { cs.Close() })
		return cs
	}
	cs := connect()
	id := contentCall(t, cs, "maestro_context", map[string]any{}, false)["workspace_id"].(string)
	before := windowsCoreTree(t, workspacePath)
	status := brainCall(t, cs, "status", nil, false)
	if status["mutation_available"] != true {
		t.Fatal("qualified native backend was not available", status)
	}
	brainCall(t, cs, "templates", nil, false)
	approved = false
	brainCall(t, cs, "prepare", nil, true)
	if !reflect.DeepEqual(before, windowsCoreTree(t, workspacePath)) {
		t.Fatal("query or declined preparation changed workspace")
	}
	approved = true
	brainCall(t, cs, "prepare", nil, false)
	const page = "projects/native-brief.md"
	body := authoredBrief(id, page, "Native reviewed brief")
	created := brainCall(t, cs, "create", map[string]any{"page": page, "text": body}, false)
	if created["state"] != "written" {
		t.Fatal("ordinary attended native page was not written", created)
	}
	appendRequest := map[string]any{"page": page, "section": "## Decisions", "text": "- Native reviewed decision.", "expected_page_revision": created["revision"]}
	changed := brainCall(t, cs, "append", appendRequest, false)
	if err := cs.Close(); err != nil {
		t.Fatal(err)
	}
	cs = connect()
	read := brainCall(t, cs, "read", map[string]any{"page": page, "purpose": "Continue actual native authored page after restart"}, false)
	if read["revision"] != changed["revision"] || strings.Count(read["text"].(string), "Native reviewed decision.") != 1 {
		t.Fatal("native restart lost authored decision", read)
	}
	exactReplay := func(operation string, args map[string]any, written map[string]any) {
		replayed := brainCall(t, cs, operation, args, false)
		if replayed["state"] != "unchanged" || replayed["revision"] != written["revision"] {
			t.Fatal("exact native operation did not converge through canonical journal", operation, replayed)
		}
	}
	exactReplay("append", appendRequest, changed)
	updatedBody := strings.Replace(read["text"].(string), "A reviewed synthetic kickoff brief.", "The native project scope has been reviewed.", 1)
	updateRequest := map[string]any{"page": page, "text": updatedBody, "expected_page_revision": changed["revision"]}
	updated := brainCall(t, cs, "update", updateRequest, false)
	exactReplay("update", updateRequest, updated)
	fieldRequest := map[string]any{"page": page, "field": "Status", "value": "ready for native review", "expected_page_revision": updated["revision"]}
	field := brainCall(t, cs, "set_field", fieldRequest, false)
	exactReplay("set_field", fieldRequest, field)
	const target = "people/native-sponsor.md"
	brainCall(t, cs, "create", map[string]any{"page": target, "text": authoredBrief(id, target, "Native sponsor")}, false)
	linkRequest := map[string]any{"page": page, "section": "## Related", "target": target, "label": "Native sponsor", "expected_page_revision": field["revision"]}
	linked := brainCall(t, cs, "link", linkRequest, false)
	exactReplay("link", linkRequest, linked)
	read = brainCall(t, cs, "read", map[string]any{"page": page, "purpose": "Verify the complete native reviewed brief"}, false)
	finalText := read["text"].(string)
	if read["revision"] != linked["revision"] || !strings.Contains(finalText, "The native project scope has been reviewed.") || !strings.Contains(finalText, "- **Status:** ready for native review") || !strings.Contains(finalText, "[Native sponsor](../people/native-sponsor.md)") || strings.Count(finalText, "Native reviewed decision.") != 1 {
		t.Fatal("native edit workflow lost prose, field, reference or exact-once decision", read)
	}
	for _, operation := range []string{"create", "update"} {
		candidatePage := page
		if operation == "create" {
			candidatePage = "projects/native-ambiguous.md"
		}
		for _, prefix := range []string{"id: [ignored]\n", "id: {ignored: value}\n", "context: {source: first}\ncontext: {source: second}\n"} {
			beforeWorkspace, beforeOwner := windowsCoreTree(t, workspacePath), windowsCoreTree(t, home)
			beforeReviews := reviews.Load()
			candidate := strings.Replace(authoredBrief(id, candidatePage, "Native reviewed brief"), "---\n", "---\n"+prefix, 1)
			args := map[string]any{"page": candidatePage, "text": candidate}
			if operation == "update" {
				args["expected_page_revision"] = linked["revision"]
			}
			brainCall(t, cs, operation, args, true)
			if reviews.Load() != beforeReviews || !reflect.DeepEqual(beforeWorkspace, windowsCoreTree(t, workspacePath)) || !reflect.DeepEqual(beforeOwner, windowsCoreTree(t, home)) {
				t.Fatal("ambiguous native metadata reached review or changed page/journal/grant/owner state")
			}
		}
	}
	const unicodePage = "projects/native-unicode.md"
	unicodeBody := authoredBrief(id, unicodePage, "Native Unicode brief") + strings.Repeat("colaboração 漢字 🎷\n", 900)
	brainCall(t, cs, "create", map[string]any{"page": unicodePage, "text": unicodeBody}, false)
	for offset := 0; offset < len(unicodeBody); {
		fragment := brainCall(t, cs, "read", map[string]any{"page": unicodePage, "purpose": "Read actual native bounded UTF8 continuation", "offset": offset, "max_bytes": 4096}, false)
		text := fragment["text"].(string)
		if len(text) == 0 || len(text) > 4096 || !utf8.ValidString(text) || !strings.HasPrefix(unicodeBody[offset:], text) {
			t.Fatal("native UTF8 page split or exceeded its bound", offset, len(text))
		}
		offset += len(text)
	}
	later := strings.Replace(finalText, "## Related", "- Native newer work.\n\n## Related", 1)
	intervene = func() error {
		return scheduler.WritePrivateFileAtomic(filepath.Join(workspacePath, "brain", filepath.FromSlash(page)), []byte(later))
	}
	brainCall(t, cs, "update", map[string]any{"page": page, "text": body, "expected_page_revision": read["revision"]}, true)
	after := brainCall(t, cs, "read", map[string]any{"page": page, "purpose": "Preserve intervening native work after stale review"}, false)
	if after["text"] != later {
		t.Fatal("native stale review overwrote newer prose", after)
	}
	// A hardlink leaf is not an authored page, even when its bytes are valid.
	outside := filepath.Join(root, "outside.md")
	if err := scheduler.WritePrivateFileAtomic(outside, []byte(body)); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(workspacePath, "brain", "alias.md")
	if err := os.Link(outside, alias); err != nil {
		t.Fatal("native hardlink fixture unavailable", err)
	}
	brainCall(t, cs, "read", map[string]any{"page": "alias.md", "purpose": "Refuse a hardlink alias"}, true)
	brainCall(t, cs, "create", map[string]any{"page": "alias.md", "text": authoredBrief(id, "alias.md", "Alias")}, true)
	actual, err := os.ReadFile(outside)
	if err != nil || string(actual) != body {
		t.Fatal("native alias touched outside bytes", err)
	}
	const crlfPage = "projects/native-crlf.md"
	crlfBody := strings.ReplaceAll(authoredBrief(id, crlfPage, "Native CRLF brief"), "\n", "\r\n")
	crlfCreated := brainCall(t, cs, "create", map[string]any{"page": crlfPage, "text": crlfBody}, false)
	crlfAppend := brainCall(t, cs, "append", map[string]any{"page": crlfPage, "section": "## Decisions", "text": "- Preserve native CRLF.", "expected_page_revision": crlfCreated["revision"]}, false)
	crlfField := brainCall(t, cs, "set_field", map[string]any{"page": crlfPage, "field": "Status", "value": "native CRLF reviewed", "expected_page_revision": crlfAppend["revision"]}, false)
	crlfLink := brainCall(t, cs, "link", map[string]any{"page": crlfPage, "section": "## Related", "target": target, "label": "Native sponsor", "expected_page_revision": crlfField["revision"]}, false)
	crlfRead := brainCall(t, cs, "read", map[string]any{"page": crlfPage, "purpose": "Verify actual native Windows CRLF preservation"}, false)
	crlfExpected := strings.Replace(crlfBody, "## Decisions\r\n", "## Decisions\r\n- Preserve native CRLF.\r\n", 1)
	crlfExpected = strings.Replace(crlfExpected, "- **Status:** draft\r\n", "- **Status:** native CRLF reviewed\r\n", 1)
	crlfExpected = strings.Replace(crlfExpected, "## Related\r\n", "## Related\r\n- [Native sponsor](../people/native-sponsor.md)\r\n", 1)
	if crlfRead["text"] != crlfExpected || crlfRead["revision"] != crlfLink["revision"] {
		t.Fatal("actual native CRLF edits changed surrounding bytes", crlfRead)
	}

	// The same canonical namespace owner must protect actual case-insensitive
	// Windows reads and both endpoints of an authored reference.
	marker := filepath.Join(workspacePath, "brain", "accounts", ".active")
	if err = scheduler.EnsurePrivateDirectory(filepath.Dir(marker)); err != nil {
		t.Fatal(err)
	}
	if err = scheduler.WritePrivateFileAtomic(marker, []byte("a/selected\n")); err != nil {
		t.Fatal(err)
	}
	const selected = "accounts/a/cases/selected/notes.md"
	selectedBody := strings.Replace(authoredBrief(id, selected, "Native selected case"), "scope: workspace/"+id, "scope: account/a/case/selected", 1)
	selectedBody = strings.Replace(selectedBody, "sensitivity: internal", "sensitivity: client-confidential", 1)
	selectedResult := brainCall(t, cs, "create", map[string]any{"page": selected, "text": selectedBody}, false)
	selectedRead := brainCall(t, cs, "read", map[string]any{"page": selected, "purpose": "Continue the actual selected native case"}, false)
	if selectedRead["text"] != selectedBody {
		t.Fatal("native current-case source unavailable", selectedRead)
	}
	foreign := filepath.Join(workspacePath, "brain", "accounts", "a", "cases", "foreign", "notes.md")
	if err = scheduler.EnsurePrivateDirectory(filepath.Dir(foreign)); err != nil {
		t.Fatal(err)
	}
	foreignBody := "# Preserve native foreign case bytes\n"
	if err = scheduler.WritePrivateFileAtomic(foreign, []byte(foreignBody)); err != nil {
		t.Fatal(err)
	}
	const legacy = "accounts/a/cases/selected/legacy-converted.md"
	legacyText := "# Native legacy converted knowledge\n\nExisting prose remains usable without authored frontmatter.\n"
	if err = scheduler.WritePrivateFileAtomic(filepath.Join(workspacePath, "brain", filepath.FromSlash(legacy)), []byte(legacyText)); err != nil {
		t.Fatal(err)
	}
	collected := scopedKnowledgeCollect(t, cs, legacy, false)
	if pages := collected["pages"].([]any); len(pages) != 1 || pages[0].(map[string]any)["content"] != legacyText {
		t.Fatal("native selected legacy knowledge lost compatibility", collected)
	}
	beforeForeignReviews := reviews.Load()
	for _, alias := range []string{"accounts/a/cases/foreign/notes.md", "Accounts/a/cases/foreign/notes.md", "accounts/a/Cases/foreign/notes.md", "ACCOUNTS/A/CASES/foreign/notes.md"} {
		brainCall(t, cs, "read", map[string]any{"page": alias, "purpose": "Refuse native foreign-case aliases"}, true)
		scopedKnowledgeCollect(t, cs, alias, true)
		brainCall(t, cs, "link", map[string]any{"page": selected, "section": "## Related", "target": alias, "label": "Foreign case", "expected_page_revision": selectedResult["revision"]}, true)
	}
	if reviews.Load() != beforeForeignReviews {
		t.Fatal("native foreign case reached trusted host review")
	}
	if actual, err := scheduler.ReadPrivateFile(foreign, 4096); err != nil || string(actual) != foreignBody {
		t.Fatal("native foreign case bytes changed", err)
	}
	if !reflect.DeepEqual(managed, windowsCoreTree(t, product)) {
		t.Fatal("authored native journey changed managed product")
	}
	t.Log("native_authored_brain shipping_server=true attended_create=true restarted_read=true update=true set_field=true link=true exact_replay_all_edits=true stale_preserves_newer=true declined_prepare_readonly=true hardlink_refused=true selected_case_available=true foreign_case_aliases_refused=true legacy_knowledge_allowed=true knowledge_scope_refused=true crlf_preserved=true duplicate_metadata_refused_before_review=true utf8_continuations_bounded=true")

	t.Run("eager_registered_brain_identity", func(t *testing.T) {
		local := windowsPrivateRoot(t)
		registered, err := workspace.Initialize(workspace.Options{WorkspacePath: filepath.Join(local, "workspace"), DataRoot: filepath.Join(local, "state")})
		if err != nil {
			t.Fatal(err)
		}
		engine, err := atlasops.OpenWorkspace(atlas.Options{DataRoot: registered.DataRoot, WorkspacePath: registered.WorkspacePath, WorkspaceID: registered.WorkspaceID})
		if err != nil {
			t.Fatal(err)
		}
		old := engine.Root() + "-old"
		if err = os.Rename(engine.Root(), old); err != nil {
			t.Fatal("native replace-directory fixture unavailable", err)
		}
		if err = scheduler.EnsurePrivateDirectory(engine.Root()); err != nil {
			t.Fatal(err)
		}
		original, err := scheduler.ReadPrivateFile(filepath.Join(old, "README.md"), 256<<10)
		if err != nil {
			t.Fatal(err)
		}
		if err = scheduler.WritePrivateFileAtomic(filepath.Join(engine.Root(), "README.md"), original); err != nil {
			t.Fatal(err)
		}
		if _, err = engine.ReviewPage(atlasops.CollectRequest{Purpose: "Refuse a replaced admitted brain", Reader: atlasops.ReaderOwnerSession, Pages: []string{"README.md"}}); err == nil {
			t.Fatal("lazy path identity accepted a replacement brain")
		}
		if _, err = engine.CreatePage(atlasops.CreatePageRequest{Page: "must-not-land.md", Body: "# Refused\n", Provenance: atlasops.Provenance{Origin: atlasops.OriginAttended, SessionID: "native-adversarial", IdempotencyKey: "refuse-replaced-brain"}}); err == nil {
			t.Fatal("replaced binding acquired new write authority")
		}
	})
}
