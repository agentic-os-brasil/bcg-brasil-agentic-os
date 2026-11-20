package main

import (
	"encoding/json"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installtx"
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
	t.Run("shared source enrollment", func(t *testing.T) {
		fixture := filepath.Join(root, "shared-source")
		owner := filepath.Join(fixture, "owner")
		product := filepath.Join(fixture, "product")
		kit := filepath.Join(fixture, "kit")
		shared := filepath.Join(fixture, "hydrated team source")
		run := filepath.Join(fixture, "run")
		for _, path := range []string{filepath.Join(owner, ".maestro-install", "config"), product, filepath.Join(kit, "source"), shared, run} {
			if err := os.MkdirAll(path, 0700); err != nil {
				t.Fatal(err)
			}
		}
		state, _ := json.Marshal(installtx.State{SchemaVersion: 4, ManagedRoot: product})
		if err := os.WriteFile(filepath.Join(owner, ".maestro-install/config/install-state.json"), state, 0600); err != nil {
			t.Fatal(err)
		}
		meta := filepath.Join(fixture, "metadata.json")
		if err := os.WriteFile(meta, []byte(`{"identity":{"source":{"source_id":"team","channel":"canary","os":"darwin","arch":"arm64"}}}`), 0600); err != nil {
			t.Fatal(err)
		}
		producer := extract("current_clients=false", "# Current installer owns")
		script := `set -euo pipefail
umask 077
fail(){ printf '%s' "$*" >&2; exit 1; }
jget(){ /usr/bin/plutil -extract "$2" raw -o - "$1"; }
COMMON=(--fixture)
fixture_installer(){
  case "$1" in
    clients-status) printf '%s' '{"state":"pending"}';;
    source-review)
      cat > "$RUN/observed-request.json"
      printf '%s\n' source-review >> "$RUN/commands"
      [[ "$(jget "$STATE" schema_version)" == 4 && "$(jget "$STATE" managed_root)" == "$PRODUCT" ]] || { printf 'original owner rejects state' >&2; return 32; }
      if selected="$(jget "$RUN/observed-request.json" source_root 2>/dev/null)"; then
        [[ "$(jget "$RUN/observed-request.json" source.source_id)" == team ]] || return 33
      else
        selected="$(jget "$STATE" source_control.locator)" || return 34
      fi
      [[ "$selected" == "$EXPECTED_SOURCE" ]] || { printf 'source choice ignored: %s' "$selected" >&2; return 31; }
      printf '%s' '{"digest":"exact-review"}';;
    source-check)
      cat > "$RUN/checked-request.json"
      [[ "$2" == --review-digest && "$3" == exact-review && "$4" == --confirm ]] || return 35
      /usr/bin/cmp -s "$RUN/observed-request.json" "$RUN/checked-request.json" || return 36
      if ! /usr/bin/plutil -type source_control "$STATE" >/dev/null 2>&1; then
        /usr/bin/plutil -insert source_control -dictionary "$STATE"
        /usr/bin/plutil -insert source_control.locator -string "$EXPECTED_SOURCE" "$STATE"
      fi
      if [[ "$SOURCE_ADVANCED" == true ]]; then printf '%s' '{"state":"candidate_staged_activation_unconfirmed","plan_id":"exact-plan"}'; else printf '%s' '{"state":"current_signed_release"}'; fi;;
    update-review) cat > "$RUN/update-reviewed.json"; printf '%s' '{"plan_id":"exact-plan"}';;
    update) cat > "$RUN/update-applied.json"; [[ "$2" == --plan-id && "$3" == exact-plan && "$4" == --confirm ]] || return 37; printf '%s' '{"state":"activation_readback"}';;
    *) return 93;;
  esac
}
INSTALLED=fixture_installer
manage(){ :; }
` + extract("json() {", "# Resolve normal installer symlinks") + extract("review_confirm() {", "if [[ ! -e \"$INSTALLED\"") + producer
		statePath := filepath.Join(owner, ".maestro-install/config/install-state.json")
		attempt := func(directory, input, expected, advanced string) ([]byte, error) {
			if err := os.MkdirAll(directory, 0700); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command("/bin/bash", "-c", script)
			cmd.Env = append(os.Environ(), "ROOT="+kit, "OWNER="+owner, "PRODUCT="+product, "RUN="+directory, "SHARED="+filepath.Join(kit, "source"), "EXPECTED_SOURCE="+expected, "META="+meta, "STATE="+statePath, "SOURCE_ADVANCED="+advanced)
			cmd.Stdin = strings.NewReader(input)
			return cmd.CombinedOutput()
		}
		out, err := attempt(run, shared+"\nYES\n", shared, "false")
		if err != nil {
			t.Fatalf("first source must honor external hydrated folder before review: %s %v", out, err)
		}
		var observed struct {
			SourceRoot string `json:"source_root"`
		}
		body, err := os.ReadFile(filepath.Join(run, "checked-request.json"))
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(body, &observed); err != nil || observed.SourceRoot != shared {
			t.Fatalf("wrong reviewed source %s %v", body, err)
		}

		boundBefore, _ := os.ReadFile(statePath)
		rerun := filepath.Join(fixture, "rerun")
		if out, err := attempt(rerun, "YES\n", shared, "false"); err != nil {
			t.Fatalf("bound source must resume without a new choice: %s %v", out, err)
		}
		body, _ = os.ReadFile(filepath.Join(rerun, "observed-request.json"))
		var request map[string]any
		if err := json.Unmarshal(body, &request); err != nil {
			t.Fatal(err)
		}
		if _, ok := request["source_root"]; ok {
			t.Fatal("legacy cached path was resubmitted for bound source")
		}
		if _, ok := request["source"]; ok {
			t.Fatal("bound source tuple was replaced by cached distribution metadata")
		}
		boundAfter, _ := os.ReadFile(statePath)
		if string(boundBefore) != string(boundAfter) {
			t.Fatal("rerun rewrote source custody")
		}
		commands, _ := os.ReadFile(filepath.Join(rerun, "commands"))
		if string(commands) != "source-review\n" {
			t.Fatalf("source review repeated: %s", commands)
		}
		for _, scenario := range []string{"local-test", "decline", "empty-choice", "advanced-source", "unknown-schema", "wrong-schema-type", "malformed", "wrong-root", "invalid-source"} {
			t.Run(scenario, func(t *testing.T) {
				body := state
				if scenario == "unknown-schema" {
					body = []byte(`{"schema_version":99}`)
				}
				if scenario == "wrong-schema-type" {
					body, _ = json.Marshal(map[string]any{"schema_version": "4", "managed_root": product})
				}
				if scenario == "malformed" {
					body = []byte(`{`)
				}
				if scenario == "wrong-root" {
					body = []byte(`{"schema_version":4,"managed_root":"/another/product"}`)
				}
				if scenario == "invalid-source" {
					body, _ = json.Marshal(map[string]any{"schema_version": 4, "managed_root": product, "source_control": "invalid"})
				}
				if err := os.WriteFile(statePath, body, 0600); err != nil {
					t.Fatal(err)
				}
				dir := filepath.Join(fixture, scenario)
				input, expected, advanced := shared+"\nYES\n", shared, "false"
				if scenario == "local-test" {
					input = "TESTE-LOCAL\nYES\n"
					expected = filepath.Join(kit, "source")
				}
				if scenario == "decline" {
					input = shared + "\n\n"
				}
				if scenario == "empty-choice" {
					input = "\n"
				}
				if scenario == "advanced-source" {
					input = shared + "\nYES\nYES\n"
					advanced = "true"
				}
				if strings.HasPrefix(scenario, "unknown") || scenario == "wrong-schema-type" || scenario == "malformed" || scenario == "wrong-root" || scenario == "invalid-source" {
					input = ""
				}
				out, err := attempt(dir, input, expected, advanced)
				wantSuccess := scenario == "local-test" || scenario == "advanced-source"
				if (err == nil) != wantSuccess {
					t.Fatalf("source scenario %s: %s %v", scenario, out, err)
				}
				if scenario == "local-test" && !strings.Contains(string(out), "Ensaio local") {
					t.Fatal("local sample was presented as a team source")
				}
				if scenario == "advanced-source" {
					if _, err := os.Stat(filepath.Join(dir, "update-applied.json")); err != nil {
						t.Fatal("advanced first source lost existing reviewed update flow")
					}
				}
				if !wantSuccess {
					after, _ := os.ReadFile(statePath)
					if string(after) != string(body) {
						t.Fatal("refusal rewrote state")
					}
					if _, err := os.Stat(filepath.Join(dir, "checked-request.json")); !os.IsNotExist(err) {
						t.Fatal("refusal invoked source-check")
					}
					if strings.Contains(string(out), "Selecione a pasta SOURCE") && scenario != "decline" && scenario != "empty-choice" {
						t.Fatal("unknown state was treated as unenrolled")
					}
				}
			})
		}
	})

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
