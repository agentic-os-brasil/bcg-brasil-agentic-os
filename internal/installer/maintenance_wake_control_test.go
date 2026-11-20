package installer

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
)

func windowsWakeControlFixture(t *testing.T) MaintenanceWakeBinding {
	t.Helper()
	root := t.TempDir()
	b := MaintenanceWakeBinding{ManagedRoot: filepath.Join(root, "product"), UserHome: filepath.Join(root, "owner"), BrainHome: filepath.Join(root, "owner", "Developer", "Maestro"), Workspace: filepath.Join(root, "workspace"), WorkspaceID: strings.Repeat("a", 32), CarrierID: strings.Repeat("b", 32), AdmissionSHA256: strings.Repeat("c", 64), RegistrySHA256: strings.Repeat("d", 64), InstallerVersion: "0.1.59", SourceLocator: filepath.Join(root, "source"), Source: releasecontract.SourceTuple{SourceID: "test-source", Channel: "beta", OS: "windows", Arch: "amd64"}}
	if err := os.MkdirAll(filepath.Join(b.UserHome, ".maestro-install"), 0700); err != nil {
		t.Fatal(err)
	}
	return b
}
func TestWindowsWakeControlRequiresDurableEnabledRevision(t *testing.T) {
	b := windowsWakeControlFixture(t)
	if _, err := b.ReadWindowsWakeControl(); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("fresh control must be absent", err)
	}
	if _, err := b.RequireWindowsWakeControl(context.Background()); err == nil {
		t.Fatal("missing control enabled worker")
	}
	c, err := b.WriteWindowsWakeControl(context.Background(), 0, false)
	if err != nil || c.Revision != 1 || c.Enabled {
		t.Fatal("disabled bootstrap failed", c, err)
	}
	if _, err := b.RequireWindowsWakeControl(context.Background()); err == nil {
		t.Fatal("disabled control enabled worker")
	}
	c, err = b.WriteWindowsWakeControl(context.Background(), 1, true)
	if err != nil || c.Revision != 2 || !c.Enabled {
		t.Fatal("successful acknowledgement cannot open control", c, err)
	}
	if _, err := b.RequireWindowsWakeControl(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := b.WriteWindowsWakeControl(context.Background(), 1, false); err == nil {
		t.Fatal("stale revision changed native definition")
	}
	c, err = b.ReadWindowsWakeControl()
	if err != nil || c.Revision != 2 || !c.Enabled {
		t.Fatal("stale action changed control", c, err)
	}
}
func TestWindowsWakeControlCanceledWritePreservesAbsence(t *testing.T) {
	b := windowsWakeControlFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := b.WriteWindowsWakeControl(ctx, 0, false); !errors.Is(err, context.Canceled) {
		t.Fatal("canceled control write admitted", err)
	}
	if _, err := b.ReadWindowsWakeControl(); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("canceled write created control", err)
	}
}

func TestWindowsWakeControlPreservesUnsafeOrForeignExistingState(t *testing.T) {
	for _, kind := range []string{"malformed", "foreign", "null", "extra", "duplicate", "hardlink", "symlink", "ancestor", "mode", "directory"} {
		t.Run(kind, func(t *testing.T) {
			b := windowsWakeControlFixture(t)
			if _, err := b.WriteWindowsWakeControl(context.Background(), 0, false); err != nil {
				t.Fatal(err)
			}
			path, _, err := b.windowsWakeControlPath()
			if err != nil {
				t.Fatal(err)
			}
			inspect := path
			switch kind {
			case "malformed":
				if err := os.WriteFile(path, []byte("{"), 0600); err != nil {
					t.Fatal(err)
				}
			case "foreign":
				body, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				digest, _ := b.Digest()
				if err := os.WriteFile(path, []byte(strings.Replace(string(body), digest, strings.Repeat("f", 64), 1)), 0600); err != nil {
					t.Fatal(err)
				}
			case "null":
				body, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(strings.Replace(string(body), `"enabled":false`, `"enabled":null`, 1)), 0600); err != nil {
					t.Fatal(err)
				}
			case "extra":
				body, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				body = append(body[:len(body)-1], []byte(`,"other":true}`)...)
				if err := os.WriteFile(path, body, 0600); err != nil {
					t.Fatal(err)
				}
			case "duplicate":
				body, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				body = append(body[:len(body)-1], []byte(`,"enabled":false}`)...)
				if err := os.WriteFile(path, body, 0600); err != nil {
					t.Fatal(err)
				}
			case "hardlink":
				if err := os.Link(path, path+".other"); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				inspect = path + ".saved"
				if err := os.Rename(path, inspect); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(inspect, path); err != nil {
					t.Fatal(err)
				}
			case "ancestor":
				dir := filepath.Dir(path)
				other := dir + ".saved"
				if err := os.Rename(dir, other); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(other, dir); err != nil {
					t.Fatal(err)
				}
				inspect = filepath.Join(other, filepath.Base(path))
			case "mode":
				if err := os.Chmod(path, 0644); err != nil {
					t.Fatal(err)
				}
			case "directory":
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(path, 0700); err != nil {
					t.Fatal(err)
				}
			}
			before, err := os.Lstat(inspect)
			if err != nil {
				t.Fatal(err)
			}
			var body []byte
			if before.Mode().IsRegular() {
				body, err = os.ReadFile(inspect)
				if err != nil {
					t.Fatal(err)
				}
			}
			if _, err := b.ReadWindowsWakeControl(); err == nil {
				t.Fatal("unsafe existing definition read as valid")
			}
			if _, err := b.WriteWindowsWakeControl(context.Background(), 1, true); err == nil {
				t.Fatal("unsafe existing definition repaired or replaced")
			}
			after, err := os.Lstat(inspect)
			if err != nil || !os.SameFile(before, after) || before.Mode() != after.Mode() {
				t.Fatal("unsafe path identity/mode changed", err)
			}
			if before.Mode().IsRegular() {
				actual, err := os.ReadFile(inspect)
				if err != nil || string(actual) != string(body) {
					t.Fatal("unsafe bytes changed", err)
				}
			}
		})
	}
}
