// Package atlasops performs named, bounded operations over one owner or
// registered workspace atlas. A skill asks for an operation here and
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

// Engine binds the operations to one canonical owner or workspace atlas root.
type Engine struct {
	root             string
	dataRoot         string
	now              func() time.Time
	workspace        *workspaceScope
	boundDirectories []os.FileInfo
	workspacePolicy  WorkspacePolicy
	// afterPageEffect permits deterministic interruption tests without an
	// exposed runtime option or model-controlled failure switch.
	afterPageEffect func() error
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
	if err := engine.admitWorkspacePage(page); err != nil {
		return err
	}
	if engine.workspacePolicy.BeforeMutation != nil {
		if err := engine.workspacePolicy.BeforeMutation(); err != nil {
			return err
		}
	}
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

// Root is the canonical atlas root this engine is bound to.
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
	release, err := engine.beginMutation()
	if err != nil {
		return Result{}, err
	}
	defer func() { _ = release() }()
	if err := engine.authorize(request.Provenance, "create-page", relative); err != nil {
		return Result{}, err
	}

	operationID := engine.operationID("create-page", relative, request.Provenance.IdempotencyKey)
	requestDigest, err := digestRequest(request)
	if err != nil {
		return Result{}, err
	}
	if recorded, found, err := engine.recordedResult(operationID, requestDigest); err != nil {
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
	existing, readErr := engine.read(relative)
	if readErr == nil {
		result.State = StateUnchanged
		result.Revision = digest(existing)
		result.Reason = "page already exists and was preserved"
		return result, engine.record(result, requestDigest)
	}
	if !isNotExist(readErr) {
		return Result{}, readErr
	}
	result.State = StateWritten
	result.Revision = digest(request.Body)
	if err := engine.prepare(result, "", requestDigest); err != nil {
		return Result{}, err
	}
	if err := engine.validateBinding(); err != nil {
		return Result{}, err
	}
	if err := engine.authorize(request.Provenance, "create-page", relative); err != nil {
		return Result{}, err
	}
	writeErr := publishAtlasFile(full, []byte(request.Body), true, engine.workspace != nil)
	switch {
	case writeErr == nil:
		if engine.afterPageEffect != nil {
			if err := engine.afterPageEffect(); err != nil {
				return Result{}, err
			}
		}
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
	if err := engine.record(result, requestDigest); err != nil {
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
	release, err := engine.beginMutation()
	if err != nil {
		return Result{}, err
	}
	defer func() { _ = release() }()
	if err := engine.authorize(request.Provenance, "append-entry", relative); err != nil {
		return Result{}, err
	}

	operationID := engine.operationID("append-entry", relative, request.Provenance.IdempotencyKey)
	requestDigest, err := digestRequest(request)
	if err != nil {
		return Result{}, err
	}
	if recorded, found, err := engine.recordedResult(operationID, requestDigest); err != nil {
		return Result{}, err
	} else if found {
		return recorded, nil
	}

	current, err := engine.read(relative)
	if err != nil {
		return Result{}, err
	}
	updated, err := PreviewAppend(current, section, entry)
	if err != nil {
		return Result{}, err
	}

	return engine.commitEdit(commitRequest{operation: "append-entry", operationID: operationID,
		page: relative, current: current, updated: updated, changed: true,
		expectedRevision: request.ExpectedRevision, provenance: request.Provenance, requestDigest: requestDigest})
}

// PreviewAppend computes the exact section edit without acquiring authority or
// touching disk. Attended adapters can present the same body the writer uses.
func PreviewAppend(body, section, entry string) (string, error) {
	section = strings.TrimRight(section, " \t\r")
	entry = strings.TrimRight(entry, "\r\n")
	if !strings.HasPrefix(section, "#") || strings.ContainsAny(section, "\r\n") || strings.TrimSpace(entry) == "" || len(entry) > maximumEntryBytes {
		return "", errors.New("atlas append requires one existing heading and a bounded entry")
	}
	return insertUnderSection(body, section, entry)
}

// insertUnderSection places the entry at the end of the named section, leaving
// every other section untouched. A section the page does not declare is an
// error: the operation set does not invent structure.
func insertUnderSection(body, section, entry string) (string, error) {
	lines := strings.Split(body, "\n")
	start := -1
	matches := 0
	for index, line := range lines {
		if strings.TrimRight(line, " \t\r") == section {
			matches++
			if start < 0 {
				start = index
			}
		}
	}
	if start < 0 {
		return "", fmt.Errorf("owner atlas page does not declare the section %q", section)
	}
	if matches > 1 {
		// A page may legitimately repeat a heading — an objectives page with an
		// evidence section under each objective is the ordinary case. Taking the
		// first match would file the entry under the wrong objective and report
		// success, so an ambiguous target is refused instead.
		return "", fmt.Errorf("owner atlas page declares the section %q %d times; the target is ambiguous", section, matches)
	}
	level := len(section) - len(strings.TrimLeft(section, "#"))
	end := len(lines)
	for index := start + 1; index < len(lines); index++ {
		trimmed := strings.TrimRight(lines[index], " \t\r")
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
	// Existing line strings retain their original CR bytes. Only the new entry
	// adopts the selected heading's line ending; surrounding mixed/LF/CRLF
	// source remains byte-for-byte unchanged.
	crlf := strings.HasSuffix(lines[start], "\r")
	if start == len(lines)-1 && start > 0 {
		crlf = strings.HasSuffix(lines[start-1], "\r")
	}
	entries := strings.Split(strings.ReplaceAll(entry, "\r\n", "\n"), "\n")
	if crlf {
		if insert == len(lines) && insert > 0 && !strings.HasSuffix(lines[insert-1], "\r") {
			lines[insert-1] += "\r"
		}
		for index := range entries {
			if index < len(entries)-1 || insert < len(lines) {
				entries[index] += "\r"
			}
		}
	}
	updated := make([]string, 0, len(lines)+len(entries))
	updated = append(updated, lines[:insert]...)
	updated = append(updated, entries...)
	updated = append(updated, lines[insert:]...)
	return strings.Join(updated, "\n"), nil
}

// resolve rejects anything that is not a plain relative page inside the root.
func (engine *Engine) resolve(page string) (string, error) {
	if err := engine.validateBinding(); err != nil {
		return "", err
	}
	if err := engine.admitWorkspacePage(page); err != nil {
		return "", err
	}
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
	if err := engine.validateBinding(); err != nil {
		return "", err
	}
	if err := engine.admitWorkspacePage(relative); err != nil {
		return "", err
	}
	body, err := readAtlasFile(engine.pagePath(relative), maximumPageBytes, engine.workspace != nil)
	if err != nil {
		return "", err
	}
	if err = engine.admitWorkspacePage(relative); err != nil {
		return "", err
	}
	return string(body), nil
}

func (engine *Engine) operationID(operation, page, key string) string {
	return digest(operation + "\x00" + page + "\x00" + key)
}

// recordedResult replays a completed operation instead of repeating it, so a
// retry of the same idempotency key converges rather than duplicating.
func (engine *Engine) recordedResult(operationID, requestDigest string) (Result, bool, error) {
	return engine.recordedResultChecked(operationID, requestDigest, nil)
}

// A checked replay never finalizes or returns a receipt before the caller's
// owning, locked endpoint authority has been revalidated.
func (engine *Engine) recordedResultChecked(operationID, requestDigest string, validate func() error) (Result, bool, error) {
	if err := engine.validateBinding(); err != nil {
		return Result{}, false, err
	}
	body, err := readAtlasFile(engine.journalPath(operationID), maximumPageBytes, true)
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
	if entry.SchemaVersion != schemaVersion && entry.SchemaVersion != 2 {
		return Result{}, false, fmt.Errorf("owner atlas journal schema %d is not supported", entry.SchemaVersion)
	}
	if err := engine.validateScope(entry.Workspace); err != nil {
		return Result{}, false, err
	}
	if entry.RequestDigest != requestDigest && (entry.RequestDigest != "" || engine.workspace != nil || entry.SchemaVersion == 2) {
		return Result{}, false, errors.New("atlas idempotency key is already bound to a different request")
	}
	if entry.Result.OperationID != operationID {
		return Result{}, false, errors.New("atlas journal operation identity changed")
	}
	if entry.SchemaVersion == 2 {
		if entry.Prepared == nil || !validDigest(entry.Prepared.AfterRevision) ||
			(entry.Prepared.BeforeRevision != "" && !validDigest(entry.Prepared.BeforeRevision)) ||
			!validDigest(entry.RequestDigest) || entry.Result.Revision != entry.Prepared.AfterRevision || entry.Result.State != StateWritten {
			return Result{}, false, errors.New("atlas prepared journal is invalid")
		}
		relative, err := engine.resolve(entry.Result.Page)
		if err != nil || relative != entry.Result.Page {
			return Result{}, false, errors.New("atlas prepared page is invalid")
		}
		current, err := engine.read(relative)
		currentRevision := ""
		if err == nil {
			currentRevision = digest(current)
		} else if !isNotExist(err) {
			return Result{}, false, err
		}
		switch currentRevision {
		case entry.Prepared.AfterRevision:
			if validate != nil {
				if err := validate(); err != nil {
					return Result{}, false, err
				}
			}
			if err := engine.record(entry.Result, requestDigest); err != nil {
				return Result{}, false, err
			}
		case entry.Prepared.BeforeRevision:
			return Result{}, false, nil
		default:
			conflict := entry.Result
			conflict.State, conflict.Revision = StateProposed, currentRevision
			conflict.Reason = "page changed after an interrupted operation; pending evidence and newer content were preserved"
			return conflict, true, nil
		}
	} else if entry.Prepared != nil {
		return Result{}, false, errors.New("atlas completed journal contains pending state")
	}
	if validate != nil {
		if err := validate(); err != nil {
			return Result{}, false, err
		}
	}
	replayed := entry.Result
	if replayed.State == StateWritten {
		replayed.State = StateUnchanged
		replayed.Reason = "operation already applied under this idempotency key"
	}
	return replayed, true, nil
}

type journalRecord struct {
	SchemaVersion int             `json:"schema_version"`
	Result        Result          `json:"result"`
	Workspace     *workspaceScope `json:"workspace,omitempty"`
	RequestDigest string          `json:"request_digest,omitempty"`
	Prepared      *preparedEdit   `json:"prepared,omitempty"`
}

type preparedEdit struct {
	BeforeRevision string `json:"before_revision"`
	AfterRevision  string `json:"after_revision"`
}

func digestRequest(request any) (string, error) {
	body, err := json.Marshal(request)
	if err != nil {
		return "", err
	}
	return digest(string(body)), nil
}

func validDigest(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size && value == strings.ToLower(value)
}

// Prepared records use v2 so an older reader cannot mistake intent for a
// completed effect. They contain hashes and metadata, never draft content.
func (engine *Engine) prepare(result Result, before, requestDigest string) error {
	if err := engine.validateBinding(); err != nil {
		return err
	}
	entry := journalRecord{SchemaVersion: 2, Result: result, Workspace: engine.workspace, RequestDigest: requestDigest,
		Prepared: &preparedEdit{BeforeRevision: before, AfterRevision: result.Revision}}
	body, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	err = publishAtlasFile(engine.journalPath(result.OperationID), body, true, true)
	if !errors.Is(err, os.ErrExist) {
		return err
	}
	existing, err := readAtlasFile(engine.journalPath(result.OperationID), maximumPageBytes, true)
	if err != nil {
		return err
	}
	var pending journalRecord
	if err := json.Unmarshal(existing, &pending); err != nil {
		return err
	}
	if err := engine.validateScope(pending.Workspace); err != nil {
		return err
	}
	if pending.SchemaVersion != 2 || pending.Prepared == nil || *pending.Prepared != *entry.Prepared || pending.RequestDigest != requestDigest || pending.Result.OperationID != result.OperationID {
		return errors.New("atlas prepared effect no longer matches this request")
	}
	return nil
}

func (engine *Engine) record(result Result, requestDigest string) error {
	if err := engine.validateBinding(); err != nil {
		return err
	}
	directory := filepath.Join(engine.root, journalDirectory)
	if err := scheduler.EnsurePrivateDirectory(directory); err != nil {
		return err
	}
	// The proposal body is deliberately not persisted: it is a draft the caller
	// still owns, and a receipt keeps metadata rather than content.
	persisted := result
	persisted.Proposal = ""
	body, err := json.Marshal(journalRecord{SchemaVersion: schemaVersion, Result: persisted, Workspace: engine.workspace, RequestDigest: requestDigest})
	if err != nil {
		return err
	}
	err = publishAtlasFile(engine.journalPath(result.OperationID), body, true, true)
	if errors.Is(err, os.ErrExist) {
		existing, err := readAtlasFile(engine.journalPath(result.OperationID), maximumPageBytes, true)
		if err != nil {
			return err
		}
		var pending journalRecord
		if err := json.Unmarshal(existing, &pending); err != nil {
			return err
		}
		if err := engine.validateScope(pending.Workspace); err != nil {
			return err
		}
		if pending.SchemaVersion != 2 || pending.Prepared == nil || pending.RequestDigest != requestDigest || pending.Result.OperationID != result.OperationID {
			return errors.New("atlas completion does not match a prepared effect")
		}
		return publishAtlasFile(engine.journalPath(result.OperationID), body, false, true)
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
