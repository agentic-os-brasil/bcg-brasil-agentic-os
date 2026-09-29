package zipmigration

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCommittedTargetSafetyAndRecovery(t *testing.T) {
	for _, change := range []string{"file_symlink", "ancestor_symlink", "directory_type", "unreadable_file", "unreadable_directory", "new_symlink_after_edit"} {
		t.Run(change, func(t *testing.T) {
			if runtime.GOOS == "windows" && strings.HasPrefix(change, "unreadable") {
				t.Skip("native Windows ACL acceptance required")
			}
			root := t.TempDir()
			put(t, root, "data/owner/a.md", "original a")
			put(t, root, "data/owner/z/note.md", "original z")
			first := Run(root, Options{})
			if first.State != "committed" {
				t.Fatal(first)
			}
			statePath := filepath.Join(root, control, "state.json")
			receiptPath := filepath.Join(root, control, "attempts", first.AttemptID, "receipt.json")
			originalState, _ := os.ReadFile(statePath)
			originalReceipt, _ := os.ReadFile(receiptPath)
			// An earlier ordinary edit must never short-circuit topology validation.
			put(t, root, "brain/owner/a.md", "legitimate edit")
			target := filepath.Join(root, "brain/owner/z/note.md")
			directory := filepath.Dir(target)
			outside := t.TempDir()
			put(t, outside, "secret.md", "external")
			var restore func()
			switch change {
			case "file_symlink":
				if err := os.Remove(target); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(filepath.Join(outside, "secret.md"), target); err != nil {
					t.Skip(err)
				}
				restore = func() { os.Remove(target); put(t, root, "brain/owner/z/note.md", "original z") }
			case "ancestor_symlink":
				saved := directory + ".saved"
				if err := os.Rename(directory, saved); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(outside, directory); err != nil {
					t.Skip(err)
				}
				restore = func() { os.Remove(directory); os.Rename(saved, directory) }
			case "directory_type":
				saved := directory + ".saved"
				if err := os.Rename(directory, saved); err != nil {
					t.Fatal(err)
				}
				put(t, root, "brain/owner/z", "not a directory")
				restore = func() { os.Remove(directory); os.Rename(saved, directory) }
			case "unreadable_file":
				if err := os.Chmod(target, 0000); err != nil {
					t.Fatal(err)
				}
				restore = func() { os.Chmod(target, 0600) }
			case "unreadable_directory":
				if err := os.Chmod(directory, 0000); err != nil {
					t.Fatal(err)
				}
				restore = func() { os.Chmod(directory, 0700) }
			case "new_symlink_after_edit":
				target = filepath.Join(root, "brain/zz-new-link")
				if err := os.Symlink(outside, target); err != nil {
					t.Skip(err)
				}
				restore = func() { os.Remove(target) }
			}
			t.Cleanup(restore)
			for _, opts := range []Options{{Status: true}, {}, {}} {
				got := Run(root, opts)
				if got.State != "blocked" || got.Current || len(got.Errors) == 0 {
					t.Fatalf("unsafe target accepted: %+v", got)
				}
				saved, _ := os.ReadFile(statePath)
				if string(saved) != string(originalState) {
					t.Fatal("temporary failure overwrote committed state")
				}
			}
			restore()
			recovered := Run(root, Options{})
			if recovered.State != "committed" || recovered.Verification != "target_evolved" || recovered.Current {
				t.Fatal(recovered)
			}
			receipt, _ := os.ReadFile(receiptPath)
			if string(receipt) != string(originalReceipt) {
				t.Fatal("historical receipt changed")
			}
			source, _ := os.ReadFile(filepath.Join(root, "data/owner/a.md"))
			if string(source) != "original a" {
				t.Fatal("source changed")
			}
		})
	}
}

func TestCommittedOrdinaryAddDeleteAndEditRemainUsable(t *testing.T) {
	root := t.TempDir()
	put(t, root, "data/owner/a.md", "a")
	put(t, root, "data/owner/b.md", "b")
	first := Run(root, Options{})
	if first.State != "committed" {
		t.Fatal(first)
	}
	put(t, root, "brain/owner/a.md", "authored edit")
	put(t, root, "brain/new-note.md", "new note")
	if err := os.Remove(filepath.Join(root, "brain/owner/b.md")); err != nil {
		t.Fatal(err)
	}
	got := Run(root, Options{})
	if got.State != "committed" || got.Current || got.Verification != "target_evolved" || len(got.Errors) != 0 {
		t.Fatal(got)
	}
}

func TestBOMCaseMarkersTransformWithoutChangingOriginalBytes(t *testing.T) {
	root := t.TempDir()
	put(t, root, "data/cases/example/brain/canon/note.md", "case")
	for _, marker := range []string{".active", ".pending"} {
		put(t, root, "data/cases/"+marker, "\ufeffexample\r\n")
	}
	got := Run(root, Options{})
	if got.State != "committed" {
		t.Fatal(got)
	}
	for _, marker := range []string{".active", ".pending"} {
		source, _ := os.ReadFile(filepath.Join(root, "data/cases", marker))
		target, _ := os.ReadFile(filepath.Join(root, "brain/accounts", marker))
		if string(source) != "\ufeffexample\r\n" || string(target) != "_sem-conta/example\n" {
			t.Fatalf("wrong marker conversion: source=%q target=%q", source, target)
		}
	}
}

func TestCommittedImplicitAncestorTypeIsUnsafe(t *testing.T) {
	root := t.TempDir()
	put(t, root, "data/cases/example/brain/canon/note.md", "case")
	if got := Run(root, Options{}); got.State != "committed" {
		t.Fatal(got)
	}
	accounts := filepath.Join(root, "brain/accounts")
	if err := os.Rename(accounts, accounts+".saved"); err != nil {
		t.Fatal(err)
	}
	put(t, root, "brain/accounts", "a regular file replacing a mapped ancestor")
	if got := Run(root, Options{Status: true}); got.State != "blocked" {
		t.Fatal("implicit ancestor type accepted", got)
	}
}

func TestEachOrdinaryCommittedEvolutionIsExplicit(t *testing.T) {
	for _, change := range []string{"edit", "new_file", "delete_file", "delete_directory"} {
		t.Run(change, func(t *testing.T) {
			root := t.TempDir()
			put(t, root, "data/owner/note.md", "original")
			if got := Run(root, Options{}); got.State != "committed" {
				t.Fatal(got)
			}
			switch change {
			case "edit":
				put(t, root, "brain/owner/note.md", "edited")
			case "new_file":
				put(t, root, "brain/new.md", "new")
			case "delete_file":
				if err := os.Remove(filepath.Join(root, "brain/owner/note.md")); err != nil {
					t.Fatal(err)
				}
			case "delete_directory":
				if err := os.Remove(filepath.Join(root, "brain/owner/note.md")); err != nil {
					t.Fatal(err)
				}
				if err := os.Remove(filepath.Join(root, "brain/owner")); err != nil {
					t.Fatal(err)
				}
			}
			got := Run(root, Options{})
			if got.State != "committed" || got.Current || got.Verification != "target_evolved" || len(got.Errors) != 0 {
				t.Fatal(got)
			}
		})
	}
}
