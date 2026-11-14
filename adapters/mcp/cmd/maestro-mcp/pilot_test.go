package main

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	product "github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp/packaging"
)

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
	if desktopSupported && (!strings.Contains(report.CodePrompt, report.WorkspaceID) || !strings.Contains(report.DesktopPrompt, report.WorkspaceID)) || !desktopSupported && (report.CodePrompt != "" || report.DesktopPrompt != "") {
		t.Fatal("client guidance ignored native Desktop availability")
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
}

func sha256Sum(body []byte) []byte { sum := sha256.Sum256(body); return sum[:] }

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

// The public pilot command must coexist with the ordinary extension and prefill
// only this admitted rehearsal's paths before the client asks for confirmation.
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
			Name        string `json:"name"`
			DisplayName string `json:"display_name"`
			UserConfig  map[string]struct {
				Default, Type string
				Required      bool
			} `json:"user_config"`
		}
		if err := json.Unmarshal(body, &manifest); err != nil {
			t.Fatal(err)
		}
		if manifest.Name != "maestro-pilot-"+prepared.WorkspaceID || manifest.DisplayName != "Maestro MCP — Ensaio "+prepared.WorkspaceID[:8] {
			t.Fatalf("Desktop identity is not isolated: %+v", manifest)
		}
		for key, path := range map[string]string{"user_home": owner, "workspace": work} {
			field := manifest.UserConfig[key]
			if field.Default != path || field.Type != "directory" || !field.Required {
				t.Fatalf("Desktop selection %s is not prefilled and confirmed: %+v", key, field)
			}
		}
		return
	}
	t.Fatal("Desktop manifest missing")
}

// Stay in the existing native command admission family: Desktop configuration
// remains editable, so startup must enforce the prepared identity independently.
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
}
