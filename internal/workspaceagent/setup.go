package workspaceagent

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/agentscaffold"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/atlas"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/execution"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspace"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspacepolicy"
)

var ErrSetupChanged = errors.New("case_setup_changed")

type SetupInput struct {
	Mode           string `json:"mode"`
	ProjectName    string `json:"project_name"`
	AccountID      string `json:"account_id,omitempty"`
	AccountOwner   string `json:"account_owner,omitempty"`
	AccountMandate string `json:"account_mandate,omitempty"`
}
type SetupWork struct {
	Objective string `json:"objective"`
	NextStep  string `json:"next_step"`
	Summary   string `json:"summary"`
}
type SetupState struct {
	SchemaVersion  int        `json:"schema_version"`
	WorkspaceID    string     `json:"workspace_id"`
	SetupID        string     `json:"setup_id"`
	Input          SetupInput `json:"input"`
	State          string     `json:"state"`
	BriefPage      string     `json:"brief_page"`
	CaseAgentID    string     `json:"case_agent_id"`
	CaseID         string     `json:"case_id"`
	AccountAgentID string     `json:"account_agent_id,omitempty"`
	BriefRevision  string     `json:"brief_revision,omitempty"`
	Work           *SetupWork `json:"work,omitempty"`
	ItemID         string     `json:"item_id,omitempty"`
	CleanupPending string     `json:"cleanup_pending_preimage,omitempty"`
	CleanupActive  string     `json:"cleanup_active_preimage,omitempty"`
}
type SetupStatus struct {
	State         string                      `json:"state"`
	WorkspaceID   string                      `json:"workspace_id"`
	SetupRevision string                      `json:"setup_revision"`
	Setup         *SetupState                 `json:"setup,omitempty"`
	Markers       workspacepolicy.CaseMarkers `json:"selection"`
	Ledger        map[string]any              `json:"ledger,omitempty"`
	Registrations map[string]string           `json:"registrations,omitempty"`
}
type SetupReview struct {
	Operation string                    `json:"operation"`
	Current   SetupStatus               `json:"current"`
	Proposed  SetupState                `json:"proposed"`
	Account   *agentscaffold.Enrollment `json:"account,omitempty"`
	Case      agentscaffold.Enrollment  `json:"case"`
	Page      *SetupPage                `json:"brief,omitempty"`
	Work      *SetupWork                `json:"requested_work,omitempty"`
}
type SetupPage struct {
	Page     string `json:"page"`
	Revision string `json:"revision"`
	Text     string `json:"text"`
}

// SetupOwner is confined to an already admitted workspace; the adapter owns
// binding admission. All handles are acquired before review and held until close.
type SetupOwner struct {
	Root, Workspace, WorkspaceID string
	pins                         []*os.File
	dataDirectory                *scheduler.HeldDirectory
	brainDirectory               *scheduler.HeldDirectory
	// Internal interruption seam, never serialized or adapter selectable.
	FaultPoint func(string) error
}

func OpenSetup(root, path, id string) (*SetupOwner, error) {
	if !workspaceIDPattern.MatchString(id) || len(id) != 32 {
		return nil, errors.New("case_workspace_invalid")
	}
	owner := &SetupOwner{Root: root, Workspace: path, WorkspaceID: id}
	for _, p := range []string{root, path, filepath.Join(path, "brain")} {
		f, err := scheduler.OpenDirectory(p)
		if err != nil {
			owner.Close()
			return nil, err
		}
		owner.pins = append(owner.pins, f)
	}
	var e error
	owner.dataDirectory, e = scheduler.HoldDirectory(root, owner.pins[0])
	if e != nil {
		owner.Close()
		return nil, e
	}
	owner.brainDirectory, e = scheduler.HoldDirectory(filepath.Join(path, "brain"), owner.pins[2])
	if e != nil {
		owner.Close()
		return nil, e
	}
	if err := owner.validate(); err != nil {
		owner.Close()
		return nil, err
	}
	return owner, nil
}
func (o *SetupOwner) Close() {
	if o.dataDirectory != nil {
		o.dataDirectory.Close()
		o.dataDirectory = nil
	}
	if o.brainDirectory != nil {
		o.brainDirectory.Close()
		o.brainDirectory = nil
	}
	for _, p := range o.pins {
		p.Close()
	}
	o.pins = nil
}
func (o *SetupOwner) validate() error {
	for i, p := range []string{o.Root, o.Workspace, filepath.Join(o.Workspace, "brain")} {
		if i >= len(o.pins) {
			return errors.New("case_custody_closed")
		}
		f, err := scheduler.OpenDirectory(p)
		if err != nil {
			return err
		}
		actual, e := f.Stat()
		f.Close()
		original, e2 := o.pins[i].Stat()
		if e != nil || e2 != nil || !os.SameFile(actual, original) {
			return errors.New("case_binding_changed")
		}
	}
	path, err := workspace.ResolveReference(o.WorkspaceID, o.Root)
	if err != nil || path != o.Workspace {
		return errors.New("case_binding_changed")
	}
	return scheduler.ValidatePrivateDirectory(o.Root)
}
func setupDigest(v any) string {
	body, _ := json.Marshal(v)
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}
func (o *SetupOwner) path() string {
	return filepath.Join(o.Root, "workspaces", o.WorkspaceID, "agent", "setup.json")
}
func strictSetup(body []byte, target any) error {
	d := json.NewDecoder(bytes.NewReader(body))
	d.DisallowUnknownFields()
	if err := d.Decode(target); err != nil {
		return err
	}
	if d.Decode(new(any)) != io.EOF {
		return errors.New("case_setup_invalid")
	}
	return nil
}
func (o *SetupOwner) Status() (SetupStatus, error) {
	out := SetupStatus{State: "absent", WorkspaceID: o.WorkspaceID, SetupRevision: "absent"}
	if err := o.validate(); err != nil {
		return out, err
	}
	if err := agentscaffold.ValidateEnrollmentStoreAt(o.dataDirectory); err != nil {
		return out, err
	}
	markers, err := workspacepolicy.SnapshotCaseMarkersAt(o.brainDirectory)
	if err != nil {
		return out, err
	}
	out.Markers = markers
	body, err := o.readData(o.path(), 64<<10)
	if errors.Is(err, os.ErrNotExist) {
		if markers.Active != "" || markers.Pending != "" {
			out.State = "reconciliation_required"
			out.SetupRevision = setupDigest(markers)
		}
		// A legacy gatekeeper is allowed, but corrupt dependencies never become fresh state.
		if _, e := o.readData(filepath.Join(filepath.Dir(o.path()), "agent.json"), 64<<10); e == nil {
			if _, e = o.gatekeeper(false); e != nil {
				return out, e
			}
		} else if !errors.Is(e, os.ErrNotExist) {
			return out, e
		}
		return out, nil
	}
	if err != nil {
		return out, err
	}
	var state SetupState
	if err = strictSetup(body, &state); err != nil {
		return out, err
	}
	if state.CaseID == "" {
		return out, errors.New("case_setup_invalid")
	}
	if state.SchemaVersion != 1 || state.WorkspaceID != o.WorkspaceID || state.SetupID != setupDigest([]string{"maestro-first-case", o.WorkspaceID}) || state.CaseAgentID == "" || state.BriefPage == "" {
		return out, errors.New("case_setup_invalid")
	}
	if state.State != "preparing" && state.State != "pending" && state.State != "active" && state.State != "active_cleanup_pending" {
		return out, errors.New("case_setup_invalid")
	}
	if _, err := o.gatekeeper(false); err != nil && !(state.State == "preparing" && errors.Is(err, os.ErrNotExist)) {
		return out, err
	}
	if state.Input.Mode != "client" && state.Input.Mode != "internal" {
		return out, errors.New("case_setup_invalid")
	}
	expectedPage := "projects/" + o.WorkspaceID + ".md"
	if state.Input.Mode == "client" {
		expectedPage = "accounts/" + state.Input.AccountID + "/cases/" + state.CaseID + "/" + state.CaseID + ".md"
	}
	if state.BriefPage != expectedPage {
		return out, errors.New("case_setup_invalid")
	}
	if state.Input.Mode == "internal" && (state.CaseID != o.WorkspaceID || state.AccountAgentID != "" || state.Input.AccountID != "" || state.Input.AccountOwner != "" || state.Input.AccountMandate != "") {
		return out, errors.New("case_internal_scope_conflict")
	}
	if state.Input.Mode == "internal" && (markers.Active != "" || markers.Pending != "") {
		return out, errors.New("case_selection_conflict")
	}
	out.Registrations = map[string]string{"case": "absent", "account": "not_requested"}
	for _, entry := range []struct{ kind, id string }{{"account", state.AccountAgentID}, {"case", state.CaseAgentID}} {
		if entry.id == "" {
			continue
		}
		registration, e := agentscaffold.InspectAt(o.dataDirectory, entry.id)
		if errors.Is(e, os.ErrNotExist) {
			out.Registrations[entry.kind] = "absent"
			if state.State != "preparing" {
				return out, errors.New("case_registration_missing")
			}
			continue
		}
		if e != nil {
			return out, e
		}
		if entry.kind == "account" && (registration.Instance.Role != "client_account_agent" || registration.Instance.ScopeKind != "account" || registration.Instance.ScopeID != state.Input.AccountID) {
			return out, errors.New("case_registration_changed")
		}
		if entry.kind == "case" && (registration.Instance.Role != "case_agent" || state.Input.Mode == "internal" && registration.Instance.ScopeKind != "workspace" || state.Input.Mode == "client" && registration.Instance.ScopeKind != "case") {
			return out, errors.New("case_registration_changed")
		}
		if entry.kind == "case" && (registration.Instance.ScopeID != state.CaseID || registration.Instance.AccountAgentID != state.AccountAgentID) {
			return out, errors.New("case_registration_changed")
		}
		out.Registrations[entry.kind] = "scaffolded_runtime_unavailable"
	}
	if state.Work != nil {
		store, e := o.ledgerStore()
		if e != nil {
			return out, e
		}
		item, e := store.InspectForSetup(state.SetupID, o.workInput(state, *state.Work))
		if e == nil {
			out.Ledger = map[string]any{"item_id": item.Contract.ItemID, "state": item.State.State, "state_revision": item.State.StateRevision}
		} else if !errors.Is(e, os.ErrNotExist) {
			return out, e
		}
	}
	out.Setup = &state
	out.State = state.State
	if state.Input.Mode == "client" {
		pair := state.Input.AccountID + "/" + state.CaseID
		if markers.Active != "" && workspacepolicy.MarkerPair(markers.Active) != pair || markers.Pending != "" && workspacepolicy.MarkerPair(markers.Pending) != pair {
			return out, errors.New("case_selection_conflict")
		}
		if workspacepolicy.MarkerPair(markers.Active) == pair && state.State != "active" && state.BriefRevision != "" {
			out.State = "active_cleanup_pending"
		}
		if state.State == "active" && (workspacepolicy.MarkerPair(markers.Active) != pair || markers.Pending != "") {
			return out, errors.New("case_selection_changed")
		}
	}
	out.SetupRevision = setupDigest(struct {
		Body    string
		Markers workspacepolicy.CaseMarkers
	}{string(body), markers})
	return out, o.validate()
}
func (o *SetupOwner) requests(in SetupInput) (SetupState, *agentscaffold.Enrollment, agentscaffold.Enrollment, error) {
	var s SetupState
	var account *agentscaffold.Enrollment
	var cp agentscaffold.Enrollment
	if in.Mode != "client" && in.Mode != "internal" || strings.TrimSpace(in.ProjectName) == "" || len(in.ProjectName) > 256 || strings.ContainsAny(in.ProjectName, "\x00\r\n") {
		return s, nil, cp, errors.New("case_input_invalid")
	}
	s = SetupState{SchemaVersion: 1, WorkspaceID: o.WorkspaceID, SetupID: setupDigest([]string{"maestro-first-case", o.WorkspaceID}), Input: in, State: "preparing", CaseAgentID: "case-agent-" + o.WorkspaceID, CaseID: o.WorkspaceID, BriefPage: "projects/" + o.WorkspaceID + ".md"}
	request := agentscaffold.Request{AgentID: s.CaseAgentID, Role: "case_agent", ScopeKind: "workspace", ScopeID: o.WorkspaceID, ParentAgent: "maestro", ParentRole: "hub"}
	if in.Mode == "client" {
		request.ScopeKind = "case"
		request.AccountAgentID = "client-account-agent-" + in.AccountID
		if _, e := agentscaffold.InspectAt(o.dataDirectory, request.AccountAgentID); errors.Is(e, os.ErrNotExist) {
			if legacy, e := agentscaffold.InspectAt(o.dataDirectory, "account-agent-"+in.AccountID); e == nil {
				request.AccountAgentID = legacy.Instance.AgentID
			} else if !errors.Is(e, os.ErrNotExist) {
				return s, nil, cp, e
			}
		} else if e != nil {
			return s, nil, cp, e
		}
		a, err := agentscaffold.PreviewEnrollmentAt(o.dataDirectory, agentscaffold.Request{AgentID: request.AccountAgentID, Role: "client_account_agent", ScopeKind: "account", ScopeID: in.AccountID, ParentAgent: "maestro", ParentRole: "hub", Owner: in.AccountOwner, Mandate: in.AccountMandate})
		if err != nil {
			return s, nil, cp, err
		}
		account = &a
		s.AccountAgentID = request.AccountAgentID
		s.BriefPage = "accounts/" + in.AccountID + "/cases/" + o.WorkspaceID + "/" + o.WorkspaceID + ".md"
	} else if in.AccountID != "" || in.AccountOwner != "" || in.AccountMandate != "" {
		return s, nil, cp, errors.New("internal_case_has_account")
	}
	cp, err := agentscaffold.PreviewEnrollmentAt(o.dataDirectory, request)
	return s, account, cp, err
}
func (o *SetupOwner) ReviewPrepare(in SetupInput, expected string) (SetupReview, error) {
	var r SetupReview
	current, err := o.Status()
	if err != nil {
		return r, err
	}
	if current.SetupRevision != expected {
		return r, ErrSetupChanged
	}

	proposed, a, c, err := o.requests(in)
	if err != nil {
		return r, err
	}
	if current.State == "reconciliation_required" {
		parts := strings.Split(workspacepolicy.MarkerPair(current.Markers.Active), "/")
		if in.Mode != "client" || len(parts) != 2 || parts[0] != in.AccountID || current.Markers.Pending != "" {
			return r, errors.New("case_reconciliation_required")
		}
		prior, e := agentscaffold.InspectAt(o.dataDirectory, "case-agent-"+parts[1])
		if e != nil || prior.Instance.ScopeKind != "case" || prior.Instance.ScopeID != parts[1] || prior.Instance.AccountAgentID != proposed.AccountAgentID {
			return r, errors.New("case_reconciliation_required")
		}
		proposed.CaseID = parts[1]
		proposed.CaseAgentID = prior.Instance.AgentID
		proposed.BriefPage = "accounts/" + in.AccountID + "/cases/" + parts[1] + "/" + parts[1] + ".md"
		request := c.Request
		request.AgentID = prior.Instance.AgentID
		request = existingSetupIdentity(request, prior.Instance)
		request.ScopeID = parts[1]
		c, e = agentscaffold.PreviewEnrollmentAt(o.dataDirectory, request)
		if e != nil {
			return r, e
		}
	}
	if current.Setup == nil && in.Mode == "internal" {
		prior, e := agentscaffold.InspectAt(o.dataDirectory, "workspace-agent-"+o.WorkspaceID)
		if e == nil {
			request := c.Request
			request.AgentID = prior.Instance.AgentID
			request = existingSetupIdentity(request, prior.Instance)
			c, e = agentscaffold.PreviewEnrollmentAt(o.dataDirectory, request)
			if e != nil {
				return r, e
			}
			proposed.CaseAgentID = prior.Instance.AgentID
		} else if !errors.Is(e, os.ErrNotExist) {
			return r, e
		}
	}
	if current.Setup != nil {
		if current.Setup.Input != in {
			return r, errors.New("case_immutable_binding")
		}
		proposed = *current.Setup
		if c.Request.AgentID != proposed.CaseAgentID || c.Request.ScopeID != proposed.CaseID {
			request := c.Request
			request.AgentID = proposed.CaseAgentID
			if existing, e := agentscaffold.InspectAt(o.dataDirectory, request.AgentID); e == nil {
				request = existingSetupIdentity(request, existing.Instance)
			} else {
				return r, e
			}
			request.ScopeID = proposed.CaseID
			c, err = agentscaffold.PreviewEnrollmentAt(o.dataDirectory, request)
			if err != nil {
				return r, err
			}
		}
	}
	if in.Mode == "internal" && (current.Markers.Active != "" || current.Markers.Pending != "") {
		return r, errors.New("case_selection_conflict")
	}
	if current.Setup == nil && current.State != "reconciliation_required" && (current.Markers.Active != "" || current.Markers.Pending != "") {
		return r, errors.New("case_selection_conflict")
	}
	return SetupReview{Operation: "case.setup.prepare", Current: current, Proposed: proposed, Account: a, Case: c}, nil
}
func (o *SetupOwner) save(state SetupState, guard func() error, action func(*SetupState) error) (SetupStatus, error) {
	var effectErr error
	err := o.updateData(o.path(), 64<<10, func(body []byte) ([]byte, error) {
		if err := o.validate(); err != nil {
			return nil, err
		}
		if err := guard(); err != nil {
			return nil, err
		}
		effectErr = action(&state)
		next, err := json.Marshal(state)
		if err != nil {
			return nil, err
		}
		return next, nil
	})
	if err != nil {
		return SetupStatus{}, err
	}
	out, err := o.Status()
	if err != nil {
		return out, err
	}
	return out, effectErr
}
func (o *SetupOwner) gatekeeper(create bool) (Status, error) {
	dir := filepath.Dir(o.path())
	if create {
		for _, entry := range []struct {
			path  string
			value any
		}{{"agent/agent.json", registry{1, o.WorkspaceID, "workspace-agent-" + o.WorkspaceID, "case_agent"}}, {"agent/state.json", OperationalState{SchemaVersion: 1, WorkspaceID: o.WorkspaceID, Lifecycle: "setup", UpdatedAt: time.Now().UTC()}}, {"dossier/README.md", dossierTemplate}} {
			path := filepath.Join(o.Root, "workspaces", o.WorkspaceID, entry.path)
			if err := o.ensureData(filepath.Dir(path)); err != nil {
				return Status{}, err
			}
			var body []byte
			if text, ok := entry.value.(string); ok {
				body = []byte(text)
			} else {
				body, _ = json.Marshal(entry.value)
			}
			if err := o.publishData(path, body); err != nil && !errors.Is(err, os.ErrExist) {
				return Status{}, err
			}
		}
	}
	var reg registry
	body, err := o.readData(filepath.Join(dir, "agent.json"), 64<<10)
	if err != nil {
		return Status{}, err
	}
	if err = strictSetup(body, &reg); err != nil {
		return Status{}, err
	}
	if reg.SchemaVersion != 1 || reg.WorkspaceID != o.WorkspaceID || reg.AgentID != "workspace-agent-"+o.WorkspaceID || (reg.Role != "case_agent" && reg.Role != "workspace_agent" && reg.Role != "") {
		return Status{}, errors.New("case_gatekeeper_invalid")
	}
	var operational OperationalState
	stateBody, e := o.readData(filepath.Join(dir, "state.json"), 64<<10)
	if e != nil {
		return Status{}, e
	}
	if e = strictSetup(stateBody, &operational); e != nil {
		return Status{}, e
	}
	if operational.SchemaVersion != 1 || operational.WorkspaceID != o.WorkspaceID {
		return Status{}, errors.New("case_gatekeeper_invalid")
	}
	for _, path := range []string{filepath.Join(filepath.Dir(dir), "dossier", "README.md")} {
		if _, err := o.readData(path, 64<<10); err != nil {
			return Status{}, err
		}
	}
	return Status{Initialized: true, WorkspaceID: o.WorkspaceID, AgentID: reg.AgentID, Role: "case_agent"}, nil
}
func (o *SetupOwner) ApplyPrepare(review SetupReview) (SetupStatus, error) {
	staged, err := o.stageIntent(review, nil)
	if err != nil {
		return SetupStatus{}, err
	}
	return o.applyPrepared(staged)
}
func (o *SetupOwner) applyPrepared(review SetupReview) (SetupStatus, error) {
	// One domain lock orders setup -> local enrollment -> selector files -> ledger.
	// No callback below asks a human; the adapter has already completed elicitation.
	return o.save(review.Proposed, func() error {
		fresh, err := o.ReviewPrepare(review.Proposed.Input, review.Current.SetupRevision)
		if err != nil {
			return err
		}
		if setupDigest(fresh) != setupDigest(review) {
			return ErrSetupChanged
		}
		return nil
	}, func(state *SetupState) error {
		_, err := o.gatekeeper(true)
		if err != nil {
			return err
		}
		if review.Account != nil {
			if _, err = agentscaffold.RegisterEnrollmentAt(o.dataDirectory, *review.Account); err != nil {
				return err
			}
		}
		if _, err = agentscaffold.RegisterEnrollmentAt(o.dataDirectory, review.Case); err != nil {
			return err
		}
		if o.FaultPoint != nil {
			if err = o.FaultPoint("registration"); err != nil {
				return err
			}
		}
		if state.Input.Mode == "client" && review.Current.State != "active" && review.Current.State != "active_cleanup_pending" && review.Current.State != "reconciliation_required" {
			pair := state.Input.AccountID + "/" + state.CaseID
			if err = workspacepolicy.PrepareCaseSelectionAt(o.brainDirectory, pair, review.Current.Markers); err != nil {
				return err
			}
		}
		if state.State == "preparing" {
			state.State = "pending"
		}
		return o.validate()
	})
}
func (o *SetupOwner) ReviewFinish(expected string, page SetupPage, work *SetupWork) (SetupReview, error) {
	var r SetupReview
	current, err := o.Status()
	if err != nil {
		return r, err
	}
	if current.SetupRevision != expected {
		return r, ErrSetupChanged
	}
	if current.Setup == nil {
		return r, errors.New("case_prepare_required")
	}
	state := *current.Setup
	if state.State == "preparing" {
		return r, errors.New("case_prepare_incomplete")
	}
	if current.State == "active_cleanup_pending" && (current.Markers.Active != state.CleanupActive || current.Markers.Pending != "" && current.Markers.Pending != state.CleanupPending) {
		return r, ErrSetupChanged
	}
	if page.Page != state.BriefPage || len(page.Revision) != 64 || page.Text == "" {
		return r, errors.New("case_brief_required")
	}
	metadata, err := atlas.ValidateAuthoredPage(page.Page, o.WorkspaceID, page.Text)
	if err != nil {
		return r, err
	}
	if metadata["type"] != "project-brief" {
		return r, errors.New("case_brief_type_invalid")
	}
	_, a, c, err := o.requests(state.Input)
	if err == nil && (c.Request.AgentID != state.CaseAgentID || c.Request.ScopeID != state.CaseID) {
		request := c.Request
		request.AgentID = state.CaseAgentID
		if existing, e := agentscaffold.InspectAt(o.dataDirectory, request.AgentID); e == nil {
			request = existingSetupIdentity(request, existing.Instance)
		} else {
			return r, e
		}
		request.ScopeID = state.CaseID
		c, err = agentscaffold.PreviewEnrollmentAt(o.dataDirectory, request)
	}
	if err != nil {
		return r, err
	}
	if _, err = agentscaffold.InspectAt(o.dataDirectory, state.CaseAgentID); err != nil {
		return r, err
	}
	if work != nil {
		normalized := SetupWork{Objective: strings.TrimSpace(work.Objective), NextStep: strings.TrimSpace(work.NextStep), Summary: strings.TrimSpace(work.Summary)}
		work = &normalized
	}
	if state.Work != nil && (work == nil || *work != *state.Work) {
		return r, execution.ErrContractChanged
	}
	if work != nil && (strings.TrimSpace(work.Objective) == "" || len(work.Objective) > 4096 || strings.TrimSpace(work.NextStep) == "" || len(work.NextStep) > 768 || strings.TrimSpace(work.Summary) == "" || len(work.Summary) > 768) {
		return r, errors.New("case_work_invalid")
	}
	if work != nil {
		if e := execution.ValidateSetupCheckpoint(state.SetupID, o.workInput(state, *work), work.Summary); e != nil {
			return r, errors.New("case_work_projection_invalid")
		}
	}
	return SetupReview{Operation: "case.setup.finish", Current: current, Proposed: state, Account: a, Case: c, Page: &page, Work: work}, nil
}
func (o *SetupOwner) ApplyFinish(review SetupReview, readPage func() (SetupPage, error)) (SetupStatus, error) {
	staged, err := o.stageIntent(review, readPage)
	if err != nil {
		return SetupStatus{}, err
	}
	return o.applyFinished(staged, readPage)
}
func (o *SetupOwner) applyFinished(review SetupReview, readPage func() (SetupPage, error)) (SetupStatus, error) {
	return o.save(review.Proposed, func() error {
		page, err := readPage()
		if err != nil {
			return err
		}
		fresh, err := o.ReviewFinish(review.Current.SetupRevision, page, review.Work)
		if err != nil {
			return err
		}
		if setupDigest(fresh) != setupDigest(review) {
			return ErrSetupChanged
		}
		return nil
	}, func(state *SetupState) error {
		var err error
		state.BriefRevision = review.Page.Revision
		if review.Work != nil {
			state.Work = review.Work
			item, err := o.setupWork(*state, *review.Work)
			if err != nil {
				return err
			}
			state.ItemID = item.Contract.ItemID
		}
		currentPage, e := readPage()
		if e != nil {
			return e
		}
		fresh, e := o.ReviewFinish(review.Current.SetupRevision, currentPage, review.Work)
		if e != nil {
			return e
		}
		if setupDigest(fresh.Page) != setupDigest(review.Page) || setupDigest(fresh.Case) != setupDigest(review.Case) || setupDigest(fresh.Account) != setupDigest(review.Account) {
			return ErrSetupChanged
		}
		if state.Input.Mode == "internal" {
			state.State = "active"
			return o.validate()
		}
		pair := state.Input.AccountID + "/" + state.CaseID
		if workspacepolicy.MarkerPair(review.Current.Markers.Active) != pair {
			if err = workspacepolicy.CommitCaseSelectionAt(o.brainDirectory, pair, review.Current.Markers); err != nil {
				return err
			}
		}
		state.State = "active_cleanup_pending"
		if o.FaultPoint != nil {
			if err = o.FaultPoint("active_publication"); err != nil {
				return err
			}
		}
		if review.Current.Markers.Pending != "" {
			exact := workspacepolicy.CaseMarkers{Active: state.CleanupActive, Pending: state.CleanupPending}
			if err = workspacepolicy.CleanCaseSelectionAt(o.brainDirectory, pair, exact); err != nil {
				return err
			}
		}
		if o.FaultPoint != nil {
			if err = o.FaultPoint("pending_cleanup"); err != nil {
				return err
			}
		}
		state.State = "active"
		return o.validate()
	})
}
func (o *SetupOwner) setupWork(state SetupState, work SetupWork) (execution.Item, error) {
	store, err := o.ledgerStore()
	if err != nil {
		return execution.Item{}, err
	}
	ref := "bcgos://workspace/brain/" + state.BriefPage
	item, err := store.CreateForSetup(state.SetupID, o.workInput(state, work))
	if err != nil {
		return item, err
	}
	if o.FaultPoint != nil {
		if e := o.FaultPoint("ledger_create_before_link"); e != nil {
			return item, e
		}
	}
	if item.State.State == execution.StateReady && item.State.StateRevision == 1 {
		item, err = store.Start(o.WorkspaceID, item.Contract.ItemID, 1)
		if err != nil {
			return item, err
		}
	}
	if item.State.State == execution.StateRunning && item.State.StateRevision == 2 && item.Checkpoint == nil {
		item, err = store.Checkpoint(o.WorkspaceID, item.Contract.ItemID, execution.CheckpointInput{ExpectedRevision: 2, AttemptID: item.State.ActiveAttemptID, Summary: work.Summary, NextStep: work.NextStep, ArtifactRefs: []string{ref}})
		if err != nil {
			return item, err
		}
	}
	if item.Checkpoint == nil || item.Checkpoint.Summary != work.Summary || item.Checkpoint.NextStep != work.NextStep || len(item.Checkpoint.ArtifactRefs) != 1 || item.Checkpoint.ArtifactRefs[0] != ref {
		return item, errors.New("case_checkpoint_changed")
	}
	if item.State.State == execution.StateRunning && item.State.StateRevision == 3 {
		item, err = store.Pause(o.WorkspaceID, item.Contract.ItemID, 3, item.State.ActiveAttemptID)
		if err != nil {
			return item, err
		}
	}
	if item.State.State != execution.StatePaused || item.State.StateRevision != 4 {
		return item, errors.New("case_work_already_continued")
	}
	return item, nil
}
func (r SetupReview) Digest() string { return setupDigest(r) }
func (o *SetupOwner) PartialStatus() map[string]any {
	out, readErr := o.Status()
	if readErr != nil {
		out.State = "blocked"
		out.SetupRevision = "unavailable"
	}
	return map[string]any{"code": "case_setup_rejected", "workspace_id": o.WorkspaceID, "ledger": out.Ledger, "state": out.State, "setup_revision": out.SetupRevision, "setup": out.Setup, "registrations": out.Registrations, "retryable": false, "next_action": "Read case.setup.status and review the exact remaining effects before retrying."}
}

// The canonical intent is durable before any registration, ledger creation or
// selection effect. A crash can therefore recover by reviewing the same setup.
// Each short update owns only its file lock; no human callback executes here.
func (o *SetupOwner) stageIntent(review SetupReview, readPage func() (SetupPage, error)) (SetupReview, error) {
	replay := func(expected string) (SetupReview, error) {
		if review.Operation == "case.setup.prepare" {
			return o.ReviewPrepare(review.Proposed.Input, expected)
		}
		if readPage == nil {
			return SetupReview{}, errors.New("case_brief_required")
		}
		page, err := readPage()
		if err != nil {
			return SetupReview{}, err
		}
		return o.ReviewFinish(expected, page, review.Work)
	}
	status, err := o.save(review.Proposed, func() error {
		fresh, e := replay(review.Current.SetupRevision)
		if e != nil {
			return e
		}
		if setupDigest(fresh) != setupDigest(review) {
			return ErrSetupChanged
		}
		return nil
	}, func(state *SetupState) error {
		if review.Operation == "case.setup.finish" {
			state.BriefRevision = review.Page.Revision
			state.Work = review.Work
			if state.Input.Mode == "client" && state.CleanupActive == "" {
				state.CleanupPending = review.Current.Markers.Pending
				state.CleanupActive = review.Current.Markers.Active
				if state.CleanupActive == "" {
					state.CleanupActive = state.Input.AccountID + "/" + state.CaseID + "\n"
				}
			}
		}
		return nil
	})
	if err != nil {
		return SetupReview{}, err
	}
	fresh, err := replay(status.SetupRevision)
	if err != nil {
		return fresh, err
	}
	// Only our just-committed intent metadata can differ from the native review.
	// Personalization, signed relation, templates/catalog and complete content stay exact.
	effect := func(r SetupReview) any {
		return struct {
			Operation                                               string
			Input                                                   SetupInput
			SetupID, CaseID, CaseAgentID, AccountAgentID, BriefPage string
			Account                                                 *agentscaffold.Enrollment
			Case                                                    agentscaffold.Enrollment
			Page                                                    *SetupPage
			Work                                                    *SetupWork
		}{r.Operation, r.Proposed.Input, r.Proposed.SetupID, r.Proposed.CaseID, r.Proposed.CaseAgentID, r.Proposed.AccountAgentID, r.Proposed.BriefPage, r.Account, r.Case, r.Page, r.Work}
	}
	if setupDigest(effect(fresh)) != setupDigest(effect(review)) {
		return fresh, ErrSetupChanged
	}
	return fresh, nil
}

func (o *SetupOwner) workInput(state SetupState, work SetupWork) execution.CreateInput {
	ref := "bcgos://workspace/brain/" + state.BriefPage
	return execution.CreateInput{WorkspaceID: o.WorkspaceID, Objective: work.Objective, InitialNextStep: work.NextStep, AllowedRefs: []string{ref}, Criteria: []execution.Criterion{{ID: "artifact_1", Type: execution.CriterionArtifactSnapshot, TargetRef: ref}}}
}

func existingSetupIdentity(request agentscaffold.Request, instance agentscaffold.Instance) agentscaffold.Request {
	request.DisplayName = instance.DisplayName
	request.Emoji = instance.Emoji
	request.OwnerID = instance.OwnerID
	request.OwnershipScope = instance.OwnershipScope
	return request
}

func (o *SetupOwner) readData(path string, maximum int64) ([]byte, error) {
	r, e := o.dataDirectory.Relative(path)
	if e != nil {
		return nil, e
	}
	return o.dataDirectory.ReadFile(r, maximum, true)
}
func (o *SetupOwner) updateData(path string, maximum int64, update func([]byte) ([]byte, error)) error {
	r, e := o.dataDirectory.Relative(path)
	if e != nil {
		return e
	}
	return o.dataDirectory.UpdatePrivateFile(r, maximum, update)
}
func (o *SetupOwner) ensureData(path string) error {
	r, e := o.dataDirectory.Relative(path)
	if e != nil {
		return e
	}
	return o.dataDirectory.EnsureDirectory(r)
}
func (o *SetupOwner) publishData(path string, body []byte) error {
	r, e := o.dataDirectory.Relative(path)
	if e != nil {
		return e
	}
	return o.dataDirectory.PublishNewFile(r, body)
}
func (o *SetupOwner) ledgerStore() (execution.Store, error) {
	if o.dataDirectory == nil {
		return execution.Store{}, errors.New("case_custody_closed")
	}
	return execution.Store{Root: o.Root, DataDirectory: o.dataDirectory}, nil
}

// BrainDirectory is borrowed for the current admitted canonical page review.
func (o *SetupOwner) BrainDirectory() *scheduler.HeldDirectory { return o.brainDirectory }
