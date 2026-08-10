package setupauth

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestOneConfirmationAuthorizesIdempotentLocalSetup(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	store := Store{Root: root, Clock: func() time.Time { return now }}
	identity := DeriveIdentity("corp\\owner", "device-01")
	request := Request{WorkspaceID: strings.Repeat("a", 32), WorkspacePath: filepath.Join(root, "workspace"), SourceFingerprint: strings.Repeat("b", 64)}

	if status, err := store.Status(request, identity); err != nil || status.State != StateAuthorizationRequired {
		t.Fatalf("initial status = %#v, err = %v", status, err)
	}
	grant, err := store.Authorize(request, identity, true)
	if err != nil || grant.State != StateActive || len(grant.AllowedActions) == 0 {
		t.Fatalf("grant = %#v, err = %v", grant, err)
	}
	resumed, err := store.Status(request, identity)
	if err != nil || resumed.State != StateActive || resumed.GrantDigest != grant.GrantDigest {
		t.Fatalf("resumed = %#v, err = %v", resumed, err)
	}
	second, err := store.Authorize(request, identity, false)
	if err != nil || second.GrantDigest != grant.GrantDigest {
		t.Fatalf("idempotent authorize = %#v, err = %v", second, err)
	}

	body, err := os.ReadFile(store.path(request.WorkspaceID))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "corp\\owner") || strings.Contains(string(body), "device-01") || strings.Contains(string(body), "https://") {
		t.Fatalf("grant leaked raw identity or source content: %s", body)
	}
	var strict map[string]any
	if err := json.Unmarshal(body, &strict); err != nil {
		t.Fatal(err)
	}
}

func TestSetupAuthorizationRejectsChangedBindingAndExpiry(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	store := Store{Root: root, Clock: func() time.Time { return now }}
	request := Request{WorkspaceID: strings.Repeat("a", 32), WorkspacePath: filepath.Join(root, "workspace"), SourceFingerprint: strings.Repeat("b", 64)}
	identity := DeriveIdentity("owner", "device")
	if _, err := store.Authorize(request, identity, true); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		req  Request
		id   Identity
		want string
	}{
		{name: "source", req: Request{WorkspaceID: request.WorkspaceID, WorkspacePath: request.WorkspacePath, SourceFingerprint: strings.Repeat("c", 64)}, id: identity, want: StateScopeChanged},
		{name: "principal", req: request, id: DeriveIdentity("another", "device"), want: StateIdentityChanged},
		{name: "device", req: request, id: DeriveIdentity("owner", "another-device"), want: StateIdentityChanged},
		{name: "workspace", req: Request{WorkspaceID: strings.Repeat("c", 32), WorkspacePath: filepath.Join(root, "other")}, id: identity, want: StateAuthorizationRequired},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			status, err := store.Status(test.req, test.id)
			if err != nil || status.State != test.want {
				t.Fatalf("status = %#v, err = %v", status, err)
			}
		})
	}

	now = now.Add(DefaultValidity + time.Second)
	if status, err := store.Status(request, identity); err != nil || status.State != StateExpired {
		t.Fatalf("expired status = %#v, err = %v", status, err)
	}
}

func TestSelectedSourceCanOnlyNarrowAnExistingGrant(t *testing.T) {
	root := t.TempDir()
	store := Store{Root: root}
	identity := DeriveIdentity("owner", "device")
	request := Request{WorkspaceID: strings.Repeat("a", 32), WorkspacePath: filepath.Join(root, "workspace"), SourceFingerprint: strings.Repeat("b", 64)}
	if _, err := store.BindSelectedSource(request, identity); !errors.Is(err, ErrAuthorizationRequired) {
		t.Fatalf("source selection created authority: %v", err)
	}
	base := request
	base.SourceFingerprint = ""
	if _, err := store.Authorize(base, identity, true); err != nil {
		t.Fatal(err)
	}
	if status, err := store.BindSelectedSource(request, identity); err != nil || status.State != StateActive {
		t.Fatalf("bound source = %#v, err = %v", status, err)
	}
	changed := request
	changed.SourceFingerprint = strings.Repeat("c", 64)
	if _, err := store.BindSelectedSource(changed, identity); err == nil {
		t.Fatal("changed source scope reused the existing grant")
	}
}

func TestSetupAuthorizationRefusesSymlinkedStore(t *testing.T) {
	root := t.TempDir()
	out := t.TempDir()
	if err := os.Symlink(out, filepath.Join(root, "setup-authorizations")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	store := Store{Root: root}
	request := Request{WorkspaceID: strings.Repeat("a", 32), WorkspacePath: filepath.Join(root, "workspace")}
	if _, err := store.Authorize(request, DeriveIdentity("owner", "device"), true); err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("symlinked store accepted: %v", err)
	}
}
