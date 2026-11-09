package atlasops

import (
	"encoding/json"
	"errors"
)

// These inspections disclose only whether this exact typed request already has
// an effect matching the current page. They never finalize a pending receipt,
// repair a journal or grant permission to write. The mutation rechecks the same
// request under its corpus lock and CAS after attended review.
func (engine *Engine) ReviewAppendReplay(request AppendEntryRequest) (bool, error) {
	return engine.reviewExactReplay("append-entry", request.Page, request.ExpectedRevision, request, request.Provenance)
}
func (engine *Engine) ReviewUpdateReplay(request UpdatePageRequest) (bool, error) {
	return engine.reviewExactReplay("update-page", request.Page, request.ExpectedRevision, request, request.Provenance)
}
func (engine *Engine) ReviewFieldReplay(request SetFieldRequest) (bool, error) {
	return engine.reviewExactReplay("set-field", request.Page, request.ExpectedRevision, request, request.Provenance)
}
func (engine *Engine) ReviewLinkReplay(request LinkRequest) (bool, error) {
	target, err := engine.resolve(request.Target)
	if err != nil {
		return false, err
	}
	if err = engine.validateLinkTarget(target, request.ExpectedTargetRevision); err != nil {
		return false, err
	}
	return engine.reviewExactReplay("link", request.Page, request.ExpectedRevision, request, request.Provenance)
}

func (engine *Engine) reviewExactReplay(operation, page, expected string, request any, provenance Provenance) (bool, error) {
	relative, err := engine.resolve(page)
	if err != nil {
		return false, err
	}
	if err = provenance.validate(); err != nil {
		return false, err
	}
	if !validDigest(expected) {
		return false, errors.New("atlas replay requires the original exact base")
	}
	requestDigest, err := digestRequest(request)
	if err != nil {
		return false, err
	}
	operationID := engine.operationID(operation, relative, provenance.IdempotencyKey)
	body, err := readAtlasFile(engine.journalPath(operationID), maximumPageBytes, true)
	if err != nil {
		if isNotExist(err) {
			return false, nil
		}
		return false, err
	}
	var entry journalRecord
	if json.Unmarshal(body, &entry) != nil {
		return false, errors.New("atlas replay journal is invalid")
	}
	if err = engine.validateScope(entry.Workspace); err != nil {
		return false, err
	}
	if entry.RequestDigest != requestDigest || entry.Result.OperationID != operationID || entry.Result.Operation != operation || entry.Result.Page != relative || entry.Result.Provenance != provenance || entry.Result.RecordedAt.IsZero() || !validDigest(entry.Result.Revision) {
		return false, errors.New("atlas replay identity does not match the exact request")
	}
	switch entry.SchemaVersion {
	case schemaVersion:
		if entry.Prepared != nil || (entry.Result.State != StateWritten && entry.Result.State != StateUnchanged) {
			return false, nil
		}
	case 2:
		if entry.Prepared == nil || entry.Prepared.BeforeRevision != expected || entry.Prepared.AfterRevision != entry.Result.Revision || entry.Result.State != StateWritten {
			return false, errors.New("atlas replay prepared identity is invalid")
		}
	default:
		return false, errors.New("atlas replay schema is invalid")
	}
	current, err := engine.read(relative)
	if err != nil {
		return false, err
	}
	return digest(current) == entry.Result.Revision, nil
}
