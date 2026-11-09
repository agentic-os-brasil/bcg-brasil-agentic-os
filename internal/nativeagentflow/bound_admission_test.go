package nativeagentflow_test

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/nativeagentflow"
)

func admittedFlowFixture(t *testing.T, workspace, session string, active bool) (nativeagentflow.Store, string) {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	store, err := nativeagentflow.NewBound(root, workspace)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.BeginOccurrence(session, "prompt"); err != nil {
		t.Fatal(err)
	}
	if active {
		if err := store.Admit(session, "prompt", "dispatch", "case-agent"); err != nil {
			t.Fatal(err)
		}
		if err := store.Correlate(session, "prompt", "agent", "case-agent"); err != nil {
			t.Fatal(err)
		}
		if err := store.Authorize(session, "prompt", "agent", "case-agent"); err != nil {
			t.Fatalf("valid single-link fixture refused: %v", err)
		}
	}
	var path string
	if err := filepath.WalkDir(root, func(candidate string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() && filepath.Ext(candidate) == ".json" {
			if path != "" {
				t.Fatal("ambiguous state fixture")
			}
			path = candidate
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if path == "" {
		t.Fatal("public flow did not persist state")
	}
	return store, path
}
func readFlowBytes(t *testing.T, path string) []byte {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return body
}
func replaceFlowBytes(t *testing.T, path string, body []byte) {
	t.Helper()
	if err := os.WriteFile(path, body, 0600); err != nil {
		t.Fatal(err)
	}
}
func rejectStoredAuthority(t *testing.T, store nativeagentflow.Store, path, session string) {
	t.Helper()
	before := readFlowBytes(t, path)
	if err := store.Authorize(session, "prompt", "agent", "case-agent"); err == nil {
		t.Error("invalid stored identity authorized child")
	}
	if after := readFlowBytes(t, path); !bytes.Equal(before, after) {
		t.Error("authority read repaired stored bytes")
	}
	if err := store.ObserveStop(session, "prompt", "agent", "case-agent"); err == nil {
		t.Error("invalid stored identity admitted transition")
	}
	if after := readFlowBytes(t, path); !bytes.Equal(before, after) {
		t.Error("rejected stored identity was overwritten")
	}
}

func TestBoundStoredIdentityMustBePresentAndNonNull(t *testing.T) {
	for _, field := range []string{"schema_version", "profile", "workspace_id", "session_digest"} {
		for _, mode := range []string{"absent", "null"} {
			t.Run(field+"/"+mode, func(t *testing.T) {
				store, path := admittedFlowFixture(t, "workspace", "session", true)
				var fields map[string]json.RawMessage
				if err := json.Unmarshal(readFlowBytes(t, path), &fields); err != nil {
					t.Fatal(err)
				}
				if mode == "absent" {
					delete(fields, field)
				} else {
					fields[field] = json.RawMessage("null")
				}
				body, err := json.Marshal(fields)
				if err != nil {
					t.Fatal(err)
				}
				replaceFlowBytes(t, path, body)
				rejectStoredAuthority(t, store, path, "session")
			})
		}
	}
}
func TestBoundStoredIdentityCannotBeCopiedAcrossBindings(t *testing.T) {
	for _, scope := range []string{"workspace", "session"} {
		for _, mode := range []string{"complete", "missing_binding"} {
			t.Run(scope+"/"+mode, func(t *testing.T) {
				_, source := admittedFlowFixture(t, "workspace-source", "session-source", true)
				workspace, session, field := "workspace-source", "session-target", "session_digest"
				if scope == "workspace" {
					workspace, session, field = "workspace-target", "session-source", "workspace_id"
				}
				target, path := admittedFlowFixture(t, workspace, session, true)
				body := readFlowBytes(t, source)
				if mode == "missing_binding" {
					var fields map[string]json.RawMessage
					if err := json.Unmarshal(body, &fields); err != nil {
						t.Fatal(err)
					}
					delete(fields, field)
					var err error
					body, err = json.Marshal(fields)
					if err != nil {
						t.Fatal(err)
					}
				}
				replaceFlowBytes(t, path, body)
				rejectStoredAuthority(t, target, path, session)
			})
		}
	}
}
func TestBoundAuthorityRejectsHardlinksWithoutWrites(t *testing.T) {
	for _, active := range []bool{true, false} {
		name := "main"
		if active {
			name = "child"
		}
		t.Run(name, func(t *testing.T) {
			store, path := admittedFlowFixture(t, "workspace", "session", active)
			check := func() error {
				if active {
					return store.Authorize("session", "prompt", "agent", "case-agent")
				}
				return store.CheckMain("session", "prompt")
			}
			if err := check(); err != nil {
				t.Fatalf("single-link control refused: %v", err)
			}
			alias := filepath.Join(filepath.Dir(path), "authority-alias")
			if err := os.Link(path, alias); err != nil {
				t.Fatal(err)
			}
			before := readFlowBytes(t, path)
			info, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			entries, err := os.ReadDir(filepath.Dir(path))
			if err != nil {
				t.Fatal(err)
			}
			if err := check(); err == nil {
				t.Error("hardlinked authority state admitted")
			}
			afterInfo, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			afterEntries, err := os.ReadDir(filepath.Dir(path))
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(before, readFlowBytes(t, path)) || !info.ModTime().Equal(afterInfo.ModTime()) || info.Mode() != afterInfo.Mode() || len(entries) != len(afterEntries) {
				t.Error("authority check mutated private store")
			}
			if err := os.Remove(alias); err != nil {
				t.Fatal(err)
			}
			if err := check(); err != nil {
				t.Fatalf("restored single-link control refused: %v", err)
			}
		})
	}
}
