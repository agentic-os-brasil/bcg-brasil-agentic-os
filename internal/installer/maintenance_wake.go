package installer

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installtx"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/macosadapter"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/windowsadapter"
)

// MaintenanceWakeBinding identifies an owned discovery pulse, never a job grant.
// Current release and record identities are observed separately so the native
// definition keeps its exact bytes across a same-installation source update.
type MaintenanceWakeBinding struct {
	ManagedRoot, UserHome, BrainHome, Workspace string
	WorkspaceID, CarrierID                      string
	AdmissionSHA256, RegistrySHA256             string
	InstallerVersion                            string
	SourceLocator                               string
	Source                                      releasecontract.SourceTuple
}

type MaintenanceWakeState struct {
	Binding         MaintenanceWakeBinding `json:"binding"`
	BindingSHA256   string                 `json:"binding_sha256"`
	ClientRecordSHA string                 `json:"client_record_sha256"`
	CurrentRelease  string                 `json:"current_release"`
	CurrentManifest string                 `json:"current_manifest"`
	Record          ClientRecord           `json:"-"`
}

type MaintenanceWakeCommand struct {
	ManagedRoot, UserHome, Workspace, CarrierID, BindingSHA256         string
	ManifestSHA256, ClientInputsSHA256, AdmissionSHA256, InstalledRoot string
}

func ParseMaintenanceWakeCommand(args []string, child bool) (MaintenanceWakeCommand, error) {
	var out MaintenanceWakeCommand
	fields := map[string]*string{"--product-root": &out.ManagedRoot, "--user-home": &out.UserHome, "--workspace": &out.Workspace, "--carrier-id": &out.CarrierID, "--binding-sha256": &out.BindingSHA256}
	if child {
		fields["--installed-manifest"], fields["--installed-client-inputs"] = &out.ManifestSHA256, &out.ClientInputsSHA256
		fields["--experimental-seed-sha256"], fields["--experimental-installed-root"] = &out.AdmissionSHA256, &out.InstalledRoot
	}
	if len(args) != len(fields)*2 {
		return out, errors.New("closed installed maintenance wake selection required")
	}
	for i := 0; i < len(args); i += 2 {
		value, ok := fields[args[i]]
		if !ok || *value != "" || args[i+1] == "" || len(args[i+1]) > 4096 || strings.ContainsAny(args[i+1], "\x00\r\n") {
			return out, errors.New("closed installed maintenance wake selection required")
		}
		*value = args[i+1]
	}
	for _, path := range []string{out.ManagedRoot, out.UserHome, out.Workspace} {
		if !filepath.IsAbs(path) || filepath.Clean(path) != path {
			return out, errors.New("canonical maintenance wake roots required")
		}
	}
	if !carrierIDPattern.MatchString(out.CarrierID) || !clientSHA.MatchString(out.BindingSHA256) {
		return out, errors.New("exact maintenance wake selectors required")
	}
	if child && (out.InstalledRoot != out.ManagedRoot || !clientSHA.MatchString(out.ManifestSHA256) || !clientSHA.MatchString(out.ClientInputsSHA256) || !clientSHA.MatchString(out.AdmissionSHA256)) {
		return out, errors.New("exact installed maintenance child pins required")
	}
	return out, nil
}

func (b MaintenanceWakeBinding) Digest() (string, error) {
	for _, path := range []string{b.ManagedRoot, b.UserHome, b.BrainHome, b.Workspace, b.SourceLocator} {
		if !filepath.IsAbs(path) || filepath.Clean(path) != path || strings.ContainsAny(path, "\x00\r\n") {
			return "", errors.New("exact installed maintenance wake paths required")
		}
	}
	if !carrierIDPattern.MatchString(b.CarrierID) || !carrierIDPattern.MatchString(b.WorkspaceID) || !clientSHA.MatchString(b.AdmissionSHA256) || !clientSHA.MatchString(b.RegistrySHA256) {
		return "", errors.New("exact installed maintenance wake identity required")
	}
	if b.InstallerVersion == "" || strings.ContainsAny(b.InstallerVersion, "\x00\r\n") {
		return "", errors.New("fixed installer version required")
	}
	if err := b.Source.Validate(); err != nil {
		return "", err
	}
	body, err := json.Marshal(b)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", sha256.Sum256(body)), nil
}

func (b MaintenanceWakeBinding) WindowsSpec() (windowsadapter.NativeWakeSpec, error) {
	digest, err := b.Digest()
	if err != nil {
		return windowsadapter.NativeWakeSpec{}, err
	}
	args := []string{"maintenance-wake", "--product-root", b.ManagedRoot, "--user-home", b.UserHome, "--workspace", b.Workspace, "--carrier-id", b.CarrierID, "--binding-sha256", digest}
	return windowsadapter.NativeWakeSpec{Name: "MaestroMaintenance_" + digest, Program: filepath.Join(b.ManagedRoot, "maestro-install.exe"), Arguments: windowsadapter.QuoteWakeArguments(args), WorkingDirectory: b.Workspace, BindingSHA256: digest}, nil
}

func (b MaintenanceWakeBinding) VerifyNativeWake(ctx context.Context, osHome string) error {
	switch runtime.GOOS {
	case "darwin":
		return b.VerifyMacWake(osHome)
	case "windows":
		return b.VerifyWindowsNativeWake(ctx)
	}
	return errors.New("native maintenance wake platform unavailable")
}

func MaintenanceWakeOSHome() (string, error) {
	if runtime.GOOS == "windows" {
		return "", nil
	}
	if runtime.GOOS != "darwin" {
		return "", errors.New("native wake platform unavailable")
	}
	lifecycle, err := macosadapter.NewCurrentLifecycle("", nil)
	return lifecycle.CurrentHome, err
}

func (b MaintenanceWakeBinding) MacSpec() (macosadapter.Spec, error) {
	digest, err := b.Digest()
	if err != nil {
		return macosadapter.Spec{}, err
	}
	return macosadapter.Spec{Label: "com.bcg.maestro.maintenance." + digest, Program: filepath.Join(b.ManagedRoot, "maestro-install"), Arguments: []string{"maintenance-wake", "--product-root", b.ManagedRoot, "--user-home", b.UserHome, "--workspace", b.Workspace, "--carrier-id", b.CarrierID, "--binding-sha256", digest}, StartInterval: 900, RunAtLoad: true}, nil
}

// VerifyMacWake is a read-only veto, including inside the existing publication
// guard. Pause/revoke use that same guard to close the definition before bootout.
func (b MaintenanceWakeBinding) VerifyMacWake(osHome string) error {
	spec, err := b.MacSpec()
	if err != nil {
		return err
	}
	status, err := macosadapter.ReadStatus(osHome, spec.Label)
	if err != nil {
		return err
	}
	file, err := scheduler.OpenRegularFile(status.Path)
	if err != nil {
		return err
	}
	err = scheduler.ValidatePrivateFile(file, 0600)
	closeErr := file.Close()
	if err != nil || closeErr != nil {
		return errors.Join(err, closeErr)
	}
	_, err = macosadapter.Verify(osHome, spec)
	return err
}

func ReadMaintenanceWake(ctx context.Context, o ClientOptions) (MaintenanceWakeState, error) {
	if o.ExperimentalSeed == nil {
		return MaintenanceWakeState{}, errors.New("explicit experimental installed maintenance wake required")
	}
	status, err := ReadClientsSeeded(ctx, o)
	if err != nil || status.Record == nil || status.State != "current_materialization" {
		return MaintenanceWakeState{}, errors.New("current admitted maintenance carrier required")
	}
	state, err := installtx.ValidateQualifiedLaunch(o.ManagedRoot, o.DataRoot, installtx.ReleaseBinding{}, time.Now())
	if err != nil || state.Source == nil {
		return MaintenanceWakeState{}, errors.New("current enrolled maintenance source required")
	}
	r := *status.Record
	b := MaintenanceWakeBinding{ManagedRoot: r.Binding.ManagedRoot, UserHome: r.Binding.UserHome, BrainHome: r.Binding.Home, Workspace: r.Binding.Workspace, WorkspaceID: r.Binding.WorkspaceID, CarrierID: r.CarrierID, AdmissionSHA256: o.ExperimentalSeed.Identity().AdmissionSHA256, RegistrySHA256: r.Binding.RegistrySHA256, InstallerVersion: o.ExperimentalSeed.Identity().InstallerVersion, Source: state.Source.Tuple, SourceLocator: state.Source.Locator}
	digest, err := b.Digest()
	if err != nil {
		return MaintenanceWakeState{}, err
	}
	return MaintenanceWakeState{Binding: b, BindingSHA256: digest, ClientRecordSHA: ClientRecordDigest(r), CurrentRelease: state.Release, CurrentManifest: state.Binding.ManifestSHA256, Record: r}, nil
}

// PrepareMaintenanceWake returns only the actual current server and original
// qualified release/role argv. It performs no OS enrollment or domain work.
func PrepareMaintenanceWake(ctx context.Context, o ClientOptions, expected string) (QualifiedLaunch, MaintenanceWakeState, error) {
	state, err := ReadMaintenanceWake(ctx, o)
	if err != nil || state.BindingSHA256 != expected {
		return QualifiedLaunch{}, state, errors.New("maintenance wake binding changed")
	}
	launch, err := PrepareQualifiedLaunch(ctx, QualifiedLaunchOptions{SeededOperationOptions: o.SeededOperationOptions, ExpectedBinding: installtx.ReleaseBinding{ManifestSHA256: state.Record.Materialization.ManifestSHA256, ClientInputsSHA256: state.Record.Materialization.ClientInputsSHA256}, Workspace: state.Binding.Workspace})
	if err != nil {
		return launch, state, err
	}
	if len(launch.Args) == 0 || launch.Args[0] != "serve" || runtime.GOOS != launch.State.TargetOS {
		return QualifiedLaunch{}, state, errors.New("canonical current maintenance launch required")
	}
	launch.Args = append([]string{"installed-maintenance-wake", "--carrier-id", state.Binding.CarrierID, "--binding-sha256", expected}, launch.Args[1:]...)
	return launch, state, nil
}
