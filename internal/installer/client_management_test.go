package installer

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestFixedUpdateLauncherKeepsNativeReviewWithExactParticipantSelection(t *testing.T) {
	o := ClientOptions{SeededOperationOptions: SeededOperationOptions{Self: "/Users/participant/Developer/Maestro/maestro-install", ManagedRoot: "/Users/participant/Developer/Maestro", UserHome: "/Users/participant"}, Request: ClientRequest{CarrierID: strings.Repeat("a", 32), Workspace: "/Users/participant/Developer/Brain", CodeRoot: "/Users/participant/Developer/Clients/Code", DesktopPath: "/Users/participant/Developer/Clients/Desktop.mcpb", CodeScope: "user"}}
	_, body := ClientManagementLauncher(o, true)
	if !bytes.Contains(body, []byte("--management-intent")) || !bytes.Contains(body, []byte("update")) || bytes.Contains(body, []byte("--confirm")) || bytes.Contains(body, []byte("--review-digest")) {
		t.Fatal("handoff recipe altered installer approval boundary")
	}
	root := t.TempDir()
	path := filepath.Join(root, "Update Maestro.command")
	if err := os.WriteFile(path, body, 0700); err != nil {
		t.Fatal(err)
	}
	if err := verifyManagementFile(path, body, true); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("foreign command\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := verifyManagementFile(path, body, true); err == nil {
		t.Fatal("changed update launcher admitted")
	}
	if bytes.Contains(bytes.ToLower(ClientWindowsManagementEntry()), []byte("bypass")) {
		t.Fatal("Windows normal policy bypassed")
	}
}

func TestManagementRecipeRetainsReviewedCodeConfiguration(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX recipe execution; Windows quoting covered by source recipe owner")
	}
	root := t.TempDir()
	profile := filepath.Join(root, "profile with ' quote")
	if err := os.Mkdir(profile, 0700); err != nil {
		t.Fatal(err)
	}
	request := ClientRequest{CarrierID: strings.Repeat("b", 32), Workspace: root, CodeRoot: filepath.Join(root, "code"), DesktopPath: filepath.Join(root, "desktop.mcpb"), CodeScope: "user"}
	raw, _ := json.Marshal(request)
	var fields map[string]any
	json.Unmarshal(raw, &fields)
	fields["code_configuration_directory"] = profile
	raw, _ = json.Marshal(fields)
	json.Unmarshal(raw, &request)
	out := filepath.Join(root, "observed")
	executable := filepath.Join(root, "owner recorder")
	if err := os.WriteFile(executable, []byte("#!/bin/sh\nprintf '%s\\n' \"$CLAUDE_CONFIG_DIR\" \"$@\" > \"$MAESTRO_PROFILE_OBSERVATION\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	o := ClientOptions{SeededOperationOptions: SeededOperationOptions{Self: executable, ManagedRoot: root, UserHome: root}, Request: request}
	for _, update := range []bool{false, true} {
		_, body := ClientManagementLauncher(o, update)
		cmd := exec.Command("/bin/sh", "-c", string(body))
		cmd.Env = append(os.Environ(), "CLAUDE_CONFIG_DIR=unrelated-main-profile", "MAESTRO_PROFILE_OBSERVATION="+out)
		if b, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("recipe: %s %v", b, err)
		}
		b, err := os.ReadFile(out)
		if err != nil {
			t.Fatal(err)
		}
		lines := strings.Split(string(b), "\n")
		if lines[0] != profile {
			t.Fatalf("selected profile lost: %q", lines[0])
		}
		if !strings.Contains(string(b), "--client-code-config\n"+profile+"\n") {
			t.Fatalf("reviewed profile argument lost: %s", b)
		}
		if strings.Contains(string(b), "--confirm") {
			t.Fatal("recipe bypassed review")
		}
	}
}

func TestWindowsManagementRecipeRestoresEnvironmentAndQuotesSelection(t *testing.T) {
	pwsh, err := exec.LookPath("pwsh")
	if err != nil {
		t.Skip("PowerShell runtime unavailable; Windows runtime not claimed")
	}
	root := t.TempDir()
	profile := filepath.Join(root, "profile ' with $() "+string(rune(96))+" literal")
	recorder := filepath.Join(root, "recorder.ps1")
	if err := os.WriteFile(recorder, []byte("[IO.File]::WriteAllText($env:MAESTRO_PROFILE_OBSERVATION, $env:CLAUDE_CONFIG_DIR + [Environment]::NewLine + [string]::Join([Environment]::NewLine, $args)); exit ([int]$env:MAESTRO_PROFILE_EXIT)"), 0600); err != nil {
		t.Fatal(err)
	}
	request := ClientRequest{CarrierID: strings.Repeat("c", 32), Workspace: root, CodeRoot: filepath.Join(root, "code"), DesktopPath: filepath.Join(root, "desktop.mcpb"), CodeScope: "user", CodeConfigurationDirectory: profile}
	o := ClientOptions{SeededOperationOptions: SeededOperationOptions{Self: recorder, ManagedRoot: root, UserHome: root}, Request: request}
	for _, exitCode := range []string{"0", "17"} {
		_, body := clientManagementLauncherForOS(o, true, "windows")
		path := filepath.Join(root, "Update Maestro.ps1")
		if err := os.WriteFile(path, body, 0600); err != nil {
			t.Fatal(err)
		}
		out := filepath.Join(root, "observed")
		cmd := exec.Command(pwsh, "-NoLogo", "-NoProfile", "-Command", "& $env:MAESTRO_PROFILE_RECIPE; $actual = $LASTEXITCODE; if ($env:CLAUDE_CONFIG_DIR -cne 'unrelated-main-profile') {throw 'caller profile changed'}; if ($actual -ne [int]$env:MAESTRO_PROFILE_EXIT) {throw 'exit changed'}")
		cmd.Env = append(os.Environ(), "CLAUDE_CONFIG_DIR=unrelated-main-profile", "MAESTRO_PROFILE_RECIPE="+path, "MAESTRO_PROFILE_OBSERVATION="+out, "MAESTRO_PROFILE_EXIT="+exitCode)
		if b, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("PowerShell recipe %s: %s %v", exitCode, b, err)
		}
		b, err := os.ReadFile(out)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(string(b), profile+"\n") || !strings.Contains(string(b), "--client-code-config\n"+profile) {
			t.Fatalf("literal profile lost: %s", b)
		}
	}
}

func TestWindowsManagementRecipeRefusesMissingExecutable(t *testing.T) {
	pwsh, err := exec.LookPath("pwsh")
	if err != nil {
		t.Skip("PowerShell runtime unavailable")
	}
	root := t.TempDir()
	o := ClientOptions{SeededOperationOptions: SeededOperationOptions{Self: filepath.Join(root, "missing-executable"), ManagedRoot: root, UserHome: root}, Request: ClientRequest{CodeConfigurationDirectory: filepath.Join(root, "selected-profile")}}
	_, body := clientManagementLauncherForOS(o, false, "windows")
	path := filepath.Join(root, "Manage Maestro.ps1")
	if err := os.WriteFile(path, body, 0600); err != nil {
		t.Fatal(err)
	}
	for _, prior := range []string{"0", "7"} {
		cmd := exec.Command(pwsh, "-NoLogo", "-NoProfile", "-Command", "$LASTEXITCODE = [int]$env:MAESTRO_PRIOR_EXIT; try { & $env:MAESTRO_PROFILE_RECIPE; throw 'missing executable reported success' } catch { if ($env:CLAUDE_CONFIG_DIR -cne 'unrelated-main-profile') {throw 'profile not restored'}; if ($_.Exception.Message -like '*reported success*') {throw}; exit 23 }")
		cmd.Env = append(os.Environ(), "CLAUDE_CONFIG_DIR=unrelated-main-profile", "MAESTRO_PROFILE_RECIPE="+path, "MAESTRO_PRIOR_EXIT="+prior)
		b, err := cmd.CombinedOutput()
		var child *exec.ExitError
		if !errors.As(err, &child) || child.ExitCode() != 23 {
			t.Fatalf("missing executable did not refuse after restored env: %s %v", b, err)
		}
	}
}
