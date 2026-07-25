package execution

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func evidenceStore(t *testing.T) Store {
	t.Helper()
	ids := []string{"item-evidence", "attempt-evidence", "evidence-artifact", "evidence-command"}
	return Store{
		Root: t.TempDir(),
		Now:  func() time.Time { return time.Date(2026, 7, 25, 18, 0, 0, 0, time.UTC) },
		NewID: func(kind string) (string, error) {
			if len(ids) == 0 {
				t.Fatal("unexpected ID request")
			}
			id := ids[0]
			ids = ids[1:]
			return id, nil
		},
	}
}

func evidenceContract() CreateInput {
	return CreateInput{
		WorkspaceID:     testWorkspaceID,
		Objective:       "Prove evidence-backed completion.",
		InitialNextStep: "Collect the artifact and command evidence.",
		Criteria: []Criterion{
			{
				ID:        "artifact",
				Type:      CriterionArtifactSnapshot,
				TargetRef: "bcgos://workspace/result.txt",
			},
			{
				ID:      "command",
				Type:    CriterionCommandCheck,
				Command: []string{"go", "version"},
			},
		},
		AllowedRefs: []string{"bcgos://workspace/result.txt"},
	}
}

func TestCollectEvidenceAndCompleteRevalidatesDoneContract(t *testing.T) {
	workspaceRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspaceRoot, "result.txt"), []byte("final result\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	store := evidenceStore(t)
	created, err := store.Create(evidenceContract())
	if err != nil {
		t.Fatal(err)
	}
	started, err := store.Start(testWorkspaceID, created.Contract.ItemID, 1)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := store.CollectEvidence(testWorkspaceID, created.Contract.ItemID, EvidenceInput{
		WorkspaceRoot:    workspaceRoot,
		ExpectedRevision: 2,
		AttemptID:        started.State.ActiveAttemptID,
		CriterionID:      "artifact",
	})
	if err != nil {
		t.Fatal(err)
	}
	if artifact.Receipt.Outcome != EvidencePassed || artifact.Receipt.ArtifactSHA256 == "" ||
		artifact.Receipt.TargetRef != "bcgos://workspace/result.txt" {
		t.Fatalf("artifact receipt = %#v", artifact.Receipt)
	}
	command, err := store.CollectEvidence(testWorkspaceID, created.Contract.ItemID, EvidenceInput{
		WorkspaceRoot:    workspaceRoot,
		ExpectedRevision: 3,
		AttemptID:        started.State.ActiveAttemptID,
		CriterionID:      "command",
	})
	if err != nil {
		t.Fatal(err)
	}
	if command.Receipt.Outcome != EvidencePassed || command.Receipt.CommandSHA256 == "" || command.Receipt.ToolSHA256 == "" {
		t.Fatalf("command receipt = %#v", command.Receipt)
	}
	completed, err := store.Complete(testWorkspaceID, created.Contract.ItemID, CompletionInput{
		WorkspaceRoot:    workspaceRoot,
		ExpectedRevision: 4,
		AttemptID:        started.State.ActiveAttemptID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if completed.State.State != StateCompleted || completed.State.StateRevision != 5 ||
		completed.Attempt == nil || completed.Attempt.State != AttemptCompleted {
		t.Fatalf("completed item = %#v", completed)
	}
}

func TestCommandEvidenceIgnoresMutablePathAndGoFlags(t *testing.T) {
	fakeRoot := t.TempDir()
	fakeName := "go"
	if runtime.GOOS == "windows" {
		fakeName = "go.exe"
	}
	if err := os.WriteFile(filepath.Join(fakeRoot, fakeName), []byte("not a trusted tool"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", fakeRoot)
	t.Setenv("GOFLAGS", "-definitely-invalid")

	workspaceRoot := t.TempDir()
	store := evidenceStore(t)
	input := evidenceContract()
	input.Criteria = input.Criteria[1:]
	created, err := store.Create(input)
	if err != nil {
		t.Fatal(err)
	}
	started, err := store.Start(testWorkspaceID, created.Contract.ItemID, 1)
	if err != nil {
		t.Fatal(err)
	}
	result, err := store.CollectEvidence(testWorkspaceID, created.Contract.ItemID, EvidenceInput{
		WorkspaceRoot: workspaceRoot, ExpectedRevision: 2,
		AttemptID: started.State.ActiveAttemptID, CriterionID: "command",
	})
	if err != nil || result.Receipt.Outcome != EvidencePassed || len(result.Receipt.ToolSHA256) != 64 {
		t.Fatalf("controlled command receipt = %#v, err = %v", result.Receipt, err)
	}
}

func TestCommandEvidenceRejectsCallerSuppliedGoRoot(t *testing.T) {
	fakeRoot := t.TempDir()
	fakeBin := filepath.Join(fakeRoot, "bin")
	if err := os.MkdirAll(fakeBin, 0o700); err != nil {
		t.Fatal(err)
	}
	fakeName := "go"
	if runtime.GOOS == "windows" {
		fakeName = "go.exe"
	}
	if err := os.WriteFile(filepath.Join(fakeBin, fakeName), []byte("fake"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOROOT", fakeRoot)

	workspaceRoot := t.TempDir()
	store := evidenceStore(t)
	input := evidenceContract()
	input.Criteria = input.Criteria[1:]
	created, err := store.Create(input)
	if err != nil {
		t.Fatal(err)
	}
	started, err := store.Start(testWorkspaceID, created.Contract.ItemID, 1)
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.CollectEvidence(testWorkspaceID, created.Contract.ItemID, EvidenceInput{
		WorkspaceRoot: workspaceRoot, ExpectedRevision: 2,
		AttemptID: started.State.ActiveAttemptID, CriterionID: "command",
	})
	if err == nil || !strings.Contains(err.Error(), "caller-supplied GOROOT") {
		t.Fatalf("hostile GOROOT error = %v", err)
	}
}

func TestCriterionAndEvidenceVariantsMatchRuntimeContract(t *testing.T) {
	valid := []string{
		`{"id":"artifact","type":"artifact_snapshot","target_ref":"bcgos://workspace/result.txt"}`,
		`{"id":"command","type":"command_check","command":["go","version"]}`,
		`{"id":"command","type":"command_check","command":["go","test","./..."]}`,
	}
	invalid := []string{
		`{"id":"artifact","type":"artifact_snapshot"}`,
		`{"id":"hybrid","type":"artifact_snapshot","target_ref":"bcgos://workspace/result.txt","command":["go","version"]}`,
		`{"id":"command","type":"command_check","command":["go","env"]}`,
	}
	for _, body := range append(valid, invalid...) {
		var criterion Criterion
		decoder := json.NewDecoder(bytes.NewBufferString(body))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&criterion); err != nil {
			t.Fatalf("decode %s: %v", body, err)
		}
		input := CreateInput{
			WorkspaceID: testWorkspaceID, Objective: "x", InitialNextStep: "y",
			Criteria:    []Criterion{criterion},
			AllowedRefs: []string{"bcgos://workspace/result.txt"},
		}
		err := validateCreateInput(input)
		isValid := false
		for _, candidate := range valid {
			if body == candidate {
				isValid = true
			}
		}
		if isValid && err != nil {
			t.Fatalf("valid criterion %s: %v", body, err)
		}
		if !isValid && err == nil {
			t.Fatalf("invalid criterion accepted: %s", body)
		}
	}

	hybrid := EvidenceReceipt{
		SchemaVersion: 1, EvidenceID: "evidence-a", ItemID: "item-a",
		WorkspaceID: testWorkspaceID, AttemptID: "attempt-a",
		CriterionID: "criterion-a", Type: CriterionArtifactSnapshot,
		Outcome: EvidencePassed, TargetRef: "bcgos://workspace/result.txt",
		ArtifactSHA256: strings.Repeat("a", 64), CommandSHA256: strings.Repeat("b", 64),
		ObservedAt: time.Now().UTC(),
	}
	if err := validateEvidenceReceipt(hybrid); err == nil {
		t.Fatal("hybrid evidence receipt was accepted")
	}
}

func TestCompleteFailsWhenArtifactChangedAfterEvidence(t *testing.T) {
	workspaceRoot := t.TempDir()
	target := filepath.Join(workspaceRoot, "result.txt")
	if err := os.WriteFile(target, []byte("first\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	store := evidenceStore(t)
	input := evidenceContract()
	input.Criteria = input.Criteria[:1]
	created, err := store.Create(input)
	if err != nil {
		t.Fatal(err)
	}
	started, err := store.Start(testWorkspaceID, created.Contract.ItemID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CollectEvidence(testWorkspaceID, created.Contract.ItemID, EvidenceInput{
		WorkspaceRoot: workspaceRoot, ExpectedRevision: 2,
		AttemptID: started.State.ActiveAttemptID, CriterionID: "artifact",
	}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("changed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = store.Complete(testWorkspaceID, created.Contract.ItemID, CompletionInput{
		WorkspaceRoot: workspaceRoot, ExpectedRevision: 3,
		AttemptID: started.State.ActiveAttemptID,
	})
	if !errors.Is(err, ErrCompletionUnsatisfied) {
		t.Fatalf("changed artifact completion error = %v", err)
	}
	inspected, inspectErr := store.Inspect(testWorkspaceID, created.Contract.ItemID)
	if inspectErr != nil || inspected.State.State != StateRunning || inspected.State.StateRevision != 3 {
		t.Fatalf("item mutated after failed completion: %#v, err = %v", inspected, inspectErr)
	}
}

func TestEvidenceRejectsUnsafeCommandsAndWorkspaceEscape(t *testing.T) {
	input := evidenceContract()
	input.Criteria[1].Command = []string{"sh", "-c", "echo secret"}
	if _, err := evidenceStore(t).Create(input); err == nil || !strings.Contains(err.Error(), "command") {
		t.Fatalf("unsafe command error = %v", err)
	}

	input = evidenceContract()
	input.Criteria = input.Criteria[:1]
	input.Criteria[0].TargetRef = "bcgos://workspace/../outside"
	if _, err := evidenceStore(t).Create(input); err == nil {
		t.Fatal("workspace escape target was accepted")
	}
}

func TestEvidenceTransitionAndMutationReceiptRemainMetadataOnly(t *testing.T) {
	workspaceRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspaceRoot, "result.txt"), []byte("private body\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	store := evidenceStore(t)
	input := evidenceContract()
	input.Criteria = input.Criteria[:1]
	created, err := store.Create(input)
	if err != nil {
		t.Fatal(err)
	}
	started, err := store.Start(testWorkspaceID, created.Contract.ItemID, 1)
	if err != nil {
		t.Fatal(err)
	}
	result, err := store.CollectEvidence(testWorkspaceID, created.Contract.ItemID, EvidenceInput{
		WorkspaceRoot: workspaceRoot, ExpectedRevision: 2,
		AttemptID: started.State.ActiveAttemptID, CriterionID: "artifact",
	})
	if err != nil {
		t.Fatal(err)
	}
	receipt := EvidenceMutationReceipt(result)
	body := mustJSON(t, receipt)
	for _, prohibited := range []string{"private body", workspaceRoot, "objective", "command"} {
		if strings.Contains(body, prohibited) {
			t.Fatalf("evidence mutation receipt leaked %q: %s", prohibited, body)
		}
	}
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}
