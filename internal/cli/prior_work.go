package cli

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
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
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/priorworksync"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspace"
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
		fmt.Fprintln(errOut, "usage: bcgos prior-work <actor|source|rationale|enroll|status|import|find|sync-due>")
		return ExitUsage
	}
	switch args[0] {
	case "help", "--help", "-h":
		fmt.Fprintln(out, "usage: bcgos prior-work <actor|source|rationale|enroll|status|import|find|sync-due>")
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
	case "source":
		return runPriorWorkSource(args[1:], in, out, errOut, dataRoot)
	case "rationale":
		return runPriorWorkRationale(args[1:], in, out, errOut, dataRoot)
	case "status":
		return runPriorWorkStatus(args[1:], out, errOut, dataRoot)
	case "import":
		return runPriorWorkImport(args[1:], out, errOut, dataRoot)
	case "find":
		return runPriorWorkFind(args[1:], in, out, errOut, dataRoot)
	case "sync-due":
		return runPriorWorkSyncDue(args[1:], out, errOut, dataRoot)
	default:
		fmt.Fprintf(errOut, "unknown prior-work command %q\n", args[0])
		return ExitUsage
	}
}

func runPriorWorkRationale(
	args []string,
	in io.Reader,
	out io.Writer,
	errOut io.Writer,
	dataRoot func() (string, error),
) int {
	if len(args) == 0 || args[0] != "ingest" {
		fmt.Fprintln(errOut, "usage: bcgos prior-work rationale ingest --workspace PATH --stdin --confirm")
		return ExitUsage
	}
	flags := flag.NewFlagSet("prior-work rationale ingest", flag.ContinueOnError)
	flags.SetOutput(errOut)
	workspacePath := flags.String("workspace", "", "initialized Maestro workspace path")
	stdin := flags.Bool("stdin", false, "read a signed, bounded rationale batch from standard input")
	confirm := flags.Bool("confirm", false, "confirm the reviewed local rationale ingestion")
	if err := flags.Parse(args[1:]); err != nil {
		return ExitUsage
	}
	if flags.NArg() != 0 || strings.TrimSpace(*workspacePath) == "" || !*stdin || !*confirm {
		fmt.Fprintln(errOut, "usage: bcgos prior-work rationale ingest --workspace PATH --stdin --confirm")
		return ExitUsage
	}
	root, err := dataRoot()
	if err != nil {
		return writePriorWorkError(errOut, err)
	}
	inspection, err := workspace.Inspect(*workspacePath, root)
	if err != nil {
		return writePriorWorkError(errOut, err)
	}
	if inspection.WorkspaceID == "" || (inspection.State != "ready" && inspection.State != "warning") {
		return writePriorWorkError(errOut, errors.New("rationale ingestion requires an initialized Maestro workspace"))
	}
	selectionStore := priorWorkSourceSelectionStore(root)
	status, err := selectionStore.Status(inspection.WorkspaceID)
	if err != nil {
		return writePriorWorkError(errOut, err)
	}
	if status.State != priorwork.SourceSelected {
		return writePriorWorkError(errOut, errors.New("rationale ingestion requires an explicitly selected SharePoint source"))
	}
	batch, err := priorwork.ParseRationaleBatch(in)
	if err != nil {
		return writePriorWorkError(errOut, err)
	}
	if batch.WorkspaceID != inspection.WorkspaceID || batch.SourceSelectionFingerprint != status.Fingerprint {
		return writePriorWorkError(errOut, errors.New("rationale batch does not bind the selected Maestro workspace and SharePoint folders"))
	}
	folders, err := selectionStore.SelectedFolders(inspection.WorkspaceID)
	if err != nil {
		return writePriorWorkError(errOut, err)
	}
	priorStore, err := priorWorkStore(func() (string, error) { return root, nil })
	if err != nil {
		fmt.Fprintf(errOut, "prior-work rationale: collector trust is unavailable: %v\n", err)
		return ExitUnavailable
	}
	enrollment, err := priorStore.Enrollment()
	if err != nil {
		fmt.Fprintf(errOut, "prior-work rationale: signed Claude collection is unavailable: %v\n", err)
		return ExitUnavailable
	}
	report, err := priorwork.MaterializeRationales(*workspacePath, batch, folders, enrollment)
	if err != nil {
		return writePriorWorkError(errOut, err)
	}
	return writePriorWorkJSON(out, report)
}

func runPriorWorkSource(
	args []string,
	in io.Reader,
	out io.Writer,
	errOut io.Writer,
	dataRoot func() (string, error),
) int {
	if len(args) == 0 || (args[0] != "status" && args[0] != "select" && args[0] != "defer") {
		fmt.Fprintln(errOut, "usage: bcgos prior-work source <status|select|defer> --workspace PATH")
		return ExitUsage
	}
	action := args[0]
	flags := flag.NewFlagSet("prior-work source "+action, flag.ContinueOnError)
	flags.SetOutput(errOut)
	workspacePath := flags.String("workspace", "", "initialized Maestro workspace path")
	stdin := flags.Bool("stdin", false, "read exact SharePoint folder selection from standard input")
	confirm := flags.Bool("confirm", false, "confirm the reviewed source choice")
	if err := flags.Parse(args[1:]); err != nil {
		return ExitUsage
	}
	if flags.NArg() != 0 || strings.TrimSpace(*workspacePath) == "" {
		fmt.Fprintln(errOut, priorWorkSourceUsage(action))
		return ExitUsage
	}
	switch action {
	case "status":
		if *stdin || *confirm {
			fmt.Fprintln(errOut, priorWorkSourceUsage(action))
			return ExitUsage
		}
	case "select":
		if !*stdin || !*confirm {
			fmt.Fprintln(errOut, priorWorkSourceUsage(action))
			return ExitUsage
		}
	case "defer":
		if *stdin || !*confirm {
			fmt.Fprintln(errOut, priorWorkSourceUsage(action))
			return ExitUsage
		}
	}
	root, err := dataRoot()
	if err != nil {
		return writePriorWorkError(errOut, err)
	}
	inspection, err := workspace.Inspect(*workspacePath, root)
	if err != nil {
		return writePriorWorkError(errOut, err)
	}
	if inspection.WorkspaceID == "" || (inspection.State != "ready" && inspection.State != "warning") {
		return writePriorWorkError(errOut, errors.New("guided SharePoint source selection requires an initialized Maestro workspace"))
	}
	store := priorWorkSourceSelectionStore(root)
	var status priorwork.SourceSelectionStatus
	switch action {
	case "status":
		status, err = store.Status(inspection.WorkspaceID)
	case "defer":
		if err = ensurePriorWorkEnrollmentParent(store.Root); err != nil {
			break
		}
		status, err = store.Defer(inspection.WorkspaceID)
	case "select":
		var input priorwork.SourceSelectionInput
		input, err = priorwork.ParseSourceSelectionInput(in)
		if err == nil {
			err = ensurePriorWorkEnrollmentParent(store.Root)
		}
		if err == nil {
			status, err = store.Select(inspection.WorkspaceID, input.FolderURLs)
		}
	}
	if err != nil {
		return writePriorWorkError(errOut, err)
	}
	return writePriorWorkJSON(out, status)
}

func priorWorkSourceUsage(action string) string {
	switch action {
	case "status":
		return "usage: bcgos prior-work source status --workspace PATH"
	case "select":
		return "usage: bcgos prior-work source select --workspace PATH --stdin --confirm"
	case "defer":
		return "usage: bcgos prior-work source defer --workspace PATH --confirm"
	default:
		return "usage: bcgos prior-work source <status|select|defer> --workspace PATH"
	}
}

func runPriorWorkSyncDue(
	args []string,
	out io.Writer,
	errOut io.Writer,
	dataRoot func() (string, error),
) int {
	flags := flag.NewFlagSet("prior-work sync-due", flag.ContinueOnError)
	flags.SetOutput(errOut)
	runtime := flags.String("runtime", "", "active runtime: claude or codex")
	purpose := flags.String("purpose", "prior_work_retrieval", "authorized retrieval purpose")
	if err := flags.Parse(args); err != nil {
		return ExitUsage
	}
	if flags.NArg() != 0 || (*runtime != "claude" && *runtime != "codex") {
		fmt.Fprintln(errOut, "usage: bcgos prior-work sync-due --runtime <claude|codex>")
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
	policy, err := store.SchedulePolicy(access)
	if err != nil {
		return writePriorWorkError(errOut, err)
	}
	root, err := dataRoot()
	if err != nil {
		return writePriorWorkError(errOut, err)
	}
	service := priorworksync.Service{
		Store: scheduler.Store{Root: filepath.Join(root, "scheduler")},
	}
	report, err := service.RunPresence(context.Background(), *runtime, policy)
	if err != nil {
		return writePriorWorkError(errOut, err)
	}
	if code := writePriorWorkJSON(out, report); code != ExitOK {
		return code
	}
	for _, receipt := range report.Receipts {
		if receipt.State == scheduler.Unavailable {
			return ExitUnavailable
		}
		if receipt.State == scheduler.Failed {
			return ExitFailure
		}
	}
	return ExitOK
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
	keyID := strings.TrimSpace(os.Getenv("BCGOS_PRIOR_WORK_AUTHORITY_KEY_ID"))
	encodedKey := strings.TrimSpace(os.Getenv("BCGOS_PRIOR_WORK_AUTHORITY_PUBLIC_KEY"))
	publicKey, decodeErr := base64.StdEncoding.Strict().DecodeString(encodedKey)
	if keyID == "" || decodeErr != nil || len(publicKey) != ed25519.PublicKeySize {
		return priorwork.Store{}, errors.New("prior-work enrollment authority trust anchor is unavailable")
	}
	return priorwork.Store{
		Root:                     priorWorkRoot(root),
		EnrollmentAuthorityKeyID: keyID,
		EnrollmentAuthority:      publicKey,
	}, nil
}

func priorWorkRoot(dataRoot string) string {
	return filepath.Join(dataRoot, "atlases", "organization", "sharepoint-work")
}

func priorWorkSourceSelectionStore(dataRoot string) priorwork.SourceSelectionStore {
	return priorwork.SourceSelectionStore{Root: priorWorkRoot(dataRoot)}
}

func priorWorkSourceStatus(dataRoot, workspaceID string) (priorwork.SourceSelectionStatus, error) {
	if strings.TrimSpace(workspaceID) == "" {
		return priorwork.SourceSelectionStatus{
			SchemaVersion: 1, State: priorwork.SourceSelectionUnavailable,
			SourceAuthority: "sharepoint", LocalProjection: "metadata_and_source_pointers_only",
			AuthorizationState: "unavailable", CollectionRuntime: "claude",
			CollectionState: "unavailable", CodexCollectionState: "unavailable/corporate_policy",
		}, nil
	}
	return priorWorkSourceSelectionStore(dataRoot).Status(workspaceID)
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
