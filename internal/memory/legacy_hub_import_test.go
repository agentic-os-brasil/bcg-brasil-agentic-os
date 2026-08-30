package memory

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestImportLegacyHubPreservesActiveL1AndImportsEveryLegacyLayerOnlyIntoL1(t *testing.T) {
	engine := testEngine(t)
	workspace := "case-a"
	now := engine.Now()
	old := Artifact{
		SchemaVersion: 1, WorkspaceID: workspace, Layer: "L1", Period: now.Format("2006-01-02"), GeneratedAt: now.Add(-time.Hour),
		SourceFingerprint: strings.Repeat("1", 64), Sources: []SourceRef{{ID: "existing", SHA256: strings.Repeat("2", 64)}},
		SynthesizerID: "daily-v1", Content: "existing continuity",
	}
	if err := engine.activate(workspace, []Artifact{old}); err != nil {
		t.Fatal(err)
	}
	sources := []LegacyHubSource{
		legacySource("a", "recent", "recent memory"),
		legacySource("b", "weekly", "weekly memory"),
		legacySource("c", "medium-term", "medium memory"),
		legacySource("d", "lifetime", "lifetime memory"),
	}
	result, err := engine.ImportLegacyHub(context.Background(), LegacyHubImportRequest{WorkspaceID: workspace, Sources: sources, ImportedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	if result.State != "applied" || len(result.ImportID) != 64 {
		t.Fatalf("unexpected result: %#v", result)
	}
	artifact, _, err := engine.readArtifactByKey(workspace, "L1/"+now.Format("2006-01-02"))
	if err != nil {
		t.Fatal(err)
	}
	for _, sentinel := range []string{"existing continuity", "recent memory", "weekly memory", "medium memory", "lifetime memory"} {
		if !strings.Contains(artifact.Content, sentinel) {
			t.Fatalf("L1 omitted %q: %s", sentinel, artifact.Content)
		}
	}
	if !strings.HasPrefix(artifact.SynthesizerID, LegacyHubBridgeSynthesizerID+"/") {
		t.Fatalf("synthesizer = %q", artifact.SynthesizerID)
	}
	for _, key := range []string{"L2/", "L3/current", "lifetime/current"} {
		if _, _, err := engine.readArtifactByKey(workspace, key); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("bridge unexpectedly activated %s: %v", key, err)
		}
	}
	for _, source := range sources {
		body, err := os.ReadFile(filepath.Join(engine.workspaceRoot(workspace), "imports", "legacy-hub", result.ImportID, source.CandidateID+".md"))
		if err != nil || string(body) != string(source.Content) {
			t.Fatalf("snapshot %s = %q, %v", source.CandidateID, body, err)
		}
	}
}

func TestImportLegacyHubIsHistoricallyIdempotentAfterLaterCommit(t *testing.T) {
	engine := testEngine(t)
	now := engine.Now()
	request := LegacyHubImportRequest{WorkspaceID: "case-a", ImportedAt: now, Sources: []LegacyHubSource{legacySource("a", "recent", "import once")}}
	first, err := engine.ImportLegacyHub(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	later := Artifact{SchemaVersion: 1, WorkspaceID: "case-a", Layer: "L1", Period: now.Format("2006-01-02"), GeneratedAt: now.Add(time.Hour), SourceFingerprint: strings.Repeat("5", 64), Sources: []SourceRef{{ID: "later", SHA256: strings.Repeat("6", 64)}}, SynthesizerID: "daily-v2", Content: "later continuity"}
	if err := engine.activate("case-a", []Artifact{later}); err != nil {
		t.Fatal(err)
	}
	second, err := engine.ImportLegacyHub(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if second.State != "already_applied" || second.ImportID != first.ImportID {
		t.Fatalf("retry = %#v, first = %#v", second, first)
	}
	active, _, err := engine.readArtifactByKey("case-a", "L1/"+now.Format("2006-01-02"))
	if err != nil || active.Content != "later continuity" {
		t.Fatalf("historical retry changed active memory: %#v, %v", active, err)
	}
}

func TestImportLegacyHubPreservesLatestCrossDayL1AndSurvivesSameDayDream(t *testing.T) {
	engine := testEngine(t)
	workspace := "case-a"
	importDay := engine.Now()
	previousDay := importDay.AddDate(0, 0, -1)
	previous := Artifact{SchemaVersion: 1, WorkspaceID: workspace, Layer: "L1", Period: previousDay.Format("2006-01-02"), GeneratedAt: previousDay, SourceFingerprint: strings.Repeat("1", 64), Sources: []SourceRef{{ID: "previous", SHA256: strings.Repeat("2", 64)}}, SynthesizerID: "daily-v1", Content: "previous-day continuity"}
	if err := engine.activate(workspace, []Artifact{previous}); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.ImportLegacyHub(context.Background(), LegacyHubImportRequest{WorkspaceID: workspace, ImportedAt: importDay, Sources: []LegacyHubSource{legacySource("a", "recent", "durable imported sentinel")}}); err != nil {
		t.Fatal(err)
	}
	imported, _, err := engine.readArtifactByKey(workspace, "L1/"+importDay.Format("2006-01-02"))
	if err != nil || !strings.Contains(imported.Content, "previous-day continuity") {
		t.Fatalf("cross-day import lost active L1: %#v, %v", imported, err)
	}

	engine.Synthesizer = &fakeSynthesizer{outputs: map[string]string{"L1": "new same-day continuity"}}
	engine.SynthesizerID = "daily-v2"
	if _, err := engine.Capture(Capture{WorkspaceID: workspace, RecordedAt: importDay, Kind: "thread", Text: "today", Sanitized: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.DreamDaily(context.Background(), workspace, importDay); err != nil {
		t.Fatal(err)
	}
	afterDream, _, err := engine.readArtifactByKey(workspace, "L1/"+importDay.Format("2006-01-02"))
	if err != nil {
		t.Fatal(err)
	}
	for _, wanted := range []string{"new same-day continuity", "durable imported sentinel"} {
		if !strings.Contains(afterDream.Content, wanted) {
			t.Fatalf("same-day dream lost %q: %s", wanted, afterDream.Content)
		}
	}
	if afterDream.SynthesizerID != "daily-v2" {
		t.Fatalf("daily synthesizer not restored: %q", afterDream.SynthesizerID)
	}
	foundLegacy := false
	for _, source := range afterDream.Sources {
		if strings.HasPrefix(source.ID, "legacy-hub/") {
			foundLegacy = true
		}
	}
	if !foundLegacy {
		t.Fatalf("same-day dream lost import provenance: %#v", afterDream.Sources)
	}
}

func TestImportLegacyHubFailureAndBudgetOverflowPreserveLastKnownGood(t *testing.T) {
	for _, test := range []struct {
		name  string
		setup func(*Engine)
	}{
		{name: "activation interruption", setup: func(engine *Engine) {
			engine.FaultPoint = func(point string) error {
				if point == "after_publish_version" {
					return errors.New("injected")
				}
				return nil
			}
		}},
		{name: "budget overflow", setup: func(engine *Engine) { engine.Budgets["L1"] = 4 }},
	} {
		t.Run(test.name, func(t *testing.T) {
			engine := testEngine(t)
			now := engine.Now()
			old := Artifact{SchemaVersion: 1, WorkspaceID: "case-a", Layer: "L1", Period: now.Format("2006-01-02"), GeneratedAt: now.Add(-time.Hour), SourceFingerprint: strings.Repeat("1", 64), Sources: []SourceRef{{ID: "old", SHA256: strings.Repeat("2", 64)}}, SynthesizerID: "daily-v1", Content: "old"}
			if err := engine.activate("case-a", []Artifact{old}); err != nil {
				t.Fatal(err)
			}
			test.setup(engine)
			_, err := engine.ImportLegacyHub(context.Background(), LegacyHubImportRequest{WorkspaceID: "case-a", ImportedAt: now, Sources: []LegacyHubSource{legacySource("a", "recent", "new memory")}})
			if err == nil {
				t.Fatal("expected import failure")
			}
			engine.FaultPoint = nil
			active, _, readErr := engine.readArtifactByKey("case-a", "L1/"+now.Format("2006-01-02"))
			if readErr != nil || active.Content != "old" {
				t.Fatalf("last known good changed: %#v, %v", active, readErr)
			}
		})
	}
}

func TestImportLegacyHubRecoversCommitBeforeResponseAndFailsClosedOnHistoryOverflow(t *testing.T) {
	engine := testEngine(t)
	request := LegacyHubImportRequest{WorkspaceID: "case-a", ImportedAt: engine.Now(), Sources: []LegacyHubSource{legacySource("a", "recent", "committed")}}
	engine.FaultPoint = func(point string) error {
		if point == "after_commit" {
			return errors.New("response interrupted")
		}
		return nil
	}
	result, err := engine.ImportLegacyHub(context.Background(), request)
	if err != nil || result.State != "already_applied" {
		t.Fatalf("post-commit recovery = %#v, %v", result, err)
	}
	engine.FaultPoint = nil

	commits := filepath.Join(engine.workspaceRoot("case-b"), "commits")
	if err := os.MkdirAll(commits, 0o700); err != nil {
		t.Fatal(err)
	}
	for index := 0; index <= legacyHubHistoryLimit; index++ {
		if err := os.WriteFile(filepath.Join(commits, fmt.Sprintf("%04d.json", index)), []byte("{}"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	_, err = engine.ImportLegacyHub(context.Background(), LegacyHubImportRequest{WorkspaceID: "case-b", ImportedAt: engine.Now(), Sources: []LegacyHubSource{legacySource("b", "recent", "bounded")}})
	if err == nil || !strings.Contains(err.Error(), "history") {
		t.Fatalf("history overflow error = %v", err)
	}
}

func TestImportLegacyHubSnapshotInterruptionIsInertReusableAndConflictFailsClosed(t *testing.T) {
	engine := testEngine(t)
	request := LegacyHubImportRequest{WorkspaceID: "case-a", ImportedAt: engine.Now(), Sources: []LegacyHubSource{legacySource("a", "recent", "snapshot sentinel")}}
	engine.FaultPoint = func(point string) error {
		if point == "after_publish_legacy_hub_snapshots" {
			return errors.New("interrupted")
		}
		return nil
	}
	if _, err := engine.ImportLegacyHub(context.Background(), request); err == nil {
		t.Fatal("expected snapshot interruption")
	}
	if _, _, err := engine.readArtifactByKey("case-a", "L1/"+engine.Now().Format("2006-01-02")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("inert snapshot became active: %v", err)
	}
	engine.FaultPoint = nil
	result, err := engine.ImportLegacyHub(context.Background(), request)
	if err != nil || result.State != "applied" {
		t.Fatalf("identical snapshot retry = %#v, %v", result, err)
	}

	conflictEngine := testEngine(t)
	sources, err := validateLegacyHubSources(request.Sources)
	if err != nil {
		t.Fatal(err)
	}
	importID := legacyHubImportID(request.WorkspaceID, sources)
	conflict := filepath.Join(conflictEngine.workspaceRoot("case-a"), "imports", "legacy-hub", importID, request.Sources[0].CandidateID+".md")
	if err := os.MkdirAll(filepath.Dir(conflict), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(conflict, []byte("conflicting bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := conflictEngine.ImportLegacyHub(context.Background(), request); err == nil || !strings.Contains(err.Error(), "conflict") {
		t.Fatalf("conflicting immutable snapshot error = %v", err)
	}
	if _, _, err := conflictEngine.readArtifactByKey("case-a", "L1/"+engine.Now().Format("2006-01-02")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("conflicting snapshot became active: %v", err)
	}
}

func legacySource(idDigit, layer, content string) LegacyHubSource {
	digest := sha256.Sum256([]byte(content))
	return LegacyHubSource{CandidateID: strings.Repeat(idDigit, 32), LegacyLayer: layer, SHA256: hex.EncodeToString(digest[:]), Content: []byte(content)}
}
