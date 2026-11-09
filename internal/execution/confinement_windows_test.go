//go:build windows

package execution

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

func confinementDirectoryAlias(t *testing.T, alias, target string) {
	t.Helper()
	shell := filepath.Join(os.Getenv("SystemRoot"), "System32", "cmd.exe")
	if out, err := exec.Command(shell, "/c", "mklink", "/J", alias, target).CombinedOutput(); err != nil {
		t.Fatalf("required ordinary-user junction fixture unavailable: %s %v", out, err)
	}
}

func confinementPublicDirectory(t *testing.T, path string) {
	t.Helper()
	descriptor, err := windows.SecurityDescriptorFromString("D:P(A;;FA;;;WD)")
	if err != nil {
		t.Fatal(err)
	}
	acl, _, err := descriptor.DACL()
	if err != nil {
		t.Fatal(err)
	}
	if err = windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, acl, nil); err != nil {
		t.Fatal(err)
	}
}

func TestWindowsConfinedStoreRejectsJunctionAndPrivateHardlink(t *testing.T) {
	t.Run("junction-parent", func(t *testing.T) {
		root, outside := t.TempDir(), t.TempDir()
		marker := filepath.Join(outside, "outside.txt")
		body := []byte("outside synthetic bytes")
		if err := os.WriteFile(marker, body, 0600); err != nil {
			t.Fatal(err)
		}
		store, err := OpenConfinedStore(root)
		if err != nil {
			t.Fatal(err)
		}
		confinementDirectoryAlias(t, filepath.Join(root, "workspaces"), outside)
		if _, err = store.Create(testCreateInput()); err == nil {
			t.Fatal("create traversed junction parent")
		}
		entries, err := os.ReadDir(outside)
		if err != nil || len(entries) != 1 {
			t.Fatalf("outside entries changed: %v %v", entries, err)
		}
		if got, err := os.ReadFile(marker); err != nil || !bytes.Equal(got, body) {
			t.Fatal("outside bytes changed", err)
		}
	})
	t.Run("private-contract-hardlink", func(t *testing.T) {
		store, err := OpenConfinedStore(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		item, err := store.Create(testCreateInput())
		if err != nil {
			t.Fatal(err)
		}
		contract := filepath.Join(store.Root, "workspaces", testWorkspaceID, "execution", "items", item.State.ItemID, "contract.v1.json")
		before, err := os.ReadFile(contract)
		if err != nil {
			t.Fatal(err)
		}
		alias := filepath.Join(t.TempDir(), "outside.json")
		if err = os.Link(contract, alias); err != nil {
			t.Fatal("required native hardlink fixture unavailable", err)
		}
		if _, err = store.Start(testWorkspaceID, item.State.ItemID, 1); err == nil {
			t.Fatal("private ledger accepted multiple links")
		}
		if got, err := os.ReadFile(alias); err != nil || !bytes.Equal(got, before) {
			t.Fatal("outside bytes changed", err)
		}
		if err = os.Remove(alias); err != nil {
			t.Fatal(err)
		}
		observed, err := store.Inspect(testWorkspaceID, item.State.ItemID)
		if err != nil || observed.State != item.State {
			t.Fatalf("failed mutation changed state: %+v %v", observed.State, err)
		}
	})
}

func schemaLeafAlias(t *testing.T, alias, directory string) {
	t.Helper()
	confinementDirectoryAlias(t, alias, directory)
}

func confinementSecuritySnapshot(t *testing.T, path string) string {
	t.Helper()
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	return sd.String()
}
