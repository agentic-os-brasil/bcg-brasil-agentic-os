package packaging

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	product "github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/datahome"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ingest/docling"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releaseverify"
)

const EngineeringRuntimeProfile = "engineering-runtime-pilot-v1"

// EngineeringRuntimeBinding identifies this explicitly selected experiment;
// its issuer is deliberately separate from organizational publisher authority.
type EngineeringRuntimeBinding struct {
	Profile        string `json:"profile"`
	ClientVersion  string `json:"client_version"`
	RegistrySHA256 string `json:"registry_sha256"`
	ManifestSHA256 string `json:"manifest_sha256"`
	PackSHA256     string `json:"pack_sha256"`
	ServerSHA256   string `json:"server_sha256"`
	Target         Target `json:"target"`
	Issuer         string `json:"issuer"`
	KeyID          string `json:"key_id"`
	ExpiresAt      string `json:"expires_at"`
}

func (b EngineeringRuntimeBinding) validate() error {
	if b.Profile != EngineeringRuntimeProfile || !regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`).MatchString(b.ClientVersion) || !regexp.MustCompile(`^maestro-engineering-pilot-[0-9]{8}$`).MatchString(b.Issuer) || !regexp.MustCompile(`^pilot-[0-9]{8}$`).MatchString(b.KeyID) {
		return errors.New("engineering runtime profile unavailable")
	}
	if b.Target != (Target{OS: "darwin", Arch: "arm64"}) && b.Target != (Target{OS: "windows", Arch: "amd64"}) {
		return errors.New("engineering runtime target unavailable")
	}
	for _, pin := range []string{b.RegistrySHA256, b.ManifestSHA256, b.PackSHA256, b.ServerSHA256} {
		if !regexp.MustCompile(`^[a-f0-9]{64}$`).MatchString(pin) {
			return errors.New("engineering runtime identity unavailable")
		}
	}
	if _, err := time.Parse(time.RFC3339, b.ExpiresAt); err != nil {
		return errors.New("engineering runtime expiry unavailable")
	}
	return nil
}

func engineeringRuntimeAuthority(body []byte, pin, issuer, keyID string, now time.Time) (time.Time, error) {
	if digest(body) != pin || !regexp.MustCompile(`^maestro-engineering-pilot-[0-9]{8}$`).MatchString(issuer) || !regexp.MustCompile(`^pilot-[0-9]{8}$`).MatchString(keyID) {
		return time.Time{}, errors.New("explicit engineering authority required")
	}
	registry, err := releaseverify.ParseAuthorityRegistry(bytes.NewReader(body), func() time.Time { return now })
	if err != nil {
		return time.Time{}, err
	}
	if _, ok := registry.Lookup("maestro", issuer, keyID); !ok {
		return time.Time{}, errors.New("engineering authority expired or unavailable; request a newly reviewed kit")
	}
	// The canonical decoder above owns registry structure and key validation.
	var scope struct {
		Authorities []struct {
			Issuer     string `json:"issuer"`
			KeyID      string `json:"key_id"`
			ValidFrom  string `json:"valid_from"`
			ValidUntil string `json:"valid_until"`
		} `json:"authorities"`
	}
	if err := json.Unmarshal(body, &scope); err != nil || len(scope.Authorities) != 1 {
		return time.Time{}, errors.New("engineering pilot requires exactly one authority")
	}
	a := scope.Authorities[0]
	start, err := time.Parse(time.RFC3339, a.ValidFrom)
	if err != nil {
		return time.Time{}, err
	}
	end, err := time.Parse(time.RFC3339, a.ValidUntil)
	if err != nil || a.Issuer != issuer || a.KeyID != keyID || end.Sub(start) > 14*24*time.Hour || !end.After(now) {
		return time.Time{}, errors.New("engineering authority must have its own at-most-fourteen-day lifetime")
	}
	return end, nil
}

func preparedEngineeringRuntime(o product.Options, server string) (EngineeringRuntimeBinding, error) {
	runtimeOptions, err := engineeringLayout(o, server)
	if err != nil {
		return EngineeringRuntimeBinding{}, err
	}
	record, _, err := readEngineeringPlacement(o.ProductRoot)
	if err != nil {
		return EngineeringRuntimeBinding{}, err
	}
	body, target, err := readNativeBinary(server)
	if err != nil || target != record.Binding.Target || digest(body) != record.Binding.ServerSHA256 {
		return EngineeringRuntimeBinding{}, errors.New("prepared engineering server changed")
	}
	if err := record.Binding.ValidateSelection(o); err != nil {
		return EngineeringRuntimeBinding{}, err
	}
	status := docling.New(runtimeOptions).Status(context.Background())
	if status.State != "ready" || status.PackSHA256 != record.Binding.PackSHA256 {
		return EngineeringRuntimeBinding{}, errors.New("prepared engineering converter unavailable")
	}
	return record.Binding, nil
}

// ValidateEngineeringPacketSelection runs after complete inventory validation
// and before using the new carrier's authority seed.
func ValidateEngineeringPacketSelection(root string, status PacketStatus, executingPath, executingSHA256, version, registrySHA256 string) error {
	if status.DesktopPilot == nil || status.DesktopPilot.Runtime == nil || status.Stable != nil || status.NativeMode != "mcp_only" {
		return errors.New("explicit engineering Desktop carrier required")
	}
	binding := status.DesktopPilot.Runtime
	if _, err := os.Lstat(filepath.Join(status.DesktopPilot.UserHome, ".maestro-install")); !errors.Is(err, os.ErrNotExist) {
		return errors.New("engineering Desktop cannot use an enrolled installation")
	}
	if err := binding.validate(); err != nil {
		return err
	}
	expected := filepath.Join(root, "server", binaryName(status.Target.Platform()))
	pathMatches := expected == executingPath
	if runtime.GOOS == "windows" {
		pathMatches = strings.EqualFold(expected, executingPath)
	}
	if !pathMatches || executingSHA256 != status.ServerSHA256 || binding.ServerSHA256 != executingSHA256 || status.Version != version || binding.ClientVersion != version || binding.RegistrySHA256 != registrySHA256 || status.RegistrySHA256 != registrySHA256 || binding.Target != status.Target {
		return errors.New("engineering Desktop executing image or authority differs")
	}
	return nil
}

// ValidateSelection reads only the selected public registry and signed runtime
// metadata. Full converter verification remains with the canonical content owner.
func (b EngineeringRuntimeBinding) ValidateSelection(o product.Options) error {
	if err := b.validate(); err != nil {
		return err
	}
	if b.ClientVersion != o.CLIVersion || b.RegistrySHA256 != o.AuthorityRegistrySHA256 || o.AuthorityRegistryPath != filepath.Join(o.ProductRoot, "managed", "trust", "release-authority-registry.json") {
		return errors.New("engineering runtime constructor seed differs")
	}
	registryBody, err := readEngineeringFile(o.AuthorityRegistryPath, 1<<20)
	if err != nil {
		return err
	}
	expires, err := engineeringRuntimeAuthority(registryBody, b.RegistrySHA256, b.Issuer, b.KeyID, time.Now())
	if err != nil || expires.UTC().Format(time.RFC3339) != b.ExpiresAt {
		return errors.New("engineering runtime authority expired or changed; request a newly reviewed kit")
	}
	home, err := datahome.Resolve(datahome.Options{ProductRoot: o.ProductRoot, UserHome: o.UserHome, LookupEnv: o.LookupEnv})
	if err != nil || !home.Writable {
		return errors.New("engineering runtime home unavailable")
	}
	base := filepath.Join(home.Home, "state", "ingestion", "docling")
	manifestBody, err := readEngineeringFile(filepath.Join(base, releaseverify.ManifestName), 1<<20)
	if err != nil || digest(manifestBody) != b.ManifestSHA256 {
		return errors.New("engineering runtime signed manifest changed")
	}
	signature, err := readEngineeringFile(filepath.Join(base, releaseverify.ManifestSignatureName), 64)
	if err != nil {
		return err
	}
	registry, err := releaseverify.LoadPinnedAuthorityRegistry(o.AuthorityRegistryPath, b.RegistrySHA256, time.Now)
	if err != nil {
		return err
	}
	manifest, _, err := releaseverify.VerifyManifest(manifestBody, signature, registry)
	if err != nil || !manifest.Modern() || manifest.Channel != "canary" || manifest.Issuer.ID != b.Issuer || manifest.Issuer.KeyID != b.KeyID || manifest.Server.Version != b.ClientVersion {
		return errors.New("engineering runtime signed scope differs")
	}
	servers, packs := 0, 0
	for _, a := range manifest.Artifacts {
		if a.OS == b.Target.OS && a.Arch == b.Target.Arch {
			if a.Kind == "mcp_server" && a.SHA256 == b.ServerSHA256 {
				servers++
			}
			if a.Kind == "runtime_pack" && a.Adapter == "docling" && a.SHA256 == b.PackSHA256 {
				packs++
			}
		}
	}
	if servers != 1 || packs != 1 {
		return errors.New("engineering runtime signed target differs")
	}
	return nil
}
