package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestParticipantDesktopConfirmationUsesExactCurrentPackageAndContract(t *testing.T) {
	bashPath, bashErr := posixBash()
	plutil, plutilErr := exec.LookPath("plutil")
	macAvailable := runtime.GOOS == "darwin" && bashErr == nil && plutilErr == nil
	powershell, powershellErr := powerShellTool()
	if !macAvailable && powershellErr != nil {
		t.Skip("pure predicate runtimes unavailable; no launcher/native execution claimed")
	}
	t.Logf("pure predicate execution only: Bash/plutil=%v, PowerShell on this host=%v; no UI or client", macAvailable, powershellErr == nil)
	root, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	readHelper := func(file, start, end string) string {
		t.Helper()
		body, err := os.ReadFile(filepath.Join(root, "dev/pilot/templates/installed-seed", file))
		if err != nil {
			t.Fatal(err)
		}
		_, tail, ok := strings.Cut(string(body), start)
		if !ok {
			t.Fatal("helper missing")
		}
		helper, _, ok := strings.Cut(tail, end)
		if !ok {
			t.Fatal("helper boundary missing")
		}
		return start + helper
	}
	bash := readHelper("Maestro.command", "desktop_install_is_current() {", "open_desktop() {")
	ps := readHelper("Maestro.ps1", "  function Test-CurrentDesktopInstall($ClientStatus) {", "  function Open-Desktop(")
	for _, test := range []struct {
		name      string
		change    func(map[string]any, map[string]any)
		confirmed bool
	}{
		{"confirmed_but_connection_pending", func(_, _ map[string]any) {}, true},
		{"compatible_reuse_historical_manifest", func(d, m map[string]any) {
			d["manifest_sha256"] = strings.Repeat("c", 64)
			m["manifest_sha256"] = strings.Repeat("d", 64)
		}, true},
		{"not_confirmed", func(d, _ map[string]any) { d["install_confirmation"] = "awaiting_user_install" }, false},
		{"stale_package", func(d, _ map[string]any) { d["package_sha256"] = strings.Repeat("c", 64) }, false},
		{"stale_contract", func(d, _ map[string]any) { d["native_contract_sha256"] = strings.Repeat("c", 64) }, false},
		{"missing_confirmation_hash", func(d, _ map[string]any) { delete(d, "package_sha256") }, false},
		{"not_operator_attested", func(d, _ map[string]any) { d["evidence_class"] = "not_observed" }, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			d := map[string]any{"install_confirmation": "user_confirmed_install", "evidence_class": "operator_attested", "package_sha256": strings.Repeat("a", 64), "native_contract_sha256": strings.Repeat("b", 64), "connection": "pending", "functional_check": "pending"}
			m := map[string]any{"desktop_sha256": strings.Repeat("a", 64), "desktop_native_contract_sha256": strings.Repeat("b", 64)}
			test.change(d, m)
			body, err := json.Marshal(map[string]any{"record": map[string]any{"desktop": d, "materialization": m}})
			if err != nil {
				t.Fatal(err)
			}
			fixture := filepath.Join(t.TempDir(), "status.json")
			if err := os.WriteFile(fixture, body, 0600); err != nil {
				t.Fatal(err)
			}
			want := "pending"
			if test.confirmed {
				want = "confirmed"
			}
			if macAvailable {
				// Execute only the pure extracted predicate. The launcher, open,
				// clipboard, client, profile and setup paths are never executed.
				cmd := exec.Command(bashPath, "-c", `set -euo pipefail
jget(){ "$MAESTRO_PLUTIL" -extract "$2" raw -o - "$1"; }
`+bash+`if desktop_install_is_current "$MAESTRO_STATUS_FIXTURE"; then printf confirmed; else printf pending; fi`)
				cmd.Env = append(os.Environ(), "MAESTRO_STATUS_FIXTURE="+fixture, "MAESTRO_PLUTIL="+plutil)
				if out, err := cmd.CombinedOutput(); err != nil || string(out) != want {
					t.Fatalf("Mac confirmation predicate: %s %v want %s", out, err, want)
				}
			}
			if powershellErr == nil {
				cmd := exec.Command(powershell, "-NoLogo", "-NoProfile", "-Command", `Set-StrictMode -Version Latest; $ErrorActionPreference='Stop';`+ps+`$s=Get-Content -LiteralPath $env:MAESTRO_STATUS_FIXTURE -Raw|ConvertFrom-Json;if(Test-CurrentDesktopInstall $s){[Console]::Write('confirmed')}else{[Console]::Write('pending')}`)
				cmd.Env = append(os.Environ(), "MAESTRO_STATUS_FIXTURE="+fixture)
				if dev := os.Getenv("GOCACHE"); strings.Contains(dev, "/Developer/") {
					base := filepath.Join(strings.SplitN(dev, "/Developer/", 2)[0], "Developer", "ToolData", "powershell-maestro")
					cmd.Env = append(cmd.Env, "XDG_CACHE_HOME="+filepath.Join(base, "cache"), "XDG_CONFIG_HOME="+filepath.Join(base, "config"), "XDG_DATA_HOME="+filepath.Join(base, "data"))
				}
				if out, err := cmd.CombinedOutput(); err != nil || string(out) != want {
					t.Fatalf("Windows predicate in PowerShell on Mac: %s %v want %s", out, err, want)
				}
			}
		})
	}
}
