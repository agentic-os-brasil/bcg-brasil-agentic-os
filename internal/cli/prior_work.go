package cli

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"strings"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/priorwork"
)

const maximumPriorWorkQueryBytes = 64 << 10

func runPriorWork(
	args []string,
	in io.Reader,
	out io.Writer,
	errOut io.Writer,
	dataRoot func() (string, error),
) int {
	if len(args) == 0 {
		fmt.Fprintln(errOut, "usage: bcgos prior-work <actor|enroll|status|import|find>")
		return ExitUsage
	}
	switch args[0] {
	case "help", "--help", "-h":
		fmt.Fprintln(out, "usage: bcgos prior-work <actor|enroll|status|import|find>")
		return ExitOK
	case "actor":
		if len(args) != 1 {
			fmt.Fprintln(errOut, "usage: bcgos prior-work actor")
			return ExitUsage
		}
		actor, err := localPriorWorkActorRef()
		if err != nil {
			return writePriorWorkError(errOut, err)
		}
		return writePriorWorkJSON(out, struct {
			SchemaVersion int    `json:"schema_version"`
			ActorRef      string `json:"actor_ref"`
			Binding       string `json:"binding"`
		}{SchemaVersion: 1, ActorRef: actor, Binding: "local_os_principal"})
	case "enroll":
		return runPriorWorkEnroll(args[1:], in, out, errOut, dataRoot)
	case "status":
		return runPriorWorkStatus(args[1:], out, errOut, dataRoot)
	case "import":
		return runPriorWorkImport(args[1:], out, errOut, dataRoot)
	case "find":
		return runPriorWorkFind(args[1:], in, out, errOut, dataRoot)
	default:
		fmt.Fprintf(errOut, "unknown prior-work command %q\n", args[0])
		return ExitUsage
	}
}

func runPriorWorkEnroll(
	args []string,
	in io.Reader,
	out io.Writer,
	errOut io.Writer,
	dataRoot func() (string, error),
) int {
	flags := flag.NewFlagSet("prior-work enroll", flag.ContinueOnError)
	flags.SetOutput(errOut)
	stdin := flags.Bool("stdin", false, "read enrollment JSON from standard input")
	confirm := flags.Bool("confirm", false, "confirm the bounded SharePoint enrollment")
	if err := flags.Parse(args); err != nil {
		return ExitUsage
	}
	if flags.NArg() != 0 || !*stdin || !*confirm {
		fmt.Fprintln(errOut, "usage: bcgos prior-work enroll --stdin --confirm")
		return ExitUsage
	}
	enrollment, err := priorwork.ParseEnrollment(in)
	if err != nil {
		return writePriorWorkError(errOut, err)
	}
	store, err := priorWorkStore(dataRoot)
	if err != nil {
		return writePriorWorkError(errOut, err)
	}
	if err := ensurePriorWorkEnrollmentParent(store.Root); err != nil {
		return writePriorWorkError(errOut, err)
	}
	actor, err := localPriorWorkActorRef()
	if err != nil {
		return writePriorWorkError(errOut, err)
	}
	if enrollment.AuthorizedBy != actor {
		return writePriorWorkError(errOut, errors.New("enrollment actor does not match the authenticated local OS principal"))
	}
	if err := store.Enroll(enrollment); err != nil {
		return writePriorWorkError(errOut, err)
	}
	return writePriorWorkJSON(out, struct {
		SchemaVersion int    `json:"schema_version"`
		State         string `json:"state"`
		TenantRef     string `json:"tenant_ref"`
		PolicyVersion string `json:"policy_version"`
		Roots         int    `json:"roots"`
	}{
		SchemaVersion: 1, State: "enrolled", TenantRef: enrollment.TenantRef,
		PolicyVersion: enrollment.PolicyVersion, Roots: len(enrollment.Roots),
	})
}

func runPriorWorkStatus(
	args []string,
	out io.Writer,
	errOut io.Writer,
	dataRoot func() (string, error),
) int {
	flags := flag.NewFlagSet("prior-work status", flag.ContinueOnError)
	flags.SetOutput(errOut)
	purpose := flags.String("purpose", "prior_work_retrieval", "authorized retrieval purpose")
	if err := flags.Parse(args); err != nil {
		return ExitUsage
	}
	if flags.NArg() != 0 {
		fmt.Fprintln(errOut, "usage: bcgos prior-work status [--purpose prior_work_retrieval]")
		return ExitUsage
	}
	store, err := priorWorkStore(dataRoot)
	if err != nil {
		return writePriorWorkError(errOut, err)
	}
	access, err := localPriorWorkAccess(*purpose)
	if err != nil {
		return writePriorWorkError(errOut, err)
	}
	status, err := store.Status(access)
	if err != nil {
		return writePriorWorkError(errOut, err)
	}
	return writePriorWorkJSON(out, status)
}

func runPriorWorkImport(
	args []string,
	out io.Writer,
	errOut io.Writer,
	dataRoot func() (string, error),
) int {
	flags := flag.NewFlagSet("prior-work import", flag.ContinueOnError)
	flags.SetOutput(errOut)
	snapshotPath := flags.String("snapshot", "", "normalized snapshot JSON")
	receiptPath := flags.String("receipt", "", "externally signed collector receipt JSON")
	purpose := flags.String("purpose", "prior_work_retrieval", "authorized retrieval purpose")
	if err := flags.Parse(args); err != nil {
		return ExitUsage
	}
	if flags.NArg() != 0 || *snapshotPath == "" || *receiptPath == "" {
		fmt.Fprintln(errOut, "usage: bcgos prior-work import --snapshot <json> --receipt <json>")
		return ExitUsage
	}
	snapshotFile, err := os.Open(*snapshotPath)
	if err != nil {
		return writePriorWorkError(errOut, err)
	}
	snapshot, parseErr := priorwork.ParseSnapshot(snapshotFile)
	closeErr := snapshotFile.Close()
	if parseErr != nil {
		return writePriorWorkError(errOut, parseErr)
	}
	if closeErr != nil {
		return writePriorWorkError(errOut, closeErr)
	}
	receiptFile, err := os.Open(*receiptPath)
	if err != nil {
		return writePriorWorkError(errOut, err)
	}
	receipt, parseErr := priorwork.ParseImportReceipt(receiptFile, snapshot)
	closeErr = receiptFile.Close()
	if parseErr != nil {
		return writePriorWorkError(errOut, parseErr)
	}
	if closeErr != nil {
		return writePriorWorkError(errOut, closeErr)
	}
	store, err := priorWorkStore(dataRoot)
	if err != nil {
		return writePriorWorkError(errOut, err)
	}
	access, err := localPriorWorkAccess(*purpose)
	if err != nil {
		return writePriorWorkError(errOut, err)
	}
	report, err := store.Apply(snapshot, receipt, access)
	if err != nil {
		return writePriorWorkError(errOut, err)
	}
	return writePriorWorkJSON(out, report)
}

func runPriorWorkFind(
	args []string,
	in io.Reader,
	out io.Writer,
	errOut io.Writer,
	dataRoot func() (string, error),
) int {
	flags := flag.NewFlagSet("prior-work find", flag.ContinueOnError)
	flags.SetOutput(errOut)
	stdin := flags.Bool("stdin", false, "read the retrieval request from standard input")
	explicit := flags.Bool("explicit", false, "confirm explicit prior-work retrieval intent")
	limit := flags.Int("limit", 5, "maximum result count")
	purpose := flags.String("purpose", "prior_work_retrieval", "authorized retrieval purpose")
	if err := flags.Parse(args); err != nil {
		return ExitUsage
	}
	if flags.NArg() != 0 || !*stdin || !*explicit {
		fmt.Fprintln(errOut, "usage: bcgos prior-work find --explicit --stdin [--limit <n>]")
		return ExitUsage
	}
	body, err := io.ReadAll(io.LimitReader(in, maximumPriorWorkQueryBytes+1))
	if err != nil {
		return writePriorWorkError(errOut, err)
	}
	if len(body) > maximumPriorWorkQueryBytes {
		return writePriorWorkError(errOut, errors.New("prior-work query exceeds 64 KiB"))
	}
	store, err := priorWorkStore(dataRoot)
	if err != nil {
		return writePriorWorkError(errOut, err)
	}
	access, err := localPriorWorkAccess(*purpose)
	if err != nil {
		return writePriorWorkError(errOut, err)
	}
	response, err := store.Find(priorwork.Query{
		Text: string(body), ExplicitPriorWorkIntent: true, Limit: *limit,
		Access: access,
	})
	if err != nil {
		return writePriorWorkError(errOut, err)
	}
	return writePriorWorkJSON(out, response)
}

func localPriorWorkAccess(purpose string) (priorwork.AccessContext, error) {
	actor, err := localPriorWorkActorRef()
	if err != nil {
		return priorwork.AccessContext{}, err
	}
	return priorwork.AccessContext{ActorRef: actor, Purpose: purpose}, nil
}

func localPriorWorkActorRef() (string, error) {
	current, err := user.Current()
	if err != nil {
		return "", fmt.Errorf("resolve authenticated local OS principal: %w", err)
	}
	if strings.TrimSpace(current.Uid) == "" && strings.TrimSpace(current.Username) == "" {
		return "", errors.New("authenticated local OS principal has no stable identifier")
	}
	sum := sha256.Sum256([]byte("bcgos-local-principal-v1\x00" + current.Uid + "\x00" + current.Username))
	return fmt.Sprintf("actor-local-%x", sum[:16]), nil
}

func priorWorkStore(dataRoot func() (string, error)) (priorwork.Store, error) {
	root, err := dataRoot()
	if err != nil {
		return priorwork.Store{}, err
	}
	if strings.TrimSpace(root) == "" {
		return priorwork.Store{}, errors.New("BCGOS data root is unavailable")
	}
	return priorwork.Store{
		Root: filepath.Join(root, "atlases", "organization", "sharepoint-work"),
	}, nil
}

func ensurePriorWorkEnrollmentParent(storeRoot string) error {
	const relative = "atlases/organization"
	dataRoot := filepath.Dir(filepath.Dir(filepath.Dir(storeRoot)))
	expected := filepath.Join(dataRoot, filepath.FromSlash(relative), "sharepoint-work")
	if expected != storeRoot {
		return errors.New("invalid prior-work storage layout")
	}
	root, err := os.OpenRoot(dataRoot)
	if err != nil {
		return err
	}
	defer root.Close()
	return root.MkdirAll(filepath.FromSlash(relative), 0o700)
}

func writePriorWorkJSON(out io.Writer, value any) int {
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		return ExitFailure
	}
	return ExitOK
}

func writePriorWorkError(errOut io.Writer, err error) int {
	fmt.Fprintf(errOut, "prior-work: %v\n", err)
	return ExitFailure
}
