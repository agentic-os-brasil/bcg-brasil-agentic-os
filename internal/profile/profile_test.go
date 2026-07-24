package profile_test

import (
	"os"
	"path/filepath"
	"testing"

	baseprofile "github.com/agentic-os-brasil/bcg-brasil-agentic-os/bundles/base/profile"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/profile"
)

func TestStoreDefaultsAndPersistsAnExplicitProfile(t *testing.T) {
	policy, err := baseprofile.Policy()
	if err != nil {
		t.Fatal(err)
	}
	store := profile.Store{Root: t.TempDir(), Policy: policy}

	state, err := store.Get()
	if err != nil {
		t.Fatal(err)
	}
	if state.Profile != "standard" || state.Source != "default" {
		t.Fatalf("default state = %#v", state)
	}

	state, err = store.Set("power")
	if err != nil {
		t.Fatal(err)
	}
	if state.Profile != "power" || state.Source != "configured" {
		t.Fatalf("set state = %#v", state)
	}

	reloaded, err := store.Get()
	if err != nil {
		t.Fatal(err)
	}
	if reloaded != state {
		t.Fatalf("reloaded state = %#v, want %#v", reloaded, state)
	}
}

func TestStoreFallsBackSafelyFromInvalidConfiguration(t *testing.T) {
	policy, err := baseprofile.Policy()
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "config"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "config", "interaction-profile.json"), []byte(`{"schema_version":1,"profile":"unknown"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	state, err := (profile.Store{Root: root, Policy: policy}).Get()
	if err != nil {
		t.Fatal(err)
	}
	if state.Profile != "standard" || state.Source != "fallback" || state.Warning == "" {
		t.Fatalf("invalid configuration state = %#v", state)
	}
}

func TestStoreRejectsUnknownProfileWithoutPersistingIt(t *testing.T) {
	policy, err := baseprofile.Policy()
	if err != nil {
		t.Fatal(err)
	}
	store := profile.Store{Root: t.TempDir(), Policy: policy}
	if _, err := store.Set("expert"); err == nil {
		t.Fatal("expected invalid profile to fail")
	}
	state, err := store.Get()
	if err != nil {
		t.Fatal(err)
	}
	if state.Profile != "standard" || state.Source != "default" {
		t.Fatalf("state after rejected set = %#v", state)
	}
}
