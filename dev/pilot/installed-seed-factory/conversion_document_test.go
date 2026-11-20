package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

// These are frontend delegation tests with a clearly synthetic argv recorder.
// They neither execute the installed owner nor qualify conversion/native hosts.
// The real rooted no-clobber owner is tested at its own command boundary.
func TestInstalledDocumentFrontendsDelegateOnlyExplicitReadyJourney(t *testing.T) {
	root := developerFixture(t)
	repo, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	stub := filepath.Join(root, "frontend-argv-fixture")
	if runtime.GOOS == "windows" {
		stub += ".exe"
	}
	program := `package main
import("encoding/json";"os")
func main(){b,_:=json.Marshal(os.Args[1:]);if err:=os.WriteFile(os.Getenv("MAESTRO_DOC_ARGV"),b,0600);err!=nil{panic(err)};state:=os.Getenv("MAESTRO_DOC_STATE");if state==""{state="fixture_prepared"};json.NewEncoder(os.Stdout).Encode(map[string]any{"schema_version":1,"state":state,"source":"maestro-pilot.docx","source_sha256":"6bd282a12170035e8af5352f2de5a21f4fe4bc5c7919283482a26703c206bf81","conversion":"not_tested","native":"not_tested"})}
`
	source := writeFixture(t, root, "argv-fixture.go", []byte(program))
	compile := exec.Command(goTool(t), "build", "-o", stub, source)
	compile.Env = append(os.Environ(), "GOOS="+runtime.GOOS, "GOARCH="+runtime.GOARCH)
	if out, err := compile.CombinedOutput(); err != nil {
		t.Fatalf("synthetic argv recorder compile: %s %v", out, err)
	}
	stubBody, err := os.ReadFile(stub)
	if err != nil {
		t.Fatal(err)
	}
	publicDoc, err := os.ReadFile(filepath.Join(repo, "internal/ingest/docling/testdata/sample.docx"))
	if err != nil {
		t.Fatal(err)
	}
	extract := func(file, start, end string) string {
		t.Helper()
		body, err := os.ReadFile(filepath.Join(repo, "dev/pilot/templates/installed-seed", file))
		if err != nil {
			t.Fatal(err)
		}
		_, tail, ok := strings.Cut(string(body), start)
		if !ok {
			t.Fatal("frontend helper missing")
		}
		fn, _, ok := strings.Cut(tail, end)
		if !ok {
			t.Fatal("frontend helper boundary missing")
		}
		return start + fn
	}
	bash, bashErr := posixBash()
	plutil, plutilErr := exec.LookPath("plutil")
	mac := runtime.GOOS == "darwin" && bashErr == nil && plutilErr == nil
	ps, psErr := powerShellTool()
	if !mac && psErr != nil {
		t.Skip("frontend interpreters unavailable; delegation/native/Windows runtime not claimed")
	}
	macFunctions := extract("Maestro.command", "guard_test_prompt() {", "open_code() {")
	psFunctions := extract("Maestro.ps1", "  function Assert-TestPrompt(", "  function Open-Code(")
	for _, test := range []struct {
		name, prompt, conversion, status, workspace, manifest, result string
		empty, called, ok                                             bool
	}{
		{name: "complete_code_ready", prompt: "CODE.txt", called: true, ok: true},
		{name: "complete_desktop_ready", prompt: "DESKTOP.txt", called: true, ok: true},
		{name: "runtime_pending_no_workspace_write", prompt: "CODE.txt", conversion: "pending_runtime", ok: true},
		{name: "blocked_report", prompt: "CODE.txt", status: "blocked"},
		{name: "empty_prompt", prompt: "CODE.txt", empty: true},
		{name: "wrong_runtime_identity", prompt: "CODE.txt", conversion: "blocked_identity_mismatch"},
		{name: "changed_workspace", prompt: "CODE.txt", workspace: "foreign"},
		{name: "changed_current_manifest", prompt: "CODE.txt", manifest: strings.Repeat("f", 64)},
		{name: "connection_prompt_is_read_only", prompt: "CODE-CONEXAO.txt"},
		{name: "readback_prompt_is_read_only", prompt: "CODE-RELEITURA.txt"},
		{name: "desktop_connection_is_read_only", prompt: "DESKTOP-CONEXAO.txt"},
		{name: "owner_result_refused", prompt: "CODE.txt", result: "unavailable", called: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			for _, platform := range []string{"mac", "powershell"} {
				if platform == "mac" && !mac || platform == "powershell" && psErr != nil {
					continue
				}
				t.Run(platform, func(t *testing.T) {
					fixture := filepath.Join(root, test.name+"-"+platform)
					run := filepath.Join(fixture, "run")
					stage := filepath.Join(fixture, "stage")
					work := filepath.Join(fixture, "work")
					for _, dir := range []string{run, filepath.Join(stage, "test-documents"), work} {
						if err := os.MkdirAll(dir, 0700); err != nil {
							t.Fatal(err)
						}
					}
					writeFixture(t, filepath.Join(stage, "test-documents"), "maestro-pilot.docx", publicDoc)
					prompt := []byte("Synthetic prompt; never delivered to a client.")
					if test.empty {
						prompt = nil
					}
					writeFixture(t, run, test.prompt, prompt)
					state := test.conversion
					if state == "" {
						state = "ready_for_participant_test"
					}
					status := test.status
					if status == "" {
						status = "partial"
					}
					boundWork := work
					if test.workspace != "" {
						boundWork = filepath.Join(fixture, test.workspace)
					}
					manifest := test.manifest
					if manifest == "" {
						manifest = strings.Repeat("a", 64)
					}
					writeJSON := func(path string, v any) {
						t.Helper()
						b, err := json.Marshal(v)
						if err != nil {
							t.Fatal(err)
						}
						if err = os.WriteFile(path, b, 0600); err != nil {
							t.Fatal(err)
						}
					}
					writeJSON(filepath.Join(run, "report.json"), map[string]any{"status": status, "conversion": map[string]string{"state": state, "expected_manifest_sha256": strings.Repeat("a", 64), "expected_pack_sha256": strings.Repeat("b", 64), "source": "maestro-pilot.docx", "source_sha256": pilotDocumentSHA256}})
					writeJSON(filepath.Join(fixture, "clients.json"), map[string]any{"state": "current_materialization", "record": map[string]any{"binding": map[string]string{"workspace": boundWork}, "materialization": map[string]string{"manifest_sha256": manifest}}})
					writeJSON(filepath.Join(fixture, "metadata.json"), map[string]any{"server_hashes": []string{sum(stubBody), strings.Repeat("f", 64)}, "identity": map[string]string{"installer_version": "0.1.51"}})
					argv := filepath.Join(fixture, "argv.json")
					env := append(os.Environ(), "MAESTRO_DOC_FIXTURE="+fixture, "MAESTRO_DOC_SERVER="+stub, "MAESTRO_DOC_ARGV="+argv, "MAESTRO_DOC_PROMPT="+test.prompt, "MAESTRO_DOC_STATE="+test.result, "MAESTRO_DOC_PLUTIL="+plutil)
					var command *exec.Cmd
					if platform == "mac" {
						body := `set -euo pipefail
RUN="$MAESTRO_DOC_FIXTURE/run";STAGE="$MAESTRO_DOC_FIXTURE/stage";WORK="$MAESTRO_DOC_FIXTURE/work";META="$MAESTRO_DOC_FIXTURE/metadata.json";SERVER="$MAESTRO_DOC_SERVER";PRODUCT="$MAESTRO_DOC_FIXTURE/product";ROLE=cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc;VERSION=0.1.51
COMMON=(--product-root "$PRODUCT" --user-home "$MAESTRO_DOC_FIXTURE/owner" --workspace "$WORK" --carrier-id dddddddddddddddddddddddddddddddd)
jget(){ "$MAESTRO_DOC_PLUTIL" -extract "$2" raw -o - "$1"; }
sha(){ local value;value="$(/usr/bin/shasum -a 256 "$1")";printf '%s' "${value%% *}"; }
fail(){ printf '%s\n' "$*" >&2;exit 1; }
` + macFunctions + `prepare_conversion_document "$MAESTRO_DOC_FIXTURE/clients.json" "$MAESTRO_DOC_PROMPT"`
						command = exec.Command(bash, "-c", body)
					} else {
						body := `$ErrorActionPreference='Stop';Set-StrictMode -Version Latest
$Run=Join-Path $env:MAESTRO_DOC_FIXTURE 'run';$Stage=Join-Path $env:MAESTRO_DOC_FIXTURE 'stage';$Config=[PSCustomObject]@{workspace=(Join-Path $env:MAESTRO_DOC_FIXTURE 'work')};$Checked=Get-Content -LiteralPath (Join-Path $Run 'report.json') -Raw|ConvertFrom-Json;$ClientStatus=Get-Content -LiteralPath (Join-Path $env:MAESTRO_DOC_FIXTURE 'clients.json') -Raw|ConvertFrom-Json;$Meta=Get-Content -LiteralPath (Join-Path $env:MAESTRO_DOC_FIXTURE 'metadata.json') -Raw|ConvertFrom-Json;$Server=$env:MAESTRO_DOC_SERVER;$Product=Join-Path $env:MAESTRO_DOC_FIXTURE 'product';$Role='cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc';$Common=@('--product-root',$Product,'--user-home',(Join-Path $env:MAESTRO_DOC_FIXTURE 'owner'),'--workspace',$Config.workspace,'--carrier-id','dddddddddddddddddddddddddddddddd')
function Stop-Maestro([string]$Message){throw $Message}
function Private-File([string]$Path){$f=Get-Item -LiteralPath $Path -Force;if($f.PSIsContainer -or ($f.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0){throw 'not regular'};return $f}
function Hash([string]$Path){(Get-FileHash -LiteralPath $Path -Algorithm SHA256).Hash.ToLowerInvariant()}
function Write-Json([string]$Path,$Value){[IO.File]::WriteAllText($Path,($Value|ConvertTo-Json -Depth 10))}
` + psFunctions + `Prepare-ConversionDocument $ClientStatus $env:MAESTRO_DOC_PROMPT`
						command = exec.Command(ps, "-NoLogo", "-NoProfile", "-NonInteractive", "-Command", body)
					}
					command.Env = env
					out, err := command.CombinedOutput()
					if (err == nil) != test.ok {
						t.Fatalf("delegation ok=%v wanted %v: %s %v", err == nil, test.ok, out, err)
					}
					actual, err := os.ReadFile(argv)
					if test.called {
						if err != nil {
							t.Fatal(err)
						}
						var got []string
						if err = json.Unmarshal(actual, &got); err != nil {
							t.Fatal(err)
						}
						want := []string{"installed-pilot-prepare-document", "--product-root", filepath.Join(fixture, "product"), "--user-home", filepath.Join(fixture, "owner"), "--workspace", work, "--carrier-id", strings.Repeat("d", 32), "--installer-version", "0.1.51", "--experimental-seed-sha256", strings.Repeat("c", 64), "--experimental-installed-root", filepath.Join(fixture, "product")}
						if !reflect.DeepEqual(got, want) {
							t.Fatalf("wrong fixed helper argv: %#v", got)
						}
					} else if !os.IsNotExist(err) {
						t.Fatalf("read-only/blocked/pending path called document helper: %s %v", actual, err)
					}
					if _, err = os.Lstat(filepath.Join(work, "maestro-pilot.docx")); !os.IsNotExist(err) {
						t.Fatal("frontend wrote workspace instead of delegating to the real rooted owner")
					}
				})
			}
		})
	}
}
