// This engineering witness is compiled against the unchanged detached Task1
// module through an external module replace. It never edits that module.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/dev/releasepack"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installer"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installtx"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releaseverify"
)

type input struct{ UserHome, ManagedRoot, DataRoot, ReleaseDir, Installer, Registry, Baseline, Output, Version, InstallerVersion string }

func must(err error) {
	if err != nil {
		panic(err)
	}
}
func hash(body []byte) string { return fmt.Sprintf("%x", sha256.Sum256(body)) }
func read(path string) []byte { b, e := os.ReadFile(path); must(e); return b }
func inventory(root string) map[string]string {
	result := map[string]string{}
	must(filepath.WalkDir(root, func(path string, d fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		r, e := filepath.Rel(root, path)
		if e != nil {
			return e
		}
		if d.Type()&os.ModeSymlink != 0 {
			return errors.New("witness input alias")
		}
		info, e := d.Info()
		if e != nil {
			return e
		}
		if d.IsDir() {
			result[r] = "directory:" + info.Mode().String()
			return nil
		}
		result[r] = info.Mode().String() + ":" + hash(read(path))
		return nil
	}))
	return result
}
func main() {
	if len(os.Args) == 3 && os.Args[1] == "factory" {
		var in input
		must(json.Unmarshal(read(os.Args[2]), &in))
		candidate, e := releasepack.BuildModernCandidate(context.Background(), releasepack.ModernOptions{Root: in.Baseline, Output: in.Output, Version: in.Version, InstallerVersion: in.InstallerVersion, Channel: "canary", AuthorityRegistry: in.Registry, Target: releasepack.Target{OS: runtime.GOOS, Arch: runtime.GOARCH}})
		must(e)
		must(json.NewEncoder(os.Stdout).Encode(candidate))
		return
	}
	if len(os.Args) != 2 {
		panic("one private input JSON path required")
	}
	var in input
	must(json.Unmarshal(read(os.Args[1]), &in))
	beforeSource := map[string]map[string]string{}
	for _, dir := range []string{"internal/installtx", "internal/installer", "internal/releasecontract", "internal/releaseverify", "internal/rolemigration", "internal/processwait"} {
		beforeSource[dir] = inventory(filepath.Join(in.Baseline, dir))
	}
	mods := map[string]string{}
	for _, p := range []string{"go.mod", "go.sum"} {
		mods[p] = hash(read(filepath.Join(in.Baseline, p)))
	}
	registry, e := releaseverify.LoadAuthorityRegistry(in.Registry, time.Now)
	must(e)
	verified, e := releaseverify.VerifyDirectory(in.ReleaseDir, registry)
	must(e)
	if !verified.Manifest.Modern() {
		panic("genuine historical modern release required")
	}
	var identity releasecontract.ServerIdentity
	for _, a := range verified.Manifest.Artifacts {
		if a.Kind == "mcp_server" {
			identity, e = releasecontract.ReadServerIdentity(filepath.Join(in.ReleaseDir, a.Name))
			must(e)
			break
		}
	}
	if identity.SchemaVersion != 1 || identity.Version != verified.Manifest.Server.Version {
		panic("witness release must carry actual unchanged Task1 identity1 server")
	}
	var header struct {
		SchemaVersion int    `json:"schema_version"`
		Release       string `json:"release"`
		Channel       string `json:"channel"`
		Server        struct {
			Version string `json:"version"`
		} `json:"server"`
		BundleVersion string `json:"bundle_version"`
		TargetOS      string `json:"target_os"`
		TargetArch    string `json:"target_arch"`
	}
	must(json.Unmarshal(read(filepath.Join(in.DataRoot, "config", "install-state.json")), &header))
	if header.SchemaVersion != 4 {
		panic("real protected current schema4 required")
	}
	options := installtx.PrepareOptions{UserHome: in.UserHome, Transition: "update", ConfirmationPlanID: strings.Repeat("a", 32), FromRelease: header.Release, FromChannel: header.Channel, FromCLIVersion: header.Server.Version, FromBundleVersion: header.BundleVersion, TargetOS: header.TargetOS, TargetArch: header.TargetArch, ManagedRoot: in.ManagedRoot, DataRoot: in.DataRoot}
	planPath, e := installtx.Prepare(verified, options)
	must(e)
	plan, e := installtx.ReadPlan(planPath)
	must(e)
	if plan.SchemaVersion != 3 {
		panic("unchanged baseline did not prepare genuine modern3 plan")
	}
	backup := filepath.Join(in.ManagedRoot, "recovery", "cli", plan.FromCLIVersion+"-"+plan.TransactionID, filepath.Base(plan.StagedCLI))
	receipt := installtx.ActivationReceipt{Profile: plan.Profile, Server: plan.Server, SchemaVersion: 2, ConfirmationPlanID: plan.ConfirmationPlanID, TransactionID: plan.TransactionID, Release: plan.Release, Channel: plan.Channel, CLIVersion: plan.CLIVersion, BundleVersion: plan.BundleVersion, TargetOS: plan.TargetOS, TargetArch: plan.TargetArch, ManagedRoot: plan.ManagedRoot, DataRoot: plan.DataRoot, ManifestSHA256: plan.ManifestSHA256, CLISHA256: plan.CLISHA256, CLISize: plan.CLISize, BundleSHA256: plan.BundleSHA256, BundleSize: plan.BundleSize, RoleMigrationID: plan.RoleMigrationID, CatalogSHA256: plan.CatalogSHA256, PolicySHA256: plan.PolicySHA256, SourceCLIBackup: backup, CommittedAt: time.Now().UTC()}
	receiptBody, e := json.MarshalIndent(receipt, "", "  ")
	must(e)
	must(os.WriteFile(filepath.Join(filepath.Dir(planPath), installtx.ReceiptName), receiptBody, 0600))
	beforeManaged, beforeOwner := inventory(in.ManagedRoot), inventory(in.DataRoot)
	results := map[string]string{}
	check := func(name string, operation func() error) {
		err := operation()
		if err == nil {
			panic(name + " unexpectedly accepted protected source state")
		}
		if !strings.Contains(err.Error(), "unknown field") && !strings.Contains(err.Error(), "unsupported install-state") && !strings.Contains(err.Error(), "invalid install state") {
			panic(name + " did not reach the actual protected-state fence: " + err.Error())
		}
		results[name] = err.Error()
		if !reflect.DeepEqual(beforeManaged, inventory(in.ManagedRoot)) || !reflect.DeepEqual(beforeOwner, inventory(in.DataRoot)) {
			panic(name + " changed protected managed/owner bytes or modes")
		}
	}
	check("public_reader", func() error { _, err := installtx.ReadStateForManagedRoot(in.DataRoot, in.ManagedRoot); return err })
	check("public_activation", func() error {
		return installtx.Activate(planPath, verified, installtx.ActivateOptions{PrepareOptions: options})
	})
	check("public_reconciliation_review", func() error { _, err := installtx.ValidateReconciliation(planPath, options); return err })
	check("public_reconciliation_apply", func() error {
		_, err := installtx.ReconcileActivated(planPath, installtx.ActivateOptions{PrepareOptions: options})
		return err
	})
	check("public_rollback", func() error { return installtx.Rollback(in.ManagedRoot, in.DataRoot, nil) })
	seedSHA := hash(read(in.Installer))
	native := func(_ context.Context, path string) error {
		if hash(read(path)) != seedSHA {
			return errors.New("engineering original real seed changed")
		}
		return nil
	}
	check("public_installer_install", func() error {
		options := installer.Options{UserHome: in.UserHome, ReleaseDir: in.ReleaseDir, Bootstrapper: in.Installer, AuthorityRegistry: in.Registry, ManagedRoot: in.ManagedRoot, DataRoot: in.DataRoot, TargetOS: header.TargetOS, TargetArch: header.TargetArch, NativeTrustMode: installer.NativeTrustStrict, VerifyNative: native}
		review, _, err := installer.Prepare(options)
		if err != nil {
			panic("genuine old first-install review did not complete: " + err.Error())
		}
		options.ExpectedPlanDigest = review.PlanDigest
		_, err = installer.Install(context.Background(), options)
		return err
	})
	for p, before := range beforeSource {
		if !reflect.DeepEqual(before, inventory(filepath.Join(in.Baseline, p))) {
			panic("unchanged baseline source mutated")
		}
	}
	for p, before := range mods {
		if hash(read(filepath.Join(in.Baseline, p))) != before {
			panic("baseline module input mutated")
		}
	}
	report := map[string]any{"baseline": in.Baseline, "baseline_identity": "root-attested 8c1ede63fef4b3a5a17d2d4471e4dc5b4f8c37fe", "actual_old_identity": identity, "actual_prepared_schema": plan.SchemaVersion, "actual_receipt_schema": receipt.SchemaVersion, "public_refusals": results, "full_managed_before_and_after": beforeManaged, "full_owner_before_and_after": beforeOwner, "baseline_source_before_and_after": beforeSource, "baseline_modules_before_and_after": mods, "engineering_native_scope": "controlled constructor verifier compares original actual native seed bytes; no production trust override", "raw_old_WriteState": "explicitly not invoked or claimed fenced"}
	body, e := json.MarshalIndent(report, "", "  ")
	must(e)
	must(os.WriteFile(in.Output, append(body, '\n'), 0600))
	fmt.Println("actual unchanged Task1 public reader/install/activation/reconciliation/rollback refused protected schema4; full managed/owner and baseline inputs unchanged")
}
