package main

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	product "github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp/packaging"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/datahome"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspace"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// This exercises canonical packet selection without launching an MCP server or
// client. A qualifier's MCP-only fallback never promotes native capabilities.
func TestPilotCodePacketSelectionPreservesCanonicalInventory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("ordinary native owner admission runs in the Windows qualification journey")
	}
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	o := product.Options{ProductRoot: filepath.Join(root, "product"), UserHome: filepath.Join(root, "owner"), WorkspacePath: filepath.Join(root, "workspace"), LookupEnv: func(string) (string, bool) { return "", false }}
	for _, path := range []string{o.ProductRoot, o.UserHome, filepath.Join(root, "exports")} {
		if err := os.Mkdir(path, 0700); err != nil {
			t.Fatal(err)
		}
	}
	home, err := datahome.InitializeFresh(datahome.Options{ProductRoot: o.ProductRoot, UserHome: o.UserHome, LookupEnv: o.LookupEnv})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := workspace.Initialize(workspace.Options{WorkspacePath: o.WorkspacePath, DataRoot: filepath.Join(home.Home, "state")}); err != nil {
		t.Fatal(err)
	}
	server, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	image, err := os.ReadFile(server)
	if err != nil {
		t.Fatal(err)
	}
	serverSHA := fmt.Sprintf("%x", sha256.Sum256(image))
	host := packaging.Target{OS: runtime.GOOS, Arch: runtime.GOARCH}
	base := filepath.Join(root, "exports", "code")
	options := packaging.Options{Destination: base, Server: server, Version: "0.0.0", Binding: o}
	if err := packaging.BuildPlugin(options); err != nil {
		t.Fatal(err)
	}
	assertBase := func() {
		t.Helper()
		selected, status, err := pilotCodePacket(o, host, serverSHA)
		if err != nil || selected != base || status.NativeMode != "mcp_only" {
			t.Fatalf("ordinary MCP selection changed: %s %#v %v", selected, status, err)
		}
	}
	assertBase()
	if _, _, err := pilotCodePacket(o, host, strings.Repeat("0", 64)); err == nil {
		t.Fatal("untrusted executable accepted")
	}
	if _, _, err := pilotCodePacket(o, packaging.Target{OS: "other", Arch: host.Arch}, serverSHA); err == nil {
		t.Fatal("other target accepted")
	}
	native := filepath.Join(root, "exports", "code-native-qualified")
	options.Destination = native
	if err := packaging.BuildPlugin(options); err != nil {
		t.Fatal(err)
	}
	assertBase()
	extra := filepath.Join(native, "untracked")
	writePilotFixture(t, extra, []byte("untracked"))
	if _, _, err := pilotCodePacket(o, host, serverSHA); err == nil {
		t.Fatal("optional export bypassed its closed inventory")
	}
	if err := os.Remove(extra); err != nil {
		t.Fatal(err)
	}
	assertBase()
	if err := os.Rename(native, native+".retained"); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(base, native); err != nil {
		t.Fatal(err)
	}
	if _, _, err := pilotCodePacket(o, host, serverSHA); err == nil {
		t.Fatal("optional export symlink accepted")
	}
}

// Invoked inside the existing ordinary-account command admission on Windows.
func assertPilotCheckPackets(t *testing.T) {
	t.Helper()
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	prod, owner, work := filepath.Join(base, "product"), filepath.Join(base, "owner"), filepath.Join(base, "workspace")
	for _, dir := range []string{prod, owner} {
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("MAESTRO_HOME", "")
	if err := os.Unsetenv("MAESTRO_HOME"); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(prod, "maestro-mcp")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	if out, err := exec.Command("go", "build", "-o", binary, ".").CombinedOutput(); err != nil {
		t.Fatalf("build public command: %s %v", out, err)
	}
	assertWindowsPilotPreparation(t, binary)
	args := []string{"--product-root", prod, "--user-home", owner, "--workspace", work}
	kitID := strings.Repeat("a", 64)
	if err := os.WriteFile(filepath.Join(base, ".maestro-pilot-owned"), []byte(kitID+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(binary, append([]string{"init"}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("synthetic public init: %v %s", err, out)
	}
	if err := os.WriteFile(filepath.Join(base, "preparation.json"), out, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(base, "exports"), 0700); err != nil {
		t.Fatal(err)
	}
	options := packaging.Options{Destination: filepath.Join(base, "exports", "code"), Server: binary, Version: "0.0.0", Binding: product.Options{ProductRoot: prod, UserHome: owner, WorkspacePath: work}}
	if err := packaging.BuildPlugin(options); err != nil {
		t.Fatal(err)
	}
	options.Destination = filepath.Join(base, "exports", "maestro-product.mcpb")
	desktopSupported := runtime.GOOS == "darwin" || runtime.GOOS == "windows"
	if desktopSupported {
		if output, err := exec.Command(binary, append([]string{"mcpb", "--desktop-pilot", "--server", binary, "--destination", options.Destination, "--version", "0.0.0"}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("isolated Desktop public producer: %v %s", err, output)
		}
		assertDesktopPilotManifest(t, options.Destination, owner, work, out)
		assertDesktopPilotRuntimeSelection(t, binary, options.Destination, options.Binding)
		for _, token := range []string{"${__dirname}", "${user_config.workspace}"} {
			root := filepath.Join(t.TempDir(), token)
			productRoot, userHome, workspace := filepath.Join(root, "product"), filepath.Join(root, "owner"), filepath.Join(root, "workspace")
			for _, path := range []string{productRoot, userHome} {
				if err := os.MkdirAll(path, 0700); err != nil {
					t.Fatal(err)
				}
			}
			selection := []string{"--product-root", productRoot, "--user-home", userHome, "--workspace", workspace}
			if output, err := exec.Command(binary, append([]string{"init"}, selection...)...).CombinedOutput(); err != nil {
				t.Fatalf("admit literal interpolation fixture: %v %s", err, output)
			}
			destination := filepath.Join(root, "exports", "refused.mcpb")
			output, err := exec.Command(binary, append([]string{"mcpb", "--desktop-pilot", "--server", binary, "--destination", destination, "--version", "0.0.0"}, selection...)...).CombinedOutput()
			if err == nil || !strings.Contains(string(output), "without ${") {
				t.Fatalf("public Desktop export accepted or misdiagnosed reserved interpolation: %v %s", err, output)
			}
			if _, err := os.Lstat(filepath.Dir(destination)); !os.IsNotExist(err) {
				t.Fatalf("reserved interpolation created export material: %v", err)
			}
		}
	}
	snapshot := func() map[string]string {
		files := map[string]string{}
		if err := filepath.WalkDir(base, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() {
				return nil
			}
			body, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			files[path] = string(sha256Sum(body))
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		return files
	}
	before := snapshot()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, binary, append([]string{"pilot-check"}, args...)...)
	out, err = command.Output()
	if desktopSupported {
		if err != nil {
			t.Fatalf("public diagnostic command failed: %v %s", err, out)
		}
	} else if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 1 {
		t.Fatalf("unavailable Desktop must return a blocked diagnostic: %v %s", err, out)
	}
	var report pilotReport
	if err := json.Unmarshal(out, &report); err != nil {
		t.Fatal(err)
	}
	expectedStatus := "partial"
	if !desktopSupported {
		expectedStatus = "blocked"
	}
	if report.Status != expectedStatus || len(report.WorkspaceID) != 32 {
		t.Fatalf("actual packet did not produce guided client handoff: %+v", report)
	}
	assertPilotReadinessPrompt(t, out, report.WorkspaceID, desktopSupported)
	if desktopSupported && (!strings.Contains(report.CodePrompt, report.WorkspaceID) || !strings.Contains(report.DesktopPrompt, report.WorkspaceID)) || !desktopSupported && (report.CodePrompt != "" || report.DesktopPrompt != "") {
		t.Fatal("client guidance ignored native Desktop availability")
	}
	if desktopSupported {
		for _, prompt := range []string{report.CodePrompt, report.DesktopPrompt} {
			for _, selector := range []string{"execution.next somente com item_id", "lista estiver vazia, pule execution.next"} {
				if !strings.Contains(prompt, selector) {
					t.Fatalf("generated public prompt omits the required execution selector or empty-list branch: %s", selector)
				}
			}
		}
		for _, selector := range []string{"methods.plan com method_id", "methods.read com resource_uri", "instruction_uri", "4096", "next_offset", "brain.templates", "brain.create uma única vez com page e text", "A primeira operação na página deve ser brain.create", "independentemente do estado do recibo"} {
			if !strings.Contains(report.CodePrompt, selector) {
				t.Fatalf("generated public prompt omits a required method selector or read bound: %s", selector)
			}
		}
		if gate, journey := strings.Index(report.CodePrompt, "Antes dos métodos ou de criar a página"), strings.Index(report.CodePrompt, "Quero testar explicitamente os métodos"); gate < 0 || journey <= gate {
			t.Fatal("repeat-work gate must precede the method and page journey")
		}
		if gate, page := strings.Index(report.DesktopPrompt, "pare antes de brain.read"), strings.Index(report.DesktopPrompt, "Somente com um trabalho identificado recupere"); gate < 0 || page <= gate {
			t.Fatal("Desktop must report the verified context and stop before page reads when no unique Code work exists")
		}
	}
	if report.KitID != kitID || !strings.Contains(report.Markdown, kitID) || !strings.Contains(report.Markdown, runtime.GOOS+"/"+runtime.GOARCH) {
		t.Fatal("shareable Markdown lost the actual host or bounded local kit identifier")
	}
	for _, name := range []string{"code_packet", "binding_admission", "preparation", "mcp_connection", "mcp_tools", "mcp_resources", "method_catalog"} {
		found := false
		for _, check := range report.Checks {
			if check.Name == name && check.Status == "passed" {
				found = true
			}
		}
		if !found {
			t.Fatalf("actual public diagnostic did not verify %s: %+v", name, report.Checks)
		}
	}
	archiveStatus := "failed"
	if desktopSupported {
		archiveStatus = "passed"
	}
	archiveFound := false
	for _, check := range report.Checks {
		if check.Name == "desktop_archive" && check.Status == archiveStatus {
			archiveFound = true
		}
	}
	if !archiveFound {
		t.Fatal("unsupported native Desktop must remain unavailable while actual Code SDK proof still runs")
	}
	if desktopSupported && (report.DesktopExtension != "Maestro MCP — Ensaio "+report.WorkspaceID[:8] || !strings.Contains(report.DesktopPrompt, report.DesktopExtension)) {
		t.Fatal("Desktop prompt does not identify this exact local extension")
	}
	if !reflect.DeepEqual(before, snapshot()) {
		t.Fatal("automatic diagnostic changed packet, workspace or owner files")
	}
	if desktopSupported {
		isolated := options.Destination + ".isolated"
		if err := os.Rename(options.Destination, isolated); err != nil {
			t.Fatal(err)
		}
		if err := packaging.BuildExtension(options); err != nil {
			t.Fatal(err)
		}
		var legacy bytes.Buffer
		if err := runPilotCheck(ctx, args, &legacy); err != nil {
			t.Fatalf("legacy generic rehearsal refused: %v", err)
		}
		var generic pilotReport
		if err := json.Unmarshal(legacy.Bytes(), &generic); err != nil || generic.DesktopExtension != "" || generic.DesktopPrompt == "" {
			t.Fatal("generic archive was silently converted to an isolated profile")
		}
		if err := os.Remove(options.Destination); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(isolated, options.Destination); err != nil {
			t.Fatal(err)
		}
	}
	canceled, stop := context.WithCancel(context.Background())
	stop()
	if err := probePilotPacket(canceled, filepath.Join(base, "exports", "code"), options.Binding, report.WorkspaceID, &pilotReport{}); err == nil {
		t.Fatal("canceled diagnostic started a successful MCP session")
	}
	if err := os.WriteFile(filepath.Join(base, "preparation.json"), []byte(`{"state":"ready","workspace_id":"00000000000000000000000000000000"}`), 0600); err != nil {
		t.Fatal(err)
	}
	var blocked bytes.Buffer
	if err := runPilotCheck(ctx, args, &blocked); err == nil {
		t.Fatal("mismatched expected workspace admitted")
	}
	if err := json.Unmarshal(blocked.Bytes(), &report); err != nil || report.Status != "blocked" || report.CodePrompt != "" {
		t.Fatalf("workspace mismatch published unsafe guidance: %s", blocked.String())
	}
	assertPilotReadinessPrompt(t, blocked.Bytes(), "", false)
}

func sha256Sum(body []byte) []byte { sum := sha256.Sum256(body); return sum[:] }

// The emitted launchers extract this public report field even when admission fails.
func assertPilotReadinessPrompt(t *testing.T, body []byte, workspaceID string, admitted bool) {
	t.Helper()
	var report map[string]json.RawMessage
	if err := json.Unmarshal(body, &report); err != nil {
		t.Fatal(err)
	}
	var prompt string
	field, present := report["code_ready_prompt"]
	if !present || json.Unmarshal(field, &prompt) != nil {
		t.Fatal("public diagnostic must emit code_ready_prompt as a string, including blocked reports")
	}
	var desktopPrompt string
	desktopField, desktopPresent := report["desktop_ready_prompt"]
	if !desktopPresent || json.Unmarshal(desktopField, &desktopPrompt) != nil {
		t.Fatal("public diagnostic must emit desktop_ready_prompt as a string, including blocked reports")
	}
	if !admitted {
		if prompt != "" || desktopPrompt != "" {
			t.Fatal("blocked or unbound diagnostic released a Code connection prompt")
		}
		return
	}
	for _, required := range []string{"plugin:maestro-product:maestro", "maestro_context", "uma única vez", "sem parâmetros", workspaceID, "ready"} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("Code connection prompt omitted native plugin selection or its single bound read: %s", required)
		}
	}
	if strings.Count(prompt, "maestro_context") != 1 {
		t.Fatal("Code connection prompt must request only one context call")
	}
	for _, forbidden := range []string{"maestro_apply", "maestro_query", "execution.", "brain.", "owner.", "memory.", "methods.", "--mcp-config", "--strict-mcp-config"} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("read-only Code connection prompt expanded the journey: %s", forbidden)
		}
	}

	extension := "Maestro MCP — Ensaio " + workspaceID[:8]
	for _, required := range []string{extension, "extensão", "maestro_context", "uma única vez", "sem parâmetros", workspaceID, "ready"} {
		if !strings.Contains(desktopPrompt, required) {
			t.Fatalf("Desktop connection prompt omitted its unique extension or single bound read: %s", required)
		}
	}
	if strings.Count(desktopPrompt, "maestro_context") != 1 {
		t.Fatal("Desktop connection prompt must request only one context call")
	}
	for _, forbidden := range []string{"maestro_apply", "maestro_query", "execution.", "brain.", "owner.", "memory.", "methods.", "--mcp-config", "--strict-mcp-config"} {
		if strings.Contains(desktopPrompt, forbidden) {
			t.Fatalf("read-only Desktop connection prompt expanded the journey: %s", forbidden)
		}
	}
}

func TestPilotRuntimePromptsMatchBundledDocument(t *testing.T) {
	_, source, _, _ := runtime.Caller(0)
	fixture := filepath.Join(filepath.Dir(source), "..", "..", "..", "..", "internal", "ingest", "docling", "testdata", "sample.docx")
	archive, err := zip.OpenReader(fixture)
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	var texts []string
	var rows [][]string
	for _, entry := range archive.File {
		if entry.Name != "word/document.xml" {
			continue
		}
		stream, err := entry.Open()
		if err != nil {
			t.Fatal(err)
		}
		decoder := xml.NewDecoder(io.LimitReader(stream, 1<<20))
		var row []string
		cell := ""
		inCell := false
		for {
			token, err := decoder.Token()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatal(err)
			}
			switch element := token.(type) {
			case xml.StartElement:
				switch element.Name.Local {
				case "tr":
					row = nil
				case "tc":
					cell, inCell = "", true
				case "t":
					var text string
					if err := decoder.DecodeElement(&text, &element); err != nil {
						t.Fatal(err)
					}
					texts = append(texts, text)
					if inCell {
						cell += text
					}
				}
			case xml.EndElement:
				switch element.Name.Local {
				case "tc":
					row, inCell = append(row, cell), false
				case "tr":
					rows = append(rows, row)
				}
			}
		}
		if err := stream.Close(); err != nil {
			t.Fatal(err)
		}
	}
	const marker = "MAESTRO_DOCLING_OFFLINE_20261101"
	if !strings.Contains(strings.Join(texts, "\n"), marker) || len(rows) < 2 {
		t.Fatal("bundled DOCX lost the independent marker/table fixture")
	}
	actualRows := map[string]bool{}
	for _, row := range rows[1:] {
		actualRows[strings.Join(row, "/")] = true
	}
	report := pilotReport{Status: "partial", WorkspaceID: strings.Repeat("a", 32), Runtime: &packaging.EngineeringRuntimeBinding{PackSHA256: strings.Repeat("b", 64)}}
	var output bytes.Buffer
	if err := finishPilotReport(&report, &output); err != nil {
		t.Fatal(err)
	}
	for _, prompt := range []string{report.CodePrompt, report.DesktopPrompt} {
		if !strings.Contains(prompt, marker) || !strings.Contains(prompt, "maestro-pilot.docx") {
			t.Fatal("runtime prompt lost its bundled source/marker")
		}
		pairs := regexp.MustCompile(`[a-z]+/[0-9]+`).FindAllString(prompt, -1)
		if len(pairs) != len(actualRows) {
			t.Fatal("runtime prompt and actual DOCX have different table expectations", pairs, actualRows)
		}
		for _, pair := range pairs {
			if !actualRows[pair] {
				t.Fatal("runtime prompt requires a row absent from the bundled DOCX", pair)
			}
		}
	}
}

func TestPilotCheckBlockedReportIsStructuredAndPrivate(t *testing.T) {
	t.Setenv("MAESTRO_HOME", "")
	base := t.TempDir()
	var output bytes.Buffer
	err := runPilotCheck(context.Background(), []string{"--product-root", filepath.Join(base, "product"), "--user-home", filepath.Join(base, "owner"), "--workspace", filepath.Join(base, "workspace")}, &output)
	if err == nil {
		t.Fatal("unprepared binding must block")
	}
	var report pilotReport
	if err := json.Unmarshal(output.Bytes(), &report); err != nil {
		t.Fatalf("blocked preparation lost its machine-readable report: %v", err)
	}
	if report.SchemaVersion != 1 || report.Status != "blocked" || len(report.Checks) == 0 || report.WorkspaceID != "" || report.CodePrompt != "" || report.DesktopPrompt != "" {
		t.Fatalf("blocked report advertises a prepared client: %+v", report)
	}
	assertPilotReadinessPrompt(t, output.Bytes(), "", false)
	if strings.Contains(output.String(), base) {
		t.Fatal("shareable diagnostic exposed a local private path")
	}
	if _, err := os.Stat(filepath.Join(base, "owner")); !os.IsNotExist(err) {
		t.Fatal("read-only check created owner state")
	}
}

func TestPilotCheckRejectsHomeOverrideWithoutLeakingItsValue(t *testing.T) {
	base := t.TempDir()
	private := filepath.Join(base, "private-owner-never-selected")
	t.Setenv("MAESTRO_HOME", private)
	var output bytes.Buffer
	err := runPilotCheck(context.Background(), []string{"--product-root", filepath.Join(base, "product"), "--user-home", filepath.Join(base, "owner"), "--workspace", filepath.Join(base, "workspace")}, &output)
	if err == nil {
		t.Fatal("ambient home override admitted")
	}
	var report pilotReport
	if err := json.Unmarshal(output.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if report.Status != "blocked" || len(report.Checks) != 2 || report.Checks[1].Reason != "home_override_present" || report.WorkspaceID != "" || strings.Contains(output.String(), private) {
		t.Fatalf("override refusal leaked or inspected another binding: %s", output.String())
	}
	if _, err := os.Stat(private); !os.IsNotExist(err) {
		t.Fatal("override refusal created foreign state")
	}
}

func TestPilotCheckRejectsUnsupportedFlagsAsJSON(t *testing.T) {
	for _, args := range [][]string{{"--repair"}, {"--product-root", "product"}, {"unexpected"}} {
		var output bytes.Buffer
		if err := runPilotCheck(context.Background(), args, &output); err == nil {
			t.Fatal("invalid command accepted")
		}
		var report pilotReport
		if json.Unmarshal(output.Bytes(), &report) != nil || report.Status != "blocked" {
			t.Fatalf("invalid flags must still return JSON: %q", output.String())
		}
	}
}

// The public pilot command must coexist with the ordinary extension and start
// with its admitted paths, without requiring client-owned configuration.
func assertDesktopPilotManifest(t *testing.T, archive, owner, work string, preparation []byte) {
	t.Helper()
	var prepared struct {
		WorkspaceID string `json:"workspace_id"`
	}
	if err := json.Unmarshal(preparation, &prepared); err != nil {
		t.Fatal(err)
	}
	reader, err := zip.OpenReader(archive)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	if runtime.GOOS == "darwin" {
		for _, file := range reader.File {
			want := os.FileMode(0600)
			if file.Name == "server/maestro-mcp" {
				want = 0700
			}
			if file.Mode().Perm() != want {
				t.Fatalf("Darwin Desktop pilot does not emit exact owner-only roles: %s %04o, want %04o", file.Name, file.Mode().Perm(), want)
			}
		}
	}
	for _, file := range reader.File {
		if file.Name != "manifest.json" {
			continue
		}
		f, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(f)
		f.Close()
		if err != nil {
			t.Fatal(err)
		}
		var manifest struct {
			Name        string          `json:"name"`
			DisplayName string          `json:"display_name"`
			UserConfig  json.RawMessage `json:"user_config"`
			Server      struct {
				Config struct {
					Args []string `json:"args"`
				} `json:"mcp_config"`
			} `json:"server"`
		}
		if err := json.Unmarshal(body, &manifest); err != nil {
			t.Fatal(err)
		}
		if manifest.Name != "maestro-pilot-"+prepared.WorkspaceID || manifest.DisplayName != "Maestro MCP — Ensaio "+prepared.WorkspaceID[:8] {
			t.Fatalf("Desktop identity is not isolated: %+v", manifest)
		}
		if len(manifest.UserConfig) != 0 {
			t.Fatal("Desktop pilot still requires client-owned user_config before server launch")
		}
		wantArgs := []string{"serve", "--packet", "--product-root", "${__dirname}", "--user-home", owner, "--workspace", work}
		if !reflect.DeepEqual(manifest.Server.Config.Args, wantArgs) {
			t.Fatalf("Desktop server is not bound to its prepared paths: %q", manifest.Server.Config.Args)
		}
		return
	}
	t.Fatal("Desktop manifest missing")
}

// Stay in the existing native command admission family: startup independently
// enforces the prepared identity even if a caller changes the selected paths.
func assertDesktopPilotRuntimeSelection(t *testing.T, binary, archive string, binding product.Options) {
	t.Helper()
	packet := filepath.Join(t.TempDir(), "desktop packet")
	reader, err := zip.OpenReader(archive)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	for _, entry := range reader.File {
		if !filepath.IsLocal(filepath.FromSlash(entry.Name)) || !entry.Mode().IsRegular() {
			t.Fatal("unsafe emitted test packet")
		}
		path := filepath.Join(packet, filepath.FromSlash(entry.Name))
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		input, err := entry.Open()
		if err != nil {
			t.Fatal(err)
		}
		output, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, entry.Mode().Perm())
		if err != nil {
			input.Close()
			t.Fatal(err)
		}
		_, copyErr := io.Copy(output, input)
		closeErr := output.Close()
		input.Close()
		if copyErr != nil || closeErr != nil {
			t.Fatal("packet extraction failed", copyErr, closeErr)
		}
	}
	server := filepath.Join(packet, "server", filepath.Base(binary))
	packetArgs := func(owner, work string) []string {
		return []string{"--packet", "--product-root", packet, "--user-home", owner, "--workspace", work}
	}
	if out, err := exec.Command(server, append([]string{"inspect"}, packetArgs(binding.UserHome, binding.WorkspacePath)...)...).CombinedOutput(); err != nil {
		t.Fatalf("correct Desktop binding refused: %v %s", err, out)
	}
	assertDesktopPilotNativeMaterialization(t, binary, packet, archive, binding)
	alternate := filepath.Join(t.TempDir(), "other rehearsal")
	otherOwner, otherWork := filepath.Join(alternate, "owner"), filepath.Join(alternate, "workspace")
	if err := os.MkdirAll(otherOwner, 0700); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(binary, "init", "--product-root", binding.ProductRoot, "--user-home", otherOwner, "--workspace", otherWork).CombinedOutput(); err != nil {
		t.Fatalf("alternate admitted test binding: %v %s", err, out)
	}
	refuse := func(reason, owner, work string) {
		t.Helper()
		for _, command := range []string{"init", "inspect", "serve"} {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			_, err := exec.CommandContext(ctx, server, append([]string{command}, packetArgs(owner, work)...)...).CombinedOutput()
			cancel()
			if err == nil || ctx.Err() == context.DeadlineExceeded {
				t.Fatalf("Desktop packet %s accepted %s selection: %v", command, reason, err)
			}
		}
	}
	refuse("other admitted paths", otherOwner, otherWork)
	// Workspace IDs derive from canonical paths. Exercise a stale/corrupt
	// expected ID against real admitted state without inventing ID rotation.
	status, err := packaging.ValidateArchive(archive)
	if err != nil || status.DesktopPilot == nil {
		t.Fatal("local Desktop binding unavailable", err)
	}
	wrong := *status.DesktopPilot
	wrong.WorkspaceID = strings.Repeat("0", 32)
	if wrong.WorkspaceID == status.DesktopPilot.WorkspaceID {
		wrong.WorkspaceID = strings.Repeat("1", 32)
	}
	if err := wrong.ValidateSelection(binding); err == nil || !strings.Contains(err.Error(), "identity mismatch") {
		t.Fatalf("Desktop binding accepted wrong current workspace identity: %v", err)
	}
	inventoryPath := filepath.Join(packet, packaging.InventoryName)
	original, err := os.ReadFile(inventoryPath)
	if err != nil {
		t.Fatal(err)
	}
	var inventory map[string]json.RawMessage
	if err := json.Unmarshal(original, &inventory); err != nil {
		t.Fatal(err)
	}
	for _, token := range []string{"${__dirname}", "${user_config.workspace}"} {
		wrong := *status.DesktopPilot
		wrong.UserHome = filepath.Join(filepath.Dir(binding.UserHome), token, "owner")
		wrong.Workspace = filepath.Join(filepath.Dir(binding.UserHome), token, "workspace")
		inventory["desktop_pilot"], err = json.Marshal(wrong)
		if err != nil {
			t.Fatal(err)
		}
		body, err := json.Marshal(inventory)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(inventoryPath, body, 0644); err != nil {
			t.Fatal(err)
		}
		if _, err := packaging.ValidatePacket(packet); err == nil || !strings.Contains(err.Error(), "without ${") {
			t.Fatalf("canonical packet accepted or misdiagnosed reserved MCPB interpolation: %v", err)
		}
	}
	if err := os.WriteFile(inventoryPath, original, 0644); err != nil {
		t.Fatal(err)
	}
}

func desktopDirectoryAlias(t *testing.T, alias, target string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		shell := filepath.Join(os.Getenv("SystemRoot"), "System32", "cmd.exe")
		if out, err := exec.Command(shell, "/c", "mklink", "/J", alias, target).CombinedOutput(); err != nil {
			t.Fatalf("ordinary-user junction fixture: %v %s", err, out)
		}
		return
	}
	if err := os.Symlink(target, alias); err != nil {
		t.Fatal(err)
	}
}

func assertDesktopPilotNativeMaterialization(t *testing.T, binary, packet, archive string, binding product.Options) {
	t.Helper()
	status, err := packaging.ValidatePacket(packet)
	if err != nil || status.DesktopPilot == nil {
		t.Fatal("physical Desktop packet unavailable", err)
	}
	alias := filepath.Join(t.TempDir(), "application support alias")
	desktopDirectoryAlias(t, alias, filepath.Dir(packet))
	selected := filepath.Join(alias, filepath.Base(packet))
	if _, err := packaging.ValidatePacket(selected); err == nil {
		t.Fatal("strict packet API unexpectedly followed an ancestor alias")
	}
	var manifest struct {
		Server struct {
			Config struct {
				Command string   `json:"command"`
				Args    []string `json:"args"`
			} `json:"mcp_config"`
		} `json:"server"`
	}
	body, err := os.ReadFile(filepath.Join(packet, "manifest.json"))
	if err != nil || json.Unmarshal(body, &manifest) != nil {
		t.Fatal("emitted Desktop configuration unavailable", err)
	}
	replace := strings.NewReplacer("${__dirname}", selected)
	executable := replace.Replace(manifest.Server.Config.Command)
	args := append([]string(nil), manifest.Server.Config.Args...)
	for i := range args {
		args[i] = replace.Replace(args[i])
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, executable, args...)
	if runtime.GOOS == "darwin" {
		vector := append([]string{"-c", `umask 077; exec "$@"`, "desktop-materialization", executable}, args...)
		cmd = exec.CommandContext(ctx, "/bin/sh", vector...)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	client := mcp.NewClient(&mcp.Implementation{Name: "desktop-materialization-contract", Version: "1"}, nil)
	session, err := client.Connect(ctx, &mcp.CommandTransport{Command: cmd, TerminateDuration: time.Second}, nil)
	if err != nil {
		// Failure-only synthetic paths distinguish packet materialization from
		// executable startup without changing the product's redacted diagnostics.
		parent, parentErr := managedPacketParent(filepath.Dir(selected))
		physical := filepath.Join(parent, filepath.Base(selected))
		physicalStatus, physicalErr := packaging.ValidatePacket(physical)
		expectedImage, imageErr := filepath.EvalSymlinks(executable)
		physicalServer := filepath.Join(physical, "server", filepath.Base(binary))
		serverBody, serverErr := os.ReadFile(physicalServer)
		sameImage := expectedImage == physicalServer
		if runtime.GOOS == "windows" {
			sameImage = strings.EqualFold(expectedImage, physicalServer)
		}
		t.Logf("desktop_materialization physical_root=%q parent_error=%v physical_validation=%v native_mode=%q physical_server=%q hint_eval_path=%q hint_eval_error=%v hint_eval_path_equal=%t image_read=%v image_hash_equal=%t", physical, parentErr, physicalErr, physicalStatus.NativeMode, physicalServer, expectedImage, imageErr, sameImage, serverErr, fmt.Sprintf("%x", sha256.Sum256(serverBody)) == physicalStatus.ServerSHA256)
		for _, probe := range [][]string{{"client-capabilities"}, append([]string{"inspect"}, args[1:]...)} {
			probeContext, probeCancel := context.WithTimeout(context.Background(), 5*time.Second)
			var probeStderr bytes.Buffer
			probeCommand := exec.CommandContext(probeContext, executable, probe...)
			probeCommand.Stderr = &probeStderr
			probeErr := probeCommand.Run()
			t.Logf("desktop_materialization public_probe=%s error=%v timed_out=%t stderr=%q", probe[0], probeErr, probeContext.Err() != nil, probeStderr.String())
			probeCancel()
		}
		t.Fatalf("actual emitted Desktop server cannot initialize through its ancestor alias: %v %s", err, stderr.String())
	}
	var metadata struct {
		WorkspaceID string `json:"workspace_id"`
		State       string `json:"state"`
	}
	err = pilotCall(ctx, session, "maestro_context", map[string]any{}, &metadata)
	closeErr := session.Close()
	if err != nil || closeErr != nil || metadata.WorkspaceID != status.DesktopPilot.WorkspaceID || metadata.State != "ready" {
		t.Fatalf("emitted aliased Desktop context mismatch: %v %v %+v", err, closeErr, metadata)
	}
	reject := func(reason, command string, selectedArgs []string) {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_, err := exec.CommandContext(ctx, command, selectedArgs...).CombinedOutput()
		cancel()
		if err == nil || ctx.Err() == context.DeadlineExceeded {
			t.Fatalf("Desktop startup accepted %s: %v", reason, err)
		}
	}
	reject("another executing image", binary, args)
	leaf := filepath.Join(t.TempDir(), "linked packet")
	desktopDirectoryAlias(t, leaf, packet)
	leafArgs := append([]string(nil), args...)
	leafArgs[3] = leaf
	reject("linked managed root leaf", filepath.Join(leaf, "server", filepath.Base(binary)), leafArgs)
	code := filepath.Join(filepath.Dir(archive), "code")
	codeAlias := filepath.Join(t.TempDir(), "code ancestor")
	desktopDirectoryAlias(t, codeAlias, filepath.Dir(code))
	code = filepath.Join(codeAlias, filepath.Base(code))
	codeArgs := append([]string(nil), args...)
	codeArgs[3] = code
	reject("non-pilot Code ancestor alias", filepath.Join(code, "scripts", filepath.Base(binary)), codeArgs)
	for index, target := range map[int]string{5: binding.UserHome, 7: binding.WorkspacePath} {
		alias := filepath.Join(t.TempDir(), "private state alias")
		desktopDirectoryAlias(t, alias, target)
		changed := append([]string(nil), args...)
		changed[index] = alias
		reject("owner/workspace alias", executable, changed)
	}
	managed, held := filepath.Join(packet, "managed"), filepath.Join(t.TempDir(), "held managed")
	if err := os.Rename(managed, held); err != nil {
		t.Fatal(err)
	}
	desktopDirectoryAlias(t, managed, held)
	reject("linked inner managed directory", executable, args)
	if err := os.Remove(managed); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(held, managed); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "darwin" {
		for _, name := range []string{"LICENSE.md", packaging.InventoryName} {
			path := filepath.Join(packet, name)
			for _, mode := range []os.FileMode{0400, 0644} {
				if err := os.Chmod(path, mode); err != nil {
					t.Fatal(err)
				}
				reject("changed exact private mode", executable, args)
			}
			if err := os.Chmod(path, 0600); err != nil {
				t.Fatal(err)
			}
		}
	}
	if _, err := packaging.ValidatePacket(packet); err != nil {
		t.Fatal("negative fixtures changed the original packet", err)
	}
}
