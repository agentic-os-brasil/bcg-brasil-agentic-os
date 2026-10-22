package spike

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fixtureRoot(t *testing.T) string {
	t.Helper()
	base := os.Getenv("GOTMPDIR")
	developer, err := developerRoot()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(base, developer+string(filepath.Separator)) {
		t.Fatal("tests require GOTMPDIR physically under Developer")
	}
	dir, err := os.MkdirTemp(base, "maestro-mcp-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return filepath.Join(dir, "synthetic")
}

func TestCheckpointRestartResumeAndCaseIsolation(t *testing.T) {
	root := fixtureRoot(t)
	if _, err := Seed(root); err != nil {
		t.Fatal(err)
	}
	a, err := Open(root, "case-a")
	if err != nil {
		t.Fatal(err)
	}
	before, err := a.Context(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if before.State != "running" || before.StateRevision != 2 || before.Workspace != "case-a" {
		t.Fatalf("initial context: %+v", before)
	}
	_, err = a.Checkpoint(context.Background(), CheckpointInput{ExpectedRevision: before.StateRevision, AttemptID: before.AttemptID, Summary: "Synthetic A conclusion", NextStep: "Validate recommendation A", Pause: true})
	if err != nil {
		t.Fatal(err)
	}
	restarted, err := Open(root, "case-a")
	if err != nil {
		t.Fatal(err)
	}
	paused, err := restarted.Context(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if paused.State != "paused" || paused.Summary != "Synthetic A conclusion" || paused.NextStep != "Validate recommendation A" {
		t.Fatalf("lost handoff: %+v", paused)
	}
	resumed, err := restarted.Resume(context.Background(), ResumeInput{ExpectedRevision: paused.StateRevision})
	if err != nil {
		t.Fatal(err)
	}
	if resumed.AttemptID == before.AttemptID || resumed.State != "running" {
		t.Fatalf("resume: %+v", resumed)
	}
	if _, err := restarted.Checkpoint(context.Background(), CheckpointInput{ExpectedRevision: resumed.StateRevision, AttemptID: before.AttemptID, Summary: "stale", NextStep: "must fail"}); err == nil {
		t.Fatal("old attempt wrote after resume")
	}
	b, err := Open(root, "case-b")
	if err != nil {
		t.Fatal(err)
	}
	bctx, err := b.Context(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if bctx.Summary != "" || bctx.ItemID == before.ItemID || bctx.NextStep == paused.NextStep {
		t.Fatalf("case boundary failed: %+v", bctx)
	}
	if _, err := b.Checkpoint(context.Background(), CheckpointInput{ExpectedRevision: bctx.StateRevision, AttemptID: resumed.AttemptID, Summary: "A in B", NextStep: "must fail"}); err == nil {
		t.Fatal("cross-case attempt accepted")
	}
}

func TestSeedRefusesReuseAndAliases(t *testing.T) {
	root := fixtureRoot(t)
	if _, err := Seed(root); err != nil {
		t.Fatal(err)
	}
	if _, err := Seed(root); err == nil {
		t.Fatal("seed reused existing root")
	}
	alias := filepath.Join(filepath.Dir(root), "alias")
	if err := os.Symlink(root, alias); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := Open(alias, "case-a"); err == nil {
		t.Fatal("aliased root accepted")
	}
	if _, err := Open(root, "../case-a"); err == nil {
		t.Fatal("unknown workspace accepted")
	}
}
