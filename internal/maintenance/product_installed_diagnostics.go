package maintenance

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installer"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installtx"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releaseverify"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
	"path/filepath"
	"runtime"
)

// InstalledSelection is trusted native constructor input. Every observation
// independently reads its canonical owners; these pins are not evidence.
type InstalledSelection struct {
	ManifestSHA256, ClientInputsSHA256 string
	CarrierID, InstallerVersion        string
}

// InstalledConfiguration describes authenticated configuration only. No client
// session, hook, trusted review, OS wake or native capability is qualified here.
type InstalledConfiguration struct {
	Profile            string `json:"profile"`
	State              string `json:"state"`
	Release            string `json:"release"`
	ManifestSHA256     string `json:"manifest_sha256"`
	ClientInputsSHA256 string `json:"client_inputs_sha256"`
	ClientState        string `json:"client_state"`
}

func installedInstallerName() string {
	if runtime.GOOS == "windows" {
		return "maestro-install.exe"
	}
	return "maestro-install"
}

func (s *Service) installedRuntimeDiagnosis(out RuntimeDiagnosis) (RuntimeDiagnosis, error) {
	ctx := s.options.DiagnosticContext
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return out, err
	}
	selected := s.options.Installed
	if len(selected.ManifestSHA256) != 64 || !digestPattern.MatchString(selected.ManifestSHA256) || len(selected.ClientInputsSHA256) != 64 || !digestPattern.MatchString(selected.ClientInputsSHA256) || (selected.CarrierID == "") != (selected.InstallerVersion == "") {
		return out, errors.New("installed diagnostic selection unavailable")
	}
	registryPath := filepath.Join(s.options.ProductRoot, "trust", "release-authority-registry.json")
	if s.options.AuthorityRegistryPath != "" && s.options.AuthorityRegistryPath != registryPath {
		return out, errors.New("installed diagnostic registry unavailable")
	}
	if _, err := releaseverify.LoadPinnedAuthorityRegistry(registryPath, s.options.AuthorityRegistrySHA256, s.now); err != nil {
		return out, err
	}
	dataRoot := filepath.Join(s.options.UserHome, ".maestro-install")
	statePath := filepath.Join(dataRoot, "config", "install-state.json")
	before, err := scheduler.ReadPrivateStateFile(statePath, 1<<20)
	if err != nil {
		return out, err
	}
	// Select only the modern profile here; ReadSourceState is the canonical
	// read-only owner and never performs the legacy installation migration.
	var header struct {
		SchemaVersion int    `json:"schema_version"`
		Profile       string `json:"profile"`
	}
	if json.Unmarshal(before, &header) != nil || header.SchemaVersion != 4 || header.Profile != releasecontract.ModernProfile {
		return out, errors.New("modern installed diagnostic state required")
	}
	seed := installer.SeededOperationOptions{Self: filepath.Join(s.options.ProductRoot, installedInstallerName()), UserHome: s.options.UserHome, ManagedRoot: s.options.ProductRoot, DataRoot: dataRoot, AuthorityRegistrySHA256: s.options.AuthorityRegistrySHA256, Version: selected.InstallerVersion}
	if s.options.SourceAdmission != nil {
		role, ok := s.options.SourceAdmission.(*installer.VerifiedExperimentalSeed)
		if !ok || role == nil {
			return out, errors.New("installed diagnostic admission unavailable")
		}
		seed.ExperimentalSeed = role
		seed.Version = role.Identity().InstallerVersion
		if selected.InstallerVersion != "" && selected.InstallerVersion != seed.Version {
			return out, errors.New("installed diagnostic installer changed")
		}
		if err := installer.VerifyExperimentalInstalledBindingContext(ctx, seed); err != nil {
			return out, err
		}
	}
	state, err := installtx.ReadSourceState(dataRoot, s.options.ProductRoot)
	if err != nil {
		return out, err
	}
	expected := installtx.ReleaseBinding{ManifestSHA256: selected.ManifestSHA256, ClientInputsSHA256: selected.ClientInputsSHA256}
	if state.Source != nil {
		state, err = installtx.ValidateQualifiedLaunchContext(ctx, s.options.ProductRoot, dataRoot, expected, s.now())
	} else {
		state, err = installtx.ValidateActivatedCurrentContext(ctx, s.options.ProductRoot, dataRoot, expected)
	}
	if err != nil {
		return out, err
	}
	if state.Profile != releasecontract.ModernProfile || state.Server.SHA256 != s.build || state.Server.Version != s.options.CLIVersion {
		return out, errors.New("installed diagnostic current executable changed")
	}
	registration, _, err := observeFile(s.options.WorkspacePath, ".bcgos/workspace.json", "workspace_registration")
	if err != nil {
		return out, err
	}
	out.Observations = []SourceObservation{registration, {Source: "modern_installation_state", State: "observed", SHA256: installtx.StateDigest(state)}, {Source: "running_executable", State: "observed", SHA256: s.build}}
	out.Installed = &InstalledConfiguration{Profile: state.Profile, State: "observed_configuration", Release: state.Release, ManifestSHA256: state.Binding.ManifestSHA256, ClientInputsSHA256: state.Binding.ClientInputsSHA256, ClientState: "unattested"}
	client := SourceObservation{Source: "modern_client_materialization", State: "unattested"}
	if selected.CarrierID != "" {
		status, err := installer.ReadClientsSeeded(ctx, installer.ClientOptions{SeededOperationOptions: seed, Request: installer.ClientRequest{CarrierID: selected.CarrierID, Workspace: s.options.WorkspacePath}, LookupEnv: s.options.LookupEnv})
		if err != nil || status.Record == nil {
			return out, errors.New("installed diagnostic client unavailable")
		}
		r := *status.Record
		if status.State != "current_materialization" {
			if status.State != installer.ClientPartialPreparation || !r.CodePreparationPending() || installer.ClientSelectedTarget(r) != "" {
				return out, errors.New("installed diagnostic client changed")
			}
			if _, err := installer.DesktopPreparationRecord(r); err != nil {
				return out, err
			}
		}
		if r.Binding.Home != s.home || r.Binding.Workspace != s.options.WorkspacePath || r.Binding.WorkspaceID != s.workspaceID || r.Materialization.ManifestSHA256 != state.Binding.ManifestSHA256 || r.Materialization.ClientInputsSHA256 != state.Binding.ClientInputsSHA256 {
			return out, errors.New("installed diagnostic client binding changed")
		}
		client.State, client.SHA256 = "observed", installer.ClientRecordDigest(r)
		out.Installed.ClientState = status.State
	}
	out.Observations = append(out.Observations, client)
	current, err := installtx.ReadSourceState(dataRoot, s.options.ProductRoot)
	if err != nil || installtx.StateDigest(current) != installtx.StateDigest(state) {
		return out, errors.New("installed diagnostic current changed during observation")
	}
	after, err := scheduler.ReadPrivateStateFile(statePath, 1<<20)
	if err != nil || rawDigest(before) != rawDigest(after) {
		return out, errors.New("installed diagnostic custody changed during observation")
	}
	if seed.ExperimentalSeed != nil {
		if err := installer.VerifyExperimentalInstalledBindingContext(ctx, seed); err != nil {
			return out, err
		}
	}
	out.InstallationCoverage = "configured"
	return s.withUnavailableCapabilities(out)
}
