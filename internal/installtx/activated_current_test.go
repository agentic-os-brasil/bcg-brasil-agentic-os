package installtx

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitialCurrentGuardRefusesLegacyMissingPinsAndExistingLock(t *testing.T) {
	root := t.TempDir()
	managed := filepath.Join(root, "managed")
	data := filepath.Join(root, "data")
	for _, p := range []string{managed, filepath.Join(data, "config")} {
		if e := os.MkdirAll(p, 0700); e != nil {
			t.Fatal(e)
		}
	}
	body := []byte(`{"schema_version":1,"release":"0.1.0","channel":"canary","cli_version":"0.1.0","bundle_version":"0.1.0","target_os":"darwin","target_arch":"arm64","activated_at":"2026-11-03T00:00:00Z"}`)
	statePath := filepath.Join(data, "config", "install-state.json")
	if e := os.WriteFile(statePath, body, 0600); e != nil {
		t.Fatal(e)
	}
	expected := ReleaseBinding{ManifestSHA256: strings.Repeat("a", 64), ClientInputsSHA256: strings.Repeat("b", 64)}
	for name, binding := range map[string]ReleaseBinding{"missing pins": {}, "partial pins": {ManifestSHA256: expected.ManifestSHA256}, "legacy": expected} {
		t.Run(name, func(t *testing.T) {
			if _, e := ValidateActivatedCurrent(managed, data, binding); e == nil {
				t.Fatal("initial Current reader admitted unsupported state")
			}
			current, e := os.ReadFile(statePath)
			if e != nil || !bytes.Equal(current, body) {
				t.Fatal("read-only admission migrated legacy state", e)
			}
			if _, e = os.Lstat(filepath.Join(managed, ".activation.lock")); !os.IsNotExist(e) {
				t.Fatal("read-only admission created lock", e)
			}
		})
	}
	lock := filepath.Join(managed, ".activation.lock")
	lockBody := []byte("999999999\n")
	if e := os.WriteFile(lock, lockBody, 0600); e != nil {
		t.Fatal(e)
	}
	called := false
	if e := GuardActivatedCurrent(managed, data, expected, func() error { called = true; return nil }); e == nil || called {
		t.Fatal("existing lock was reclaimed or publication admitted")
	}
	actual, e := os.ReadFile(lock)
	if e != nil || !bytes.Equal(actual, lockBody) {
		t.Fatal("initial Current guard changed existing lock", e)
	}
	current, e := os.ReadFile(statePath)
	if e != nil || !bytes.Equal(current, body) {
		t.Fatal("guard changed legacy state", e)
	}
}
