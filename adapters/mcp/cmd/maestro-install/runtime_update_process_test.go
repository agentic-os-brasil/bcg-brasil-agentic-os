package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installer"
)

// Controlled in-process native admission, exact ephemeral fixture seed bytes.
// The production command retains strict native verification.
func TestRuntimeCurrentnessManagerProcess(t *testing.T) {
	path := os.Getenv("MAESTRO_RUNTIME_MANAGER_FIXTURE")
	if path == "" {
		t.Skip("explicit synthetic manager lifecycle fixture required")
	}
	var f struct {
		Self, Version, Pin, SeedSHA, ManagedRoot, UserHome string
		Client                                             installer.ClientRequest
	}
	body, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	if e = json.Unmarshal(body, &f); e != nil {
		t.Fatal(e)
	}
	verify := func(_ context.Context, path string) error {
		actual, e := installer.ClientFileDigest(path)
		if e != nil {
			return e
		}
		if actual != f.SeedSHA {
			return errors.New("controlled manager seed identity changed")
		}
		return nil
	}
	o := installer.ClientOptions{SeededOperationOptions: installer.SeededOperationOptions{Self: f.Self, Version: f.Version, AuthorityRegistrySHA256: f.Pin, ManagedRoot: f.ManagedRoot, UserHome: f.UserHome, DataRoot: filepath.Join(f.UserHome, ".maestro-install"), VerifyNative: verify}, Request: f.Client}
	action := os.Getenv("MAESTRO_RUNTIME_MANAGER_ACTION")
	input := strings.Repeat("YES\n", 100)
	switch action {
	case "update":
	case "update-decline-runtime":
		action = "update"
		input = "YES\nYES\n\n"
	case "reconcile-decline-snapshot":
		action = ""
		input = "reconcile\n\n"
	default:
		input = action + "\n" + input
		action = ""
	}
	if e = manageClientsWithIntent(context.Background(), o, strings.NewReader(input), os.Stdout, action); e != nil {
		t.Fatal(e)
	}
}
