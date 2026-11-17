package ownerctx

import (
	"bytes"
	"context"
	"os"
	"testing"
)

func TestWeeklySelfSourceInvalidationPurgesBeforeOwningMutation(t *testing.T) {
	for _, working := range []bool{false, true} {
		t.Run(map[bool]string{false: "pending", true: "working"}[working], func(t *testing.T) {
			root, options, now := weeklyCoreRoot(t)
			input, err := FreezeWeeklySelf(root, options, nil, now)
			if err != nil {
				t.Fatal(err)
			}
			if working {
				if _, err := PublishWeeklySelf(context.Background(), root, input.InputSHA256, weeklyCandidate(input), 0, func(write func() error) error { return write() }, now); err != nil {
					t.Fatal(err)
				}
			}
			before, _ := os.ReadFile(weeklySelfPath(root))
			changed, err := invalidateWeeklySelfSourcesLocked(root, weeklySelfSourceChange{Facets: []string{"unselected-facet"}})
			if err != nil || changed {
				t.Fatal("unrelated source invalidated projection", err)
			}
			unchanged, _ := os.ReadFile(weeklySelfPath(root))
			if !bytes.Equal(before, unchanged) {
				t.Fatal("unrelated change rewrote private state")
			}
			changed, err = invalidateWeeklySelfSourcesLocked(root, weeklySelfSourceChange{Facets: []string{"communication-style"}})
			if err != nil || !changed {
				t.Fatal("selected source was not physically purged", err)
			}
			state, err := readWeeklySelfState(root)
			if err != nil || state.Pending != nil || state.Working != nil || state.LastInputSHA256 != input.InputSHA256 {
				t.Fatal("purge lost replay watermark or retained private view", err)
			}
			after, _ := os.ReadFile(weeklySelfPath(root))
			if bytes.Contains(after, []byte("Concise messages; explicit next step.")) {
				t.Fatal("working text retained after source barrier")
			}
			if _, err := FreezeWeeklySelf(root, options, nil, now); err == nil {
				t.Fatal("invalidated occurrence resurrected")
			}
		})
	}
}

func TestWeeklySelfInvalidationAbsentDoesNotBootstrapAndMissingLockFailsClosed(t *testing.T) {
	root := t.TempDir()
	if changed, err := invalidateWeeklySelfSourcesLocked(root, weeklySelfSourceChange{All: true}); err != nil || changed {
		t.Fatal("absent source purge failed", err)
	}
	entries, _ := os.ReadDir(root)
	if len(entries) != 0 {
		t.Fatal("source barrier created owner state")
	}
	root, options, now := weeklyCoreRoot(t)
	if _, err := FreezeWeeklySelf(root, options, nil, now); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(root + "/owner/self/weekly/.lock"); err != nil {
		t.Fatal(err)
	}
	if _, err := invalidateWeeklySelfSourcesLocked(root, weeklySelfSourceChange{All: true}); err == nil {
		t.Fatal("existing private state with missing lock was silently accepted")
	}
}
