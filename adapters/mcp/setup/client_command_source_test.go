package setup

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installtx"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installer"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
)

func TestInterruptedVersionedDesktopReuseRequiresAuthenticatedPriorBinding(t *testing.T) {
	tests := []struct {
		name   string
		change func(*installer.ClientOptions, *installer.ClientBinding, **installer.ClientRecord, *string)
		ok     bool
	}{
		{name: "ExactAdmittedPriorReuse", ok: true},
		{name: "NoPrior", change: func(_ *installer.ClientOptions, _ *installer.ClientBinding, r **installer.ClientRecord, _ *string) {
			*r = nil
		}},
		{name: "FixedSlotPrior", change: func(_ *installer.ClientOptions, _ *installer.ClientBinding, r **installer.ClientRecord, _ *string) {
			(*r).Materialization.Selection = nil
		}},
		{name: "OrdinaryFirstPublication", change: func(o *installer.ClientOptions, _ *installer.ClientBinding, _ **installer.ClientRecord, _ *string) {
			o.VersionedManagement = false
		}},
		{name: "ManualReplacement", change: func(o *installer.ClientOptions, _ *installer.ClientBinding, _ **installer.ClientRecord, _ *string) {
			o.Request.ManualDesktopUpdate = true
		}},
		{name: "WrongSchema", change: func(_ *installer.ClientOptions, _ *installer.ClientBinding, r **installer.ClientRecord, _ *string) {
			(*r).SchemaVersion = 2
		}},
		{name: "WrongProfile", change: func(_ *installer.ClientOptions, _ *installer.ClientBinding, r **installer.ClientRecord, _ *string) {
			(*r).Profile = "foreign"
		}},
		{name: "WrongProtocol", change: func(_ *installer.ClientOptions, _ *installer.ClientBinding, r **installer.ClientRecord, _ *string) {
			(*r).Protocol = "foreign"
		}},
		{name: "WrongCarrierVersion", change: func(_ *installer.ClientOptions, _ *installer.ClientBinding, r **installer.ClientRecord, _ *string) {
			(*r).CarrierVersion = "9.0.0"
		}},
		{name: "WrongCarrierID", change: func(_ *installer.ClientOptions, _ *installer.ClientBinding, r **installer.ClientRecord, _ *string) {
			(*r).CarrierID = strings.Repeat("f", 32)
		}},
		{name: "WrongFullBinding", change: func(_ *installer.ClientOptions, _ *installer.ClientBinding, r **installer.ClientRecord, _ *string) {
			(*r).Binding.Home += "-other"
		}},
		{name: "WrongRecordedWorkspace", change: func(_ *installer.ClientOptions, _ *installer.ClientBinding, r **installer.ClientRecord, _ *string) {
			(*r).Binding.Workspace += "-other"
		}},
		{name: "WrongActualWorkspace", change: func(o *installer.ClientOptions, _ *installer.ClientBinding, _ **installer.ClientRecord, _ *string) {
			o.Request.Workspace += "-other"
		}},
		{name: "WrongCurrentNativeContract", change: func(_ *installer.ClientOptions, _ *installer.ClientBinding, _ **installer.ClientRecord, native *string) {
			*native = strings.Repeat("f", 64)
		}},
		{name: "MissingBootstrapIdentity", change: func(_ *installer.ClientOptions, _ *installer.ClientBinding, r **installer.ClientRecord, _ *string) {
			(*r).Materialization.BootstrapServerSHA256 = ""
		}},
		{name: "WrongPackageHash", change: func(_ *installer.ClientOptions, _ *installer.ClientBinding, r **installer.ClientRecord, _ *string) {
			(*r).Materialization.DesktopSHA256 = strings.Repeat("f", 64)
		}},
		{name: "ChangedDestination"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			o := installer.ClientOptions{SeededOperationOptions: installer.SeededOperationOptions{ManagedRoot: filepath.Join(root, "managed"), UserHome: filepath.Join(root, "owner"), AuthorityRegistrySHA256: strings.Repeat("a", 64)}, Request: installer.ClientRequest{CarrierID: strings.Repeat("b", 32), Workspace: filepath.Join(root, "workspace"), DesktopPath: filepath.Join(root, "new.mcpb")}, VersionedManagement: true}
			binding := installer.ClientBinding{ManagedRoot: o.ManagedRoot, UserHome: o.UserHome, Home: filepath.Join(root, "brain"), Workspace: o.Request.Workspace, WorkspaceID: strings.Repeat("c", 32), RegistrySHA256: o.AuthorityRegistrySHA256}
			launcher := "maestro-install"
			if runtime.GOOS == "windows" {
				launcher += ".exe"
			}
			manifest, err := json.Marshal(map[string]any{"server": map[string]any{"mcp_config": map[string]any{"command": filepath.Join(o.ManagedRoot, launcher), "args": []string{"stable-launch", "--product-root", o.ManagedRoot, "--user-home", o.UserHome, "--workspace", o.Request.Workspace, "--carrier-id", o.Request.CarrierID}}}})
			if err != nil {
				t.Fatal(err)
			}
			var archive bytes.Buffer
			zw := zip.NewWriter(&archive)
			entry, err := zw.Create("manifest.json")
			if err != nil {
				t.Fatal(err)
			}
			if _, err = entry.Write(manifest); err != nil {
				t.Fatal(err)
			}
			if err = zw.Close(); err != nil {
				t.Fatal(err)
			}
			priorPath := filepath.Join(root, "prior.mcpb")
			for _, path := range []string{priorPath, o.Request.DesktopPath} {
				if err = os.WriteFile(path, archive.Bytes(), 0600); err != nil {
					t.Fatal(err)
				}
			}
			packageSHA, err := installer.ClientArchiveDigest(priorPath)
			if err != nil {
				t.Fatal(err)
			}
			native := fmt.Sprintf("%x", sha256.Sum256(manifest))
			// This unit seam receives a previously authenticated private record.
			// Its historical inert resources are not current canonical assets.
			prior := &installer.ClientRecord{SchemaVersion: 1, Profile: installer.StableClientProfile, Protocol: installer.StableClientProtocol, CarrierVersion: installer.StableCarrierVersion, CarrierID: o.Request.CarrierID, Binding: binding, Materialization: installer.ClientMaterialization{DesktopPath: priorPath, DesktopSHA256: packageSHA, BootstrapServerSHA256: strings.Repeat("d", 64), BootstrapEmbeddedInputsSHA256: strings.Repeat("e", 64), DesktopNativeContractSHA256: native, Selection: &installer.ClientPacketSelection{CodeRoot: filepath.Join(root, "selected-code"), DesktopPath: filepath.Join(root, "selected.mcpb")}}}
			if tt.change != nil {
				tt.change(&o, &binding, &prior, &native)
			}
			if tt.name == "ChangedDestination" {
				if err = os.WriteFile(o.Request.DesktopPath, []byte("foreign destination must survive\n"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			before, err := os.ReadFile(o.Request.DesktopPath)
			if err != nil {
				t.Fatal(err)
			}
			beforeInfo, err := os.Stat(o.Request.DesktopPath)
			if err != nil {
				t.Fatal(err)
			}
			priorInfo, err := os.Stat(priorPath)
			if err != nil {
				t.Fatal(err)
			}
			state := installtx.State{TargetOS: runtime.GOOS}
			err = inspectFirstDesktopContract(context.Background(), o, state, binding, prior, releasecontract.ServerIdentity{}, native)
			if (err == nil) != tt.ok {
				t.Fatalf("authenticated prior reuse=%v: %v", tt.ok, err)
			}
			after, readErr := os.ReadFile(o.Request.DesktopPath)
			old, oldErr := os.ReadFile(priorPath)
			info, statErr := os.Stat(o.Request.DesktopPath)
			priorAfterInfo, priorStatErr := os.Stat(priorPath)
			if readErr != nil || oldErr != nil || statErr != nil || priorStatErr != nil || !bytes.Equal(before, after) || !bytes.Equal(old, archive.Bytes()) || !os.SameFile(beforeInfo, info) || info.Mode() != beforeInfo.Mode() || !os.SameFile(priorInfo, priorAfterInfo) || priorAfterInfo.Mode() != priorInfo.Mode() {
				t.Fatal("observation changed prior or destination bytes/identity/mode", readErr, oldErr, statErr, priorStatErr)
			}
		})
	}
}

func TestVersionedCodeSourceUsesFixedShortLauncherOnMacAndWindows(t *testing.T) {
	for _, platform := range []string{"darwin", "windows"} {
		o := installer.ClientOptions{SeededOperationOptions: installer.SeededOperationOptions{Self: "/Users/participant/Developer/Maestro/maestro-install", ManagedRoot: "/Users/participant/Developer/Maestro", UserHome: "/Users/participant"}, Request: installer.ClientRequest{CarrierID: strings.Repeat("a", 32), CodeRoot: "/Users/participant/Developer/Maestro Clients/Code", Workspace: "/Users/participant/Developer/Brain", VersionedSlots: true}}
		if platform == "windows" {
			o.Self = `C:\Users\participant\Developer\Maestro\maestro-install.exe`
			o.ManagedRoot = `C:\Users\participant\Developer\Maestro`
			o.UserHome = `C:\Users\participant`
			o.Request.CodeRoot = `C:\Users\participant\Developer\Maestro Clients\Code`
			o.Request.Workspace = `C:\Users\participant\Developer\Brain`
		}
		_, files, name, err := commandSourceFiles(o, platform)
		if err != nil {
			t.Fatal(platform, err)
		}
		var manifest struct {
			Plugins []struct {
				Source struct {
					Source, Command, Mode string
					Timeout               int
				}
			}
		}
		if err := json.Unmarshal(files[".claude-plugin/marketplace.json"], &manifest); err != nil {
			t.Fatal(err)
		}
		s := manifest.Plugins[0].Source
		if s.Source != "command" || s.Mode != "copy" || s.Timeout != 30 || len(s.Command) > 500 || strings.Contains(s.Command, "--product-root") || !strings.Contains(string(files[name]), "client-plugin-path") {
			t.Fatalf("unsupported command source %s: %+v", platform, s)
		}
	}
}

func TestVersionedProviderPreservesChangedFilesAndForeignEntries(t *testing.T) {
	parent := t.TempDir()
	if err := os.Chmod(parent, 0700); err != nil {
		t.Fatal(err)
	}
	o := installer.ClientOptions{SeededOperationOptions: installer.SeededOperationOptions{Self: "/Users/participant/Developer/Maestro/maestro-install", ManagedRoot: "/Users/participant/Developer/Maestro", UserHome: "/Users/participant"}, Request: installer.ClientRequest{CodeRoot: filepath.Join(parent, "Code"), CarrierID: strings.Repeat("a", 32), Workspace: "/Users/participant/Developer/Brain"}}
	root, err := ensureCommandMarketplace(o)
	if err != nil {
		t.Fatal(err)
	}
	if again, err := ensureCommandMarketplace(o); err != nil || again != root {
		t.Fatal("fixed source not idempotent", err)
	}
	path := filepath.Join(root, ".claude-plugin", "marketplace.json")
	if err := os.WriteFile(path, []byte("foreign manifest\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := ensureCommandMarketplace(o); err == nil {
		t.Fatal("changed provider was replaced")
	}
	body, err := os.ReadFile(path)
	if err != nil || string(body) != "foreign manifest\n" {
		t.Fatal("foreign provider was not preserved", err)
	}
}

func TestFullNativeClientMaterializationHasNoResourceOnlyFallback(t *testing.T) {
	o := installer.ClientOptions{Request: installer.ClientRequest{NativeEnforcement: true}}
	if _, err := CurrentClientMaterializer(o)(context.Background(), installtx.State{}, installer.ClientBinding{}, nil); err == nil || !strings.Contains(err.Error(), "genuine qualifier") {
		t.Fatal("missing native proof downgraded profile", err)
	}
}

func TestExplicitNativeRetryUsesFreshReviewedSlotsAndPreservesFailedCandidate(t *testing.T) {
	root := t.TempDir()
	manifest := strings.Repeat("b", 64)
	o := installer.ClientOptions{Request: installer.ClientRequest{VersionedSlots: true, NativeEnforcement: true, CarrierID: strings.Repeat("a", 32), GenerationID: strings.Repeat("c", 32), CodeRoot: filepath.Join(root, "Code"), DesktopPath: filepath.Join(root, "Desktop.mcpb"), CodeExecutable: filepath.Join(root, "selected-code")}}
	state := installtx.State{Binding: installtx.ReleaseBinding{ManifestSHA256: manifest}}
	_, first, err := nativeClientCommand(o, state, NativeClientSelection{}, "native-review")
	if err != nil {
		t.Fatal(err)
	}
	destination := func(args []string) string {
		for i, arg := range args {
			if arg == "--destination" {
				return args[i+1]
			}
		}
		t.Fatal("missing exact native destination")
		return ""
	}
	failed := destination(first)
	if err := os.Mkdir(failed, 0700); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(failed, "maestro-packet.json")
	bytes := []byte(`{"state":"unavailable","reason":"native_challenge_evidence_incomplete"}`)
	if err := os.WriteFile(marker, bytes, 0600); err != nil {
		t.Fatal(err)
	}
	o.Request.GenerationID = strings.Repeat("d", 32)
	_, second, err := nativeClientCommand(o, state, NativeClientSelection{}, "native-review")
	if err != nil {
		t.Fatal(err)
	}
	retry := destination(second)
	if retry == failed || len(filepath.Base(retry)) > 100 {
		t.Fatal("retry reused/expanded failed slot", retry)
	}
	if _, err := os.Lstat(retry); !os.IsNotExist(err) {
		t.Fatal("retry slot already materialized", err)
	}
	if got, err := os.ReadFile(marker); err != nil || string(got) != string(bytes) {
		t.Fatal("failed qualifier evidence was changed", err)
	}
	o.Request.GenerationID = "not-a-reviewed-generation"
	if _, _, err := nativeClientCommand(o, state, NativeClientSelection{}, "native-review"); err == nil {
		t.Fatal("unbounded generation accepted")
	}
}

func TestMaintenanceRefreshSelectsOnlyPreviouslyQualifiedStaleJobs(t *testing.T) {
	status := ClientMaintenanceStatus{Jobs: []ClientMaintenanceJob{{JobID: "active-stale", State: "active", QualificationState: "stale", Qualification: json.RawMessage(`{"artifact_ref":"metadata-only"}`)}, {JobID: "paused-stale", State: "paused", QualificationState: "stale", Qualification: json.RawMessage(`{}`)}, {JobID: "new", State: "disabled", QualificationState: "unqualified"}, {JobID: "current", State: "active", QualificationState: "qualified", Qualification: json.RawMessage(`{}`)}}}
	jobs := MaintenanceRefreshCandidates(status)
	if len(jobs) != 2 || jobs[0].JobID != "active-stale" || jobs[1].State != "paused" {
		t.Fatalf("refresh changed opt-in/enrollment selection: %+v", jobs)
	}
}

func TestCompatibleDesktopCarrierReusesBytesWithoutChangingPriorSlot(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "prior.mcpb")
	target := filepath.Join(root, "new.mcpb")
	body := []byte("exact admitted stable carrier bytes\n")
	if err := os.WriteFile(source, body, 0600); err != nil {
		t.Fatal(err)
	}
	hash, err := installer.ClientArchiveDigest(source)
	if err != nil {
		t.Fatal(err)
	}
	if err := retainCurrentDesktopCarrier(source, target, hash); err != nil {
		t.Fatal(err)
	}
	if err := retainCurrentDesktopCarrier(source, target, hash); err != nil {
		t.Fatal("idempotent reuse refused", err)
	}
	if got, _ := os.ReadFile(source); string(got) != string(body) {
		t.Fatal("previous carrier changed")
	}
	if err := os.WriteFile(target, []byte("foreign"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := retainCurrentDesktopCarrier(source, target, hash); err == nil {
		t.Fatal("foreign generation overwritten")
	}
}

func TestVersionedCodeReadbackAcceptsOnlyMatchingNativeCopy(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	cache := filepath.Join(t.TempDir(), "copied-plugin")
	source := filepath.Join(root, "plugins", "maestro-product")
	for _, dir := range []string{source, cache} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "method.md"), []byte("current canonical method\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	hash, err := installer.ClientTreeDigest(root)
	if err != nil {
		t.Fatal(err)
	}
	id := strings.Repeat("a", 32)
	r := installer.ClientRecord{CarrierID: id, Binding: installer.ClientBinding{Workspace: t.TempDir()}, Materialization: installer.ClientMaterialization{CodeRoot: root, CodeTreeSHA256: hash, Selection: &installer.ClientPacketSelection{CodeRoot: filepath.Join(t.TempDir(), "selection")}, Release: "0.1.51"}, Code: installer.CodeObservation{Executable: executable, Marketplace: "maestro-local-" + id, PluginID: "maestro-product@maestro-local-" + id, Scope: "user"}}
	run := func(_ context.Context, _ string, args []string, _ string) ([]byte, error) {
		if args[0] == "--version" {
			return []byte("2.1.295"), nil
		}
		if args[len(args)-1] == "--help" {
			return []byte("--json --scope"), nil
		}
		if len(args) > 2 && args[1] == "marketplace" {
			return json.Marshal([]any{map[string]any{"name": r.Code.Marketplace, "source": "directory", "path": installer.ClientMarketplaceRoot(r)}})
		}
		return json.Marshal([]any{map[string]any{"id": r.Code.PluginID, "version": r.Materialization.Release, "scope": "user", "enabled": true, "installPath": cache, "readFromFolder": false}})
	}
	review, err := ReviewCodeRegistration(context.Background(), r, run)
	if err != nil || review.Observation.State != "registered" || len(review.Actions) != 0 {
		t.Fatalf("matching native copy rejected: %+v %v", review, err)
	}
	if err := os.WriteFile(filepath.Join(cache, "method.md"), []byte("foreign edit"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReviewCodeRegistration(context.Background(), r, run); err == nil {
		t.Fatal("modified native copy admitted")
	}
}
