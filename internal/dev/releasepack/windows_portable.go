package releasepack

import (
	"archive/zip"
	"bytes"
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
		seedStatus = readBootstrapperSeedStatus
	}
	seed, err := seedStatus(options.Bootstrapper)
	if err != nil {
		return WindowsPortableResult{}, fmt.Errorf("inspect portable bootstrapper seed: %w", err)
	}
	if seed.Version != options.Version || seed.AuthorityRegistrySHA256 != options.AuthorityRegistrySHA256 {
		return WindowsPortableResult{}, errors.New("portable bootstrapper seed does not match the release and authority registry")
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
	if err := os.MkdirAll(filepath.Join(root, "workspace"), 0o755); err != nil {
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
		"Activate-Maestro.cmd":     portableActivationScript(options.Version),
		"README-PORTABLE.md":       portableReadme(options.Version),
		"portable-provenance.json": provenanceBody,
		"workspace/README.md":      []byte("# Maestro workspace\n\nOpen this workspace folder in Claude Desktop after running Activate-Maestro.cmd once.\n"),
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
			header.SetMode(0o600)
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

func portableActivationScript(version string) []byte {
	return []byte("@echo off\r\n" +
		"setlocal\r\n" +
		"if \"%LOCALAPPDATA%\"==\"\" (\r\n" +
		"  echo LOCALAPPDATA is unavailable. Maestro was not activated.\r\n" +
		"  exit /b 1\r\n" +
		")\r\n" +
		"set \"ROOT=%~dp0\"\r\n" +
		"set \"MANAGED=%ROOT%managed\"\r\n" +
		"set \"DATA=%LOCALAPPDATA%\\BCGOS\"\r\n" +
		"set \"WORKSPACE=%ROOT%workspace\"\r\n" +
		"if not exist \"%MANAGED%\\bin\\bcgos.exe\" (\r\n" +
		"  \"%MANAGED%\\bcgos-bootstrap.exe\" install --verified-directory \"%ROOT%release\" --data-root \"%DATA%\"\r\n" +
		"  if errorlevel 1 exit /b 1\r\n" +
		")\r\n" +
		"\"%MANAGED%\\bin\\bcgos.exe\" version | findstr /x /c:\"bcgos " + version + "\" >nul\r\n" +
		"if errorlevel 1 (\r\n" +
		"  echo The active bcgos version does not match this portable package.\r\n" +
		"  exit /b 1\r\n" +
		")\r\n" +
		"\"%MANAGED%\\bin\\bcgos.exe\" setup apply --workspace \"%WORKSPACE%\" --runtime claude --executable \"%MANAGED%\\bin\\bcgos.exe\" --confirm\r\n" +
		"if errorlevel 1 exit /b 1\r\n" +
		"\"%MANAGED%\\bin\\bcgos.exe\" adapter verify --runtime claude \"%WORKSPACE%\"\r\n" +
		"if errorlevel 1 exit /b 1\r\n" +
		"echo Maestro is ready. Open the workspace folder in Claude Desktop.\r\n")
}

func portableReadme(version string) []byte {
	return []byte("# Maestro Portable " + version + " for Windows\n\n" +
		"Extract this complete directory to a fixed user-writable location and do not move it after activation. " +
		"Run `Activate-Maestro.cmd` once, then open the workspace folder in Claude Desktop every day. " +
		"Claude uses the installed absolute `bcgos.exe` path and the projected managed skills; no terminal or wizard is part of daily use.\n\n" +
		"This is an unsigned controlled Canary package. Verify the separately delivered SHA-256 before activation. " +
		"SmartScreen, WDAC or AppLocker may still block unsigned executables.\n")
}

func readBootstrapperSeedStatus(path string) (BootstrapperSeedStatus, error) {
	if runtime.GOOS != "windows" {
		return BootstrapperSeedStatus{}, errors.New("Windows portable packaging requires a Windows factory to inspect the bootstrapper seed")
	}
	output, err := exec.Command(path, "seed-status").Output()
	if err != nil {
		return BootstrapperSeedStatus{}, err
	}
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
	if status.SchemaVersion != 1 || status.Product != "maestro" {
		return BootstrapperSeedStatus{}, errors.New("bootstrapper seed status has an unexpected identity")
	}
	return BootstrapperSeedStatus{Version: status.BootstrapperVersion, AuthorityRegistrySHA256: status.AuthorityRegistrySHA256}, nil
}

func windowsAuthenticodeStatus(path string) (string, error) {
	if runtime.GOOS != "windows" {
		return "", errors.New("Windows portable packaging requires a Windows factory to inspect Authenticode")
	}
	escaped := strings.ReplaceAll(path, "'", "''")
	command := "$ErrorActionPreference='Stop'; $s=Get-AuthenticodeSignature -LiteralPath '" + escaped + "'; [Console]::Out.Write([string]$s.Status)"
	output, err := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", command).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}
