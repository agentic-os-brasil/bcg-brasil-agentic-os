package main

import (
	stdbinary "encoding/binary"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestWindowsFactoriesRejectIncompleteLocalBetaProfileBeforeInputs(t *testing.T) {
	pwsh, err := exec.LookPath("pwsh")
	if err != nil {
		if runtime.GOOS == "windows" {
			pwsh, err = exec.LookPath("powershell.exe")
		}
		if err != nil {
			t.Skip("PowerShell is unavailable")
		}
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	arguments := []string{
		"-NoProfile", "-NonInteractive", "-File",
		filepath.Join(root, "dev", "release", "build-windows-installer.ps1"),
		"-Version", "0.1.21",
		"-Icon", filepath.Join(root, "missing.ico"),
		"-IconSHA256", strings.Repeat("0", 64),
		"-WizardDir", filepath.Join(root, "installers", "wizard"),
		"-ReleaseDirectory", filepath.Join(root, "missing-release"),
		"-AuthorityRegistry", filepath.Join(root, "missing-registry.json"),
		"-Bootstrapper", filepath.Join(root, "missing-bootstrapper.exe"),
		"-ResourceCompilerSHA256", strings.Repeat("0", 64),
		"-OutputDirectory", filepath.Join(root, "missing-output"),
		"-LocalBeta",
	}
	output, err := exec.Command(pwsh, arguments...).CombinedOutput()
	normalizedOutput := strings.Join(strings.Fields(string(output)), " ")
	if err == nil ||
		!strings.Contains(normalizedOutput, "LocalBeta requires issuer, key ID") ||
		!strings.Contains(normalizedOutput, "bootstrapper SHA-256 pins.") {
		t.Fatalf("incomplete local-beta profile: err=%v output=%s", err, output)
	}
}

func TestCrossPlatformPECertificateInspectionDistinguishesNotSigned(t *testing.T) {
	pwsh, err := exec.LookPath("pwsh")
	if err != nil {
		t.Skip("cross-platform PowerShell is unavailable")
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	helper := filepath.Join(root, "dev", "release", "windows-native-signature.ps1")
	for _, test := range []struct {
		name              string
		certificateOffset uint32
		certificateSize   uint32
		want              string
		wantErr           bool
	}{
		{name: "not signed", want: "NotSigned"},
		{name: "certificate present", certificateOffset: 480, certificateSize: 8, want: "CertificatePresent"},
		{name: "malformed certificate entry", certificateOffset: 480, wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			body := make([]byte, 512)
			body[0], body[1] = 'M', 'Z'
			stdbinary.LittleEndian.PutUint32(body[0x3c:], 0x80)
			copy(body[0x80:], []byte{'P', 'E', 0, 0})
			stdbinary.LittleEndian.PutUint16(body[0x80+4:], 0x8664)
			stdbinary.LittleEndian.PutUint16(body[0x80+20:], 240)
			optional := 0x80 + 24
			stdbinary.LittleEndian.PutUint16(body[optional:], 0x20b)
			stdbinary.LittleEndian.PutUint32(body[optional+108:], 16)
			certificateEntry := optional + 112 + 4*8
			stdbinary.LittleEndian.PutUint32(body[certificateEntry:], test.certificateOffset)
			stdbinary.LittleEndian.PutUint32(body[certificateEntry+4:], test.certificateSize)
			path := filepath.Join(t.TempDir(), "bootstrapper.exe")
			if err := os.WriteFile(path, body, 0o600); err != nil {
				t.Fatal(err)
			}
			command := ". '" + strings.ReplaceAll(helper, "'", "''") + "'; Get-MaestroPECertificateTableStatus -Path '" + strings.ReplaceAll(path, "'", "''") + "'"
			output, err := exec.Command(pwsh, "-NoProfile", "-NonInteractive", "-Command", command).CombinedOutput()
			if test.wantErr {
				if err == nil || !strings.Contains(string(output), "certificate-table entry is malformed") {
					t.Fatalf("malformed PE certificate entry: output=%q err=%v", output, err)
				}
				return
			}
			if err != nil || strings.TrimSpace(string(output)) != test.want {
				t.Fatalf("PE certificate status: got=%q want=%q err=%v", output, test.want, err)
			}
		})
	}
}

func TestWindowsSingleFileFactoryDeclaresPinnedLocalBetaInputs(t *testing.T) {
	body, err := os.ReadFile("build-windows-singlefile-installer.ps1")
	if err != nil {
		t.Fatal(err)
	}
	source := string(body)
	for _, required := range []string{
		"LocalBetaIssuer",
		"LocalBetaKeyID",
		"LocalBetaAuthorityRegistrySHA256",
		"LocalBetaBootstrapperSHA256",
		"distribution_profile",
		"windows-local-beta",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("single-file factory is missing local-beta contract %q", required)
		}
	}
}

func TestWindowsFactoryRequiresTheOnboardingGuidesInTheReleaseBundle(t *testing.T) {
	body, err := os.ReadFile("build-windows-installer.ps1")
	if err != nil {
		t.Fatal(err)
	}
	source := string(body)
	for _, required := range []string{
		"skills/maestro-onboarding/SKILL.md",
		"skills/maestro-onboarding/agents/openai.yaml",
		"skills/interaction-profile/SKILL.md",
		"skills/interaction-profile/agents/openai.yaml",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("Windows factory must require onboarding artifact %q", required)
		}
	}
}
