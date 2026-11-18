package setup

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp/packaging"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installer"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installtx"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

// NativeClientSelection selects only inputs to the existing real qualifier.
// No imported diagnostics, qualification issuer or proof override is accepted.
type NativeClientSelection struct {
	ScratchRoot        string  `json:"scratch_root"`
	ChallengeDirectory string  `json:"challenge_directory"`
	MaxBudgetUSD       float64 `json:"max_budget_usd_per_call"`
}

func nativeClientCommand(o installer.ClientOptions, s installtx.State, n NativeClientSelection, command string) (string, []string, error) {
	if !o.Request.VersionedSlots || !o.Request.NativeEnforcement || o.Request.LifecycleContext || o.Request.CodeExecutable == "" {
		return "", nil, errors.New("full native client selection required")
	}
	paths, err := installer.ClientGenerationPaths(o.Request, s.Binding.ManifestSHA256)
	if err != nil {
		return "", nil, err
	}
	name := "maestro-mcp"
	if s.TargetOS == "windows" {
		name += ".exe"
	}
	server := filepath.Join(o.ManagedRoot, "bin", name)
	destination := paths.CodeRoot + ".native-plugin"
	args := currentGeneratorArgs(command, destination, server, o, s)
	args = append(args, "--native-executable", o.Request.CodeExecutable, "--native-scratch-root", n.ScratchRoot, "--native-challenge-directory", n.ChallengeDirectory, "--max-budget-usd", strconv.FormatFloat(n.MaxBudgetUSD, 'f', -1, 64))
	return server, args, nil
}

func ReviewNativeClientUpdate(ctx context.Context, o installer.ClientOptions, n NativeClientSelection) (packaging.NativeReview, error) {
	s, err := installtx.ValidateQualifiedLaunch(o.ManagedRoot, o.DataRoot, installtx.ReleaseBinding{}, time.Now())
	if err != nil {
		return packaging.NativeReview{}, err
	}
	server, args, err := nativeClientCommand(o, s, n, "native-review")
	if err != nil {
		return packaging.NativeReview{}, err
	}
	before, err := installer.ClientFileDigest(server)
	if err != nil || before != s.Server.SHA256 {
		return packaging.NativeReview{}, errors.New("current native qualifier identity changed")
	}
	body, err := clientCommandWithProfile(o.Request.CodeConfigurationDirectory)(ctx, server, args, o.Request.Workspace)
	if err != nil {
		return packaging.NativeReview{}, err
	}
	after, err := installer.ClientFileDigest(server)
	if err != nil || after != before {
		return packaging.NativeReview{}, errors.New("current native qualifier identity changed")
	}
	var review packaging.NativeReview
	err = releasecontract.DecodeStrictJSON(body, &review)
	return review, err
}

// ApplyNativeClientUpdate runs the existing producer outside installation
// serialization. Only its same-call native_observed result plus the unchanged
// canonical candidate can enter the normal guarded current-record publisher.
// It never treats old proof as qualification for the new executable/profile.
func ApplyNativeClientUpdate(ctx context.Context, o installer.ClientOptions, n NativeClientSelection, clientDigest, nativeDigest string, confirmed bool) (installer.ClientRecord, error) {
	if !confirmed {
		return installer.ClientRecord{}, errors.New("explicit full client/native review confirmation required")
	}
	reviewed, err := installer.ReviewClientsSeeded(ctx, o)
	if err != nil || reviewed.Digest != clientDigest {
		return installer.ClientRecord{}, errors.New("client review changed before native qualification")
	}
	native, err := ReviewNativeClientUpdate(ctx, o, n)
	if err != nil {
		return installer.ClientRecord{}, err
	}
	if native.ReviewDigest != nativeDigest || native.Plan.State != "reviewable" {
		return installer.ClientRecord{}, errors.New("current genuine native qualification is unavailable or review changed")
	}
	state, err := installtx.ValidateQualifiedLaunch(o.ManagedRoot, o.DataRoot, installtx.ReleaseBinding{ManifestSHA256: reviewed.Current.ManifestSHA256, ClientInputsSHA256: reviewed.Current.ClientInputsSHA256}, time.Now())
	if err != nil {
		return installer.ClientRecord{}, err
	}
	server, args, err := nativeClientCommand(o, state, n, "native-qualify-and-build")
	if err != nil {
		return installer.ClientRecord{}, err
	}
	args = append(args, "--review-digest", nativeDigest, "--confirm")
	// Three 120s sessions plus bounded startup/inspection. No installer lock is
	// held while model calls or qualification hooks execute.
	bounded, cancel := context.WithTimeout(ctx, 8*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(bounded, server, args...)
	cmd.Dir = o.Request.Workspace
	cmd.Env = clientCodeEnvironment(o.Request.CodeConfigurationDirectory)
	cmd.WaitDelay = 2 * time.Second
	var output boundedClientOutput
	cmd.Stdout = &output
	cmd.Stderr = &boundedClientOutput{}
	if err := cmd.Run(); err != nil {
		return installer.ClientRecord{}, fmt.Errorf("genuine native qualification remains pending; previous packets retained: %w", err)
	}
	var result packaging.NativeResult
	if err := releasecontract.DecodeStrictJSON(output.Bytes(), &result); err != nil {
		return installer.ClientRecord{}, err
	}
	if result.State != "qualified" || result.EvidenceClass != "native_observed" || result.ReviewDigest != nativeDigest {
		return installer.ClientRecord{}, errors.New("genuine native qualification remains pending; no MCP-only fallback")
	}
	hash, err := installer.ClientFileDigest(server)
	if err != nil || hash != state.Server.SHA256 {
		return installer.ClientRecord{}, errors.New("native qualifier executable changed")
	}
	paths, err := installer.ClientGenerationPaths(o.Request, state.Binding.ManifestSHA256)
	if err != nil {
		return installer.ClientRecord{}, err
	}
	qualified := paths.CodeRoot + ".native-plugin"
	identity, err := releasecontract.ReadServerIdentity(server)
	if err != nil {
		return installer.ClientRecord{}, err
	}
	if _, err := inspectCurrentPacket(ctx, qualified, "plugin", o, identity, state.Server.SHA256); err != nil {
		return installer.ClientRecord{}, err
	}
	nativeSHA, err := installer.ClientTreeDigest(qualified)
	if err != nil {
		return installer.ClientRecord{}, err
	}
	stage := paths.CodeRoot + ".candidate-" + o.Request.CarrierID + "-" + state.Binding.ManifestSHA256
	if err := os.Mkdir(stage, 0700); err != nil {
		return installer.ClientRecord{}, fmt.Errorf("native staging slot occupied; preserve and reconcile: %w", err)
	}
	if err := os.Mkdir(filepath.Join(stage, "plugins"), 0700); err != nil {
		return installer.ClientRecord{}, err
	}
	if err := copyQualifiedClientTree(qualified, filepath.Join(stage, "plugins", "maestro-product"), nativeSHA); err != nil {
		return installer.ClientRecord{}, err
	}
	generation := o
	generation.VersionedManagement = true
	generation.Request.CodeRoot = paths.CodeRoot
	generation.Request.DesktopPath = paths.DesktopPath
	generation.Request.VersionedSlots = false
	if err := writeMarketplaceAndLauncher(stage, generation); err != nil {
		return installer.ClientRecord{}, err
	}
	return installer.MaterializeClientsSeeded(ctx, o, clientDigest, true, currentClientMaterializer(o, nativeSHA, nil))
}

func copyQualifiedClientTree(source, destination, expected string) error {
	before, err := installer.ClientTreeDigest(source)
	if err != nil || before != expected {
		return errors.New("qualified native source changed")
	}
	err = filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return errors.New("native packet alias")
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, rel)
		if entry.IsDir() {
			return os.Mkdir(target, info.Mode().Perm())
		}
		in, err := scheduler.OpenRegularFile(path)
		if err != nil {
			return err
		}
		defer in.Close()
		if err := scheduler.ValidateSingleLink(in); err != nil {
			return err
		}
		out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, info.Mode().Perm())
		if err != nil {
			return err
		}
		count, copyErr := io.Copy(out, io.LimitReader(in, (128<<20)+1))
		modeErr := out.Chmod(info.Mode().Perm())
		syncErr := out.Sync()
		closeErr := out.Close()
		if count != info.Size() || count > 128<<20 {
			return errors.New("qualified native file changed or exceeds bound")
		}
		return errors.Join(copyErr, modeErr, syncErr, closeErr)
	})
	if err != nil {
		return err
	}
	after, err := installer.ClientTreeDigest(source)
	if err != nil || after != expected {
		return errors.New("qualified native source changed during copy")
	}
	actual, err := installer.ClientTreeDigest(destination)
	if err != nil || actual != expected {
		return errors.New("qualified native candidate differs after copy")
	}
	return nil
}
