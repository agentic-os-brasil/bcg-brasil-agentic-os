package ownerctx

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"
)

func namespaceTestRoot(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "brain")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := Initialize(root); err != nil {
		t.Fatal(err)
	}
	if err := withOwnerTransition(root, func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	return root
}
func namespaceFiles(t *testing.T, root string) []string {
	t.Helper()
	var files []string
	if err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		files = append(files, path)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	sort.Strings(files)
	return files
}
func TestReadWeeklySelfOwnerIDDoesNotInitializeOrInvent(t *testing.T) {
	root := namespaceTestRoot(t)
	before := namespaceFiles(t, root)
	label, err := ReadWeeklySelfOwnerID(root)
	if err != nil || label != "" {
		t.Fatalf("unbound namespace was not reported honestly: %q %v", label, err)
	}
	if after := namespaceFiles(t, root); !reflect.DeepEqual(before, after) {
		t.Fatal("namespace read initialized private state")
	}
}

func TestReadWeeklySelfOwnerIDPreservesBoundLabelsAndRefusesConflict(t *testing.T) {
	for _, tc := range []struct {
		name, history, state, want string
		conflict                   bool
	}{
		{name: "configured-empty-history", history: "history-owner", want: "history-owner"},
		{name: "weekly-watermark", state: "weekly-owner", want: "weekly-owner"},
		{name: "agreed", history: "existing-owner", state: "existing-owner", want: "existing-owner"},
		{name: "inconsistent", history: "history-owner", state: "other-owner", conflict: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := namespaceTestRoot(t)
			if tc.history != "" {
				config := DefaultPromptHistoryConfig()
				if err := ConfigurePromptHistory(root, config); err != nil {
					t.Fatal(err)
				}
				// This source-only fixture represents an already-bound config with
				// zero retained entries; it carries no native/model authority.
				config.OwnerID = tc.history
				_, path, err := promptHistoryPaths(root)
				if err != nil {
					t.Fatal(err)
				}
				body, _ := json.Marshal(config)
				if err = atomicPrivateWrite(path, body); err != nil {
					t.Fatal(err)
				}
			}
			if tc.state != "" {
				err := withWeeklySelfState(root, true, func(state *weeklySelfState) error {
					*state = weeklySelfState{SchemaVersion: 1, OwnerID: tc.state, Revision: 1, LastScheduledFor: time.Unix(1700000000, 0).UTC(), LastOccurrenceID: strings.Repeat("a", 64), LastInputSHA256: strings.Repeat("b", 64)}
					return writeWeeklySelfState(root, *state)
				})
				if err != nil {
					t.Fatal(err)
				}
			}
			before := namespaceFiles(t, root)
			label, err := ReadWeeklySelfOwnerID(root)
			if tc.conflict {
				if err == nil || label != "" {
					t.Fatal("inconsistent owner labels were relabelled", label, err)
				}
			} else if err != nil || label != tc.want {
				t.Fatalf("existing namespace lost: got %q want %q: %v", label, tc.want, err)
			}
			if after := namespaceFiles(t, root); !reflect.DeepEqual(before, after) {
				t.Fatal("namespace read changed private state membership")
			}
		})
	}
}
