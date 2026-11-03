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
	switch command {
	case "clients-review", "clients-apply", "clients-reconcile", "clients-status", "code-review", "code-apply", "desktop-confirm", "stable-launch", "manage":
	default:
		return false, nil
	}
	o := installer.ClientOptions{SeededOperationOptions: installer.SeededOperationOptions{Self: self, Version: Version, AuthorityRegistrySHA256: AuthorityRegistrySHA256, ManagedRoot: f.ManagedRoot, UserHome: f.UserHome, DataRoot: filepath.Join(f.UserHome, ".maestro-install")}, Request: installer.ClientRequest{CarrierID: f.CarrierID, Workspace: f.Workspace}}
	if command == "manage" {
		o.Request.CodeRoot = f.ClientCodeRoot
		o.Request.DesktopPath = f.ClientDesktopPath
		o.Request.CodeExecutable = f.ClientCodeExecutable
		o.Request.CodeScope = f.ClientScope
	} else if f.ClientCodeRoot != "" || f.ClientDesktopPath != "" || f.ClientCodeExecutable != "" || f.ClientScope != "" {
		return true, errors.New("operator launcher recovery choices are valid only for manage")
	}
	encode := func(value any, err error) (bool, error) {
		if e := json.NewEncoder(os.Stdout).Encode(value); e != nil {
			return true, e
		}
		return true, err
	}
	if command == "stable-launch" {
		if f.ManifestSHA256 != "" || f.ClientInputsSHA256 != "" || f.PlanID != "" {
			return true, errors.New("stable launch accepts only fixed roots/workspace/carrier ID")
		}
		launch, err := installer.PrepareStableLaunch(ctx, o)
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
		return true, manageClients(ctx, o, os.Stdin, os.Stdout)
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

func manageClients(ctx context.Context, o installer.ClientOptions, input io.Reader, output io.Writer) error {
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
		review, e := installer.ReviewClientsSeeded(ctx, o)
		if e != nil {
			return e
		}
		if e = confirm(review); e != nil {
			return e
		}
		r, e := setup.ApplyClients(ctx, o, review.Digest, true)
		if e != nil {
			return e
		}
		fmt.Fprintln(output, "Stable client packets are ready. Desktop installation still uses its normal manual extension prompts; external-launcher compatibility and native session loading remain unqualified.", r.Materialization.DesktopPath)
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
		return json.NewEncoder(output).Encode(r)
	}
	r := *status.Record
	o.Request = installer.ClientRequest{CarrierID: r.CarrierID, Workspace: r.Binding.Workspace, CodeRoot: r.Materialization.CodeRoot, DesktopPath: r.Materialization.DesktopPath, CodeExecutable: r.Code.Executable, CodeScope: r.Code.Scope}
	fmt.Fprintln(output, "Maestro local management. Client loading and native qualification are reported separately.")
	action, e := prompt("Choose status, update, regenerate, reconcile, register-code, desktop-confirm, manual-desktop-update, or rollback: ")
	if e != nil {
		return e
	}
	switch action {
	case "", "status":
		return json.NewEncoder(output).Encode(status)
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
		if readErr != nil || status.State != "current_materialization" {
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
			r, e = setup.ApplyClients(ctx, o, selection.Digest, true)
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
	review, e := installer.ReviewClientsSeeded(ctx, o)
	if e != nil {
		return e
	}
	if e = confirm(review); e != nil {
		return e
	}
	result, e := setup.ApplyClients(ctx, o, review.Digest, true)
	if e != nil {
		return e
	}
	fmt.Fprintln(output, "Current client packets are materialized. Reload/reconnect/new-session evidence remains pending. Code registration and manual Desktop confirmation remain separate supported actions.")
	return json.NewEncoder(output).Encode(result)
}
