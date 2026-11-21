package installer

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releaseverify"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

const (
	ExperimentalSeedProfile      = "engineering-installed-seed-v1"
	ExperimentalSeedPurpose      = "maestro-experimental-installed-seed-source-rehearsal-v1"
	ExperimentalSeedProtocol     = "maestro-experimental-installed-seed-v1"
	maximumExperimentalSeedBytes = 128 << 10
)

var experimentalSeedVersionPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)
var experimentalSeedIssuerPattern = regexp.MustCompile(`^maestro-engineering-pilot-[0-9]{8}$`)
var experimentalSeedKeyPattern = regexp.MustCompile(`^pilot-[0-9]{8}$`)

// ExperimentalSeedRecord is a public, signed role declaration. A decoded value
// alone conveys no admission: callers must obtain VerifiedExperimentalSeed.
type ExperimentalSeedRecord struct {
	SchemaVersion    int                         `json:"schema_version"`
	Product          string                      `json:"product"`
	Purpose          string                      `json:"purpose"`
	Protocol         string                      `json:"protocol"`
	Profile          string                      `json:"profile"`
	RegistrySHA256   string                      `json:"registry_sha256"`
	InstallerVersion string                      `json:"installer_version"`
	InstallerSHA256  string                      `json:"installer_sha256"`
	TargetOS         string                      `json:"target_os"`
	TargetArch       string                      `json:"target_arch"`
	Source           releasecontract.SourceTuple `json:"source"`
	Releases         []ExperimentalSeedRelease   `json:"releases"`
	Issuer           string                      `json:"issuer"`
	KeyID            string                      `json:"key_id"`
	NotBefore        string                      `json:"not_before"`
	ExpiresAt        string                      `json:"expires_at"`
}

type ExperimentalSeedRelease struct {
	Release            string `json:"release"`
	ManifestSHA256     string `json:"manifest_sha256"`
	ClientInputsSHA256 string `json:"client_inputs_sha256"`
	ServerSHA256       string `json:"server_sha256"`
	BundleSHA256       string `json:"bundle_sha256"`
	RuntimeSHA256      string `json:"runtime_sha256"`
}

// ExperimentalSeedIdentity is inspectable provenance, never native trust or a
// replacement for revalidating the opaque admission at an effect boundary.
type ExperimentalSeedIdentity struct {
	Profile          string                      `json:"profile"`
	Evidence         string                      `json:"evidence"`
	AdmissionSHA256  string                      `json:"admission_sha256"`
	RegistrySHA256   string                      `json:"registry_sha256"`
	InstallerSHA256  string                      `json:"installer_sha256"`
	InstallerVersion string                      `json:"installer_version"`
	TargetOS         string                      `json:"target_os"`
	TargetArch       string                      `json:"target_arch"`
	Source           releasecontract.SourceTuple `json:"source"`
	ExpiresAt        string                      `json:"expires_at"`
}

// VerifiedExperimentalSeed can be constructed only by the signed-role verifier.
// Its inputs are owned copies; no exported record or byte slice can mutate it.
type VerifiedExperimentalSeed struct {
	record                        ExperimentalSeedRecord
	body, signature, registryBody []byte
	pin                           string
	clock                         func() time.Time
}

func VerifyExperimentalSeedAdmission(body, signature, registryBody []byte, pin string, clock func() time.Time) (*VerifiedExperimentalSeed, error) {
	if clock == nil || len(body) == 0 || len(body) > maximumExperimentalSeedBytes || len(signature) != ed25519.SignatureSize || len(registryBody) == 0 || len(registryBody) > 1<<20 || !isLowerSHA256(pin) {
		return nil, errors.New("bounded experimental seed role, signature, registry pin and clock required")
	}
	var record ExperimentalSeedRecord
	if err := releasecontract.DecodeStrictJSON(body, &record); err != nil {
		return nil, fmt.Errorf("decode experimental seed role: %w", err)
	}
	now := clock()
	if err := validateExperimentalSeedRecord(record, now); err != nil {
		return nil, err
	}
	registryDigest := sha256.Sum256(registryBody)
	if hex.EncodeToString(registryDigest[:]) != pin || record.RegistrySHA256 != pin {
		return nil, errors.New("experimental seed registry differs from the compiled pin")
	}
	registry, err := releaseverify.ParseAuthorityRegistry(bytes.NewReader(registryBody), func() time.Time { return now })
	if err != nil {
		return nil, err
	}
	key, ok := registry.Lookup("maestro", record.Issuer, record.KeyID)
	if !ok || !ed25519.Verify(key, body, signature) {
		return nil, errors.New("experimental installed-seed role signature or authority unavailable")
	}
	// The canonical registry parser owns its closed structure and key semantics.
	// Read only the already validated lifetime to constrain this distinct role.
	var scope struct {
		Authorities []struct {
			Issuer     string `json:"issuer"`
			KeyID      string `json:"key_id"`
			ValidFrom  string `json:"valid_from"`
			ValidUntil string `json:"valid_until"`
		} `json:"authorities"`
	}
	if err := json.Unmarshal(registryBody, &scope); err != nil || len(scope.Authorities) != 1 {
		return nil, errors.New("experimental seed requires exactly one engineering authority")
	}
	a := scope.Authorities[0]
	start, startErr := time.Parse(time.RFC3339, a.ValidFrom)
	end, endErr := time.Parse(time.RFC3339, a.ValidUntil)
	roleStart, _ := time.Parse(time.RFC3339, record.NotBefore)
	roleEnd, _ := time.Parse(time.RFC3339, record.ExpiresAt)
	if startErr != nil || endErr != nil || !end.After(start) || end.Sub(start) > 14*24*time.Hour || a.Issuer != record.Issuer || a.KeyID != record.KeyID || roleStart.Before(start) || roleEnd.After(end) {
		return nil, errors.New("experimental seed validity exceeds its at-most-fourteen-day engineering authority")
	}
	return &VerifiedExperimentalSeed{record: record, body: bytes.Clone(body), signature: bytes.Clone(signature), registryBody: bytes.Clone(registryBody), pin: pin, clock: clock}, nil
}

func validateExperimentalSeedRecord(r ExperimentalSeedRecord, now time.Time) error {
	if r.SchemaVersion != 1 || r.Product != "maestro" || r.Purpose != ExperimentalSeedPurpose || r.Protocol != ExperimentalSeedProtocol || r.Profile != ExperimentalSeedProfile || !experimentalSeedVersionPattern.MatchString(r.InstallerVersion) || !experimentalSeedIssuerPattern.MatchString(r.Issuer) || !experimentalSeedKeyPattern.MatchString(r.KeyID) {
		return errors.New("unsupported experimental installed-seed purpose or protocol")
	}
	if !isLowerSHA256(r.RegistrySHA256) || !isLowerSHA256(r.InstallerSHA256) || (r.TargetOS != "darwin" && r.TargetOS != "windows") || (r.TargetArch != "amd64" && r.TargetArch != "arm64") || r.Source.Validate() != nil || r.Source.Channel != "canary" || r.Source.OS != r.TargetOS || r.Source.Arch != r.TargetArch {
		return errors.New("experimental seed requires exact installer, source and target identities")
	}
	start, err := time.Parse(time.RFC3339, r.NotBefore)
	if err != nil {
		return errors.New("experimental seed start is invalid")
	}
	end, err := time.Parse(time.RFC3339, r.ExpiresAt)
	if err != nil || now.IsZero() || !end.After(start) || end.Sub(start) > 14*24*time.Hour || now.Before(start) || !now.Before(end) {
		return errors.New("experimental installed-seed role is not currently valid")
	}
	if len(r.Releases) != 2 {
		return errors.New("experimental seed requires exactly two finite release bindings")
	}
	seenVersions, seenManifests := map[string]bool{}, map[string]bool{}
	for _, release := range r.Releases {
		if !experimentalSeedVersionPattern.MatchString(release.Release) || seenVersions[release.Release] || seenManifests[release.ManifestSHA256] {
			return errors.New("experimental releases must have distinct exact versions and manifests")
		}
		for _, hash := range []string{release.ManifestSHA256, release.ClientInputsSHA256, release.ServerSHA256, release.BundleSHA256, release.RuntimeSHA256} {
			if !isLowerSHA256(hash) {
				return errors.New("experimental release binding lacks an exact component hash")
			}
		}
		seenVersions[release.Release], seenManifests[release.ManifestSHA256] = true, true
	}
	return nil
}

func (v *VerifiedExperimentalSeed) Identity() ExperimentalSeedIdentity {
	if v == nil || v.clock == nil {
		return ExperimentalSeedIdentity{}
	}
	sum := sha256.Sum256(v.body)
	r := v.record
	return ExperimentalSeedIdentity{Profile: r.Profile, Evidence: "engineering_only", AdmissionSHA256: hex.EncodeToString(sum[:]), RegistrySHA256: r.RegistrySHA256, InstallerSHA256: r.InstallerSHA256, InstallerVersion: r.InstallerVersion, TargetOS: r.TargetOS, TargetArch: r.TargetArch, Source: r.Source, ExpiresAt: r.ExpiresAt}
}

func (v *VerifiedExperimentalSeed) Revalidate() error {
	if v == nil {
		return errors.New("verified experimental installed-seed role required")
	}
	_, err := VerifyExperimentalSeedAdmission(v.body, v.signature, v.registryBody, v.pin, v.clock)
	return err
}

func (v *VerifiedExperimentalSeed) RecordBytes() ([]byte, []byte) {
	if v == nil {
		return nil, nil
	}
	return bytes.Clone(v.body), bytes.Clone(v.signature)
}

func (v *VerifiedExperimentalSeed) VerifyInstaller(path, version, targetOS, targetArch string) error {
	return v.VerifyInstallerContext(context.Background(), path, version, targetOS, targetArch)
}

func (v *VerifiedExperimentalSeed) VerifyInstallerContext(ctx context.Context, path, version, targetOS, targetArch string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := v.Revalidate(); err != nil {
		return err
	}
	if version != v.record.InstallerVersion || targetOS != v.record.TargetOS || targetArch != v.record.TargetArch {
		return errors.New("experimental installer version or target differs from its role")
	}
	file, err := scheduler.OpenRegularFile(path)
	if err != nil {
		return err
	}
	defer file.Close()
	if err := scheduler.ValidateSingleLink(file); err != nil {
		return err
	}
	before, err := file.Stat()
	if err != nil {
		return err
	}
	if !before.Mode().IsRegular() || before.Size() <= 0 || before.Size() > maxBootstrapperBytes {
		return errors.New("experimental installer must be a bounded regular file")
	}
	hash := sha256.New()
	n, err := io.Copy(hash, installerReadContext{ctx, io.LimitReader(file, maxBootstrapperBytes+1)})
	if err != nil {
		return err
	}
	after, err := file.Stat()
	if err != nil {
		return err
	}
	current, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if n != before.Size() || after.Size() != before.Size() || !after.ModTime().Equal(before.ModTime()) || !current.Mode().IsRegular() || !os.SameFile(after, current) || hex.EncodeToString(hash.Sum(nil)) != v.record.InstallerSHA256 {
		return errors.New("experimental installer differs from the exact signed role")
	}
	return ctx.Err()
}

func (v *VerifiedExperimentalSeed) VerifySource(source releasecontract.SourceTuple) error {
	if err := v.Revalidate(); err != nil {
		return err
	}
	if source != v.record.Source {
		return errors.New("source differs from the experimental installed-seed role")
	}
	return nil
}

func (v *VerifiedExperimentalSeed) VerifyReleaseIdentity(manifestSHA256, clientInputsSHA256 string) error {
	if err := v.Revalidate(); err != nil {
		return err
	}
	for _, r := range v.record.Releases {
		if r.ManifestSHA256 == manifestSHA256 && r.ClientInputsSHA256 == clientInputsSHA256 {
			return nil
		}
	}
	return errors.New("release identity is outside the two experimental bindings")
}

func (v *VerifiedExperimentalSeed) VerifyRelease(release releaseverify.VerifiedRelease) error {
	if err := v.VerifyReleaseIdentity(release.ManifestSHA256, release.Manifest.Delivery.ClientInputsSHA256); err != nil {
		return err
	}
	m := release.Manifest
	if !m.Modern() || m.Product != "maestro" || m.Channel != "canary" || m.Server.ExecutionContract != releasecontract.SyncedSourceContract || m.Delivery.InstallerVersion != v.record.InstallerVersion || m.Issuer.ID != v.record.Issuer || m.Issuer.KeyID != v.record.KeyID {
		return errors.New("release does not use the exact experimental installed-seed contract")
	}
	registry, err := releaseverify.ParseAuthorityRegistry(bytes.NewReader(v.registryBody), v.clock)
	if err != nil {
		return err
	}
	key, ok := registry.Lookup("maestro", v.record.Issuer, v.record.KeyID)
	if !ok || !bytes.Equal(key, release.PublicKey) {
		return errors.New("release was verified under another authority")
	}
	var binding ExperimentalSeedRelease
	for _, r := range v.record.Releases {
		if r.ManifestSHA256 == release.ManifestSHA256 {
			binding = r
		}
	}
	if binding.Release != m.Release {
		return errors.New("experimental release version differs from its manifest identity")
	}
	server, bundle, runtimePack := "", "", ""
	serverCount, bundleCount, runtimeCount := 0, 0, 0
	for _, a := range m.Artifacts {
		switch {
		case a.Kind == "bundle" && a.OS == "any" && a.Arch == "any":
			bundle, bundleCount = a.SHA256, bundleCount+1
		case a.Kind == "mcp_server" && a.OS == v.record.TargetOS && a.Arch == v.record.TargetArch:
			server, serverCount = a.SHA256, serverCount+1
		case a.Kind == "runtime_pack" && a.Adapter == "docling" && a.OS == v.record.TargetOS && a.Arch == v.record.TargetArch:
			runtimePack, runtimeCount = a.SHA256, runtimeCount+1
		}
	}
	if serverCount != 1 || bundleCount != 1 || runtimeCount != 1 || server != binding.ServerSHA256 || bundle != binding.BundleSHA256 || runtimePack != binding.RuntimeSHA256 {
		return errors.New("experimental release component identities differ from the exact role")
	}
	return nil
}
