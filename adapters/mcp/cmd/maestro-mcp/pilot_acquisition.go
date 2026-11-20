package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	product "github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/datahome"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ingest/docling/acquisition"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releaseverify"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspace"
	"time"
)

type pilotAcquisitionReview struct {
	ProductRoot      string           `json:"product_root"`
	UserHome         string           `json:"user_home"`
	Workspace        string           `json:"workspace"`
	WorkspaceID      string           `json:"workspace_id"`
	ReleaseDirectory string           `json:"release_directory"`
	CacheRoot        string           `json:"cache_root"`
	Plan             acquisition.Plan `json:"plan"`
	Effects          []string         `json:"effects"`
	SHA256           string           `json:"review_sha256"`
}

func acquisitionFrame(o product.Options, server, directory string) (pilotAcquisitionReview, acquisition.Options, error) {
	var frame pilotAcquisitionReview
	var opts acquisition.Options
	root := filepath.Dir(o.ProductRoot)
	if o.ProductRoot != filepath.Join(root, "product") || o.UserHome != filepath.Join(root, "owner") || o.WorkspacePath != filepath.Join(root, "workspace") || server != filepath.Join(o.ProductRoot, "server", "maestro-mcp.exe") || o.CLIVersion == "" || o.AuthorityRegistrySHA256 == "" || o.AuthorityRegistryPath != filepath.Join(o.ProductRoot, "managed", "trust", "release-authority-registry.json") || o.InstalledManifestSHA256 != "" || o.InstalledClientInputsSHA256 != "" {
		return frame, opts, errors.New("isolated engineering acquisition layout required")
	}
	for _, path := range []string{o.ProductRoot, o.UserHome, o.WorkspacePath, server, directory} {
		if !filepath.IsAbs(path) || filepath.Clean(path) != path || strings.ContainsAny(path, "\x00\r\n") {
			return frame, opts, errors.New("exact acquisition paths required")
		}
	}
	if filepath.Base(directory) != "runtime-release" {
		return frame, opts, errors.New("closed acquisition carrier required")
	}
	if _, err := os.Lstat(filepath.Join(o.UserHome, ".maestro-install")); !errors.Is(err, os.ErrNotExist) {
		return frame, opts, errors.New("enrolled installation preserved")
	}
	if _, err := product.Open(o); err != nil {
		return frame, opts, err
	}
	home, err := datahome.Resolve(datahome.Options{ProductRoot: o.ProductRoot, UserHome: o.UserHome, LookupEnv: o.LookupEnv})
	if err != nil || !home.Writable {
		return frame, opts, errors.New("private acquisition home unavailable")
	}
	inspection, err := workspace.Inspect(o.WorkspacePath, filepath.Join(home.Home, "state"))
	if err != nil || inspection.State != "ready" {
		return frame, opts, errors.New("acquisition workspace unavailable")
	}
	cache := filepath.Join(home.Home, "state", "runtime-acquisition")
	frame = pilotAcquisitionReview{ProductRoot: o.ProductRoot, UserHome: o.UserHome, Workspace: o.WorkspacePath, WorkspaceID: inspection.WorkspaceID, ReleaseDirectory: directory, CacheRoot: cache, Effects: []string{"Download only the listed authenticated public inputs from their declared upstream hosts; no credentials, models, services or runtime execution.", "Retain verified cache and at most three bounded attempts, including failed/partial bytes; maximum retained plus worst additional bytes is 2 GiB. The review expires after at most 30 minutes.", "This acquires and assembles a new private complete release only. A separate complete runtime review and confirmation are required before placement and Windows sandbox qualification. Existing private work is preserved; no production installation or update is authorized."}}
	opts = acquisition.Options{ReleaseDirectory: directory, RecipeDirectory: filepath.Join(filepath.Dir(directory), "runtime-recipe"), ServerPath: server, RegistryPath: o.AuthorityRegistryPath, RegistrySHA256: o.AuthorityRegistrySHA256, CacheRoot: cache}
	return frame, opts, nil
}
func acquisitionReviewDigest(r pilotAcquisitionReview) string {
	r.SHA256 = ""
	b, _ := json.Marshal(r)
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}
func reviewPilotAcquisition(ctx context.Context, o product.Options, server, directory string) (pilotAcquisitionReview, error) {
	frame, opts, err := acquisitionFrame(o, server, directory)
	if err != nil {
		return frame, err
	}
	frame.Plan, err = acquisition.Review(ctx, opts)
	if err != nil {
		return frame, err
	}
	frame.SHA256 = acquisitionReviewDigest(frame)
	return frame, nil
}
func readPilotAcquisitionReview(path string) (pilotAcquisitionReview, error) {
	var r pilotAcquisitionReview
	f, err := scheduler.OpenRegularFile(path)
	if err != nil {
		return r, err
	}
	defer f.Close()
	if err = scheduler.ValidateSingleLink(f); err != nil {
		return r, err
	}
	b, err := io.ReadAll(io.LimitReader(f, (1<<20)+1))
	if err != nil || len(b) > 1<<20 {
		return r, errors.New("bounded acquisition review required")
	}
	if err := releasecontract.DecodeStrictJSON(b, &r); err != nil {
		return r, err
	}
	return r, nil
}
func applyPilotAcquisition(ctx context.Context, o product.Options, server, directory, path, digest string, confirmed bool) (any, error) {
	if !confirmed {
		return nil, errors.New("conscious local acquisition confirmation required")
	}
	r, err := readPilotAcquisitionReview(path)
	if err != nil {
		return nil, err
	}
	if digest == "" || r.SHA256 != digest || acquisitionReviewDigest(r) != digest {
		return nil, errors.New("acquisition review differs")
	}
	frame, opts, err := acquisitionFrame(o, server, directory)
	if err != nil {
		return nil, err
	}
	frame.Plan = r.Plan
	frame.SHA256 = acquisitionReviewDigest(frame)
	if frame.SHA256 != digest {
		return nil, errors.New("acquisition destination or workspace changed")
	}
	if runtime.GOOS != "windows" || runtime.GOARCH != "amd64" {
		return nil, errors.New("native Windows acquisition required")
	}
	verified, err := acquisition.Acquire(ctx, opts, r.Plan)
	if err != nil {
		return nil, err
	}
	registry, err := releaseverify.LoadPinnedAuthorityRegistry(o.AuthorityRegistryPath, o.AuthorityRegistrySHA256, time.Now)
	if err != nil {
		return nil, err
	}
	complete, err := releaseverify.VerifyDirectory(verified.Directory, registry)
	if err != nil || complete.ManifestSHA256 != verified.ManifestSHA256 {
		return nil, errors.New("acquired whole release unavailable")
	}
	return map[string]string{"state": "acquired", "release_directory": complete.Directory, "manifest_sha256": complete.ManifestSHA256, "next": "Read the separate pilot-runtime-review before any placement."}, nil
}
