package releasepack

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releaseprovider"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releaseverify"
)

type NativeComponent string

const (
	NativeCLI          NativeComponent = "cli"
	NativeBootstrapper NativeComponent = "bootstrapper"
)

type SeededBuildOptions struct {
	Root              string
	Output            string
	Version           string
	Target            Target
	ProviderConfig    string
	PublicationRepo   string
	AuthorityRegistry string
	Builder           SeededComponentBuilder
	Clock             func() time.Time
}

type SeededNativeArtifacts struct {
	CLI                     string
	Bootstrapper            string
	AuthorityRegistrySHA256 string
}

type SeededComponentBuilder interface {
	Build(
		context.Context,
		string,
		string,
		string,
		Target,
		NativeComponent,
		string,
	) error
}

type GoSeededComponentBuilder struct{}

func (GoSeededComponentBuilder) Build(
	ctx context.Context,
	root, output, version string,
	target Target,
	component NativeComponent,
	ldflags string,
) error {
	if runtime.GOOS != target.OS || runtime.GOARCH != target.Arch {
		return fmt.Errorf(
			"native seeded build target %s/%s does not match runner %s/%s",
			target.OS, target.Arch, runtime.GOOS, runtime.GOARCH,
		)
	}
	cgo := "0"
	if target.OS == "darwin" {
		cgo = "1"
	}
	environment := append(
		filteredEnvironment(os.Environ(), "GOOS", "GOARCH", "CGO_ENABLED"),
		"CGO_ENABLED="+cgo,
	)
	commandPath := "./cmd/bcgos"
	if component == NativeBootstrapper {
		commandPath = "./cmd/bcgos-bootstrap"
	}
	command := exec.CommandContext(
		ctx,
		"go", "build",
		"-mod=readonly",
		"-buildvcs=false",
		"-trimpath",
		"-ldflags", ldflags,
		"-o", output,
		commandPath,
	)
	command.Dir = root
	command.Env = environment
	outputBytes, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf(
			"build seeded %s for %s/%s: %w: %s",
			component,
			target.OS,
			target.Arch,
			err,
			strings.TrimSpace(string(outputBytes)),
		)
	}
	return nil
}

func BuildSeededNativeBinaries(
	ctx context.Context,
	options SeededBuildOptions,
) (SeededNativeArtifacts, error) {
	if options.Root == "" || options.Output == "" ||
		options.ProviderConfig == "" || options.PublicationRepo == "" ||
		options.AuthorityRegistry == "" {
		return SeededNativeArtifacts{}, errors.New("seeded native build paths are required")
	}
	if !supportedCandidateTarget(options.Target) {
		return SeededNativeArtifacts{}, fmt.Errorf(
			"unsupported seeded native target %s/%s",
			options.Target.OS,
			options.Target.Arch,
		)
	}
	configBody, _, err := readSeedInput(options.ProviderConfig, 16<<10)
	if err != nil {
		return SeededNativeArtifacts{}, err
	}
	config, err := releaseprovider.ParseConfig(bytes.NewReader(configBody))
	if err != nil {
		return SeededNativeArtifacts{}, err
	}
	if !config.Approved() {
		return SeededNativeArtifacts{}, errors.New("seeded native build requires an approved provider configuration")
	}
	if !strings.EqualFold(
		config.Owner+"/"+config.Repository,
		options.PublicationRepo,
	) {
		return SeededNativeArtifacts{}, fmt.Errorf(
			"approved provider repository %s/%s does not match publication repository %s",
			config.Owner,
			config.Repository,
			options.PublicationRepo,
		)
	}
	clock := options.Clock
	if clock == nil {
		clock = time.Now
	}
	registryBody, registryDigest, err := readSeedInput(options.AuthorityRegistry, 1<<20)
	if err != nil {
		return SeededNativeArtifacts{}, err
	}
	if _, err := releaseverify.ParseAuthorityRegistry(bytes.NewReader(registryBody), clock); err != nil {
		return SeededNativeArtifacts{}, fmt.Errorf("load seeded authority registry: %w", err)
	}
	if _, err := releasecontract.ParseVersionRange(compatibleRange(options.Version)); err != nil {
		return SeededNativeArtifacts{}, fmt.Errorf("invalid seeded binary version %q: %w", options.Version, err)
	}
	if _, err := os.Stat(options.Output); err == nil {
		return SeededNativeArtifacts{}, fmt.Errorf("seeded output already exists: %s", options.Output)
	} else if !errors.Is(err, os.ErrNotExist) {
		return SeededNativeArtifacts{}, err
	}
	if options.Builder == nil {
		options.Builder = GoSeededComponentBuilder{}
	}
	parent := filepath.Dir(options.Output)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return SeededNativeArtifacts{}, err
	}
	staging, err := os.MkdirTemp(parent, ".maestro-seeded-native-")
	if err != nil {
		return SeededNativeArtifacts{}, err
	}
	defer os.RemoveAll(staging)
	providerBase64 := base64.StdEncoding.EncodeToString(configBody)
	cliName := binaryName(options.Version, options.Target)
	bootstrapperName := bootstrapperBinaryName(options.Version, options.Target)
	builds := []struct {
		component NativeComponent
		name      string
		ldflags   string
	}{
		{
			component: NativeCLI,
			name:      cliName,
			ldflags: strings.Join([]string{
				"-s", "-w",
				"-X", "github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/cli.Version=" + options.Version,
				"-X", "github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/cli.AuthorityRegistrySHA256=" + registryDigest,
				"-X", "github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/cli.ProviderConfigBase64=" + providerBase64,
			}, " "),
		},
		{
			component: NativeBootstrapper,
			name:      bootstrapperName,
			ldflags: strings.Join([]string{
				"-s", "-w",
				"-X", "main.Version=" + options.Version,
				"-X", "main.AuthorityRegistrySHA256=" + registryDigest,
			}, " "),
		},
	}
	for _, build := range builds {
		output := filepath.Join(staging, build.name)
		if err := options.Builder.Build(
			ctx,
			options.Root,
			output,
			options.Version,
			options.Target,
			build.component,
			build.ldflags,
		); err != nil {
			return SeededNativeArtifacts{}, err
		}
		if _, _, err := digestBoundedRegular(output, 1<<30); err != nil {
			return SeededNativeArtifacts{}, err
		}
	}
	if err := os.Rename(staging, options.Output); err != nil {
		return SeededNativeArtifacts{}, err
	}
	return SeededNativeArtifacts{
		CLI:                     filepath.Join(options.Output, cliName),
		Bootstrapper:            filepath.Join(options.Output, bootstrapperName),
		AuthorityRegistrySHA256: registryDigest,
	}, nil
}

func bootstrapperBinaryName(version string, target Target) string {
	name := "bcgos-bootstrap_" + version + "_" + target.OS + "_" + target.Arch
	if target.OS == "windows" {
		name += ".exe"
	}
	return name
}

func readSeedInput(path string, maximum int64) ([]byte, string, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, "", err
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > maximum {
		return nil, "", errors.New("release seed input must be a bounded regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, "", err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil {
		return nil, "", err
	}
	if !opened.Mode().IsRegular() || !os.SameFile(info, opened) {
		return nil, "", errors.New("release seed input changed while opening")
	}
	body, err := io.ReadAll(io.LimitReader(file, maximum+1))
	if err != nil {
		return nil, "", err
	}
	if int64(len(body)) != info.Size() || int64(len(body)) > maximum {
		return nil, "", errors.New("release seed input changed while reading")
	}
	sum := sha256.Sum256(body)
	return body, hex.EncodeToString(sum[:]), nil
}
