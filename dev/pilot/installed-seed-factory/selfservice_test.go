package main

import (
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
	if err != nil || string(got) != "{\"autoMemoryEnabled\":false,\"syncClaudeAiPlugins\":false}\n" {
		t.Fatalf("new settings %q %v", got, err)
	}
	original := []byte("{\"custom\":\"preserve-existing\"}\n")
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
