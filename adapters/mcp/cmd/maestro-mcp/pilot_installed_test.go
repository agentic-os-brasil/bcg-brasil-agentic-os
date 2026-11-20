package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	product "github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp/packaging"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/datahome"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installer"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installtx"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspace"
)

// This tests metadata comparison after the real archive verifier, never host
// admission or native qualification. A retained bootstrap is not current code.
func TestInstalledPilotDesktopComparisonKeepsHistoricalBootstrapSeparate(t *testing.T) {
	hash := func(c string) string { return strings.Repeat(c, 64) }
	carrier := packaging.StableCarrier{Profile: installer.StableClientProfile, Protocol: installer.StableClientProtocol, ID: strings.Repeat("a", 32), Version: installer.StableCarrierVersion}
	host := packaging.Target{OS: "darwin", Arch: "arm64"}
	record := installer.ClientRecord{CarrierID: carrier.ID, Profile: carrier.Profile, Protocol: carrier.Protocol, CarrierVersion: carrier.Version, Binding: installer.ClientBinding{RegistrySHA256: hash("e")}, Materialization: installer.ClientMaterialization{ServerSHA256: hash("a"), BootstrapServerSHA256: hash("b"), BootstrapEmbeddedInputsSHA256: hash("c"), DesktopNativeContractSHA256: hash("d")}}
	archive := packaging.PacketStatus{Format: "mcpb", Target: host, ServerSHA256: hash("b"), EmbeddedInputsSHA256: hash("c"), NativeContractSHA256: hash("d"), RegistrySHA256: hash("e"), Stable: &carrier}
	if err := installedDesktopMatches(record, archive, host); err != nil {
		t.Fatalf("compatible historical bootstrap was confused with current server: %v", err)
	}
	for name, mutate := range map[string]func(*packaging.PacketStatus){
		"changed bootstrap":       func(s *packaging.PacketStatus) { s.ServerSHA256 = hash("a") },
		"changed embedded inputs": func(s *packaging.PacketStatus) { s.EmbeddedInputsSHA256 = hash("f") },
		"changed native contract": func(s *packaging.PacketStatus) { s.NativeContractSHA256 = hash("f") },
		"foreign registry":        func(s *packaging.PacketStatus) { s.RegistrySHA256 = hash("f") },
		"foreign carrier":         func(s *packaging.PacketStatus) { c := *s.Stable; c.ID = strings.Repeat("f", 32); s.Stable = &c },
		"missing carrier":         func(s *packaging.PacketStatus) { s.Stable = nil },
		"foreign format":          func(s *packaging.PacketStatus) { s.Format = "plugin" },
		"foreign target":          func(s *packaging.PacketStatus) { s.Target.OS = "windows" },
	} {
		t.Run(name, func(t *testing.T) {
			changed := archive
			mutate(&changed)
			if err := installedDesktopMatches(record, changed, host); err == nil {
				t.Fatal("changed contract admitted")
			}
		})
	}
}

func TestInstalledPilotPromptsSelectExactAdmittedStableConnector(t *testing.T) {
	record := installer.ClientRecord{CarrierID: strings.Repeat("a", 32), Profile: installer.StableClientProfile, Protocol: installer.StableClientProtocol, CarrierVersion: installer.StableCarrierVersion, Materialization: installer.ClientMaterialization{ServerSHA256: strings.Repeat("b", 64), BootstrapServerSHA256: strings.Repeat("c", 64), DesktopNativeContractSHA256: strings.Repeat("d", 64)}}
	identity, err := installedDesktopExtensionName(record)
	if err != nil {
		t.Fatal(err)
	}
	r := installedPilotReport{pilotReport: pilotReport{SchemaVersion: 2, Status: "partial", WorkspaceID: strings.Repeat("e", 32), DesktopExtension: identity}}
	var output bytes.Buffer
	if err := finishInstalledPilotReport(&r, &output); err != nil {
		t.Fatal(err)
	}
	for _, prompt := range []string{r.DesktopPrompt, r.DesktopReadyPrompt, r.Markdown} {
		if !strings.Contains(prompt, "maestro-connector-"+record.CarrierID) || strings.Contains(prompt, `extensão "maestro-product"`) {
			t.Fatalf("prompt/report selects a foreign or absent Desktop connector: %s", prompt)
		}
	}
	for _, change := range []string{"carrier", "contract", "profile"} {
		bad := record
		switch change {
		case "carrier":
			bad.CarrierID = "foreign"
		case "contract":
			bad.Materialization.DesktopNativeContractSHA256 = ""
		case "profile":
			bad.Profile = "foreign"
		}
		if _, err := installedDesktopExtensionName(bad); err == nil {
			t.Fatal("unadmitted connector identity formatted")
		}
	}
}

func TestInstalledPilotSelectionRejectsUnclosedOrForeignInputs(t *testing.T) {
	root := t.TempDir()
	base := []string{"--product-root", filepath.Join(root, "installed"), "--user-home", filepath.Join(root, "participant"), "--workspace", filepath.Join(root, "existing-work"), "--carrier-id", strings.Repeat("a", 32), "--installer-version", "0.1.51", "--experimental-seed-sha256", strings.Repeat("b", 64), "--experimental-installed-root", filepath.Join(root, "installed"), "--check-only"}
	selected, err := parseInstalledPilotSelection(base)
	if err != nil || selected.CarrierID != strings.Repeat("a", 32) || selected.Workspace != filepath.Join(root, "existing-work") {
		t.Fatalf("exact selections refused: %#v %v", selected, err)
	}
	for name, args := range map[string][]string{
		"missing":                nil,
		"unknown executable":     append(append([]string{}, base...), "--server", "/private/secret"),
		"duplicate selection":    append(append([]string{}, base...), "--carrier-id", strings.Repeat("c", 32)),
		"foreign installed root": append(append([]string{}, base[:len(base)-3]...), "--experimental-installed-root", filepath.Join(root, "foreign")),
		"relative product":       {"--product-root", "relative"},
		"positional":             append(append([]string{}, base...), "other"),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := parseInstalledPilotSelection(args); err == nil {
				t.Fatal("unclosed selection admitted")
			}
		})
	}
}

func TestInstalledPilotBlockedReportIsPrivateAndDoesNotInitialize(t *testing.T) {
	secret := "/private/DO_NOT_EXPORT_PARTICIPANT_SELECTION"
	var output bytes.Buffer
	if err := runInstalledPilotCheck(context.Background(), []string{"--server", secret}, &output); err == nil {
		t.Fatal("unknown source accepted")
	}
	if strings.Contains(output.String(), secret) {
		t.Fatal("private selection leaked")
	}
	var report struct {
		SchemaVersion int          `json:"schema_version"`
		Profile       string       `json:"profile"`
		Evidence      string       `json:"evidence_class"`
		Status        string       `json:"status"`
		Checks        []pilotCheck `json:"checks"`
	}
	if err := json.Unmarshal(output.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if report.SchemaVersion != 2 || report.Profile != installer.ExperimentalSeedProfile || report.Evidence != "engineering_only" || report.Status != "blocked" || len(report.Checks) != 1 || report.Checks[0].Name != "installed_selection" {
		t.Fatalf("untruthful blocked report: %#v", report)
	}
	root := t.TempDir()
	installed, participant, work := filepath.Join(root, "installed"), filepath.Join(root, "participant"), filepath.Join(root, "work")
	output.Reset()
	args := []string{"--product-root", installed, "--user-home", participant, "--workspace", work, "--carrier-id", strings.Repeat("a", 32), "--installer-version", "0.1.51", "--experimental-seed-sha256", strings.Repeat("b", 64), "--experimental-installed-root", installed}
	if err := runInstalledPilotCheck(context.Background(), args, &output); err == nil {
		t.Fatal("missing role admitted")
	}
	for _, path := range []string{installed, participant, work} {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Fatal("blocked checker initialized participant state")
		}
	}
	if strings.Contains(output.String(), root) {
		t.Fatal("role refusal exported private path")
	}
}

func TestInstalledPilotCheckerRejectsStaleOrForeignExecutingImage(t *testing.T) {
	root := t.TempDir()
	name := "maestro-mcp"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	path := filepath.Join(root, "bin", name)
	state := installtx.State{TargetOS: runtime.GOOS, TargetArch: runtime.GOARCH, Server: installtx.ExecutableIdentity{Version: "0.1.52", SHA256: strings.Repeat("a", 64)}}
	if !installedCurrentImageMatches(path, state.Server.SHA256, "0.1.52", root, state) {
		t.Fatal("exact image refused")
	}
	for name, args := range map[string][3]string{
		"old bytes":    {path, strings.Repeat("b", 64), "0.1.52"},
		"old build":    {path, state.Server.SHA256, "0.1.51"},
		"foreign path": {filepath.Join(root, "other", name), state.Server.SHA256, "0.1.52"},
		"unversioned":  {path, state.Server.SHA256, ""},
	} {
		t.Run(name, func(t *testing.T) {
			if installedCurrentImageMatches(args[0], args[1], args[2], root, state) {
				t.Fatal("stale or foreign checker admitted")
			}
		})
	}
}

// This ordinary synthetic binding exercises only the shared read-only SDK helper,
// not experimental seed admission, a native client or a qualified installation.
func TestInstalledPilotSDKReadsWithoutStartingMaintenanceOrWritingOwnerFiles(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("ordinary-user Windows owner admission has its separate native qualification")
	}
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	o := product.Options{ProductRoot: filepath.Join(root, "product"), UserHome: filepath.Join(root, "owner"), WorkspacePath: filepath.Join(root, "work"), CLIVersion: "0.1.51", LookupEnv: func(string) (string, bool) { return "", false }}
	for _, path := range []string{o.ProductRoot, o.UserHome} {
		if err := os.Mkdir(path, 0700); err != nil {
			t.Fatal(err)
		}
	}
	home, err := datahome.InitializeFresh(datahome.Options{ProductRoot: o.ProductRoot, UserHome: o.UserHome, LookupEnv: o.LookupEnv})
	if err != nil {
		t.Fatal(err)
	}
	space, err := workspace.Initialize(workspace.Options{WorkspacePath: o.WorkspacePath, DataRoot: filepath.Join(home.Home, "state")})
	if err != nil {
		t.Fatal(err)
	}
	snapshot := func() map[string]string {
		t.Helper()
		out := map[string]string{}
		if err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			info, err := d.Info()
			if err != nil {
				return err
			}
			if d.IsDir() {
				out[path] = "directory:" + info.Mode().String()
				return nil
			}
			b, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			out[path] = fmt.Sprintf("%s:%x", info.Mode(), sha256.Sum256(b))
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		return out
	}
	before := snapshot()
	r := pilotReport{SchemaVersion: 2, Status: "partial", Checks: []pilotCheck{}}
	if err := installedPilotSDK(context.Background(), o, space.WorkspaceID, strings.Repeat("b", 64), strings.Repeat("a", 64), &r); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, snapshot()) {
		t.Fatal("read-only SDK changed owner or workspace files")
	}
	if len(r.Checks) < 3 || r.Checks[0].Reason != "current_in_process_sdk_context_verified_not_host_loading" {
		t.Fatalf("incorrect SDK evidence: %#v", r.Checks)
	}
}

func TestInstalledPilotCodeExecutableUsesNativeHostContract(t *testing.T) {
	root := t.TempDir()
	// A bounded Mach-O executable header is sufficient for read-only identity;
	// none of these synthetic files is ever executed.
	header := make([]byte, 32)
	for i, value := range []uint32{0xfeedfacf, 0x0100000c, 0, 2} {
		binary.LittleEndian.PutUint32(header[i*4:], value)
	}
	write := func(name string, body []byte) string {
		t.Helper()
		path := filepath.Join(root, name)
		if err := os.WriteFile(path, body, 0700); err != nil {
			t.Fatal(err)
		}
		return path
	}
	regular := write("native-regular", header)
	native := write("native", header)
	linked := filepath.Join(root, "native-hardlink")
	if err := os.Link(native, linked); err != nil {
		t.Fatal(err)
	}
	large := write("native-large", header)
	if err := os.Truncate(large, (128<<20)+1); err != nil {
		t.Fatal(err)
	}
	invalid := write("invalid", []byte("not an executable"))
	for _, tc := range []struct {
		name, path string
		matches    bool
	}{
		{"regular host", regular, true},
		{"hardlinked host", linked, true},
		{"host above packet limit", large, true},
		{"invalid native format", invalid, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			file, err := os.Open(tc.path)
			if err != nil {
				t.Fatal(err)
			}
			hash := sha256.New()
			_, copyErr := io.Copy(hash, file)
			closeErr := file.Close()
			if copyErr != nil || closeErr != nil {
				t.Fatal(copyErr, closeErr)
			}
			code := installer.CodeObservation{Executable: tc.path, ExecutableSHA256: fmt.Sprintf("%x", hash.Sum(nil))}
			if got := installedCodeExecutableMatches(code); got != tc.matches {
				t.Fatalf("installed diagnostic identity match = %v, want %v", got, tc.matches)
			}
			changed, err := os.OpenFile(tc.path, os.O_WRONLY|os.O_APPEND, 0)
			if err != nil {
				t.Fatal(err)
			}
			_, writeErr := changed.Write([]byte("changed"))
			closeErr = changed.Close()
			if writeErr != nil || closeErr != nil {
				t.Fatal(writeErr, closeErr)
			}
			if installedCodeExecutableMatches(code) {
				t.Fatal("changed executable bytes were admitted")
			}
		})
	}
	if installedCodeExecutableMatches(installer.CodeObservation{Executable: filepath.Join(root, "missing"), ExecutableSHA256: strings.Repeat("a", 64)}) {
		t.Fatal("missing executable was admitted")
	}
}
