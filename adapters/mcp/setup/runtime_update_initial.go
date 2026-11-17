package setup

import (
	"context"
	"errors"
	"path/filepath"
	"runtime"
	"time"

	product "github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ingest/docling"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installer"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installtx"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releaseverify"
)

// First setup has no enrolled source yet. Its already-confirmed installation
// review owns placement; the actual activated consumer performs qualification.
// These fixed child arguments bind that same signed activation and owner.
func placeInitialRuntime(ctx context.Context, binding product.Options, c Config, state installtx.State, verified releaseverify.VerifiedRelease) error {
	if state.SchemaVersion != 4 {
		return errors.New("current installed consumer has no authenticated runtime-update binding; converter setup pending; use a compatible source-aware release; prior metadata preserved")
	}
	if state.Binding.ManifestSHA256 != verified.ManifestSHA256 || state.Binding.ClientInputsSHA256 != verified.Manifest.Delivery.ClientInputsSHA256 {
		return errors.New("initial converter activation binding differs")
	}
	name := "maestro-mcp"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	executable := filepath.Join(binding.ProductRoot, "bin", name)
	args := []string{"runtime-review", "--product-root", binding.ProductRoot, "--user-home", binding.UserHome, "--workspace", binding.WorkspacePath, "--installed-manifest", state.Binding.ManifestSHA256, "--installed-client-inputs", state.Binding.ClientInputsSHA256, "--release-directory", verified.Directory}
	args = installer.ExperimentalChildArgs(args, binding.ExperimentalSeed, binding.ExperimentalSeedRoot)
	run := func(args []string) ([]byte, error) {
		if c.Run == nil {
			return runRuntimeConsumer(ctx, executable, args, binding.WorkspacePath)
		}
		bounded, cancel := context.WithTimeout(ctx, 3*time.Minute)
		defer cancel()
		return c.Run(bounded, executable, args...)
	}
	body, err := run(args)
	if err != nil {
		return err
	}
	var review product.RuntimeUpdateReview
	if err = releasecontract.DecodeStrictJSON(body, &review); err != nil {
		return err
	}
	required, err := docling.RequirementForRelease(verified.Manifest, verified.ManifestSHA256)
	if err != nil || review.Requirement != required || review.ServerSHA256 != state.Server.SHA256 || review.StateSHA256 != installtx.StateDigest(state) {
		return errors.New("initial converter consumer review differs from confirmed activation")
	}
	args[0] = "runtime-apply"
	args = append(args, "--review-digest", review.Digest, "--confirm")
	body, err = run(args)
	if err != nil {
		return err
	}
	var result docling.Availability
	if err = releasecontract.DecodeStrictJSON(body, &result); err != nil {
		return err
	}
	if result.State != "ready" || result.PackSHA256 != required.PackSHA256 {
		return errors.New("initial converter remains unavailable after current-consumer placement")
	}
	return nil
}
