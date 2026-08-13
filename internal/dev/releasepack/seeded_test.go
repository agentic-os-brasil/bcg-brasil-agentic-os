package releasepack

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type recordingSeededBuilder struct {
	ldflags map[NativeComponent]string
}

func (builder *recordingSeededBuilder) Build(
	_ context.Context,
	_, output, version string,
	target Target,
	component NativeComponent,
	ldflags string,
) error {
	if builder.ldflags == nil {
		builder.ldflags = map[NativeComponent]string{}
	}
	builder.ldflags[component] = ldflags
	return os.WriteFile(
		output,
		[]byte(string(component)+" "+version+" "+target.OS+"/"+target.Arch),
		0o755,
	)
}

func TestBuildSeededNativeBinariesBindsProviderAndAuthority(t *testing.T) {
	skipIfBundleOnly(t)
	root := t.TempDir()
	providerPath := filepath.Join(root, "provider.json")
	providerBody := approvedProviderBody(t)
	if err := os.WriteFile(providerPath, providerBody, 0o600); err != nil {
		t.Fatal(err)
	}
	registryPath := filepath.Join(root, "registry.json")
	registryBody := authorityRegistryBody(t)
	if err := os.WriteFile(registryPath, registryBody, 0o600); err != nil {
		t.Fatal(err)
	}
	builder := &recordingSeededBuilder{}
	output := filepath.Join(t.TempDir(), "native")
	artifacts, err := BuildSeededNativeBinaries(context.Background(), SeededBuildOptions{
		Root: root, Output: output, Version: "0.2.0",
		Target:            Target{OS: "darwin", Arch: "arm64"},
		ProviderConfig:    providerPath,
		PublicationRepo:   "agentic-os-brasil/maestro-private-releases",
		AuthorityRegistry: registryPath,
		Builder:           builder, Clock: func() time.Time { return time.Unix(2000, 0).UTC() },
	})
	if err != nil {
		t.Fatalf("BuildSeededNativeBinaries() error = %v", err)
	}
	for _, path := range []string{artifacts.CLI, artifacts.Bootstrapper} {
		if info, err := os.Lstat(path); err != nil || !info.Mode().IsRegular() {
			t.Fatalf("seeded binary %q is unavailable: %v", path, err)
		}
	}
	if len(artifacts.AuthorityRegistrySHA256) != 64 {
		t.Fatalf("registry digest = %q", artifacts.AuthorityRegistrySHA256)
	}
	cliFlags := builder.ldflags[NativeCLI]
	if !strings.Contains(cliFlags, "AuthorityRegistrySHA256="+artifacts.AuthorityRegistrySHA256) ||
		!strings.Contains(cliFlags, "ProviderConfigBase64="+base64.StdEncoding.EncodeToString(providerBody)) {
		t.Fatalf("CLI build flags do not bind approved public inputs: %q", cliFlags)
	}
	bootstrapperFlags := builder.ldflags[NativeBootstrapper]
	if !strings.Contains(bootstrapperFlags, "AuthorityRegistrySHA256="+artifacts.AuthorityRegistrySHA256) ||
		strings.Contains(bootstrapperFlags, "ProviderConfigBase64") {
		t.Fatalf("bootstrapper build flags are invalid: %q", bootstrapperFlags)
	}
}

func TestBuildSeededNativeBinariesRejectsUnavailableOrMutableInputs(t *testing.T) {
	root := t.TempDir()
	registryPath := filepath.Join(root, "registry.json")
	if err := os.WriteFile(registryPath, authorityRegistryBody(t), 0o600); err != nil {
		t.Fatal(err)
	}
	providerPath := filepath.Join(root, "provider.json")
	unavailable := `{"schema_version":1,"state":"unavailable","provider":"github",` +
		`"auth_base":"https://github.com","api_base":"https://api.github.com",` +
		`"client_id":"","owner":"","repository":"","reason":"approval pending"}`
	if err := os.WriteFile(providerPath, []byte(unavailable), 0o600); err != nil {
		t.Fatal(err)
	}
	options := SeededBuildOptions{
		Root: root, Output: filepath.Join(t.TempDir(), "native"), Version: "0.2.0",
		Target:            Target{OS: "darwin", Arch: "arm64"},
		ProviderConfig:    providerPath,
		PublicationRepo:   "agentic-os-brasil/maestro-private-releases",
		AuthorityRegistry: registryPath,
		Builder:           &recordingSeededBuilder{},
		Clock:             func() time.Time { return time.Unix(2000, 0).UTC() },
	}
	if _, err := BuildSeededNativeBinaries(context.Background(), options); err == nil {
		t.Fatal("seeded build accepted unavailable provider")
	}

	if err := os.WriteFile(providerPath, approvedProviderBody(t), 0o600); err != nil {
		t.Fatal(err)
	}
	options.Output = filepath.Join(t.TempDir(), "native")
	options.PublicationRepo = "agentic-os-brasil/different-repository"
	if _, err := BuildSeededNativeBinaries(context.Background(), options); err == nil {
		t.Fatal("seeded build accepted provider outside the publication repository")
	}
	options.PublicationRepo = "agentic-os-brasil/maestro-private-releases"

	realRegistry := filepath.Join(root, "real-registry.json")
	if err := os.Rename(registryPath, realRegistry); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(realRegistry, registryPath); err != nil {
		t.Fatal(err)
	}
	options.Output = filepath.Join(t.TempDir(), "native")
	if _, err := BuildSeededNativeBinaries(context.Background(), options); err == nil {
		t.Fatal("seeded build accepted symlinked authority registry")
	}
}

func approvedProviderBody(t *testing.T) []byte {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"schema_version": 1,
		"state":          "approved",
		"provider":       "github",
		"auth_base":      "https://github.com",
		"api_base":       "https://api.github.com",
		"client_id":      "client-id",
		"owner":          "agentic-os-brasil",
		"repository":     "maestro-private-releases",
		"reason":         "",
	})
	if err != nil {
		t.Fatal(err)
	}
	return append(body, '\n')
}

func authorityRegistryBody(t *testing.T) []byte {
	t.Helper()
	publicKey, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(map[string]any{
		"schema_version": 1,
		"product":        "maestro",
		"authorities": []map[string]any{{
			"issuer":      "maestro-release",
			"key_id":      "pilot-2026",
			"algorithm":   "ed25519",
			"public_key":  base64.StdEncoding.EncodeToString(publicKey),
			"status":      "active",
			"valid_from":  "1970-01-01T00:00:00Z",
			"valid_until": "2100-01-01T00:00:00Z",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return append(body, '\n')
}
