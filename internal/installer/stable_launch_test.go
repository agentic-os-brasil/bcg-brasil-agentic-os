package installer

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestStableLaunchUnverifiedSelectionCannotRecoverLock(t *testing.T) {
	root := t.TempDir()
	lock := filepath.Join(root, ".activation.lock")
	body := []byte("999999999\n")
	if err := os.WriteFile(lock, body, 0600); err != nil {
		t.Fatal(err)
	}
	before, err := os.Lstat(lock)
	if err != nil {
		t.Fatal(err)
	}
	o := ClientOptions{SeededOperationOptions: SeededOperationOptions{ManagedRoot: root, UserHome: root, DataRoot: filepath.Join(root, ".maestro-install"), Self: filepath.Join(root, "maestro-install")}, Request: ClientRequest{CarrierID: "unverified", Workspace: filepath.Join(root, "work")}}
	if _, err := PrepareStableLaunch(context.Background(), o); err == nil {
		t.Fatal("unverified selection launched")
	}
	after, statErr := os.Lstat(lock)
	actual, readErr := os.ReadFile(lock)
	if statErr != nil || readErr != nil || !os.SameFile(before, after) || !bytes.Equal(actual, body) {
		t.Fatal("unverified startup changed activation ownership", statErr, readErr)
	}
	if _, err := os.Lstat(filepath.Join(root, ".activation-recovery.guard")); !os.IsNotExist(err) {
		t.Fatal("unverified startup created a recovery guard", err)
	}
}

func TestStableCheckLaunchRetainsUnverifiedSelectionRefusal(t *testing.T) {
	root := t.TempDir()
	o := ClientOptions{SeededOperationOptions: SeededOperationOptions{ManagedRoot: root, UserHome: root, DataRoot: filepath.Join(root, ".maestro-install"), Self: filepath.Join(root, "maestro-install")}, Request: ClientRequest{CarrierID: "unverified", Workspace: filepath.Join(root, "work")}}
	if _, err := PrepareStableCheckLaunch(context.Background(), o); err == nil {
		t.Fatal("unverified checker selection launched")
	}
	if _, err := AdmitStableCheckServer(context.Background(), o); err == nil {
		t.Fatal("options-only checker server admitted")
	}
	for _, capability := range []*VerifiedStableCheck{nil, {}} {
		if err := capability.ValidateContext(context.Background(), StableCheckBinding{}); err == nil {
			t.Fatal("unverified capability validated")
		}
	}
}
