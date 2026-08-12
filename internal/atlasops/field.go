package atlasops

import (
	"errors"
	"fmt"
	"path"
	"strings"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

// SetFieldRequest sets one declared field without rewriting the prose around
// it. The field must already exist: the operation set does not invent
// structure, here any more than in append-entry.
type SetFieldRequest struct {
	Page  string
	Field string
	Value string
	// ExpectedRevision, when set, refuses the write if the page changed
	// underneath the caller and returns a reviewable proposal instead.
	ExpectedRevision string
	Provenance       Provenance
}

// LinkRequest adds a reference from one owner page to another. The target is
// resolved inside the owner root: a link is navigation within the corpus, not
// a way to point somewhere the boundary would refuse to read.
type LinkRequest struct {
	Page             string
	Section          string
	Target           string
	Label            string
	ExpectedRevision string
	Provenance       Provenance
}

const maximumFieldBytes = 4 << 10

// SetField replaces the value of a declared field. Fields are written as a
// Markdown list item — `- **Name:** value` — which is the shape the segment
// templates use, so a field is legible to a reader and addressable to the
// command layer without either needing a parser for the whole page.
func (engine *Engine) SetField(request SetFieldRequest) (Result, error) {
	relative, err := engine.resolve(request.Page)
	if err != nil {
		return Result{}, err
	}
	if err := request.Provenance.validate(); err != nil {
		return Result{}, err
	}
	field := strings.TrimSpace(request.Field)
	if field == "" || strings.ContainsAny(field, "\r\n*") {
		return Result{}, errors.New("owner atlas field name is required and must be a single plain label")
	}
	value := strings.TrimSpace(request.Value)
	if strings.ContainsAny(value, "\r\n") || len(value) > maximumFieldBytes {
		return Result{}, fmt.Errorf("owner atlas field value must be a single line under %d bytes", maximumFieldBytes)
	}
	if err := engine.authorize(request.Provenance, "set-field", relative); err != nil {
		return Result{}, err
	}

	operationID := engine.operationID("set-field", relative, request.Provenance.IdempotencyKey)
	if recorded, found, err := engine.recordedResult(operationID); err != nil {
		return Result{}, err
	} else if found {
		return recorded, nil
	}

	current, err := engine.read(relative)
	if err != nil {
		return Result{}, err
	}
	updated, changed, err := replaceField(current, field, value)
	if err != nil {
		return Result{}, err
	}
	return engine.commitEdit(commitRequest{
		operation:        "set-field",
		operationID:      operationID,
		page:             relative,
		current:          current,
		updated:          updated,
		changed:          changed,
		unchangedReason:  "field already holds this value",
		expectedRevision: request.ExpectedRevision,
		provenance:       request.Provenance,
	})
}

// Link adds a path-preserving Markdown reference under a section the page
// already declares. Repeating the same target is a no-op, so a ritual that
// re-runs does not accumulate duplicate edges.
func (engine *Engine) Link(request LinkRequest) (Result, error) {
	relative, err := engine.resolve(request.Page)
	if err != nil {
		return Result{}, err
	}
	if err := request.Provenance.validate(); err != nil {
		return Result{}, err
	}
	target, err := engine.resolve(request.Target)
	if err != nil {
		return Result{}, fmt.Errorf("link target: %w", err)
	}
	section := strings.TrimRight(request.Section, " \t")
	if !strings.HasPrefix(section, "#") {
		return Result{}, errors.New("owner atlas link requires a Markdown heading as its section")
	}
	label := strings.TrimSpace(request.Label)
	if label == "" || strings.ContainsAny(label, "\r\n[]") {
		return Result{}, errors.New("owner atlas link label is required and must not contain brackets")
	}
	if err := engine.authorize(request.Provenance, "link", relative); err != nil {
		return Result{}, err
	}

	operationID := engine.operationID("link", relative, request.Provenance.IdempotencyKey)
	if recorded, found, err := engine.recordedResult(operationID); err != nil {
		return Result{}, err
	} else if found {
		return recorded, nil
	}

	current, err := engine.read(relative)
	if err != nil {
		return Result{}, err
	}

	// The relative form from this page to the target, so the written link
	// resolves from the page itself rather than only from the root.
	reference := relativeReference(relative, target)
	updated := current
	changed := false
	if !strings.Contains(current, "("+reference+")") {
		updated, err = insertUnderSection(current, section, "- ["+label+"]("+reference+")")
		if err != nil {
			return Result{}, err
		}
		changed = true
	}
	return engine.commitEdit(commitRequest{
		operation:        "link",
		operationID:      operationID,
		page:             relative,
		current:          current,
		updated:          updated,
		changed:          changed,
		unchangedReason:  "page already references this target",
		expectedRevision: request.ExpectedRevision,
		provenance:       request.Provenance,
	})
}

// replaceField finds the single declared field and swaps its value.
func replaceField(body, field, value string) (string, bool, error) {
	lines := strings.Split(body, "\n")
	prefix := "- **" + field + ":**"
	index := -1
	matches := 0
	for position, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), prefix) {
			matches++
			if index < 0 {
				index = position
			}
		}
	}
	if index < 0 {
		return "", false, fmt.Errorf("owner atlas page does not declare the field %q", field)
	}
	if matches > 1 {
		// The same reasoning as a repeated section heading: filing under the
		// first match would set the wrong field and report success.
		return "", false, fmt.Errorf("owner atlas page declares the field %q %d times; the target is ambiguous", field, matches)
	}
	indent := lines[index][:len(lines[index])-len(strings.TrimLeft(lines[index], " \t"))]
	replacement := indent + prefix
	if value != "" {
		replacement += " " + value
	}
	if lines[index] == replacement {
		return body, false, nil
	}
	lines[index] = replacement
	return strings.Join(lines, "\n"), true, nil
}

// relativeReference expresses target relative to the page that will hold the
// link, so the path stays resolvable when the page is opened directly.
func relativeReference(from, target string) string {
	fromParts := strings.Split(path.Dir(from), "/")
	targetParts := strings.Split(target, "/")
	if path.Dir(from) == "." {
		fromParts = nil
	}
	common := 0
	for common < len(fromParts) && common < len(targetParts)-1 && fromParts[common] == targetParts[common] {
		common++
	}
	reference := strings.Repeat("../", len(fromParts)-common) + strings.Join(targetParts[common:], "/")
	if reference == "" {
		return target
	}
	return reference
}

// commitRequest carries what every editing operation needs to finish: the
// conflict check, the write and the receipt are identical across them, and
// only the reason a no-op is a no-op differs.
type commitRequest struct {
	operation        string
	operationID      string
	page             string
	current          string
	updated          string
	changed          bool
	unchangedReason  string
	expectedRevision string
	provenance       Provenance
}

func (engine *Engine) commitEdit(request commitRequest) (Result, error) {
	revision := digest(request.current)
	result := Result{
		Operation:   request.operation,
		Page:        request.page,
		OperationID: request.operationID,
		RecordedAt:  engine.now().UTC(),
		Provenance:  request.provenance,
	}

	if expected := strings.TrimSpace(request.expectedRevision); expected != "" && expected != revision {
		result.State = StateProposed
		result.Revision = revision
		result.Proposal = request.updated
		result.Reason = "page revision changed since it was read; nothing was written"
		if err := engine.record(result); err != nil {
			return Result{}, err
		}
		return result, nil
	}

	if !request.changed {
		result.State = StateUnchanged
		result.Revision = revision
		result.Reason = request.unchangedReason
		if err := engine.record(result); err != nil {
			return Result{}, err
		}
		return result, nil
	}

	if err := scheduler.ReplacePrivateFile(engine.pagePath(request.page), []byte(request.updated)); err != nil {
		return Result{}, err
	}
	result.State = StateWritten
	result.Revision = digest(request.updated)
	if err := engine.record(result); err != nil {
		return Result{}, err
	}
	return result, nil
}
