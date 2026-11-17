package setup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	product "github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ingest/docling"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installer"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installtx"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
)

func runtimeUpdateLaunch(ctx context.Context, o installer.ClientOptions, command string) (installer.QualifiedLaunch, error) {
	if command != "runtime-review" && command != "runtime-apply" {
		return installer.QualifiedLaunch{}, errors.New("unsupported runtime command")
	}
	// Preserve the whole seeded composition. Future typed admission context and
	// child argv remain the responsibility of the existing qualified launcher.
	review, err := installer.ReviewClientsSeeded(ctx, o)
	if err != nil {
		return installer.QualifiedLaunch{}, err
	}
	launch, err := installer.PrepareQualifiedLaunch(ctx, installer.QualifiedLaunchOptions{SeededOperationOptions: o.SeededOperationOptions, Workspace: o.Request.Workspace, ExpectedBinding: installtx.ReleaseBinding{ManifestSHA256: review.Current.ManifestSHA256, ClientInputsSHA256: review.Current.ClientInputsSHA256}})
	if err != nil {
		return launch, err
	}
	if len(launch.Args) == 0 || launch.Args[0] != "serve" {
		return launch, errors.New("qualified runtime launcher command differs")
	}
	launch.Args = append([]string(nil), launch.Args...)
	launch.Args[0] = command
	return launch, nil
}

func ReviewRuntimeUpdate(ctx context.Context, o installer.ClientOptions) (product.RuntimeUpdateReview, error) {
	launch, err := runtimeUpdateLaunch(ctx, o, "runtime-review")
	if err != nil {
		return product.RuntimeUpdateReview{}, err
	}
	manifest, err := releasecontract.Parse(strings.NewReader(launch.State.Binding.ManifestJSON))
	if err != nil {
		return product.RuntimeUpdateReview{}, err
	}
	required, err := docling.RequirementForRelease(manifest, launch.State.Binding.ManifestSHA256)
	if err != nil {
		return product.RuntimeUpdateReview{}, err
	}
	// A release with no converter needs no new worker ABI. This preserves older
	// admitted packet management without falsely claiming converter availability.
	if required.PackSHA256 == "" {
		review := product.RuntimeUpdateReview{SchemaVersion: 1, State: "not_declared", Requirement: required, ServerSHA256: launch.State.Server.SHA256, StateSHA256: installtx.StateDigest(launch.State), Facts: docling.PlacementFacts{Requirement: required}, Availability: docling.Availability{State: "unavailable", Reason: "current_pack_not_declared"}, Effects: []string{}}
		body, err := releasecontract.CanonicalJSON(review)
		if err != nil {
			return review, err
		}
		sum := sha256.Sum256(body)
		review.Digest = hex.EncodeToString(sum[:])
		return review, nil
	}
	body, err := clientCommand(ctx, launch.Executable, launch.Args, o.Request.Workspace)
	if err != nil {
		return product.RuntimeUpdateReview{}, fmt.Errorf("current converter review unavailable; retry manage reconcile: %w", err)
	}
	var review product.RuntimeUpdateReview
	if err = releasecontract.DecodeStrictJSON(body, &review); err != nil {
		return review, err
	}
	if review.Requirement.ManifestSHA256 != launch.State.Binding.ManifestSHA256 || review.Requirement.ServerVersion != launch.State.Server.Version || review.ServerSHA256 != launch.State.Server.SHA256 {
		return review, errors.New("current runtime review identity changed")
	}
	return review, nil
}
func ApplyRuntimeUpdate(ctx context.Context, o installer.ClientOptions, digest string, confirmed bool) (docling.Availability, error) {
	pending := docling.Availability{State: "unavailable", Reason: "runtime_pending"}
	if !confirmed || len(digest) != 64 {
		return pending, errors.New("exact runtime review and explicit confirmation required")
	}
	review, err := ReviewRuntimeUpdate(ctx, o)
	if err != nil {
		return pending, err
	}
	if review.Digest != digest {
		return pending, errors.New("current runtime review changed")
	}
	if review.Requirement.PackSHA256 == "" && len(review.Effects) == 0 {
		return review.Availability, nil
	}
	launch, err := runtimeUpdateLaunch(ctx, o, "runtime-apply")
	if err != nil {
		return pending, err
	}
	launch.Args = append(launch.Args, "--review-digest", digest, "--confirm")
	body, err := runRuntimeConsumer(ctx, launch.Executable, launch.Args, o.Request.Workspace)
	var result docling.Availability
	if decodeErr := releasecontract.DecodeStrictJSON(body, &result); decodeErr != nil {
		if err != nil {
			return pending, err
		}
		return pending, decodeErr
	}
	if err != nil {
		return result, fmt.Errorf("source-updated/runtime-pending; retry manage reconcile: %w", err)
	}
	if review.Requirement.PackSHA256 != "" && (result.State != "ready" || result.PackSHA256 != review.Requirement.PackSHA256) {
		return result, errors.New("source-updated/runtime-pending; current converter not ready; retry manage reconcile")
	}
	return result, nil
}
func runRuntimeConsumer(ctx context.Context, path string, args []string, cwd string) ([]byte, error) {
	bounded, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	command := exec.CommandContext(bounded, path, args...)
	command.Dir = cwd
	command.WaitDelay = 2 * time.Second
	var stdout, stderr boundedClientOutput
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()
	return stdout.Bytes(), err
}
