package main

import (
	"encoding/json"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Execute the shipped frontend helpers in a synthetic filesystem only.
// No installer, Claude, authentication, native qualification or GUI is invoked.
func TestMacSelfServicePreparesExactOwnerClientsAndPreservesProfile(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Mac shell/filesystem contract")
	}
	root := developerFixture(t)
	repo, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	source, err := os.ReadFile(filepath.Join(repo, "dev/pilot/templates/installed-seed/Maestro.command"))
	if err != nil {
		t.Fatal(err)
	}
	extract := func(start, end string) string {
		_, tail, ok := strings.Cut(string(source), start)
		if !ok {
			t.Fatalf("missing shipped helper %s", start)
		}
		body, _, ok := strings.Cut(tail, end)
		if !ok {
			t.Fatal("helper boundary missing")
		}
		return start + body
	}
	helpers := extract("private_dir() {", "private_dir \"$DEV/Maestro-Pilot\"") + extract("resolve_code_executable() {", "ensure_code_login() {")
	dev := filepath.Join(root, "Developer")
	owner := filepath.Join(dev, "existing owner")
	base := filepath.Join(dev, "pilot")
	for _, p := range []string{dev, owner, base} {
		if err := os.Mkdir(p, 0700); err != nil {
			t.Fatal(err)
		}
	}
	profile := filepath.Join(owner, "Code profile")
	run := func(body, input string) (string, error) {
		cmd := exec.Command("/bin/bash", "-c", "set -euo pipefail\numask 077\nfail(){ printf '%s' \"$*\" >&2; exit 1; }\n"+helpers+"\n"+body)
		cmd.Env = append(os.Environ(), "DEV="+dev, "OWNER="+owner, "BASE="+base, "ROLE=test-role", "BRAIN="+filepath.Join(owner, "brain"), "WORK="+filepath.Join(dev, "workspace"), "PROFILE="+profile)
		cmd.Stdin = strings.NewReader(input)
		b, e := cmd.CombinedOutput()
		return string(b), e
	}
	if out, err := run("prepare_client_parent", ""); err != nil || out != filepath.Join(owner, "Maestro-Clients", "test-role") {
		t.Fatalf("exact owner client parent: %q %v", out, err)
	}
	target := filepath.Join(owner, "Maestro-Clients", "test-role")
	info, err := os.Stat(target)
	if err != nil || info.Mode().Perm() != 0700 {
		t.Fatalf("client parent private: %v %v", info, err)
	}
	if out, err := run("prepare_code_configuration \"$PROFILE\"", "\n"); err == nil {
		t.Fatalf("profile created without consent: %s", out)
	}
	if _, err := os.Lstat(profile); !os.IsNotExist(err) {
		t.Fatal("decline created profile")
	}
	if out, err := run("prepare_code_configuration \"$PROFILE\"", "YES\n"); err != nil {
		t.Fatalf("new reviewed profile: %s %v", out, err)
	}
	settings := filepath.Join(profile, "settings.json")
	got, err := os.ReadFile(settings)
	if err != nil || string(got) != "{\"autoMemoryEnabled\":false,\"syncClaudeAiPlugins\":false,\"syncClaudeAiSkills\":false}\n" {
		t.Fatalf("new settings %q %v", got, err)
	}
	original := []byte("{\"syncClaudeAiSkills\":true,\"custom\":\"preserve-existing\"}\n")
	if err := os.WriteFile(settings, original, 0600); err != nil {
		t.Fatal(err)
	}
	if out, err := run("prepare_code_configuration \"$PROFILE\"", ""); err != nil {
		t.Fatalf("existing private profile: %s %v", out, err)
	}
	got, _ = os.ReadFile(settings)
	if string(got) != string(original) {
		t.Fatal("existing settings rewritten")
	}
	alias := filepath.Join(owner, "profile alias")
	if err := os.Symlink(profile, alias); err != nil {
		t.Fatal(err)
	}
	if out, err := run("prepare_code_configuration \"$OWNER/profile alias\"", "YES\n"); err == nil {
		t.Fatalf("alias accepted %s", out)
	}
	if out, err := run(`mkdir "$OWNER/.maestro-install"; prepare_code_configuration "$OWNER/.maestro-install/profile"`, "YES\n"); err == nil {
		t.Fatalf("canonical state admitted before profile creation: %s", out)
	}
	if _, err := os.Lstat(filepath.Join(owner, ".maestro-install", "profile")); !os.IsNotExist(err) {
		t.Fatal("reserved state received profile")
	}
	if out, err := run(`BRAIN="$OWNER/Maestro-Clients/brain"; ROLE=must-not-create; prepare_client_parent`, ""); err == nil {
		t.Fatalf("overlapping brain/client parent accepted: %s", out)
	}
	if _, err := os.Lstat(filepath.Join(owner, "Maestro-Clients", "must-not-create")); !os.IsNotExist(err) {
		t.Fatal("overlap created client child")
	}
	if out, err := run(`BRAIN="$OWNER/brain/../Maestro-Clients/brain"; ROLE=must-not-create; prepare_client_parent`, ""); err == nil {
		t.Fatalf("ambiguous brain/client parent accepted: %s", out)
	}
	executable := filepath.Join(owner, "regular executable")
	if err := os.WriteFile(executable, []byte("not executed"), 0700); err != nil {
		t.Fatal(err)
	}
	for _, pair := range [][2]string{{"regular executable", "relative-link"}, {"relative-link", "outer-link"}, {"loop-b", "loop-a"}, {"loop-a", "loop-b"}} {
		if err := os.Symlink(pair[0], filepath.Join(owner, pair[1])); err != nil {
			t.Fatal(err)
		}
	}
	if out, err := run("resolve_code_executable \"$OWNER/outer-link\"", ""); err != nil || out != executable {
		t.Fatalf("canonical executable %q %v", out, err)
	}
	if out, err := run("resolve_code_executable \"$OWNER/loop-a\"", ""); err == nil {
		t.Fatalf("symlink loop accepted %s", out)
	}
}

// The copied fixture is a synthetic identity-protocol process, not a native
// qualification witness. This exercises the exact cat staging and the original
// setup-review identity reader; no signed role or real installation is issued.
func TestMacStagedInitialServerCanAnswerOriginalIdentityReview(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Mac staging file-mode contract")
	}
	root := developerFixture(t)
	repo, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	source, err := os.ReadFile(filepath.Join(repo, "dev/pilot/templates/installed-seed/Maestro.command"))
	if err != nil {
		t.Fatal(err)
	}
	_, tail, ok := strings.Cut(string(source), "# plutil raw arrays")
	if !ok {
		t.Fatal("staging block missing")
	}
	staging, _, ok := strings.Cut(tail, "BOOTSTRAP=")
	if !ok {
		t.Fatal("staging boundary missing")
	}
	staging = "# plutil raw arrays" + staging
	public := filepath.Join(root, "kit")
	stage := filepath.Join(root, "stage")
	for _, p := range []string{public, stage, filepath.Join(public, "initial-release"), filepath.Join(public, "bootstrap"), filepath.Join(public, "test-documents")} {
		if err := os.Mkdir(p, 0700); err != nil {
			t.Fatal(err)
		}
	}
	resources := []releasecontract.EmbeddedResource{{URI: "maestro://synthetic-staging-test", Bytes: 1, SHA256: strings.Repeat("a", 64)}}
	digest, err := releasecontract.EmbeddedDigest(resources)
	if err != nil {
		t.Fatal(err)
	}
	identity := releasecontract.ServerIdentity{SchemaVersion: 2, Product: "maestro", Component: "mcp_server", Protocol: releasecontract.ServerProtocol, Version: "0.1.53", CompatibleBundle: ">=0.1.0 <0.2.0", AuthorityRegistrySHA256: strings.Repeat("b", 64), Resources: resources, EmbeddedInputsSHA256: digest, ExecutionContract: releasecontract.SyncedSourceContract}
	identityJSON, err := json.Marshal(identity)
	if err != nil {
		t.Fatal(err)
	}
	serverName := "maestro-mcp_0.1.53_darwin_" + runtime.GOARCH
	serverBody := []byte("#!/bin/sh\n[ \"$1\" = identity ] || exit 71\nprintf '%s\\n' '" + string(identityJSON) + "'\n")
	manifest := map[string]any{"server": map[string]any{"version": "0.1.53"}, "artifacts": []any{map[string]any{"kind": "mcp_server", "os": "darwin", "arch": runtime.GOARCH, "name": serverName, "size": len(serverBody), "sha256": sum(serverBody)}}}
	manifestJSON, _ := json.Marshal(manifest)
	bodies := map[string][]byte{"initial-release/" + serverName: serverBody, "initial-release/release-manifest.json": manifestJSON, "initial-release/release-manifest.json.sig": []byte("synthetic-not-a-signature"), "initial-release/notes.md": []byte("notes"), "initial-release/archive.tar.gz": []byte("archive"), "test-documents/sample.docx": []byte("document"), "bootstrap/inert": []byte("not executed")}
	files := []kitFile{}
	for name, body := range bodies {
		if err := os.WriteFile(filepath.Join(public, name), body, 0600); err != nil {
			t.Fatal(err)
		}
		files = append(files, kitFile{Path: name, SHA256: sum(body), Size: int64(len(body))})
	}
	metadata := map[string]any{"files": files, "server_hashes": []string{sum(serverBody), strings.Repeat("d", 64)}, "identity": map[string]any{"target_os": "darwin", "target_arch": runtime.GOARCH, "admission_sha256": strings.Repeat("c", 64)}}
	metaJSON, _ := json.Marshal(metadata)
	meta := filepath.Join(public, "DISTRIBUTION.json")
	if err := os.WriteFile(meta, metaJSON, 0600); err != nil {
		t.Fatal(err)
	}
	script := `set -euo pipefail
umask 077
fail(){ printf '%s' "$*" >&2; exit 1; }
sha(){ local value; value="$(/usr/bin/shasum -a 256 "$1")"; printf '%s' "${value%% *}"; }
jget(){ /usr/bin/plutil -extract "$2" raw -o - "$1"; }
[[ "$(sha "$META")" == "$EXPECTED_META" ]] || fail 'metadata changed'
[[ "$(jget "$META" identity.admission_sha256)" == "$ROLE" ]] || fail 'role selection changed'
[[ "$(jget "$META" identity.target_os)" == darwin && "$(jget "$META" identity.target_arch)" == "$ARCH" ]] || fail 'target changed'
` + staging + `
if declare -F prepare_initial_server >/dev/null; then prepare_initial_server; fi
`
	cmd := exec.Command("/bin/bash", "-c", script)
	cmd.Env = append(os.Environ(), "ROOT="+public, "STAGE="+stage, "META="+meta, "ARCH="+runtime.GOARCH, "ROLE="+strings.Repeat("c", 64), "EXPECTED_META="+sum(metaJSON))
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("shipping inventory staging: %s %v", out, err)
	}
	staged := filepath.Join(stage, "initial-release", serverName)
	actual, err := releasecontract.ReadServerIdentity(staged)
	if err != nil {
		t.Fatalf("original setup-review identity failed after actual cat staging: %v", err)
	}
	if actual.Version != identity.Version {
		t.Fatal("wrong server identity")
	}
	info, err := os.Stat(staged)
	if err != nil || info.Mode().Perm() != 0700 {
		t.Fatalf("exact server permission: %v %v", info, err)
	}
	for name := range bodies {
		if strings.HasSuffix(name, serverName) {
			continue
		}
		info, err := os.Stat(filepath.Join(stage, name))
		if err != nil || info.Mode().Perm() != 0600 {
			t.Fatalf("non-server permission changed: %s %v %v", name, info, err)
		}
	}
	// Resume must not turn an aliased or public staged file into an executable.
	for _, scenario := range []string{"group-readable", "hardlink"} {
		t.Run(scenario, func(t *testing.T) {
			if err := os.Chmod(staged, 0600); err != nil {
				t.Fatal(err)
			}
			link := filepath.Join(stage, "linked-server")
			if scenario == "group-readable" {
				if err := os.Chmod(staged, 0644); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := os.Link(staged, link); err != nil {
					t.Fatal(err)
				}
				defer os.Remove(link)
			}
			retry := exec.Command("/bin/bash", "-c", script)
			retry.Env = cmd.Env
			if out, err := retry.CombinedOutput(); err == nil {
				t.Fatalf("unsafe staged server accepted: %s", out)
			}
			info, err := os.Stat(staged)
			if err != nil {
				t.Fatal(err)
			}
			if info.Mode().Perm()&0100 != 0 {
				t.Fatal("unsafe staged server was granted execution")
			}
		})
	}

}
