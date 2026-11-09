// Package setup composes canonical signed activation, genuine owner admission
// and participant-local packets. It owns no second transaction or home authority.
package setup

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	product "github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp/packaging"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/datahome"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ingest/docling"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installer"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installtx"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releaseverify"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspace"
)

type Request struct {
	ReleaseDirectory string `json:"release_directory"`
	ManagedRoot      string `json:"managed_root"`
	UserHome         string `json:"user_home"`
	Workspace        string `json:"workspace"`
	CodePacket       string `json:"code_packet"`
	DesktopPacket    string `json:"desktop_packet"`
}

// Config is process composition, never decoded from user/model JSON. The
// production command always uses strict native trust and its compiled seed.
type Config struct {
	Installer, AuthorityRegistry string
	Clock                        func() time.Time
	VerifyNative                 func(context.Context, string) error
	Run                          func(context.Context, string, ...string) ([]byte, error)
	LookupEnv                    func(string) (string, bool)
}
type Effect struct {
	Operation   string `json:"operation"`
	Destination string `json:"destination"`
}

type Review struct {
	SchemaVersion int             `json:"schema_version"`
	Request       Request         `json:"request"`
	Installation  installer.Plan  `json:"installation"`
	Home          datahome.Result `json:"home"`
	Effects       []Effect        `json:"effects"`
	Packets       []PacketReview  `json:"packets"`
	Digest        string          `json:"review_digest"`
}

type PacketReview struct {
	Destination string                  `json:"destination"`
	Format      string                  `json:"format"`
	Existing    *packaging.PacketStatus `json:"existing,omitempty"`
}
type Result struct {
	Installation     installer.Result     `json:"installation"`
	State            string               `json:"state"`
	Home             string               `json:"home,omitempty"`
	WorkspaceID      string               `json:"workspace_id,omitempty"`
	CodePacket       string               `json:"code_packet,omitempty"`
	DesktopPacket    string               `json:"desktop_packet,omitempty"`
	CodeRegistration string               `json:"code_registration"`
	DesktopLoading   string               `json:"desktop_loading"`
	Content          docling.Availability `json:"content"`
}

func installOptions(r Request, c Config) installer.Options {
	return installer.Options{UserHome: r.UserHome, ReleaseDir: r.ReleaseDirectory, Bootstrapper: c.Installer, AuthorityRegistry: c.AuthorityRegistry, ManagedRoot: r.ManagedRoot, DataRoot: filepath.Join(r.UserHome, ".maestro-install"), TargetOS: runtime.GOOS, TargetArch: runtime.GOARCH, Clock: c.Clock, VerifyNative: c.VerifyNative, Run: c.Run, NativeTrustMode: installer.NativeTrustStrict}
}

func ReviewInstallation(r Request, c Config) (Review, error) {
	plan, verified, err := installer.Prepare(installOptions(r, c))
	if err != nil {
		return Review{}, err
	}
	if !verified.Manifest.Modern() {
		return Review{}, errors.New("modern setup requires authenticated mcp-local-v1")
	}
	r.ManagedRoot = plan.ManagedRoot
	r.ReleaseDirectory = plan.ReleaseDir
	home, err := datahome.PreviewInstallation(datahome.Options{ProductRoot: r.ManagedRoot, UserHome: r.UserHome, LookupEnv: c.LookupEnv})
	if err != nil {
		return Review{}, err
	}
	if !home.Writable && home.Diagnostic != "fresh-unconfirmed" {
		return Review{}, errors.New("selected home is not admitted or fresh")
	}
	if overlap(plan.DataRoot, home.Home) {
		return Review{}, errors.New("private installation state overlaps admitted home")
	}
	if err = workspace.PreflightInitialize(workspace.Options{WorkspacePath: r.Workspace, DataRoot: filepath.Join(home.Home, "state")}); err != nil {
		return Review{}, err
	}
	for _, other := range []string{home.Home, r.ManagedRoot, plan.DataRoot, plan.ReleaseDir, c.Installer, c.AuthorityRegistry} {
		if overlap(other, r.Workspace) {
			return Review{}, errors.New("workspace overlaps installation/home")
		}
	}
	packets := []PacketReview{{Destination: r.CodePacket, Format: "plugin"}, {Destination: r.DesktopPacket, Format: "mcpb"}}
	for i := range packets {
		destination := packets[i].Destination
		if destination == "" || !filepath.IsAbs(destination) || filepath.Clean(destination) != destination || strings.ContainsAny(destination, "\x00\r\n") {
			return Review{}, errors.New("clean absolute packet destinations required")
		}
		if err = scheduler.ValidateDirectory(filepath.Dir(destination)); err != nil {
			return Review{}, err
		}
		for _, other := range []string{home.Home, r.ManagedRoot, plan.DataRoot, r.Workspace, plan.ReleaseDir, c.Installer, c.AuthorityRegistry} {
			if overlap(other, destination) {
				return Review{}, errors.New("packet overlaps private binding/installation")
			}
		}
		if _, err = os.Lstat(destination); err == nil {
			status, readErr := inspectPacket(packets[i], plan)
			if readErr != nil {
				return Review{}, fmt.Errorf("existing packet preserved; choose a new destination or inspect exact inventory: %w", readErr)
			}
			packets[i].Existing = &status
		} else if !errors.Is(err, os.ErrNotExist) {
			return Review{}, err
		}
	}
	if overlap(r.CodePacket, r.DesktopPacket) {
		return Review{}, errors.New("participant packets overlap")
	}
	review := Review{SchemaVersion: 1, Request: r, Installation: plan, Home: home, Packets: packets, Effects: []Effect{
		{"activate_authenticated_managed_payload", plan.ManagedRoot},
		{"write_private_installation_state", plan.DataRoot},
		{"admit_selected_owner_home", home.Home},
		{"publish_canonical_home_locator_if_required", filepath.Join(r.UserHome, ".maestro-home.json")},
		{"admit_selected_workspace", r.Workspace},
		{"initialize_owner_and_memory", home.Home},
	}}
	for _, packet := range packets {
		operation := "materialize_" + packet.Format + "_packet"
		if packet.Existing != nil {
			operation = "preserve_verified_" + packet.Format + "_packet"
		}
		review.Effects = append(review.Effects, Effect{operation, packet.Destination})
	}
	for _, artifact := range verified.Manifest.Artifacts {
		if artifact.Kind == "runtime_pack" && artifact.OS == runtime.GOOS && artifact.Arch == runtime.GOARCH {
			review.Effects = append(review.Effects, Effect{"place_authenticated_docling_pack", filepath.Join(home.Home, "state", "ingestion", "docling")})
		}
	}
	body, err := releasecontract.CanonicalJSON(review)
	if err != nil {
		return Review{}, err
	}
	review.Digest = fmt.Sprintf("%x", sha256.Sum256(body))
	return review, nil
}

func Apply(ctx context.Context, r Request, c Config, digest string, confirmed bool) (Result, error) {
	result := Result{State: "pending", CodeRegistration: "pending", DesktopLoading: "pending", Content: docling.Availability{State: "unavailable", Reason: "pack_unavailable"}}
	if !confirmed || len(digest) != 64 {
		return result, errors.New("explicit confirmation and exact review digest required")
	}
	review, err := ReviewInstallation(r, c)
	if err != nil {
		return result, err
	}
	if review.Digest != digest {
		return result, errors.New("setup review changed; review exact effects again")
	}
	o := installOptions(review.Request, c)
	o.ExpectedPlanDigest = review.Installation.PlanDigest
	result.Installation, err = installer.Install(ctx, o)
	if err != nil {
		return result, err
	}
	result.State = "activated_setup_pending"
	pending := func(err error) (Result, error) {
		return result, fmt.Errorf("signed payload activated; owner data preserved; setup pending: %w", err)
	}
	homeOptions := datahome.Options{ProductRoot: review.Installation.ManagedRoot, UserHome: r.UserHome, LookupEnv: c.LookupEnv}
	home, err := datahome.Resolve(homeOptions)
	if err != nil {
		return pending(err)
	}
	if home.Home != review.Home.Home || home.SelectedBy != review.Home.SelectedBy || home.Origin != review.Home.Origin || home.Writable != review.Home.Writable {
		return pending(errors.New("home selection changed after activation"))
	}
	if !home.Writable {
		home, err = datahome.InitializeFresh(homeOptions)
		if err != nil {
			return pending(err)
		}
	}
	result.Home = home.Home
	w, err := workspace.Initialize(workspace.Options{WorkspacePath: r.Workspace, DataRoot: filepath.Join(home.Home, "state")})
	if err != nil {
		return pending(err)
	}
	result.WorkspaceID = w.WorkspaceID
	registry := filepath.Join(review.Installation.ManagedRoot, "trust", "release-authority-registry.json")
	binding := product.Options{ProductRoot: review.Installation.ManagedRoot, UserHome: r.UserHome, WorkspacePath: r.Workspace, LookupEnv: c.LookupEnv, CLIVersion: review.Installation.ServerVersion, AuthorityRegistryPath: registry, AuthorityRegistrySHA256: review.Installation.RegistrySHA256}
	bound, err := product.Open(binding)
	if err != nil {
		return pending(err)
	}
	if _, err = bound.InitializeOwner(); err != nil {
		return pending(err)
	}
	if err = bound.InitializeMemory(); err != nil {
		return pending(err)
	}
	state, err := installtx.ReadStateForManagedRoot(review.Installation.DataRoot, review.Installation.ManagedRoot)
	if err != nil {
		return pending(err)
	}
	if err = installtx.CheckActivatedServer(review.Installation.ManagedRoot, state); err != nil {
		return pending(err)
	}
	trusted, err := releaseverify.LoadPinnedAuthorityRegistry(registry, review.Installation.RegistrySHA256, time.Now)
	if err != nil {
		return pending(err)
	}
	verified, err := releaseverify.VerifyDirectory(review.Installation.ReleaseDir, trusted)
	if err != nil {
		return pending(err)
	}
	if verified.ManifestSHA256 != review.Installation.ManifestSHA256 {
		return pending(errors.New("release changed after activation"))
	}
	for _, artifact := range verified.Manifest.Artifacts {
		if artifact.Kind == "runtime_pack" && artifact.OS == runtime.GOOS && artifact.Arch == runtime.GOARCH {
			if err = docling.PlaceVerified(ctx, docling.Options{DataRoot: filepath.Join(home.Home, "state"), AuthorityRegistryPath: registry, AuthorityRegistrySHA256: review.Installation.RegistrySHA256, CLIVersion: review.Installation.ServerVersion}, verified); err != nil {
				return pending(err)
			}
		}
	}
	result.Content = bound.DoclingRuntime().Status(ctx)
	for _, artifact := range verified.Manifest.Artifacts {
		if artifact.Kind == "runtime_pack" && artifact.OS == runtime.GOOS && artifact.Arch == runtime.GOARCH && result.Content.State != "ready" {
			return pending(errors.New("supplied authenticated converter is unavailable: " + result.Content.Reason))
		}
	}
	for _, packet := range review.Packets {
		if packet.Existing != nil {
			current, readErr := inspectPacket(packet, review.Installation)
			if readErr != nil {
				return pending(readErr)
			}
			if current != *packet.Existing {
				return pending(errors.New("reviewed existing packet changed; preserved without replacement"))
			}
		} else {
			options := packaging.Options{Server: result.Installation.CLIPath, Version: review.Installation.Release, Binding: binding, Destination: packet.Destination}
			if packet.Format == "plugin" {
				err = packaging.BuildPlugin(options)
			} else {
				err = packaging.BuildExtension(options)
			}
			if err != nil {
				return pending(err)
			}
		}
		if packet.Format == "plugin" {
			result.CodePacket = packet.Destination
		} else {
			result.DesktopPacket = packet.Destination
		}
	}
	if _, err = product.Open(binding); err != nil {
		return pending(err)
	}
	result.State = "ready"
	return result, nil
}

func inspectPacket(packet PacketReview, plan installer.Plan) (packaging.PacketStatus, error) {
	var status packaging.PacketStatus
	var err error
	if packet.Format == "plugin" {
		status, err = packaging.ValidatePacket(packet.Destination)
	} else {
		status, err = packaging.ValidateArchive(packet.Destination)
	}
	if err != nil {
		return status, err
	}
	if plan.Server == nil || status.Format != packet.Format || status.Version != plan.Release || status.Target.OS != plan.TargetOS || status.Target.Arch != plan.TargetArch || status.ServerSHA256 != plan.Server.SHA256 || status.RegistrySHA256 != plan.RegistrySHA256 || status.NativeMode != "mcp_only" {
		return status, errors.New("existing packet differs from exact reviewed product, target, seed or portable mode")
	}
	return status, nil
}

func overlap(a, b string) bool {
	a, b = strings.ToLower(a), strings.ToLower(b)
	for _, pair := range [][2]string{{a, b}, {b, a}} {
		rel, err := filepath.Rel(pair[0], pair[1])
		if err == nil && (rel == "." || rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))) {
			return true
		}
	}
	return false
}

// DecodeRequest is the closed operator input; configuration and trust callbacks
// never come from this document.
func DecodeRequest(body []byte) (Request, error) {
	var request Request
	err := releasecontract.DecodeStrictJSON(body, &request)
	return request, err
}
