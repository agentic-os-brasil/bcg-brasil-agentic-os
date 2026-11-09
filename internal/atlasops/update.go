package atlasops

import (
	"errors"
	"strings"
	"unicode/utf8"
)

// UpdatePageRequest replaces one complete reviewed page. The exact base is
// mandatory; an authored replacement never turns into an unfenced overwrite.
type UpdatePageRequest struct {
	Page             string
	Body             string
	ExpectedRevision string
	Provenance       Provenance
}

// UpdatePage uses the same corpus lock, conflict result and prepared journal as
// section/field edits. Exact retries recover the original operation, not a new
// replacement assembled from whatever happens to be on disk now.
func (engine *Engine) UpdatePage(request UpdatePageRequest) (Result, error) {
	relative, err := engine.resolve(request.Page)
	if err != nil {
		return Result{}, err
	}
	if err = request.Provenance.validate(); err != nil {
		return Result{}, err
	}
	if !validDigest(request.ExpectedRevision) || len(request.Body) == 0 || len(request.Body) > maximumPageBytes || !utf8.ValidString(request.Body) || strings.ContainsRune(request.Body, 0) {
		return Result{}, errors.New("atlas replacement requires a bounded page and exact revision")
	}
	release, err := engine.beginMutation()
	if err != nil {
		return Result{}, err
	}
	defer func() { _ = release() }()
	if err = engine.authorize(request.Provenance, "update-page", relative); err != nil {
		return Result{}, err
	}
	operationID := engine.operationID("update-page", relative, request.Provenance.IdempotencyKey)
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
	return engine.commitEdit(commitRequest{operation: "update-page", operationID: operationID, page: relative, current: current, updated: request.Body, changed: current != request.Body, unchangedReason: "page already holds the reviewed body", expectedRevision: request.ExpectedRevision, provenance: request.Provenance, requestDigest: requestDigest})
}
