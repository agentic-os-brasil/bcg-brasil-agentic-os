package releasepack

import (
	"bytes"
	"debug/macho"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releaseverify"
)

const machOLoadCommandCodeSignature = 0x1d

// MacOSPortableOptions describes the controlled macOS arm64 portable handoff.
// It ships the closed release unchanged and exposes only its matching native
// bootstrapper seed.
type MacOSPortableOptions struct {
	Version                 string
	ReleaseDirectory        string
	AuthorityRegistry       string
	AuthorityRegistrySHA256 string
	Bootstrapper            string
	BootstrapperSHA256      string
	Output                  string
	Clock                   func() time.Time
	BootstrapperSeedStatus  func(string) (BootstrapperSeedStatus, error)
	StructuralSignature     func(string) (string, error)
	NativeSignature         func(string) (string, error)
}

type MacOSPortableResult struct {
	Output     string `json:"output"`
	SHA256     string `json:"sha256"`
	Status     string `json:"status"`
	Provenance string `json:"provenance"`
	Checksum   string `json:"checksum"`
}

type macOSPortableProvenance struct {
	SchemaVersion               int    `json:"schema_version"`
	Product                     string `json:"product"`
	Version                     string `json:"version"`
	TargetOS                    string `json:"target_os"`
	TargetArch                  string `json:"target_arch"`
	DistributionProfile         string `json:"distribution_profile"`
	ReleaseManifestSHA256       string `json:"release_manifest_sha256"`
	ReleaseIssuer               string `json:"release_issuer"`
	ReleaseKeyID                string `json:"release_key_id"`
	AuthorityRegistrySHA256     string `json:"authority_registry_sha256"`
	BootstrapperSHA256          string `json:"bootstrapper_sha256"`
	BootstrapperSignatureStatus string `json:"bootstrapper_codesign_status"`
	Status                      string `json:"status"`
}

func BuildMacOSPortable(options MacOSPortableOptions) (MacOSPortableResult, error) {
	if !portableVersionPattern.MatchString(options.Version) {
		return MacOSPortableResult{}, errors.New("portable version must be MAJOR.MINOR.PATCH")
	}
	if !portableDigestPattern.MatchString(options.AuthorityRegistrySHA256) || !portableDigestPattern.MatchString(options.BootstrapperSHA256) {
		return MacOSPortableResult{}, errors.New("portable authority-registry and bootstrapper pins must be lowercase SHA-256 values")
	}
	for label, path := range map[string]string{
		"signed release": options.ReleaseDirectory, "authority registry": options.AuthorityRegistry,
		"bootstrapper": options.Bootstrapper, "output": options.Output,
	} {
		if !filepath.IsAbs(path) {
			return MacOSPortableResult{}, fmt.Errorf("%s path must be absolute", label)
		}
	}
	expectedOutput := "Maestro-Portable-" + options.Version + "-macos-arm64-local-beta-unsigned.zip"
	if filepath.Base(options.Output) != expectedOutput {
		return MacOSPortableResult{}, fmt.Errorf("portable output must be named %s", expectedOutput)
	}
	checksumPath, provenancePath := options.Output+".sha256", options.Output+".provenance.json"
	for _, path := range []string{options.Output, checksumPath, provenancePath} {
		if _, err := os.Lstat(path); err == nil {
			return MacOSPortableResult{}, fmt.Errorf("portable output already exists: %s", path)
		} else if !errors.Is(err, os.ErrNotExist) {
			return MacOSPortableResult{}, err
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
		return MacOSPortableResult{}, fmt.Errorf("read portable authority registry: %w", err)
	}
	if registryDigest != options.AuthorityRegistrySHA256 {
		return MacOSPortableResult{}, errors.New("portable authority registry does not match its approved pin")
	}
	clock := options.Clock
	if clock == nil {
		clock = time.Now
	}
	registry, err := releaseverify.LoadPinnedAuthorityRegistry(options.AuthorityRegistry, options.AuthorityRegistrySHA256, clock)
	if err != nil {
		return MacOSPortableResult{}, err
	}
	verified, err := releaseverify.VerifyDirectory(options.ReleaseDirectory, registry)
	if err != nil {
		return MacOSPortableResult{}, fmt.Errorf("verify portable signed release: %w", err)
	}
	manifest := verified.Manifest
	if manifest.Release != options.Version || manifest.Channel != "canary" {
		return MacOSPortableResult{}, errors.New("portable package requires the exact canary release version")
	}
	if _, ok := registry.Lookup("maestro", manifest.Issuer.ID, manifest.Issuer.KeyID); !ok {
		return MacOSPortableResult{}, errors.New("portable release issuer is not active in the pinned registry")
	}
	if err := requireMacOSPortableArtifacts(manifest.Release, manifest.Artifacts); err != nil {
		return MacOSPortableResult{}, err
	}
	expectedBootstrapper := "bcgos-bootstrap_" + options.Version + "_darwin_arm64"
	if filepath.Base(options.Bootstrapper) != expectedBootstrapper {
		return MacOSPortableResult{}, fmt.Errorf("portable bootstrapper must be named %s", expectedBootstrapper)
	}
	bootstrapperBody, bootstrapperDigest, err := readSeedInput(options.Bootstrapper, 1<<30)
	if err != nil {
		return MacOSPortableResult{}, fmt.Errorf("read portable bootstrapper: %w", err)
	}
	if bootstrapperDigest != options.BootstrapperSHA256 {
		return MacOSPortableResult{}, errors.New("portable bootstrapper does not match its approved pin")
	}
	structural := options.StructuralSignature
	if structural == nil {
		structural = macOSStructuralSignatureStatus
	}
	status, err := structural(options.Bootstrapper)
	if err != nil {
		return MacOSPortableResult{}, fmt.Errorf("inspect portable macOS bootstrapper signature structure: %w", err)
	}
	if status != "NotSigned" {
		return MacOSPortableResult{}, fmt.Errorf("portable macOS local-beta bootstrapper must be exactly NotSigned; got %s", status)
	}
	if runtime.GOOS == "darwin" {
		native := options.NativeSignature
		if native == nil {
			native = macOSNativeSignatureStatus
		}
		status, err = native(options.Bootstrapper)
		if err != nil {
			return MacOSPortableResult{}, fmt.Errorf("inspect portable macOS bootstrapper codesign status: %w", err)
		}
		if status != "NotSigned" {
			return MacOSPortableResult{}, fmt.Errorf("portable macOS local-beta bootstrapper codesign status must be exactly NotSigned; got %s", status)
		}
	}
	seedStatus := options.BootstrapperSeedStatus
	if seedStatus == nil {
		expectedVersion, expectedRegistry := options.Version, options.AuthorityRegistrySHA256
		seedStatus = func(path string) (BootstrapperSeedStatus, error) {
			return readBootstrapperSeedStatus(path, expectedVersion, expectedRegistry)
		}
	}
	seed, err := seedStatus(options.Bootstrapper)
	if err != nil {
		return MacOSPortableResult{}, fmt.Errorf("inspect portable bootstrapper seed: %w", err)
	}
	if seed.Version != options.Version || seed.AuthorityRegistrySHA256 != options.AuthorityRegistrySHA256 {
		return MacOSPortableResult{}, errors.New("portable bootstrapper seed does not match the release and authority registry")
	}
	if !bytes.Contains(bootstrapperBody, []byte(portableInstallContract)) {
		return MacOSPortableResult{}, errors.New("portable bootstrapper does not support portable core installation")
	}

	parent := filepath.Dir(options.Output)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return MacOSPortableResult{}, err
	}
	staging, err := os.MkdirTemp(parent, ".maestro-portable-")
	if err != nil {
		return MacOSPortableResult{}, err
	}
	defer os.RemoveAll(staging)
	rootName := "Maestro-Portable-" + options.Version + "-macos-arm64"
	root := filepath.Join(staging, rootName)
	if err := os.MkdirAll(filepath.Join(root, "managed", "trust"), 0o700); err != nil {
		return MacOSPortableResult{}, err
	}
	if err := os.MkdirAll(filepath.Join(root, "maestro-os"), 0o755); err != nil {
		return MacOSPortableResult{}, err
	}
	if err := copyRegularBytes(registryBody, filepath.Join(root, "managed", "trust", "release-authority-registry.json"), 0o600); err != nil {
		return MacOSPortableResult{}, err
	}
	if err := copyRegularBytes(bootstrapperBody, filepath.Join(root, "managed", "bcgos-bootstrap"), 0o700); err != nil {
		return MacOSPortableResult{}, err
	}
	if err := copyClosedRelease(options.ReleaseDirectory, filepath.Join(root, "release")); err != nil {
		return MacOSPortableResult{}, err
	}
	provenance := macOSPortableProvenance{
		SchemaVersion: 1, Product: "maestro", Version: options.Version,
		TargetOS: "darwin", TargetArch: "arm64", DistributionProfile: "macos-portable-local-beta",
		ReleaseManifestSHA256: verified.ManifestSHA256, ReleaseIssuer: manifest.Issuer.ID,
		ReleaseKeyID: manifest.Issuer.KeyID, AuthorityRegistrySHA256: registryDigest,
		BootstrapperSHA256: bootstrapperDigest, BootstrapperSignatureStatus: status,
		Status: "unsigned-controlled-canary",
	}
	provenanceBody, err := json.MarshalIndent(provenance, "", "  ")
	if err != nil {
		return MacOSPortableResult{}, err
	}
	provenanceBody = append(provenanceBody, '\n')
	for relative, body := range map[string][]byte{
		"README-PORTABLE.md":       macOSPortableReadme(options.Version),
		"portable-provenance.json": provenanceBody,
		"maestro-os/CLAUDE.md":     macOSPortableClaudeOnboarding(),
		"maestro-os/README.md":     []byte("# Maestro OS\n\nAbra esta pasta no Claude Code e envie uma mensagem para comecar. O Claude conduz a preparacao e o onboarding.\n"),
	} {
		if err := copyRegularBytes(body, filepath.Join(root, filepath.FromSlash(relative)), 0o600); err != nil {
			return MacOSPortableResult{}, err
		}
	}
	if err := writeDeterministicZip(staging, rootName, options.Output); err != nil {
		return MacOSPortableResult{}, err
	}
	_, archiveDigest, err := digestBoundedRegular(options.Output, 2<<30)
	if err != nil {
		return MacOSPortableResult{}, err
	}
	if err := writeExclusive(checksumPath, []byte(archiveDigest+"  "+filepath.Base(options.Output)+"\n"), 0o600); err != nil {
		return MacOSPortableResult{}, err
	}
	if err := writeExclusive(provenancePath, provenanceBody, 0o600); err != nil {
		return MacOSPortableResult{}, err
	}
	succeeded = true
	return MacOSPortableResult{Output: options.Output, SHA256: archiveDigest, Status: "unsigned-controlled-canary", Provenance: provenancePath, Checksum: checksumPath}, nil
}

func requireMacOSPortableArtifacts(version string, artifacts []releasecontract.Artifact) error {
	macCLI, bundle := 0, 0
	for _, artifact := range artifacts {
		if artifact.Kind == "cli" && artifact.OS == "darwin" && artifact.Arch == "arm64" {
			macCLI++
			if artifact.Name != "bcgos_"+version+"_darwin_arm64" {
				return errors.New("portable release has an unexpected macOS arm64 CLI identity")
			}
		}
		if artifact.Kind == "bundle" && artifact.OS == "any" && artifact.Arch == "any" {
			bundle++
			if artifact.Name != "maestro-base_"+version+".tar.gz" {
				return errors.New("portable release has an unexpected base bundle identity")
			}
		}
	}
	if macCLI != 1 || bundle != 1 {
		return errors.New("portable release requires exactly one macOS arm64 CLI and one platform-neutral base bundle")
	}
	return nil
}

func macOSPortableClaudeOnboarding() []byte {
	return []byte(`# Maestro Portable - primeiro uso

Voce esta na pasta de trabalho correta do Maestro. Conduza a pessoa em portugues claro, sem presumir conhecimento tecnico.

## Regra de primeiro uso

1. Antes de pedir confirmacao, confirme silenciosamente que o sistema e macOS Apple Silicon usando o contexto da sessao ou uma verificacao somente leitura.
2. Se nao for macOS arm64, pare antes da ativacao. Explique que este pacote funciona somente em macOS Apple Silicon e nao tente executar, converter ou substituir seus binarios.
3. No macOS arm64, verifique silenciosamente se .bcgos/runtime-projection.json existe.
4. Se existir, releia este CLAUDE.md e siga o bloco gerenciado do Maestro. Se a orientacao atual indicar que o onboarding ainda esta incompleto, use a skill maestro-onboarding e continue uma pergunta por vez. Se ele ja estiver concluido, atenda ao pedido normal da pessoa e nao repita o onboarding.
5. Se nao existir, explique em uma frase que voce pode preparar o Maestro nesta pasta. Peca uma unica confirmacao curta antes de qualquer ativacao, por exemplo: "Posso preparar o Maestro agora?".
6. Nao peca para a pessoa digitar ou executar comandos, abrir terminal ou localizar arquivos internos.
7. Somente depois de uma resposta afirmativa clara, execute internamente, a partir desta pasta: ../managed/bcgos-bootstrap portable-install. Em seguida, use somente o CLI instalado em ../managed/bin/bcgos para executar setup apply para este workspace e adapter verify para Claude.
8. A permissao nativa do Claude Code ou do macOS pode aparecer. Explique que a pessoa deve aprovar somente se reconhecer este pacote Maestro; isso nao e uma segunda autorizacao do produto.
9. Se a preparacao terminar com sucesso, releia este CLAUDE.md porque a projecao gerenciada foi acrescentada, informe que a preparacao terminou e invoque a skill maestro-onboarding.
10. Se falhar, nao improvise instalacao, nao baixe substitutos e nao altere a estrutura. Resuma o erro em linguagem simples, confirme que nenhum trabalho da pessoa foi apagado e oriente-a a procurar o responsavel pelo piloto.

A ativacao e idempotente: se uma tentativa anterior tiver terminado parcialmente, use o mesmo fluxo apos nova confirmacao e deixe o ativador verificar, reparar ou concluir o estado existente.
`)
}

func macOSPortableReadme(version string) []byte {
	return []byte("# Maestro Portable " + version + " para macOS\n\n" +
		"1. Extraia a pasta completa para um local fixo em que voce possa gravar arquivos.\n" +
		"2. Abra a pasta `maestro-os` no Claude Code.\n" +
		"3. Envie uma mensagem como `Quero comecar`. O Claude explica e conduz o restante.\n\n" +
		"Nao abra terminal nem execute arquivos internos. Na primeira preparacao, o Claude pedira uma confirmacao curta e o macOS ou o Claude Code podera mostrar uma permissao nativa para voce aprovar. Depois disso, use sempre a mesma pasta `maestro-os`; nao mova a pasta completa depois da ativacao.\n\n" +
		"Este e um pacote Canary controlado sem Developer ID ou notarizacao. Antes da entrega, o responsavel pelo piloto deve conferir o SHA-256 enviado separadamente. Gatekeeper pode bloquear executaveis sem assinatura.\n")
}

func macOSStructuralSignatureStatus(path string) (string, error) {
	file, err := macho.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	for _, load := range file.Loads {
		raw := load.Raw()
		if len(raw) >= 4 && file.ByteOrder.Uint32(raw[:4]) == machOLoadCommandCodeSignature {
			return "Signed", nil
		}
	}
	return "NotSigned", nil
}

func macOSNativeSignatureStatus(path string) (string, error) {
	if runtime.GOOS != "darwin" {
		return "", errors.New("native macOS codesign inspection is unavailable on this host")
	}
	output, err := exec.Command("/usr/bin/codesign", "-d", "--verbose=4", path).CombinedOutput()
	if err != nil {
		if bytes.Contains(output, []byte("code object is not signed at all")) {
			return "NotSigned", nil
		}
		return "", errors.New(strings.TrimSpace(string(output)))
	}
	return "Signed", nil
}
