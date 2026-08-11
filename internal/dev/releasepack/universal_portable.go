package releasepack

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releaseverify"
)

// UniversalPortableOptions describes a controlled test archive. The archive is
// one Claude-directed handoff, but it carries only native payloads; no runtime
// is ever shared across operating systems.
type UniversalPortableOptions struct {
	Version, ReleaseDirectory, AuthorityRegistry, AuthorityRegistrySHA256 string
	WindowsBootstrapper, WindowsBootstrapperSHA256                        string
	DarwinARM64Bootstrapper, DarwinARM64BootstrapperSHA256                string
	DarwinAMD64Bootstrapper, DarwinAMD64BootstrapperSHA256                string
	Output                                                                string
	Clock                                                                 func() time.Time
	BootstrapperSeedStatus                                                func(string) (BootstrapperSeedStatus, error)
	WindowsSignatureStatus                                                func(string) (string, error)
}

type UniversalPortableResult struct{ Output, SHA256, Status, Provenance, Checksum string }

const universalPortableActivationContract = "maestro-portable-activate-v1"

func BuildUniversalPortable(options UniversalPortableOptions) (UniversalPortableResult, error) {
	if !portableVersionPattern.MatchString(options.Version) {
		return UniversalPortableResult{}, errors.New("portable version must be MAJOR.MINOR.PATCH")
	}
	for label, value := range map[string]string{
		"authority registry": options.AuthorityRegistrySHA256, "Windows bootstrapper": options.WindowsBootstrapperSHA256,
		"macOS arm64 bootstrapper": options.DarwinARM64BootstrapperSHA256, "macOS amd64 bootstrapper": options.DarwinAMD64BootstrapperSHA256,
	} {
		if !portableDigestPattern.MatchString(value) {
			return UniversalPortableResult{}, fmt.Errorf("%s pin must be a lowercase SHA-256", label)
		}
	}
	for label, path := range map[string]string{"signed release": options.ReleaseDirectory, "authority registry": options.AuthorityRegistry, "Windows bootstrapper": options.WindowsBootstrapper, "macOS arm64 bootstrapper": options.DarwinARM64Bootstrapper, "macOS amd64 bootstrapper": options.DarwinAMD64Bootstrapper, "output": options.Output} {
		if !filepath.IsAbs(path) {
			return UniversalPortableResult{}, fmt.Errorf("%s path must be absolute", label)
		}
	}
	expected := "Maestro-Portable-" + options.Version + "-universal-local-beta-unsigned.zip"
	if filepath.Base(options.Output) != expected {
		return UniversalPortableResult{}, fmt.Errorf("portable output must be named %s", expected)
	}
	checksumPath, provenancePath := options.Output+".sha256", options.Output+".provenance.json"
	for _, path := range []string{options.Output, checksumPath, provenancePath} {
		if _, err := os.Lstat(path); err == nil {
			return UniversalPortableResult{}, fmt.Errorf("portable output already exists: %s", path)
		} else if !errors.Is(err, os.ErrNotExist) {
			return UniversalPortableResult{}, err
		}
	}
	succeeded := false
	defer func() {
		if !succeeded {
			_ = os.Remove(options.Output)
			_ = os.Remove(checksumPath)
			_ = os.Remove(provenancePath)
		}
	}()
	registryBody, registryDigest, err := readSeedInput(options.AuthorityRegistry, 1<<20)
	if err != nil {
		return UniversalPortableResult{}, err
	}
	if registryDigest != options.AuthorityRegistrySHA256 {
		return UniversalPortableResult{}, errors.New("portable authority registry does not match its approved pin")
	}
	clock := options.Clock
	if clock == nil {
		clock = time.Now
	}
	registry, err := releaseverify.LoadPinnedAuthorityRegistry(options.AuthorityRegistry, options.AuthorityRegistrySHA256, clock)
	if err != nil {
		return UniversalPortableResult{}, err
	}
	verified, err := releaseverify.VerifyDirectory(options.ReleaseDirectory, registry)
	if err != nil {
		return UniversalPortableResult{}, fmt.Errorf("verify portable signed release: %w", err)
	}
	if verified.Manifest.Release != options.Version || verified.Manifest.Channel != "canary" {
		return UniversalPortableResult{}, errors.New("portable package requires the exact canary release version")
	}
	if err := requireUniversalPortableArtifacts(options.Version, verified.Manifest.Artifacts); err != nil {
		return UniversalPortableResult{}, err
	}
	inputs := []struct{ target, path, digest string }{
		{"windows-amd64", options.WindowsBootstrapper, options.WindowsBootstrapperSHA256}, {"darwin-arm64", options.DarwinARM64Bootstrapper, options.DarwinARM64BootstrapperSHA256}, {"darwin-amd64", options.DarwinAMD64Bootstrapper, options.DarwinAMD64BootstrapperSHA256},
	}
	bodies := make(map[string][]byte)
	for _, input := range inputs {
		name := "bcgos-bootstrap_" + options.Version + "_" + strings.ReplaceAll(input.target, "-", "_")
		if input.target == "windows-amd64" {
			name += ".exe"
		}
		if filepath.Base(input.path) != name {
			return UniversalPortableResult{}, fmt.Errorf("portable bootstrapper must be named %s", name)
		}
		body, digest, err := readSeedInput(input.path, 1<<30)
		if err != nil {
			return UniversalPortableResult{}, err
		}
		if digest != input.digest {
			return UniversalPortableResult{}, fmt.Errorf("portable %s bootstrapper does not match its approved pin", input.target)
		}
		seedStatus := options.BootstrapperSeedStatus
		if seedStatus == nil {
			seedStatus = func(path string) (BootstrapperSeedStatus, error) {
				return readBootstrapperSeedStatus(path, options.Version, options.AuthorityRegistrySHA256)
			}
		}
		seed, err := seedStatus(input.path)
		if err != nil || seed.Version != options.Version || seed.AuthorityRegistrySHA256 != options.AuthorityRegistrySHA256 {
			return UniversalPortableResult{}, fmt.Errorf("portable %s bootstrapper seed does not match the release and authority registry", input.target)
		}
		if !bytes.Contains(body, []byte(universalPortableActivationContract)) {
			return UniversalPortableResult{}, fmt.Errorf("portable %s bootstrapper does not support direct portable activation", input.target)
		}
		bodies[input.target] = body
	}
	statusProbe := options.WindowsSignatureStatus
	if statusProbe == nil {
		statusProbe = windowsAuthenticodeStatus
	}
	status, err := statusProbe(options.WindowsBootstrapper)
	if err != nil {
		return UniversalPortableResult{}, fmt.Errorf("inspect portable Windows bootstrapper Authenticode: %w", err)
	}
	if status != "NotSigned" {
		return UniversalPortableResult{}, fmt.Errorf("portable Windows local-beta bootstrapper Authenticode status must be exactly NotSigned; got %s", status)
	}
	parent := filepath.Dir(options.Output)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return UniversalPortableResult{}, err
	}
	staging, err := os.MkdirTemp(parent, ".maestro-universal-")
	if err != nil {
		return UniversalPortableResult{}, err
	}
	defer os.RemoveAll(staging)
	rootName := "Maestro-Portable-" + options.Version + "-universal"
	root := filepath.Join(staging, rootName)
	for _, dir := range []string{filepath.Join(root, "managed", "trust"), filepath.Join(root, "managed", "windows"), filepath.Join(root, "managed", "macos", "arm64"), filepath.Join(root, "managed", "macos", "amd64"), filepath.Join(root, "maestro-os")} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return UniversalPortableResult{}, err
		}
	}
	files := map[string]struct {
		body []byte
		mode os.FileMode
	}{
		"managed/trust/release-authority-registry.json": {registryBody, 0o600}, "managed/windows/bcgos-bootstrap.exe": {bodies["windows-amd64"], 0o700}, "managed/macos/arm64/bcgos-bootstrap": {bodies["darwin-arm64"], 0o700}, "managed/macos/amd64/bcgos-bootstrap": {bodies["darwin-amd64"], 0o700},
		"maestro-os/CLAUDE.md": {universalPortableClaudeOnboarding(), 0o600}, "maestro-os/README.md": {[]byte("# Maestro OS\n\nAbra esta pasta no Claude Code e envie uma mensagem para comecar.\n"), 0o600}, "README-PORTABLE.md": {universalPortableReadme(options.Version), 0o600},
	}
	provenance, err := json.MarshalIndent(map[string]any{"schema_version": 1, "product": "maestro", "version": options.Version, "distribution_profile": "universal-portable-local-beta", "targets": []string{"windows-amd64", "darwin-amd64", "darwin-arm64"}, "release_manifest_sha256": verified.ManifestSHA256, "authority_registry_sha256": registryDigest, "status": "unsigned-controlled-canary"}, "", "  ")
	if err != nil {
		return UniversalPortableResult{}, err
	}
	files["portable-provenance.json"] = struct {
		body []byte
		mode os.FileMode
	}{append(provenance, '\n'), 0o600}
	for relative, file := range files {
		if err := copyRegularBytes(file.body, filepath.Join(root, filepath.FromSlash(relative)), file.mode); err != nil {
			return UniversalPortableResult{}, err
		}
	}
	if err := copyClosedRelease(options.ReleaseDirectory, filepath.Join(root, "release")); err != nil {
		return UniversalPortableResult{}, err
	}
	if err := writeDeterministicZip(staging, rootName, options.Output); err != nil {
		return UniversalPortableResult{}, err
	}
	_, digest, err := digestBoundedRegular(options.Output, 2<<30)
	if err != nil {
		return UniversalPortableResult{}, err
	}
	if err := writeExclusive(checksumPath, []byte(digest+"  "+filepath.Base(options.Output)+"\n"), 0o600); err != nil {
		return UniversalPortableResult{}, err
	}
	if err := writeExclusive(provenancePath, append(provenance, '\n'), 0o600); err != nil {
		return UniversalPortableResult{}, err
	}
	succeeded = true
	return UniversalPortableResult{Output: options.Output, SHA256: digest, Status: "unsigned-controlled-canary", Provenance: provenancePath, Checksum: checksumPath}, nil
}

func requireUniversalPortableArtifacts(version string, artifacts []releasecontract.Artifact) error {
	windows, darwinAMD64, darwinARM64, bundle := false, false, false, false
	for _, artifact := range artifacts {
		if artifact.Kind == "cli" {
			switch artifact.OS + "/" + artifact.Arch {
			case "windows/amd64":
				windows = artifact.Name == "bcgos_"+version+"_windows_amd64.exe"
			case "darwin/amd64":
				darwinAMD64 = artifact.Name == "bcgos_"+version+"_darwin_amd64"
			case "darwin/arm64":
				darwinARM64 = artifact.Name == "bcgos_"+version+"_darwin_arm64"
			}
		}
		if artifact.Kind == "bundle" && artifact.OS == "any" && artifact.Arch == "any" {
			bundle = artifact.Name == "maestro-base_"+version+".tar.gz"
		}
	}
	if !windows || !darwinAMD64 || !darwinARM64 || !bundle {
		return errors.New("universal portable release requires Windows amd64, macOS amd64, macOS arm64 CLIs and one platform-neutral base bundle")
	}
	return nil
}

func universalPortableClaudeOnboarding() []byte {
	return []byte("# Maestro Portable - primeiro uso\n\nConduza a pessoa sem pedir terminal ou scripts. Detecte silenciosamente Windows ou macOS. Se nao for um deles, pare sem alterar nada.\n\n1. Se a projecao ainda nao existir, explique que pode preparar o Maestro. Peca uma unica confirmacao curta.\n2. Depois da confirmacao, execute internamente somente o bootstrapper nativo correspondente com o argumento `portable-activate`: no Windows, `..\\managed\\windows\\bcgos-bootstrap.exe portable-activate`; em macOS Apple Silicon, `../managed/macos/arm64/bcgos-bootstrap portable-activate`; em macOS Intel, `../managed/macos/amd64/bcgos-bootstrap portable-activate`.\n3. Nunca use intermediarios, o bootstrapper da outra plataforma, substitutos ou conversoes de binarios.\n4. Quando terminar, releia este arquivo e continue com `maestro-onboarding`. Se a execucao nativa nao estiver disponivel, explique somente que a preparacao nao pode ser concluida neste runtime e ofereca encaminhar para o responsavel pelo piloto.\n")
}
func universalPortableReadme(version string) []byte {
	return []byte("# Maestro Portable " + version + "\n\nExtraia a pasta em local fixo, abra `maestro-os` no Claude Code e envie uma mensagem para comecar. O Claude detecta Windows ou macOS e conduz a preparacao; nao execute os scripts internos. Este e um canario controlado sem assinatura de producao.\n")
}
