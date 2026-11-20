package setup

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installer"
)

// This freezes the public host's copy-version protocol independently of the
// production observer. A directory lease is host metadata, not plugin content.
func TestCodeCurrentCommandCopyReadbackWithoutFolderFields(t *testing.T) {
	r, original, effects := commandConsentFixture(t)
	payload := filepath.Join(r.Materialization.CodeRoot, "plugins", "maestro-product")
	manifest := []byte(`{"name":"maestro-product","version":"0.1.55"}`)
	if err := os.Mkdir(filepath.Join(payload, ".claude-plugin"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(payload, ".claude-plugin", "plugin.json"), manifest, 0600); err != nil {
		t.Fatal(err)
	}
	r.Materialization.CodeTreeSHA256, _ = installer.ClientTreeDigest(r.Materialization.CodeRoot)
	const content = "fccc98d23b4738483721fac2d915d792b11c9c17540179914edc46b3bd3240a3"
	version := r.Materialization.Release + "-" + content[:12]
	cache := filepath.Join(r.Code.ConfigurationDirectory, "plugins", "cache", r.Code.Marketplace, "maestro-product", version)
	if err := os.MkdirAll(filepath.Join(cache, ".claude-plugin"), 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"method.md", ".claude-plugin/plugin.json"} {
		body, err := os.ReadFile(filepath.Join(payload, filepath.FromSlash(name)))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(cache, filepath.FromSlash(name)), body, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(cache, ".in_use"), 0700); err != nil {
		t.Fatal(err)
	}
	row := map[string]any{"id": r.Code.PluginID, "version": version, "scope": "user", "enabled": true, "installPath": cache, "projectEnabled": false}
	run := func(ctx context.Context, path string, args []string, cwd string) ([]byte, error) {
		if reflect.DeepEqual(args, []string{"plugin", "list", "--json"}) {
			return json.Marshal([]any{row})
		}
		return original(ctx, path, args, cwd)
	}
	review, err := ReviewCodeRegistration(context.Background(), r, run)
	if err != nil || review.Observation.State != "registered" || review.Observation.InstalledVersion != version || len(review.Actions) != 0 || *effects != 0 {
		t.Fatalf("actual command copy shape was not independently verified: %+v %v", review, err)
	}
	if review.Observation.CopyManifestVersion != r.Materialization.Release || review.Observation.CopyContentSHA256 != content {
		t.Fatal("copy proof lost raw and manifest version distinction")
	}
	if _, err := ApplyCodeRegistration(context.Background(), r, run, review.Digest, true); err != nil || *effects != 0 {
		t.Fatalf("already installed native copy did not reconcile read-only: %v", err)
	}
	for _, marker := range []struct {
		name  string
		body  string
		valid bool
	}{
		{"optional-fields-absent", `{"pid":123}`, true},
		{"procStart-string", `{"pid":123,"procStart":"synthetic process epoch"}`, true},
		{"procStartFt-string", `{"pid":123,"procStartFt":"synthetic process epoch"}`, true},
		{"procStart-null", `{"pid":123,"procStart":null}`, false},
		{"procStartFt-null", `{"pid":123,"procStartFt":null}`, false},
	} {
		t.Run(marker.name, func(t *testing.T) {
			if err := os.WriteFile(filepath.Join(cache, ".in_use", "123"), []byte(marker.body), 0600); err != nil {
				t.Fatal(err)
			}
			_, err := ReviewCodeRegistration(context.Background(), r, run)
			if (err == nil) != marker.valid {
				t.Fatalf("typed host lease acceptance = %v, want %v: %v", err == nil, marker.valid, err)
			}
		})
	}
	if err := os.Remove(filepath.Join(cache, ".in_use", "123")); err != nil {
		t.Fatal(err)
	}
	row["installPath"] = filepath.Join(r.Binding.Workspace, "foreign-cache")
	if _, err := ReviewCodeRegistration(context.Background(), r, run); err == nil {
		t.Fatal("foreign cache path accepted")
	}
	row["installPath"] = cache
	if err := os.WriteFile(filepath.Join(cache, "method.md"), []byte("foreign edit\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReviewCodeRegistration(context.Background(), r, run); err == nil {
		t.Fatal("changed native content accepted")
	}
	if err := os.WriteFile(filepath.Join(cache, "method.md"), []byte("canonical current method\n"), 0600); err != nil {
		t.Fatal(err)
	}
	row["version"] = r.Materialization.Release + "-000000000000"
	if _, err := ReviewCodeRegistration(context.Background(), r, run); err == nil {
		t.Fatal("unverified host suffix accepted")
	}
	row["version"] = version
	if err := os.WriteFile(filepath.Join(cache, ".in_use", "unexpected.md"), []byte("foreign payload"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReviewCodeRegistration(context.Background(), r, run); err == nil {
		t.Fatal("arbitrary lease subtree excluded from copy verification")
	}
}

func TestCodeCommandConsentRejectsForeignScriptEvenWithRehashedRecord(t *testing.T) {
	r, run, effects := commandConsentFixture(t)
	review, err := ReviewCodeRegistration(context.Background(), r, run)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(review.CommandSourceConsent.LauncherPath, []byte("foreign script\n"), 0700); err != nil {
		t.Fatal(err)
	}
	r.Materialization.CodeProviderSHA256, err = installer.ClientTreeDigest(r.Materialization.CodeProviderRoot)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ReviewCodeRegistration(context.Background(), r, run); err == nil || *effects != 0 {
		t.Fatal("provider digest was substituted for the original producer contract")
	}
}

func TestCodeWindowsSourcePreservesUnicodeArgumentsForWindowsPowerShell(t *testing.T) {
	o := installer.ClientOptions{SeededOperationOptions: installer.SeededOperationOptions{Self: `C:\synthetic\Developer\maestro-install.exe`, ManagedRoot: `C:\synthetic\Developer`, UserHome: `C:\synthetic`}, Request: installer.ClientRequest{CodeRoot: `C:\synthetic\Developer\Code`, CarrierID: strings.Repeat("a", 32), Workspace: `C:\synthetic\Developer\Cérebro d'owner`}}
	_, files, name, err := commandSourceFiles(o, "windows")
	if err != nil {
		t.Fatal(err)
	}
	// Windows PowerShell 5.1 decodes -File as the ANSI code page without a BOM.
	if !strings.HasPrefix(string(files[name]), "\xef\xbb\xbf") {
		t.Fatal("Unicode provider arguments are not explicitly UTF-8 for Windows PowerShell")
	}
	if !strings.Contains(string(files[name]), "Cérebro d''owner") {
		t.Fatal("PowerShell argument quote/byte preservation failed")
	}
}

func TestCodeCopyContentProtocolPreservesUTF16OrderAndExactBytes(t *testing.T) {
	root := t.TempDir()
	for name, body := range map[string][]byte{"a/z": []byte("line\r\n"), "a0": {}, "\ue000.txt": []byte("bmp"), "\U00010000.txt": []byte("pair")} {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, body, 0600); err != nil {
			t.Fatal(err)
		}
	}
	_, digest, err := codeCopyInventory(root, false)
	want := "7e9d6a04f37ff594682a59df3f83dd6e4ddae9df7c29d600a5a451a0b91d4c9c"
	if runtime.GOOS == "windows" {
		want = "37378962ecc6ba21ab0d8722702ca18b7328dc1da1fff69dd7e266abe3ff4e40"
	}
	if err != nil || digest != want {
		t.Fatalf("public content protocol changed UTF16/platform ordering, empty-file or CRLF bytes: %s %v", digest, err)
	}
}

func TestCodeCommandSourceRegistrationUsesOnlyReviewedNarrowConsent(t *testing.T) {
	r, run, effects := commandConsentFixture(t)
	review, err := ReviewCodeRegistration(context.Background(), r, run)
	if err != nil {
		t.Fatal(err)
	}
	if *effects != 0 {
		t.Fatal("registration review executed a mutation")
	}
	consent := review.CommandSourceConsent
	if consent == nil || consent.Kind != "command_source" || consent.Mode != "copy" || consent.PluginID != r.Code.PluginID || !strings.Contains(consent.LauncherScript, "client-plugin-path") || len(consent.ProviderTreeSHA256) != 64 || len(consent.LauncherSHA256) != 64 {
		t.Fatalf("review omitted the exact generated command and script: %+v", consent)
	}
	if _, err := ApplyCodeRegistration(context.Background(), r, run, review.Digest, false); err == nil || *effects != 0 {
		t.Fatal("narrow consent ran without operator confirmation")
	}
	observation, err := ApplyCodeRegistration(context.Background(), r, run, review.Digest, true)
	if err != nil || observation.State != "registered" || *effects != 1 {
		t.Fatalf("reviewed command-source installation remained unavailable: %+v %v (effects %d)", observation, err, *effects)
	}
}

func TestCodeCommandConsentPublicVendorGoldenFrames(t *testing.T) {
	// Independently calculated from the public 2.1.319 F()/Z() protocol, including
	// ASCII HTML characters, literal quotes, shell quoting and Windows slashes.
	cases := []struct{ command, revision, digest string }{
		{`'/synthetic/Developer/Maestro & O'"'"'Brien/Code.provider-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa/Maestro plugin path.command'`, "sha256:c37f416c51c325a5428037ee0f86e0443fbd207444efea878ab6487b203dc879", "8bea6e27a73407ab26565e81190fcdeadb4659363b7ef8c2c59b12606193ccbe"},
		{`powershell.exe -NoProfile -NonInteractive -File "C:\synthetic\Developer\Maestro & O'Brien\Code.provider-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\Maestro plugin path.ps1"`, "sha256:87b93b1d1151d54f692992e45f584390844ef5737a6eb36c64b9312bb2c246d0", "7314325629c869a828811124036d1ee602bec2196231ef7d74a572beeb27d260"},
	}
	for _, tc := range cases {
		catalog, err := commandSourceManifest(strings.Repeat("a", 32), tc.command)
		if err != nil {
			t.Fatal(err)
		}
		revision, digest, err := codeCommandHostDigests(catalog, "maestro-product@maestro-local-"+strings.Repeat("a", 32), tc.command)
		if err != nil || revision != tc.revision || digest != tc.digest {
			t.Fatalf("host protocol mismatch: %s %s %v", revision, digest, err)
		}
	}
}

func TestCodeCommandConsentRefusesChangedReviewedInputsBeforeEffects(t *testing.T) {
	for _, kind := range []string{"catalog", "script", "script-mode", "script-link", "current-source", "profile", "carrier", "owner", "provider-record"} {
		t.Run(kind, func(t *testing.T) {
			r, run, effects := commandConsentFixture(t)
			review, err := ReviewCodeRegistration(context.Background(), r, run)
			if err != nil {
				t.Fatal(err)
			}
			launcher := review.CommandSourceConsent.LauncherPath
			switch kind {
			case "catalog":
				err = os.WriteFile(filepath.Join(r.Materialization.CodeProviderRoot, ".claude-plugin", "marketplace.json"), []byte(`{"source":"foreign"}`), 0600)
			case "script":
				err = os.WriteFile(launcher, []byte("foreign command\n"), 0700)
			case "script-mode":
				if runtime.GOOS == "windows" {
					t.Skip("Windows ACL role is checked by native custody tests")
				}
				err = os.Chmod(launcher, 0600)
			case "script-link":
				if runtime.GOOS == "windows" {
					t.Skip("Windows alias creation requires host privilege")
				}
				old := launcher + ".preserved"
				if err = os.Rename(launcher, old); err == nil {
					err = os.Symlink(old, launcher)
				}
			case "current-source":
				err = os.WriteFile(filepath.Join(r.Materialization.CodeRoot, "plugins", "maestro-product", "method.md"), []byte("changed current source\n"), 0600)
			case "profile":
				r.Code.ConfigurationDirectory = filepath.Join(r.Binding.Workspace, "different-profile")
			case "carrier":
				r.CarrierID = strings.Repeat("b", 32)
			case "owner":
				r.Binding.UserHome = filepath.Join(r.Binding.Workspace, "different-owner")
			case "provider-record":
				r.Materialization.CodeProviderSHA256 = strings.Repeat("b", 64)
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err := ApplyCodeRegistration(context.Background(), r, run, review.Digest, true); err == nil || *effects != 0 {
				t.Fatalf("changed %s reached a command effect (%d): %v", kind, *effects, err)
			}
		})
	}
}

func TestCodeCommandConsentRechecksAfterNativeIdentityRead(t *testing.T) {
	r, original, effects := commandConsentFixture(t)
	review, err := ReviewCodeRegistration(context.Background(), r, original)
	if err != nil {
		t.Fatal(err)
	}
	versions := 0
	run := func(ctx context.Context, path string, args []string, cwd string) ([]byte, error) {
		if reflect.DeepEqual(args, []string{"--version"}) {
			versions++
			if versions == 2 {
				if err := os.WriteFile(review.CommandSourceConsent.LauncherPath, []byte("changed between observations\n"), 0700); err != nil {
					t.Fatal(err)
				}
			}
		}
		return original(ctx, path, args, cwd)
	}
	if _, err := ApplyCodeRegistration(context.Background(), r, run, review.Digest, true); err == nil || *effects != 0 {
		t.Fatalf("late provider change was not refused: %v, %d", err, *effects)
	}
}

func TestCodeCommandConsentUnavailableHostPreservesPendingState(t *testing.T) {
	r, original, effects := commandConsentFixture(t)
	run := func(ctx context.Context, path string, args []string, cwd string) ([]byte, error) {
		if args[len(args)-1] == "--help" {
			return []byte("--json --scope --accept-command-unrelated"), nil
		}
		return original(ctx, path, args, cwd)
	}
	if _, err := ReviewCodeRegistration(context.Background(), r, run); err == nil || !strings.Contains(err.Error(), "consent capability unavailable") || *effects != 0 {
		t.Fatalf("unsupported consent was not explicit and read-only: %v", err)
	}
}

func TestCodeCommandConsentKeepsOnlyClosedHostFailureCategory(t *testing.T) {
	for _, body := range []string{"PRIVATE DISPLAY\n" + `{"outcome":"failed","failureCode":"command_source_refused","message":"PRIVATE MESSAGE","shownCommand":{"command":"PRIVATE COMMAND"}}`, `{"outcome":"failed","failureCode":"PRIVATE UNKNOWN"}`} {
		err := codeMutationError([]byte(body), errors.New("exit status 1"))
		if strings.Contains(err.Error(), "PRIVATE") {
			t.Fatal("host disclosure escaped closed diagnostic", err)
		}
	}
}

func TestCodeCommandConsentPreservesOfficialChildSessionGuards(t *testing.T) {
	t.Setenv("CLAUDECODE", "0")
	t.Setenv("CLAUDE_CODE_CHILD_SESSION", "synthetic-child")
	t.Setenv("CLAUDE_CONFIG_DIR", "original-profile")
	values := map[string]string{}
	for _, entry := range clientCodeEnvironment("selected-profile") {
		key, value, ok := strings.Cut(entry, "=")
		if ok {
			values[key] = value
		}
	}
	if values["CLAUDECODE"] != "0" || values["CLAUDE_CODE_CHILD_SESSION"] != "synthetic-child" || values["CLAUDE_CONFIG_DIR"] != "selected-profile" {
		t.Fatal("profile selection changed host-owned session guards")
	}
	if os.Getenv("CLAUDE_CONFIG_DIR") != "original-profile" {
		t.Fatal("caller profile was changed")
	}
}

func commandConsentFixture(t *testing.T) (installer.ClientRecord, ClientCommand, *int) {
	t.Helper()
	root := t.TempDir()
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	id := strings.Repeat("a", 32)
	installerName := "maestro-install"
	if runtime.GOOS == "windows" {
		installerName += ".exe"
	}
	o := installer.ClientOptions{SeededOperationOptions: installer.SeededOperationOptions{Self: filepath.Join(root, "managed", installerName), ManagedRoot: filepath.Join(root, "managed"), UserHome: filepath.Join(root, "owner")}, Request: installer.ClientRequest{CarrierID: id, CodeRoot: filepath.Join(root, "Code"), Workspace: root}}
	provider, err := ensureCommandMarketplace(o)
	if err != nil {
		t.Fatal(err)
	}
	providerSHA, err := installer.ClientTreeDigest(provider)
	if err != nil {
		t.Fatal(err)
	}
	code := filepath.Join(root, "Code.generation-current")
	payload := filepath.Join(code, "plugins", "maestro-product")
	cache := filepath.Join(root, "native-cache")
	for _, dir := range []string{payload, cache} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(payload, "method.md"), []byte("canonical current method\n"), 0600); err != nil {
		t.Fatal(err)
	}
	codeSHA, err := installer.ClientTreeDigest(code)
	if err != nil {
		t.Fatal(err)
	}
	r := installer.ClientRecord{CarrierID: id, Binding: installer.ClientBinding{ManagedRoot: o.ManagedRoot, UserHome: o.UserHome, Workspace: root}, Materialization: installer.ClientMaterialization{Release: "0.1.55", CodeRoot: code, CodeTreeSHA256: codeSHA, Selection: &installer.ClientPacketSelection{CodeRoot: o.Request.CodeRoot}, CodeProviderRoot: provider, CodeProviderSHA256: providerSHA}, Code: installer.CodeObservation{Executable: executable, ConfigurationDirectory: filepath.Join(root, "selected-profile"), Marketplace: "maestro-local-" + id, PluginID: "maestro-product@maestro-local-" + id, Scope: "user"}}
	_, generated, _, err := commandSourceFiles(o, runtime.GOOS)
	if err != nil {
		t.Fatal(err)
	}
	_, providerCommand, err := commandSourceLauncher(provider, runtime.GOOS)
	if err != nil {
		t.Fatal(err)
	}
	_, expectedConsent, err := codeCommandHostDigests(generated[".claude-plugin/marketplace.json"], r.Code.PluginID, providerCommand)
	if err != nil {
		t.Fatal(err)
	}
	effects := new(int)
	installed := false
	run := func(_ context.Context, path string, args []string, cwd string) ([]byte, error) {
		if path != executable || cwd != root {
			t.Fatal("Code selection drift")
		}
		if reflect.DeepEqual(args, []string{"--version"}) {
			return []byte("2.1.319 (Claude Code)\n"), nil
		}
		if args[len(args)-1] == "--help" {
			return []byte("--json --scope --accept-command <sha256>\n"), nil
		}
		if reflect.DeepEqual(args, []string{"plugin", "marketplace", "list", "--json"}) {
			return json.Marshal([]any{map[string]any{"name": r.Code.Marketplace, "source": "directory", "path": provider}})
		}
		if reflect.DeepEqual(args, []string{"plugin", "list", "--json"}) {
			if !installed {
				return []byte("[]"), nil
			}
			return json.Marshal([]any{map[string]any{"id": r.Code.PluginID, "version": r.Materialization.Release, "scope": "user", "enabled": true, "readFromFolder": false, "installPath": cache}})
		}
		*effects++
		if len(args) < 2 || args[0] != "plugin" || args[1] != "install" {
			t.Fatalf("unexpected mutation: %q", args)
		}
		accepted := ""
		for i, arg := range args {
			if arg == "--yes" || arg == "-y" {
				t.Fatal("broad command approval replaced narrow consent")
			}
			if arg == "--accept-command" && i+1 < len(args) {
				accepted = args[i+1]
			}
		}
		if accepted != expectedConsent {
			return []byte(`{"command":"install","outcome":"failed","failureCode":"command_source_refused"}`), errors.New("command_source_refused")
		}
		if err := os.WriteFile(filepath.Join(cache, "method.md"), []byte("canonical current method\n"), 0600); err != nil {
			t.Fatal(err)
		}
		installed = true
		return []byte(`{"command":"install","outcome":"success"}`), nil
	}
	return r, run, effects
}
