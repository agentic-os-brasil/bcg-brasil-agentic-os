package atlasops

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/atlas"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

var ErrLifecycleChanged = errors.New("authored lifecycle changed")
var ErrLifecyclePending = errors.New("authored lifecycle operation incomplete")
var ErrPageRetired = errors.New("authored page retired")

type PageLocation struct {
	StableID string   `json:"stable_id"`
	Location string   `json:"location"`
	State    string   `json:"state"`
	Names    []string `json:"names"`
}

type lifecycleState struct {
	SchemaVersion int                     `json:"schema_version"`
	Workspace     *workspaceScope         `json:"workspace"`
	Revision      uint64                  `json:"revision"`
	Pages         map[string]PageLocation `json:"pages"`
	Pending       *lifecyclePrepared      `json:"pending,omitempty"`
}

type MovePageRequest struct {
	Page                      string
	Target                    string
	ExpectedRevision          string
	ExpectedLifecycleRevision uint64
	Provenance                Provenance
	ReferencesDigest          string `json:"-"`
}

type RetirePageRequest struct {
	Page                      string
	ExpectedRevision          string
	ExpectedLifecycleRevision uint64
	Provenance                Provenance
	ReferencesDigest          string `json:"-"`
}

func (request MovePageRequest) lifecycleRequest() lifecycleRequest {
	return lifecycleRequest{Operation: "move-page", Page: request.Page, Target: request.Target, ExpectedRevision: request.ExpectedRevision, ExpectedLifecycleRevision: request.ExpectedLifecycleRevision, Provenance: request.Provenance, ReferencesDigest: request.ReferencesDigest}
}

func (request RetirePageRequest) lifecycleRequest() lifecycleRequest {
	return lifecycleRequest{Operation: "retire-page", Page: request.Page, ExpectedRevision: request.ExpectedRevision, ExpectedLifecycleRevision: request.ExpectedLifecycleRevision, Provenance: request.Provenance, ReferencesDigest: request.ReferencesDigest}
}

func (engine *Engine) ReviewMove(request MovePageRequest) (LifecycleReview, error) {
	return engine.reviewLifecycle(request.lifecycleRequest())
}

func (engine *Engine) MovePage(request MovePageRequest) (Result, error) {
	return engine.applyLifecycle(request.lifecycleRequest())
}

func (engine *Engine) ReviewRetire(request RetirePageRequest) (LifecycleReview, error) {
	return engine.reviewLifecycle(request.lifecycleRequest())
}

func (engine *Engine) RetirePage(request RetirePageRequest) (Result, error) {
	return engine.applyLifecycle(request.lifecycleRequest())
}

type lifecycleRequest struct {
	Operation                 string
	Page                      string
	Target                    string
	ExpectedRevision          string
	ExpectedLifecycleRevision uint64
	Provenance                Provenance
	ReferencesDigest          string `json:"-"`
}

type LifecycleReview struct {
	prepared         *lifecyclePrepared
	Operation        string          `json:"operation"`
	Source           PageProjection  `json:"source"`
	StableID         string          `json:"stable_id"`
	Target           string          `json:"target"`
	Proposed         string          `json:"proposed"`
	Revision         uint64          `json:"lifecycle_revision"`
	References       []PageReference `json:"affected_references"`
	ReferencesDigest string          `json:"references_digest"`
	Receipt          *Result         `json:"receipt,omitempty"`
}

type lifecyclePrepared struct {
	OperationID   string           `json:"operation_id"`
	RequestDigest string           `json:"request_digest"`
	Request       lifecycleRequest `json:"request"`
	StableID      string           `json:"stable_id"`
	Source        string           `json:"source"`
	Target        string           `json:"target"`
	Before        string           `json:"before_revision"`
	After         string           `json:"after_revision"`
	Result        Result           `json:"result"`
}

type lifecycleRecord struct {
	SchemaVersion int               `json:"schema_version"`
	Workspace     *workspaceScope   `json:"workspace"`
	Prepared      lifecyclePrepared `json:"lifecycle"`
	Complete      bool              `json:"complete"`
}

func (engine *Engine) lifecycle() (lifecycleState, error) {
	state := lifecycleState{SchemaVersion: 1, Workspace: engine.workspace, Pages: map[string]PageLocation{}}
	if engine.workspace == nil {
		return state, nil
	}
	if err := engine.validateBinding(); err != nil {
		return state, err
	}
	body, err := readAtlasFile(engine.journalDirPath("lifecycle.json"), maximumPageBytes, true)
	if err != nil && isNotExist(err) {
		return state, nil
	}
	if err != nil {
		return state, err
	}
	if err = json.Unmarshal(body, &state); err != nil {
		return state, err
	}
	if _, err = engine.lifecycleBody(state); err != nil {
		return state, err
	}
	return state, nil
}

func (engine *Engine) lifecycleBody(state lifecycleState) ([]byte, error) {
	if state.SchemaVersion != 1 || state.Pages == nil {
		return nil, ErrLifecycleChanged
	}
	if err := engine.validateScope(state.Workspace); err != nil {
		return nil, err
	}
	if len(state.Pages) > 1024 || state.Revision > 9007199254740991 {
		return nil, ErrLifecycleChanged
	}
	seen := map[string]string{}
	for id, page := range state.Pages {
		if id == "" || page.StableID != id || len(page.Names) == 0 || len(page.Names) > 128 || page.State != "active" && page.State != "retired" {
			return nil, ErrLifecycleChanged
		}
		for _, name := range page.Names {
			clean, err := relativeAtlasPage(name)
			if err != nil || clean != name || seen[canonicalPageName(name)] != "" && seen[canonicalPageName(name)] != id {
				return nil, ErrLifecycleChanged
			}
			seen[canonicalPageName(name)] = id
		}
		validLocation, validIdentity := false, false
		for _, name := range page.Names {
			validLocation = validLocation || canonicalPageName(name) == canonicalPageName(page.Location)
			validIdentity = validIdentity || name == id+".md"
		}
		if !validLocation || !validIdentity {
			return nil, ErrLifecycleChanged
		}
	}
	if state.Pending != nil {
		pending := *state.Pending
		if err := validateLifecyclePrepared(pending); err != nil || state.Revision != pending.Request.ExpectedLifecycleRevision+1 {
			return nil, ErrLifecycleChanged
		}
		source := pending.Request.Page
		if item, ok := state.Pages[pending.StableID]; ok {
			source = item.Location
		}
		if pending.Source != source {
			return nil, ErrLifecycleChanged
		}
	}
	body, err := json.Marshal(state)
	if err != nil {
		return nil, err
	}
	if len(body) > maximumPageBytes {
		return nil, errors.New("authored lifecycle exceeds bounded metadata")
	}
	return body, nil
}

func validateLifecyclePrepared(pending lifecyclePrepared) error {
	request, result := pending.Request, pending.Result
	calculated, err := digestRequest(request)
	if err != nil || request.Provenance.validate() != nil || request.Provenance.Origin != OriginAttended || request.Operation != "move-page" && request.Operation != "retire-page" || calculated != pending.RequestDigest || !validDigest(pending.OperationID) || !validDigest(pending.Before) || !validDigest(pending.After) || pending.Before != request.ExpectedRevision || request.ExpectedLifecycleRevision > 9007199254740989 {
		return ErrLifecycleChanged
	}
	if pending.OperationID != digest(request.Operation+"\x00"+pending.StableID+".md\x00"+request.Provenance.IdempotencyKey) {
		return ErrLifecycleChanged
	}
	source, sourceErr := relativeAtlasPage(pending.Source)
	selected, selectedErr := relativeAtlasPage(request.Page)
	target, targetErr := relativeAtlasPage(request.Target)
	location, pageState := pending.Target, "active"
	if request.Operation == "retire-page" {
		if request.Target != "" || pending.Before != pending.After {
			return ErrLifecycleChanged
		}
		target, targetErr = archivePage(pending.OperationID), nil
		location, pageState = pending.Source, "retired"
	}
	if sourceErr != nil || selectedErr != nil || selected != request.Page || source != pending.Source || targetErr != nil || pending.Target != target || pending.Source == pending.Target || strings.EqualFold(source, "README.md") || strings.EqualFold(target, "README.md") {
		return ErrLifecycleChanged
	}
	if result.Operation != request.Operation || result.PageState != pageState || result.OperationID != pending.OperationID || result.Page != pending.Source || result.StableID != pending.StableID || result.Location != location || result.Revision != pending.After || result.Provenance != request.Provenance || result.State != StateWritten || result.Proposal != "" || result.RecordedAt.IsZero() || result.LifecycleRevision != request.ExpectedLifecycleRevision+2 {
		return ErrLifecycleChanged
	}
	return nil
}

func (state lifecycleState) location(page string) (PageLocation, error) {
	for _, item := range state.Pages {
		for _, name := range item.Names {
			if canonicalPageName(name) == canonicalPageName(page) {
				if state.Pending != nil && state.Pending.StableID == item.StableID {
					return item, ErrLifecyclePending
				}
				if item.State == "retired" {
					return item, ErrPageRetired
				}
				return item, nil
			}
		}
	}
	if state.Pending != nil && (canonicalPageName(state.Pending.Source) == canonicalPageName(page) || canonicalPageName(state.Pending.Target) == canonicalPageName(page)) {
		return PageLocation{}, ErrLifecyclePending
	}
	return PageLocation{StableID: strings.TrimSuffix(page, ".md"), Location: page, State: "active", Names: []string{page}}, nil
}

// LocatePage resolves identity while retaining the current location's admission.
func (engine *Engine) LocatePage(page string) (PageLocation, uint64, error) {
	name, err := engine.relativePage(page)
	if err != nil {
		return PageLocation{}, 0, err
	}
	state, err := engine.lifecycle()
	if err != nil {
		return PageLocation{}, 0, err
	}
	location, stateErr := state.location(name)
	if errors.Is(stateErr, ErrLifecyclePending) && state.Pending != nil {
		if err = engine.admitWorkspacePage(state.Pending.Source); err != nil {
			return PageLocation{}, state.Revision, err
		}
		if state.Pending.Request.Operation == "move-page" {
			if err = engine.admitWorkspacePage(state.Pending.Target); err != nil {
				return PageLocation{}, state.Revision, err
			}
		}
	}
	if location.Location != "" {
		if err = engine.admitWorkspacePage(location.Location); err != nil {
			return location, state.Revision, err
		}
	}
	if stateErr != nil {
		return location, state.Revision, stateErr
	}
	return location, state.Revision, nil
}

func (engine *Engine) LifecycleRevision() (uint64, error) {
	state, err := engine.lifecycle()
	return state.Revision, err
}

// AdmitPhysicalPage never follows an alias for a conversion-source pathname.
func (engine *Engine) AdmitPhysicalPage(page string) error {
	location, _, err := engine.LocatePage(page)
	if err != nil {
		return err
	}
	if location.Location != page {
		return ErrLifecycleChanged
	}
	return nil
}

func (engine *Engine) saveLifecycle(state lifecycleState) error {
	if err := engine.validateBinding(); err != nil {
		return err
	}
	body, err := engine.lifecycleBody(state)
	if err != nil {
		return err
	}
	_, readErr := readAtlasFile(engine.journalDirPath("lifecycle.json"), maximumPageBytes, true)
	if readErr != nil && !isNotExist(readErr) {
		return readErr
	}
	return publishAtlasFile(engine.journalDirPath("lifecycle.json"), body, readErr != nil && isNotExist(readErr), true)
}

func (engine *Engine) lifecycleIdentity(request lifecycleRequest) (string, string, error) {
	if request.Provenance.Origin != OriginAttended || request.Operation != "move-page" && request.Operation != "retire-page" {
		return "", "", ErrLifecycleChanged
	}
	if err := request.Provenance.validate(); err != nil {
		return "", "", err
	}
	page, err := engine.relativePage(request.Page)
	if err != nil {
		return "", "", err
	}
	id := engine.operationID(request.Operation, page, request.Provenance.IdempotencyKey)
	digest, err := digestRequest(request)
	return id, digest, err
}

func (engine *Engine) reviewLifecycle(request lifecycleRequest) (LifecycleReview, error) {
	var review LifecycleReview
	if engine.workspace == nil || !validDigest(request.ExpectedRevision) {
		return review, ErrLifecycleChanged
	}
	id, requestDigest, err := engine.lifecycleIdentity(request)
	if err != nil {
		return review, err
	}
	state, err := engine.lifecycle()
	if err != nil {
		return review, err
	}
	if state.Pending != nil {
		pending := state.Pending
		if pending.OperationID != id || pending.RequestDigest != requestDigest {
			return review, ErrLifecyclePending
		}
		return engine.reviewPreparedLifecycle(state, *pending)
	}
	if body, readErr := readAtlasFile(engine.journalPath(id), maximumPageBytes, true); readErr == nil {
		var entry lifecycleRecord
		if json.Unmarshal(body, &entry) != nil || entry.SchemaVersion != 3 || entry.Prepared.OperationID != id || entry.Prepared.RequestDigest != requestDigest || engine.validateScope(entry.Workspace) != nil || validateLifecyclePrepared(entry.Prepared) != nil {
			return review, ErrLifecycleChanged
		}
		pending := entry.Prepared
		if !entry.Complete && state.Revision == request.ExpectedLifecycleRevision {
			location, _, err := engine.LocatePage(request.Page)
			if err != nil || location.Location != pending.Source || location.StableID != pending.StableID {
				return review, ErrLifecycleChanged
			}
			source, err := readAtlasFile(engine.pagePath(pending.Source), maximumPageBytes, true)
			if err != nil || digest(string(source)) != pending.Before {
				return review, ErrLifecycleChanged
			}
			if _, err := readAtlasFile(engine.pagePath(pending.Target), maximumPageBytes, true); err == nil || !isNotExist(err) {
				return review, ErrLifecycleChanged
			}
			review, err = engine.reviewPreparedLifecycle(state, pending)
			if err == nil {
				review.prepared = &pending
			}
			return review, err
		}
		location, _, err := engine.LocatePage(request.Page)
		if err != nil && !(request.Operation == "retire-page" && errors.Is(err, ErrPageRetired)) {
			return review, err
		}
		if state.Revision != pending.Result.LifecycleRevision || location.StableID != pending.StableID || location.Location != pending.Result.Location || location.State != pending.Result.PageState {
			return review, ErrLifecycleChanged
		}
		if _, err := readAtlasFile(engine.pagePath(pending.Source), maximumPageBytes, true); err == nil || !isNotExist(err) {
			return review, ErrLifecycleChanged
		}
		target, err := readAtlasFile(engine.pagePath(pending.Target), maximumPageBytes, true)
		if err != nil || digest(string(target)) != pending.After {
			return review, ErrLifecycleChanged
		}
		review, err = engine.reviewPreparedLifecycle(state, pending)
		if err != nil {
			return review, err
		}
		receipt := pending.Result
		receipt.State = StateUnchanged
		review.Receipt = &receipt
		if !entry.Complete {
			review.prepared = &pending
		}
		return review, nil
	} else if !isNotExist(readErr) {
		return review, readErr
	}
	if state.Revision != request.ExpectedLifecycleRevision || state.Revision > 9007199254740989 {
		return review, ErrLifecycleChanged
	}
	source, err := engine.ReviewPage(CollectRequest{Purpose: "Review exact authored relocation", Reader: ReaderOwnerSession, Pages: []string{request.Page}})
	if err != nil {
		return review, err
	}
	location, _, err := engine.LocatePage(request.Page)
	if err != nil {
		return review, err
	}
	if source.Revision != request.ExpectedRevision {
		return review, ErrLifecycleChanged
	}
	target := archivePage(id)
	if request.Operation == "move-page" {
		target, err = engine.relativePage(request.Target)
		if err != nil {
			return review, err
		}
	}
	if strings.EqualFold(source.Page, "README.md") || strings.EqualFold(target, "README.md") {
		return review, errors.New("registered workspace orientation is not relocatable")
	}
	if request.Operation == "move-page" {
		if err = engine.admitWorkspacePage(target); err != nil {
			return review, err
		}
		if existing, err := state.location(target); err != nil || existing.Location != target && existing.StableID != location.StableID || existing.StableID != strings.TrimSuffix(target, ".md") && existing.StableID != location.StableID {
			return review, ErrLifecycleChanged
		}
	}
	if _, err = readAtlasFile(engine.pagePath(target), maximumPageBytes, true); err == nil {
		return review, os.ErrExist
	} else if !isNotExist(err) {
		return review, err
	}
	proposed := source.Content
	if request.Operation == "move-page" {
		proposed, err = atlas.RelocateAuthored(source.Page, target, engine.workspace.WorkspaceID, source.Content, location.StableID)
	} else {
		_, err = atlas.ValidateAuthoredIdentity(source.Page, engine.workspace.WorkspaceID, source.Content, location.StableID)
	}
	if err != nil {
		return review, err
	}
	names := append(append([]string{}, location.Names...), target)
	references, err := engine.affectedReferences(names, source.Page, nil)
	if err != nil {
		return review, err
	}
	referencesDigest, err := digestRequest(references)
	if err != nil {
		return review, err
	}
	current, err := engine.LifecycleRevision()
	if err != nil {
		return review, err
	}
	if current != state.Revision {
		return review, ErrLifecycleChanged
	}
	return LifecycleReview{Operation: request.Operation, Source: source, StableID: location.StableID, Target: target, Proposed: proposed, Revision: state.Revision, References: references, ReferencesDigest: referencesDigest}, nil
}

func (engine *Engine) reviewPreparedLifecycle(state lifecycleState, pending lifecyclePrepared) (LifecycleReview, error) {
	if err := engine.admitWorkspacePage(pending.Source); err != nil {
		return LifecycleReview{}, err
	}
	if pending.Request.Operation == "move-page" {
		if err := engine.admitWorkspacePage(pending.Target); err != nil {
			return LifecycleReview{}, err
		}
	}
	var body []byte
	for _, page := range []string{pending.Source, pending.Target} {
		current, err := readAtlasFile(engine.pagePath(page), maximumPageBytes, true)
		if err != nil && isNotExist(err) {
			continue
		}
		if err != nil {
			return LifecycleReview{}, err
		}
		if len(body) != 0 || digest(string(current)) != pending.Before && digest(string(current)) != pending.After {
			return LifecycleReview{}, ErrLifecycleChanged
		}
		body = current
	}
	if len(body) == 0 {
		return LifecycleReview{}, ErrLifecycleChanged
	}
	proposed := string(body)
	if digest(proposed) == pending.Before && pending.Request.Operation == "move-page" {
		var err error
		proposed, err = atlas.RelocateAuthored(pending.Source, pending.Target, engine.workspace.WorkspaceID, proposed, pending.StableID)
		if err != nil {
			return LifecycleReview{}, err
		}
	}
	if digest(proposed) != pending.After {
		return LifecycleReview{}, ErrLifecycleChanged
	}
	selected := PageProjection{Page: pending.Source, Revision: digest(string(body)), Content: string(body), StableID: pending.StableID, LifecycleRevision: state.Revision}
	names := append([]string{pending.Source, pending.Target}, state.Pages[pending.StableID].Names...)
	references, err := engine.affectedReferences(names, pending.Source, &selected)
	if err != nil {
		return LifecycleReview{}, err
	}
	hash, err := digestRequest(references)
	return LifecycleReview{Operation: pending.Request.Operation, Source: selected, StableID: pending.StableID, Target: pending.Target, Proposed: proposed, Revision: state.Revision, References: references, ReferencesDigest: hash}, err
}

func (engine *Engine) applyLifecycle(request lifecycleRequest) (Result, error) {
	release, err := engine.beginMutation()
	if err != nil {
		return Result{}, err
	}
	defer release()
	review, err := engine.reviewLifecycle(request)
	if err != nil {
		return Result{}, err
	}
	if review.Receipt != nil {
		if review.prepared != nil {
			if engine.workspacePolicy.BeforeMutation != nil {
				if err = engine.workspacePolicy.BeforeMutation(); err != nil {
					return Result{}, err
				}
			}
			entry, _ := json.Marshal(lifecycleRecord{SchemaVersion: 3, Workspace: engine.workspace, Prepared: *review.prepared, Complete: true})
			if err = publishAtlasFile(engine.journalPath(review.prepared.OperationID), entry, false, true); err != nil {
				return Result{}, err
			}
		}
		return *review.Receipt, nil
	}
	if request.ReferencesDigest == "" || request.ReferencesDigest != review.ReferencesDigest {
		return Result{}, ErrLifecycleChanged
	}
	state, err := engine.lifecycle()
	if err != nil {
		return Result{}, err
	}
	id, requestDigest, err := engine.lifecycleIdentity(request)
	if err != nil {
		return Result{}, err
	}
	var pending lifecyclePrepared
	if state.Pending != nil {
		pending = *state.Pending
	} else if review.prepared != nil {
		pending = *review.prepared
	} else {
		location, pageState := review.Target, "active"
		if request.Operation == "retire-page" {
			location, pageState = review.Source.Page, "retired"
		}
		result := Result{PageState: pageState, Operation: request.Operation, Page: review.Source.Page, State: StateWritten, Revision: digest(review.Proposed), OperationID: id, RecordedAt: engine.now().UTC(), Provenance: request.Provenance, StableID: review.StableID, Location: location, LifecycleRevision: state.Revision + 2}
		pending = lifecyclePrepared{OperationID: id, RequestDigest: requestDigest, Request: request, StableID: review.StableID, Source: review.Source.Page, Target: review.Target, Before: review.Source.Revision, After: digest(review.Proposed), Result: result}
	}
	terminal := terminalLifecycle(state, pending)
	if _, err = engine.lifecycleBody(terminal); err != nil {
		return Result{}, err
	}
	if state.Pending == nil {
		barrier := state
		barrier.Pending, barrier.Revision = &pending, state.Revision+1
		if _, err = engine.lifecycleBody(barrier); err != nil {
			return Result{}, err
		}
		if err = engine.authorize(request.Provenance, request.Operation, review.Source.Page); err != nil {
			return Result{}, err
		}
		if review.prepared == nil {
			entry, _ := json.Marshal(lifecycleRecord{SchemaVersion: 3, Workspace: engine.workspace, Prepared: pending})
			if err = publishAtlasFile(engine.journalPath(id), entry, true, true); err != nil {
				return Result{}, err
			}
			if err = engine.lifecyclePhase("prepared"); err != nil {
				return Result{}, err
			}
		}
		state = barrier
		if err = engine.saveLifecycle(state); err != nil {
			return Result{}, err
		}
		if err = engine.lifecyclePhase("barrier"); err != nil {
			return Result{}, err
		}
	}
	if err = engine.validateBinding(); err != nil {
		return Result{}, err
	}
	if err = engine.admitWorkspacePage(pending.Source); err != nil {
		return Result{}, err
	}
	if pending.Request.Operation == "move-page" {
		if err = engine.admitWorkspacePage(pending.Target); err != nil {
			return Result{}, err
		}
	}
	if engine.workspacePolicy.BeforeMutation != nil {
		if err = engine.workspacePolicy.BeforeMutation(); err != nil {
			return Result{}, err
		}
	}
	if err = scheduler.EnsurePrivateDirectory(filepath.Dir(engine.pagePath(pending.Target))); err != nil {
		return Result{}, err
	}
	if body, readErr := readAtlasFile(engine.pagePath(pending.Source), maximumPageBytes, true); readErr == nil {
		if digest(string(body)) != pending.Before {
			return Result{}, ErrLifecycleChanged
		}
		if err = moveAtlasFile(engine.pagePath(pending.Source), engine.pagePath(pending.Target), pending.Before); err != nil {
			return Result{}, err
		}
		if engine.afterPageEffect != nil {
			if err = engine.afterPageEffect(); err != nil {
				return Result{}, err
			}
		}
	} else if !isNotExist(readErr) {
		return Result{}, readErr
	}
	body, err := readAtlasFile(engine.pagePath(pending.Target), maximumPageBytes, true)
	if err != nil {
		return Result{}, err
	}
	if digest(string(body)) == pending.Before && pending.Before != pending.After {
		if err = engine.validateBinding(); err != nil {
			return Result{}, err
		}
		if err = engine.admitWorkspacePage(pending.Target); err != nil {
			return Result{}, err
		}
		if engine.workspacePolicy.BeforeMutation != nil {
			if err = engine.workspacePolicy.BeforeMutation(); err != nil {
				return Result{}, err
			}
		}
		if err = publishAtlasFile(engine.pagePath(pending.Target), []byte(review.Proposed), false, true); err != nil {
			return Result{}, err
		}
		if engine.afterPageEffect != nil {
			if err = engine.afterPageEffect(); err != nil {
				return Result{}, err
			}
		}
	} else if digest(string(body)) != pending.After {
		return Result{}, ErrLifecycleChanged
	}
	if _, err = readAtlasFile(engine.pagePath(pending.Source), maximumPageBytes, true); err == nil || !isNotExist(err) {
		return Result{}, ErrLifecycleChanged
	}
	if err = engine.validateBinding(); err != nil {
		return Result{}, err
	}
	if pending.Request.Operation == "move-page" {
		if err = engine.admitWorkspacePage(pending.Target); err != nil {
			return Result{}, err
		}
	}
	if engine.workspacePolicy.BeforeMutation != nil {
		if err = engine.workspacePolicy.BeforeMutation(); err != nil {
			return Result{}, err
		}
	}
	state = terminal
	if err = engine.saveLifecycle(state); err != nil {
		return Result{}, err
	}
	if err = engine.lifecyclePhase("lifecycle"); err != nil {
		return Result{}, err
	}
	entry, _ := json.Marshal(lifecycleRecord{SchemaVersion: 3, Workspace: engine.workspace, Prepared: pending, Complete: true})
	if err = publishAtlasFile(engine.journalPath(id), entry, false, true); err != nil {
		return Result{}, err
	}
	if err = engine.lifecyclePhase("receipt"); err != nil {
		return Result{}, err
	}
	return pending.Result, nil
}

func terminalLifecycle(state lifecycleState, pending lifecyclePrepared) lifecycleState {
	terminal := state
	terminal.Pages = make(map[string]PageLocation, len(state.Pages)+1)
	for id, page := range state.Pages {
		terminal.Pages[id] = page
	}
	location, exists := state.Pages[pending.StableID]
	if !exists {
		location = PageLocation{StableID: pending.StableID, Names: []string{pending.Source}}
	}
	location.Names = append([]string{}, location.Names...)
	location.Location, location.State = pending.Result.Location, pending.Result.PageState
	found := false
	for _, name := range location.Names {
		found = found || canonicalPageName(name) == canonicalPageName(pending.Target)
	}
	if !found && pending.Request.Operation == "move-page" {
		location.Names = append(location.Names, pending.Target)
		sort.Strings(location.Names)
	}
	terminal.Pages[location.StableID], terminal.Pending, terminal.Revision = location, nil, pending.Result.LifecycleRevision
	return terminal
}

// ExpectLifecycleRevision installs the host's private reviewed state fence.
func (engine *Engine) ExpectLifecycleRevision(revision uint64) {
	engine.workspacePolicy.ExpectedLifecycleRevision = &revision
}

// Alias keys use a portable fold; displayed spellings and authored IDs stay exact.
func canonicalPageName(name string) string {
	var out strings.Builder
	for _, r := range norm.NFC.String(name) {
		first, best := r, r
		for next := unicode.SimpleFold(r); next != first; next = unicode.SimpleFold(next) {
			if next < best {
				best = next
			}
		}
		out.WriteRune(best)
	}
	return out.String()
}

func archivePage(operationID string) string {
	return journalDirectory + "/retired/" + operationID + ".md"
}

func (engine *Engine) lifecyclePhase(phase string) error {
	if engine.afterLifecyclePhase != nil {
		return engine.afterLifecyclePhase(phase)
	}
	return nil
}
