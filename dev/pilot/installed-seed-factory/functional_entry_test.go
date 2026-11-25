package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// Exercise the shipped entry's explicit choice, report writer and failure return.
// The inert checker stands in for the separately tested installed MCP owner.
func TestParticipantFunctionalCheckIsExplicitAndRecoverable(t *testing.T) {
	for _, platform := range []string{"mac", "windows"} {
		t.Run(platform, func(t *testing.T) {
			var shell string
			if platform == "mac" {
				if runtime.GOOS != "darwin" {
					t.Skip("system Bash entry is qualified on macOS")
				}
				shell = "/bin/bash"
			} else {
				name := "pwsh"
				if runtime.GOOS == "windows" {
					name = "powershell.exe"
				}
				var err error
				shell, err = exec.LookPath(name)
				if err != nil {
					if runtime.GOOS == "windows" {
						t.Fatal("native Windows PowerShell 5.1 is required")
					}
					t.Skip("PowerShell unavailable; native Windows remains unqualified")
				}
			}
			name := "Maestro.command"
			if platform == "windows" {
				name = "Maestro.ps1"
			}
			source, err := os.ReadFile(filepath.Join("../templates/installed-seed", name))
			if err != nil {
				t.Fatal(err)
			}
			extract := func(t *testing.T, start, end string) string {
				t.Helper()
				_, tail, found := strings.Cut(string(source), start)
				if !found {
					t.Fatalf("shipped helper missing: %s", start)
				}
				body, _, found := strings.Cut(tail, end)
				if !found {
					t.Fatalf("shipped helper end missing: %s", end)
				}
				return start + body
			}
			for _, outcome := range []string{"passed", "incomplete", "malformed", "wrong_mode"} {
				t.Run(outcome, func(t *testing.T) {
					root := filepath.Join(developerFixture(t), "entry with spaces")
					if err := os.Mkdir(root, 0700); err != nil {
						t.Fatal(err)
					}
					response := `{"schema_version":2,"evidence_class":"engineering_only","check_mode":"functional","functional_check":"passed","markdown":"MCP SDK only; native clients pending","code_prompt":"Code unchanged","code_readback_prompt":"Read unchanged","desktop_prompt":"Desktop unchanged","code_ready_prompt":"Code pending","desktop_ready_prompt":"Desktop pending"}`
					if outcome == "wrong_mode" {
						response = strings.Replace(response, `"check_mode":"functional","functional_check":"passed"`, `"check_mode":"check_only","functional_check":"not_requested"`, 1)
					}
					writeFixture(t, root, "response.json", []byte(response))
					var script string
					var args []string
					if platform == "mac" {
						helper := extract(t, "refresh_report() {", "\nrefresh_report\n") + "\n" + extract(t, "functional_test() {", "\nattest() {")
						action := extract(t, "    10) ", ";;")
						action = strings.TrimPrefix(action, "    10) ")
						server := writeFixture(t, root, "checker with spaces", []byte(`#!/bin/bash
printf 'BEGIN\n' >> "$ROOT/calls"
printf '%s\n' "$@" >> "$ROOT/calls"
if [[ " $* " == *' --functional '* ]]; then
  if [[ "$OUTCOME" == malformed ]]; then printf 'not-json'; exit 3; fi
  cat "$ROOT/response.json"
  [[ "$OUTCOME" != incomplete ]] || exit 2
else cat "$ROOT/response.json"; fi
`))
						if err := os.Chmod(server, 0700); err != nil {
							t.Fatal(err)
						}
						body, _ := os.ReadFile(server)
						writeFixture(t, root, "metadata.json", []byte(`{"server_hashes":["`+sum(body)+`","`+strings.Repeat("f", 64)+`"]}`))
						script = `set -euo pipefail
umask 077
fail(){ printf '%s\n' "$*" >&2; exit 19; }
private_dir(){ [[ -d "$1" ]] || mkdir "$1"; }
verify_installer(){ :; }
sha(){ local value; value="$(/usr/bin/shasum -a 256 "$1")"; printf '%s' "${value%% *}"; }
jget(){ /usr/bin/plutil -extract "$2" raw -o - "$1"; }
installer_fixture(){ printf '{}'; }
DIAGNOSTIC_ROOT="$ROOT/diagnostic"; RUN="$DIAGNOSTIC_ROOT"; mkdir "$RUN"
INSTALLED=installer_fixture; SERVER="$ROOT/checker with spaces"; META="$ROOT/metadata.json"; VERSION=0.1.1; ROLE=role; PRODUCT="$ROOT/product with spaces"
COMMON=(--workspace "$ROOT/workspace with spaces" --carrier-id exact-carrier); CLIENT_STATUS=("${COMMON[@]}"); PILOT_OBSERVATION_CLIENT=desktop
` + helper + `
refresh_report
cp "$RUN/RELATORIO.md" "$ROOT/original.md"
` + action + `
printf 'RETURNED_TO_MENU\n'
cmp "$DIAGNOSTIC_ROOT/RELATORIO.md" "$ROOT/original.md"
`
						args = []string{"-c", script}
					} else {
						helper := extract(t, "  function Update-PilotReport", "\n  Update-PilotReport\n") + extract(t, "  function Invoke-FunctionalTest", "\n  function Assert-TestPrompt")
						action := strings.TrimPrefix(extract(t, "      '10' { ", " }"), "      '10' { ")
						script = `Set-StrictMode -Version Latest; $ErrorActionPreference='Stop'
if([Environment]::OSVersion.Platform -eq 'Win32NT' -and ($PSVersionTable.PSEdition -cne 'Desktop' -or $PSVersionTable.PSVersion.Major -ne 5 -or $PSVersionTable.PSVersion.Minor -ne 1)){throw 'Windows PowerShell 5.1 required'}
$Utf8=New-Object Text.UTF8Encoding($false)
$DiagnosticRoot=Join-Path $env:ROOT 'diagnostic'; $Run=$DiagnosticRoot; [IO.Directory]::CreateDirectory($Run)|Out-Null
$Report=Join-Path $Run 'RELATORIO.md'; $Installer='inert'; $Server='Checker-Fixture'; $Meta=@{identity=@{installer_version='0.1.1'};server_hashes=@('pin')}; $Role='role'; $Product=Join-Path $env:ROOT 'product with spaces'
$Common=@('--workspace',(Join-Path $env:ROOT 'workspace with spaces'),'--carrier-id','exact-carrier'); $script:ObservationClient='desktop'
function Private-Directory([string]$Path){[IO.Directory]::CreateDirectory($Path)|Out-Null}
function Verify-Installer([string]$Path){}
function Current-Clients{return @{state='partial'}}
function Private-File([string]$Path){}
function Hash([string]$Path){return 'pin'}
function Stop-Maestro([string]$Message){throw $Message}
function Write-Json([string]$Path,$Value){if(Test-Path -LiteralPath $Path){throw 'overwritten report'};[IO.File]::WriteAllText($Path,($Value|ConvertTo-Json -Depth 16),$Utf8)}
function Checker-Fixture {
  [IO.File]::AppendAllText((Join-Path $env:ROOT 'calls'),('BEGIN'+[char]10+($args -join [string][char]10)+[char]10),$Utf8)
  $global:LASTEXITCODE=0
  if($args -ccontains '--functional'){
    if($env:OUTCOME -ceq 'malformed'){$global:LASTEXITCODE=3;return 'not-json'}
    if($env:OUTCOME -ceq 'incomplete'){$global:LASTEXITCODE=2}
  }
  Get-Content -LiteralPath (Join-Path $env:ROOT 'response.json') -Raw -Encoding UTF8
}
` + helper + `
Update-PilotReport
$original=[IO.File]::ReadAllText($Report)
$originalRun=$Run
` + action + `
Write-Host 'RETURNED_TO_MENU'
if($Run -cne $originalRun -or $Report -cne (Join-Path $originalRun 'RELATORIO.md')){throw 'manual client journey lost its current report'}
if([IO.File]::ReadAllText((Join-Path $DiagnosticRoot 'RELATORIO.md')) -cne $original){throw 'earlier report changed'}
`
						args = []string{"-NoLogo", "-NoProfile", "-Command", script}
					}
					ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
					defer cancel()
					cmd := exec.CommandContext(ctx, shell, args...)
					cmd.Env = append(os.Environ(), "ROOT="+root, "OUTCOME="+outcome)
					output, err := cmd.CombinedOutput()
					if err != nil || !strings.Contains(string(output), "RETURNED_TO_MENU") {
						t.Fatalf("functional action did not return to menu: %v\n%s", err, output)
					}
					if passed := strings.Contains(string(output), "Teste MCP encerrado"); passed != (outcome == "passed") {
						t.Fatalf("functional result was projected incorrectly: %s", output)
					}
					if outcome != "passed" && !strings.Contains(string(output), "Teste MCP incompleto") {
						t.Fatalf("incomplete action has no recovery guidance: %s", output)
					}
					calls, err := os.ReadFile(filepath.Join(root, "calls"))
					if err != nil {
						t.Fatal(err)
					}
					parts := strings.Split(strings.TrimSpace(string(calls)), "BEGIN\n")
					if len(parts) != 3 {
						t.Fatalf("expected one startup check and one explicit check: %s", calls)
					}
					for index, mode := range []string{"--check-only", "--functional"} {
						want := fmt.Sprintf("installed-pilot-check\n--workspace\n%s\n--carrier-id\nexact-carrier\n--installer-version\n0.1.1\n--experimental-seed-sha256\nrole\n--experimental-installed-root\n%s\n%s\n--observation-client\ndesktop", filepath.Join(root, "workspace with spaces"), filepath.Join(root, "product with spaces"), mode)
						if strings.TrimSpace(parts[index+1]) != want {
							t.Fatalf("wrong literal checker arguments: %q", parts[index+1])
						}
					}
					matches, _ := filepath.Glob(filepath.Join(root, "diagnostic", "recheck-*", "report*.json"))
					if platform == "windows" && outcome == "malformed" {
						matches, _ = filepath.Glob(filepath.Join(root, "diagnostic", "recheck-*", "checker-stdout-private.log"))
					}
					if len(matches) != 1 {
						t.Fatalf("functional diagnostic missing or overwritten: %v", matches)
					}
					if outcome != "malformed" {
						md, _ := filepath.Glob(filepath.Join(root, "diagnostic", "recheck-*", "RELATORIO.md"))
						if len(md) != 1 {
							t.Fatal("functional Markdown report not saved")
						}
					}
				})
			}
		})
	}
}
