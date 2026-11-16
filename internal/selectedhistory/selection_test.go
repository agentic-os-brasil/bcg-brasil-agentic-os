package selectedhistory

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func selectionFixture(t *testing.T) (Manifest, string) {
	t.Helper()
	root := t.TempDir()
	workspace := t.TempDir()
	path := filepath.Join(root, "session.jsonl")
	raw, _ := json.Marshal(map[string]any{"type": "user", "cwd": workspace, "message": map[string]string{"content": "synthetic private prompt"}})
	if err := os.WriteFile(path, append(raw, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	return Manifest{[]string{workspace}, []Source{{Kind: "claude_code", Workspace: workspace, Root: root, Sessions: []Session{{Path: "session.jsonl", ID: "session-a"}}}}}, path
}
func TestSelectedHistoryExactBatchAndMetadataChange(t *testing.T) {
	manifest, path := selectionFixture(t)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := Prepare(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Count != 1 || plan.Bytes != int64(len(before)) {
		t.Fatal(plan)
	}
	out, err := Read(plan, plan.Digest, 0, 5)
	if err != nil || len(out.Sessions) != 1 || !out.Complete || out.Sessions[0].Body != string(before) {
		t.Fatal(out, err)
	}
	after, err := os.ReadFile(path)
	if err != nil || string(after) != string(before) {
		t.Fatal("source changed", err)
	}
	if _, err = Read(plan, "wrong", 0, 5); err == nil {
		t.Fatal("foreign digest accepted")
	}
	if err = os.WriteFile(path, append(before, []byte("{}\n")...), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = Read(plan, plan.Digest, 0, 5); err == nil {
		t.Fatal("changed selected source accepted")
	}
}
func TestSelectedHistoryExplicitScopeAliasesAndBounds(t *testing.T) {
	for _, kind := range []string{"traversal", "glob", "wrong extension", "duplicate", "foreign workspace", "wrong cwd", "hardlink", "symlink"} {
		t.Run(kind, func(t *testing.T) {
			manifest, path := selectionFixture(t)
			switch kind {
			case "traversal":
				manifest.Sources[0].Sessions[0].Path = "../session.jsonl"
			case "glob":
				manifest.Sources[0].Sessions[0].Path = "*.jsonl"
			case "wrong extension":
				manifest.Sources[0].Sessions[0].Path = "session.zip"
			case "duplicate":
				manifest.Sources[0].Sessions = append(manifest.Sources[0].Sessions, manifest.Sources[0].Sessions[0])
			case "foreign workspace":
				manifest.Sources[0].Workspace = t.TempDir()
			case "wrong cwd":
				if err := os.WriteFile(path, []byte(`{"cwd":"foreign-workspace"}`), 0600); err != nil {
					t.Fatal(err)
				}
			case "hardlink":
				if err := os.Link(path, path+".alias"); err != nil {
					t.Skip(err)
				}
			case "symlink":
				if err := os.Rename(path, path+".original"); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(path+".original", path); err != nil {
					t.Skip(err)
				}
			}
			plan, err := Prepare(manifest)
			if kind == "wrong cwd" {
				if err != nil {
					t.Fatal(err)
				}
				if _, err = Read(plan, plan.Digest, 0, 5); err == nil {
					t.Fatal("foreign transcript read")
				}
			} else if err == nil {
				t.Fatal("unsafe selection admitted")
			}
		})
	}
}
func TestSelectedHistoryAllSourcesRevalidatedAndExportsBounded(t *testing.T) {
	manifest, path := selectionFixture(t)
	second := filepath.Join(filepath.Dir(path), "other.jsonl")
	if err := os.WriteFile(second, []byte("{}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	manifest.Sources[0].Sessions = append(manifest.Sources[0].Sessions, Session{Path: "other.jsonl", ID: "other"})
	plan, err := Prepare(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(second, []byte(`{"changed":true}`), 0600); err != nil {
		t.Fatal(err)
	}
	if out, err := Read(plan, plan.Digest, 0, 1); err == nil || len(out.Sessions) != 0 {
		t.Fatal("unread changed source failed to invalidate entire selection", err)
	}
	export := filepath.Join(filepath.Dir(path), "export.json")
	if err = os.WriteFile(export, []byte(`{"not":"array"}`), 0600); err != nil {
		t.Fatal(err)
	}
	manifest.Sources[0].Kind = "claude_export"
	manifest.Sources[0].Sessions = []Session{{Path: "export.json", ID: "export"}}
	plan, err = Prepare(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = Read(plan, plan.Digest, 0, 5); err == nil {
		t.Fatal("unbounded container admitted")
	}
}

func TestSelectedHistoryRejectsPhysicalCaseSpellingAliases(t *testing.T) {
	for _, split := range []bool{false, true} {
		t.Run(fmt.Sprint("separate_sources=", split), func(t *testing.T) {
			manifest, path := selectionFixture(t)
			alias := filepath.Join(filepath.Dir(path), "SESSION.jsonl")
			original, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			aliased, err := os.Stat(alias)
			if errors.Is(err, os.ErrNotExist) {
				t.Skip("filesystem distinguishes case spellings")
			}
			if err != nil {
				t.Fatal(err)
			}
			if !os.SameFile(original, aliased) {
				t.Fatal("fixture must use the same physical file")
			}
			other := Session{Path: "SESSION.jsonl", ID: "independent-looking-session"}
			if split {
				source := manifest.Sources[0]
				source.Sessions = []Session{other}
				manifest.Sources = append(manifest.Sources, source)
			} else {
				manifest.Sources[0].Sessions = append(manifest.Sources[0].Sessions, other)
			}
			if _, err := Prepare(manifest); err == nil {
				t.Fatal("same physical source admitted twice under case spelling aliases")
			}
		})
	}
}
func TestSelectedHistoryKeepsDifferentPhysicalFiles(t *testing.T) {
	for _, caseSensitive := range []bool{false, true} {
		t.Run(fmt.Sprint("case_sensitive_names=", caseSensitive), func(t *testing.T) {
			manifest, path := selectionFixture(t)
			otherPath := filepath.Join(filepath.Dir(path), "other.jsonl")
			if caseSensitive {
				otherPath = filepath.Join(filepath.Dir(path), "SESSION.jsonl")
			}
			other, err := os.OpenFile(otherPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
			if caseSensitive && errors.Is(err, os.ErrExist) {
				t.Skip("filesystem aliases different-case spellings")
			}
			if err != nil {
				t.Fatal(err)
			}
			body, err := os.ReadFile(path)
			if err != nil {
				other.Close()
				t.Fatal(err)
			}
			_, writeErr := other.Write(body)
			closeErr := other.Close()
			if err = errors.Join(writeErr, closeErr); err != nil {
				t.Fatal(err)
			}
			originalInfo, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			otherInfo, err := os.Stat(otherPath)
			if err != nil {
				t.Fatal(err)
			}
			if os.SameFile(originalInfo, otherInfo) {
				t.Fatal("positive fixture must have distinct physical files")
			}
			manifest.Sources[0].Sessions = append(manifest.Sources[0].Sessions, Session{Path: filepath.Base(otherPath), ID: "second-physical-session"})
			plan, err := Prepare(manifest)
			if err != nil {
				t.Fatal("distinct physical files were merged by path spelling", err)
			}
			batch, err := Read(plan, plan.Digest, 0, 5)
			if err != nil || len(batch.Sessions) != 2 || batch.Sessions[0].Body != string(body) || batch.Sessions[1].Body != string(body) {
				t.Fatal("two physical sources were not preserved", err)
			}
			if batch.Sessions[0].Session == batch.Sessions[1].Session {
				t.Fatal("distinct sources lost their provenance")
			}
		})
	}
}
