package updateplan

import (
	"bytes"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installtx"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
)

func buildModern(current installtx.State, m releasecontract.Manifest, osName, arch string, source SourceBinding) (Plan, error) {
	if current.Source != nil && current.Source.RecallCapacityExhausted {
		return Plan{}, errors.New("source recall capacity exhausted; update plan unavailable")
	}
	if current.SchemaVersion != 4 || current.Profile != releasecontract.ModernProfile || !m.Modern() || source.Modern == nil || current.Source == nil || current.Source.SourceBinding != *source.Modern || source.Provider != "synced-folder" || source.ProviderReleaseID != 0 || source.ManifestSHA256 == "" {
		return Plan{}, errors.New("modern standard update unavailable without authenticated enrolled source-aware current state")
	}
	if m.Server.ExecutionContract != releasecontract.SyncedSourceContract || current.Server.ExecutionContract != releasecontract.SyncedSourceContract {
		return Plan{}, errors.New("source-aware execution contract unavailable")
	}
	if m.Channel != current.Channel || m.Channel != source.Modern.Tuple.Channel || current.TargetOS != osName || current.TargetArch != arch || source.Modern.Tuple.OS != osName || source.Modern.Tuple.Arch != arch {
		return Plan{}, errors.New("source update target/channel mismatch")
	}
	comparison, err := CompareVersions(m.Release, current.Release)
	if err != nil || comparison <= 0 {
		return Plan{}, errors.New("standard update must move to a newer release")
	}
	var server installtx.ExecutableIdentity
	var bundle string
	for _, a := range m.Artifacts {
		if a.Kind == "mcp_server" && a.OS == osName && a.Arch == arch {
			if server.ArtifactName != "" {
				return Plan{}, errors.New("duplicate target server")
			}
			server = installtx.ExecutableIdentity{Version: m.Server.Version, Protocol: m.Server.Protocol, CompatibleBundle: m.Server.CompatibleBundle, CompatibleServer: m.Bundle.CompatibleServer, EmbeddedInputsSHA256: m.Server.EmbeddedInputsSHA256, ExecutionContract: m.Server.ExecutionContract, ArtifactName: a.Name, SHA256: a.SHA256, Size: a.Size}
		}
		if a.Kind == "bundle" {
			if bundle != "" {
				return Plan{}, errors.New("duplicate bundle")
			}
			bundle = a.Name
		}
	}
	if server.ArtifactName == "" || bundle == "" {
		return Plan{}, errors.New("signed release lacks target server/bundle")
	}
	p := Plan{SchemaVersion: 3, Profile: releasecontract.ModernProfile, Server: server, Source: *source.Modern, FromStateSHA256: installtx.StateDigest(current), ClientInputsSHA256: m.Delivery.ClientInputsSHA256, InstallerVersion: m.Delivery.InstallerVersion, State: "available", FromRelease: current.Release, FromChannel: current.Channel, FromCLIVersion: current.Server.Version, FromBundleVersion: current.BundleVersion, ToRelease: m.Release, Channel: m.Channel, CLIVersion: m.Server.Version, BundleVersion: m.Bundle.Version, CLIArtifact: server.ArtifactName, BundleArtifact: bundle, TargetOS: osName, TargetArch: arch, Provider: "synced-folder", ManifestSHA256: source.ManifestSHA256, ConfirmationRequired: true}
	p.ID, err = computePlanID(p)
	if err != nil {
		return Plan{}, err
	}
	return p, validateModern(p)
}

func validateModern(p Plan) error {
	hash := func(v string) bool { return len(v) == 64 && strings.Trim(v, "0123456789abcdef") == "" }
	if p.SchemaVersion != 3 || p.Profile != releasecontract.ModernProfile || p.State != "available" || !p.ConfirmationRequired || p.Provider != "synced-folder" || p.ProviderReleaseID != 0 || !hash(p.ManifestSHA256) || !hash(p.ClientInputsSHA256) || !hash(p.FromStateSHA256) || !hash(p.Source.GenerationSHA256) || p.Source.Sequence == 0 || p.Source.Tuple.Validate() != nil || p.Source.Tuple.OS != p.TargetOS || p.Source.Tuple.Arch != p.TargetArch || p.Source.Tuple.Channel != p.Channel || p.Channel != p.FromChannel || p.Server.ExecutionContract != releasecontract.SyncedSourceContract || p.Server.Version != p.CLIVersion || p.Server.ArtifactName != p.CLIArtifact || p.BundleArtifact == "" || p.WorkspaceMigrationRequired || p.WorkspaceMigrationVersion != 0 || p.WorkspaceMigrationState != "" || p.WorkspaceMigrationExecution != "" || p.RoleMigrationID != "" || p.CatalogSHA256 != "" || p.PolicySHA256 != "" {
		return errors.New("invalid modern update plan")
	}
	if p.Server.Protocol != releasecontract.ServerProtocol || !hash(p.Server.SHA256) || !hash(p.Server.EmbeddedInputsSHA256) || p.Server.Size <= 0 || p.CLIArtifact == "" || filepath.Base(p.CLIArtifact) != p.CLIArtifact || filepath.Base(p.BundleArtifact) != p.BundleArtifact || (p.TargetOS != "darwin" && p.TargetOS != "windows") {
		return errors.New("modern update server/target binding invalid")
	}
	if _, err := releasecontract.ParseVersionRange(p.Server.CompatibleBundle); err != nil {
		return err
	}
	if _, err := releasecontract.ParseVersionRange(p.Server.CompatibleServer); err != nil {
		return err
	}
	for _, v := range []string{p.FromRelease, p.ToRelease, p.FromCLIVersion, p.FromBundleVersion, p.CLIVersion, p.BundleVersion, p.InstallerVersion} {
		if _, err := parseVersion(v); err != nil {
			return err
		}
	}
	c, err := CompareVersions(p.ToRelease, p.FromRelease)
	if err != nil || c <= 0 {
		return errors.New("modern update must move to a newer release")
	}
	id, err := computePlanID(p)
	if err != nil || id != p.ID {
		return errors.New("modern update plan digest mismatch")
	}
	return nil
}

func (p Plan) MarshalJSON() ([]byte, error) {
	type raw Plan
	body, err := json.Marshal(raw(p))
	if err != nil {
		return nil, err
	}
	if p.Profile == "" {
		index := bytes.Index(body, []byte(`"schema_version"`))
		if index < 0 {
			return nil, errors.New("legacy plan schema missing")
		}
		return append([]byte{'{'}, body[index:]...), nil
	}
	var f map[string]json.RawMessage
	if err = json.Unmarshal(body, &f); err != nil {
		return nil, err
	}
	for a, b := range map[string]string{"from_cli_version": "from_server_version", "cli_version": "server_version", "cli_artifact": "server_artifact"} {
		f[b] = f[a]
		delete(f, a)
	}
	delete(f, "provider_release_id")
	return json.Marshal(f)
}

func (p *Plan) UnmarshalJSON(body []byte) error {
	var f map[string]json.RawMessage
	if err := releasecontract.DecodeStrictJSON(body, &f); err != nil {
		return err
	}
	var profile string
	if raw, ok := f["profile"]; ok {
		if err := json.Unmarshal(raw, &profile); err != nil {
			return err
		}
		if profile != releasecontract.ModernProfile {
			return errors.New("unsupported update profile")
		}
	}
	if profile != "" {
		for a, b := range map[string]string{"from_cli_version": "from_server_version", "cli_version": "server_version", "cli_artifact": "server_artifact"} {
			if _, ok := f[a]; ok {
				return errors.New("modern plan contains historical CLI fields")
			}
			f[a] = f[b]
			delete(f, b)
		}
		if _, ok := f["provider_release_id"]; ok {
			return errors.New("modern source cannot carry GitHub release ID")
		}
	}
	normalized, err := json.Marshal(f)
	if err != nil {
		return err
	}
	type raw Plan
	var v raw
	d := json.NewDecoder(bytes.NewReader(normalized))
	d.DisallowUnknownFields()
	if err = d.Decode(&v); err != nil {
		return err
	}
	*p = Plan(v)
	return nil
}
