package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp/setup"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installer"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installtx"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/updateservice"
)

type clientRequest struct {
	ManagedRoot string                  `json:"managed_root"`
	UserHome    string                  `json:"user_home"`
	Client      installer.ClientRequest `json:"client"`
}

func runClients(ctx context.Context, command, self string, f lifecycleFlags, digest string, confirmed bool) (bool, error) {
	return runClientsWithNativeVerifier(ctx, command, self, f, digest, confirmed, nil)
}

// The trusted native-verifier seam is process composition, never a CLI field.
func runClientsWithNativeVerifier(ctx context.Context, command, self string, f lifecycleFlags, digest string, confirmed bool, verifyNative func(context.Context, string) error) (bool, error) {
	switch command {
	case "clients-review", "clients-apply", "clients-reconcile", "clients-status", "code-review", "code-apply", "desktop-confirm", "stable-launch", "stable-hook", "client-plugin-path", "manage":
	default:
		return false, nil
	}
	o := installer.ClientOptions{SeededOperationOptions: installer.SeededOperationOptions{ExperimentalSeed: f.ExperimentalSeed, Self: self, Version: Version, AuthorityRegistrySHA256: AuthorityRegistrySHA256, ManagedRoot: f.ManagedRoot, UserHome: f.UserHome, DataRoot: filepath.Join(f.UserHome, ".maestro-install"), VerifyNative: verifyNative}, Request: installer.ClientRequest{CarrierID: f.CarrierID, Workspace: f.Workspace}}
	if command == "manage" {
		o.Request.CodeRoot = f.ClientCodeRoot
		o.Request.DesktopPath = f.ClientDesktopPath
		o.Request.CodeExecutable = f.ClientCodeExecutable
		o.Request.CodeConfigurationDirectory = f.ClientCodeConfigurationDirectory
		o.Request.CodeScope = f.ClientScope
		o.Request.LifecycleContext = f.LifecycleContext
	} else if f.ClientCodeRoot != "" || f.ClientDesktopPath != "" || f.ClientCodeExecutable != "" || f.ClientScope != "" || f.ClientCodeConfigurationDirectory != "" {
		return true, errors.New("operator launcher recovery choices are valid only for manage")
	}
	encode := func(value any, err error) (bool, error) {
		if e := json.NewEncoder(os.Stdout).Encode(value); e != nil {
			return true, e
		}
		return true, err
	}
	if command == "client-plugin-path" {
		if f.ManifestSHA256 != "" || f.ClientInputsSHA256 != "" || f.PlanID != "" {
			return true, errors.New("client path accepts only fixed selection")
		}
		path, err := installer.CurrentClientPluginPath(ctx, o)
		if err != nil {
			return true, err
		}
		_, err = fmt.Fprintln(os.Stdout, path)
		return true, err
	}
	if command == "stable-launch" || command == "stable-hook" {
		if f.ManifestSHA256 != "" || f.ClientInputsSHA256 != "" || f.PlanID != "" {
			return true, errors.New("stable launch accepts only fixed roots/workspace/carrier ID")
		}
		var launch installer.QualifiedLaunch
		var err error
		if command == "stable-hook" {
			launch, err = installer.PrepareStableLifecycleHook(ctx, o, f.HookEvent)
		} else {
			launch, err = installer.PrepareStableLaunch(ctx, o)
		}
		if err != nil {
			return true, err
		}
		child := exec.CommandContext(ctx, launch.Executable, launch.Args...)
		child.Stdin = os.Stdin
		child.Stdout = os.Stdout
		child.Stderr = os.Stderr
		return true, child.Run()
	}
	if command == "manage" {
		return true, manageClientsWithIntent(ctx, o, os.Stdin, os.Stdout, f.ManagementIntent)
	}
	if f.CarrierID == "" || command == "clients-review" || command == "clients-apply" || command == "clients-reconcile" {
		body, err := io.ReadAll(io.LimitReader(os.Stdin, 64<<10+1))
		if err != nil || len(body) > 64<<10 {
			return true, errors.New("bounded client request required")
		}
		var r clientRequest
		if err = releasecontract.DecodeStrictJSON(body, &r); err != nil {
			return true, err
		}
		if f.ManagedRoot != "" && f.ManagedRoot != r.ManagedRoot || f.UserHome != "" && f.UserHome != r.UserHome || f.CarrierID != "" && f.CarrierID != r.Client.CarrierID || f.Workspace != "" && f.Workspace != r.Client.Workspace {
			return true, errors.New("fixed client request differs")
		}
		o.ManagedRoot = r.ManagedRoot
		o.UserHome = r.UserHome
		o.DataRoot = filepath.Join(r.UserHome, ".maestro-install")
		o.Request = r.Client
	}
	if command == "clients-review" {
		r, e := installer.ReviewClientsSeeded(ctx, o)
		return encode(r, e)
	}
	if command == "clients-apply" || command == "clients-reconcile" {
		if command == "clients-reconcile" && !o.Request.Reconcile {
			return true, errors.New("client reconciliation requires the exact review with reconcile:true")
		}
		r, e := setup.ApplyClients(ctx, o, digest, confirmed)
		return encode(r, e)
	}
	status, err := installer.ReadClientsSeeded(ctx, o)
	if command == "clients-status" {
		return encode(status, err)
	}
	if err != nil {
		return encode(status, err)
	}
	if status.Record == nil || status.State != "current_materialization" {
		return encode(status, errors.New("current client materialization required"))
	}
	if command == "code-review" {
		review, e := setup.ReviewCodeRegistration(ctx, *status.Record, nil)
		return encode(review, e)
	}
	if command == "code-apply" {
		r, e := setup.RegisterCode(ctx, o, nil, digest, confirmed)
		return encode(r, e)
	}
	r, e := setup.ConfirmDesktop(ctx, o, f.PackageSHA256, f.NativeContractSHA256, digest, confirmed)
	return encode(r, e)
}

func promptClientOwnerReviewChannel(prompt func(string) (string, error)) (string, error) {
	channel, err := prompt("For protected foreground MCP changes in BOTH Code and Desktop, choose native-first (Enter) or local-browser for complete one-shot review in this computer's browser. Each change still requires its own approval; this carrier choice approves no write [native-first]: ")
	if err != nil {
		return "", err
	}
	if channel == "" || channel == "native-first" {
		return "", nil
	}
	if channel != "local-browser" {
		return "", errors.New("unsupported owner review channel; client materialization remains pending")
	}
	return channel, nil
}

func manageClients(ctx context.Context, o installer.ClientOptions, input io.Reader, output io.Writer) error {
	return manageClientsWithIntent(ctx, o, input, output, "")
}

func manageClientsWithIntent(ctx context.Context, o installer.ClientOptions, input io.Reader, output io.Writer, intent string) error {
	if intent != "" && intent != "update" {
		return errors.New("unsupported management intent; nothing applied")
	}
	reader := bufio.NewReader(io.LimitReader(input, 64<<10))
	prompt := func(text string) (string, error) {
		fmt.Fprint(output, text)
		line, e := reader.ReadString('\n')
		if e != nil {
			return "", e
		}
		return strings.TrimSpace(line), nil
	}
	confirm := func(value any) error {
		body, e := json.MarshalIndent(value, "", "  ")
		if e != nil {
			return e
		}
		fmt.Fprintln(output, string(body))
		answer, e := prompt("Review the exact effects above. Type YES to apply, or Enter to leave pending: ")
		if e != nil {
			return e
		}
		if answer != "YES" {
			return errors.New("declined; reviewed operation remains pending")
		}
		return nil
	}
	applyClients := func(options installer.ClientOptions, digest string) (installer.ClientRecord, error) {
		if !options.Request.NativeEnforcement {
			return setup.ApplyClients(ctx, options, digest, true)
		}
		var nonce [12]byte
		if _, err := rand.Read(nonce[:]); err != nil {
			return installer.ClientRecord{}, err
		}
		selection := setup.NativeClientSelection{ScratchRoot: filepath.Dir(options.Request.CodeRoot), ChallengeDirectory: filepath.Join(options.Request.Workspace, ".maestro-native-update-"+hex.EncodeToString(nonce[:])), MaxBudgetUSD: 0.25}
		review, err := setup.ReviewNativeClientUpdate(ctx, options, selection)
		if err != nil {
			return installer.ClientRecord{}, fmt.Errorf("native qualification waiting for supported inputs; previous packets preserved: %w", err)
		}
		if review.Plan.State != "reviewable" {
			_ = json.NewEncoder(output).Encode(review)
			return installer.ClientRecord{}, errors.New("full native qualification blocked; no MCP-only downgrade applied")
		}
		if err := confirm(review); err != nil {
			return installer.ClientRecord{}, err
		}
		return setup.ApplyNativeClientUpdate(ctx, options, selection, digest, review.ReviewDigest, true)
	}
	revalidateMaintenance := func(options installer.ClientOptions) error {
		status, err := setup.InspectClientMaintenance(ctx, options)
		if err != nil {
			fmt.Fprintln(output, "Maintenance qualification remains pending: current job inputs/status unavailable.")
			return json.NewEncoder(output).Encode(map[string]string{"maintenance_qualification": "waiting_input", "functional_acceptance": "pending"})
		}
		for _, job := range setup.MaintenanceRefreshCandidates(status) {
			review, err := setup.ReviewClientMaintenance(ctx, options, job)
			if err != nil {
				fmt.Fprintln(output, "Maintenance job", job.JobID, "waiting for genuine source inputs; prior enrollment and opt-in preserved.")
				continue
			}
			if err := confirm(review); err != nil {
				return err
			}
			result, err := setup.ApplyClientMaintenance(ctx, options, job, review.ReviewDigest, true)
			if err != nil {
				fmt.Fprintln(output, "Maintenance job", job.JobID, "requalification remains pending; it was not marked qualified.")
				continue
			}
			if err := json.NewEncoder(output).Encode(result); err != nil {
				return err
			}
		}
		status, err = setup.InspectClientMaintenance(ctx, options)
		if err != nil {
			return err
		}
		return json.NewEncoder(output).Encode(status)
	}
	status, readErr := installer.ReadClientsSeeded(ctx, o)
	if status.Record == nil {
		// First run collects ordinary local choices, generates the opaque ID and
		// enters the same exact reviewed owners. No hand-written JSON, saved
		// approval or native-client invocation is required to materialize.
		if o.ManagedRoot == "" {
			o.ManagedRoot = filepath.Dir(o.Self)
		}
		if o.UserHome == "" {
			var e error
			o.UserHome, e = os.UserHomeDir()
			if e != nil {
				return e
			}
			o.DataRoot = filepath.Join(o.UserHome, ".maestro-install")
		}
		state, e := installtx.ReadSourceState(o.DataRoot, o.ManagedRoot)
		if e != nil {
			return fmt.Errorf("complete the admitted installation before first client setup: %w", e)
		}
		if o.Request.CarrierID != "" && (o.Request.CodeRoot == "" || o.Request.DesktopPath == "" || o.Request.CodeScope == "") {
			return fmt.Errorf("existing selected carrier unavailable; preserve it and inspect clients-status: %w", readErr)
		}
		if state.Source == nil {
			folder, e := prompt("Select the hydrated local signed release folder: ")
			if e != nil {
				return e
			}
			id, e := prompt("Enter the reviewed team source ID: ")
			if e != nil {
				return e
			}
			source := installer.SeededSourceOptions{SeededOperationOptions: o.SeededOperationOptions, SourceRoot: folder, ExpectedSource: releasecontract.SourceTuple{SourceID: id, Channel: state.Channel, OS: runtime.GOOS, Arch: runtime.GOARCH}}
			review, e := installer.ReviewSourceSeeded(ctx, source)
			if e != nil {
				return e
			}
			if e = confirm(review); e != nil {
				return e
			}
			source.ReviewDigest = review.Digest
			if _, e = installer.CheckSourceSeeded(ctx, source); !errors.Is(e, updateservice.ErrCurrent) {
				if e == nil {
					return errors.New("source advertises another release; review the existing update flow before first client setup")
				}
				return e
			}
		}
		if o.Request.Workspace == "" {
			o.Request.Workspace, e = prompt("Select the existing admitted workspace folder: ")
			if e != nil {
				return e
			}
		}
		if o.Request.CodeRoot == "" {
			o.Request.CodeRoot, e = prompt("Choose a new participant-local Code marketplace folder: ")
			if e != nil {
				return e
			}
		}
		if o.Request.DesktopPath == "" {
			o.Request.DesktopPath, e = prompt("Choose a new participant-local Desktop .mcpb path: ")
			if e != nil {
				return e
			}
		}
		if o.Request.CodeScope == "" {
			o.Request.CodeExecutable, e = prompt("Select the installed Code executable, or Enter to leave registration pending: ")
			if e != nil {
				return e
			}
			o.Request.CodeScope, e = prompt("Select Code scope user, local, or project: ")
			if e != nil {
				return e
			}
		}
		if o.Request.CarrierID == "" {
			var nonce [16]byte
			if _, e = rand.Read(nonce[:]); e != nil {
				return e
			}
			o.Request.CarrierID = hex.EncodeToString(nonce[:])
		} else {
			o.Request.Reconcile = true
		}
		// New once-only setups use a stable native command-source provider and
		// private manifest slots. Existing legacy records keep their contract.
		o.Request.VersionedSlots = true
		if o.Request.CodeExecutable != "" && !o.Request.LifecycleContext {
			profile, e := prompt("Choose complete for identity, skills, native agents and seven enforcing hooks, or resources-only to leave native execution unavailable [complete]: ")
			if e != nil {
				return e
			}
			if profile != "" && profile != "complete" && profile != "resources-only" {
				return errors.New("unsupported client profile; nothing applied")
			}
			o.Request.NativeEnforcement = profile != "resources-only"
		}
		if o.Request.NativeEnforcement {
			var generation [16]byte
			if _, err := rand.Read(generation[:]); err != nil {
				return err
			}
			o.Request.GenerationID = hex.EncodeToString(generation[:])
		}
		if o.ExperimentalSeed != nil {
			o.Request.OwnerReviewChannel, e = promptClientOwnerReviewChannel(prompt)
			if e != nil {
				return e
			}
		}
		if err := reconcileRuntime(ctx, o, confirm, output); err != nil {
			return err
		}
		review, e := installer.ReviewClientsSeeded(ctx, o)
		if e != nil {
			return e
		}
		if e = confirm(review); e != nil {
			return e
		}
		r, e := applyClients(o, review.Digest)
		if e != nil {
			return e
		}
		fmt.Fprintln(output, "Current client packets are prepared. Install the Desktop connector and live-method bootstrap once through normal host prompts. Reconnect/reload and actual host acceptance remain pending.", r.Materialization.DesktopPath)
		if r.Materialization.DesktopMethods != nil {
			fmt.Fprintln(output, "Desktop method library:", r.Materialization.DesktopMethods.Archive)
		}
		if r.Code.Executable != "" {
			codeReview, e := setup.ReviewCodeRegistration(ctx, r, nil)
			if e != nil {
				return e
			}
			if e = confirm(codeReview); e != nil {
				return e
			}
			r, e = setup.RegisterCode(ctx, o, nil, codeReview.Digest, true)
			if e != nil {
				return e
			}
		}
		if err := json.NewEncoder(output).Encode(r); err != nil {
			return err
		}
		return revalidateMaintenance(o)
	}
	r := *status.Record
	o.Request = installer.ClientRequestFromRecord(r)
	fmt.Fprintln(output, "Maestro local management. Client loading and native qualification are reported separately.")
	action := intent
	var e error
	if action == "" {
		action, e = prompt("Choose status, update, regenerate, reconcile, register-code, maintenance, revalidate-maintenance, desktop-confirm, manual-desktop-update, or rollback: ")
		if e != nil {
			return e
		}
	}
	switch action {
	case "", "status":
		return json.NewEncoder(output).Encode(status)
	case "revalidate-maintenance":
		return revalidateMaintenance(o)
	case "update":
		state, e := installtx.ReadSourceState(o.DataRoot, o.ManagedRoot)
		if e != nil {
			return e
		}
		if state.Source == nil {
			return errors.New("source enrollment requires explicit source-review first")
		}
		source := installer.SeededSourceOptions{SeededOperationOptions: o.SeededOperationOptions, SourceRoot: state.Source.Locator, ExpectedSource: state.Source.Tuple}
		review, e := installer.ReviewSourceSeeded(ctx, source)
		if e != nil {
			return e
		}
		if e = confirm(review); e != nil {
			return e
		}
		source.ReviewDigest = review.Digest
		checked, e := installer.CheckSourceSeeded(ctx, source)
		if e != nil && !errors.Is(e, updateservice.ErrCurrent) {
			return e
		}
		if errors.Is(e, updateservice.ErrCurrent) && status.State == "current_materialization" {
			if err := reconcileRuntime(ctx, o, confirm, output); err != nil {
				return err
			}
			fmt.Fprintln(output, "The installed source and current packet generation are unchanged. No native qualification or model session was repeated. Loading remains separately unverified.")
			if r.Code.Executable != "" {
				registration, err := setup.ReviewCodeRegistration(ctx, r, nil)
				if err != nil {
					return err
				}
				if len(registration.Actions) > 0 {
					if err := confirm(registration); err != nil {
						return err
					}
					r, err = setup.RegisterCode(ctx, o, nil, registration.Digest, true)
					if err != nil {
						return err
					}
				}
			}
			if err := json.NewEncoder(output).Encode(installer.ClientPhases(r)); err != nil {
				return err
			}
			return revalidateMaintenance(o)
		}
		if e == nil {
			pending, e := updateservice.StagePending(o.DataRoot, checked.Current, checked, checked.Current.Source.ObservedAt)
			if e != nil {
				return e
			}
			operation := installer.SeededUpdateOptions{SeededOperationOptions: o.SeededOperationOptions, PlanID: pending.Plan.ID}
			pending, e = installer.ReviewPendingSeeded(ctx, operation)
			if e != nil {
				return e
			}
			if e = confirm(pending); e != nil {
				return e
			}
			if _, e = installer.UpdateSeeded(ctx, operation); e != nil {
				return e
			}
		}
	case "rollback":
		review, e := installer.ReviewRollbackSeeded(ctx, o.SeededOperationOptions)
		if e != nil {
			return e
		}
		if e = confirm(review); e != nil {
			return e
		}
		if _, e = installer.RollbackSeeded(ctx, installer.SeededRollbackOptions{SeededOperationOptions: o.SeededOperationOptions, ReviewDigest: review.Digest}); e != nil {
			return e
		}
	case "regenerate":
	case "reconcile":
		o.Request.Reconcile = true
	case "manual-desktop-update":
		o.Request.ManualDesktopUpdate = true
	case "register-code":
		// A first deferred choice may have produced a reviewed candidate before
		// the manual custody step. Re-prompt that same still-unselected choice
		// and require full current regeneration before any registration effect.
		// Source/current/binding failures never enter this recovery branch.
		recoverFirstSelection := r.Code.Executable == "" && status.Reason == "packet_changed"
		if !recoverFirstSelection && (readErr != nil || status.State != "current_materialization") {
			return errors.New("regenerate current clients before registering")
		}
		if r.Code.Executable == "" {
			selected, e := prompt("Select the installed Code executable for this pending carrier: ")
			if e != nil {
				return e
			}
			o.Request.CodeExecutable = selected
			selection, e := installer.ReviewClientsSeeded(ctx, o)
			if e != nil {
				return e
			}
			if e = confirm(selection); e != nil {
				return e
			}
			r, e = applyClients(o, selection.Digest)
			if e != nil {
				return e
			}
		}
		review, e := setup.ReviewCodeRegistration(ctx, r, nil)
		if e != nil {
			return e
		}
		if e = confirm(review); e != nil {
			return e
		}
		result, e := setup.RegisterCode(ctx, o, nil, review.Digest, true)
		if e != nil {
			return e
		}
		return json.NewEncoder(output).Encode(result)
	case "maintenance":
		return manageClientMaintenance(ctx, o, prompt, confirm, output)
	case "desktop-confirm":
		if readErr != nil || status.State != "current_materialization" {
			return errors.New("regenerate current clients before confirming Desktop")
		}
		fmt.Fprintln(output, "Install the exact .mcpb through Desktop's normal extension prompts. External-launcher compatibility remains pending native qualification. This records only operator confirmation, never connection or functional evidence.", r.Materialization.DesktopPath)
		if e = confirm(map[string]string{"package_sha256": r.Materialization.DesktopSHA256, "native_contract_sha256": r.Materialization.DesktopNativeContractSHA256}); e != nil {
			return e
		}
		result, e := setup.ConfirmDesktop(ctx, o, r.Materialization.DesktopSHA256, r.Materialization.DesktopNativeContractSHA256, installer.ClientRecordDigest(r), true)
		if e != nil {
			return e
		}
		return json.NewEncoder(output).Encode(result)
	default:
		return errors.New("unsupported local management action; nothing applied")
	}
	if action == "update" || action == "rollback" || action == "reconcile" {
		if err := reconcileRuntime(ctx, o, confirm, output); err != nil {
			return err
		}
	}
	if o.Request.NativeEnforcement {
		var generation [16]byte
		if _, err := rand.Read(generation[:]); err != nil {
			return err
		}
		o.Request.GenerationID = hex.EncodeToString(generation[:])
	}
	review, e := installer.ReviewClientsSeeded(ctx, o)
	if e != nil {
		return e
	}
	if e = confirm(review); e != nil {
		return e
	}
	result, e := applyClients(o, review.Digest)
	if e != nil {
		return e
	}
	if result.Code.Executable != "" {
		registration, err := setup.ReviewCodeRegistration(ctx, result, nil)
		if err != nil {
			return err
		}
		if len(registration.Actions) > 0 {
			if err := confirm(registration); err != nil {
				return err
			}
			result, err = setup.RegisterCode(ctx, o, nil, registration.Digest, true)
			if err != nil {
				return err
			}
		}
	}
	fmt.Fprintln(output, "Current client packets are prepared and Code registration was inspected. Use a deliberate reload/reconnect/new session to activate. Desktop's once-installed live-method bootstrap reads current canonical instructions; ordinary recipe changes need no reimport. Missing bootstrap or changed native permissions still require normal host review. Actual native/Desktop functional acceptance remains pending.")
	if result.Materialization.DesktopMethods != nil {
		fmt.Fprintln(output, "Desktop method library:", result.Materialization.DesktopMethods.Archive)
	}
	if err := json.NewEncoder(output).Encode(result); err != nil {
		return err
	}
	return revalidateMaintenance(o)
}
