// Package atlasops performs the named, bounded operations over the owner
// atlas. A skill never edits a page itself: it asks for an operation here and
// reports what came back, exactly as the canonical memory engine is used.
//
// The owner still owns the corpus. These operations exist for the cases where
// a caller needs idempotency, provenance and conflict detection, not to become
// a gate on the owner's own Markdown.
package atlasops

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/atlas"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

// Terminal states a write can report. A caller must be able to tell a page it
// wrote from a page it only proposed.
const (
	StateWritten   = "written"
	StateUnchanged = "unchanged"
	StateProposed  = "proposed"
)

// Origin records whether a write came from an attended request or from a
// standing grant the owner created for a ritual.
const (
	OriginAttended = "attended"
	OriginGrant    = "standing_grant"
)

const (
	maximumPageBytes  = 256 << 10
	maximumEntryBytes = 8 << 10
	journalDirectory  = ".atlasops"
	schemaVersion     = 1
)

// Provenance answers how a line got onto a page: who asked, under what
// authority, and with which idempotency key.
type Provenance struct {
	Origin         string `json:"origin"`
	SessionID      string `json:"session_id,omitempty"`
	GrantID        string `json:"grant_id,omitempty"`
	Ritual         string `json:"ritual,omitempty"`
	OccurrenceID   string `json:"occurrence_id,omitempty"`
	IdempotencyKey string `json:"idempotency_key"`
}

func (provenance Provenance) validate() error {
	if strings.TrimSpace(provenance.IdempotencyKey) == "" {
		return errors.New("owner atlas write requires an idempotency key")
	}
	switch provenance.Origin {
	case OriginAttended:
		if strings.TrimSpace(provenance.SessionID) == "" {
			return errors.New("attended owner atlas write requires a session")
		}
	case OriginGrant:
		if strings.TrimSpace(provenance.GrantID) == "" || strings.TrimSpace(provenance.OccurrenceID) == "" {
			return errors.New("granted owner atlas write requires a grant and an occurrence")
		}
	default:
		return fmt.Errorf("owner atlas write origin %q is not recognized", provenance.Origin)
	}
	return nil
}

// Result is the terminal outcome of one operation.
type Result struct {
	Operation   string     `json:"operation"`
	Page        string     `json:"page"`
	State       string     `json:"state"`
	Revision    string     `json:"revision,omitempty"`
	OperationID string     `json:"operation_id"`
	RecordedAt  time.Time  `json:"recorded_at"`
	Provenance  Provenance `json:"provenance"`
	// Proposal carries the content the caller would have written when a
	// revision conflict stopped the write. Nothing was persisted.
	Proposal string `json:"proposal,omitempty"`
	Reason   string `json:"reason,omitempty"`
}

type CreatePageRequest struct {
	Page       string
	Body       string
	Provenance Provenance
}

type AppendEntryRequest struct {
	Page    string
	Section string
	Entry   string
	// ExpectedRevision, when set, refuses the write if the page changed
	// underneath the caller and returns a reviewable proposal instead.
	ExpectedRevision string
	Provenance       Provenance
}

// Engine binds the operations to one canonical owner atlas root.
type Engine struct {
	root     string
	dataRoot string
	now      func() time.Time
}

// Open resolves the owner atlas root and makes sure it is reachable through
// the no-follow boundary. It creates no page.
func Open(dataRoot string, now func() time.Time) (*Engine, error) {
	root, err := atlas.OwnerRoot(dataRoot)
	if err != nil {
		return nil, err
	}
	if err := scheduler.EnsurePrivateDirectory(root); err != nil {
		return nil, err
	}
	if now == nil {
		now = time.Now
	}
	return &Engine{root: root, dataRoot: dataRoot, now: now}, nil
}

// authorize enforces a standing grant at the point of effect. A write that
// names a grant is checked against it here rather than relying on the caller
// to remember: otherwise revocation only holds for callers who ask, which is
// not revocation. Attended writes carry the owner's own request and need no
// further authority.
func (engine *Engine) authorize(provenance Provenance, operation, page string) error {
	if provenance.Origin != OriginGrant {
		return nil
	}
	return engine.AuthorizeGrant(provenance.GrantID, operation, page)
}

// journalDirPath addresses machine state kept beside the corpus. Passing an
// empty name yields the directory itself.
func (engine *Engine) journalDirPath(name string) string {
	if name == "" {
		return filepath.Join(engine.root, journalDirectory)
	}
	return filepath.Join(engine.root, journalDirectory, name)
}

// Root is the canonical owner atlas root this engine is bound to.
func (engine *Engine) Root() string { return engine.root }

// CreatePage writes a page if it is absent. An existing page is preserved:
// creation never replaces content, so a repeated bootstrap or a re-run of a
// ritual cannot discard what the owner wrote.
func (engine *Engine) CreatePage(request CreatePageRequest) (Result, error) {
	relative, err := engine.resolve(request.Page)
	if err != nil {
		return Result{}, err
	}
	if err := request.Provenance.validate(); err != nil {
		return Result{}, err
	}
	if len(request.Body) == 0 || len(request.Body) > maximumPageBytes {
		return Result{}, fmt.Errorf("owner atlas page body must be between 1 and %d bytes", maximumPageBytes)
	}
	if err := engine.authorize(request.Provenance, "create-page", relative); err != nil {
		return Result{}, err
	}

	operationID := engine.operationID("create-page", relative, request.Provenance.IdempotencyKey)
	if recorded, found, err := engine.recordedResult(operationID); err != nil {
		return Result{}, err
	} else if found {
		return recorded, nil
	}

	full := engine.pagePath(relative)
	if err := scheduler.EnsurePrivateDirectory(filepath.Dir(full)); err != nil {
		return Result{}, err
	}

	result := Result{
		Operation:   "create-page",
		Page:        relative,
		OperationID: operationID,
		RecordedAt:  engine.now().UTC(),
		Provenance:  request.Provenance,
	}
	writeErr := scheduler.WriteNewPrivateFile(full, []byte(request.Body))
	switch {
	case writeErr == nil:
		result.State = StateWritten
		result.Revision = digest(request.Body)
	case errors.Is(writeErr, os.ErrExist):
		existing, err := engine.read(relative)
		if err != nil {
			return Result{}, err
		}
		result.State = StateUnchanged
		result.Revision = digest(existing)
		result.Reason = "page already exists and was preserved"
	default:
		return Result{}, writeErr
	}
	if err := engine.record(result); err != nil {
		return Result{}, err
	}
	return result, nil
}

// AppendEntry adds one entry under a section the page already declares. It
// never creates the section, never touches another section, and never
// overwrites a page that changed underneath the caller.
func (engine *Engine) AppendEntry(request AppendEntryRequest) (Result, error) {
	relative, err := engine.resolve(request.Page)
	if err != nil {
		return Result{}, err
	}
	if err := request.Provenance.validate(); err != nil {
		return Result{}, err
	}
	section := strings.TrimRight(request.Section, " \t")
	if !strings.HasPrefix(section, "#") {
		return Result{}, errors.New("owner atlas append requires a Markdown heading as its section")
	}
	entry := strings.TrimRight(request.Entry, "\r\n")
	if strings.TrimSpace(entry) == "" || len(entry) > maximumEntryBytes {
		return Result{}, fmt.Errorf("owner atlas entry must be between 1 and %d bytes", maximumEntryBytes)
	}
	if err := engine.authorize(request.Provenance, "append-entry", relative); err != nil {
		return Result{}, err
	}

	operationID := engine.operationID("append-entry", relative, request.Provenance.IdempotencyKey)
	if recorded, found, err := engine.recordedResult(operationID); err != nil {
		return Result{}, err
	} else if found {
		return recorded, nil
	}

	current, err := engine.read(relative)
	if err != nil {
		return Result{}, err
	}
	revision := digest(current)

	updated, err := insertUnderSection(current, section, entry)
	if err != nil {
		return Result{}, err
	}

	result := Result{
		Operation:   "append-entry",
		Page:        relative,
		OperationID: operationID,
		RecordedAt:  engine.now().UTC(),
		Provenance:  request.Provenance,
	}

	if expected := strings.TrimSpace(request.ExpectedRevision); expected != "" && expected != revision {
		// The page moved under the caller. Hand back what would have been
		// written rather than deciding on the owner's behalf.
		result.State = StateProposed
		result.Revision = revision
		result.Proposal = updated
		result.Reason = "page revision changed since it was read; nothing was written"
		if err := engine.record(result); err != nil {
			return Result{}, err
		}
		return result, nil
	}

	if err := scheduler.ReplacePrivateFile(engine.pagePath(relative), []byte(updated)); err != nil {
		return Result{}, err
	}
	result.State = StateWritten
	result.Revision = digest(updated)
	if err := engine.record(result); err != nil {
		return Result{}, err
	}
	return result, nil
}

// insertUnderSection places the entry at the end of the named section, leaving
// every other section untouched. A section the page does not declare is an
// error: the operation set does not invent structure.
func insertUnderSection(body, section, entry string) (string, error) {
	lines := strings.Split(body, "\n")
	start := -1
	for index, line := range lines {
		if strings.TrimRight(line, " \t") == section {
			start = index
			break
		}
	}
	if start < 0 {
		return "", fmt.Errorf("owner atlas page does not declare the section %q", section)
	}
	level := len(section) - len(strings.TrimLeft(section, "#"))
	end := len(lines)
	for index := start + 1; index < len(lines); index++ {
		trimmed := strings.TrimRight(lines[index], " \t")
		if !strings.HasPrefix(trimmed, "#") {
			continue
		}
		if depth := len(trimmed) - len(strings.TrimLeft(trimmed, "#")); depth <= level {
			end = index
			break
		}
	}
	// Step back over the blank lines that separate this section from the next
	// so the entry joins the section's own content.
	insert := end
	for insert > start+1 && strings.TrimSpace(lines[insert-1]) == "" {
		insert--
	}
	updated := make([]string, 0, len(lines)+1)
	updated = append(updated, lines[:insert]...)
	updated = append(updated, entry)
	updated = append(updated, lines[insert:]...)
	return strings.Join(updated, "\n"), nil
}

// resolve rejects anything that is not a plain relative page inside the root.
func (engine *Engine) resolve(page string) (string, error) {
	trimmed := strings.TrimSpace(page)
	if trimmed == "" {
		return "", errors.New("owner atlas page is required")
	}
	slashed := filepath.ToSlash(trimmed)
	if strings.HasPrefix(slashed, "/") || filepath.IsAbs(trimmed) || strings.Contains(slashed, "\x00") {
		return "", fmt.Errorf("owner atlas page %q must be relative to the owner root", page)
	}
	cleaned := path.Clean(slashed)
	if cleaned == "." || strings.HasPrefix(cleaned, "../") || cleaned == ".." {
		return "", fmt.Errorf("owner atlas page %q escapes the owner root", page)
	}
	if strings.HasPrefix(cleaned, journalDirectory+"/") || cleaned == journalDirectory {
		return "", errors.New("owner atlas journal is not an addressable page")
	}
	if !strings.HasSuffix(cleaned, ".md") {
		return "", fmt.Errorf("owner atlas page %q must be a Markdown page", page)
	}
	return cleaned, nil
}

func (engine *Engine) pagePath(relative string) string {
	return filepath.Join(engine.root, filepath.FromSlash(relative))
}

func (engine *Engine) read(relative string) (string, error) {
	body, err := scheduler.ReadPrivateFile(engine.pagePath(relative), maximumPageBytes)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func (engine *Engine) operationID(operation, page, key string) string {
	return digest(operation + "\x00" + page + "\x00" + key)
}

// recordedResult replays a completed operation instead of repeating it, so a
// retry of the same idempotency key converges rather than duplicating.
func (engine *Engine) recordedResult(operationID string) (Result, bool, error) {
	body, err := scheduler.ReadPrivateFile(engine.journalPath(operationID), maximumPageBytes)
	if errors.Is(err, os.ErrNotExist) {
		return Result{}, false, nil
	}
	if err != nil {
		if isNotExist(err) {
			return Result{}, false, nil
		}
		return Result{}, false, err
	}
	var entry journalRecord
	if err := json.Unmarshal(body, &entry); err != nil {
		return Result{}, false, err
	}
	if entry.SchemaVersion != schemaVersion {
		return Result{}, false, fmt.Errorf("owner atlas journal schema %d is not supported", entry.SchemaVersion)
	}
	replayed := entry.Result
	if replayed.State == StateWritten {
		replayed.State = StateUnchanged
		replayed.Reason = "operation already applied under this idempotency key"
	}
	return replayed, true, nil
}

type journalRecord struct {
	SchemaVersion int    `json:"schema_version"`
	Result        Result `json:"result"`
}

func (engine *Engine) record(result Result) error {
	directory := filepath.Join(engine.root, journalDirectory)
	if err := scheduler.EnsurePrivateDirectory(directory); err != nil {
		return err
	}
	// The proposal body is deliberately not persisted: it is a draft the caller
	// still owns, and a receipt keeps metadata rather than content.
	persisted := result
	persisted.Proposal = ""
	body, err := json.Marshal(journalRecord{SchemaVersion: schemaVersion, Result: persisted})
	if err != nil {
		return err
	}
	err = scheduler.WriteNewPrivateFile(engine.journalPath(result.OperationID), body)
	if errors.Is(err, os.ErrExist) {
		return nil
	}
	return err
}

func (engine *Engine) journalPath(operationID string) string {
	return filepath.Join(engine.root, journalDirectory, operationID+".json")
}

func digest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

// isNotExist covers the platform error the no-follow boundary surfaces for a
// missing leaf, which is not always wrapped as os.ErrNotExist.
func isNotExist(err error) bool {
	if errors.Is(err, os.ErrNotExist) {
		return true
	}
	return strings.Contains(strings.ToLower(err.Error()), "cannot find the file")
}
