package priorwork

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

func TestGuidedSourceSelectionIsWorkspaceBoundVersionedAndPointerOnly(t *testing.T) {
	root := filepath.Join(t.TempDir(), "sharepoint-work")
	store := SourceSelectionStore{Root: root, clock: func() time.Time {
		return time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	}}
	workspaceID := strings.Repeat("a", 32)

	status, err := store.Status(workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	if status.State != SourceSelectionRequired || status.SourceAuthority != "sharepoint" || status.CollectionRuntime != "claude" || status.CodexCollectionState != "unavailable/corporate_policy" {
		t.Fatalf("initial status = %#v", status)
	}

	selection, err := store.Select(workspaceID, []string{
		"https://bcg.sharepoint.com/sites/project/Shared%20Documents/Workstream-B/",
		"https://bcg.sharepoint.com/sites/project/Shared%20Documents/Workstream-A",
	})
	if err != nil {
		t.Fatal(err)
	}
	if selection.State != SourceSelected || selection.FolderCount != 2 || selection.AuthorizationState != "pending_signed_enrollment" || selection.LocalProjection != "metadata_and_source_pointers_only" || selection.Pointer == "" {
		t.Fatalf("selection = %#v", selection)
	}
	if strings.Contains(selection.Pointer, "Workstream") {
		t.Fatalf("status pointer exposed source content: %q", selection.Pointer)
	}

	selectionPath := filepath.Join(root, filepath.FromSlash(selection.Pointer))
	info, err := os.Stat(selectionPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("selection permissions = %o", info.Mode().Perm())
	}
	body, err := os.ReadFile(selectionPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "Workstream-A") || !strings.Contains(string(body), "Workstream-B") || strings.Contains(string(body), "document_body") {
		t.Fatalf("selection body = %s", body)
	}

	repeated, err := store.Select(workspaceID, []string{
		"https://bcg.sharepoint.com/sites/project/Shared%20Documents/Workstream-A",
		"https://bcg.sharepoint.com/sites/project/Shared%20Documents/Workstream-B",
	})
	if err != nil {
		t.Fatal(err)
	}
	if repeated.State != SourceSelected || repeated.Version != selection.Version || repeated.Fingerprint != selection.Fingerprint {
		t.Fatalf("repeated selection = %#v", repeated)
	}

	deferred, err := store.Defer(workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	if deferred.State != SourceDeferred || deferred.FolderCount != 0 || deferred.Version == selection.Version {
		t.Fatalf("deferred status = %#v", deferred)
	}
}

func TestGuidedSourceSelectionRejectsBroadOrNonCanonicalPointers(t *testing.T) {
	store := SourceSelectionStore{Root: filepath.Join(t.TempDir(), "sharepoint-work")}
	workspaceID := strings.Repeat("b", 32)
	tests := []struct {
		name string
		urls []string
	}{
		{name: "empty", urls: nil},
		{name: "tenant root", urls: []string{"https://bcg.sharepoint.com/"}},
		{name: "site root", urls: []string{"https://bcg.sharepoint.com/sites/project"}},
		{name: "query sharing link", urls: []string{"https://bcg.sharepoint.com/sites/project/Shared%20Documents/Folder?share=secret"}},
		{name: "fragment", urls: []string{"https://bcg.sharepoint.com/sites/project/Shared%20Documents/Folder#section"}},
		{name: "non sharepoint", urls: []string{"https://example.com/sites/project/Shared%20Documents/Folder"}},
		{name: "bare sharepoint host", urls: []string{"https://sharepoint.com/sites/project/Shared%20Documents/Folder"}},
		{name: "credentials", urls: []string{"https://user:secret@bcg.sharepoint.com/sites/project/Shared%20Documents/Folder"}},
		{name: "duplicate", urls: []string{
			"https://bcg.sharepoint.com/sites/project/Shared%20Documents/Folder",
			"https://bcg.sharepoint.com/sites/project/Shared%20Documents/Folder",
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := store.Select(workspaceID, test.urls); err == nil {
				t.Fatal("unsafe SharePoint source selection was accepted")
			}
		})
	}
	if _, err := store.Status("../outside"); err == nil {
		t.Fatal("unsafe workspace ID was accepted")
	}
}

func TestGuidedSourceSelectionMapsLibraryAndSharePointViewRootsWithoutReading(t *testing.T) {
	store := SourceSelectionStore{Root: filepath.Join(t.TempDir(), "sharepoint-work")}
	workspaceID := strings.Repeat("c", 32)

	status, err := store.Select(workspaceID, []string{"https://bcgcloud.sharepoint.com/sites/xek407-rt/Shared Documents"})
	if err != nil || status.State != SourceSelected {
		t.Fatalf("library root selection err=%v status=%#v", err, status)
	}
	if _, err := store.SelectedFolders(workspaceID); !errors.Is(err, ErrLibraryRootNeedsProjectScope) {
		t.Fatalf("library root collection guard err=%v", err)
	}

	viewRoot, err := ParseSourceSelectionInput(strings.NewReader(`{"schema_version":1,"folder_urls":["https://bcgcloud.sharepoint.com/sites/xek407-rt/Shared%20Documents/Forms/AllItems.aspx"]}`))
	if err != nil || len(viewRoot.FolderURLs) != 1 || viewRoot.FolderURLs[0] != "https://bcgcloud.sharepoint.com/sites/xek407-rt/Shared%20Documents" {
		t.Fatalf("view root normalization = %#v err=%v", viewRoot, err)
	}
	viewFolder, err := ParseSourceSelectionInput(strings.NewReader(`{"schema_version":1,"folder_urls":["https://bcgcloud.sharepoint.com/sites/xek407-rt/Shared%20Documents/Forms/AllItems.aspx?id=%2Fsites%2Fxek407-rt%2FShared%20Documents%2FHDI%20AI%20for%20Sales"]}`))
	if err != nil || len(viewFolder.FolderURLs) != 1 || viewFolder.FolderURLs[0] != "https://bcgcloud.sharepoint.com/sites/xek407-rt/Shared%20Documents/HDI%20AI%20for%20Sales" {
		t.Fatalf("view folder normalization = %#v err=%v", viewFolder, err)
	}
	for _, body := range []string{
		`{"schema_version":1,"folder_urls":["https://bcgcloud.sharepoint.com/sites/xek407-rt/Shared%20Documents/.."]}`,
		`{"schema_version":1,"folder_urls":["https://bcgcloud.sharepoint.com/sites/xek407-rt/Shared%20Documents/Forms/AllItems.aspx?id=%2Fsites%2Fxek407-rt%2FShared%20Documents%2F.."]}`,
		`{"schema_version":1,"folder_urls":["https://bcgcloud.sharepoint.com/sites/xek407-rt/Shared%20Documents/Forms/AllItems.aspx?id=%2Fsites%2Fother-project%2FShared%20Documents%2FSecret"]}`,
		`{"schema_version":1,"folder_urls":["https://bcgcloud.sharepoint.com/sites/xek407-rt/Shared%20Documents/Forms/AllItems.aspx?id=%2Fsites%2Fxek407-rt%2FOther%20Library%2FSecret"]}`,
		`{"schema_version":1,"folder_urls":["https://bcgcloud.sharepoint.com/sites/xek407-rt/Shared%20Documents/Forms/AllItems.aspx?id=%2Fsites%2Fxek407-rt%2FShared%20Documents%2FFolder&id=%2Fsites%2Fxek407-rt%2FShared%20Documents%2FOther"]}`,
	} {
		if _, err := ParseSourceSelectionInput(strings.NewReader(body)); err == nil {
			t.Fatalf("unsafe SharePoint path was accepted: %s", body)
		}
	}
}

func TestGuidedSourceSelectionSchemaCompilesAndMatchesContract(t *testing.T) {
	path := filepath.Join("..", "..", "schemas", "sharepoint-project-source-selection.schema.json")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var document any
	if err := json.Unmarshal(body, &document); err != nil {
		t.Fatal(err)
	}
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource(filepath.Base(path), document); err != nil {
		t.Fatal(err)
	}
	schema, err := compiler.Compile(filepath.Base(path))
	if err != nil {
		t.Fatal(err)
	}
	valid := map[string]any{
		"schema_version": 1,
		"folder_urls": []any{
			"https://bcg.sharepoint.com/sites/project/Shared%20Documents/Folder",
		},
	}
	if err := schema.Validate(valid); err != nil {
		t.Fatalf("valid guided source selection rejected: %v", err)
	}
	valid["folder_urls"] = []any{"https://bcg.sharepoint.com/sites/project/Shared%20Documents"}
	if err := schema.Validate(valid); err != nil {
		t.Fatalf("valid library-root source selection rejected: %v", err)
	}
	invalid := map[string]any{
		"schema_version": 1,
		"folder_urls": []any{
			"https://bcg.sharepoint.com/sites/project/Shared%20Documents/Folder",
			"https://bcg.sharepoint.com/sites/project/Shared%20Documents/Folder",
		},
	}
	if err := schema.Validate(invalid); err == nil {
		t.Fatal("schema accepted duplicate SharePoint folder pointers")
	}
}

func TestParseSourceSelectionInputIsStrictAndBounded(t *testing.T) {
	input, err := ParseSourceSelectionInput(strings.NewReader(`{"schema_version":1,"folder_urls":["https://bcg.sharepoint.com/sites/project/Shared%20Documents/Folder"]}`))
	if err != nil || len(input.FolderURLs) != 1 {
		t.Fatalf("input = %#v, err = %v", input, err)
	}
	for _, body := range []string{
		`{"schema_version":1,"folder_urls":[],"unknown":true}`,
		`{"schema_version":1,"schema_version":1,"folder_urls":[]}`,
	} {
		if _, err := ParseSourceSelectionInput(strings.NewReader(body)); err == nil {
			t.Fatalf("invalid source selection input was accepted: %.40q", body)
		}
	}
	if _, err := ParseSourceSelectionInput(strings.NewReader(strings.Repeat("x", MaximumSourceSelectionInputBytes+1))); !errors.Is(err, ErrSourceSelectionTooLarge) {
		t.Fatalf("oversized source selection error = %v", err)
	}
}
