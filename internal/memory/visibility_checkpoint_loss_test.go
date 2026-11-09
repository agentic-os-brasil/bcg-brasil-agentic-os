package memory

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Checkpoint damage must preserve revocation on every admitted backend.
func TestOwnedMemoryCheckpointLossNeverReopensHistory(t *testing.T) {
	for _, damage := range []string{"missing", "missing-directory", "corrupt", "symlink", "hardlink", "missing-marker"} {
		t.Run(damage, func(t *testing.T) {
			e := testEngine(t)
			if err := e.PrepareOwnedCaptureAuthority("case-a"); err != nil {
				t.Fatal(err)
			}
			day := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
			reviewed := ownedInput(t, e, "", 0, "checkpoint-denied-marker", day)
			r, err := e.CaptureOwned("case-a", reviewed)
			if err != nil {
				t.Fatal(err)
			}
			e.Synthesizer = DeterministicL1Synthesizer{MaxRunes: 4000, MaxEntries: 64, MaxInputBytes: 1 << 20, MaxInputEntries: 256, Attestor: CaptureAttestor{Root: e.Root}}
			if _, err := e.DreamDailyAttested(context.Background(), "case-a", day); err != nil {
				t.Fatal(err)
			}
			old, _, err := e.latestManifest("case-a")
			if err != nil {
				t.Fatal(err)
			}
			e.FaultPoint = func(point string) error {
				if point == "after_visibility_barrier" {
					return errors.New("interrupted cleanup")
				}
				return nil
			}
			if _, err = e.DeleteCapture("case-a", r.ID, 1); err == nil {
				t.Fatal("fixture did not interrupt")
			}
			e.FaultPoint = nil
			directory := filepath.Join(e.workspaceRoot("case-a"), ".visibility")
			checkpoint := filepath.Join(directory, "checkpoint.json")
			body, err := os.ReadFile(checkpoint)
			if err != nil {
				t.Fatal(err)
			}
			marker := filepath.Join(e.workspaceRoot("case-a"), ".visibility-checkpoint-required")
			requirement, err := os.ReadFile(marker)
			if err != nil {
				t.Fatal(err)
			}
			legacy, _ := json.Marshal(visibilityBarrier{SchemaVersion: 1, WorkspaceID: "case-a", Revision: 1, Denied: map[string]uint64{}})
			if err := os.WriteFile(filepath.Join(directory, "00000000000000000001.json"), legacy, 0600); err != nil {
				t.Fatal(err)
			}
			if damage == "missing-marker" {
				err = os.Remove(marker)
			} else if damage == "missing-directory" {
				err = os.RemoveAll(directory)
			} else {
				if err := os.MkdirAll(directory, 0700); err != nil {
					t.Fatal(err)
				}
				if err = os.Remove(checkpoint); err == nil {
					switch damage {
					case "corrupt":
						err = os.WriteFile(checkpoint, []byte(`{"schema_version":1`), 0600)
					case "symlink", "hardlink":
						external := filepath.Join(t.TempDir(), "old-checkpoint")
						if err = os.WriteFile(external, legacy, 0600); err == nil {
							if damage == "symlink" {
								err = os.Symlink(external, checkpoint)
							} else {
								err = os.Link(external, checkpoint)
							}
						}
					}
				}
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err := e.ReadCapture("case-a", r.ID); err == nil {
				t.Fatal("damaged checkpoint exposed capture")
			}
			if _, err := e.ListCaptures("case-a", 0, 10); err == nil {
				t.Fatal("damaged checkpoint exposed metadata")
			}
			if _, err := e.AssembleContext("case-a"); err == nil {
				t.Fatal("damaged checkpoint became fresh/empty state")
			}
			if _, _, err := e.readArtifactFromManifest("case-a", old, "L1/2026-07-20"); err == nil {
				t.Fatal("cached artifact bypassed checkpoint")
			}
			if _, err := e.CaptureOwned("case-a", reviewed); err == nil {
				t.Fatal("old seal bypassed checkpoint")
			}
			if damage == "missing-marker" {
				if err := os.WriteFile(marker, requirement, 0600); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := os.MkdirAll(directory, 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.Remove(checkpoint); err != nil && !errors.Is(err, os.ErrNotExist) {
					t.Fatal(err)
				}
				if err := os.WriteFile(checkpoint, body, 0600); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := e.CaptureOwned("case-a", reviewed); !errors.Is(err, ErrCaptureConflict) {
				t.Fatalf("old seal resurrected after checkpoint recovery: %v", err)
			}
			if _, err := e.DeleteCapture("case-a", r.ID, 1); err != nil {
				t.Fatal(err)
			}
		})
	}
}
