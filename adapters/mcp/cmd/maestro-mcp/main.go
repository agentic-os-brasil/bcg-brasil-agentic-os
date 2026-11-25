// maestro-mcp explicitly enrolls a workspace under fresh or admitted owner state.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	product "github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp/packaging"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/datahome"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installer"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/maintenance"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/processidentity"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspace"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
)

// AuthorityRegistrySHA256 is linker-bound by the release builder; empty is unavailable.
var AuthorityRegistrySHA256 string
var Version string
var CompatibleBundle string

func main() {
	if err := run(); err != nil {
		if len(os.Args) > 1 && os.Args[1] == "lifecycle-hook" {
			fmt.Fprintln(os.Stderr, "maestro: lifecycle context unavailable; inspect the selected binding and explicit preparation")
			os.Exit(1)
		}
		if len(os.Args) > 1 && (os.Args[1] == "hook" || os.Args[1] == "installed-native-hook") {
			event := ""
			for i, arg := range os.Args {
				if value, ok := strings.CutPrefix(arg, "--event="); ok {
					event = value
				}
				if arg == "--event" && i+1 < len(os.Args) {
					event = os.Args[i+1]
				}
			}
			fmt.Fprintln(os.Stderr, "maestro: hook unavailable; inspect admitted state before retrying")
			os.Exit(product.HookExitCode(event))
		}
		if len(os.Args) > 1 && strings.HasPrefix(os.Args[1], "maintenance-") {
			_ = json.NewEncoder(os.Stderr).Encode(product.MaintenanceCommandFailure(err, "input").Diagnostic)
		} else if errors.Is(err, product.ErrSourceRecoveryRequired) {
			fmt.Fprintln(os.Stderr, "maestro: signed source recovery required; use the independently admitted local maestro-install source-review/source-check route before restarting")
		} else if errors.Is(err, packaging.ErrDesktopPilotInterpolation) {
			fmt.Fprintln(os.Stderr, "maestro: Desktop rehearsal path contains MCPB interpolation; select a local rehearsal path without ${")

		} else if len(os.Args) > 1 && strings.HasPrefix(os.Args[1], "knowledge-") {
			fmt.Fprintln(os.Stderr, "maestro: knowledge operation refused; read knowledge-review from JSON stdin, then use knowledge-apply with the same request, exact --review-digest and explicit --confirm")
		} else if len(os.Args) > 1 && strings.HasPrefix(os.Args[1], "memory-") {
			fmt.Fprintln(os.Stderr, "maestro: memory operation refused; read the current private memory-review from JSON stdin, then retry memory-apply with the same request, its --review-digest and explicit --confirm")
		} else if len(os.Args) > 1 && (os.Args[1] == "owner-apply" || os.Args[1] == "owner-confirm-onboarding") {
			fmt.Fprintln(os.Stderr, "maestro: owner confirmation refused; inspect the selected binding, read the current full owner-review or owner-onboarding-review, then retry with its exact --review-digest and --confirm")
		} else if len(os.Args) > 1 && os.Args[1] == "owner-track" {
			fmt.Fprintln(os.Stderr, "maestro: owner track selection refused; inspect the selected binding, then use --track quick|complete with explicit --confirm")
		} else if len(os.Args) > 1 && os.Args[1] == "runtime-apply" {
			fmt.Fprintln(os.Stderr, runtimeApplyErrorMessage(err))
		} else {
			fmt.Fprintln(os.Stderr, "maestro: operation unavailable; inspect the selected binding before retrying")
		}
		os.Exit(1)
	}
}
func runtimeApplyErrorMessage(err error) string {
	const message = "maestro: operation unavailable; inspect the selected binding before retrying"
	operation := "filesystem"
	cause := err
	var pathError *os.PathError
	var linkError *os.LinkError
	if errors.As(err, &pathError) && pathError != nil {
		operation, cause = pathError.Op, pathError.Err
	} else if errors.As(err, &linkError) && linkError != nil {
		operation, cause = linkError.Op, linkError.Err
	}
	var errno syscall.Errno
	if !errors.As(cause, &errno) {
		return message
	}
	switch operation {
	case "open", "openat", "mkdir", "chmod", "write", "close", "rename", "stat", "lstat", "readdir", "sync", "unlinkat":
	default:
		operation = "filesystem"
	}
	return fmt.Sprintf("%s [filesystem operation=%s errno=%d]", message, operation, errno)
}

func run() (runErr error) {
	if len(os.Args) > 1 && os.Args[1] == "pilot-check" {
		return runPilotCheck(context.Background(), os.Args[2:], os.Stdout)
	}
	running, err := processidentity.Current()
	if err != nil {
		return err
	}
	if len(os.Args) < 2 {
		return errors.New("command_required")
	}
	if handled, err := experimentalEarlyCommand(context.Background(), os.Args[1], os.Args[2:], os.Stdout); handled {
		return err
	}
	if os.Args[1] == "identity" {
		if len(os.Args) != 2 {
			return errors.New("unexpected_arguments")
		}
		identity, err := product.Identity(Version, CompatibleBundle, AuthorityRegistrySHA256)
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(identity)
	}
	if os.Args[1] == "lifecycle-capabilities" {
		if len(os.Args) != 2 || Version == "" || AuthorityRegistrySHA256 == "" {
			return errors.New("lifecycle protocol unavailable")
		}
		return json.NewEncoder(os.Stdout).Encode(installer.LifecycleClientCapabilities("server", Version, AuthorityRegistrySHA256))
	}
	if os.Args[1] == "client-target-capabilities" {
		if len(os.Args) != 2 || Version == "" || AuthorityRegistrySHA256 == "" {
			return errors.New("server target capabilities unavailable")
		}
		return json.NewEncoder(os.Stdout).Encode(installer.DesktopClientCapabilities("server", Version, AuthorityRegistrySHA256))
	}
	if os.Args[1] == "client-preparation-capabilities" {
		if len(os.Args) != 2 || Version == "" || AuthorityRegistrySHA256 == "" {
			return errors.New("server preparation capabilities unavailable")
		}
		return json.NewEncoder(os.Stdout).Encode(installer.ClientPreparationCapabilities("server", Version, AuthorityRegistrySHA256))
	}
	if os.Args[1] == "client-capabilities" {
		if len(os.Args) != 2 {
			return errors.New("unexpected_arguments")
		}
		return json.NewEncoder(os.Stdout).Encode(installer.StableClientCapabilities("server", Version, AuthorityRegistrySHA256))
	}
	flags := flag.NewFlagSet("maestro-mcp", flag.ContinueOnError)
	if os.Args[1] == "hook" || os.Args[1] == "lifecycle-hook" || strings.HasPrefix(os.Args[1], "maintenance-") {
		flags.SetOutput(io.Discard)
	}
	var o product.Options
	bindExperimental := experimentalCommandFlags(flags, &o)
	if os.Args[1] == "serve" || os.Args[1] == "installed-check-serve" {
		flags.StringVar(&o.ManagementCarrierID, "management-carrier-id", "", "fixed independently admitted client carrier")
		flags.StringVar(&o.ManagementInstallerVersion, "management-installer-version", "", "fixed validated installed manager version")
	}
	var destination, server, version string
	var carrierProfile, carrierID, carrierVersion, packetFormat string
	if os.Args[1] == "plugin" || os.Args[1] == "mcpb" || os.Args[1] == "packet-verify" || os.Args[1] == "native-review" || os.Args[1] == "native-qualify-and-build" {
		flags.StringVar(&carrierProfile, "carrier-profile", "", "explicit stable packet profile")
		flags.StringVar(&carrierID, "carrier-id", "", "fixed opaque stable carrier ID")
		flags.StringVar(&carrierVersion, "carrier-version", "", "fixed stable carrier version")
	}
	var nativeExecutable, nativeScratch, nativeChallenge, nativeNonce, nativeFault, scopeDirectory, scopeOutside string
	var nativeBudget float64
	if os.Args[1] == "packet-verify" {
		flags.StringVar(&packetFormat, "packet-format", "", "exact emitted plugin or mcpb format")
	}
	var hookEvent, nativeBinding string
	var proposalID, reviewDigest, track string
	var confirmed, packet, lifecycleContext, desktopPilot, engineeringRuntimePilot, codePilot bool
	var runtimeRelease, acquisitionReviewFile string
	var weeklyRegistrationFile *string
	var maintenanceDeadline string
	var maintenanceProgress bool
	var selectedPacket packaging.PacketStatus
	switch os.Args[1] {
	case "serve", "maintenance-review", "maintenance-apply", "maintenance-status", "maintenance-run":
		if os.Args[1] != "serve" {
			flags.StringVar(&maintenanceDeadline, "maintenance-deadline", "", "restrictive deadline propagated by the original local manager")
			flags.BoolVar(&maintenanceProgress, "maintenance-progress", false, "emit finite metadata-only progress for the original manager")
		}
		weeklyRegistrationFile = flags.String("weekly-registration-file", "", "explicit local weekly model declaration; loading neither qualifies nor activates it")
	}
	if os.Args[1] == "mcpb" {
		flags.BoolVar(&desktopPilot, "desktop-pilot", false, "isolate this local engineering rehearsal and bind its exact prepared Desktop paths")
		flags.BoolVar(&engineeringRuntimePilot, "engineering-runtime-pilot", false, "explicitly carry the prepared short-lived engineering converter identity")
	}
	if os.Args[1] == "runtime-review" || os.Args[1] == "runtime-apply" || os.Args[1] == "pilot-runtime-review" || os.Args[1] == "pilot-runtime-apply" || os.Args[1] == "pilot-acquisition-review" || os.Args[1] == "pilot-acquisition-apply" {
		flags.StringVar(&runtimeRelease, "release-directory", "", "exact complete signed engineering release")
	}
	if os.Args[1] == "pilot-acquisition-apply" {
		flags.StringVar(&acquisitionReviewFile, "acquisition-review-file", "", "exact full local acquisition review")
	}
	if os.Args[1] == "plugin" {
		flags.BoolVar(&lifecycleContext, "lifecycle-context", false, "include explicitly selected passive lifecycle context hooks")
	}
	if os.Args[1] == "plugin" || os.Args[1] == "native-review" || os.Args[1] == "native-qualify-and-build" {
		flags.BoolVar(&codePilot, "code-pilot", false, "explicit participant-local experimental Code packet")
	}
	if os.Args[1] == "serve" || os.Args[1] == "installed-check-serve" || os.Args[1] == "mcpb" || os.Args[1] == "plugin" || os.Args[1] == "native-review" || os.Args[1] == "native-qualify-and-build" {
		flags.StringVar(&o.OwnerReviewChannel, "owner-review-channel", "", "explicit local-browser selection for complete foreground owner review; empty keeps native-first")
	}
	flags.BoolVar(&packet, "packet", false, "validate the complete managed packet before runtime use")
	flags.StringVar(&destination, "destination", "", "new packet destination")
	flags.StringVar(&server, "server", "", "compiled native server binary")
	flags.StringVar(&version, "version", "", "packet semantic version")
	flags.StringVar(&o.ProductRoot, "product-root", "", "existing installed product directory")
	flags.StringVar(&o.UserHome, "user-home", "", "existing owner directory")
	flags.StringVar(&o.WorkspacePath, "workspace", "", "explicit selected workspace directory")
	flags.StringVar(&o.InstalledManifestSHA256, "installed-manifest", "", "fixed carrier signed release manifest digest")
	flags.StringVar(&o.InstalledClientInputsSHA256, "installed-client-inputs", "", "fixed carrier immutable client input digest")
	if os.Args[1] == "native-review" || os.Args[1] == "native-qualify-and-build" {
		flags.StringVar(&nativeExecutable, "native-executable", "", "explicit installed Code executable")
		flags.StringVar(&nativeScratch, "native-scratch-root", "", "existing private owner scratch directory")
		flags.StringVar(&nativeChallenge, "native-challenge-directory", "", "new exact synthetic challenge directory inside the selected workspace")
		flags.Float64Var(&nativeBudget, "max-budget-usd", 0, "reviewed estimated per-call limit, greater than zero and at most one USD")
	}
	if os.Args[1] == "native-startup-check" || os.Args[1] == "native-qualification-hook" {
		flags.StringVar(&nativeNonce, "nonce", "", "qualification startup nonce")
	}
	if os.Args[1] == "native-scope-guard" {
		flags.StringVar(&scopeDirectory, "challenge-directory", "", "exact qualification control directory")
		flags.StringVar(&scopeOutside, "outside-probe-path", "", "exact reviewed public outside read")
	}
	if os.Args[1] == "native-fault" {
		flags.StringVar(&nativeFault, "fault", "", "fixed qualification fault")
	}
	if os.Args[1] == "hook" || os.Args[1] == "lifecycle-hook" {
		flags.StringVar(&hookEvent, "event", "", "native hook event")
		flags.StringVar(&nativeBinding, "binding-sha256", "", "optional reviewed local selection equality check; not qualification authority")
	}
	switch os.Args[1] {
	case "owner-review", "owner-apply":
		flags.StringVar(&proposalID, "proposal-id", "", "exact owner proposal ID")
	}
	switch os.Args[1] {
	case "owner-apply", "owner-confirm-onboarding", "memory-apply", "knowledge-apply", "maintenance-apply", "native-qualify-and-build", "runtime-apply", "pilot-runtime-apply", "pilot-acquisition-apply":
		flags.StringVar(&reviewDigest, "review-digest", "", "exact SHA-256 digest from the full local review")
	}
	switch os.Args[1] {
	case "owner-apply", "owner-confirm-onboarding", "owner-track", "memory-apply", "knowledge-apply", "maintenance-apply", "native-qualify-and-build", "runtime-apply", "pilot-runtime-apply", "pilot-acquisition-apply":
		flags.BoolVar(&confirmed, "confirm", false, "local operator confirms the reviewed operation")
	}
	if os.Args[1] == "owner-track" {
		flags.StringVar(&track, "track", "", "quick or complete")
	}
	if err := flags.Parse(os.Args[2:]); err != nil {
		return err
	}
	var commandCtx context.Context = context.Background()
	var observeMaintenance func(string)
	switch os.Args[1] {
	case "maintenance-review", "maintenance-apply", "maintenance-status", "maintenance-run":
		ctx, stopBudget, err := product.MaintenanceCommandContext(commandCtx, maintenanceDeadline)
		if err != nil {
			return err
		}
		defer stopBudget()
		ctx, stopSignal := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
		defer stopSignal()
		stopRead := context.AfterFunc(ctx, func() { _ = os.Stdin.Close() })
		defer stopRead()
		phase, progressCount := "admission", 0
		observeMaintenance = func(next string) {
			phase = next
			if maintenanceProgress && progressCount < 32 {
				_ = json.NewEncoder(os.Stderr).Encode(product.MaintenanceCommandProgress(next))
				progressCount++
			}
		}
		defer func() {
			if runErr != nil {
				runErr = product.MaintenanceCommandFailure(runErr, phase)
			}
		}()
		commandCtx = maintenance.WithOperationObserver(ctx, observeMaintenance)
		observeMaintenance("admission")
	}
	if err := bindExperimental(commandCtx); err != nil {
		return err
	}
	authoritySeeded := false
	if packet {
		switch os.Args[1] {
		case "init", "serve", "inspect", "hook", "lifecycle-hook", "native-startup-check", "native-qualification-hook", "native-scope-guard":
			root, status, err := runtimePacketRoot(o.ProductRoot, running)
			if err != nil {
				return err
			}
			o.ProductRoot = root
			selectedPacket = status
			if status.DesktopPilot != nil && status.DesktopPilot.Runtime != nil {
				if err := packaging.ValidateEngineeringPacketSelection(root, status, running.Path, running.SHA256, Version, AuthorityRegistrySHA256); err != nil {
					return err
				}
				seedDoclingAuthority(&o)
				authoritySeeded = true
			}
			if status.CodePilot != nil && status.CodePilot.Runtime != nil {
				if err := packaging.ValidateEngineeringCodePacketSelection(root, status, running.Path, running.SHA256, Version, AuthorityRegistrySHA256); err != nil {
					return err
				}
				seedDoclingAuthority(&o)
				authoritySeeded = true
			}
			if status.CodePilot != nil {
				if err := status.CodePilot.ValidateSelection(o, os.Args[1] == "serve"); err != nil {
					return err
				}
			} else if o.OwnerReviewChannel != "" && status.DesktopPilot == nil {
				return errors.New("packet owner review requires an explicit experimental pilot")
			}
			if status.NativeMode == "native" {
				if err := packaging.ValidateNativeSelection(o, status.NativeBindingSHA256); err != nil {
					return err
				}
			}
			if status.DesktopPilot != nil {
				if err := status.DesktopPilot.ValidateSelection(o); err != nil {
					return err
				}
			}
		default:
			return errors.New("packet validation is available only for init, serve, inspect and hook")
		}
	}
	if !authoritySeeded {
		seedDoclingAuthority(&o)
	}
	o.GuardProcess = Version != ""
	if flags.NArg() != 0 {
		return errors.New("unexpected_arguments")
	}
	if weeklyRegistrationFile != nil {
		if err := configureWeeklyRegistration(&o, *weeklyRegistrationFile); err != nil {
			return err
		}
	}
	var carrier *packaging.StableCarrier
	if carrierProfile != "" || carrierID != "" || carrierVersion != "" {
		carrier = &packaging.StableCarrier{Profile: carrierProfile, Protocol: installer.StableClientProtocol, ID: carrierID, Version: carrierVersion}
	}
	switch os.Args[1] {
	case "runtime-review", "runtime-apply":
		if packet || server != "" || destination != "" || version != "" || carrier != nil {
			return errors.New("installed runtime accepts only fixed current owner selection")
		}
		return runInstalledRuntime(context.Background(), os.Args[1], o, runtimeRelease, reviewDigest, confirmed, os.Stdout)
	case "desktop-methods", "desktop-methods-verify":
		if packet || destination == "" || server != "" || version != "" || carrier != nil || o.ProductRoot != "" || o.UserHome != "" || o.WorkspacePath != "" {
			return errors.New("exact current Desktop methods destination required")
		}
		var report packaging.DesktopFrontendReport
		var err error
		if os.Args[1] == "desktop-methods" {
			report, err = packaging.BuildDesktopMethodFrontend(packaging.DesktopFrontendOptions{Destination: destination, Version: Version, LiveMethods: true})
		} else {
			report, err = packaging.VerifyDesktopLiveMethodFrontend(destination, Version)
		}
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(report)
	case "pilot-acquisition-review", "pilot-acquisition-apply":
		if packet || server != "" || destination != "" || version != "" || carrier != nil || runtimeRelease == "" {
			return errors.New("exact local acquisition arguments required")
		}
		var value any
		var err error
		if os.Args[1] == "pilot-acquisition-review" {
			value, err = reviewPilotAcquisition(context.Background(), o, running.Path, runtimeRelease)
		} else {
			value, err = applyPilotAcquisition(context.Background(), o, running.Path, runtimeRelease, acquisitionReviewFile, reviewDigest, confirmed)
		}
		if err != nil {
			return err
		}
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(value)
	case "pilot-runtime-review", "pilot-runtime-apply":
		if packet || server != "" || destination != "" || version != "" || carrier != nil || runtimeRelease == "" {
			return errors.New("exact local engineering runtime arguments required")
		}
		var value any
		var err error
		if os.Args[1] == "pilot-runtime-review" {
			value, err = packaging.ReviewEngineeringRuntime(o, running.Path, runtimeRelease)
		} else {
			value, err = packaging.ApplyEngineeringRuntime(context.Background(), o, running.Path, runtimeRelease, reviewDigest, confirmed)
		}
		if err != nil {
			return err
		}
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(value)
	case "packet-verify":
		if carrier == nil || destination == "" || server != "" || version != "" || o.ProductRoot != "" || o.UserHome != "" || o.WorkspacePath != "" {
			return errors.New("exact read-only packet verification arguments required")
		}
		identity, err := product.Identity(Version, CompatibleBundle, AuthorityRegistrySHA256)
		if err != nil {
			return err
		}
		running, err := processidentity.Current()
		if err != nil {
			return err
		}
		status, err := packaging.VerifyStablePacket(destination, packetFormat, *carrier, identity, running.SHA256)
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(status)
	case "native-review", "native-qualify-and-build":
		request := packaging.NativeOptions{Packet: packaging.Options{Destination: destination, Server: server, Version: version, Binding: o, CodePilot: codePilot, Carrier: carrier}, Executable: nativeExecutable, ScratchRoot: nativeScratch, ChallengeDirectory: nativeChallenge, MaxBudgetUSD: nativeBudget}
		var value any
		var err error
		if os.Args[1] == "native-review" {
			value, err = packaging.ReviewNative(context.Background(), request)
		} else {
			value, err = packaging.QualifyAndBuildNative(context.Background(), request, reviewDigest, confirmed)
		}
		if err != nil {
			if partial, ok := value.(packaging.NativeResult); ok && partial.ScratchDirectory != "" {
				_ = json.NewEncoder(os.Stdout).Encode(partial)
			}
			return err
		}
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(value)
	case "native-qualification-hook":
		return packaging.QualificationHook(o, nativeNonce, os.Stdin, os.Stdout)
	case "native-scope-guard":
		return packaging.QualificationScopeGuard(o, scopeDirectory, scopeOutside, os.Stdin, os.Stdout)
	case "native-startup-check":
		return packaging.CheckNativeStartup(o, nativeNonce, os.Stdin, os.Stdout)
	case "native-fault":
		return packaging.NativeFault(context.Background(), nativeFault)
	case "hook", "lifecycle-hook":
		if nativeBinding != "" {
			if !packet {
				return errors.New("native selection requires packet validation")
			}
			if err := packaging.ValidateNativeSelection(o, nativeBinding); err != nil {
				return err
			}
		}
		bound, err := product.Open(o)
		if err != nil {
			return err
		}
		if os.Args[1] == "lifecycle-hook" {
			return bound.HandleClaudeLifecycleHook(hookEvent, os.Stdin, os.Stdout)
		}
		return bound.HandleClaudeHook(hookEvent, os.Stdin, os.Stdout)
	case "plugin":
		return packaging.BuildPlugin(packaging.Options{Destination: destination, Server: server, Version: version, Binding: o, CodePilot: codePilot, Carrier: carrier, LifecycleContext: lifecycleContext})
	case "mcpb":
		return packaging.BuildExtension(packaging.Options{Destination: destination, Server: server, Version: version, Binding: o, Carrier: carrier, DesktopPilot: desktopPilot, EngineeringRuntimePilot: engineeringRuntimePilot})
	case "inspect":
		h, err := datahome.Resolve(datahome.Options{ProductRoot: o.ProductRoot, UserHome: o.UserHome})
		diagnostic := h.Diagnostic
		if diagnostic == "" {
			diagnostic = "unavailable"
		}
		result := map[string]any{"diagnostic": diagnostic, "writable": h.Writable, "origin": h.Origin}
		if packet {
			result["packet_mode"], result["packet_server_sha256"] = selectedPacket.NativeMode, selectedPacket.ServerSHA256
		}
		_ = json.NewEncoder(os.Stdout).Encode(result)
		return err
	case "init":
		if err := product.Preflight(o); err != nil {
			return err
		}
		homeOptions := datahome.Options{ProductRoot: o.ProductRoot, UserHome: o.UserHome}
		home, err := datahome.Resolve(homeOptions)
		if err != nil {
			return err
		}
		if !home.Writable {
			home, err = datahome.InitializeFresh(homeOptions)
			if err != nil {
				return err
			}
		}
		w, err := workspace.Initialize(workspace.Options{WorkspacePath: o.WorkspacePath, DataRoot: filepath.Join(home.Home, "state")})
		if err != nil {
			return err
		}
		bound, err := product.Open(o)
		if err != nil {
			return err
		}
		if _, err := bound.InitializeOwner(); err != nil {
			return err
		}
		if err := bound.InitializeMemory(); err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(map[string]string{"state": "ready", "workspace_id": w.WorkspaceID})
	case "owner-review", "owner-apply", "owner-onboarding-review", "owner-confirm-onboarding", "owner-track":
		bound, err := product.Open(o)
		if err != nil {
			return err
		}
		var value any
		switch os.Args[1] {
		case "owner-review":
			value, err = bound.OwnerReview(proposalID)
		case "owner-apply":
			value, err = bound.ApplyOwnerReview(proposalID, reviewDigest, confirmed)
		case "owner-onboarding-review":
			value, err = bound.OwnerOnboardingReview()
		case "owner-confirm-onboarding":
			value, err = bound.ConfirmOwnerOnboarding(reviewDigest, confirmed)
		case "owner-track":
			value, err = bound.SelectOwnerTrack(track, confirmed)
		}
		if err != nil {
			return err
		}
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(value)
	case "knowledge-review", "knowledge-apply":
		bound, err := product.Open(o)
		if err != nil {
			return err
		}
		body, err := io.ReadAll(io.LimitReader(os.Stdin, (2<<20)+1))
		if err != nil || len(body) > 2<<20 {
			return errors.New("knowledge_input_size")
		}
		request, err := product.DecodeKnowledgeMutation(body)
		if err != nil {
			return err
		}
		var value any
		if os.Args[1] == "knowledge-review" {
			value, err = bound.KnowledgeReview(request)
		} else {
			value, err = bound.ApplyKnowledgeReview(request, reviewDigest, confirmed)
		}
		if err != nil {
			return err
		}
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(value)
	case "memory-review", "memory-apply":
		bound, err := product.Open(o)
		if err != nil {
			return err
		}
		body, err := io.ReadAll(io.LimitReader(os.Stdin, (128<<10)+1))
		if err != nil || len(body) > 128<<10 {
			return errors.New("memory_input_size")
		}
		request, err := product.DecodeMemoryMutation(body)
		if err != nil {
			return err
		}
		var value any
		if os.Args[1] == "memory-review" {
			value, err = bound.MemoryReview(request)
		} else {
			value, err = bound.ApplyMemoryReview(request, reviewDigest, confirmed)
		}
		if err != nil {
			return err
		}
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(value)
	case "maintenance-review", "maintenance-apply", "maintenance-status", "maintenance-run":
		ctx := commandCtx
		observe := observeMaintenance
		bound, err := product.OpenContext(ctx, o)
		if err != nil {
			return err
		}
		var value any
		if os.Args[1] == "maintenance-status" {
			observe("verification")
			value, err = bound.MaintenanceStatusContext(ctx)
		} else if os.Args[1] == "maintenance-run" {
			observe("execution")
			value, err = bound.RunMaintenance(ctx)
		} else {
			observe("input")
			body, readErr := io.ReadAll(io.LimitReader(os.Stdin, (16<<10)+1))
			if readErr != nil || len(body) > 16<<10 {
				if err := ctx.Err(); err != nil {
					return err
				}
				return errors.New("maintenance_input_size")
			}
			request, decodeErr := product.DecodeMaintenanceMutation(body)
			if decodeErr != nil {
				return decodeErr
			}
			observe("review")
			if os.Args[1] == "maintenance-review" {
				value, err = bound.MaintenanceReviewContext(ctx, request)
			} else {
				value, err = bound.ApplyMaintenanceReview(ctx, request, reviewDigest, confirmed)
			}
		}
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		observe("output")
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(value)
	case "serve", "installed-check-serve":
		bound, err := product.Open(o)
		if err != nil {
			return err
		}
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		if os.Args[1] == "installed-check-serve" {
			name := "maestro-install"
			if runtime.GOOS == "windows" {
				name += ".exe"
			}
			checker, checkErr := installer.AdmitStableCheckServer(ctx, installer.ClientOptions{SeededOperationOptions: installer.SeededOperationOptions{ExperimentalSeed: o.ExperimentalSeed, Self: filepath.Join(o.ProductRoot, name), Version: o.ManagementInstallerVersion, AuthorityRegistrySHA256: o.AuthorityRegistrySHA256, ManagedRoot: o.ProductRoot, UserHome: o.UserHome, DataRoot: filepath.Join(o.UserHome, ".maestro-install")}, Request: installer.ClientRequest{CarrierID: o.ManagementCarrierID, Workspace: o.WorkspacePath}})
			if checkErr != nil {
				return checkErr
			}
			err = bound.ServeInstalledCheck(ctx, &mcp.StdioTransport{}, checker)
		} else {
			err = bound.Serve(ctx, &mcp.StdioTransport{})
		}
		if errors.Is(err, context.Canceled) && ctx.Err() != nil {
			return nil
		}
		return err
	default:
		return errors.New("unsupported_command")
	}
}
