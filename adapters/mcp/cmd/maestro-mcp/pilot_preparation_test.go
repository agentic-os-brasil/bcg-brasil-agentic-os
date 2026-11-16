package main

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"
)

// Exercise the actual Windows preparer inside the existing ordinary-account journey.
func assertWindowsPilotPreparation(t *testing.T, binary string) {
	t.Helper()
	if runtime.GOOS != "windows" {
		return
	}
	identity := windowsPilotArtifact(t)
	kit := t.TempDir()
	if identity != nil {
		kit = extractWindowsPilotArtifact(t, os.Getenv("MAESTRO_WINDOWS_PILOT_KIT"), kit, identity)
	} else {
		source := filepath.Join("..", "..", "..", "..", "dev", "pilot")
		files := map[string]string{
			"Preparar Maestro.ps1":       "setup-windows.ps1",
			"templates/test-windows.ps1": "test-windows.ps1",
			"templates/test-windows.cmd": "test-windows.cmd",
		}
		for destination, original := range files {
			body, err := os.ReadFile(filepath.Join(source, original))
			if err != nil {
				t.Fatal(err)
			}
			if strings.HasSuffix(destination, ".ps1") && !bytes.HasPrefix(body, []byte{0xef, 0xbb, 0xbf}) {
				body = append([]byte{0xef, 0xbb, 0xbf}, body...)
			}
			writePilotFixture(t, filepath.Join(kit, destination), body)
		}
		body, err := os.ReadFile(binary)
		if err != nil {
			t.Fatal(err)
		}
		writePilotFixture(t, filepath.Join(kit, "bin", "maestro-mcp-windows-amd64.exe"), body)
		writePilotFixture(t, filepath.Join(kit, "LIFECYCLE-CONTEXT.txt"), []byte("enabled\n"))
		var rows []string
		err = filepath.WalkDir(kit, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil || entry.IsDir() {
				return walkErr
			}
			body, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}
			relative, relErr := filepath.Rel(kit, path)
			if relErr != nil {
				return relErr
			}
			rows = append(rows, fmt.Sprintf("%x  ./%s\n", sha256.Sum256(body), filepath.ToSlash(relative)))
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		sort.Strings(rows)
		inventory := []byte(strings.Join(rows, ""))
		writePilotFixture(t, filepath.Join(kit, "SHA256SUMS"), inventory)
		writePilotFixture(t, filepath.Join(kit, "KIT-ID.txt"), []byte(fmt.Sprintf("%x\n", sha256.Sum256(inventory))))
	}
	// The ordinary-account runner can inherit the administrator's profile variables.
	profile := t.TempDir()
	developer := filepath.Join(profile, "Developer")
	if err := os.MkdirAll(developer, 0700); err != nil {
		t.Fatal(err)
	}
	root, err := os.MkdirTemp(developer, "MaestroPilot-selfserve-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	if err := os.Remove(root); err != nil {
		t.Fatal(err)
	}
	prepare := filepath.Join(kit, "Preparar Maestro.ps1")
	review := runPilotPowerShell(t, profile, prepare, "-Root", root, "-Review")
	match := regexp.MustCompile(`review_digest=([a-f0-9]{64})`).FindSubmatch(review)
	if len(match) != 2 {
		t.Fatal("actual preparer did not produce one review digest")
	}
	if _, err := os.Lstat(root); !os.IsNotExist(err) {
		t.Fatal("review created or adopted the preparation target", err)
	}
	runPilotPowerShell(t, profile, prepare, "-Root", root, "-ConfirmDigest", string(match[1]))
	before, err := os.ReadFile(filepath.Join(root, "managed-files.sha256"))
	if err != nil {
		t.Fatal("actual preparer did not finish managed inventory:", err)
	}
	stateBefore := windowsPilotState(t, root)
	runPilotPowerShell(t, profile, filepath.Join(root, "launchers", "Testar Maestro.ps1"), "-CheckOnly")
	runPilotPowerShell(t, profile, prepare, "-Root", root, "-ConfirmDigest", string(match[1]))
	after, err := os.ReadFile(filepath.Join(root, "managed-files.sha256"))
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("self-check or repeated preparation changed managed inventory")
	}
	if !reflect.DeepEqual(stateBefore, windowsPilotState(t, root)) {
		t.Fatal("emitted self-check or repeated preparation changed private or managed bytes/modes")
	}
	paths, err := filepath.Glob(filepath.Join(root, "diagnostics", "*", "report.json"))
	if err != nil || len(paths) != 3 {
		t.Fatal("preparation and emitted self-check must each retain a report")
	}
	for _, path := range paths {
		body, err := os.ReadFile(path)
		var report struct {
			Status      string `json:"status"`
			WorkspaceID string `json:"workspace_id"`
			Markdown    string `json:"markdown"`
			Checks      []struct {
				Name   string `json:"name"`
				Status string `json:"status"`
			} `json:"checks"`
		}
		if err != nil || json.Unmarshal(body, &report) != nil || report.Status == "blocked" || report.WorkspaceID == "" {
			t.Fatal("actual Windows preparer or emitted self-check is blocked")
		}
		if !strings.Contains(report.Markdown, "diagnóstico do ensaio") {
			t.Fatal("Windows PowerShell lost UTF-8 report content")
		}
		for _, required := range []string{"code_packet", "desktop_archive", "preparation", "binding_admission", "mcp_connection"} {
			passed := false
			for _, check := range report.Checks {
				if check.Name == required && check.Status == "passed" {
					passed = true
				}
			}
			if !passed {
				t.Fatalf("emitted report lacks passing %s", required)
			}
		}
	}
	if identity != nil {
		writeWindowsPilotArtifactProof(t, root, kit, binary, identity, len(paths))
	}
}

func writePilotFixture(t *testing.T, path string, body []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0600); err != nil {
		t.Fatal(err)
	}
}

func stockPilotPowerShell() string {
	return filepath.Join(os.Getenv("SystemRoot"), "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
}

func runPilotPowerShell(t *testing.T, profile, script string, args ...string) []byte {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	arguments := append([]string{"-NoProfile", "-NonInteractive", "-File", script}, args...)
	cmd := exec.CommandContext(ctx, stockPilotPowerShell(), arguments...)
	for _, entry := range os.Environ() {
		key := strings.ToUpper(strings.SplitN(entry, "=", 2)[0])
		if key != "MAESTRO_HOME" && key != "USERPROFILE" && key != "HOME" && key != "PSMODULEPATH" {
			cmd.Env = append(cmd.Env, entry)
		}
	}
	cmd.Env = append(cmd.Env, "USERPROFILE="+profile, "HOME="+profile)
	body, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("actual Windows pilot script %s failed: %v\n%s", filepath.Base(script), err, body)
	}
	return body
}

// This branch remains inside TestMigratedHomePublicMCPInit. Explicit artifact
// qualification must consume the supplied ZIP; no source-built substitute exists.
type windowsPilotArtifactIdentity struct {
	SchemaVersion  int    `json:"schema_version"`
	SourceRevision string `json:"source_revision"`
	SourceTree     string `json:"source_tree"`
	ProducerRunID  string `json:"producer_run_id"`
	ZIPHash        string `json:"generic_zip_sha256"`
	KitID          string `json:"kit_id"`
	ServerHash     string `json:"shipping_server_sha256"`
}

func windowsPilotArtifact(t *testing.T) *windowsPilotArtifactIdentity {
	t.Helper()
	keys := []string{"MAESTRO_WINDOWS_PILOT_KIT", "MAESTRO_WINDOWS_PILOT_IDENTITY", "MAESTRO_WINDOWS_PILOT_METHODS", "MAESTRO_WINDOWS_PILOT_PROOF"}
	selected := 0
	for _, key := range keys {
		if os.Getenv(key) != "" {
			selected++
		}
	}
	if selected == 0 {
		return nil
	}
	if selected != len(keys) || os.Getenv("MAESTRO_WINDOWS_CORE_PROOF") != "1" {
		t.Fatal("exact Windows artifact mode requires all ordinary-account inputs")
	}
	for _, key := range keys {
		if !filepath.IsAbs(os.Getenv(key)) {
			t.Fatal("exact Windows artifact input is not absolute", key)
		}
	}
	body, err := os.ReadFile(os.Getenv("MAESTRO_WINDOWS_PILOT_IDENTITY"))
	if err != nil || len(body) > 65536 {
		t.Fatal("exact artifact identity unavailable", err)
	}
	var identity windowsPilotArtifactIdentity
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&identity); err != nil || identity.SchemaVersion != 1 {
		t.Fatal("exact artifact identity invalid", err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		t.Fatal("exact artifact identity has trailing data", err)
	}
	return &identity
}

func windowsPilotFileHash(t *testing.T, path string) string {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal("artifact file unavailable", filepath.Base(path), err)
	}
	return fmt.Sprintf("%x", sha256.Sum256(body))
}

func extractWindowsPilotArtifact(t *testing.T, selected, destination string, identity *windowsPilotArtifactIdentity) string {
	t.Helper()
	archive := selected + ".zip"
	if windowsPilotFileHash(t, archive) != identity.ZIPHash {
		t.Fatal("exact supplied ZIP changed before preparation")
	}
	reader, err := zip.OpenReader(archive)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	name := filepath.Base(selected)
	seen := map[string]bool{}
	var total uint64
	for _, entry := range reader.File {
		path := filepath.FromSlash(entry.Name)
		key := strings.ToLower(path)
		if !filepath.IsLocal(path) || !strings.HasPrefix(entry.Name, name+"/") || strings.ContainsAny(entry.Name, "\\:\x00") || seen[key] || entry.Mode()&os.ModeSymlink != 0 {
			t.Fatal("unsafe or duplicate generic ZIP entry", entry.Name)
		}
		seen[key] = true
		total += entry.UncompressedSize64
		if entry.UncompressedSize64 > 128<<20 || total > 256<<20 {
			t.Fatal("generic ZIP exceeds bounded artifact size")
		}
		target := filepath.Join(destination, path)
		if entry.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0700); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if !entry.Mode().IsRegular() {
			t.Fatal("generic ZIP contains a nonregular file")
		}
		if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
			t.Fatal(err)
		}
		input, err := entry.Open()
		if err != nil {
			t.Fatal(err)
		}
		output, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			input.Close()
			t.Fatal(err)
		}
		_, copyErr := io.Copy(output, input)
		closeErr := output.Close()
		input.Close()
		if copyErr != nil || closeErr != nil {
			t.Fatal("exact ZIP extraction failed", copyErr, closeErr)
		}
	}
	kit := filepath.Join(destination, name)
	if windowsPilotFileHash(t, filepath.Join(kit, "bin", "maestro-mcp-windows-amd64.exe")) != identity.ServerHash ||
		windowsPilotFileHash(t, filepath.Join(kit, "SHA256SUMS")) != identity.KitID {
		t.Fatal("extracted exact ZIP PE or KIT-ID differs")
	}
	return kit
}

func windowsPilotState(t *testing.T, root string) map[string]string {
	t.Helper()
	state := map[string]string{}
	for _, part := range []string{"owner", "workspace", "product", "exports", "launchers"} {
		if err := filepath.WalkDir(filepath.Join(root, part), func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			info, err := entry.Info()
			if err != nil {
				return err
			}
			relative, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			if entry.IsDir() {
				state[relative] = info.Mode().String()
				return nil
			}
			if !info.Mode().IsRegular() {
				return fmt.Errorf("nonregular artifact state: %s", relative)
			}
			body, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			state[relative] = fmt.Sprintf("%s:%x", info.Mode(), sha256.Sum256(body))
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	return state
}

func writeWindowsPilotArtifactProof(t *testing.T, root, kit, sourceProbe string, identity *windowsPilotArtifactIdentity, reports int) {
	t.Helper()
	var prepared struct {
		WorkspaceID string `json:"workspace_id"`
	}
	body, err := os.ReadFile(filepath.Join(root, "preparation.json"))
	if err != nil || json.Unmarshal(body, &prepared) != nil || len(prepared.WorkspaceID) != 32 {
		t.Fatal("exact preparation identity unavailable", err)
	}
	preparedHash := windowsPilotFileHash(t, filepath.Join(root, "product", "maestro-mcp.exe"))
	packetHash := windowsPilotFileHash(t, filepath.Join(root, "exports", "code", "scripts", "maestro-mcp.exe"))
	if preparedHash != identity.ServerHash || packetHash != identity.ServerHash || windowsPilotFileHash(t, filepath.Join(kit, "bin", "maestro-mcp-windows-amd64.exe")) != identity.ServerHash {
		t.Fatal("prepared or packet server substituted another PE")
	}
	desktop, err := zip.OpenReader(filepath.Join(root, "exports", "maestro-product.mcpb"))
	if err != nil {
		t.Fatal("prepared Desktop archive unavailable", err)
	}
	desktopHash := ""
	for _, entry := range desktop.File {
		if entry.Name != "server/maestro-mcp.exe" {
			continue
		}
		file, err := entry.Open()
		if err != nil {
			desktop.Close()
			t.Fatal(err)
		}
		body, readErr := io.ReadAll(io.LimitReader(file, 128<<20))
		file.Close()
		if readErr != nil {
			desktop.Close()
			t.Fatal(readErr)
		}
		desktopHash = fmt.Sprintf("%x", sha256.Sum256(body))
	}
	desktop.Close()
	if desktopHash != identity.ServerHash {
		t.Fatal("prepared Desktop packet substituted another PE")
	}
	idsBody, err := os.ReadFile(os.Getenv("MAESTRO_WINDOWS_PILOT_METHODS"))
	if err != nil {
		t.Fatal(err)
	}
	ids := strings.Fields(string(idsBody))
	sort.Strings(ids)
	var index struct {
		Methods []struct {
			ID string `json:"id"`
		} `json:"methods"`
	}
	body, err = os.ReadFile(filepath.Join(root, "exports", "code", "methods-index.json"))
	if err != nil || json.Unmarshal(body, &index) != nil {
		t.Fatal("exact method index unavailable", err)
	}
	actualIDs := []string{}
	for _, method := range index.Methods {
		actualIDs = append(actualIDs, method.ID)
	}
	sort.Strings(actualIDs)
	if !reflect.DeepEqual(ids, actualIDs) {
		t.Fatal("canonical method catalog differs from source fixture")
	}
	folders, err := filepath.Glob(filepath.Join(root, "exports", "code", "skills", "*", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	actualFolders := []string{}
	for _, path := range folders {
		actualFolders = append(actualFolders, filepath.Base(filepath.Dir(path)))
	}
	sort.Strings(actualFolders)
	wantedFolders := append(append([]string{}, ids...), "operator", "resume")
	sort.Strings(wantedFolders)
	if !reflect.DeepEqual(wantedFolders, actualFolders) {
		t.Fatal("native skill folders differ from canonical methods plus two bridges")
	}
	image, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	versionContext, versionCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer versionCancel()
	versionCommand := exec.CommandContext(versionContext, stockPilotPowerShell(), "-NoProfile", "-NonInteractive", "-Command", `[pscustomobject]@{edition=$PSVersionTable.PSEdition;version=$PSVersionTable.PSVersion.ToString()} | ConvertTo-Json -Compress`)
	for _, entry := range os.Environ() {
		if !strings.EqualFold(strings.SplitN(entry, "=", 2)[0], "PSMODULEPATH") {
			versionCommand.Env = append(versionCommand.Env, entry)
		}
	}
	versionBody, err := versionCommand.CombinedOutput()
	var powerShell struct {
		Edition string `json:"edition"`
		Version string `json:"version"`
	}
	if err != nil || json.Unmarshal(versionBody, &powerShell) != nil || powerShell.Edition != "Desktop" || !regexp.MustCompile(`^5\.1\.[0-9]+(\.[0-9]+)?$`).MatchString(powerShell.Version) {
		t.Fatal("emitted preparation requires the observed stock Windows PowerShell 5.1 executable", err)
	}
	proof := map[string]any{"passed": true, "schema_version": 1, "source_revision": identity.SourceRevision, "source_tree": identity.SourceTree, "producer_run_id": identity.ProducerRunID,
		"generic_zip_sha256": identity.ZIPHash, "kit_id": identity.KitID, "shipping_server_sha256": identity.ServerHash, "prepared_server_sha256": preparedHash, "packet_server_sha256": packetHash, "desktop_packet_server_sha256": desktopHash,
		"source_probe_server_sha256": windowsPilotFileHash(t, sourceProbe), "command_test_image_sha256": windowsPilotFileHash(t, image), "workspace_id": prepared.WorkspaceID,
		"powershell_edition": powerShell.Edition, "powershell_version": powerShell.Version, "emitted_review_confirmed": true, "private_and_managed_unchanged": true, "sdk_context_verified": true, "canonical_methods": len(ids), "skill_folders": len(actualFolders), "reports_count": reports, "native_Code_Desktop": "not_run"}
	file, err := os.OpenFile(os.Getenv("MAESTRO_WINDOWS_PILOT_PROOF"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	encodeErr := json.NewEncoder(file).Encode(proof)
	closeErr := file.Close()
	if encodeErr != nil || closeErr != nil {
		t.Fatal("exact artifact receipt unavailable", encodeErr, closeErr)
	}
}

func TestPilotWindowsCodePromptHandoff(t *testing.T) {
	shell := stockPilotPowerShell()
	if runtime.GOOS != "windows" {
		var err error
		shell, err = exec.LookPath("pwsh")
		if err != nil {
			t.Skip("PowerShell is unavailable")
		}
	}
	source := filepath.Join("..", "..", "..", "..", "dev", "pilot")
	setup, err := os.ReadFile(filepath.Join(source, "setup-windows.ps1"))
	if err != nil {
		t.Fatal(err)
	}
	start := strings.Index(string(setup), "$code = @'\n")
	if start < 0 {
		t.Fatal("Code launcher producer absent")
	}
	launcher := strings.SplitN(string(setup)[start+len("$code = @'\n"):], "\n'@", 2)[0]
	base := t.TempDir()
	root := filepath.Join(base, "prepared space ' literal")
	tools := filepath.Join(base, "tools")
	witness := filepath.Join(base, "argv.json")
	prompt := "Use exact \"quoted\" text, $HOME and $(throw 'never'), C:\\path\\\"quoted\".\nSecond line."
	report, _ := json.Marshal(map[string]any{"schema_version": 1, "status": "ready", "markdown": "synthetic report", "code_prompt": prompt + " complete", "desktop_prompt": "desktop", "code_ready_prompt": prompt + " connection", "desktop_ready_prompt": "desktop connection"})
	helper := `package main
import ("encoding/json";"os";"path/filepath";"fmt")
func main(){if filepath.Base(os.Args[0])=="maestro-mcp.exe" {if len(os.Args)>1&&os.Args[1]=="pilot-check"{b,e:=os.ReadFile(os.Getenv("PILOT_REPORT"));if e!=nil{panic(e)};fmt.Print(string(b))};return};wd,_:=os.Getwd();f,e:=os.Create(os.Getenv("PILOT_ARGS"));if e!=nil{panic(e)};defer f.Close();json.NewEncoder(f).Encode(map[string]any{"args":os.Args[1:],"cwd":wd,"profile":os.Getenv("CLAUDE_CONFIG_DIR")})}
`
	helperSource := filepath.Join(base, "recorder.go")
	writePilotFixture(t, helperSource, []byte(helper))
	if err := os.MkdirAll(tools, 0700); err != nil {
		t.Fatal(err)
	}
	recorder := filepath.Join(tools, "claude.exe")
	build := exec.Command("go", "build", "-o", recorder, helperSource)
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("argument recorder: %s %v", out, err)
	}
	image, err := os.ReadFile(recorder)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		ordinary := filepath.Join(tools, "claude")
		writePilotFixture(t, ordinary, image)
		if err := os.Chmod(ordinary, 0700); err != nil {
			t.Fatal(err)
		}
	}
	for path, body := range map[string][]byte{
		filepath.Join(root, "product", "maestro-mcp.exe"):                    image,
		filepath.Join(root, "exports", "code", "scripts", "maestro-mcp.exe"): image,
		filepath.Join(root, ".maestro-pilot-owned"):                          []byte("synthetic"),
		filepath.Join(root, "pilot-binary.sha256"):                           []byte(fmt.Sprintf("%x  product/maestro-mcp.exe\n", sha256.Sum256(image))),
		filepath.Join(base, "report.json"):                                   report,
		filepath.Join(root, "launchers", "Abrir Code.ps1"):                   append([]byte{0xef, 0xbb, 0xbf}, []byte(launcher)...),
	} {
		writePilotFixture(t, path, body)
		if strings.HasSuffix(path, ".exe") {
			if err := os.Chmod(path, 0700); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := os.MkdirAll(filepath.Join(root, "workspace"), 0700); err != nil {
		t.Fatal(err)
	}
	menuBody, err := os.ReadFile(filepath.Join(source, "test-windows.ps1"))
	if err != nil {
		t.Fatal(err)
	}
	menu := filepath.Join(root, "launchers", "Testar Maestro.ps1")
	writePilotFixture(t, menu, append([]byte{0xef, 0xbb, 0xbf}, menuBody...))
	run := func(args ...string) ([]byte, error) {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, shell, append([]string{"-NoProfile", "-NonInteractive"}, args...)...)
		cmd.Env = append(os.Environ(), "PATH="+tools, "PILOT_ARGS="+witness, "PILOT_REPORT="+filepath.Join(base, "report.json"), "PILOT_SHIM_WITNESS="+filepath.Join(base, "shim-invoked"))
		if runtime.GOOS != "windows" {
			cmd.Env = append(cmd.Env, "WINDIR="+base)
		}
		return cmd.CombinedOutput()
	}
	assertArgs := func(want ...string) {
		t.Helper()
		body, err := os.ReadFile(witness)
		if err != nil {
			t.Fatal(err)
		}
		var got struct {
			Args    []string `json:"args"`
			CWD     string   `json:"cwd"`
			Profile string   `json:"profile"`
		}
		if err := json.Unmarshal(body, &got); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got.Args, want) || got.CWD != filepath.Join(root, "workspace") || got.Profile != filepath.Join(root, "client", "code") {
			t.Fatalf("literal Code handoff differs: %#v want %q", got, want)
		}
		if err := os.Remove(witness); err != nil {
			t.Fatal(err)
		}
	}
	code := filepath.Join(root, "launchers", "Abrir Code.ps1")
	prefix := []string{"--plugin-dir", filepath.Join(root, "exports", "code")}
	if out, err := run("-File", code); err != nil {
		t.Fatalf("ordinary open: %s %v", out, err)
	}
	assertArgs(prefix...)
	if out, err := run("-File", code, prompt); err != nil {
		t.Fatalf("prompt open: %s %v", out, err)
	}
	assertArgs(append(append([]string{}, prefix...), "--", prompt)...)
	for _, invalid := range [][]string{{""}, {"one", "two"}} {
		if out, err := run(append([]string{"-File", code}, invalid...)...); err == nil {
			t.Fatalf("invalid arguments accepted: %s", out)
		}
	}
	if out, err := run("-File", menu, "-CheckOnly"); err != nil {
		t.Fatalf("check-only: %s %v", out, err)
	}
	if _, err := os.Stat(witness); !os.IsNotExist(err) {
		t.Fatal("check-only or invalid arguments opened Code")
	}
	for _, choice := range []string{"1", "3"} {
		wrapper := "function Read-Host { '" + choice + "' }; function Start-Process {}; function Set-Clipboard { throw 'Code must not use Clipboard' }; & '" + strings.ReplaceAll(menu, "'", "''") + "'"
		if out, err := run("-Command", wrapper); err != nil {
			t.Fatalf("menu %s: %s %v", choice, out, err)
		}
		suffix := " complete"
		if choice == "3" {
			suffix = " connection"
		}
		assertArgs(append(append([]string{}, prefix...), "--", prompt+suffix)...)
	}
	if err := os.Rename(recorder, recorder+".disabled"); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		if err := os.Remove(filepath.Join(tools, "claude")); err != nil {
			t.Fatal(err)
		}
	}
	shimWitness := filepath.Join(base, "shim-invoked")
	refuse := func(args ...string) {
		t.Helper()
		body, err := run(args...)
		if err == nil || !strings.Contains(string(body), "claude.exe") {
			t.Fatalf("unsupported prompt transport did not refuse clearly: %s %v", body, err)
		}
		for _, path := range []string{witness, shimWitness} {
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Fatal("unsupported initial prompt dispatched a command", path)
			}
		}
	}
	for _, ext := range []string{".ps1", ".cmd", ".bat"} {
		path := filepath.Join(tools, "claude"+ext)
		body := "[IO.File]::WriteAllText($env:PILOT_SHIM_WITNESS, 'shim invoked')\n"
		if ext != ".ps1" {
			if runtime.GOOS == "windows" {
				body = "@echo off\r\necho shim invoked> \"%PILOT_SHIM_WITNESS%\"\r\n"
			} else {
				body = "#!/bin/sh\nprintf invoked > \"$PILOT_SHIM_WITNESS\"\n"
			}
		}
		writePilotFixture(t, path, []byte(body))
		if err := os.Chmod(path, 0700); err != nil {
			t.Fatal(err)
		}
		refuse("-File", code, "protected prompt")
		if runtime.GOOS == "windows" || ext == ".ps1" {
			if out, err := run("-File", code); err != nil {
				t.Fatalf("ordinary %s open lost compatibility: %s %v", ext, out, err)
			}
			if _, err := os.Stat(shimWitness); err != nil {
				t.Fatal("ordinary shim was not invoked", err)
			}
			if err := os.Remove(shimWitness); err != nil {
				t.Fatal(err)
			}
		}
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
	}
	quotedCode := "'" + strings.ReplaceAll(code, "'", "''") + "'"
	for _, definition := range []string{
		"function claude { [IO.File]::WriteAllText($env:PILOT_SHIM_WITNESS,'function invoked') }",
		"function fixtureCode { [IO.File]::WriteAllText($env:PILOT_SHIM_WITNESS,'alias invoked') }; Set-Alias claude fixtureCode",
	} {
		refuse("-Command", definition+"; & "+quotedCode+" -InitialPrompt 'protected prompt'")
	}
}
