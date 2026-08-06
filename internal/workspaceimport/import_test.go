package workspaceimport

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFixture(t *testing.T, root, name, body string) string {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestInspectClassifiesSyntheticSourcesWithoutReadingBodies(t *testing.T) {
	tests := []struct {
		name      string
		marker    string
		wantClass string
		wantState string
	}{
		{name: "native", marker: ".bcgos/workspace.json", wantClass: ClassificationMaestroNative, wantState: "blocked"},
		{name: "legacy", marker: "maestro.json", wantClass: ClassificationMaestroLegacy, wantState: "ready"},
		{name: "kowalski", marker: "kowalski.json", wantClass: ClassificationKowalski, wantState: "ready"},
		{name: "foreign", marker: "notes.md", wantClass: ClassificationForeign, wantState: "ready"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			writeFixture(t, root, test.marker, "synthetic fixture body")
			inspection, err := Inspect(root, Limits{MaxEntries: 20, MaxDepth: 5, MaxFileBytes: 1024, MaxTotalBytes: 4096})
			if err != nil {
				t.Fatal(err)
			}
			if inspection.Classification != test.wantClass || inspection.State != test.wantState || !inspection.ReadOnly || !inspection.Bounded {
				t.Fatalf("inspection = %#v", inspection)
			}
			foundFile := false
			for _, entry := range inspection.Entries {
				if entry.Kind == "file" && entry.Size > 0 {
					foundFile = true
				}
			}
			if test.wantClass != ClassificationMaestroNative && !foundFile {
				t.Fatalf("metadata inventory did not retain file size: %#v", inspection.Entries)
			}
		})
	}
}

func TestInspectNeverFollowsSymlink(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	writeFixture(t, outside, "secret.txt", "synthetic secret must not be read")
	if err := os.Symlink(outside, filepath.Join(root, "linked")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	inspection, err := Inspect(root, DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	if inspection.SymlinkCount != 1 || len(inspection.Entries) != 1 || inspection.Entries[0].Kind != "symlink" || !inspection.Entries[0].Unsafe {
		t.Fatalf("symlink inventory = %#v", inspection)
	}
	for _, entry := range inspection.Entries {
		if strings.Contains(entry.RelativePath, "secret") {
			t.Fatalf("symlink target was traversed: %#v", inspection.Entries)
		}
	}
}

func TestInspectIsBounded(t *testing.T) {
	root := t.TempDir()
	for i := 0; i < 5; i++ {
		writeFixture(t, root, filepath.Join("nested", string(rune('a'+i))+".md"), "fixture")
	}
	inspection, err := Inspect(root, Limits{MaxEntries: 2, MaxDepth: 2, MaxFileBytes: 1024, MaxTotalBytes: 4096})
	if err != nil {
		t.Fatal(err)
	}
	if inspection.State != "bounded" || inspection.EntryCount > 2 || len(inspection.Warnings) == 0 {
		t.Fatalf("bounded inspection = %#v", inspection)
	}
}

func TestBuildPlanQuarantinesDocumentsAndReportsConflicts(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	destination := filepath.Join(root, "destination")
	if err := os.MkdirAll(destination, 0o700); err != nil {
		t.Fatal(err)
	}
	writeFixture(t, source, "brief.md", "synthetic markdown")
	writeFixture(t, source, "brief.docx", "synthetic document bytes")
	writeFixture(t, source, "binary.bin", "synthetic binary")
	writeFixture(t, destination, "brief.md", "existing destination")
	plan, err := BuildPlan(source, destination, DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Conflicts) != 1 || plan.Conflicts[0].Path != "brief.md" {
		t.Fatalf("conflicts = %#v", plan.Conflicts)
	}
	var document, binary PlanEntry
	for _, entry := range plan.Entries {
		if entry.SourcePath == "brief.docx" {
			document = entry
		}
		if entry.SourcePath == "binary.bin" {
			binary = entry
		}
	}
	if document.Action != ActionQuarantine || document.Availability != "unavailable" || !strings.Contains(document.Reason, "runtime") {
		t.Fatalf("document entry = %#v", document)
	}
	if binary.Action != ActionQuarantine || binary.Availability != "unsupported" {
		t.Fatalf("binary entry = %#v", binary)
	}
	if err := ValidatePlan(plan); err == nil || !strings.Contains(err.Error(), "conflicts") {
		t.Fatalf("conflicted plan accepted: %v", err)
	}
}

func TestNativeWorkspacePlanFailsClosed(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	destination := filepath.Join(root, "destination")
	if err := os.MkdirAll(destination, 0o700); err != nil {
		t.Fatal(err)
	}
	writeFixture(t, source, ".bcgos/workspace.json", `{"schema_version":1}`)
	writeFixture(t, source, "brain/note.md", "synthetic native content")
	plan, err := BuildPlan(source, destination, DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	if plan.Classification != ClassificationMaestroNative {
		t.Fatalf("classification = %q", plan.Classification)
	}
	if err := ValidatePlan(plan); err == nil || !strings.Contains(err.Error(), "native") {
		t.Fatalf("native plan accepted: %v", err)
	}
}

func TestExecuteIsStagedIdempotentAndRollbackLeavesSource(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	destination := filepath.Join(root, "destination")
	dataRoot := filepath.Join(root, "data")
	if err := os.MkdirAll(destination, 0o700); err != nil {
		t.Fatal(err)
	}
	sourceFile := writeFixture(t, source, "brain/note.md", "synthetic migration fixture")
	original, err := os.ReadFile(sourceFile)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := BuildPlan(source, destination, DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Entries) != 1 {
		t.Fatalf("plan entries = %#v", plan.Entries)
	}
	if _, err := Approve(plan, "synthetic-owner", "wrong"); err == nil {
		t.Fatal("approval without exact confirmation succeeded")
	}
	approval, err := Approve(plan, "synthetic-owner", ConfirmImport)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := Execute(dataRoot, plan, approval)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.State != "executed" || len(receipt.Copied) != 1 {
		t.Fatalf("receipt = %#v", receipt)
	}
	imported, err := os.ReadFile(filepath.Join(destination, "brain", "note.md"))
	if err != nil || string(imported) != string(original) {
		t.Fatalf("imported = %q, err=%v", imported, err)
	}
	repeated, err := Execute(dataRoot, plan, approval)
	if err != nil || repeated.RunID != receipt.RunID || repeated.RecordedAt != receipt.RecordedAt {
		t.Fatalf("idempotent receipt = %#v, err=%v", repeated, err)
	}
	if after, err := os.ReadFile(sourceFile); err != nil || string(after) != string(original) {
		t.Fatalf("source changed: %q, err=%v", after, err)
	}
	rolled, err := Rollback(dataRoot, plan, receipt, ConfirmRollback)
	if err != nil || rolled.State != PlanStateRolledBack {
		t.Fatalf("rollback = %#v, err=%v", rolled, err)
	}
	if _, err := os.Stat(filepath.Join(destination, "brain", "note.md")); !os.IsNotExist(err) {
		t.Fatalf("rollback left destination file: %v", err)
	}
	if after, err := os.ReadFile(sourceFile); err != nil || string(after) != string(original) {
		t.Fatalf("rollback changed source: %q, err=%v", after, err)
	}
}

func TestPlanDigestAndSourceMetadataAreImmutable(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	destination := filepath.Join(root, "destination")
	dataRoot := filepath.Join(root, "data")
	if err := os.MkdirAll(destination, 0o700); err != nil {
		t.Fatal(err)
	}
	file := writeFixture(t, source, "note.md", "one")
	plan, err := BuildPlan(source, destination, DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	approval, err := Approve(plan, "synthetic-owner", ConfirmImport)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte("two"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Execute(dataRoot, plan, approval); err == nil || !strings.Contains(err.Error(), "metadata") {
		t.Fatalf("changed source was accepted: %v", err)
	}
	plan.PlanDigest = strings.Repeat("0", 64)
	if err := ValidatePlan(plan); err == nil || !strings.Contains(err.Error(), "digest") {
		t.Fatalf("tampered plan was accepted: %v", err)
	}
}
