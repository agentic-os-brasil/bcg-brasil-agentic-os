package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func noEnv(string) (string, bool) { return "", false }

func TestPolicyStatesAndRedaction(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private-user-secret.json")
	body := `{"strictKnownMarketplaces":[],"blockedMarketplaces":[{"source":"directory","path":"/private-user/secret-marketplace"}],"allowManagedHooksOnly":false,"disableSideloadFlags":true,"env":{"DISABLE_AUTOUPDATER":"1","FORCE_AUTOUPDATE_PLUGINS":"secret-in-allowed-name","PLUGIN_API_TOKEN":"secret-token"},"hooks":{"prompt":"secret-prompt"}}`
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	r := collect(path, "darwin", func(key string) (string, bool) {
		switch key {
		case "DISABLE_AUTOUPDATER":
			return "0", true
		case "FORCE_AUTOUPDATE_PLUGINS":
			return "", true
		default:
			t.Fatalf("unexpected environment lookup: %s", key)
			return "", false
		}
	})
	if r.FileState != "read" || r.Settings["strictKnownMarketplaces"].State != "empty_array" || r.Settings["blockedMarketplaces"].State != "nonempty_array" {
		t.Fatalf("list states: %+v", r)
	}
	if v := r.Settings["allowManagedHooksOnly"].Boolean; v == nil || *v {
		t.Fatal("false must remain explicit")
	}
	if v := r.Settings["disableSideloadFlags"].Boolean; v == nil || !*v {
		t.Fatal("true must remain explicit")
	}
	if r.Settings["disableCommandPluginSources"].State != "absent" {
		t.Fatal("missing key must be absent")
	}
	if r.ManagedEnv["DISABLE_AUTOUPDATER"].State != "enabled" || r.ManagedEnv["FORCE_AUTOUPDATE_PLUGINS"].State != "unrecognized_value" {
		t.Fatal("managed env normalization failed")
	}
	if r.ProcessEnv["DISABLE_AUTOUPDATER"].State != "disabled" || r.ProcessEnv["FORCE_AUTOUPDATE_PLUGINS"].State != "empty" {
		t.Fatal("process env normalization failed")
	}
	encoded, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"private-user", "secret", "PLUGIN_API_TOKEN", "prompt", "hooks", path} {
		if strings.Contains(string(encoded), secret) {
			t.Fatalf("receipt leaked %q", secret)
		}
	}
	if r.EffectivePolicy != "uninspected" || r.StandardTier != "unverified" || r.SharedFolderVerdict != "pending_attended_evidence" {
		t.Fatal("file collection must not qualify policy or tier")
	}
}

func TestUnavailableAndInvalidInput(t *testing.T) {
	for _, tc := range []struct{ name, body, fileState string }{
		{"missing", "", "missing"}, {"malformed", `{"env":"secret`, "invalid_json"},
		{"second_document", `{} {"allowManagedHooksOnly":false}`, "invalid_json"},
		{"empty_file", "", "invalid_json"},
		{"null", `null`, "invalid_document"}, {"array", `[]`, "invalid_document"},
		{"oversized", strings.Repeat(" ", maxBytes+1), "too_large"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "settings.json")
			if tc.name != "missing" {
				if err := os.WriteFile(path, []byte(tc.body), 0600); err != nil {
					t.Fatal(err)
				}
			}
			r := collect(path, "darwin", noEnv)
			if r.FileState != tc.fileState {
				t.Fatalf("file state %q, want %q", r.FileState, tc.fileState)
			}
			for key, v := range r.Settings {
				if v.State != "unavailable" {
					t.Fatalf("%s: %+v", key, v)
				}
			}
			for key, v := range r.ManagedEnv {
				if v.State != "unavailable" {
					t.Fatalf("%s: %+v", key, v)
				}
			}
		})
	}
	path := t.TempDir()
	if r := collect(path, "windows", noEnv); r.FileState != "not_regular" {
		t.Fatalf("directory: %s", r.FileState)
	}
	if got := readFailure(os.ErrPermission); got != "unreadable" {
		t.Fatalf("permission: %s", got)
	}
	t.Run("unreadable", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "settings.json")
		if err := os.WriteFile(path, []byte(`{}`), 0000); err != nil {
			t.Fatal(err)
		}
		defer os.Chmod(path, 0600)
		if _, err := os.ReadFile(path); err == nil {
			t.Skip("host can read mode-000 file")
		}
		if r := collect(path, "darwin", noEnv); r.FileState != "unreadable" || r.Settings["allowManagedHooksOnly"].State != "unavailable" {
			t.Fatalf("unreadable file: %+v", r)
		}
	})
	t.Run("symlink", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "settings.json")
		if err := os.Symlink("unavailable-target", path); err != nil {
			t.Skipf("symlink unavailable: %v", err)
		}
		if r := collect(path, "darwin", noEnv); r.FileState != "not_regular" {
			t.Fatalf("symlink: %s", r.FileState)
		}
	})
}

func TestInvalidTypesRemainDistinct(t *testing.T) {
	for _, tc := range []struct{ body, key, state string }{
		{`{}`, "allowManagedHooksOnly", "absent"},
		{`{"allowManagedHooksOnly":null}`, "allowManagedHooksOnly", "null"},
		{`{"allowManagedHooksOnly":"false"}`, "allowManagedHooksOnly", "invalid_type"},
		{`{"strictKnownMarketplaces":false}`, "strictKnownMarketplaces", "invalid_type"},
		{`{"strictKnownMarketplaces":null}`, "strictKnownMarketplaces", "null"},
		{`{"env":false}`, "env", "invalid_type"},
		{`{"env":{"DISABLE_AUTOUPDATER":false}}`, "env", "invalid_type"},
	} {
		path := filepath.Join(t.TempDir(), "settings.json")
		if err := os.WriteFile(path, []byte(tc.body), 0600); err != nil {
			t.Fatal(err)
		}
		r := collect(path, "darwin", noEnv)
		got := r.Settings[tc.key].State
		if tc.key == "env" {
			got = r.ManagedEnv["DISABLE_AUTOUPDATER"].State
		}
		if got != tc.state {
			t.Fatalf("%s: got %s want %s", tc.body, got, tc.state)
		}
	}
}

func TestPathsAndSafeCLI(t *testing.T) {
	for goos, want := range map[string]string{"darwin": "/Library/Application Support/ClaudeCode/managed-settings.json", "windows": `C:\Program Files\ClaudeCode\managed-settings.json`, "linux": "/etc/claude-code/managed-settings.json"} {
		if got := defaultPath(goos); got != want {
			t.Fatalf("%s: %q", goos, got)
		}
	}
	var out, diagnostic bytes.Buffer
	if code := run([]string{"--bad-secret"}, &out, &diagnostic, noEnv); code != 2 || strings.Contains(diagnostic.String(), "secret") || out.Len() != 0 {
		t.Fatal("invalid arguments must fail without echoing values")
	}
}
