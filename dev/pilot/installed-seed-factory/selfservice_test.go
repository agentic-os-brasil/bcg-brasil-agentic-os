package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installtx"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// Exercise the original selector without starting a client, installer or clipboard.
// Navigation is a human choice: sending a step never advances or replays it.
func TestParticipantStepDeliveryHasRealRequestBoundaries(t *testing.T) {
	for _, platform := range []string{"mac", "windows"} {
		t.Run(platform, func(t *testing.T) {
			dir := developerFixture(t)
			steps := []map[string]string{
				{"id": "reconcile", "title": "Conferir ou continuar", "prompt": "READ-ONLY-CURRENT"},
				{"id": "memory-accept", "title": "Salvar a memória", "prompt": "ONLY-MEMORY-CURRENT"},
				{"id": "conversion", "title": "Conferir documento", "prompt": "ONLY-CONVERSION-CURRENT"},
			}
			fixture, err := json.Marshal(map[string]any{"desktop_steps": steps})
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "report.json"), fixture, 0600); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			var cmd *exec.Cmd
			if platform == "mac" {
				if runtime.GOOS != "darwin" {
					t.Skip("Mac Bash/plutil contract")
				}
				source, err := os.ReadFile("../templates/installed-seed/Maestro.command")
				if err != nil {
					t.Fatal(err)
				}
				_, body, ok := strings.Cut(string(source), "pilot_steps() {")
				if !ok {
					t.Fatal("original selector missing")
				}
				body, _, ok = strings.Cut(body, "while [[ -t 0 ]]; do")
				if !ok {
					t.Fatal("original menu boundary missing")
				}
				cmd = exec.CommandContext(ctx, "/bin/bash", "-c", `set -euo pipefail
jget(){ /usr/bin/plutil -extract "$2" raw -o - "$1"; }
fail(){ printf '%s' "$*" >&2; exit 1; }
count=0
open_desktop(){ count=$((count+1)); /bin/cp "$RUN/$1" "$RUN/delivery-$count.txt"; printf '%s\n' "$1" >> "$RUN/files.txt"; }
pilot_steps() {`+body+`pilot_steps desktop`)
				cmd.Env = append(os.Environ(), "RUN="+dir)
				// Send, choose next, send, return to reconciliation, send,
				// choose the document step and send. No automatic advancement.
				cmd.Stdin = strings.NewReader("1\n2\n1\n4\n1\n2\n2\n1\n\n")
			} else {
				powerShell, err := powerShellTool()
				if err != nil {
					t.Skip("PowerShell runtime unavailable; Windows remains unqualified")
				}
				source, err := os.ReadFile("../templates/installed-seed/Maestro.ps1")
				if err != nil {
					t.Fatal(err)
				}
				_, body, ok := strings.Cut(string(source), "  function Invoke-PilotSteps([string]$Client) {")
				if !ok {
					t.Fatal("original selector missing")
				}
				body, _, ok = strings.Cut(body, "\n  do {")
				if !ok {
					t.Fatal("original menu boundary missing")
				}
				cmd = exec.CommandContext(ctx, powerShell, "-NoLogo", "-NoProfile", "-Command", `Set-StrictMode -Version Latest; $ErrorActionPreference='Stop'
$Run=$env:MAESTRO_STEP_FIXTURE
$Utf8=New-Object Text.UTF8Encoding($false)
$Checked=Get-Content -LiteralPath (Join-Path $Run 'report.json') -Raw|ConvertFrom-Json
$script:answers=[Collections.Generic.Queue[string]]::new()
@('1','2','1','4','1','2','2','1','')|ForEach-Object{$script:answers.Enqueue($_)}
$script:count=0
function Read-Host([string]$Prompt){if($script:answers.Count -eq 0){throw 'Unexpected question'};return $script:answers.Dequeue()}
function Stop-Maestro([string]$Message){throw $Message}
function Open-Desktop([string]$File){$script:count++;[IO.File]::WriteAllText((Join-Path $Run ('delivery-'+$script:count+'.txt')),[IO.File]::ReadAllText((Join-Path $Run $File)),$Utf8);[IO.File]::AppendAllText((Join-Path $Run 'files.txt'),$File+"`+"`n"+`",$Utf8)}
function Invoke-PilotSteps([string]$Client) {`+body+`Invoke-PilotSteps 'desktop'`)
				cmd.Env = append(os.Environ(), "MAESTRO_STEP_FIXTURE="+dir)
				if dev := os.Getenv("GOCACHE"); strings.Contains(dev, "/Developer/") {
					base := filepath.Join(strings.SplitN(dev, "/Developer/", 2)[0], "Developer", "ToolData", "powershell-maestro")
					cmd.Env = append(cmd.Env, "XDG_CACHE_HOME="+filepath.Join(base, "cache"), "XDG_CONFIG_HOME="+filepath.Join(base, "config"), "XDG_DATA_HOME="+filepath.Join(base, "data"))
				}
			}
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("original selector: %s %v", output, err)
			}
			for index, want := range []string{"READ-ONLY-CURRENT", "ONLY-MEMORY-CURRENT", "READ-ONLY-CURRENT", "ONLY-CONVERSION-CURRENT"} {
				body, err := os.ReadFile(filepath.Join(dir, fmt.Sprintf("delivery-%d.txt", index+1)))
				if err != nil || string(body) != want {
					t.Fatalf("delivery %d crossed a turn boundary: %q %v", index+1, body, err)
				}
			}
			files, err := os.ReadFile(filepath.Join(dir, "files.txt"))
			if err != nil || strings.Join(strings.Fields(string(files)), ",") != "DESKTOP-ETAPA.txt,DESKTOP-ETAPA.txt,DESKTOP-ETAPA.txt,DESKTOP-CONVERSAO.txt" {
				t.Fatalf("document preparation leaked into another step: %s %v", files, err)
			}
		})
	}
}

// Keep the actual selector and Desktop helpers together under the launcher's
// errexit policy. Only absolute GUI calls in this test copy use synthetic sinks.
func TestParticipantMacDesktopOptionalGUIFailuresKeepCurrentStep(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Mac Bash/plutil contract; no client qualification")
	}
	source, err := os.ReadFile("../templates/installed-seed/Maestro.command")
	if err != nil {
		t.Fatal(err)
	}
	extract := func(start, end string) string {
		t.Helper()
		_, body, ok := strings.Cut(string(source), start)
		if !ok {
			t.Fatalf("original helper missing: %s", start)
		}
		body, _, ok = strings.Cut(body, end)
		if !ok {
			t.Fatalf("original helper boundary missing: %s", end)
		}
		return start + body
	}
	helpers := extract("guard_test_prompt() {", "prepare_conversion_document() {") +
		extract("desktop_clients_available() {", "current_clients=false") +
		extract("desktop_install_is_current() {", "pilot_steps() {") +
		extract("pilot_steps() {", "while [[ -t 0 ]]; do")
	for _, command := range []string{"/usr/bin/pbcopy", "/usr/bin/open"} {
		if strings.Count(helpers, command) != 1 {
			t.Fatalf("expected one real GUI call to replace: %s", command)
		}
	}
	helpers = strings.ReplaceAll(helpers, "/usr/bin/pbcopy", "test_clipboard")
	helpers = strings.ReplaceAll(helpers, "/usr/bin/open", "test_open")
	for _, test := range []struct {
		name                     string
		clipboardExit, openExit  int
		confirmed, blocked       bool
		clientRefused, noDesktop bool
		emptyPrompt              bool
	}{
		{name: "clipboard_failure", clipboardExit: 1},
		{name: "package_open_failure", openExit: 1},
		{name: "both_fail", clipboardExit: 1, openExit: 1},
		{name: "success"},
		{name: "confirmed_clipboard_failure", clipboardExit: 1, confirmed: true},
		{name: "diagnostic_blocked", blocked: true},
		{name: "client_status_refused", clientRefused: true},
		{name: "desktop_unavailable", noDesktop: true},
		{name: "empty_prompt", emptyPrompt: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir := developerFixture(t)
			prompt := "Preferência fictícia: conferir a memória — uma etapa por vez.\nNão avançar automaticamente."
			if test.emptyPrompt {
				prompt = ""
			}
			status := "partial"
			if test.blocked {
				status = "blocked"
			}
			report, err := json.Marshal(map[string]any{"status": status, "desktop_steps": []map[string]string{
				{"id": "memory-accept", "title": "Salvar e conferir a memória", "prompt": prompt},
				{"id": "readback", "title": "PRÓXIMA-ETAPA-NÃO-SELECIONADA", "prompt": "DO-NOT-SEND"},
			}})
			if err != nil {
				t.Fatal(err)
			}
			packageHash, contractHash := strings.Repeat("a", 64), strings.Repeat("b", 64)
			desktop := map[string]any{"install_confirmation": "awaiting_user_install"}
			if test.confirmed {
				desktop = map[string]any{"install_confirmation": "user_confirmed_install", "evidence_class": "operator_attested", "package_sha256": packageHash, "native_contract_sha256": contractHash}
			}
			state := "current_materialization"
			if test.noDesktop {
				state = "unavailable"
			}
			packet, methods := filepath.Join(dir, "Pacote Desktop.mcpb"), filepath.Join(dir, "Métodos Desktop.zip")
			clientStatus, err := json.Marshal(map[string]any{"state": state, "record": map[string]any{"desktop": desktop, "materialization": map[string]any{
				"desktop_path": packet, "desktop_sha256": packageHash, "desktop_native_contract_sha256": contractHash,
				"desktop_methods": map[string]any{"archive": methods, "version": "fixture-v1"},
			}}})
			if err != nil {
				t.Fatal(err)
			}
			markdown := "# Relatório preservado\n\nDesktop: pending; client loading: not_observed.\n"
			for file, body := range map[string][]byte{
				"report.json": report, "RELATORIO.md": []byte(markdown), "status-fixture.json": clientStatus,
				"installed-fixture": []byte("#!/bin/bash\n[[ \"$1\" == clients-status ]] || exit 99\n[[ \"$TEST_CLIENT_REFUSED\" == false ]] || exit 1\n/bin/cat \"$RUN/status-fixture.json\"\n"),
			} {
				if err := os.WriteFile(filepath.Join(dir, file), body, 0700); err != nil {
					t.Fatal(err)
				}
			}
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, "/bin/bash", "-c", `set -euo pipefail
jget(){ /usr/bin/plutil -extract "$2" raw -o - "$1"; }
fail(){ printf '%s\n' "$*" >&2; exit 1; }
INSTALLED="$RUN/installed-fixture"
CLIENT_STATUS=(--fixture)
test_clipboard(){ /bin/cat > "$RUN/clipboard-attempt.txt"; printf 'clipboard\n' >> "$RUN/gui-calls.txt"; return "$TEST_CLIPBOARD_EXIT"; }
test_open(){ printf '%s\n' "$1" > "$RUN/open-attempt.txt"; printf 'open\n' >> "$RUN/gui-calls.txt"; return "$TEST_OPEN_EXIT"; }
`+helpers+`pilot_steps desktop
printf 'menu-returned' > "$RUN/menu-returned.txt"`)
			cmd.Env = append(os.Environ(), "RUN="+dir, fmt.Sprintf("TEST_CLIPBOARD_EXIT=%d", test.clipboardExit), fmt.Sprintf("TEST_OPEN_EXIT=%d", test.openExit), fmt.Sprintf("TEST_CLIENT_REFUSED=%t", test.clientRefused))
			cmd.Stdin = strings.NewReader("1\n\n")
			output, runErr := cmd.CombinedOutput()
			refused := test.blocked || test.clientRefused || test.noDesktop || test.emptyPrompt
			if refused {
				if runErr == nil {
					t.Fatalf("admission refusal became recoverable: %s", output)
				}
				if _, err := os.Stat(filepath.Join(dir, "gui-calls.txt")); !os.IsNotExist(err) {
					t.Fatalf("refused step reached GUI commands: %v", err)
				}
			} else {
				if runErr != nil {
					t.Fatalf("optional GUI error killed current menu: %s %v", output, runErr)
				}
				assertFile := func(file, want string) {
					t.Helper()
					got, err := os.ReadFile(filepath.Join(dir, file))
					if err != nil || string(got) != want {
						t.Fatalf("%s changed: %q %v; want %q", file, got, err, want)
					}
				}
				assertFile("menu-returned.txt", "menu-returned")
				assertFile("clipboard-attempt.txt", prompt)
				assertFile("clients-status.json", string(clientStatus))
				calls := "clipboard\nopen\n"
				if test.confirmed {
					calls = "clipboard\n"
					if _, err := os.Stat(filepath.Join(dir, "open-attempt.txt")); !os.IsNotExist(err) {
						t.Fatalf("confirmed current package was reopened: %v", err)
					}
				} else {
					assertFile("open-attempt.txt", packet+"\n")
				}
				assertFile("gui-calls.txt", calls)
				text := string(output)
				if strings.Count(text, "Etapa atual: Salvar e conferir a memória") != 2 || strings.Contains(text, "PRÓXIMA-ETAPA-NÃO-SELECIONADA") {
					t.Fatalf("step advanced or retried automatically: %s", output)
				}
				for _, path := range []string{filepath.Join(dir, "DESKTOP-ETAPA.txt"), filepath.Join(dir, "RELATORIO.md"), packet, methods} {
					if !strings.Contains(text, path) {
						t.Errorf("manual recovery path missing: %s\n%s", path, output)
					}
				}
				if strings.Contains(text, "Prompt copiado.") != (test.clipboardExit == 0) || strings.Contains(text, "Cópia do prompt pendente.") != (test.clipboardExit != 0) {
					t.Errorf("clipboard outcome misreported: %s", output)
				}
				if strings.Contains(text, "Abertura do pacote pendente.") != (!test.confirmed && test.openExit != 0) {
					t.Errorf("package opening outcome misreported: %s", output)
				}
				if !strings.Contains(text, "A importação no Desktop não foi observada") || !strings.Contains(text, "A conexão e o funcionamento continuam pendentes de teste") {
					t.Errorf("host observations no longer pending: %s", output)
				}
			}
			for file, want := range map[string]string{"report.json": string(report), "RELATORIO.md": markdown, "DESKTOP-ETAPA.txt": prompt} {
				got, err := os.ReadFile(filepath.Join(dir, file))
				if err != nil || string(got) != want {
					t.Errorf("retained artifact %s changed: %q %v", file, got, err)
				}
			}
		})
	}
}

// Exercise the exact original prompt-consumer statements, not a copied decoder.
// Windows requires Desktop 5.1; alternate defaults also expose the BOM-less
// ANSI-reader failure on PowerShell 7. Synthetic sinks do not qualify clients.
func TestParticipantPowerShellPromptConsumersPreserveUTF8(t *testing.T) {
	shell := "pwsh"
	if runtime.GOOS == "windows" {
		shell = "powershell.exe"
	}
	powerShell, err := exec.LookPath(shell)
	if err != nil {
		if runtime.GOOS == "windows" {
			t.Fatalf("shipped Windows PowerShell 5.1 is required: %v", err)
		}
		t.Skip("PowerShell runtime unavailable; native Windows remains unqualified")
	}
	dir := developerFixture(t)
	text := "Preferência inteiramente fictícia: conferir o resultado salvo antes da entrega. MAESTRO_PILOT_0123456789abcdef0123456789abcdef_MEMORY_ACCEPT\nUma etapa por vez — revisão e confirmação."
	steps := []map[string]string{{"id": "memory-accept", "title": "Salvar e conferir a memória de teste", "prompt": text}}
	fixture, err := json.Marshal(map[string]any{"status": "partial", "code_steps": steps, "desktop_steps": steps})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "report.json"), fixture, 0600); err != nil {
		t.Fatal(err)
	}
	source, err := os.ReadFile("../templates/installed-seed/Maestro.ps1")
	if err != nil {
		t.Fatal(err)
	}
	extract := func(start, end string) string {
		t.Helper()
		_, body, ok := strings.Cut(string(source), start)
		if !ok {
			t.Fatalf("original helper missing: %s", start)
		}
		body, _, ok = strings.Cut(body, end)
		if !ok {
			t.Fatalf("original helper boundary missing: %s", end)
		}
		return start + body
	}
	consumer := func(body, call string) string {
		t.Helper()
		for _, line := range strings.Split(body, "\n") {
			if strings.Contains(line, call) {
				return line
			}
		}
		t.Fatalf("original consumer missing: %s", call)
		return ""
	}
	code := consumer(extract("  function Open-Code([string]$PromptFile) {", "  function Test-CurrentDesktopInstall("), "Invoke-SelectedCode -Arguments")
	desktop := consumer(extract("  function Open-Desktop([string]$PromptFile) {", "  function Invoke-PilotSteps("), "Set-Clipboard -Value")
	assert := extract("  function Assert-TestPrompt([string]$PromptFile) {", "  function Prepare-ConversionDocument(")
	selector := extract("  function Invoke-PilotSteps([string]$Client) {", "\n  do {")
	stop := extract("function Stop-Maestro([string]$Message) {", "function Hash(")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, powerShell, "-NoLogo", "-NoProfile", "-Command", `Set-StrictMode -Version Latest; $ErrorActionPreference='Stop'
if([Environment]::OSVersion.Platform -eq 'Win32NT' -and ($PSVersionTable.PSEdition -cne 'Desktop' -or $PSVersionTable.PSVersion.Major -ne 5 -or $PSVersionTable.PSVersion.Minor -ne 1)){throw 'Shipped prompt readers require Windows PowerShell Desktop 5.1'}
$Run=$env:MAESTRO_UTF8_FIXTURE
$Utf8=New-Object Text.UTF8Encoding($false)
$Checked=Get-Content -LiteralPath (Join-Path $Run 'report.json') -Raw -Encoding UTF8|ConvertFrom-Json
`+stop+assert+selector+`
function Invoke-SelectedCode([string[]]$Arguments){if($Arguments.Count -ne 1){throw 'Prompt must remain one literal argument'};[IO.File]::WriteAllText((Join-Path $Run 'code-delivery.txt'),$Arguments[0],$Utf8)}
function Set-Clipboard([string]$Value){[IO.File]::WriteAllText((Join-Path $Run 'desktop-delivery.txt'),$Value,$Utf8)}
function Open-Code([string]$PromptFile){Assert-TestPrompt $PromptFile
`+code+`
}
function Open-Desktop([string]$PromptFile){Assert-TestPrompt $PromptFile
`+desktop+`
}
$script:answers=[Collections.Generic.Queue[string]]::new()
function Read-Host([string]$Prompt){if($script:answers.Count -eq 0){throw 'Unexpected question'};return $script:answers.Dequeue()}
if($PSVersionTable.PSVersion.Major -ge 6){$defaultEncoding=1252}else{$defaultEncoding='Default'}
$PSDefaultParameterValues['Get-Content:Encoding']=$defaultEncoding
foreach($client in @('code','desktop')){
  $script:answers.Enqueue('1');$script:answers.Enqueue('')
  Invoke-PilotSteps $client
  $path=Join-Path $Run ($client.ToUpperInvariant()+'-ETAPA.txt')
  $actual=[IO.File]::ReadAllBytes($path)
  if($actual.Length -ge 3 -and $actual[0] -eq 239 -and $actual[1] -eq 187 -and $actual[2] -eq 191){throw 'Selector unexpectedly wrote a BOM'}
  $control=Get-Content -LiteralPath $path -Raw
  if($PSVersionTable.PSVersion.Major -ge 6 -and $control -ceq $Checked.code_steps[0].prompt){throw 'Alternate-default control failed to expose ANSI corruption'}
}
Write-Host ('Prompt reader shell: '+$PSVersionTable.PSEdition+' '+$PSVersionTable.PSVersion+'; decoding default '+$defaultEncoding+'; synthetic client/clipboard sinks only; no client qualification')`)
	cmd.Env = make([]string, 0)
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		// Match Maestro.cmd: Desktop 5.1 selects its own modules even when
		// hosted by the workflow's PowerShell 7 process.
		if runtime.GOOS != "windows" || !strings.EqualFold(name, "PSModulePath") {
			cmd.Env = append(cmd.Env, entry)
		}
	}
	cmd.Env = append(cmd.Env, "MAESTRO_UTF8_FIXTURE="+dir)
	if dev := os.Getenv("GOCACHE"); strings.Contains(dev, "/Developer/") {
		base := filepath.Join(strings.SplitN(dev, "/Developer/", 2)[0], "Developer", "ToolData", "powershell-maestro")
		cmd.Env = append(cmd.Env, "XDG_CACHE_HOME="+filepath.Join(base, "cache"), "XDG_CONFIG_HOME="+filepath.Join(base, "config"), "XDG_DATA_HOME="+filepath.Join(base, "data"))
	}
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("original PowerShell consumers: %s %v", output, err)
	}
	t.Logf("%s", output)
	for _, client := range []string{"code", "desktop"} {
		body, err := os.ReadFile(filepath.Join(dir, client+"-delivery.txt"))
		if err != nil || string(body) != text {
			t.Fatalf("%s original consumer changed UTF-8 bytes: got %q, want %q, error %v", client, body, text, err)
		}
		t.Logf("%s original consumer preserved all %d UTF-8 bytes", client, len(body))
	}
}

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
CLIENT_STATUS=("${COMMON[@]}")
TARGET=""
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
` + extract("json() {", "# Resolve normal installer symlinks") + extract("review_confirm() {", "if [[ ! -e \"$INSTALLED\"") + extract("desktop_clients_available() {", "current_clients=false") + producer
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

func TestMacSelfServiceDesktopSelectionAvoidsCode(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Mac shell/filesystem contract")
	}
	root := developerFixture(t)
	source, err := os.ReadFile("../templates/installed-seed/Maestro.command")
	if err != nil {
		t.Fatal(err)
	}
	extract := func(start, end string) string {
		_, tail, ok := strings.Cut(string(source), start)
		if !ok {
			t.Fatalf("missing shipped boundary %q", start)
		}
		body, _, ok := strings.Cut(tail, end)
		if !ok {
			t.Fatalf("missing shipped end %q", end)
		}
		return start + body
	}
	dev := filepath.Join(root, "Developer")
	owner := filepath.Join(dev, "existing owner")
	base := filepath.Join(dev, "pilot")
	for _, p := range []string{owner, base} {
		if err := os.MkdirAll(p, 0700); err != nil {
			t.Fatal(err)
		}
	}
	installer := filepath.Join(base, "installer-fixture")
	if err := os.WriteFile(installer, []byte("#!/bin/bash\nprintf '%s\\n' \"$@\" > \"$OBSERVED_ARGS\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	script := `set -euo pipefail
umask 077
fail(){ printf '%s' "$*" >&2; exit 1; }
jget(){ /usr/bin/plutil -extract "$2" raw -o - "$1"; }
private_dir(){ [[ -d "$1" && ! -L "$1" ]] || fail 'Missing fixture private directory'; }
prepare_client_parent(){ mkdir -p "$OWNER/Maestro-Clients/$ROLE"; printf '%s' "$OWNER/Maestro-Clients/$ROLE"; }
resolve_code_executable(){ fail 'CODE DISCOVERY MUST NOT RUN'; }
prepare_code_configuration(){ fail 'CODE PROFILE MUST NOT RUN'; }
ensure_code_login(){ fail 'CODE LOGIN MUST NOT RUN'; }
verify_installer(){ :; }
COMMON=(--fixture)
` + extract("json() {", "# Resolve normal installer symlinks") + "\n" + extract("if [[ ! -e \"$CFG\" ]]; then", "PRODUCT=\"$BASE/product\"") + "\n" + extract("manage() {", "current_clients=false") + "\nmanage\n" + `
if [[ ! -e "$BASE/observed-review.json" ]]; then
  PRODUCT="$BASE/product"; INSTALLED="$BASE/not-installed"; RUN="$BASE/setup-run"; STAGE="$BASE/stage"
  mkdir "$RUN"
  fixture_bootstrap(){
    case "$1" in
      review) cat > "$BASE/observed-review.json"; printf '%s' '{"review_digest":"reviewed-desktop"}';;
      apply) cat > "$BASE/observed-apply.json"; [[ "$2" == --review-digest && "$3" == reviewed-desktop && "$4" == --confirm ]] || return 97; printf '{}';;
      *) return 98;;
    esac
  }
  BOOTSTRAP=fixture_bootstrap
  review_confirm(){ :; }
` + extract("if [[ ! -e \"$INSTALLED\" ]]; then", "verify_installer \"$INSTALLED\"\nmanage() {") + "\nfi\n"
	cfg := filepath.Join(base, "participant.json")
	argsPath := filepath.Join(base, "args")
	run := func(input string) ([]byte, error) {
		cmd := exec.Command("/bin/bash", "-c", script)
		cmd.Env = append(os.Environ(), "DEV="+dev, "BASE="+base, "ROLE=desktop-fixture", "CFG="+cfg, "MAESTRO_HOME="+filepath.Join(owner, "brain"), "INSTALLED="+installer, "OBSERVED_ARGS="+argsPath)
		cmd.Stdin = strings.NewReader(input)
		return cmd.CombinedOutput()
	}
	if output, err := run("desktop\n" + owner + "\n" + filepath.Join(dev, "workspace") + "\n"); err != nil {
		t.Fatalf("explicit Desktop setup must not discover, prepare or authenticate Code: %s %v", output, err)
	}
	body, err := os.ReadFile(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var config map[string]any
	if err := json.Unmarshal(body, &config); err != nil || config["client_target"] != "desktop" {
		t.Fatalf("explicit target not retained: %s %v", body, err)
	}
	for _, key := range []string{"code_executable", "code_configuration_directory"} {
		if _, present := config[key]; present {
			t.Fatalf("Desktop setup created Code placeholder %s", key)
		}
	}
	if output, err := run(""); err != nil {
		t.Fatalf("same Desktop selection must resume without Code prompts: %s %v", output, err)
	}
	args, err := os.ReadFile(argsPath)
	if err != nil || !strings.Contains(string(args), "--client-target\ndesktop\n") || !strings.Contains(string(args), "--client-desktop-path\n"+filepath.Join(owner, "Maestro-Clients", "desktop-fixture", "maestro-product.mcpb")+"\n") {
		t.Fatalf("original manager lost explicit Desktop selection: %s %v", args, err)
	}
	if strings.Contains(string(args), "--client-code") || strings.Contains(string(args), "--client-scope") {
		t.Fatalf("Desktop manager received Code inputs: %s", args)
	}
	for _, name := range []string{"observed-review.json", "observed-apply.json"} {
		body, err := os.ReadFile(filepath.Join(base, name))
		var request map[string]any
		if err != nil || json.Unmarshal(body, &request) != nil || request["target"] != "desktop" {
			t.Fatalf("original bootstrap lost reviewed Desktop target: %s %v", body, err)
		}
		for _, key := range []string{"code_packet", "desktop_packet"} {
			if _, present := request[key]; present {
				t.Fatalf("Desktop bootstrap emitted portable placeholder %s", key)
			}
		}
	}
	original, err := os.ReadFile(cfg)
	if err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"unknown-target", "hidden-code"} {
		var modified map[string]any
		if err := json.Unmarshal(original, &modified); err != nil {
			t.Fatal(err)
		}
		if scenario == "unknown-target" {
			modified["client_target"] = "resources-only"
		} else {
			modified["code_executable"] = installer
		}
		body, _ := json.Marshal(modified)
		if err := os.WriteFile(cfg, body, 0600); err != nil {
			t.Fatal(err)
		}
		if output, err := run(""); err == nil {
			t.Fatalf("%s was accepted: %s", scenario, output)
		}
		preserved, _ := os.ReadFile(cfg)
		if string(preserved) != string(body) {
			t.Fatalf("%s rewrote saved choices", scenario)
		}
	}
}

// Exercise only the shipped result loops: no installer, client or GUI runs.
func TestParticipantResultCollectorsRespectDesktopTarget(t *testing.T) {
	root := developerFixture(t)
	for _, shell := range []string{"mac", "powershell"} {
		for _, target := range []string{"both", "desktop"} {
			t.Run(shell+"/"+target, func(t *testing.T) {
				dir := filepath.Join(root, shell+"-"+target)
				if err := os.Mkdir(dir, 0700); err != nil {
					t.Fatal(err)
				}
				report := filepath.Join(dir, "RELATORIO.md")
				if err := os.WriteFile(report, nil, 0600); err != nil {
					t.Fatal(err)
				}
				questions := 6
				if target == "desktop" {
					questions = 4
				}
				var cmd *exec.Cmd
				if shell == "mac" {
					if runtime.GOOS == "windows" {
						t.Skip("shipped Mac Bash collector")
					}
					source, err := os.ReadFile("../templates/installed-seed/Maestro.command")
					if err != nil {
						t.Fatal(err)
					}
					_, body, found := strings.Cut(string(source), "attest() {")
					if !found {
						t.Fatal("missing shipped result collector")
					}
					body, _, found = strings.Cut(body, "guard_test_prompt() {")
					if !found {
						t.Fatal("missing collector boundary")
					}
					cmd = exec.Command("/bin/bash", "-c", "set -euo pipefail\nattest() {"+body+"\nattest\n")
					selected := target
					if selected == "both" {
						selected = ""
					}
					cmd.Env = append(os.Environ(), "TARGET="+selected, "RUN="+dir)
					cmd.Stdin = strings.NewReader(strings.Repeat("passou\n", questions))
				} else {
					name := "pwsh"
					if runtime.GOOS == "windows" {
						name = "powershell.exe"
					}
					powerShell, err := exec.LookPath(name)
					if err != nil {
						if runtime.GOOS == "windows" {
							t.Fatal(err)
						}
						t.Skip("PowerShell required")
					}
					path := filepath.Join(dir, "collector.ps1")
					if err := os.WriteFile(path, []byte(participantResultCollectorPowerShell), 0600); err != nil {
						t.Fatal(err)
					}
					source, err := filepath.Abs("../templates/installed-seed/Maestro.ps1")
					if err != nil {
						t.Fatal(err)
					}
					cmd = exec.Command(powerShell, "-NoProfile", "-NonInteractive", "-File", path, "-Source", source, "-Report", report, "-ClientTarget", target)
				}
				output, err := cmd.CombinedOutput()
				if err != nil {
					t.Fatalf("shipped collector: %s %v", output, err)
				}
				body, err := os.ReadFile(report)
				if err != nil {
					t.Fatal(err)
				}
				if strings.Count(string(body), "(participante)") != questions {
					t.Fatalf("wrong relevant result count: %s", body)
				}
				if target == "desktop" {
					if strings.Count(string(body), "not_applicable (alvo Desktop)") != 2 {
						t.Fatalf("Code/hooks applicability not recorded: %s", body)
					}
					if strings.Contains(string(output), "Code: identidade/skills") || strings.Contains(string(output), "Hooks e delega") {
						t.Fatalf("Desktop user was asked Code/hook questions: %s", output)
					}
				} else if strings.Contains(string(body), "not_applicable") || !strings.Contains(string(output), "Code: identidade/skills") || !strings.Contains(string(output), "Hooks e delega") {
					t.Fatalf("legacy both-client questions changed: %s\n%s", output, body)
				}
			})
		}
	}
}

// Observe the real frontend's command boundary, substituting only authentication
// and the canonical installed owner. No Code, model, installer or client runs.
func TestParticipantCodeAcceptanceUsesFreshCanonicalPreparation(t *testing.T) {
	partial := `{"state":"partial_preparation","preparation":{"phase":"desktop_prepared_code_pending","selected_target":"both","desktop":"prepared","code":"pending_qualification"}}`
	for _, platform := range []string{"mac", "powershell"} {
		for _, test := range []struct{ name, status, intent string }{
			{"initial", `{"state":"unavailable"}`, ""},
			{"full", `{"state":"current_materialization"}`, ""},
			{"partial", partial, "resume-code"},
			{"partial_update", partial, "update"},
		} {
			t.Run(platform+"/"+test.name, func(t *testing.T) {
				root := developerFixture(t)
				writeFixture(t, root, "canonical-status.json", []byte(test.status))
				var cmd *exec.Cmd
				if platform == "mac" {
					if runtime.GOOS != "darwin" {
						t.Skip("Mac shell contract")
					}
					source, err := os.ReadFile("../templates/installed-seed/Maestro.command")
					if err != nil {
						t.Fatal(err)
					}
					_, helper, _ := strings.Cut(string(source), "manage() {")
					helper, _, _ = strings.Cut(helper, "CLIENT_STATUS=(")
					_, predicate, _ := strings.Cut(string(source), "desktop_clients_available() {")
					predicate, _, _ = strings.Cut(predicate, "current_clients=false")
					observer := writeFixture(t, root, "installer", []byte(`#!/bin/bash
if [[ "$1" == clients-status ]]; then printf checked >> "$ROOT/status-calls"; cat "$ROOT/canonical-status.json"; exit; fi
[[ "$1" == manage ]] || exit 21
printf '%s\n' "$@" > "$ROOT/manager-args"
printf '%s' "$CLAUDE_CONFIG_DIR" > "$ROOT/manager-profile"
`))
					if err := os.Chmod(observer, 0700); err != nil {
						t.Fatal(err)
					}
					script := `set -euo pipefail
fail(){ printf '%s' "$*" >&2; exit 22; }
verify_installer(){ :; }
ensure_code_login(){ printf auth >> "$ROOT/auth"; }
jget(){ /usr/bin/plutil -extract "$2" raw -o - "$1"; }
TARGET=""; RUN="$ROOT"; CODE="$ROOT/code"; CODE_CONFIG="$ROOT/selected-profile"; CLIENTS="$ROOT/clients"; INSTALLED="$ROOT/installer"; CODE_PREPARATION_PENDING=true
COMMON=(--workspace "$ROOT/brain" --carrier-id original-carrier); CLIENT_STATUS=("${COMMON[@]}")
desktop_clients_available() {` + predicate + `
manage() {` + helper + `
if [[ "$CASE" == partial_update ]]; then manage --management-intent update; else manage; fi
[[ "${CLAUDE_CONFIG_DIR}" == caller-profile ]] || fail 'caller profile changed'
`
					cmd = exec.Command("/bin/bash", "-c", script)
					cmd.Stdin = strings.NewReader("YES\n")
				} else {
					shell, err := powerShellTool()
					if err != nil {
						t.Skip("PowerShell helper contract unavailable")
					}
					source, _ := filepath.Abs("../templates/installed-seed/Maestro.ps1")
					script := `Set-StrictMode -Version Latest; $ErrorActionPreference='Stop'
$tokens=$null; $errors=$null; $ast=[Management.Automation.Language.Parser]::ParseFile($env:SOURCE,[ref]$tokens,[ref]$errors)
if($errors.Count){throw 'Shipped source does not parse'}
foreach($name in @('Invoke-Management','Current-Clients','Test-DesktopPreparation')){
 $node=$ast.Find({param($n) $n -is [Management.Automation.Language.FunctionDefinitionAst] -and $n.Name -ceq $name},$true)
 if($null -eq $node){throw "Missing shipped helper $name"}; Invoke-Expression $node.Extent.Text
}
function Verify-Installer([string]$Path){}
function Stop-Maestro([string]$Message){throw $Message}
function Read-Host([string]$Prompt){return 'YES'}
function Ensure-CodeLogin{[IO.File]::AppendAllText((Join-Path $env:ROOT 'auth'),'auth'); return $true}
function Test-Owner{
 if($args[0] -ceq 'clients-status'){[IO.File]::AppendAllText((Join-Path $env:ROOT 'status-calls'),'checked'); $global:LASTEXITCODE=0; return [IO.File]::ReadAllText((Join-Path $env:ROOT 'canonical-status.json'))}
 if($args[0] -cne 'manage'){throw 'Unexpected owner action'}
 [IO.File]::WriteAllLines((Join-Path $env:ROOT 'manager-args'),[string[]]$args)
 [IO.File]::WriteAllText((Join-Path $env:ROOT 'manager-profile'),$env:CLAUDE_CONFIG_DIR)
 $global:LASTEXITCODE=0
}
$Installer='Test-Owner'; $ClientTarget=''; $Common=@('--workspace',(Join-Path $env:ROOT 'brain'),'--carrier-id','original-carrier'); $Manage=$Common
$Config=[PSCustomObject]@{code_configuration_directory=(Join-Path $env:ROOT 'selected-profile')}
$script:CodePreparationPending=$true
if($env:CASE -ceq 'partial_update'){Invoke-Management -Extra @('--management-intent','update')}else{Invoke-Management}
if($env:CLAUDE_CONFIG_DIR -cne 'caller-profile'){throw 'Caller profile changed'}
`
					cmd = exec.Command(shell, "-NoProfile", "-NonInteractive", "-Command", script)
					cmd.Env = append(os.Environ(), "SOURCE="+source)
				}
				if cmd.Env == nil {
					cmd.Env = os.Environ()
				}
				cmd.Env = append(cmd.Env, "ROOT="+root, "CASE="+test.name, "CLAUDE_CONFIG_DIR=caller-profile")
				if output, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("accepted original flow failed: %s %v", output, err)
				}
				body, err := os.ReadFile(filepath.Join(root, "manager-args"))
				if err != nil {
					t.Fatal(err)
				}
				args := strings.Split(strings.TrimSpace(string(body)), "\n")
				actual := ""
				for i, arg := range args {
					if arg == "--management-intent" && i+1 < len(args) {
						actual = args[i+1]
					}
				}
				if actual != test.intent {
					t.Fatalf("canonical %s accepted route intent=%q want %q; args=%q", test.name, actual, test.intent, body)
				}
				if test.name != "partial_update" {
					if body, _ := os.ReadFile(filepath.Join(root, "status-calls")); string(body) != "checked" {
						t.Fatal("accepted route did not obtain fresh canonical status exactly once")
					}
				}
				if body, _ := os.ReadFile(filepath.Join(root, "auth")); string(body) != "auth" {
					t.Fatal("explicit YES did not follow original authentication exactly once")
				}
				if body, _ := os.ReadFile(filepath.Join(root, "manager-profile")); string(body) != filepath.Join(root, "selected-profile") {
					t.Fatal("manager lost selected Code profile")
				}
			})
		}
	}
}

func TestMacSelfServiceDesktopRecheckPreservesEarlierReports(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Mac shell contract")
	}
	source, err := os.ReadFile("../templates/installed-seed/Maestro.command")
	if err != nil {
		t.Fatal(err)
	}
	extract := func(start, end string) string {
		t.Helper()
		_, tail, ok := strings.Cut(string(source), start)
		if !ok {
			t.Fatal("missing shipped helper")
		}
		body, _, ok := strings.Cut(tail, end)
		if !ok {
			t.Fatal("missing shipped helper boundary")
		}
		return start + body
	}
	helpers := extract("private_dir() {", "private_dir \"$DEV/Maestro-Pilot\"") + extract("refresh_report() {", "\nrefresh_report\n")
	for _, test := range []struct {
		unsafe bool
		scope  string
	}{{false, "desktop"}, {true, "desktop"}, {false, ""}} {
		t.Run(fmt.Sprintf("unsafe_current_directory_%v_scope_%s", test.unsafe, test.scope), func(t *testing.T) {
			unsafe := test.unsafe
			root := developerFixture(t)
			response := `{"schema_version":2,"evidence_class":"engineering_only","markdown":"Desktop attempt","code_prompt":"","code_readback_prompt":"","desktop_prompt":"Desktop","code_ready_prompt":"","desktop_ready_prompt":"Desktop ready","client_preparation":"desktop_prepared_code_pending"}`
			writeFixture(t, root, "response.json", []byte(response))
			server := writeFixture(t, root, "server", []byte("#!/bin/bash\nprintf '%s\\n' \"$*\" >> \"$ROOT/calls\"\ncat \"$ROOT/response.json\"\n"))
			if err := os.Chmod(server, 0700); err != nil {
				t.Fatal(err)
			}
			body, _ := os.ReadFile(server)
			writeFixture(t, root, "metadata.json", []byte(`{"server_hashes":["`+sum(body)+`","`+strings.Repeat("f", 64)+`"]}`))
			script := `set -euo pipefail
umask 077
fail(){ printf '%s' "$*" >&2; exit 19; }
sha(){ local value; value="$(/usr/bin/shasum -a 256 "$1")"; printf '%s' "${value%% *}"; }
jget(){ /usr/bin/plutil -extract "$2" raw -o - "$1"; }
verify_installer(){ :; }
installer_fixture(){ [[ "$1" == clients-status ]] || fail 'unexpected client action'; printf '{}'; }
DEV="$ROOT"; DIAGNOSTIC_ROOT="$ROOT/diagnostic"; RUN="$DIAGNOSTIC_ROOT"; mkdir "$RUN"
INSTALLED=installer_fixture; SERVER="$ROOT/server"; META="$ROOT/metadata.json"; VERSION=0.1.1; ROLE=role; PRODUCT="$ROOT/product"
COMMON=(--workspace "$ROOT/brain" --carrier-id original-carrier); CLIENT_STATUS=("${COMMON[@]}"); PILOT_OBSERVATION_CLIENT="$SCOPE"
` + helpers + `
refresh_report
cp "$RUN/report.json" "$ROOT/earlier-report"
if [[ "$UNSAFE" == true ]]; then mkdir "$ROOT/foreign"; ln -s "$ROOT/foreign" "$DIAGNOSTIC_ROOT/linked"; RUN="$DIAGNOSTIC_ROOT/linked"; fi
refresh_report
[[ "$RUN" == "$DIAGNOSTIC_ROOT/recheck-"* && "$CODE_PREPARATION_PENDING" == true ]] || fail 'recheck lost scope/pending state'
cmp "$DIAGNOSTIC_ROOT/report.json" "$ROOT/earlier-report" || fail 'earlier diagnostic changed'
[[ "$(wc -l < "$ROOT/calls" | tr -d ' ')" == 2 ]] || fail 'unexpected number of checker calls'
expected_scope=0; [[ "$SCOPE" != desktop ]] || expected_scope=2
[[ "$(/usr/bin/grep -c -- '--observation-client desktop' "$ROOT/calls" || true)" == "$expected_scope" ]] || fail 'observation scope lost'
`
			cmd := exec.Command("/bin/bash", "-c", script)
			cmd.Env = append(os.Environ(), "ROOT="+root, fmt.Sprintf("UNSAFE=%v", unsafe), "SCOPE="+test.scope)
			output, err := cmd.CombinedOutput()
			if !unsafe && err != nil {
				t.Fatalf("fresh scoped recheck failed: %s %v", output, err)
			}
			if unsafe {
				calls, readErr := os.ReadFile(filepath.Join(root, "calls"))
				if err == nil || readErr != nil || strings.Count(string(calls), "installed-pilot-check") != 1 {
					t.Fatalf("unsafe current directory was not refused before another checker call: %s %v", output, err)
				}
			}
		})
	}
}

func TestMacSelfServiceCodeDeferralKeepsFullDesktopPreparation(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Mac shell contract")
	}
	source, err := os.ReadFile("../templates/installed-seed/Maestro.command")
	if err != nil {
		t.Fatal(err)
	}
	_, body, ok := strings.Cut(string(source), "manage() {")
	if !ok {
		t.Fatal("shipped manager helper missing")
	}
	body, _, ok = strings.Cut(body, "CLIENT_STATUS=(")
	if !ok {
		t.Fatal("shipped manager boundary missing")
	}
	for _, operation := range []string{"prepare-desktop", "update-desktop"} {
		t.Run(operation, func(t *testing.T) {
			root := developerFixture(t)
			installer := filepath.Join(root, "installer-fixture")
			if err := os.WriteFile(installer, []byte("#!/bin/bash\nprintf '%s\\n' \"$@\" > \"$OBSERVED_ARGS\"\nprintf '%s' \"${CLAUDE_CONFIG_DIR-unset}\" > \"$OBSERVED_PROFILE\"\n"), 0700); err != nil {
				t.Fatal(err)
			}
			script := `set -euo pipefail
fail(){ printf '%s' "$*" >&2; exit 1; }
verify_installer(){ :; }
ensure_code_login(){ printf invoked > "$ROOT/code-invoked"; fail 'Deferred Code must not be invoked'; }
TARGET=""; CODE="$ROOT/code"; CODE_CONFIG="$ROOT/selected-profile"; CLIENTS="$ROOT/clients"
COMMON=(--product-root "$ROOT/product" --user-home "$ROOT/owner" --workspace "$ROOT/workspace" --carrier-id same-carrier)
manage() {` + body + `
if [[ "$OPERATION" == update-desktop ]]; then manage --management-intent update; else manage; fi
printf diagnostics-reached > "$ROOT/final-diagnostics"
`
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, "/bin/bash", "-c", script)
			cmd.Env = append(os.Environ(), "ROOT="+root, "OPERATION="+operation, "INSTALLED="+installer, "OBSERVED_ARGS="+filepath.Join(root, "args"), "OBSERVED_PROFILE="+filepath.Join(root, "profile"), "CLAUDE_CONFIG_DIR=caller-profile")
			cmd.Stdin = strings.NewReader("\n")
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("deferral must reach Desktop preparation and diagnostics without Code: %s %v", output, err)
			}
			args, err := os.ReadFile(filepath.Join(root, "args"))
			if err != nil {
				t.Fatal(err)
			}
			for _, required := range []string{"--client-code-root\n" + filepath.Join(root, "clients", "code") + "\n", "--client-code-config\n" + filepath.Join(root, "selected-profile") + "\n", "--client-desktop-path\n" + filepath.Join(root, "clients", "maestro-product.mcpb") + "\n", "--management-intent\n" + operation + "\n"} {
				if !strings.Contains(string(args), required) {
					t.Fatalf("original selection or preparation intent lost: %s", args)
				}
			}
			if strings.Contains(string(args), "--client-target\ndesktop\n") || strings.Count(string(args), "--management-intent\n") != 1 {
				t.Fatalf("deferral retargeted the selection or mixed operations: %s", args)
			}
			if _, err := os.Lstat(filepath.Join(root, "code-invoked")); !os.IsNotExist(err) {
				t.Fatal("Code/auth path ran during deferral")
			}
			if value, _ := os.ReadFile(filepath.Join(root, "profile")); string(value) != "caller-profile" {
				t.Fatal("deferral changed the caller profile environment")
			}
			if _, err := os.Stat(filepath.Join(root, "final-diagnostics")); err != nil {
				t.Fatal("deferral exited before final diagnostics")
			}
		})
	}
}

const participantResultCollectorPowerShell = `param([string]$Source,[string]$Report,[string]$ClientTarget)
$ErrorActionPreference='Stop';Set-StrictMode -Version Latest
$Utf8=New-Object System.Text.UTF8Encoding($false)
$tokens=$null;$errors=$null;$ast=[Management.Automation.Language.Parser]::ParseFile($Source,[ref]$tokens,[ref]$errors)
if($errors.Count){throw 'Shipped frontend does not parse'}
$collector=$ast.Find({param($n) $n -is [Management.Automation.Language.ForEachStatementAst] -and $n.Extent.Text.StartsWith("foreach (` + "`" + `$label in @('Code: identidade/skills'")},$true)
if($null -eq $collector){throw 'Missing shipped participant result collector'}
$script:Answers=New-Object 'Collections.Generic.Queue[string]'
$count=6;if($ClientTarget -ceq 'desktop'){$count=4}else{$ClientTarget=''}
for($i=0;$i -lt $count;$i++){$script:Answers.Enqueue('passou')}
function Read-Host([string]$Prompt){
  if($script:Answers.Count -eq 0){throw 'Unexpected extra result question'}
  Write-Host $Prompt
  return $script:Answers.Dequeue()
}
Invoke-Expression $collector.Extent.Text
if($script:Answers.Count -ne 0){throw 'A relevant result question was omitted'}
`
