// Development-only assembly of an already admitted experimental participant kit.
// It observes only the authenticated staged installer's read-only seed-status;
// release/runtime inputs are never executed and this factory issues no authority.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"debug/buildinfo"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installer"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/pilotdocument"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releaseprovider"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releaseverify"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

type factoryOptions struct {
	Output, Installer, Role, Signature, Registry, RegistrySHA256, InitialRelease, Source, Templates string
}

const pilotDocumentSHA256 = pilotdocument.SHA256

type kitFile struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}
type distribution struct {
	SchemaVersion int                                `json:"schema_version"`
	Profile       string                             `json:"profile"`
	Evidence      string                             `json:"evidence_class"`
	Identity      installer.ExperimentalSeedIdentity `json:"identity"`
	Installer     string                             `json:"installer"`
	ServerHashes  []string                           `json:"server_hashes"`
	Files         []kitFile                          `json:"files"`
}

func main() {
	var o factoryOptions
	flag.StringVar(&o.Output, "out", "", "new private Developer output directory")
	flag.StringVar(&o.Installer, "installer", "", "already built, reviewed experimental installer")
	flag.StringVar(&o.Role, "role", "", "signed public installed-seed role")
	flag.StringVar(&o.Signature, "role-signature", "", "detached role signature")
	flag.StringVar(&o.Registry, "registry", "", "public engineering registry")
	flag.StringVar(&o.RegistrySHA256, "registry-sha256", "", "exact reviewed compiled registry pin")
	flag.StringVar(&o.InitialRelease, "initial-release", "", "signed initial release directory")
	flag.StringVar(&o.Source, "source", "", "signed synced source directory")
	flag.StringVar(&o.Templates, "templates", "", "trusted source template directory")
	flag.Parse()
	if flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "unexpected positional arguments")
		os.Exit(1)
	}
	if err := buildKit(o, time.Now); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("Engineering kit assembled; read-only staged installer identity verified. No installation, client, model, signing or activation was run.")
}

func buildKit(o factoryOptions, clock func() time.Time) error {
	body, err := readBounded(o.Role, 128<<10)
	if err != nil {
		return err
	}
	sig, err := readBounded(o.Signature, 64)
	if err != nil {
		return err
	}
	registryBody, err := readBounded(o.Registry, 1<<20)
	if err != nil {
		return err
	}
	role, err := installer.VerifyExperimentalSeedAdmission(body, sig, registryBody, o.RegistrySHA256, clock)
	if err != nil {
		return err
	}
	id := role.Identity()
	if err = role.VerifyInstaller(o.Installer, id.InstallerVersion, id.TargetOS, id.TargetArch); err != nil {
		return err
	}
	if err = verifyBuild(o.Installer, id); err != nil {
		return err
	}
	registry, err := releaseverify.ParseAuthorityRegistry(bytes.NewReader(registryBody), clock)
	if err != nil {
		return err
	}
	initial, err := releaseverify.VerifyDirectory(o.InitialRelease, registry)
	if err != nil {
		return err
	}
	if err = role.VerifyRelease(initial); err != nil {
		return err
	}
	current, err := releaseprovider.ReadSyncedMetadata(o.Source, id.Source, registry)
	if err != nil {
		return err
	}
	if err = current.Generation.CheckTime(clock(), time.Time{}); err != nil {
		return err
	}
	if err = role.VerifyReleaseIdentity(current.Generation.ManifestSHA256, current.Generation.ClientInputsSHA256); err != nil {
		return err
	}
	document, err := pilotdocument.Bytes()
	if err != nil {
		return err
	}
	if err = validateOutput(o.Output, o.Source, o.InitialRelease); err != nil {
		return err
	}
	if id.TargetOS != runtime.GOOS || id.TargetArch != runtime.GOARCH {
		return fmt.Errorf("installer identity probe requires matching target host %s/%s; current host is %s/%s", id.TargetOS, id.TargetArch, runtime.GOOS, runtime.GOARCH)
	}
	if err = os.Mkdir(o.Output, 0700); err != nil {
		return fmt.Errorf("new output required; existing material preserved: %w", err)
	}
	// Failed assembly is deliberately retained. No path-based recursive cleanup.
	if err = scheduler.EnsurePrivateDirectory(o.Output); err != nil {
		return err
	}
	ext := ""
	if id.TargetOS == "windows" {
		ext = ".exe"
	}
	installerName := "bootstrap/maestro-install_" + id.InstallerVersion + "_" + id.TargetOS + "_" + id.TargetArch + ext
	if err = copyFile(o.Installer, filepath.Join(o.Output, filepath.FromSlash(installerName)), 256<<20); err != nil {
		return err
	}
	if err = role.VerifyInstaller(filepath.Join(o.Output, filepath.FromSlash(installerName)), id.InstallerVersion, id.TargetOS, id.TargetArch); err != nil {
		return err
	}
	for name, b := range map[string][]byte{"experimental-installed-seed.json": body, "experimental-installed-seed.json.sig": sig, "release-authority-registry.json": registryBody} {
		if err = writeNew(filepath.Join(o.Output, "bootstrap", "trust", name), b, 0600); err != nil {
			return err
		}
	}
	if err = copyRelease(initial, filepath.Join(o.Output, "initial-release"), registry); err != nil {
		return err
	}
	if err = writeNew(filepath.Join(o.Output, "test-documents", pilotdocument.Name), document, 0600); err != nil {
		return err
	}
	currentDest := filepath.Join(o.Output, "source", "releases", current.Generation.Release, id.TargetOS+"-"+id.TargetArch)
	if err = os.MkdirAll(filepath.Dir(currentDest), 0700); err != nil {
		return err
	}
	currentRelease, err := releaseprovider.SnapshotSyncedRelease(o.Source, currentDest, current, registry)
	if err != nil {
		return err
	}
	if err = role.VerifyRelease(currentRelease); err != nil {
		return err
	}
	// The original entry needs its legitimate adjacent public role and registry.
	// All copied release/source inputs are authenticated before this sole probe;
	// the fresh bootstrap is retained if identity observation fails.
	if err = verifyStagedInstallerIdentity(filepath.Join(o.Output, filepath.FromSlash(installerName)), role); err != nil {
		return err
	}
	genDir := filepath.Join(o.Output, "source", "generations", current.Generation.GenerationDirectory(current.SHA256))
	for path, b := range map[string][]byte{filepath.Join(o.Output, "source", releaseprovider.CurrentGenerationName): current.Body, filepath.Join(o.Output, "source", releaseprovider.CurrentGenerationSignatureName): current.Signature, filepath.Join(genDir, releaseprovider.GenerationName): current.Body, filepath.Join(genDir, releaseprovider.GenerationSignatureName): current.Signature} {
		if err = writeNew(path, b, 0600); err != nil {
			return err
		}
	}
	if _, err = releaseprovider.ReadSyncedMetadata(filepath.Join(o.Output, "source"), id.Source, registry); err != nil {
		return err
	}
	var record installer.ExperimentalSeedRecord // Only after opaque verification; public metadata, not an admission constructor.
	if err = json.Unmarshal(body, &record); err != nil {
		return err
	}
	d := distribution{SchemaVersion: 1, Profile: installer.ExperimentalSeedProfile, Evidence: "engineering_only", Identity: id, Installer: installerName}
	for _, r := range record.Releases {
		d.ServerHashes = append(d.ServerHashes, r.ServerSHA256)
	}
	for _, dir := range []string{"bootstrap", "initial-release", "test-documents"} {
		err = filepath.WalkDir(filepath.Join(o.Output, dir), func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				return nil
			}
			info, e := entry.Info()
			if e != nil {
				return e
			}
			if !info.Mode().IsRegular() {
				return errors.New("assembly contains a nonregular file")
			}
			rel, e := filepath.Rel(o.Output, path)
			if e != nil {
				return e
			}
			b, e := readBounded(path, 1<<30)
			if e != nil {
				return e
			}
			d.Files = append(d.Files, kitFile{Path: filepath.ToSlash(rel), SHA256: sum(b), Size: int64(len(b))})
			return nil
		})
		if err != nil {
			return err
		}
	}
	sort.Slice(d.Files, func(i, j int) bool { return d.Files[i].Path < d.Files[j].Path })
	metadata, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return err
	}
	metadata = append(metadata, '\n')
	if err = writeNew(filepath.Join(o.Output, "DISTRIBUTION.json"), metadata, 0600); err != nil {
		return err
	}
	files := []string{"LEIA-ME.md"}
	if id.TargetOS == "darwin" {
		files = append(files, "Maestro.command")
	} else {
		files = append(files, "Maestro.cmd", "Maestro.ps1")
	}
	for _, name := range files {
		b, e := readBounded(filepath.Join(o.Templates, name), 256<<10)
		if e != nil {
			return e
		}
		b = bytes.ReplaceAll(b, []byte("@@DISTRIBUTION_SHA256@@"), []byte(sum(metadata)))
		mode := os.FileMode(0600)
		if strings.HasSuffix(name, ".command") {
			mode = 0700
		}
		if e = writeNew(filepath.Join(o.Output, name), b, mode); e != nil {
			return e
		}
	}
	return role.Revalidate()
}

func verifyBuild(path string, id installer.ExperimentalSeedIdentity) error {
	b, err := buildinfo.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reviewed Go installer build metadata required: %w", err)
	}
	if b.Path != "github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp/cmd/maestro-install" {
		return errors.New("input is not the expected installer command")
	}
	settings := map[string]string{}
	for _, v := range b.Settings {
		settings[v.Key] = v.Value
	}
	tag := false
	for _, v := range strings.Split(settings["-tags"], ",") {
		if v == "maestro_experimental_installed_seed" {
			tag = true
		}
	}
	// Go intentionally omits -ldflags metadata from -trimpath builds. The
	// original staged seed-status entry proves compiled pin/version instead.
	if !tag || settings["GOOS"] != id.TargetOS || settings["GOARCH"] != id.TargetArch {
		return errors.New("installer build tag or target differs from admission")
	}
	return nil
}

func verifyStagedInstallerIdentity(path string, role *installer.VerifiedExperimentalSeed) error {
	id := role.Identity()
	if err := role.VerifyInstaller(path, id.InstallerVersion, id.TargetOS, id.TargetArch); err != nil {
		return err
	}
	if err := verifyBuild(path, id); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, path, "seed-status")
	command.Dir = filepath.Dir(path)
	command.WaitDelay = 2 * time.Second
	var output, diagnostic installerIdentityOutput
	command.Stdout, command.Stderr = &output, &diagnostic
	if err := command.Run(); err != nil {
		return fmt.Errorf("staged installer identity probe failed; copied inputs retained: %w: %s", err, strings.TrimSpace(diagnostic.String()))
	}
	var status struct {
		SchemaVersion           int                                 `json:"schema_version"`
		Product                 string                              `json:"product"`
		Component               string                              `json:"component"`
		Protocol                string                              `json:"protocol"`
		InstallerVersion        string                              `json:"installer_version"`
		AuthorityRegistrySHA256 string                              `json:"authority_registry_sha256"`
		ExperimentalSeed        *installer.ExperimentalSeedIdentity `json:"experimental_seed"`
	}
	if err := releasecontract.DecodeStrictJSON(output.Bytes(), &status); err != nil {
		return fmt.Errorf("staged installer identity probe returned invalid status: %w", err)
	}
	if status.SchemaVersion != 3 || status.Product != "maestro" || status.Component != "experimental_installer" || status.Protocol != installer.ExperimentalSeedProtocol || status.InstallerVersion != id.InstallerVersion || status.AuthorityRegistrySHA256 != id.RegistrySHA256 || status.ExperimentalSeed == nil || *status.ExperimentalSeed != id {
		return errors.New("staged installer identity probe differs from exact admitted compiled pin/version/role")
	}
	return role.VerifyInstaller(path, id.InstallerVersion, id.TargetOS, id.TargetArch)
}

type installerIdentityOutput struct{ buffer bytes.Buffer }

func (o *installerIdentityOutput) Bytes() []byte  { return o.buffer.Bytes() }
func (o *installerIdentityOutput) String() string { return o.buffer.String() }

func (o *installerIdentityOutput) Write(p []byte) (int, error) {
	if o.buffer.Len()+len(p) > 16<<10 {
		return 0, errors.New("installer identity probe output exceeds bound")
	}
	return o.buffer.Write(p)
}

func validateOutput(out string, inputs ...string) error {
	if !filepath.IsAbs(out) || filepath.Clean(out) != out {
		return errors.New("absolute clean new Developer output required")
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(out))
	if err != nil {
		return err
	}
	if parent != filepath.Dir(out) || !strings.Contains(filepath.ToSlash(parent)+"/", "/Developer/") {
		return errors.New("output must be physically under Developer")
	}
	for _, input := range inputs {
		p, e := filepath.Abs(input)
		if e != nil {
			return e
		}
		if within(p, out) || within(out, p) {
			return errors.New("output must be disjoint from release/source inputs")
		}
	}
	return nil
}
func within(a, b string) bool {
	rel, err := filepath.Rel(a, b)
	return err == nil && (rel == "." || rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}
func sum(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }
func readBounded(path string, max int64) ([]byte, error) {
	f, err := scheduler.OpenRegularFile(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	before, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if before.Size() > max {
		return nil, errors.New("input exceeds finite copy bound")
	}
	b, err := io.ReadAll(io.LimitReader(f, max+1))
	if err != nil {
		return nil, err
	}
	after, err := f.Stat()
	if err != nil {
		return nil, err
	}
	current, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if int64(len(b)) != before.Size() || len(b) > int(max) || !before.ModTime().Equal(after.ModTime()) || !current.Mode().IsRegular() || !os.SameFile(after, current) {
		return nil, errors.New("input changed while reading")
	}
	return b, nil
}
func writeNew(path string, b []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	_, err = f.Write(b)
	closeErr := f.Close()
	if err != nil {
		return err
	}
	return closeErr
}
func copyFile(src, dst string, max int64) error {
	b, err := readBounded(src, max)
	if err != nil {
		return err
	}
	return writeNew(dst, b, 0700)
}
func copyRelease(v releaseverify.VerifiedRelease, dest string, registry releaseverify.KeyRegistry) error {
	if err := os.Mkdir(dest, 0700); err != nil {
		return err
	}
	files := map[string]int64{releaseverify.ManifestName: 1 << 20, releaseverify.ManifestSignatureName: 64, v.Manifest.ReleaseNotes.Name: 8 << 20}
	for _, a := range v.Manifest.Artifacts {
		files[a.Name] = a.Size
		files[a.SignatureRef] = 64
	}
	for name, max := range files {
		if err := copyFile(filepath.Join(v.Directory, name), filepath.Join(dest, name), max); err != nil {
			return err
		}
	}
	copied, err := releaseverify.VerifyDirectory(dest, registry)
	if err != nil {
		return err
	}
	if copied.ManifestSHA256 != v.ManifestSHA256 {
		return errors.New("initial release changed while copying")
	}
	return nil
}
