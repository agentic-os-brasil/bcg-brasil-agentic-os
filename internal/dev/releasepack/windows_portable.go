package releasepack

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releaseverify"
)

var portableVersionPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)
var portableDigestPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

const portableInstallContract = "maestro-portable-install-v2"

type BootstrapperSeedStatus struct {
	Version                 string `json:"bootstrapper_version"`
	AuthorityRegistrySHA256 string `json:"authority_registry_sha256"`
}

type WindowsPortableOptions struct {
	Version                 string
	ReleaseDirectory        string
	AuthorityRegistry       string
	AuthorityRegistrySHA256 string
	Bootstrapper            string
	BootstrapperSHA256      string
	Output                  string
	Clock                   func() time.Time
	BootstrapperSeedStatus  func(string) (BootstrapperSeedStatus, error)
	NativeSignatureStatus   func(string) (string, error)
}

type WindowsPortableResult struct {
	Output     string `json:"output"`
	SHA256     string `json:"sha256"`
	Status     string `json:"status"`
	Provenance string `json:"provenance"`
	Checksum   string `json:"checksum"`
}

type windowsPortableProvenance struct {
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
	BootstrapperSignatureStatus string `json:"bootstrapper_authenticode_status"`
	Status                      string `json:"status"`
}

func BuildWindowsPortable(options WindowsPortableOptions) (WindowsPortableResult, error) {
	if !portableVersionPattern.MatchString(options.Version) {
		return WindowsPortableResult{}, errors.New("portable version must be MAJOR.MINOR.PATCH")
	}
	if !portableDigestPattern.MatchString(options.AuthorityRegistrySHA256) ||
		!portableDigestPattern.MatchString(options.BootstrapperSHA256) {
		return WindowsPortableResult{}, errors.New("portable authority-registry and bootstrapper pins must be lowercase SHA-256 values")
	}
	for label, path := range map[string]string{
		"signed release":     options.ReleaseDirectory,
		"authority registry": options.AuthorityRegistry,
		"bootstrapper":       options.Bootstrapper,
		"output":             options.Output,
	} {
		if !filepath.IsAbs(path) {
			return WindowsPortableResult{}, fmt.Errorf("%s path must be absolute", label)
		}
	}
	expectedOutput := "Maestro-Portable-" + options.Version + "-windows-amd64-local-beta-unsigned.zip"
	if filepath.Base(options.Output) != expectedOutput {
		return WindowsPortableResult{}, fmt.Errorf("portable output must be named %s", expectedOutput)
	}
	checksumPath := options.Output + ".sha256"
	provenancePath := options.Output + ".provenance.json"
	for _, output := range []string{options.Output, checksumPath, provenancePath} {
		if _, err := os.Lstat(output); err == nil {
			return WindowsPortableResult{}, fmt.Errorf("portable output already exists: %s", output)
		} else if !errors.Is(err, os.ErrNotExist) {
			return WindowsPortableResult{}, err
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
		return WindowsPortableResult{}, fmt.Errorf("read portable authority registry: %w", err)
	}
	if registryDigest != options.AuthorityRegistrySHA256 {
		return WindowsPortableResult{}, errors.New("portable authority registry does not match its approved pin")
	}
	clock := options.Clock
	if clock == nil {
		clock = time.Now
	}
	registry, err := releaseverify.LoadPinnedAuthorityRegistry(options.AuthorityRegistry, options.AuthorityRegistrySHA256, clock)
	if err != nil {
		return WindowsPortableResult{}, err
	}
	verified, err := releaseverify.VerifyDirectory(options.ReleaseDirectory, registry)
	if err != nil {
		return WindowsPortableResult{}, fmt.Errorf("verify portable signed release: %w", err)
	}
	manifest := verified.Manifest
	if manifest.Release != options.Version || manifest.Channel != "canary" {
		return WindowsPortableResult{}, errors.New("portable package requires the exact canary release version")
	}
	if _, ok := registry.Lookup("maestro", manifest.Issuer.ID, manifest.Issuer.KeyID); !ok {
		return WindowsPortableResult{}, errors.New("portable release issuer is not active in the pinned registry")
	}
	if err := requirePortableArtifacts(manifest.Release, manifest.Artifacts); err != nil {
		return WindowsPortableResult{}, err
	}
	expectedBootstrapper := "bcgos-bootstrap_" + options.Version + "_windows_amd64.exe"
	if filepath.Base(options.Bootstrapper) != expectedBootstrapper {
		return WindowsPortableResult{}, fmt.Errorf("portable bootstrapper must be named %s", expectedBootstrapper)
	}
	bootstrapperBody, bootstrapperDigest, err := readSeedInput(options.Bootstrapper, 1<<30)
	if err != nil {
		return WindowsPortableResult{}, fmt.Errorf("read portable bootstrapper: %w", err)
	}
	if bootstrapperDigest != options.BootstrapperSHA256 {
		return WindowsPortableResult{}, errors.New("portable bootstrapper does not match its approved pin")
	}
	nativeStatus := options.NativeSignatureStatus
	if nativeStatus == nil {
		nativeStatus = windowsAuthenticodeStatus
	}
	status, err := nativeStatus(options.Bootstrapper)
	if err != nil {
		return WindowsPortableResult{}, fmt.Errorf("inspect portable bootstrapper Authenticode: %w", err)
	}
	if status != "NotSigned" {
		return WindowsPortableResult{}, fmt.Errorf("portable local-beta bootstrapper Authenticode status must be exactly NotSigned; got %s", status)
	}
	seedStatus := options.BootstrapperSeedStatus
	if seedStatus == nil {
		expectedVersion := options.Version
		expectedRegistry := options.AuthorityRegistrySHA256
		seedStatus = func(path string) (BootstrapperSeedStatus, error) {
			return readBootstrapperSeedStatus(path, expectedVersion, expectedRegistry)
		}
	}
	seed, err := seedStatus(options.Bootstrapper)
	if err != nil {
		return WindowsPortableResult{}, fmt.Errorf("inspect portable bootstrapper seed: %w", err)
	}
	if seed.Version != options.Version || seed.AuthorityRegistrySHA256 != options.AuthorityRegistrySHA256 {
		return WindowsPortableResult{}, errors.New("portable bootstrapper seed does not match the release and authority registry")
	}
	if !bytes.Contains(bootstrapperBody, []byte(portableInstallContract)) {
		return WindowsPortableResult{}, errors.New("portable bootstrapper does not support portable core installation")
	}

	parent := filepath.Dir(options.Output)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return WindowsPortableResult{}, err
	}
	staging, err := os.MkdirTemp(parent, ".maestro-portable-")
	if err != nil {
		return WindowsPortableResult{}, err
	}
	defer os.RemoveAll(staging)
	rootName := "Maestro-Portable-" + options.Version + "-windows-amd64"
	root := filepath.Join(staging, rootName)
	if err := os.MkdirAll(filepath.Join(root, "managed", "trust"), 0o700); err != nil {
		return WindowsPortableResult{}, err
	}
	if err := os.MkdirAll(filepath.Join(root, "maestro-os"), 0o755); err != nil {
		return WindowsPortableResult{}, err
	}
	if err := copyRegularBytes(registryBody, filepath.Join(root, "managed", "trust", "release-authority-registry.json"), 0o600); err != nil {
		return WindowsPortableResult{}, err
	}
	if err := copyRegularBytes(bootstrapperBody, filepath.Join(root, "managed", "bcgos-bootstrap.exe"), 0o700); err != nil {
		return WindowsPortableResult{}, err
	}
	if err := copyClosedRelease(options.ReleaseDirectory, filepath.Join(root, "release")); err != nil {
		return WindowsPortableResult{}, err
	}
	provenance := windowsPortableProvenance{
		SchemaVersion: 1, Product: "maestro", Version: options.Version,
		TargetOS: "windows", TargetArch: "amd64", DistributionProfile: "windows-portable-local-beta",
		ReleaseManifestSHA256: verified.ManifestSHA256, ReleaseIssuer: manifest.Issuer.ID,
		ReleaseKeyID: manifest.Issuer.KeyID, AuthorityRegistrySHA256: registryDigest,
		BootstrapperSHA256: bootstrapperDigest, BootstrapperSignatureStatus: status,
		Status: "unsigned-controlled-canary",
	}
	provenanceBody, err := json.MarshalIndent(provenance, "", "  ")
	if err != nil {
		return WindowsPortableResult{}, err
	}
	provenanceBody = append(provenanceBody, '\n')
	files := map[string][]byte{
		"README-PORTABLE.md":       portableReadme(options.Version),
		"portable-provenance.json": provenanceBody,
		"maestro-os/CLAUDE.md":     portableClaudeOnboarding(),
		"maestro-os/README.md":     []byte("# Maestro OS\n\nAbra esta pasta no Claude Code e envie uma mensagem para comecar. O Claude conduz a preparacao e o onboarding.\n"),
	}
	for relative, body := range files {
		if err := copyRegularBytes(body, filepath.Join(root, filepath.FromSlash(relative)), 0o600); err != nil {
			return WindowsPortableResult{}, err
		}
	}
	if err := writeDeterministicZip(staging, rootName, options.Output); err != nil {
		return WindowsPortableResult{}, err
	}
	_, archiveDigest, err := digestBoundedRegular(options.Output, 2<<30)
	if err != nil {
		return WindowsPortableResult{}, err
	}
	checksumBody := []byte(archiveDigest + "  " + filepath.Base(options.Output) + "\n")
	if err := writeExclusive(checksumPath, checksumBody, 0o600); err != nil {
		return WindowsPortableResult{}, err
	}
	if err := writeExclusive(provenancePath, provenanceBody, 0o600); err != nil {
		return WindowsPortableResult{}, err
	}
	succeeded = true
	return WindowsPortableResult{
		Output: options.Output, SHA256: archiveDigest, Status: "unsigned-controlled-canary",
		Provenance: provenancePath, Checksum: checksumPath,
	}, nil
}

func requirePortableArtifacts(version string, artifacts []releasecontract.Artifact) error {
	windowsCLI := 0
	bundle := 0
	for _, artifact := range artifacts {
		if artifact.Kind == "cli" && artifact.OS == "windows" && artifact.Arch == "amd64" {
			windowsCLI++
			if artifact.Name != "bcgos_"+version+"_windows_amd64.exe" {
				return errors.New("portable release has an unexpected Windows CLI identity")
			}
		}
		if artifact.Kind == "bundle" && artifact.OS == "any" && artifact.Arch == "any" {
			bundle++
			if artifact.Name != "maestro-base_"+version+".tar.gz" {
				return errors.New("portable release has an unexpected base bundle identity")
			}
		}
	}
	if windowsCLI != 1 || bundle != 1 {
		return errors.New("portable release requires exactly one Windows amd64 CLI and one platform-neutral base bundle")
	}
	return nil
}

func copyClosedRelease(source, destination string) error {
	entries, err := os.ReadDir(source)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(destination, 0o700); err != nil {
		return err
	}
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("portable signed release contains a non-regular entry: %s", entry.Name())
		}
		body, _, err := readSeedInput(filepath.Join(source, entry.Name()), 1<<30)
		if err != nil {
			return err
		}
		if err := copyRegularBytes(body, filepath.Join(destination, entry.Name()), 0o600); err != nil {
			return err
		}
	}
	return nil
}

func copyRegularBytes(body []byte, destination string, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		return err
	}
	return os.WriteFile(destination, body, mode)
}

func writeExclusive(path string, body []byte, mode os.FileMode) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	if _, err := file.Write(body); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

func writeDeterministicZip(staging, rootName, output string) error {
	var paths []string
	root := filepath.Join(staging, rootName)
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || (!entry.IsDir() && !info.Mode().IsRegular()) {
			return fmt.Errorf("portable staging contains an unsafe entry: %s", path)
		}
		paths = append(paths, path)
		return nil
	})
	if err != nil {
		return err
	}
	sort.Strings(paths)
	temporary := output + ".tmp"
	file, err := os.OpenFile(temporary, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	writer := zip.NewWriter(file)
	fixedTime := time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC)
	writeErr := error(nil)
	for _, path := range paths {
		relative, err := filepath.Rel(staging, path)
		if err != nil {
			writeErr = err
			break
		}
		name := filepath.ToSlash(relative)
		info, err := os.Lstat(path)
		if err != nil {
			writeErr = err
			break
		}
		header := &zip.FileHeader{Name: name, Method: zip.Deflate}
		header.SetModTime(fixedTime)
		if info.IsDir() {
			header.Name += "/"
			header.Method = zip.Store
			header.SetMode(os.ModeDir | 0o755)
		} else {
			mode := os.FileMode(0o600)
			if info.Mode()&0o111 != 0 {
				mode = 0o700
			}
			header.SetMode(mode)
		}
		entry, err := writer.CreateHeader(header)
		if err != nil {
			writeErr = err
			break
		}
		if info.IsDir() {
			continue
		}
		input, err := os.Open(path)
		if err != nil {
			writeErr = err
			break
		}
		_, copyErr := io.Copy(entry, input)
		closeErr := input.Close()
		if copyErr != nil {
			writeErr = copyErr
			break
		}
		if closeErr != nil {
			writeErr = closeErr
			break
		}
	}
	if closeErr := writer.Close(); writeErr == nil {
		writeErr = closeErr
	}
	if closeErr := file.Close(); writeErr == nil {
		writeErr = closeErr
	}
	if writeErr != nil {
		_ = os.Remove(temporary)
		return writeErr
	}
	return os.Rename(temporary, output)
}

func portableReadme(version string) []byte {
	archive := "Maestro-Portable-" + version + "-windows-amd64-local-beta-unsigned.zip"
	return []byte("# Maestro Portable " + version + " para Windows\n\n" +
		"1. Extraia a pasta completa para um local fixo em que voce possa gravar arquivos.\n" +
		"2. Abra a pasta `maestro-os` no Claude Code.\n" +
		"3. Envie uma mensagem como `Quero comecar`. O Claude explica e conduz o restante.\n\n" +
		"Nao abra terminal nem execute arquivos internos. Na primeira preparacao, o Claude pedira uma confirmacao curta e o Windows ou o Claude Code podera mostrar uma permissao nativa para voce aprovar. Depois disso, use sempre a mesma pasta `maestro-os`; nao mova a pasta completa depois da ativacao.\n\n" +
		"Este e um pacote Canary controlado e sem assinatura Authenticode. Antes da entrega, o responsavel pelo piloto deve conferir o SHA-256 enviado separadamente. SmartScreen, WDAC ou AppLocker ainda podem bloquear executaveis sem assinatura.\n\n" +
		"## Verificar o SHA-256 antes de abrir\n\n" +
		"Antes de extrair, abra o PowerShell na pasta em que o arquivo foi salvo e rode:\n\n" +
		"```powershell\n" +
		"Get-FileHash -Algorithm SHA256 " + archive + "\n" +
		"```\n\n" +
		"Compare o valor exibido com o SHA-256 enviado pelo responsavel pelo piloto por um canal separado. Se diferir, nao extraia o ZIP e avise o responsavel.\n\n" +
		"## Se o Windows bloquear na primeira execucao\n\n" +
		"O SmartScreen pode aparecer com \"O Windows protegeu o seu PC\". Clique em `Mais informacoes` e depois em `Executar assim mesmo` somente se voce reconhecer este pacote Maestro entregue pelo responsavel pelo piloto. Ambientes com WDAC ou AppLocker corporativo podem bloquear de vez; nesse caso, encaminhe ao responsavel para whitelisting.\n\n" +
		"## Se voce moveu a pasta depois de ativar\n\n" +
		"O Maestro grava estado dentro da pasta portable e tambem em `%LOCALAPPDATA%\\BCGOS`. Se a pasta portable foi movida ou renomeada apos a ativacao, volte a pasta ao local original antes de continuar. Se isso nao for possivel, avise o responsavel pelo piloto antes de tentar reativar; nao apague `%LOCALAPPDATA%\\BCGOS` por conta propria.\n\n" +
		"## Se algo falhar\n\n" +
		"Copie a mensagem de erro exibida pelo Claude Code e envie ao responsavel pelo piloto junto com o SHA-256 do ZIP e o horario aproximado. Nao baixe substitutos, nao edite arquivos internos e nao apague nada dentro da pasta portable nem em `%LOCALAPPDATA%\\BCGOS` sem instrucao explicita. Nenhum trabalho da pessoa e apagado por uma ativacao malsucedida.\n")
}

func portableClaudeOnboarding() []byte {
	return []byte(`# Maestro Portable - primeiro uso

Voce esta na pasta de trabalho correta do Maestro. Conduza a pessoa em portugues claro, sem presumir conhecimento tecnico.

## Regra de primeiro uso

1. Antes de pedir confirmacao, confirme silenciosamente que o sistema e Windows usando o contexto da sessao ou uma verificacao somente leitura.
2. Se nao for Windows, pare antes da ativacao. Explique que este pacote funciona somente em Windows amd64 e nao tente executar, converter ou substituir seus binarios.
3. No Windows, verifique silenciosamente se .bcgos/runtime-projection.json existe.
4. Se existir, releia este CLAUDE.md e siga o bloco gerenciado do Maestro. Se a orientacao atual indicar que o onboarding ainda esta incompleto, use a skill maestro-onboarding e continue uma pergunta por vez. Se ele ja estiver concluido, atenda ao pedido normal da pessoa e nao repita o onboarding.
5. Se nao existir, explique em uma frase que voce pode preparar o Maestro nesta pasta. Peca uma unica confirmacao curta antes de qualquer ativacao, por exemplo: "Posso preparar o Maestro agora?".
6. Nao peca para a pessoa digitar ou executar comandos, abrir terminal ou localizar arquivos internos.
7. Somente depois de uma resposta afirmativa clara, execute internamente, a partir desta pasta: ..\managed\bcgos-bootstrap.exe portable-install. Em seguida, use somente o CLI instalado em ..\managed\bin\bcgos.exe para executar setup apply para este workspace e adapter verify para Claude.
8. A permissao nativa do Claude Code ou do Windows pode aparecer. Explique que a pessoa deve aprovar somente se reconhecer este pacote Maestro; isso nao e uma segunda autorizacao do produto.
9. Se a preparacao terminar com sucesso, releia este CLAUDE.md porque a projecao gerenciada foi acrescentada, informe que a preparacao terminou e invoque a skill maestro-onboarding.
10. Se falhar, nao improvise instalacao, nao baixe substitutos e nao altere a estrutura. Capture a mensagem exata de erro da execucao interna, resuma em linguagem simples, confirme que nenhum trabalho da pessoa foi apagado e oriente-a a enviar o erro e o SHA-256 do ZIP ao responsavel pelo piloto. Nao apague %LOCALAPPDATA%\BCGOS nem arquivos internos da pasta portable sem instrucao explicita do responsavel.

A ativacao e idempotente: se uma tentativa anterior tiver terminado parcialmente, use o mesmo fluxo apos nova confirmacao e deixe o ativador verificar, reparar ou concluir o estado existente.
`)
}

func readBootstrapperSeedStatus(path, expectedVersion, expectedRegistry string) (BootstrapperSeedStatus, error) {
	if runtime.GOOS == "windows" {
		output, err := exec.Command(path, "seed-status").Output()
		if err != nil {
			return BootstrapperSeedStatus{}, err
		}
		return parseBootstrapperSeedStatus(output)
	}
	// A Windows bootstrapper cannot execute on macOS/Linux. The version and
	// registry digest are linker-bound into the PE and are checked as bounded
	// byte strings here; the Windows factory still executes seed-status.
	info, err := os.Lstat(path)
	if err != nil {
		return BootstrapperSeedStatus{}, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return BootstrapperSeedStatus{}, errors.New("bootstrapper seed must be a regular file")
	}
	if info.Size() <= 0 || info.Size() > 256<<20 {
		return BootstrapperSeedStatus{}, errors.New("bootstrapper seed exceeds the inspection bound")
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return BootstrapperSeedStatus{}, err
	}
	if !bytes.Contains(body, []byte(expectedVersion)) || !bytes.Contains(body, []byte(expectedRegistry)) {
		return BootstrapperSeedStatus{}, errors.New("bootstrapper seed does not contain the expected linker-bound version and registry digest")
	}
	return BootstrapperSeedStatus{Version: expectedVersion, AuthorityRegistrySHA256: expectedRegistry}, nil
}

func parseBootstrapperSeedStatus(output []byte) (BootstrapperSeedStatus, error) {
	decoder := json.NewDecoder(bytes.NewReader(output))
	decoder.DisallowUnknownFields()
	var status struct {
		SchemaVersion           int    `json:"schema_version"`
		Product                 string `json:"product"`
		BootstrapperVersion     string `json:"bootstrapper_version"`
		AuthorityRegistrySHA256 string `json:"authority_registry_sha256"`
	}
	if err := decoder.Decode(&status); err != nil {
		return BootstrapperSeedStatus{}, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return BootstrapperSeedStatus{}, errors.New("bootstrapper seed status contains multiple JSON values")
	}
	if status.SchemaVersion != 1 || status.Product != "maestro" {
		return BootstrapperSeedStatus{}, errors.New("bootstrapper seed status has an unexpected identity")
	}
	return BootstrapperSeedStatus{Version: status.BootstrapperVersion, AuthorityRegistrySHA256: status.AuthorityRegistrySHA256}, nil
}

func windowsAuthenticodeStatus(path string) (string, error) {
	if runtime.GOOS != "windows" {
		return peCertificateTableStatus(path)
	}
	escaped := strings.ReplaceAll(path, "'", "''")
	command := "$ErrorActionPreference='Stop'; $s=Get-AuthenticodeSignature -LiteralPath '" + escaped + "'; [Console]::Out.Write([string]$s.Status)"
	output, err := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", command).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func peCertificateTableStatus(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return "", err
	}
	if info.Size() < 64 {
		return "", errors.New("bootstrapper is too small to be a well-formed PE file")
	}
	read16 := func(offset int64) (uint16, error) {
		if offset < 0 || offset > info.Size()-2 {
			return 0, errors.New("PE field lies outside the bootstrapper")
		}
		var body [2]byte
		if _, err := file.ReadAt(body[:], offset); err != nil {
			return 0, err
		}
		return binary.LittleEndian.Uint16(body[:]), nil
	}
	read32 := func(offset int64) (uint32, error) {
		if offset < 0 || offset > info.Size()-4 {
			return 0, errors.New("PE field lies outside the bootstrapper")
		}
		var body [4]byte
		if _, err := file.ReadAt(body[:], offset); err != nil {
			return 0, err
		}
		return binary.LittleEndian.Uint32(body[:]), nil
	}
	var dos [2]byte
	if _, err := file.ReadAt(dos[:], 0); err != nil || dos != [2]byte{'M', 'Z'} {
		return "", errors.New("bootstrapper is missing the PE DOS signature")
	}
	peOffset, err := read32(0x3c)
	if err != nil || int64(peOffset) < 64 || int64(peOffset) > info.Size()-24 {
		return "", errors.New("bootstrapper has an invalid PE header offset")
	}
	var signature [4]byte
	if _, err := file.ReadAt(signature[:], int64(peOffset)); err != nil || signature != [4]byte{'P', 'E', 0, 0} {
		return "", errors.New("bootstrapper is missing the PE signature")
	}
	optionalSize, err := read16(int64(peOffset) + 20)
	if err != nil {
		return "", err
	}
	optionalOffset := int64(peOffset) + 24
	if optionalSize == 0 || optionalOffset+int64(optionalSize) > info.Size() {
		return "", errors.New("bootstrapper has an invalid PE optional header")
	}
	magic, err := read16(optionalOffset)
	if err != nil {
		return "", err
	}
	dataDirectoryOffset := int64(0)
	switch magic {
	case 0x10b:
		dataDirectoryOffset = 96
	case 0x20b:
		dataDirectoryOffset = 112
	default:
		return "", errors.New("bootstrapper uses an unsupported PE optional-header format")
	}
	certificateEntryOffset := dataDirectoryOffset + 4*8
	if int64(optionalSize) < certificateEntryOffset+8 {
		return "", errors.New("bootstrapper PE optional header lacks the certificate-table entry")
	}
	numberOfDirectories, err := read32(optionalOffset + dataDirectoryOffset - 4)
	if err != nil || numberOfDirectories < 5 {
		return "", errors.New("bootstrapper PE optional header lacks the certificate-table directory")
	}
	certificateOffset, err := read32(optionalOffset + certificateEntryOffset)
	if err != nil {
		return "", err
	}
	certificateSize, err := read32(optionalOffset + certificateEntryOffset + 4)
	if err != nil {
		return "", err
	}
	if certificateOffset == 0 && certificateSize == 0 {
		return "NotSigned", nil
	}
	if certificateOffset == 0 || certificateSize == 0 || int64(certificateOffset)+int64(certificateSize) > info.Size() {
		return "", errors.New("bootstrapper PE certificate-table entry is malformed")
	}
	return "CertificatePresent", nil
}
