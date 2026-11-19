package memory

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func modelWeeklyFixture(t *testing.T) (*Engine, time.Time) {
	t.Helper()
	engine := testEngine(t)
	engine.Synthesizer = &fakeSynthesizer{outputs: map[string]string{"L1": "eligible source continuity"}}
	engine.SynthesizerID = "model-memory-weekly-v1"
	engine.Eligibility = EligibilityFunc(func(context.Context, Artifact) (bool, string, error) {
		return false, "exact owner review required", nil
	})
	at := engine.Now()
	seedDailyDreams(t, engine, "case-a", at)
	return engine, at
}

func TestModelWeeklyPublishesOnlyThroughAtomicMemoryEngine(t *testing.T) {
	engine, at := modelWeeklyFixture(t)
	input, err := engine.InspectModelWeeklyInput("case-a", at)
	if err != nil {
		t.Fatal("eligible frozen memory cannot reach model candidate boundary:", err)
	}
	candidate := ModelWeeklyCandidate{SchemaVersion: 1, JobID: ModelWeeklyJobID, WorkspaceID: "case-a", Period: input.Period, InputSHA256: input.InputSHA256, Content: map[string]string{"L2": "Model weekly rollup", "L3": "Model thematic continuity", "lifetime": "Owner-reviewed promotion candidate"}}
	body, _ := json.Marshal(candidate)
	candidate, err = ParseModelWeeklyCandidate(input, body)
	if err != nil {
		t.Fatal(err)
	}
	result, err := engine.CommitModelWeekly(context.Background(), input, candidate, at)
	if err != nil || strings.Join(result.ActivatedLayers, ",") != "L2,L3" {
		t.Fatalf("bounded model candidate did not atomically publish L2/L3: %+v %v", result, err)
	}
	for _, layer := range []string{"L2/" + input.Period, "L3/current"} {
		if _, _, err := engine.readArtifactByKey("case-a", layer); err != nil {
			t.Fatal("model output bypassed canonical readback:", err)
		}
	}
	if _, _, err := engine.readArtifactByKey("case-a", "lifetime/current"); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("model bypassed independent lifetime review", err)
	}
	if _, err := engine.InspectModelWeeklyInput("case-a", at); !errors.Is(err, ErrModelWeeklyUnchanged) {
		t.Fatal("unchanged input would invoke the model again", err)
	}
	probe, err := engine.InspectModelWeeklyProbeInput("case-a", at)
	if err != nil {
		t.Fatal("current committed cohort cannot be independently probed:", err)
	}
	candidate.InputSHA256 = probe.InputSHA256
	if _, err := engine.CommitModelWeekly(context.Background(), probe, candidate, at); err == nil {
		t.Fatal("probe input published domain memory")
	}
}

// This is an offline storage/old-reader compatibility test, not native process,
// qualification, enrollment, calendar or model execution evidence.
func TestModelWeeklySidecarKeepsOriginalStrictReaderContract(t *testing.T) {
	e, at := modelWeeklyFixture(t)
	input, err := e.InspectModelWeeklyInput("case-a", at)
	if err != nil {
		t.Fatal(err)
	}
	candidate := ModelWeeklyCandidate{SchemaVersion: 1, JobID: ModelWeeklyJobID, WorkspaceID: "case-a", Period: input.Period, InputSHA256: input.InputSHA256, Content: map[string]string{"L2": "weekly", "L3": "continuity", "lifetime": "pending exact candidate"}}
	if _, err = e.CommitModelWeekly(context.Background(), input, candidate, at); err != nil {
		t.Fatal(err)
	}
	manifest, name, err := e.latestManifest("case-a")
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(e.workspaceRoot("case-a"), "commits", name))
	if err != nil {
		t.Fatal(err)
	}
	// These are the original strict v1 fields, in their original order. The
	// legacy decoder neither knows nor needs a model record or extra schema key.
	var legacy struct {
		VisibilityRevision uint64            `json:"visibility_revision,omitempty"`
		SchemaVersion      int               `json:"schema_version"`
		WorkspaceID        string            `json:"workspace_id"`
		TransactionID      string            `json:"transaction_id"`
		ParentCommit       string            `json:"parent_commit,omitempty"`
		CommittedAt        time.Time         `json:"committed_at"`
		Artifacts          map[string]string `json:"artifacts"`
	}
	if err = decodeMemoryJSON(body, &legacy); err != nil {
		t.Fatal("original strict consumer rejected model commit:", err)
	}
	originalJSON, _ := json.Marshal(manifest)
	legacyJSON, _ := json.Marshal(legacy)
	if string(originalJSON) != string(legacyJSON) {
		t.Fatal("original manifest changed on old-reader roundtrip")
	}
	for key, relative := range legacy.Artifacts {
		artifact, e2 := e.readArtifact(filepath.Join(e.workspaceRoot("case-a"), filepath.FromSlash(relative)))
		if e2 != nil || validateArtifactStructure(artifact) != nil || artifactKey(artifact) != key {
			t.Fatal("original canonical artifact reader rejected model output", e2)
		}
	}
	if err = e.purgeGenerated("case-a"); err != nil {
		t.Fatal(err)
	}
	if _, _, err = e.ReadModelWeeklyCommit("case-a"); err != nil {
		t.Fatal("current-generation cleanup removed valid anchored sidecar:", err)
	}
	l3path := filepath.Join(e.workspaceRoot("case-a"), filepath.FromSlash(manifest.Artifacts["L3/current"]))
	sidecar := filepath.Join(filepath.Dir(l3path), "model-weekly.json")
	body, err = os.ReadFile(sidecar)
	if err != nil {
		t.Fatal(err)
	}
	changed := strings.Replace(string(body), "pending exact candidate", "tampered candidate text", 1)
	if changed == string(body) {
		t.Fatal("test did not change exact candidate")
	}
	if err = os.WriteFile(sidecar, []byte(changed), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err = e.ReadModelWeeklyCommit("case-a"); err == nil {
		t.Fatal("unanchored sidecar edit was accepted")
	}
	if err = os.WriteFile(sidecar, body, 0o600); err != nil {
		t.Fatal(err)
	}
	if err = os.Remove(sidecar); err != nil {
		t.Fatal(err)
	}
	if _, _, err = e.ReadModelWeeklyCommit("case-a"); !errors.Is(err, ErrCaptureConflict) {
		t.Fatal("missing anchored record was treated as ordinary absence", err)
	}
}

func TestModelWeeklyCommittedEffectRetainsOriginalExecutionAfterFault(t *testing.T) {
	// Execution here is synthetic storage metadata, never native qualification.
	e, at := modelWeeklyFixture(t)
	input, err := e.InspectModelWeeklyInput("case-a", at)
	if err != nil {
		t.Fatal(err)
	}
	candidate := ModelWeeklyCandidate{SchemaVersion: 1, JobID: ModelWeeklyJobID, WorkspaceID: "case-a", Period: input.Period, InputSHA256: input.InputSHA256, Content: map[string]string{"L2": "weekly", "L3": "continuity", "lifetime": "pending candidate"}}
	d := strings.Repeat("a", 64)
	execution := &ModelWeeklyExecution{RegistrationSHA256: d, QualificationSHA256: d, ActivationSHA256: d, ImplementationSHA256: d, WorkspaceID: "case-a", CommandID: "synthetic-storage-command", AttemptID: strings.Repeat("b", 32), OccurrenceSHA256: d, ScheduledFor: at, RequestedAt: at, Deadline: at.Add(time.Minute)}
	e.FaultPoint = func(point string) error {
		if point == "after_commit" {
			return errors.New("synthetic post-commit fault")
		}
		return nil
	}
	if _, err = e.CommitModelWeeklyWithExecution(context.Background(), input, candidate, execution, at); err == nil {
		t.Fatal("fault did not exercise post-commit boundary")
	}
	committed, _, err := e.ReadModelWeeklyCommit("case-a")
	if err != nil || committed.Execution == nil || *committed.Execution != *execution {
		t.Fatal("original effect identity lost after atomic commit", err)
	}
}

func TestModelWeeklyPendingExactReviewAndSourceDenial(t *testing.T) {
	for _, deny := range []bool{false, true} {
		name := "approved"
		if deny {
			name = "source-deleted"
		}
		t.Run(name, func(t *testing.T) {
			e := generationEngine(t, 2)
			e.SynthesizerID = "weekly-memory-model-v1:engineering-test"
			e.Eligibility = EligibilityFunc(func(ctx context.Context, a Artifact) (bool, string, error) {
				ok, reason, err := (DeterministicLifetimeEligibility{MinL3Generations: 2}).Evaluate(ctx, a)
				if err != nil || !ok {
					return false, reason, err
				}
				return false, "trusted exact-candidate confirmation unavailable", nil
			})
			day := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
			var first CaptureReceipt
			for i := 0; i < 2; i++ {
				r, err := e.CaptureOwned("case-a", ownedInput(t, e, "", 0, "distinct real test source "+string(rune('a'+i)), day))
				if err != nil {
					t.Fatal(err)
				}
				if i == 0 {
					first = r
				}
				if _, err = e.DreamDailyAttested(context.Background(), "case-a", day); err != nil {
					t.Fatal(err)
				}
				input, err := e.InspectModelWeeklyInput("case-a", day)
				if err != nil {
					t.Fatal(err)
				}
				candidate := ModelWeeklyCandidate{SchemaVersion: 1, JobID: ModelWeeklyJobID, WorkspaceID: "case-a", Period: input.Period, InputSHA256: input.InputSHA256, Content: map[string]string{"L2": "bounded weekly source summary", "L3": "bounded thematic continuity", "lifetime": "PRIVATE_PENDING_MODEL_CANDIDATE"}}
				if _, err = e.CommitModelWeekly(context.Background(), input, candidate, day); err != nil {
					t.Fatal(err)
				}
			}
			record, _, err := e.ReadModelWeeklyCommit("case-a")
			if err != nil || record.PendingLifetime == nil {
				t.Fatal("exact candidate was stranded after unchanged-input skip", err)
			}
			if deny {
				if _, err = e.DeleteCapture("case-a", first.ID, first.Revision); err != nil {
					t.Fatal(err)
				}
				if _, _, err = e.ReadModelWeeklyCommit("case-a"); err == nil {
					t.Fatal("deleted source left pending candidate readable")
				}
				if _, err = e.PromoteModelWeeklyLifetime(context.Background(), "case-a", record.Digest()); err == nil {
					t.Fatal("deleted source pending candidate promoted")
				}
				err = filepath.WalkDir(e.Root, func(path string, d os.DirEntry, err error) error {
					if err != nil {
						return err
					}
					if d.Type().IsRegular() {
						body, e := os.ReadFile(path)
						if e != nil {
							return e
						}
						if strings.Contains(string(body), "PRIVATE_PENDING_MODEL_CANDIDATE") {
							t.Errorf("denied pending plaintext remains at %s", path)
						}
					}
					return nil
				})
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			calls := 0
			e.Eligibility = EligibilityFunc(func(ctx context.Context, a Artifact) (bool, string, error) {
				calls++
				ok, reason, err := (DeterministicLifetimeEligibility{MinL3Generations: 2}).Evaluate(ctx, a)
				if err != nil || !ok {
					return false, reason, err
				}
				if a.Content != record.PendingLifetime.Content {
					return false, "", errors.New("candidate changed")
				}
				return true, "exact stored candidate approved", nil
			})
			out, err := e.PromoteModelWeeklyLifetime(context.Background(), "case-a", record.Digest())
			if err != nil || !promoted(out) || calls != 1 {
				t.Fatalf("original guarded pending promotion unreachable: %+v calls=%d %v", out, calls, err)
			}
			if _, _, err = e.readArtifactByKey("case-a", "lifetime/current"); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestModelWeeklyRefusesWrongScopeMalformedAndRevokedPublication(t *testing.T) {
	engine, at := modelWeeklyFixture(t)
	input, err := engine.InspectModelWeeklyInput("case-a", at)
	if err != nil {
		t.Fatal(err)
	}
	valid := ModelWeeklyCandidate{SchemaVersion: 1, JobID: ModelWeeklyJobID, WorkspaceID: "case-a", Period: input.Period, InputSHA256: input.InputSHA256, Content: map[string]string{"L2": "weekly", "L3": "themes", "lifetime": "candidate"}}
	body, _ := json.Marshal(valid)
	for name, raw := range map[string][]byte{
		"wrong-job":       []byte(strings.Replace(string(body), ModelWeeklyJobID, "yoda-self-review-weekly", 1)),
		"wrong-workspace": []byte(strings.Replace(string(body), "case-a", "case-b", 1)),
		"wrong-input":     []byte(strings.Replace(string(body), input.InputSHA256, strings.Repeat("0", 64), 1)),
		"unknown":         []byte(strings.TrimSuffix(string(body), "}") + `,"approval":true}`),
		"duplicate":       []byte(strings.Replace(string(body), `"schema_version":1`, `"schema_version":1,"schema_version":1`, 1)),
		"oversize":        []byte(strings.Replace(string(body), "themes", strings.Repeat("x", input.LayerBytes["L3"]+1), 1)),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseModelWeeklyCandidate(input, raw); err == nil {
				t.Fatal("unsafe model output accepted")
			}
		})
	}
	engine.PublicationGuard = func(func() error) error { return errors.New("revoked") }
	if _, err := engine.CommitModelWeekly(context.Background(), input, valid, at); err == nil {
		t.Fatal("revoked model result published")
	}
	if _, _, err := engine.readArtifactByKey("case-a", "L3/current"); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("failed model commit became active", err)
	}
	engine.PublicationGuard = nil
	if _, err := engine.Capture(Capture{WorkspaceID: "case-a", RecordedAt: at, Kind: "thread", Text: "changed evidence", Sanitized: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.DreamDaily(context.Background(), "case-a", at); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.CommitModelWeekly(context.Background(), input, valid, at); err == nil {
		t.Fatal("changed frozen source accepted")
	}
}
