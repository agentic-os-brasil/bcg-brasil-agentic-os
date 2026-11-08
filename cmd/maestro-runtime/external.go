package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/datahome"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/zipmigration"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/zipruntime"
)

func externalMigration(args []string, out, diagnostic io.Writer) int {
	fail := func(err error) int {
		fmt.Fprintln(diagnostic, "external migration:", err)
		return 2
	}
	if len(args) == 0 {
		return fail(errors.New("explicit plan, apply, status or rollback operation required"))
	}
	op := args[0]
	flags := flag.NewFlagSet("migration external "+op, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var source, product, user, home, attempt, digest, planPath string
	switch op {
	case "plan":
		flags.StringVar(&source, "source", "", "preserved previous ZIP directory")
		flags.StringVar(&product, "product-root", "", "managed product directory")
		flags.StringVar(&user, "user-home", "", "OS user home")
	case "apply":
		flags.StringVar(&planPath, "plan", "", "private canonical plan file")
		flags.StringVar(&digest, "plan-sha256", "", "reviewed exact plan digest")
	case "status", "rollback":
		flags.StringVar(&home, "home", "", "planned HOME")
		flags.StringVar(&attempt, "attempt", "", "sealed migration attempt")
		flags.StringVar(&digest, "plan-sha256", "", "reviewed exact plan digest")
	default:
		return fail(errors.New("unsupported external operation"))
	}
	if flags.Parse(args[1:]) != nil || flags.NArg() != 0 {
		return fail(errors.New("invalid operation arguments"))
	}
	progress := func(p zipmigration.ExternalProgress) error {
		_, err := fmt.Fprintf(diagnostic, "phase=%s copied=%d total=%d\n", p.Phase, p.Copied, p.Total)
		return err
	}
	if op == "plan" {
		if source == "" || product == "" {
			return fail(errors.New("plan requires --source and --product-root"))
		}
		if user == "" {
			var err error
			user, err = os.UserHomeDir()
			if err != nil {
				return fail(errors.New("user home unavailable"))
			}
		}
		selected, err := datahome.Resolve(datahome.Options{UserHome: user, ProductRoot: product, SourceRoot: source, LookupEnv: os.LookupEnv})
		if err != nil {
			return fail(fmt.Errorf("HOME selection refused: %s", selected.Diagnostic))
		}
		plan, err := zipmigration.PlanExternal(zipmigration.ExternalOptions{SourceRoot: source, ProductRoot: product, UserHome: user, Home: selected.Home})
		if err != nil {
			return fail(err)
		}
		body, err := plan.Bytes()
		if err != nil {
			return fail(err)
		}
		sum := sha256.Sum256(body)
		if _, err := fmt.Fprintf(diagnostic, "plan_sha256=%s\n", hex.EncodeToString(sum[:])); err != nil {
			return fail(err)
		}
		if _, err := out.Write(body); err != nil {
			return fail(err)
		}
		return 0
	}
	var result zipmigration.ExternalStatus
	var err error
	if op == "apply" {
		if planPath == "" || digest == "" {
			return fail(errors.New("apply requires --plan and --plan-sha256"))
		}
		body, readErr := scheduler.ReadPrivateFile(planPath, 16<<20)
		if readErr != nil {
			return fail(readErr)
		}
		sum := sha256.Sum256(body)
		if hex.EncodeToString(sum[:]) != digest {
			return fail(errors.New("reviewed plan digest differs from file"))
		}
		var plan zipmigration.ExternalPlan
		decoder := json.NewDecoder(bytes.NewReader(body))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&plan); err != nil {
			return fail(err)
		}
		canonical, encodeErr := plan.Bytes()
		if encodeErr != nil || !bytes.Equal(body, canonical) {
			return fail(errors.New("plan file must contain the exact canonical document"))
		}
		result, err = zipmigration.ApplyExternal(plan, digest, progress)
	} else {
		if home == "" || attempt == "" || digest == "" {
			return fail(errors.New("status/rollback requires --home, --attempt and --plan-sha256"))
		}
		if op == "status" {
			result, err = zipmigration.InspectExternal(home, attempt, digest)
		} else {
			result, err = zipmigration.RollbackExternal(home, attempt, digest, progress)
		}
	}
	bindingPending := false
	if err == nil && op == "rollback" {
		paths := zipruntime.ResolvePaths(result.SourceRoot, zipruntime.Options{UserHome: result.UserHome, LookupEnv: os.LookupEnv})
		bindingPending = !paths.Writable || paths.Mode != "legacy"
		if bindingPending {
			result.Diagnostic = "rolled_back_binding_pending"
			fmt.Fprintln(diagnostic, "filesystem rollback preserved both histories; verify the previous binding and MAESTRO_HOME before reopening the previous runtime")
		}
	}
	if encodeErr := json.NewEncoder(out).Encode(result); encodeErr != nil {
		return fail(encodeErr)
	}
	if err != nil {
		return fail(err)
	}
	if bindingPending {
		return 2
	}
	return 0
}
