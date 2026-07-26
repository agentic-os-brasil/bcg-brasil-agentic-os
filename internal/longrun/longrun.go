// Package longrun owns Maestro's runtime-neutral long-running goal contract.
// It stores recovery metadata and opaque evidence references only: never raw
// workspace content, transcripts, prompts, client identifiers or file paths.
package longrun

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
)

var (
	idPattern       = regexp.MustCompile(`^[a-z][a-z0-9-]{2,63}$`)
	refPattern      = regexp.MustCompile(`^(goal|test|artifact|review|runtime|finding|decision|blocker)://[A-Za-z0-9][A-Za-z0-9._~:@!$&'()*+,;=%?-]{0,240}$`)
	delegationRegex = regexp.MustCompile(`^[a-z][a-z0-9-]{2,63}--[a-z][a-z0-9-]{2,63}--r[1-9][0-9]*$`)
)

type Status string

const (
	Draft          Status = "draft"
	Active         Status = "active"
	AwaitingWalter Status = "awaiting_walter"
	AwaitingHuman  Status = "awaiting_human"
	Completed      Status = "completed"
	Blocked        Status = "blocked"
)

type EvidenceClass string

const (
	EvidenceTest     EvidenceClass = "test"
	EvidenceArtifact EvidenceClass = "artifact"
	EvidenceReview   EvidenceClass = "review"
	EvidenceRuntime  EvidenceClass = "runtime"
)

type DeliverableKind string

const DeliverableCapability DeliverableKind = "capability"

type Authority string

const AuthorityHumanForExternalAction Authority = "human_for_external_action"

// DoneContract is an immutable, revisioned promise. Narrative and client
// material remain outside this core behind an opaque objective reference.
type DoneContract struct {
	Revision         int             `json:"revision"`
	ObjectiveRef     string          `json:"objective_ref"`
	Deliverables     []Deliverable   `json:"deliverables"`
	RequiredEvidence []EvidenceClass `json:"required_evidence"`
	NonGoalRefs      []string        `json:"non_goal_refs"`
	Authority        Authority       `json:"authority"`
}

type Deliverable struct {
	ID   string          `json:"id"`
	Kind DeliverableKind `json:"kind"`
}

type Evidence struct {
	ID        string        `json:"id"`
	Class     EvidenceClass `json:"class"`
	Reference string        `json:"reference"`
	Verified  bool          `json:"verified"`
}

type WorkspaceState string

const WorkspaceReady WorkspaceState = "ready"

// WorkspaceCheckpoint is already sanitized by the workspace agent.
type WorkspaceCheckpoint struct {
	GoalID       string         `json:"goal_id"`
	Phase        string         `json:"phase"`
	State        WorkspaceState `json:"state"`
	EvidenceRefs []string       `json:"evidence_refs"`
}

type SpecialistQuestion struct {
	ID         string `json:"id"`
	Capability string `json:"capability"`
	Purpose    string `json:"purpose"`
}

// SpecialistWorkPacket is the sole specialist handoff. No adapter is given a
// workspace path, body or general workspace handle.
type SpecialistWorkPacket struct {
	GoalID       string   `json:"goal_id"`
	DelegationID string   `json:"delegation_id"`
	Revision     int      `json:"revision"`
	Phase        string   `json:"phase"`
	QuestionID   string   `json:"question_id"`
	Capability   string   `json:"capability"`
	Purpose      string   `json:"purpose"`
	EvidenceRefs []string `json:"evidence_refs"`
}

// SpecialistResult stays at the workspace boundary. It is deliberately not
// accepted by Goal; the workspace adapter must emit a WorkspaceResult after
// applying its authorization and minimization rules.
type SpecialistResult struct {
	GoalID       string   `json:"goal_id"`
	DelegationID string   `json:"delegation_id"`
	FindingRefs  []string `json:"finding_refs"`
	EvidenceRefs []string `json:"evidence_refs"`
}

// WorkspaceResult is the only result Maestro's core may accept after a
// specialist cycle. It contains opaque reference IDs and completion state.
type WorkspaceResult struct {
	GoalID                string   `json:"goal_id"`
	DelegationID          string   `json:"delegation_id"`
	FindingRefs           []string `json:"finding_refs"`
	EvidenceRefs          []string `json:"evidence_refs"`
	CompletedDeliverables []string `json:"completed_deliverables"`
	BlockerRefs           []string `json:"blocker_refs"`
}

type WalterVerdict string

const (
	WalterApproved           WalterVerdict = "approved"
	WalterRefine             WalterVerdict = "refine"
	WalterNeedsHumanDecision WalterVerdict = "needs_human_decision"
)

type ReviewReason string

const (
	ReviewEvidenceGap       ReviewReason = "evidence_gap"
	ReviewAuthorityBoundary ReviewReason = "authority_boundary"
)

// WalterReview must match the exact immutable ledger state Walter reviewed.
type WalterReview struct {
	GoalID           string        `json:"goal_id"`
	ContractRevision int           `json:"contract_revision"`
	LedgerRevision   int           `json:"ledger_revision"`
	Verdict          WalterVerdict `json:"verdict"`
	Reason           ReviewReason  `json:"reason,omitempty"`
}

type Action string

const (
	ActionReturnToWorkspace  Action = "return_to_workspace"
	ActionComposeAdvancement Action = "compose_advancement"
	ActionRequestWalter      Action = "request_walter_review"
	ActionRequestHuman       Action = "request_human_decision"
	ActionCompletionAudit    Action = "completion_audit"
)

// Breadcrumb is a compact, opaque recovery point. Its strings are IDs, not
// free-form transcript fields.
type Breadcrumb struct {
	Phase       string `json:"phase"`
	DecisionRef string `json:"decision_ref,omitempty"`
	EvidenceRef string `json:"evidence_ref,omitempty"`
	NextAction  Action `json:"next_action"`
	Action      Action `json:"action"`
}

// CompletionAudit is Maestro's explicit claim that the current phase is
// fulfilled. It cannot be reused after the ledger changes.
type CompletionAudit struct {
	GoalID                string   `json:"goal_id"`
	LedgerRevision        int      `json:"ledger_revision"`
	Phase                 string   `json:"phase"`
	PhaseComplete         bool     `json:"phase_complete"`
	CompletedDeliverables []string `json:"completed_deliverables"`
	NoBlockers            bool     `json:"no_blockers"`
}

// EventKind is a durable transition performed by Maestro's state machine.
// Events intentionally carry only the same typed, opaque values accepted by
// the core; Store signs and replays them instead of trusting a saved snapshot.
type EventKind string

const (
	EventActivated           EventKind = "activated"
	EventEvidence            EventKind = "evidence"
	EventWorkspaceCheckpoint EventKind = "workspace_checkpoint"
	EventDelegated           EventKind = "delegated"
	EventWorkspaceResult     EventKind = "workspace_result"
	EventWalterRequested     EventKind = "walter_requested"
	EventWalterReviewed      EventKind = "walter_reviewed"
	EventHumanResumed        EventKind = "human_resumed"
	EventCompletionAudited   EventKind = "completion_audited"
	EventCompleted           EventKind = "completed"
)

type GoalEvent struct {
	Sequence        int                  `json:"sequence"`
	Kind            EventKind            `json:"kind"`
	Phase           string               `json:"phase,omitempty"`
	Evidence        *Evidence            `json:"evidence,omitempty"`
	Checkpoint      *WorkspaceCheckpoint `json:"checkpoint,omitempty"`
	Question        *SpecialistQuestion  `json:"question,omitempty"`
	WorkspaceResult *WorkspaceResult     `json:"workspace_result,omitempty"`
	WalterReview    *WalterReview        `json:"walter_review,omitempty"`
	CompletionAudit *CompletionAudit     `json:"completion_audit,omitempty"`
	MAC             string               `json:"mac,omitempty"`
}

// Goal is Maestro-owned private state. Adapters receive immutable views and
// can influence it only by returning typed, validated records to the engine.
type Goal struct {
	id          string
	contract    DoneContract
	status      Status
	phase       string
	evidence    []Evidence
	breadcrumbs []Breadcrumb

	needsFreshWalterReview bool
	workspaceReady         bool
	delegations            map[string]SpecialistWorkPacket
	specialistReturned     bool
	completedDeliverables  map[string]bool
	blockerRefs            map[string]bool
	ledgerRevision         int
	walterApprovedLedger   int
	completionAudit        *CompletionAudit
	events                 []GoalEvent
}

func NewGoal(id string, contract DoneContract) (*Goal, error) {
	if !idPattern.MatchString(id) || !validContract(contract) {
		return nil, errors.New("invalid long-running goal or Done Contract")
	}
	return &Goal{id: id, contract: cloneContract(contract), status: Draft, delegations: map[string]SpecialistWorkPacket{}, completedDeliverables: map[string]bool{}, blockerRefs: map[string]bool{}}, nil
}

// NewActiveGoal is the public Maestro entrypoint. Goal transition mutators are
// intentionally package-private so adapters can only influence state through
// LoopEngine's role-specific interfaces.
func NewActiveGoal(id string, contract DoneContract, phase string) (*Goal, error) {
	goal, err := NewGoal(id, contract)
	if err != nil {
		return nil, err
	}
	if err := goal.activate(phase); err != nil {
		return nil, err
	}
	return goal, nil
}

// Immutable accessors keep adapter-facing state from becoming an authority.
func (goal *Goal) ID() string {
	if goal == nil {
		return ""
	}
	return goal.id
}
func (goal *Goal) Contract() DoneContract {
	if goal == nil {
		return DoneContract{}
	}
	return cloneContract(goal.contract)
}
func (goal *Goal) Status() Status {
	if goal == nil {
		return ""
	}
	return goal.status
}
func (goal *Goal) Phase() string {
	if goal == nil {
		return ""
	}
	return goal.phase
}
func (goal *Goal) LedgerRevision() int {
	if goal == nil {
		return 0
	}
	return goal.ledgerRevision
}
func (goal *Goal) NeedsFreshWalterReview() bool { return goal == nil || goal.needsFreshWalterReview }
func (goal *Goal) Evidence() []Evidence {
	if goal == nil {
		return nil
	}
	return append([]Evidence(nil), goal.evidence...)
}
func (goal *Goal) Breadcrumbs() []Breadcrumb {
	if goal == nil {
		return nil
	}
	return append([]Breadcrumb(nil), goal.breadcrumbs...)
}
func (goal *Goal) Events() []GoalEvent {
	if goal == nil {
		return nil
	}
	return cloneEvents(goal.events)
}

func (goal *Goal) activate(phase string) error {
	if goal == nil || goal.status != Draft || !idPattern.MatchString(phase) {
		return errors.New("goal cannot be activated")
	}
	goal.status, goal.phase = Active, phase
	goal.materialChange()
	goal.appendEvent(GoalEvent{Kind: EventActivated, Phase: phase})
	return nil
}

func (goal *Goal) recordEvidence(e Evidence) error {
	if goal == nil || goal.status != Active || !validEvidence(e) || goal.hasEvidence(e.ID) {
		return errors.New("invalid long-running evidence")
	}
	goal.evidence = append(goal.evidence, e)
	goal.materialChange()
	goal.appendEvent(GoalEvent{Kind: EventEvidence, Evidence: cloneEvidence(&e)})
	return nil
}

func (goal *Goal) recordWorkspaceCheckpoint(checkpoint WorkspaceCheckpoint) error {
	if goal == nil || goal.status != Active || checkpoint.GoalID != goal.id || checkpoint.Phase != goal.phase || checkpoint.State != WorkspaceReady || len(checkpoint.EvidenceRefs) == 0 || !goal.referencesVerified(checkpoint.EvidenceRefs) {
		return errors.New("invalid workspace checkpoint")
	}
	goal.workspaceReady = true
	goal.materialChange()
	goal.appendEvent(GoalEvent{Kind: EventWorkspaceCheckpoint, Checkpoint: cloneCheckpoint(&checkpoint)})
	return nil
}

func (goal *Goal) delegate(question SpecialistQuestion) (SpecialistWorkPacket, error) {
	if goal == nil || goal.status != Active || !goal.workspaceReady || !validQuestion(question) || goal.delegations[question.ID].QuestionID != "" {
		return SpecialistWorkPacket{}, errors.New("specialist delegation is not available")
	}
	packet := SpecialistWorkPacket{GoalID: goal.id, DelegationID: goal.id + "--" + question.ID + "--r" + fmt.Sprint(goal.ledgerRevision), Revision: goal.contract.Revision, Phase: goal.phase, QuestionID: question.ID, Capability: question.Capability, Purpose: question.Purpose, EvidenceRefs: goal.verifiedEvidenceRefs()}
	goal.delegations[question.ID] = packet
	goal.appendEvent(GoalEvent{Kind: EventDelegated, Question: cloneQuestion(&question)})
	return packet, nil
}

// RecordWorkspaceResult accepts only the workspace agent's minimized result;
// raw SpecialistResult is intentionally not a Goal API.
func (goal *Goal) recordWorkspaceResult(result WorkspaceResult) error {
	if goal == nil || goal.status != Active || result.GoalID != goal.id || !delegationRegex.MatchString(result.DelegationID) || len(result.FindingRefs) == 0 || !validRefs(result.FindingRefs) || !goal.referencesVerified(result.EvidenceRefs) || !validDeliverableIDs(result.CompletedDeliverables) || !validRefs(result.BlockerRefs) {
		return errors.New("invalid workspace result")
	}
	var questionID string
	for id, packet := range goal.delegations {
		if packet.DelegationID == result.DelegationID {
			questionID = id
			break
		}
	}
	if questionID == "" {
		return errors.New("workspace result has no authorized delegation")
	}
	delete(goal.delegations, questionID)
	goal.specialistReturned = true
	for _, id := range result.CompletedDeliverables {
		goal.completedDeliverables[id] = true
	}
	for _, ref := range result.BlockerRefs {
		goal.blockerRefs[ref] = true
	}
	goal.breadcrumbs = append(goal.breadcrumbs, Breadcrumb{Phase: goal.phase, NextAction: ActionComposeAdvancement, Action: ActionComposeAdvancement})
	goal.materialChange()
	goal.appendEvent(GoalEvent{Kind: EventWorkspaceResult, WorkspaceResult: cloneWorkspaceResult(&result)})
	return nil
}

func (goal *Goal) requestWalterReview() error {
	if goal == nil || goal.status != Active || !goal.workspaceReady || !goal.specialistReturned || len(goal.delegations) != 0 {
		return errors.New("Walter review is not available")
	}
	goal.status = AwaitingWalter
	goal.breadcrumbs = append(goal.breadcrumbs, Breadcrumb{Phase: goal.phase, NextAction: ActionRequestWalter, Action: ActionRequestWalter})
	goal.appendEvent(GoalEvent{Kind: EventWalterRequested})
	return nil
}

func (goal *Goal) applyWalterReview(review WalterReview) error {
	if goal == nil || goal.status != AwaitingWalter || review.GoalID != goal.id || review.ContractRevision != goal.contract.Revision || review.LedgerRevision != goal.ledgerRevision || !validReview(review) {
		return errors.New("invalid Walter review")
	}
	switch review.Verdict {
	case WalterApproved:
		goal.status, goal.walterApprovedLedger, goal.needsFreshWalterReview = Active, goal.ledgerRevision, false
	case WalterRefine:
		goal.status, goal.workspaceReady, goal.specialistReturned = Active, false, false
		goal.needsFreshWalterReview, goal.completionAudit = true, nil
		goal.breadcrumbs = append(goal.breadcrumbs, Breadcrumb{Phase: goal.phase, NextAction: ActionReturnToWorkspace, Action: ActionReturnToWorkspace})
	case WalterNeedsHumanDecision:
		goal.status, goal.needsFreshWalterReview, goal.completionAudit = AwaitingHuman, true, nil
		goal.breadcrumbs = append(goal.breadcrumbs, Breadcrumb{Phase: goal.phase, NextAction: ActionRequestHuman, Action: ActionRequestHuman})
	}
	goal.appendEvent(GoalEvent{Kind: EventWalterReviewed, WalterReview: cloneWalterReview(&review)})
	return nil
}

func (goal *Goal) resumeAfterHumanDecision() error {
	if goal == nil || goal.status != AwaitingHuman {
		return errors.New("goal is not awaiting a human decision")
	}
	goal.status, goal.needsFreshWalterReview = Active, true
	goal.appendEvent(GoalEvent{Kind: EventHumanResumed})
	return nil
}

func (goal *Goal) recordCompletionAudit(audit CompletionAudit) error {
	if goal == nil || goal.status != Active || audit.GoalID != goal.id || audit.LedgerRevision != goal.ledgerRevision || audit.Phase != goal.phase || !audit.PhaseComplete || !audit.NoBlockers || !sameIDs(audit.CompletedDeliverables, goal.contract.Deliverables) || !goal.allDeliverablesComplete() || len(goal.blockerRefs) != 0 || !goal.hasRequiredEvidence() {
		return errors.New("invalid Maestro completion audit")
	}
	clone := audit
	clone.CompletedDeliverables = append([]string(nil), audit.CompletedDeliverables...)
	goal.completionAudit = &clone
	goal.breadcrumbs = append(goal.breadcrumbs, Breadcrumb{Phase: goal.phase, NextAction: ActionCompletionAudit, Action: ActionCompletionAudit})
	goal.appendEvent(GoalEvent{Kind: EventCompletionAudited, CompletionAudit: cloneAudit(&audit)})
	return nil
}

func (goal *Goal) complete() error {
	if goal == nil || goal.status != Active || goal.needsFreshWalterReview || goal.walterApprovedLedger != goal.ledgerRevision || goal.completionAudit == nil || goal.completionAudit.LedgerRevision != goal.ledgerRevision || !goal.hasRequiredEvidence() || !goal.allDeliverablesComplete() || len(goal.blockerRefs) != 0 {
		return errors.New("Done Contract is not satisfied")
	}
	goal.status = Completed
	goal.appendEvent(GoalEvent{Kind: EventCompleted})
	return nil
}

type WalterRecord struct {
	GoalID           string               `json:"goal_id"`
	ContractRevision int                  `json:"contract_revision"`
	LedgerRevision   int                  `json:"ledger_revision"`
	Phase            string               `json:"phase"`
	Contract         WalterContractDigest `json:"contract"`
	Evidence         []WalterEvidence     `json:"evidence"`
	Deliverables     []WalterDeliverable  `json:"deliverables"`
	BlockerCount     int                  `json:"blocker_count"`
	CompletionReady  bool                 `json:"completion_ready"`
	Trail            []WalterBreadcrumb   `json:"trail"`
}

type WalterContractDigest struct {
	ObjectiveRef     string          `json:"objective_ref"`
	RequiredEvidence []EvidenceClass `json:"required_evidence"`
	Authority        Authority       `json:"authority"`
}
type WalterEvidence struct {
	Class    EvidenceClass `json:"class"`
	Verified bool          `json:"verified"`
}
type WalterDeliverable struct {
	ID       string `json:"id"`
	Complete bool   `json:"complete"`
}
type WalterBreadcrumb struct {
	Phase  string `json:"phase"`
	Action Action `json:"action"`
}

func (goal *Goal) walterRecord() (WalterRecord, error) {
	if goal == nil || goal.status != AwaitingWalter {
		return WalterRecord{}, errors.New("Walter record is not available")
	}
	record := WalterRecord{GoalID: goal.id, ContractRevision: goal.contract.Revision, LedgerRevision: goal.ledgerRevision, Phase: goal.phase, Contract: WalterContractDigest{ObjectiveRef: goal.contract.ObjectiveRef, RequiredEvidence: append([]EvidenceClass(nil), goal.contract.RequiredEvidence...), Authority: goal.contract.Authority}, BlockerCount: len(goal.blockerRefs), CompletionReady: goal.hasRequiredEvidence() && goal.allDeliverablesComplete() && len(goal.blockerRefs) == 0}
	for _, e := range goal.evidence {
		record.Evidence = append(record.Evidence, WalterEvidence{Class: e.Class, Verified: e.Verified})
	}
	for _, d := range goal.contract.Deliverables {
		record.Deliverables = append(record.Deliverables, WalterDeliverable{ID: d.ID, Complete: goal.completedDeliverables[d.ID]})
	}
	for _, b := range goal.breadcrumbs {
		record.Trail = append(record.Trail, WalterBreadcrumb{Phase: b.Phase, Action: b.Action})
	}
	return record, nil
}
func (record WalterRecord) String() string {
	encoded, _ := json.Marshal(record)
	return string(encoded)
}

func (goal *Goal) materialChange() {
	goal.ledgerRevision++
	goal.walterApprovedLedger = 0
	goal.needsFreshWalterReview = true
	goal.completionAudit = nil
}
func (goal *Goal) appendEvent(event GoalEvent) {
	event.Sequence = len(goal.events) + 1
	event.MAC = ""
	goal.events = append(goal.events, event)
}
func (goal *Goal) hasEvidence(id string) bool {
	for _, e := range goal.evidence {
		if e.ID == id {
			return true
		}
	}
	return false
}
func (goal *Goal) referencesVerified(refs []string) bool {
	if len(refs) == 0 || !validRefs(refs) {
		return false
	}
	verified := map[string]bool{}
	for _, e := range goal.evidence {
		if e.Verified {
			verified[e.Reference] = true
		}
	}
	for _, ref := range refs {
		if !verified[ref] {
			return false
		}
	}
	return true
}
func (goal *Goal) verifiedEvidenceRefs() []string {
	refs := make([]string, 0, len(goal.evidence))
	for _, e := range goal.evidence {
		if e.Verified {
			refs = append(refs, e.Reference)
		}
	}
	sort.Strings(refs)
	return refs
}
func (goal *Goal) hasRequiredEvidence() bool {
	found := map[EvidenceClass]bool{}
	for _, e := range goal.evidence {
		if e.Verified {
			found[e.Class] = true
		}
	}
	for _, required := range goal.contract.RequiredEvidence {
		if !found[required] {
			return false
		}
	}
	return true
}
func (goal *Goal) allDeliverablesComplete() bool {
	for _, d := range goal.contract.Deliverables {
		if !goal.completedDeliverables[d.ID] {
			return false
		}
	}
	return true
}

func validContract(c DoneContract) bool {
	if c.Revision <= 0 || !validRef(c.ObjectiveRef) || len(c.Deliverables) == 0 || len(c.RequiredEvidence) == 0 || c.Authority != AuthorityHumanForExternalAction || !validRefs(c.NonGoalRefs) {
		return false
	}
	seenD := map[string]bool{}
	for _, d := range c.Deliverables {
		if !idPattern.MatchString(d.ID) || d.Kind != DeliverableCapability || seenD[d.ID] {
			return false
		}
		seenD[d.ID] = true
	}
	seenE := map[EvidenceClass]bool{}
	for _, e := range c.RequiredEvidence {
		if !validEvidenceClass(e) || seenE[e] {
			return false
		}
		seenE[e] = true
	}
	return true
}
func cloneContract(c DoneContract) DoneContract {
	c.Deliverables = append([]Deliverable(nil), c.Deliverables...)
	c.RequiredEvidence = append([]EvidenceClass(nil), c.RequiredEvidence...)
	c.NonGoalRefs = append([]string(nil), c.NonGoalRefs...)
	return c
}
func validEvidence(e Evidence) bool {
	return idPattern.MatchString(e.ID) && validEvidenceClass(e.Class) && validRef(e.Reference)
}
func validEvidenceClass(c EvidenceClass) bool {
	return c == EvidenceTest || c == EvidenceArtifact || c == EvidenceReview || c == EvidenceRuntime
}
func validQuestion(q SpecialistQuestion) bool {
	return idPattern.MatchString(q.ID) && idPattern.MatchString(q.Capability) && idPattern.MatchString(q.Purpose)
}
func validRef(ref string) bool { return refPattern.MatchString(ref) }
func validRefs(refs []string) bool {
	for _, ref := range refs {
		if !validRef(ref) {
			return false
		}
	}
	return true
}
func validDeliverableIDs(ids []string) bool {
	seen := map[string]bool{}
	for _, id := range ids {
		if !idPattern.MatchString(id) || seen[id] {
			return false
		}
		seen[id] = true
	}
	return true
}
func sameIDs(ids []string, deliverables []Deliverable) bool {
	if len(ids) != len(deliverables) || !validDeliverableIDs(ids) {
		return false
	}
	want := map[string]bool{}
	for _, d := range deliverables {
		want[d.ID] = true
	}
	for _, id := range ids {
		if !want[id] {
			return false
		}
	}
	return true
}
func validReview(r WalterReview) bool {
	if r.Verdict == WalterApproved {
		return r.Reason == ""
	}
	if r.Verdict == WalterRefine {
		return r.Reason == ReviewEvidenceGap || r.Reason == ReviewAuthorityBoundary
	}
	return r.Verdict == WalterNeedsHumanDecision && r.Reason == ReviewAuthorityBoundary
}

func cloneEvidence(value *Evidence) *Evidence {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}
func cloneCheckpoint(value *WorkspaceCheckpoint) *WorkspaceCheckpoint {
	if value == nil {
		return nil
	}
	clone := *value
	clone.EvidenceRefs = append([]string(nil), value.EvidenceRefs...)
	return &clone
}
func cloneQuestion(value *SpecialistQuestion) *SpecialistQuestion {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}
func cloneWorkspaceResult(value *WorkspaceResult) *WorkspaceResult {
	if value == nil {
		return nil
	}
	clone := *value
	clone.FindingRefs = append([]string(nil), value.FindingRefs...)
	clone.EvidenceRefs = append([]string(nil), value.EvidenceRefs...)
	clone.CompletedDeliverables = append([]string(nil), value.CompletedDeliverables...)
	clone.BlockerRefs = append([]string(nil), value.BlockerRefs...)
	return &clone
}
func cloneWalterReview(value *WalterReview) *WalterReview {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}
func cloneAudit(value *CompletionAudit) *CompletionAudit {
	if value == nil {
		return nil
	}
	clone := *value
	clone.CompletedDeliverables = append([]string(nil), value.CompletedDeliverables...)
	return &clone
}
func cloneEvents(events []GoalEvent) []GoalEvent {
	cloned := make([]GoalEvent, 0, len(events))
	for _, event := range events {
		clone := event
		clone.Evidence = cloneEvidence(event.Evidence)
		clone.Checkpoint = cloneCheckpoint(event.Checkpoint)
		clone.Question = cloneQuestion(event.Question)
		clone.WorkspaceResult = cloneWorkspaceResult(event.WorkspaceResult)
		clone.WalterReview = cloneWalterReview(event.WalterReview)
		clone.CompletionAudit = cloneAudit(event.CompletionAudit)
		cloned = append(cloned, clone)
	}
	return cloned
}

func replay(goal *Goal, event GoalEvent) error {
	if event.Sequence != len(goal.events)+1 || event.MAC != "" {
		return errors.New("invalid long-running event sequence")
	}
	switch event.Kind {
	case EventActivated:
		return goal.activate(event.Phase)
	case EventEvidence:
		if event.Evidence == nil {
			return errors.New("missing evidence event value")
		}
		return goal.recordEvidence(*event.Evidence)
	case EventWorkspaceCheckpoint:
		if event.Checkpoint == nil {
			return errors.New("missing checkpoint event value")
		}
		return goal.recordWorkspaceCheckpoint(*event.Checkpoint)
	case EventDelegated:
		if event.Question == nil {
			return errors.New("missing delegation event value")
		}
		_, err := goal.delegate(*event.Question)
		return err
	case EventWorkspaceResult:
		if event.WorkspaceResult == nil {
			return errors.New("missing workspace result event value")
		}
		return goal.recordWorkspaceResult(*event.WorkspaceResult)
	case EventWalterRequested:
		return goal.requestWalterReview()
	case EventWalterReviewed:
		if event.WalterReview == nil {
			return errors.New("missing Walter review event value")
		}
		return goal.applyWalterReview(*event.WalterReview)
	case EventHumanResumed:
		return goal.resumeAfterHumanDecision()
	case EventCompletionAudited:
		if event.CompletionAudit == nil {
			return errors.New("missing completion audit event value")
		}
		return goal.recordCompletionAudit(*event.CompletionAudit)
	case EventCompleted:
		return goal.complete()
	default:
		return errors.New("unknown long-running event")
	}
}
func (status Status) String() string { return string(status) }
func (goal Goal) String() string     { return fmt.Sprintf("%s:%s", goal.id, goal.status) }
