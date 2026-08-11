package cli

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/atlas"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/atlasops"
)

// repeatedValue collects a flag the caller may pass more than once, which is
// how collect names several pages and how a grant names several operations.
type repeatedValue []string

func (values *repeatedValue) String() string { return strings.Join(*values, ",") }

func (values *repeatedValue) Set(value string) error {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fmt.Errorf("value must not be empty")
	}
	*values = append(*values, trimmed)
	return nil
}

const maximumOwnerBodyBytes = 256 << 10

func runAtlasOwner(args []string, in io.Reader, out, errOut io.Writer, dataRoot func() (string, error)) int {
	if len(args) == 0 {
		fmt.Fprintln(errOut, "usage: bcgos atlas owner <init|collect|create-page|append-entry>")
		return ExitUsage
	}
	engine, code := openOwnerEngine(dataRoot, errOut)
	if code != ExitOK {
		return code
	}
	switch args[0] {
	case "init":
		return runAtlasOwnerInit(args[1:], out, errOut, dataRoot)
	case "collect":
		return runAtlasOwnerCollect(args[1:], out, errOut, engine)
	case "create-page":
		return runAtlasOwnerCreatePage(args[1:], in, out, errOut, engine)
	case "append-entry":
		return runAtlasOwnerAppendEntry(args[1:], in, out, errOut, engine)
	case "set-field":
		return runAtlasOwnerSetField(args[1:], in, out, errOut, engine)
	case "link":
		return runAtlasOwnerLink(args[1:], out, errOut, engine)
	}
	fmt.Fprintln(errOut, "usage: bcgos atlas owner <init|collect|create-page|append-entry|set-field|link>")
	return ExitUsage
}

func runAtlasOwnerSetField(args []string, in io.Reader, out, errOut io.Writer, engine *atlasops.Engine) int {
	flags := newFlagSet("atlas owner set-field", errOut)
	page := flags.String("page", "", "page path relative to the owner root")
	field := flags.String("field", "", "declared field name to set")
	expect := flags.String("expect-revision", "", "refuse the write if the page moved since this revision")
	stdin := flags.Bool("stdin", false, "read the field value from standard input")
	provenanceFlags := addProvenanceFlags(flags)
	if err := flags.Parse(args); err != nil {
		return ExitUsage
	}
	if rejectPositionals(flags, errOut) {
		return ExitUsage
	}
	if strings.TrimSpace(*page) == "" || strings.TrimSpace(*field) == "" {
		fmt.Fprintln(errOut, "--page and --field are required")
		return ExitUsage
	}
	provenance, code := provenanceFlags.resolve(errOut)
	if code != ExitOK {
		return code
	}
	// The value travels through standard input for the same reason a page body
	// does: a field can hold professional content, and process arguments are
	// visible to anything that can list processes.
	value, code := readOwnerBody(in, *stdin, errOut)
	if code != ExitOK {
		return code
	}
	result, err := engine.SetField(atlasops.SetFieldRequest{
		Page:             *page,
		Field:            *field,
		Value:            strings.TrimSpace(value),
		ExpectedRevision: *expect,
		Provenance:       provenance,
	})
	if err != nil {
		return reportError(errOut, err)
	}
	return writeJSON(out, result, errOut)
}

func runAtlasOwnerLink(args []string, out, errOut io.Writer, engine *atlasops.Engine) int {
	flags := newFlagSet("atlas owner link", errOut)
	page := flags.String("page", "", "page path relative to the owner root")
	section := flags.String("section", "", "Markdown heading the reference belongs under")
	target := flags.String("target", "", "page being referenced, relative to the owner root")
	label := flags.String("label", "", "link text")
	expect := flags.String("expect-revision", "", "refuse the write if the page moved since this revision")
	provenanceFlags := addProvenanceFlags(flags)
	if err := flags.Parse(args); err != nil {
		return ExitUsage
	}
	if rejectPositionals(flags, errOut) {
		return ExitUsage
	}
	if strings.TrimSpace(*page) == "" || strings.TrimSpace(*section) == "" || strings.TrimSpace(*target) == "" || strings.TrimSpace(*label) == "" {
		fmt.Fprintln(errOut, "--page, --section, --target and --label are required")
		return ExitUsage
	}
	provenance, code := provenanceFlags.resolve(errOut)
	if code != ExitOK {
		return code
	}
	// Unlike a page body or a field value, both operands here are structural —
	// a path inside the owner root and the text of a navigation label — so they
	// stay as flags rather than forcing standard input for a one-line edge.
	result, err := engine.Link(atlasops.LinkRequest{
		Page:             *page,
		Section:          *section,
		Target:           *target,
		Label:            *label,
		ExpectedRevision: *expect,
		Provenance:       provenance,
	})
	if err != nil {
		return reportError(errOut, err)
	}
	return writeJSON(out, result, errOut)
}

func openOwnerEngine(dataRoot func() (string, error), errOut io.Writer) (*atlasops.Engine, int) {
	root, err := dataRoot()
	if err != nil {
		return nil, reportError(errOut, err)
	}
	engine, err := atlasops.Open(root, time.Now)
	if err != nil {
		return nil, reportError(errOut, err)
	}
	return engine, ExitOK
}

func runAtlasOwnerInit(args []string, out, errOut io.Writer, dataRoot func() (string, error)) int {
	flags := newFlagSet("atlas owner init", errOut)
	if err := flags.Parse(args); err != nil {
		return ExitUsage
	}
	if rejectPositionals(flags, errOut) {
		return ExitUsage
	}
	root, err := dataRoot()
	if err != nil {
		return reportError(errOut, err)
	}
	// The owner atlas is deliberately reachable without a workspace.
	pointer, err := atlas.InitializeOwner(root)
	if err != nil {
		return reportError(errOut, err)
	}
	return writeJSON(out, pointer, errOut)
}

func runAtlasOwnerCollect(args []string, out, errOut io.Writer, engine *atlasops.Engine) int {
	flags := newFlagSet("atlas owner collect", errOut)
	purpose := flags.String("purpose", "", "why this content is being read")
	reader := flags.String("reader", string(atlasops.ReaderOwnerSession), "owner_session, maestro, walter or delegate")
	authorized := flags.Bool("authorized", false, "the owner authorized this delegated projection")
	var pages repeatedValue
	flags.Var(&pages, "page", "page to project, relative to the owner root; repeatable")
	if err := flags.Parse(args); err != nil {
		return ExitUsage
	}
	if rejectPositionals(flags, errOut) {
		return ExitUsage
	}
	if strings.TrimSpace(*purpose) == "" {
		fmt.Fprintln(errOut, "--purpose is required; a projection must say why it is being read")
		return ExitUsage
	}
	if len(pages) == 0 {
		fmt.Fprintln(errOut, "--page is required at least once; there is no whole-root projection")
		return ExitUsage
	}
	projection, err := engine.Collect(atlasops.CollectRequest{
		Purpose:    *purpose,
		Reader:     atlasops.Reader(*reader),
		Pages:      pages,
		Authorized: *authorized,
	})
	if err != nil {
		return reportError(errOut, err)
	}
	return writeJSON(out, projection, errOut)
}

func runAtlasOwnerCreatePage(args []string, in io.Reader, out, errOut io.Writer, engine *atlasops.Engine) int {
	flags := newFlagSet("atlas owner create-page", errOut)
	page := flags.String("page", "", "page path relative to the owner root")
	stdin := flags.Bool("stdin", false, "read the page body from standard input")
	provenanceFlags := addProvenanceFlags(flags)
	if err := flags.Parse(args); err != nil {
		return ExitUsage
	}
	if rejectPositionals(flags, errOut) {
		return ExitUsage
	}
	if strings.TrimSpace(*page) == "" {
		fmt.Fprintln(errOut, "--page is required")
		return ExitUsage
	}
	provenance, code := provenanceFlags.resolve(errOut)
	if code != ExitOK {
		return code
	}
	body, code := readOwnerBody(in, *stdin, errOut)
	if code != ExitOK {
		return code
	}
	result, err := engine.CreatePage(atlasops.CreatePageRequest{
		Page:       *page,
		Body:       body,
		Provenance: provenance,
	})
	if err != nil {
		return reportError(errOut, err)
	}
	return writeJSON(out, result, errOut)
}

func runAtlasOwnerAppendEntry(args []string, in io.Reader, out, errOut io.Writer, engine *atlasops.Engine) int {
	flags := newFlagSet("atlas owner append-entry", errOut)
	page := flags.String("page", "", "page path relative to the owner root")
	section := flags.String("section", "", "Markdown heading the entry belongs under")
	expect := flags.String("expect-revision", "", "refuse the write if the page moved since this revision")
	stdin := flags.Bool("stdin", false, "read the entry from standard input")
	provenanceFlags := addProvenanceFlags(flags)
	if err := flags.Parse(args); err != nil {
		return ExitUsage
	}
	if rejectPositionals(flags, errOut) {
		return ExitUsage
	}
	if strings.TrimSpace(*page) == "" || strings.TrimSpace(*section) == "" {
		fmt.Fprintln(errOut, "--page and --section are required")
		return ExitUsage
	}
	provenance, code := provenanceFlags.resolve(errOut)
	if code != ExitOK {
		return code
	}
	entry, code := readOwnerBody(in, *stdin, errOut)
	if code != ExitOK {
		return code
	}
	result, err := engine.AppendEntry(atlasops.AppendEntryRequest{
		Page:             *page,
		Section:          *section,
		Entry:            entry,
		ExpectedRevision: *expect,
		Provenance:       provenance,
	})
	if err != nil {
		return reportError(errOut, err)
	}
	return writeJSON(out, result, errOut)
}

// readOwnerBody keeps professional content out of process arguments, which is
// the same rule memory capture already enforces.
func readOwnerBody(in io.Reader, stdin bool, errOut io.Writer) (string, int) {
	if !stdin {
		fmt.Fprintln(errOut, "--stdin is required; professional content must not be passed in process arguments")
		return "", ExitUsage
	}
	body, err := io.ReadAll(io.LimitReader(in, maximumOwnerBodyBytes+1))
	if err != nil {
		return "", reportError(errOut, err)
	}
	if len(body) > maximumOwnerBodyBytes {
		fmt.Fprintf(errOut, "input exceeds the bounded %d bytes\n", maximumOwnerBodyBytes)
		return "", ExitUsage
	}
	return string(body), ExitOK
}

type provenanceFlagSet struct {
	key        *string
	session    *string
	grant      *string
	occurrence *string
}

func addProvenanceFlags(flags interface {
	String(string, string, string) *string
}) provenanceFlagSet {
	return provenanceFlagSet{
		key:        flags.String("key", "", "idempotency key for this write"),
		session:    flags.String("session", "", "session identity for an attended write"),
		grant:      flags.String("grant", "", "standing grant authorizing this write"),
		occurrence: flags.String("occurrence", "", "occurrence identity under the standing grant"),
	}
}

// resolve turns the flags into provenance, refusing a write that cannot say
// under what authority it happened.
func (set provenanceFlagSet) resolve(errOut io.Writer) (atlasops.Provenance, int) {
	if strings.TrimSpace(*set.key) == "" {
		fmt.Fprintln(errOut, "--key is required; a write must be idempotent under a stable key")
		return atlasops.Provenance{}, ExitUsage
	}
	attended := strings.TrimSpace(*set.session) != ""
	granted := strings.TrimSpace(*set.grant) != ""
	if attended == granted {
		fmt.Fprintln(errOut, "exactly one of --session or --grant is required; a write states the authority it acted under")
		return atlasops.Provenance{}, ExitUsage
	}
	if granted {
		return atlasops.Provenance{
			Origin:         atlasops.OriginGrant,
			GrantID:        *set.grant,
			OccurrenceID:   *set.occurrence,
			IdempotencyKey: *set.key,
		}, ExitOK
	}
	return atlasops.Provenance{
		Origin:         atlasops.OriginAttended,
		SessionID:      *set.session,
		IdempotencyKey: *set.key,
	}, ExitOK
}

func runAtlasGrant(args []string, out, errOut io.Writer, dataRoot func() (string, error)) int {
	if len(args) == 0 {
		fmt.Fprintln(errOut, "usage: bcgos atlas grant <create|list|pause|resume|revoke>")
		return ExitUsage
	}
	engine, code := openOwnerEngine(dataRoot, errOut)
	if code != ExitOK {
		return code
	}
	switch args[0] {
	case "create":
		return runAtlasGrantCreate(args[1:], out, errOut, engine)
	case "list":
		return runAtlasGrantList(args[1:], out, errOut, engine)
	case "pause", "resume", "revoke":
		return runAtlasGrantTransition(args[0], args[1:], out, errOut, engine)
	}
	fmt.Fprintln(errOut, "usage: bcgos atlas grant <create|list|pause|resume|revoke>")
	return ExitUsage
}

func runAtlasGrantCreate(args []string, out, errOut io.Writer, engine *atlasops.Engine) int {
	flags := newFlagSet("atlas grant create", errOut)
	ritual := flags.String("ritual", "", "ritual this grant authorizes")
	version := flags.String("ritual-version", "", "version of the ritual contract")
	segment := flags.String("segment", "", "page family the grant covers")
	cadence := flags.String("cadence", "", "how often the ritual runs")
	catchUp := flags.String("catch-up", atlasops.CatchUpSkip, "skip or single")
	reader := flags.String("reader", string(atlasops.ReaderOwnerSession), "reader tier for the ritual's projections")
	retention := flags.String("retention", "owner_managed", "retention policy for what the ritual writes")
	expires := flags.String("expires", "", "RFC3339 instant after which the grant stops authorizing")
	var operations repeatedValue
	flags.Var(&operations, "operation", "operation the grant allows; repeatable")
	if err := flags.Parse(args); err != nil {
		return ExitUsage
	}
	if rejectPositionals(flags, errOut) {
		return ExitUsage
	}
	request := atlasops.GrantRequest{
		Ritual:        *ritual,
		RitualVersion: *version,
		Segment:       *segment,
		Operations:    operations,
		Cadence:       *cadence,
		CatchUp:       *catchUp,
		Reader:        atlasops.Reader(*reader),
		Retention:     *retention,
	}
	if strings.TrimSpace(*expires) != "" {
		instant, err := time.Parse(time.RFC3339, *expires)
		if err != nil {
			fmt.Fprintln(errOut, "--expires must be an RFC3339 instant")
			return ExitUsage
		}
		request.ExpiresAt = &instant
	}
	grant, err := engine.CreateGrant(request)
	if err != nil {
		return reportError(errOut, err)
	}
	return writeJSON(out, grantView(grant, time.Now()), errOut)
}

func runAtlasGrantList(args []string, out, errOut io.Writer, engine *atlasops.Engine) int {
	flags := newFlagSet("atlas grant list", errOut)
	if err := flags.Parse(args); err != nil {
		return ExitUsage
	}
	if rejectPositionals(flags, errOut) {
		return ExitUsage
	}
	grants, err := engine.Grants()
	if err != nil {
		return reportError(errOut, err)
	}
	now := time.Now()
	views := make([]map[string]any, 0, len(grants))
	for _, grant := range grants {
		views = append(views, grantView(grant, now))
	}
	return writeJSON(out, map[string]any{"grants": views}, errOut)
}

func runAtlasGrantTransition(action string, args []string, out, errOut io.Writer, engine *atlasops.Engine) int {
	flags := newFlagSet("atlas grant "+action, errOut)
	grantID := flags.String("grant", "", "grant identity")
	if err := flags.Parse(args); err != nil {
		return ExitUsage
	}
	if rejectPositionals(flags, errOut) {
		return ExitUsage
	}
	if strings.TrimSpace(*grantID) == "" {
		fmt.Fprintln(errOut, "--grant is required")
		return ExitUsage
	}
	var err error
	switch action {
	case "pause":
		err = engine.PauseGrant(*grantID)
	case "resume":
		err = engine.ResumeGrant(*grantID)
	case "revoke":
		err = engine.RevokeGrant(*grantID)
	}
	if err != nil {
		return reportError(errOut, err)
	}
	grant, found, err := engine.Grant(*grantID)
	if err != nil {
		return reportError(errOut, err)
	}
	if !found {
		return reportError(errOut, fmt.Errorf("standing grant %q does not exist", *grantID))
	}
	return writeJSON(out, grantView(grant, time.Now()), errOut)
}

// grantView resolves the state at the current instant, since expiry is not
// written down anywhere and only exists relative to a clock.
func grantView(grant atlasops.Grant, now time.Time) map[string]any {
	return map[string]any{
		"grant":     grant,
		"state":     grant.StateAt(now),
		"scheduled": grant.Scheduled,
	}
}
