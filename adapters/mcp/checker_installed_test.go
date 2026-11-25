package product

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/datahome"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installer"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// This test-only child supplies the fixture registry pin and legacy carrier
// selection, then enters the real installed admission and serving owners. It
// adds no shipping selector, verifier override or installed/native claim.
func init() {
	if len(os.Args) < 2 || os.Args[1] != "installed-check-serve" || os.Getenv("MAESTRO_TEST_INSTALLED_CHECK_PIN") == "" {
		return
	}
	fields := map[string]string{}
	for i := 2; i+1 < len(os.Args); i += 2 {
		fields[os.Args[i]] = os.Args[i+1]
	}
	ctx := context.Background()
	fixed, err := installer.LoadExperimentalInstalledContext(fields["--product-root"], os.Getenv("MAESTRO_TEST_INSTALLED_CHECK_PIN"), time.Now)
	if err == nil {
		carrier := os.Getenv("MAESTRO_TEST_INSTALLED_CHECK_CARRIER")
		selected := installer.ClientOptions{SeededOperationOptions: fixed, Request: installer.ClientRequest{CarrierID: carrier, Workspace: fields["--workspace"]}}
		var capability *installer.VerifiedStableCheck
		capability, err = installer.AdmitStableCheckServer(ctx, selected)
		if err == nil {
			var b *Bound
			b, err = Open(Options{ExperimentalSeed: fixed.ExperimentalSeed, ExperimentalSeedRoot: fixed.ManagedRoot, ProductRoot: fixed.ManagedRoot, UserHome: fixed.UserHome, WorkspacePath: selected.Request.Workspace, AuthorityRegistrySHA256: fixed.AuthorityRegistrySHA256, CLIVersion: "0.2.0", InstalledManifestSHA256: fields["--installed-manifest"], InstalledClientInputsSHA256: fields["--installed-client-inputs"], ManagementCarrierID: carrier, ManagementInstallerVersion: fixed.Version})
			if err == nil {
				err = b.ServeInstalledCheck(ctx, &mcp.StdioTransport{}, capability)
			}
		}
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(0)
}

func TestInstalledCheckerAuthenticatesRealChildAndRefusesChangedArgv(t *testing.T) {
	f := newProductSeedFixtureWithInstaller(t, true, true)
	home, err := datahome.Resolve(datahome.Options{ProductRoot: f.options.ProductRoot, UserHome: f.options.UserHome})
	if err != nil {
		t.Fatal(err)
	}
	b, err := Open(f.options)
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(filepath.Dir(f.options.ProductRoot), "clients")
	code, desktop := filepath.Join(root, "code"), filepath.Join(root, "desktop.mcpb")
	if err = os.MkdirAll(code, 0700); err != nil {
		t.Fatal(err)
	}
	payload := filepath.Join(code, "fixture.txt")
	if err = os.WriteFile(payload, []byte("synthetic checker packet"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(desktop, []byte("synthetic checker archive"), 0600); err != nil {
		t.Fatal(err)
	}
	codeSHA, err := installer.ClientTreeDigest(code)
	if err != nil {
		t.Fatal(err)
	}
	desktopSHA, err := installer.ClientArchiveDigest(desktop)
	if err != nil {
		t.Fatal(err)
	}
	carrier := strings.Repeat("a", 32)
	record := installer.ClientRecord{SchemaVersion: 1, Profile: installer.StableClientProfile, Protocol: installer.StableClientProtocol, CarrierVersion: installer.StableCarrierVersion, CarrierID: carrier, Binding: installer.ClientBinding{ManagedRoot: f.options.ProductRoot, UserHome: f.options.UserHome, Home: home.Home, Workspace: f.options.WorkspacePath, WorkspaceID: b.workspaceID, RegistrySHA256: f.options.AuthorityRegistrySHA256}, Materialization: installer.ClientMaterialization{CodeRoot: code, DesktopPath: desktop, Release: f.state.Release, ManifestSHA256: f.state.Binding.ManifestSHA256, ClientInputsSHA256: f.state.Binding.ClientInputsSHA256, ServerSHA256: f.state.Server.SHA256, EmbeddedInputsSHA256: f.state.Server.EmbeddedInputsSHA256, BundleSHA256: f.state.Binding.BundleSHA256, CodeTreeSHA256: codeSHA, DesktopSHA256: desktopSHA, DesktopNativeContractSHA256: strings.Repeat("b", 64), BootstrapServerSHA256: f.state.Server.SHA256, BootstrapEmbeddedInputsSHA256: f.state.Server.EmbeddedInputsSHA256}, Code: installer.CodeObservation{Scope: "user", State: "registration_pending", EvidenceClass: "not_observed"}}
	body, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	records := filepath.Join(f.options.UserHome, ".maestro-install", "clients")
	if err = os.MkdirAll(records, 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(records, carrier+".json"), body, 0600); err != nil {
		t.Fatal(err)
	}
	name := "maestro-install"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	selected := installer.ClientOptions{SeededOperationOptions: installer.SeededOperationOptions{ExperimentalSeed: f.options.ExperimentalSeed, Self: filepath.Join(f.options.ProductRoot, name), Version: "0.2.0", AuthorityRegistrySHA256: f.options.AuthorityRegistrySHA256, ManagedRoot: f.options.ProductRoot, UserHome: f.options.UserHome, DataRoot: filepath.Join(f.options.UserHome, ".maestro-install")}, Request: installer.ClientRequest{CarrierID: carrier, Workspace: f.options.WorkspacePath}}
	launch, err := installer.PrepareStableCheckLaunch(context.Background(), selected)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("MAESTRO_TEST_INSTALLED_CHECK_PIN", f.options.AuthorityRegistrySHA256)
	t.Setenv("MAESTRO_TEST_INSTALLED_CHECK_CARRIER", carrier)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, launch.Executable, launch.Args...)
	client, err := mcp.NewClient(&mcp.Implementation{Name: "authenticated-installed-check", Version: "1"}, nil).Connect(ctx, &mcp.CommandTransport{Command: cmd}, nil)
	if err != nil {
		t.Fatal("real admitted checker child refused", err)
	}
	tools, err := client.ListTools(ctx, nil)
	if err != nil || len(tools.Tools) != 3 {
		t.Fatalf("checker lost ordinary tool inventory: %v %v", tools, err)
	}
	if err = client.Close(); err != nil {
		t.Fatal(err)
	}
	if cmd.ProcessState == nil || !cmd.ProcessState.Exited() {
		t.Fatal("checker transport did not reap its owned child")
	}
	for _, extra := range [][]string{{"--purpose", "unknown"}, {"--workspace", f.options.WorkspacePath}} {
		changed := exec.CommandContext(ctx, launch.Executable, append(append([]string{}, launch.Args...), extra...)...)
		if output, err := changed.CombinedOutput(); err == nil {
			t.Fatalf("noncanonical child argv accepted: %q", output)
		}
	}
	if err = os.WriteFile(payload, []byte("tampered packet"), 0600); err != nil {
		t.Fatal(err)
	}
	changed := exec.CommandContext(ctx, launch.Executable, launch.Args...)
	if output, err := changed.CombinedOutput(); err == nil {
		t.Fatalf("changed current packet admitted: %q", output)
	}
}
