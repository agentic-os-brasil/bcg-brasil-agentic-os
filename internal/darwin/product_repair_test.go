package darwin

import (
	"bytes"
	"errors"
	"path/filepath"
	"testing"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/maintenance"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

func TestProductRepairExactBytesAndAbsenceCompensation(t *testing.T) {
	for _, previous := range [][]byte{nil, []byte("{\"old\":true}")} {
		t.Run(string(previous), func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "darwin")
			path := filepath.Join(root, managedStateRelativePath)
			if e := scheduler.EnsurePrivateDirectory(filepath.Dir(path)); e != nil {
				t.Fatal(e)
			}
			if previous != nil {
				if e := scheduler.PublishNewPrivateFile(path, previous); e != nil {
					t.Fatal(e)
				}
			}
			refused := errors.New("post-validation refused")
			e := (ManagedStateRepairInvoker{Root: root}).refreshProduct(previous, []byte("{\"new\":true}"), func(publish func() error) error { return publish() }, func() error { return refused })
			if previous == nil {
				if !errors.Is(e, refused) || !errors.Is(e, ErrProductRecovery) {
					t.Fatalf("failed readback must retain recovery: %v", e)
				}
			} else if !errors.Is(e, maintenance.ErrDarwinRepairUnavailable) {
				t.Fatalf("existing cache replacement must be unavailable: %v", e)
			}
			actual, e := scheduler.ReadPrivateStateFile(path, productCacheLimit)
			if previous == nil {
				if e != nil || string(actual) != "{\"new\":true}" {
					t.Fatalf("partial exclusive creation was removed: %q %v", actual, e)
				}
			} else if e != nil || !bytes.Equal(actual, previous) {
				t.Fatalf("prior bytes not restored: %q %v", actual, e)
			}
		})
	}
}
func TestProductRepairFailedCompensationRetainsRecovery(t *testing.T) {
	root := filepath.Join(t.TempDir(), "darwin")
	path := filepath.Join(root, managedStateRelativePath)
	if e := scheduler.EnsurePrivateDirectory(filepath.Dir(path)); e != nil {
		t.Fatal(e)
	}
	count := 0
	guard := func(publish func() error) error {
		count++
		if count > 1 {
			return errors.New("revoked")
		}
		return publish()
	}
	e := (ManagedStateRepairInvoker{Root: root}).refreshProduct(nil, []byte("{\"new\":true}"), guard, func() error { return errors.New("readback rejected") })
	if !errors.Is(e, ErrProductRecovery) {
		t.Fatalf("failed compensation hidden: %v", e)
	}
	actual, e := scheduler.ReadPrivateStateFile(path, productCacheLimit)
	if e != nil || string(actual) != "{\"new\":true}" {
		t.Fatalf("known applied effect lost: %q %v", actual, e)
	}
}
