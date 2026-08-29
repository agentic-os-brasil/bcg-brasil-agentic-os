// Package portableactivation verifies the platform CLI already transported in
// a Maestro ZIP and records the exact local activation. It establishes local
// integrity only; native signing and publication remain separate gates.
package portableactivation

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/privatelock"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/userlevel"
)

const (
	ManifestFileName          = "install-manifest.json"
	StateRelativePath         = "install.json"
	PreviousStateRelativePath = "install.previous.json"
	maximumCLIBytes           = 512 << 20
)

var digestPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)
var versionPattern = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)

type Manifest struct {
	SchemaVersion int    `json:"schema_version"`
	Version       string `json:"version"`
	TargetOS      string `json:"target_os"`
	TargetArch    string `json:"target_arch"`
	CLIPath       string `json:"cli_path"`
	CLISHA256     string `json:"cli_sha256"`
}

type Options struct {
	ManagedRoot     string
	DataRoot        string
	ExpectedVersion string
	Clock           func() time.Time
}

type Receipt struct {
	SchemaVersion int    `json:"schema_version"`
	State         string `json:"state"`
	Version       string `json:"version"`
	TargetOS      string `json:"target_os"`
	TargetArch    string `json:"target_arch"`
	CLISHA256     string `json:"cli_sha256"`
}

type state struct {
	SchemaVersion int    `json:"schema_version"`
	Version       string `json:"version"`
	TargetOS      string `json:"target_os"`
	TargetArch    string `json:"target_arch"`
	CLISHA256     string `json:"cli_sha256"`
	ManagedRoot   string `json:"managed_root"`
	DataRoot      string `json:"data_root"`
	CLIPath       string `json:"cli_path"`
	ActivatedAt   string `json:"activated_at"`
}

func Activate(options Options) (result Receipt, returnedErr error) {
	if err := userlevel.EnsureNotElevated(); err != nil {
		return Receipt{}, err
	}
	managedRoot, err := canonicalExistingDirectory(options.ManagedRoot)
	if err != nil {
		return Receipt{}, fmt.Errorf("resolve managed root: %w", err)
	}
	dataRoot, err := canonicalDestination(options.DataRoot)
	if err != nil {
		return Receipt{}, fmt.Errorf("resolve data root: %w", err)
	}
	if sameOrNested(managedRoot, dataRoot) || sameOrNested(dataRoot, managedRoot) {
		return Receipt{}, errors.New("managed root and data root must be separate; nothing was installed or lost")
	}
	var manifest Manifest
	if err := readStrictJSON(filepath.Join(managedRoot, ManifestFileName), &manifest); err != nil {
		return Receipt{}, fmt.Errorf("read portable activation manifest: %w", err)
	}
	if err := validateManifest(manifest); err != nil {
		return Receipt{}, err
	}
	if options.ExpectedVersion != "" && options.ExpectedVersion != "0.0.0-dev" && options.ExpectedVersion != manifest.Version {
		return Receipt{}, errors.New("bootstrapper version does not match the portable activation manifest")
	}
	cliPath := filepath.Join(managedRoot, filepath.FromSlash(manifest.CLIPath))
	if !sameOrNested(managedRoot, cliPath) {
		return Receipt{}, errors.New("portable CLI path escapes managed root")
	}
	if err := rejectSymlinkComponents(managedRoot, filepath.FromSlash(manifest.CLIPath)); err != nil {
		return Receipt{}, err
	}
	digest, err := digestRegular(cliPath)
	if err != nil {
		return Receipt{}, err
	}
	if digest != manifest.CLISHA256 {
		return Receipt{}, errors.New("portable CLI digest does not match the activation manifest; nothing was installed or lost")
	}
	unlock, err := privatelock.Acquire(filepath.Join(dataRoot, ".maestro-activation.lock"))
	if err != nil {
		return Receipt{}, fmt.Errorf("acquire portable activation transaction: %w", err)
	}
	defer func() {
		if unlockErr := unlock(); unlockErr != nil && returnedErr == nil {
			result = Receipt{}
			returnedErr = fmt.Errorf("release portable activation transaction: %w", unlockErr)
		}
	}()
	now := time.Now().UTC()
	if options.Clock != nil {
		now = options.Clock().UTC()
	}
	desired := state{
		SchemaVersion: 1, Version: manifest.Version, TargetOS: manifest.TargetOS,
		TargetArch: manifest.TargetArch, CLISHA256: digest, ManagedRoot: managedRoot,
		DataRoot: dataRoot, CLIPath: cliPath, ActivatedAt: now.Format(time.RFC3339),
	}
	statePath := filepath.Join(dataRoot, StateRelativePath)
	var existing state
	if err := readStrictJSON(statePath, &existing); err == nil {
		if sameActivation(existing, desired) {
			return receipt("already_ready", manifest, digest), nil
		}
		if err := validateReplacement(existing, desired); err != nil {
			return Receipt{}, err
		}
		if err := writeJSONAtomic(filepath.Join(dataRoot, PreviousStateRelativePath), existing); err != nil {
			return Receipt{}, fmt.Errorf("preserve previous activation: %w", err)
		}
		if err := writeJSONAtomic(statePath, desired); err != nil {
			return Receipt{}, fmt.Errorf("activate replacement: %w", err)
		}
		return receipt("updated", manifest, digest), nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return Receipt{}, fmt.Errorf("inspect existing activation: %w", err)
	}
	if err := os.MkdirAll(dataRoot, 0o700); err != nil {
		return Receipt{}, err
	}
	if err := writeJSONAtomic(statePath, desired); err != nil {
		return Receipt{}, err
	}
	return receipt("activated", manifest, digest), nil
}

// Verify is the read-only gate used by the installed CLI before every public
// workspace operation and hook event. It proves only local target/version/
// digest and exact activation-state agreement.
func Verify(options Options) (Receipt, error) {
	if err := userlevel.EnsureNotElevated(); err != nil {
		return Receipt{}, err
	}
	managedRoot, err := canonicalExistingDirectory(options.ManagedRoot)
	if err != nil {
		return Receipt{}, fmt.Errorf("resolve managed root: %w", err)
	}
	dataRoot, err := canonicalDestination(options.DataRoot)
	if err != nil {
		return Receipt{}, fmt.Errorf("resolve data root: %w", err)
	}
	if sameOrNested(managedRoot, dataRoot) || sameOrNested(dataRoot, managedRoot) {
		return Receipt{}, errors.New("managed root and data root must be separate")
	}
	var manifest Manifest
	if err := readStrictJSON(filepath.Join(managedRoot, ManifestFileName), &manifest); err != nil {
		return Receipt{}, fmt.Errorf("read portable activation manifest: %w", err)
	}
	if err := validateManifest(manifest); err != nil {
		return Receipt{}, err
	}
	if options.ExpectedVersion != "" && options.ExpectedVersion != "0.0.0-dev" && options.ExpectedVersion != manifest.Version {
		return Receipt{}, errors.New("installed CLI version does not match the portable activation manifest")
	}
	cliPath := filepath.Join(managedRoot, filepath.FromSlash(manifest.CLIPath))
	if err := rejectSymlinkComponents(managedRoot, filepath.FromSlash(manifest.CLIPath)); err != nil {
		return Receipt{}, err
	}
	digest, err := digestRegular(cliPath)
	if err != nil {
		return Receipt{}, err
	}
	if digest != manifest.CLISHA256 {
		return Receipt{}, errors.New("installed CLI digest does not match the portable activation manifest")
	}
	var active state
	if err := readStrictJSON(filepath.Join(dataRoot, StateRelativePath), &active); err != nil {
		return Receipt{}, fmt.Errorf("read portable activation state: %w", err)
	}
	desired := state{
		SchemaVersion: 1, Version: manifest.Version, TargetOS: manifest.TargetOS,
		TargetArch: manifest.TargetArch, CLISHA256: digest, ManagedRoot: managedRoot,
		DataRoot: dataRoot, CLIPath: cliPath,
	}
	if !sameActivation(active, desired) {
		return Receipt{}, errors.New("installed CLI and private activation state do not agree")
	}
	return receipt("ready", manifest, digest), nil
}

func validateReplacement(existing, desired state) error {
	if existing.SchemaVersion != 1 || existing.DataRoot != desired.DataRoot ||
		existing.TargetOS != desired.TargetOS || existing.TargetArch != desired.TargetArch ||
		!versionPattern.MatchString(existing.Version) || !digestPattern.MatchString(existing.CLISHA256) ||
		!filepath.IsAbs(existing.ManagedRoot) || !filepath.IsAbs(existing.CLIPath) ||
		!sameOrNested(existing.ManagedRoot, existing.CLIPath) {
		return errors.New("an incompatible Maestro activation already exists; nothing was overwritten or lost")
	}
	if existing.Version == desired.Version && existing.CLISHA256 != desired.CLISHA256 {
		return errors.New("the same Maestro version has different CLI bytes; nothing was overwritten or lost")
	}
	return nil
}

func validateManifest(manifest Manifest) error {
	if manifest.SchemaVersion != 1 || !versionPattern.MatchString(manifest.Version) || !digestPattern.MatchString(manifest.CLISHA256) {
		return errors.New("portable activation manifest is invalid")
	}
	if manifest.TargetOS != runtime.GOOS || manifest.TargetArch != runtime.GOARCH {
		return fmt.Errorf("portable target %s/%s does not match this host %s/%s; nothing was installed", manifest.TargetOS, manifest.TargetArch, runtime.GOOS, runtime.GOARCH)
	}
	wantCLI := "bin/bcgos"
	if runtime.GOOS == "windows" {
		wantCLI += ".exe"
	}
	if filepath.ToSlash(filepath.Clean(filepath.FromSlash(manifest.CLIPath))) != wantCLI || strings.Contains(manifest.CLIPath, "..") {
		return errors.New("portable activation manifest has an invalid CLI path")
	}
	return nil
}

func receipt(stateName string, manifest Manifest, digest string) Receipt {
	return Receipt{SchemaVersion: 1, State: stateName, Version: manifest.Version, TargetOS: manifest.TargetOS, TargetArch: manifest.TargetArch, CLISHA256: digest}
}

func sameActivation(existing, desired state) bool {
	return existing.SchemaVersion == 1 && existing.Version == desired.Version &&
		existing.TargetOS == desired.TargetOS && existing.TargetArch == desired.TargetArch &&
		existing.CLISHA256 == desired.CLISHA256 && existing.ManagedRoot == desired.ManagedRoot &&
		existing.DataRoot == desired.DataRoot && existing.CLIPath == desired.CLIPath
}

func canonicalExistingDirectory(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", errors.New("path is required")
	}
	absolute, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return "", err
	}
	info, err := os.Lstat(absolute)
	if err != nil {
		return "", err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return "", errors.New("path must be a real directory")
	}
	return filepath.EvalSymlinks(absolute)
}

func canonicalDestination(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", errors.New("path is required")
	}
	absolute, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return "", err
	}
	for current := absolute; ; current = filepath.Dir(current) {
		info, statErr := os.Lstat(current)
		if statErr == nil {
			if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
				return "", errors.New("destination parent must be a real directory")
			}
			real, evalErr := filepath.EvalSymlinks(current)
			if evalErr != nil {
				return "", evalErr
			}
			relative, relErr := filepath.Rel(current, absolute)
			if relErr != nil {
				return "", relErr
			}
			return filepath.Join(real, relative), nil
		}
		if !errors.Is(statErr, os.ErrNotExist) {
			return "", statErr
		}
		if filepath.Dir(current) == current {
			return "", errors.New("destination has no existing parent")
		}
	}
}

func sameOrNested(root, path string) bool {
	relative, err := filepath.Rel(root, path)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func rejectSymlinkComponents(root, relative string) error {
	current := root
	for _, component := range strings.Split(filepath.Clean(relative), string(filepath.Separator)) {
		if component == "" || component == "." {
			continue
		}
		current = filepath.Join(current, component)
		info, err := os.Lstat(current)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return errors.New("portable CLI path crosses a symlink; nothing was installed or lost")
		}
	}
	return nil
}

func digestRegular(path string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Size() > maximumCLIBytes {
		return "", errors.New("portable CLI must be a bounded regular non-symlink file")
	}
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, io.LimitReader(file, maximumCLIBytes+1)); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func readStrictJSON(path string, target any) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Size() > 64<<10 {
		return errors.New("JSON authority must be a bounded regular non-symlink file")
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	decoder := json.NewDecoder(io.LimitReader(file, 64<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("JSON contains trailing content")
	}
	return nil
}

func writeJSONAtomic(path string, value any) error {
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return errors.New("refusing to replace a non-regular activation authority")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".maestro-activation-*.tmp")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(body); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, path)
}
