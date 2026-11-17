package packaging

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/lifecycle"
)

func nativeSyncedFixture(t *testing.T) (NativeOptions, string, string) {
	t.Helper()
	in := nativeFixture(t)
	in.Executable = controlledInspectionHost(t, in, 0)
	profile := os.Getenv("MAESTRO_CONTROLLED_PROFILE")
	synced := filepath.Join(profile, "plugins", "synced")
	manifest := filepath.Join(synced, "inactive-plugin", ".claude-plugin", "plugin.json")
	if err := os.MkdirAll(filepath.Dir(manifest), 0700); err != nil {
		t.Fatal(err)
	}
	// An inactive body is neither parsed nor admitted as an executable source.
	if err := os.WriteFile(manifest, []byte(strings.Repeat("invalid-inactive-body", 15000)), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(synced, ".sync-state.json"), []byte("opaque cache metadata"), 0600); err != nil {
		t.Fatal(err)
	}
	return in, profile, synced
}

func TestNativeReviewAdmitsStoredDisabledSyncedCache(t *testing.T) {
	in, profile, _ := nativeSyncedFixture(t)
	if err := os.WriteFile(filepath.Join(profile, "settings.json"), []byte(`{"autoMemoryEnabled":false,"syncClaudeAiPlugins":false}`), 0600); err != nil {
		t.Fatal(err)
	}
	review, err := ReviewNative(context.Background(), in)
	if err != nil || review.Plan.State != "reviewable" || review.Plan.SessionSettingsOverride != "" {
		t.Fatalf("stored opt-out did not admit inactive cache without an override: state=%s reason=%s error=%v", review.Plan.State, review.Plan.Reason, err)
	}
	assertNoNativeEffects(t, in)
}

func TestNativeReviewStoredSyncOptOutSources(t *testing.T) {
	in, profile, _ := nativeSyncedFixture(t)
	ws := in.Packet.Binding.WorkspacePath
	user := filepath.Join(profile, "settings.json")
	shared := filepath.Join(ws, ".claude", "settings.json")
	local := filepath.Join(ws, ".claude", "settings.local.json")
	inactive := filepath.Join(profile, "settings.local.json")
	for _, tc := range []struct{ name, user, shared, local, inactive, state, reason string }{
		{name: "absent", user: `{"autoMemoryEnabled":false}`, state: "unavailable", reason: "native_instruction_synced_plugins_unsupported"},
		{name: "true", user: `{"autoMemoryEnabled":false,"syncClaudeAiPlugins":true}`, state: "unavailable", reason: "native_instruction_synced_plugins_unsupported"},
		{name: "shared_false", user: `{"autoMemoryEnabled":false}`, shared: `{"syncClaudeAiPlugins":false}`, state: "unavailable", reason: "native_instruction_synced_plugins_unsupported"},
		{name: "inactive_user_local", user: `{"autoMemoryEnabled":false}`, inactive: `{"syncClaudeAiPlugins":false}`, state: "unavailable", reason: "native_instruction_synced_plugins_unsupported"},
		{name: "local_false_wins", user: `{"autoMemoryEnabled":false,"syncClaudeAiPlugins":true}`, shared: `{"syncClaudeAiPlugins":true}`, local: `{"syncClaudeAiPlugins":false}`, state: "reviewable"},
		{name: "user_false_wins", user: `{"autoMemoryEnabled":false,"syncClaudeAiPlugins":false}`, local: `{"syncClaudeAiPlugins":true}`, state: "reviewable"},
		{name: "null", user: `{"autoMemoryEnabled":false,"syncClaudeAiPlugins":null}`, state: "unavailable", reason: "native_instruction_settings_schema_unsupported"},
		{name: "string", user: `{"autoMemoryEnabled":false,"syncClaudeAiPlugins":"false"}`, state: "unavailable", reason: "native_instruction_settings_schema_unsupported"},
		{name: "malformed_with_false", user: `{"autoMemoryEnabled":false,"syncClaudeAiPlugins":false`, state: "unavailable", reason: "native_instruction_settings_schema_unsupported"},
		{name: "invalid_shared_with_user_false", user: `{"autoMemoryEnabled":false,"syncClaudeAiPlugins":false}`, shared: `{"syncClaudeAiPlugins":[]}`, state: "unavailable", reason: "native_instruction_settings_schema_unsupported"},
		{name: "other_enabled_plugin", user: `{"autoMemoryEnabled":false,"syncClaudeAiPlugins":false,"enabledPlugins":{"external@fixture":true}}`, state: "unavailable", reason: "native_instruction_plugins_unsupported"},
		{name: "external_hooks", user: `{"autoMemoryEnabled":false,"syncClaudeAiPlugins":false,"hooks":{"PreToolUse":[{"hooks":[{"type":"command","command":"unclosed"}]}]}}`, state: "unavailable", reason: "native_instruction_hooks_unsupported"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for path, body := range map[string]string{user: tc.user, shared: tc.shared, local: tc.local, inactive: tc.inactive} {
				if body == "" {
					if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
						t.Fatal(err)
					}
					continue
				}
				if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(body), 0600); err != nil {
					t.Fatal(err)
				}
			}
			review, err := ReviewNative(context.Background(), in)
			if err != nil || review.Plan.State != tc.state || review.Plan.Reason != tc.reason || review.Plan.SessionSettingsOverride != "" {
				t.Fatalf("stored source admission: state=%s reason=%s error=%v", review.Plan.State, review.Plan.Reason, err)
			}
			assertNoNativeEffects(t, in)
		})
	}
}

func TestNativeStoredSyncOptOutManagedScope(t *testing.T) {
	// Exercise managed-file semantics without modifying the machine's policy directory.
	for _, tc := range []struct {
		managed, user string
		disabled      bool
	}{
		{`{"syncClaudeAiPlugins":false}`, `{"syncClaudeAiPlugins":true}`, true},
		{`{"syncClaudeAiPlugins":true}`, `{"syncClaudeAiPlugins":false}`, true},
		{`{"syncClaudeAiPlugins":true}`, `{}`, false},
	} {
		s := newInstructionSources()
		root := t.TempDir()
		for name, body := range map[string]string{"managed-settings.json": tc.managed, "settings.json": tc.user} {
			if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0600); err != nil {
				t.Fatal(err)
			}
		}
		if err := s.root(root, map[string]nativeSettingsScope{"managed-settings.json": nativeSettingsManaged, "settings.json": nativeSettingsUser}); err != nil {
			t.Fatal(err)
		}
		if err := s.file(filepath.Join(root, "managed-settings.json"), false, 0); err != nil {
			t.Fatal(err)
		}
		if s.syncDisabled != tc.disabled {
			t.Fatalf("restrictive stored false: got %t want %t", s.syncDisabled, tc.disabled)
		}
	}
}

func TestNativeStoredSyncOptOutBindsSettingsAndCacheMembership(t *testing.T) {
	for _, mutation := range []string{"true", "scope", "membership"} {
		t.Run(mutation, func(t *testing.T) {
			in, profile, synced := nativeSyncedFixture(t)
			user := filepath.Join(profile, "settings.json")
			if err := os.WriteFile(user, []byte(`{"autoMemoryEnabled":false,"syncClaudeAiPlugins":false}`), 0600); err != nil {
				t.Fatal(err)
			}
			review, err := ReviewNative(context.Background(), in)
			if err != nil || review.Plan.State != "reviewable" {
				t.Fatalf("initial review: %+v %v", review.Plan, err)
			}
			if mutation == "membership" {
				if err := os.WriteFile(filepath.Join(synced, "new-cache-entry"), []byte("opaque"), 0600); err != nil {
					t.Fatal(err)
				}
			} else {
				body := `{"autoMemoryEnabled":false,"syncClaudeAiPlugins":true}`
				if mutation == "scope" {
					body = `{"autoMemoryEnabled":false}`
					directory := filepath.Join(in.Packet.Binding.WorkspacePath, ".claude")
					if err := os.MkdirAll(directory, 0700); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(filepath.Join(directory, "settings.json"), []byte(`{"syncClaudeAiPlugins":false}`), 0600); err != nil {
						t.Fatal(err)
					}
				}
				if err := os.WriteFile(user, []byte(body), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := QualifyAndBuildNative(context.Background(), in, review.ReviewDigest, true); err == nil || !strings.Contains(err.Error(), "review changed") {
				t.Fatalf("changed source consumed review: %v", err)
			}
			assertNoNativeEffects(t, in)
		})
	}
}

func TestNativeStoredSyncOptOutPreservesDiscoveryRefusals(t *testing.T) {
	for _, kind := range []string{"root_alias", "entry_alias", "membership_overflow", "skills_plugin"} {
		t.Run(kind, func(t *testing.T) {
			in, profile, synced := nativeSyncedFixture(t)
			if err := os.WriteFile(filepath.Join(profile, "settings.json"), []byte(`{"autoMemoryEnabled":false,"syncClaudeAiPlugins":false}`), 0600); err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "root_alias":
				target := synced + "-real"
				if err := os.Rename(synced, target); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, synced); err != nil {
					t.Fatal(err)
				}
			case "entry_alias":
				if err := os.Symlink(in.Packet.Binding.WorkspacePath, filepath.Join(synced, "alias")); err != nil {
					t.Fatal(err)
				}
			case "membership_overflow":
				for i := 0; i < 255; i++ {
					if err := os.WriteFile(filepath.Join(synced, fmt.Sprintf("entry-%d", i)), nil, 0600); err != nil {
						t.Fatal(err)
					}
				}
			case "skills_plugin":
				directory := filepath.Join(profile, "skills", "external", ".claude-plugin")
				if err := os.MkdirAll(directory, 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(directory, "plugin.json"), []byte(`{}`), 0600); err != nil {
					t.Fatal(err)
				}
			}
			review, err := ReviewNative(context.Background(), in)
			if err != nil || review.Plan.State != "unavailable" || !strings.HasPrefix(review.Plan.Reason, "native_instruction_") {
				t.Fatalf("unsafe discovery admitted: %+v %v", review.Plan, err)
			}
			assertNoNativeEffects(t, in)
		})
	}
}

func TestNativeStoredSyncOptOutRequiresSupportedHost(t *testing.T) {
	in, profile, _ := nativeSyncedFixture(t)
	if err := os.WriteFile(filepath.Join(profile, "settings.json"), []byte(`{"autoMemoryEnabled":false,"syncClaudeAiPlugins":false}`), 0600); err != nil {
		t.Fatal(err)
	}
	// Keep the enforcement minimum at least as restrictive as the stored setting's support.
	if !lifecycle.MeetsMinimum(EnforcementMinimumVersion, "2.1.273") {
		t.Fatal("synced-cache admission needs an inspected-version gate below 2.1.273")
	}
	for _, version := range []string{"2.1.272", "invalid"} {
		t.Setenv("MAESTRO_CONTROLLED_HOST_VERSION", version)
		review, err := ReviewNative(context.Background(), in)
		if err != nil || review.Plan.State != "unavailable" || review.Plan.Reason != "native_enforcement_version_unsupported" {
			t.Fatalf("unsupported host admitted: %+v %v", review.Plan, err)
		}
		assertNoNativeEffects(t, in)
	}
}

func TestNativeStoredSyncOptOutAtGitRootLocalScope(t *testing.T) {
	in, profile, _ := nativeSyncedFixture(t)
	project := filepath.Dir(in.Packet.Binding.WorkspacePath)
	makeNativeGitFixture(t, project)
	directory := filepath.Join(project, ".claude")
	if err := os.Mkdir(directory, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "settings.local.json"), []byte(`{"syncClaudeAiPlugins":false}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(profile, "settings.json"), []byte(`{"autoMemoryEnabled":false,"syncClaudeAiPlugins":true}`), 0600); err != nil {
		t.Fatal(err)
	}
	review, err := ReviewNative(context.Background(), in)
	if err != nil || review.Plan.State != "reviewable" || review.Plan.SessionSettingsOverride != "" {
		t.Fatalf("applicable Git-root local false was ignored: %+v %v", review.Plan, err)
	}
	assertNoNativeEffects(t, in)
}
