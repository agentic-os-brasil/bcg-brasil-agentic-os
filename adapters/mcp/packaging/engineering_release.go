package packaging

import (
	"bytes"
	"context"
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

	product "github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/datahome"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ingest/docling"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releaseverify"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspace"
)

const engineeringRuntimeRecord = "managed/trust/engineering-runtime-pilot.json"

type engineeringRuntimePlacement struct {
	Binding              EngineeringRuntimeBinding `json:"binding"`
	ApprovedReviewSHA256 string                    `json:"approved_review_sha256"`
}

// EngineeringRuntimeReview is a local operator presentation, never an MCP
// mutation or a transferable installation/production authority.
type EngineeringRuntimeReview struct {
	Profile                string                    `json:"profile"`
	ProductRoot            string                    `json:"product_root"`
	UserHome               string                    `json:"user_home"`
	Workspace              string                    `json:"workspace"`
	WorkspaceID            string                    `json:"workspace_id"`
	ReleaseDirectory       string                    `json:"release_directory"`
	DataRoot               string                    `json:"data_root"`
	CurrentManifestSHA256  string                    `json:"current_manifest_sha256"`
	CurrentPlacementSHA256 string                    `json:"current_placement_sha256"`
	Runtime                EngineeringRuntimeBinding `json:"runtime"`
	Effects                []string                  `json:"effects"`
	ReviewSHA256           string                    `json:"review_sha256"`
}

type EngineeringRuntimeResult struct {
	State        string                    `json:"state"`
	AlreadyReady bool                      `json:"already_ready"`
	Runtime      EngineeringRuntimeBinding `json:"runtime"`
}

func readEngineeringFile(path string, limit int64) ([]byte, error) {
	f, err := scheduler.OpenRegularFile(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if err := scheduler.ValidateSingleLink(f); err != nil {
		return nil, err
	}
	body, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil || int64(len(body)) > limit {
		return nil, errors.New("engineering runtime input exceeds its bound")
	}
	return body, nil
}

func engineeringLayout(o product.Options, server string) (docling.Options, error) {
	root := filepath.Dir(o.ProductRoot)
	if o.ProductRoot != filepath.Join(root, "product") || o.UserHome != filepath.Join(root, "owner") || o.WorkspacePath != filepath.Join(root, "workspace") || o.InstalledManifestSHA256 != "" || o.InstalledClientInputsSHA256 != "" {
		return docling.Options{}, errors.New("engineering runtime requires the prepared isolated layout")
	}
	for _, path := range []string{o.ProductRoot, o.UserHome, o.WorkspacePath, server} {
		if !filepath.IsAbs(path) || filepath.Clean(path) != path || strings.ContainsAny(path, "\x00\r\n") {
			return docling.Options{}, errors.New("engineering runtime requires exact absolute paths")
		}
	}
	if server != filepath.Join(o.ProductRoot, "server", binaryName(Target{OS: runtime.GOOS}.Platform())) {
		return docling.Options{}, errors.New("engineering runtime requires its prepared server")
	}
	if _, err := os.Lstat(filepath.Join(o.UserHome, ".maestro-install")); !errors.Is(err, os.ErrNotExist) {
		return docling.Options{}, errors.New("engineering runtime cannot use an enrolled installation")
	}
	if o.AuthorityRegistryPath != filepath.Join(o.ProductRoot, "managed", "trust", "release-authority-registry.json") || o.AuthorityRegistrySHA256 == "" || o.CLIVersion == "" {
		return docling.Options{}, errors.New("engineering runtime requires the native constructor's public authority seed")
	}
	if _, err := product.Open(o); err != nil {
		return docling.Options{}, err
	}
	home, err := datahome.Resolve(datahome.Options{ProductRoot: o.ProductRoot, UserHome: o.UserHome, LookupEnv: o.LookupEnv})
	if err != nil || !home.Writable {
		return docling.Options{}, errors.New("engineering runtime home unavailable")
	}
	return docling.Options{DataRoot: filepath.Join(home.Home, "state"), AuthorityRegistryPath: o.AuthorityRegistryPath, AuthorityRegistrySHA256: o.AuthorityRegistrySHA256, CLIVersion: o.CLIVersion}, nil
}

func readEngineeringPlacement(productRoot string) (engineeringRuntimePlacement, []byte, error) {
	body, err := readEngineeringFile(filepath.Join(productRoot, filepath.FromSlash(engineeringRuntimeRecord)), 4096)
	if err != nil {
		return engineeringRuntimePlacement{}, nil, err
	}
	if err := packetJSON(body); err != nil {
		return engineeringRuntimePlacement{}, nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var record engineeringRuntimePlacement
	if err := decoder.Decode(&record); err != nil {
		return record, nil, err
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return record, nil, errors.New("engineering runtime record has trailing input")
	}
	if err := record.Binding.validate(); err != nil || !regexp.MustCompile(`^[a-f0-9]{64}$`).MatchString(record.ApprovedReviewSHA256) {
		return record, nil, errors.New("engineering runtime placement record unavailable; existing bytes preserved")
	}
	return record, body, nil
}

func engineeringReleaseBinding(o product.Options, server, directory string) (EngineeringRuntimeBinding, releaseverify.VerifiedRelease, docling.Options, error) {
	if !filepath.IsAbs(directory) || filepath.Clean(directory) != directory || strings.ContainsAny(directory, "\x00\r\n") {
		return EngineeringRuntimeBinding{}, releaseverify.VerifiedRelease{}, docling.Options{}, errors.New("engineering runtime requires an exact absolute release directory")
	}
	runtimeOptions, err := engineeringLayout(o, server)
	if err != nil {
		return EngineeringRuntimeBinding{}, releaseverify.VerifiedRelease{}, runtimeOptions, err
	}
	registryBody, err := readEngineeringFile(o.AuthorityRegistryPath, 1<<20)
	if err != nil {
		return EngineeringRuntimeBinding{}, releaseverify.VerifiedRelease{}, runtimeOptions, err
	}
	registry, err := releaseverify.LoadPinnedAuthorityRegistry(o.AuthorityRegistryPath, o.AuthorityRegistrySHA256, time.Now)
	if err != nil {
		return EngineeringRuntimeBinding{}, releaseverify.VerifiedRelease{}, runtimeOptions, err
	}
	verified, err := releaseverify.VerifyDirectory(directory, registry)
	if err != nil {
		return EngineeringRuntimeBinding{}, verified, runtimeOptions, err
	}
	m := verified.Manifest
	if !m.Modern() || m.Channel != "canary" || m.Server.Version != o.CLIVersion {
		return EngineeringRuntimeBinding{}, verified, runtimeOptions, errors.New("engineering runtime requires its compatible modern canary release")
	}
	expires, err := engineeringRuntimeAuthority(registryBody, o.AuthorityRegistrySHA256, m.Issuer.ID, m.Issuer.KeyID, time.Now())
	if err != nil {
		return EngineeringRuntimeBinding{}, verified, runtimeOptions, err
	}
	body, target, err := readNativeBinary(server)
	if err != nil || target != (Target{OS: runtime.GOOS, Arch: runtime.GOARCH}) {
		return EngineeringRuntimeBinding{}, verified, runtimeOptions, errors.New("engineering runtime server must match the actual target")
	}
	identity, err := releasecontract.ReadServerIdentity(server)
	if err != nil || identity.Version != o.CLIVersion || identity.AuthorityRegistrySHA256 != o.AuthorityRegistrySHA256 || identity.ExecutionContract != m.Server.ExecutionContract || identity.EmbeddedInputsSHA256 != m.Server.EmbeddedInputsSHA256 || identity.CompatibleBundle != m.Server.CompatibleBundle {
		return EngineeringRuntimeBinding{}, verified, runtimeOptions, errors.New("engineering runtime server identity differs from the signed release")
	}
	binding := EngineeringRuntimeBinding{Profile: EngineeringRuntimeProfile, ClientVersion: o.CLIVersion, RegistrySHA256: o.AuthorityRegistrySHA256, ManifestSHA256: verified.ManifestSHA256, ServerSHA256: digest(body), Target: target, Issuer: m.Issuer.ID, KeyID: m.Issuer.KeyID, ExpiresAt: expires.UTC().Format(time.RFC3339)}
	servers, packs := 0, 0
	for _, artifact := range m.Artifacts {
		if artifact.OS != target.OS || artifact.Arch != target.Arch {
			continue
		}
		if artifact.Kind == "mcp_server" {
			servers++
			if artifact.SHA256 != binding.ServerSHA256 {
				return binding, verified, runtimeOptions, errors.New("engineering runtime executing server differs from the signed target")
			}
		}
		if artifact.Kind == "runtime_pack" && artifact.Adapter == "docling" {
			packs++
			binding.PackSHA256 = artifact.SHA256
		}
	}
	if servers != 1 || packs != 1 {
		return binding, verified, runtimeOptions, errors.New("engineering runtime requires exactly one current-target server and converter")
	}
	return binding, verified, runtimeOptions, binding.validate()
}

func ReviewEngineeringRuntime(o product.Options, server, directory string) (EngineeringRuntimeReview, error) {
	binding, _, runtimeOptions, err := engineeringReleaseBinding(o, server, directory)
	if err != nil {
		return EngineeringRuntimeReview{}, err
	}
	review := EngineeringRuntimeReview{Profile: EngineeringRuntimeProfile, ProductRoot: o.ProductRoot, UserHome: o.UserHome, Workspace: o.WorkspacePath, ReleaseDirectory: directory, DataRoot: runtimeOptions.DataRoot, Runtime: binding, Effects: []string{"Place only this authenticated compatible runtime under the selected private owner state; preserve source documents, existing different metadata and private work.", "No downloads, models, service, login, production enrollment or update is authorized.", "Windows explicit placement runs synthetic conversion and per-user AppContainer/Job denial controls before sealing a local qualification proof; queries do not run these effects.", "This short-lived engineering issuer is not organizational publisher authority or native platform signing. Only the formats returned by content.status are admitted; upstream package presence is not product format support. Unsupported targets and formats remain unavailable."}}
	inspection, err := workspace.Inspect(o.WorkspacePath, runtimeOptions.DataRoot)
	if err != nil || inspection.State != "ready" {
		return review, errors.New("engineering runtime workspace unavailable")
	}
	review.WorkspaceID = inspection.WorkspaceID
	current, err := readEngineeringFile(filepath.Join(runtimeOptions.DataRoot, "ingestion", "docling", releaseverify.ManifestName), 1<<20)
	if err == nil {
		review.CurrentManifestSHA256 = digest(current)
		if review.CurrentManifestSHA256 != binding.ManifestSHA256 {
			return review, errors.New("different existing converter metadata preserved; prepare a fresh rehearsal")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return review, err
	}
	record, body, err := readEngineeringPlacement(o.ProductRoot)
	if err == nil {
		if record.Binding != binding {
			return review, errors.New("different engineering placement preserved; prepare a fresh rehearsal")
		}
		review.CurrentPlacementSHA256 = digest(body)
	} else if !errors.Is(err, os.ErrNotExist) {
		return review, err
	}
	body, err = json.Marshal(review)
	if err != nil {
		return review, err
	}
	review.ReviewSHA256 = digest(body)
	return review, nil
}

func ApplyEngineeringRuntime(ctx context.Context, o product.Options, server, directory, reviewedSHA256 string, confirmed bool) (EngineeringRuntimeResult, error) {
	review, err := ReviewEngineeringRuntime(o, server, directory)
	if err != nil {
		return EngineeringRuntimeResult{}, err
	}
	if !confirmed {
		return EngineeringRuntimeResult{}, errors.New("engineering runtime requires conscious local confirmation")
	}
	runtimeOptions, err := engineeringLayout(o, server)
	if err != nil {
		return EngineeringRuntimeResult{}, err
	}
	record, _, recordErr := readEngineeringPlacement(o.ProductRoot)
	if recordErr == nil && record.Binding == review.Runtime && (record.ApprovedReviewSHA256 == reviewedSHA256 || review.ReviewSHA256 == reviewedSHA256) {
		status := docling.New(runtimeOptions).Status(ctx)
		if status.State == "ready" && status.PackSHA256 == review.Runtime.PackSHA256 {
			return EngineeringRuntimeResult{State: "ready", AlreadyReady: true, Runtime: review.Runtime}, nil
		}
	}
	if reviewedSHA256 != review.ReviewSHA256 {
		return EngineeringRuntimeResult{}, errors.New("engineering runtime review changed; read the complete current review")
	}
	binding, verified, runtimeOptions, err := engineeringReleaseBinding(o, server, directory)
	if err != nil || binding != review.Runtime {
		return EngineeringRuntimeResult{}, errors.New("engineering runtime inputs changed before placement")
	}
	if err := docling.PlaceVerified(ctx, runtimeOptions, verified); err != nil {
		return EngineeringRuntimeResult{}, err
	}
	if errors.Is(recordErr, os.ErrNotExist) {
		body, err := json.MarshalIndent(engineeringRuntimePlacement{Binding: binding, ApprovedReviewSHA256: reviewedSHA256}, "", "  ")
		if err != nil {
			return EngineeringRuntimeResult{}, err
		}
		if err := scheduler.WriteNewPrivateFile(filepath.Join(o.ProductRoot, filepath.FromSlash(engineeringRuntimeRecord)), append(body, '\n')); err != nil {
			return EngineeringRuntimeResult{}, fmt.Errorf("runtime placed; preparation record preserved/pending: %w", err)
		}
	}
	return EngineeringRuntimeResult{State: "ready", Runtime: binding}, nil
}
