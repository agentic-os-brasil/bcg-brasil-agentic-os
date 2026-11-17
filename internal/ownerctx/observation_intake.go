package ownerctx

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const maximumSelfIntakes = 1024

// SelfObservationDeclaration is an exact owner declaration, not inferred history.
type SelfObservationDeclaration struct {
	Signal     SignalClass `json:"signal"`
	Facet      string      `json:"facet"`
	Claim      string      `json:"claim"`
	Confidence float64     `json:"confidence"`
	ExpiresAt  time.Time   `json:"expires_at"`
}

func NewSelfObservationDeclaration(root string, value SelfObservationDeclaration) (SelfObservationSubmission, error) {
	scope, err := SelfObservationOwnerScope(root)
	if err != nil {
		return SelfObservationSubmission{}, err
	}
	value.ExpiresAt = value.ExpiresAt.UTC()
	input := SelfObservationSubmission{Signal: value.Signal, Facet: value.Facet, Claim: value.Claim, EvidenceType: "owner_declaration", SourceDigest: selfObservationDigest([]any{scope, value}), EpisodeID: "owner-reviewed-declaration", Confidence: value.Confidence, ExpiresAt: value.ExpiresAt}
	if _, err := selfObservationInput(input); err != nil {
		return SelfObservationSubmission{}, err
	}
	return input, nil
}
func declarationFromSubmission(value SelfObservationSubmission) SelfObservationDeclaration {
	return SelfObservationDeclaration{value.Signal, value.Facet, value.Claim, value.Confidence, value.ExpiresAt}
}

// SelfObservationSubmission carries owner-reviewable codes, never authority or bodies.
type SelfObservationSubmission struct {
	Signal       SignalClass `json:"signal"`
	Facet        string      `json:"facet"`
	Claim        string      `json:"claim"`
	EvidenceType string      `json:"evidence_type"`
	SourceDigest string      `json:"source_digest"`
	EpisodeID    string      `json:"episode_id"`
	Confidence   float64     `json:"confidence"`
	ExpiresAt    time.Time   `json:"expires_at"`
}

type SelfObservationReview struct {
	OwnerBinding string                    `json:"owner_binding"`
	Submission   SelfObservationSubmission `json:"submission"`
	ID           string                    `json:"id"`
	Scope        string                    `json:"scope"`
	SourceState  string                    `json:"source_state"`
	CurrentState string                    `json:"current_state"`
	ReviewDigest string                    `json:"review_digest"`
	seal         string
}

type SelfObservationIntakeReceipt struct {
	ID            string `json:"id"`
	ObservationID string `json:"observation_id"`
	State         string `json:"state"`
	Revision      uint64 `json:"revision"`
}

type selfObservationIntake struct {
	OwnerScope    string           `json:"owner_scope"`
	SchemaVersion int              `json:"schema_version"`
	ID            string           `json:"id"`
	Input         ObservationInput `json:"input"`
	ReviewDigest  string           `json:"review_digest"`
	ApprovedAt    time.Time        `json:"approved_at"`
	State         string           `json:"state"`
	Revision      uint64           `json:"revision"`
}

type SelfObservationCandidate struct {
	ID            string `json:"id"`
	ObservationID string `json:"observation_id"`
}
type SelfObservationSource struct {
	OwnerScope  string                         `json:"owner_scope"`
	Fingerprint string                         `json:"fingerprint"`
	Pending     []SelfObservationCandidate     `json:"pending"`
	Intakes     []SelfObservationIntakeReceipt `json:"intakes"`
}
type SelfObservationCapture struct {
	Added       bool
	Observation ObservationReceipt
}

func selfObservationDigest(value any) string {
	body, _ := json.Marshal(value)
	return digest(string(body))
}
func selfObservationIntakeID(input ObservationInput) string {
	return "self-intake-" + selfObservationDigest([]string{input.SourceEvent, input.SourceDigest, input.EpisodeID, input.Facet})[:40]
}
func SelfObservationOwnerScope(root string) (string, error) {
	if !filepath.IsAbs(root) || filepath.Clean(root) != root || strings.ContainsAny(root, "\x00\r\n") {
		return "", errors.New("self observation owner root is invalid")
	}
	return "owner-" + digest(root)[:32], nil
}
func selfObservationOwnerScope(root string) string {
	scope, _ := SelfObservationOwnerScope(root)
	return scope
}
func selfObservationIntakePath(root string) string {
	return filepath.Join(observationDirectory(root), "approved-intake.json")
}

func selfObservationInput(submission SelfObservationSubmission) (ObservationInput, error) {
	if submission.Signal != SignalExplicitInstruction && submission.Signal != SignalExplicitCorrection && submission.Signal != SignalExplicitEndorsement {
		return ObservationInput{}, errors.New("self declaration requires an explicit owner signal")
	}
	if !validObservationFacet(submission.Facet) || submission.EvidenceType != "owner_declaration" || submission.EpisodeID != "owner-reviewed-declaration" {
		return ObservationInput{}, errors.New("self declaration metadata is invalid")
	}
	event := "owner.reviewed_declaration"
	input := ObservationInput{SchemaVersion: 1, Signal: submission.Signal, Facet: submission.Facet, Claim: submission.Claim, EvidenceType: submission.EvidenceType, SourceEvent: event, SourceDigest: submission.SourceDigest, EpisodeID: submission.EpisodeID, ScopeKind: "global", ScopeID: "owner", Confidence: submission.Confidence, Sensitivity: "professional", ExpiresAt: submission.ExpiresAt, AuthenticatedOwner: true, Material: true, OwnerConfirmed: true, DeclassifiedGlobal: true}
	evaluation, err := EvaluateInteraction(input)
	if err != nil {
		return ObservationInput{}, err
	}
	if !evaluation.Persist {
		return ObservationInput{}, errors.New("self intake is not eligible owner evidence")
	}
	return input, nil
}

func readSelfObservationIntakes(root string) ([]selfObservationIntake, error) {
	body, err := readOwnerFile(selfObservationIntakePath(root), maximumObservationBytes)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var entries []selfObservationIntake
	d := json.NewDecoder(bytes.NewReader(body))
	d.DisallowUnknownFields()
	if err := d.Decode(&entries); err != nil {
		return nil, err
	}
	if d.Decode(new(any)) != io.EOF {
		return nil, errors.New("self intake trailing data")
	}
	if len(entries) > maximumSelfIntakes {
		return nil, errors.New("self intake capacity exceeded")
	}
	previous := ""
	for _, entry := range entries {
		if entry.OwnerScope != selfObservationOwnerScope(root) || entry.SchemaVersion != 1 || entry.ID != selfObservationIntakeID(entry.Input) || entry.ID <= previous || !validDigest(entry.ReviewDigest) || entry.ApprovedAt.IsZero() || entry.Revision == 0 || (entry.State != "approved" && entry.State != "rejected" && entry.State != "redacted") {
			return nil, errors.New("self intake identity is invalid")
		}
		if entry.Input.ScopeKind != "global" || entry.Input.ScopeID != "owner" || entry.Input.Sensitivity != "professional" || !entry.Input.AuthenticatedOwner || !entry.Input.OwnerConfirmed || !entry.Input.DeclassifiedGlobal || !entry.Input.Material {
			return nil, errors.New("self intake owner scope is invalid")
		}
		if evaluation, err := EvaluateInteraction(entry.Input); err != nil || !evaluation.Persist {
			return nil, errors.New("self intake is ineligible")
		}
		rebuilt, err := selfObservationInput(SelfObservationSubmission{entry.Input.Signal, entry.Input.Facet, entry.Input.Claim, entry.Input.EvidenceType, entry.Input.SourceDigest, entry.Input.EpisodeID, entry.Input.Confidence, entry.Input.ExpiresAt})
		if err != nil || !sameObservationInput(rebuilt, entry.Input) {
			return nil, errors.New("self intake envelope changed")
		}
		canonical, err := NewSelfObservationDeclaration(root, SelfObservationDeclaration{entry.Input.Signal, entry.Input.Facet, entry.Input.Claim, entry.Input.Confidence, entry.Input.ExpiresAt})
		if err != nil || canonical.SourceDigest != entry.Input.SourceDigest {
			return nil, errors.New("self declaration identity changed")
		}

		if !entry.Input.ExpiresAt.After(entry.ApprovedAt) || entry.Input.ExpiresAt.Sub(entry.ApprovedAt) > 90*24*time.Hour {
			return nil, errors.New("self intake expiry is invalid")
		}
		previous = entry.ID
	}
	return entries, nil
}
func writeSelfObservationIntakes(root string, entries []selfObservationIntake) error {
	if len(entries) > maximumSelfIntakes {
		return errors.New("self intake capacity exceeded")
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].ID < entries[j].ID })
	body, err := json.Marshal(entries)
	if err != nil {
		return err
	}
	if len(body) > maximumObservationBytes {
		return errors.New("self intake byte limit exceeded")
	}
	return publishOwnerFile(selfObservationIntakePath(root), body, false)
}
func selfObservationSource(root string, entries []selfObservationIntake, records []observationRecord, now time.Time) SelfObservationSource {
	type denial struct {
		ID       string
		Revision string
	}
	var denials []denial
	latest := latestObservations(records)
	for _, record := range latest {
		if observationTerminal(record.State) {
			denials = append(denials, denial{record.ID, observationRevision(record)})
		}
	}
	sort.Slice(denials, func(i, j int) bool { return denials[i].ID < denials[j].ID })
	result := SelfObservationSource{OwnerScope: selfObservationOwnerScope(root), Fingerprint: selfObservationDigest([]any{selfObservationOwnerScope(root), entries, denials})}
	for _, entry := range entries {
		id := observationID(entry.Input)
		result.Intakes = append(result.Intakes, SelfObservationIntakeReceipt{ID: entry.ID, ObservationID: id, State: entry.State, Revision: entry.Revision})
		if _, exists := latest[id]; entry.State == "approved" && entry.Input.ExpiresAt.After(now) && !exists {
			result.Pending = append(result.Pending, SelfObservationCandidate{entry.ID, id})
		}
	}
	return result
}
func observationTerminal(state ObservationState) bool {
	return state == ObservationRejected || state == ObservationContradicted || state == ObservationExpired || state == ObservationRedacted
}
func InspectSelfObservationSource(root string, now time.Time) (SelfObservationSource, error) {
	if _, err := SelfObservationOwnerScope(root); err != nil {
		return SelfObservationSource{}, err
	}
	var result SelfObservationSource
	err := withObservationReadSnapshot(root, func(records []observationRecord) error {
		entries, err := readSelfObservationIntakes(root)
		if err != nil {
			return err
		}
		result = selfObservationSource(root, entries, records, now)
		return nil
	})
	return result, err
}
func reviewSelfObservation(root string, submission SelfObservationSubmission, entries []selfObservationIntake, records []observationRecord) (SelfObservationReview, error) {
	input, err := selfObservationInput(submission)
	if err != nil {
		return SelfObservationReview{}, err
	}
	canonical, err := NewSelfObservationDeclaration(root, declarationFromSubmission(submission))
	if err != nil || selfObservationDigest(canonical) != selfObservationDigest(submission) {
		return SelfObservationReview{}, ErrRevisionConflict
	}

	now := time.Now().UTC()
	if !input.ExpiresAt.After(now) || input.ExpiresAt.Sub(now) > 90*24*time.Hour {
		return SelfObservationReview{}, errors.New("self intake expiry must be within ninety days")
	}
	if _, err := readOwnerFile(filepath.Join(root, "owner", "registry.json"), 1<<20); err != nil {
		return SelfObservationReview{}, err
	}
	id := selfObservationIntakeID(input)
	result := SelfObservationReview{OwnerBinding: selfObservationOwnerScope(root), Submission: submission, ID: id, Scope: "owner_global_professional_proposal_input", SourceState: selfObservationSource(root, entries, records, now).Fingerprint, CurrentState: "absent"}
	for _, entry := range entries {
		if entry.ID == id {
			if !sameObservationInput(entry.Input, input) {
				return SelfObservationReview{}, ErrRevisionConflict
			}
			result.CurrentState = entry.State
		}
	}
	if current, exists := latestObservations(records)[observationID(input)]; exists && observationTerminal(current.State) {
		result.CurrentState = string(current.State)
	}
	result.ReviewDigest = selfObservationDigest(result)
	result.seal = result.ReviewDigest
	return result, nil
}
func ReviewSelfObservation(root string, submission SelfObservationSubmission) (SelfObservationReview, error) {
	var review SelfObservationReview
	err := withObservationReadSnapshot(root, func(records []observationRecord) error {
		entries, err := readSelfObservationIntakes(root)
		if err != nil {
			return err
		}
		review, err = reviewSelfObservation(root, submission, entries, records)
		return err
	})
	return review, err
}

// ApproveSelfObservation keeps cancellation effective through owner-lock acquisition.
func ApproveSelfObservation(ctx context.Context, root string, review SelfObservationReview, confirmed bool) (SelfObservationIntakeReceipt, error) {
	if ctx == nil {
		return SelfObservationIntakeReceipt{}, errors.New("self observation context is required")
	}
	if err := ctx.Err(); err != nil {
		return SelfObservationIntakeReceipt{}, err
	}
	if !confirmed {
		return SelfObservationIntakeReceipt{}, ErrConfirmationRequired
	}
	if review.seal == "" || review.seal != review.ReviewDigest || review.OwnerBinding != selfObservationOwnerScope(root) {
		return SelfObservationIntakeReceipt{}, ErrRevisionConflict
	}
	preflight, err := ReviewSelfObservation(root, review.Submission)
	if err != nil {
		return SelfObservationIntakeReceipt{}, err
	}
	if preflight.ReviewDigest != review.ReviewDigest {
		return SelfObservationIntakeReceipt{}, ErrRevisionConflict
	}
	var receipt SelfObservationIntakeReceipt
	err = withObservationLock(root, func(_ string) error {
		records, err := readObservationRecords(root)
		if err != nil {
			return err
		}
		entries, err := readSelfObservationIntakes(root)
		if err != nil {
			return err
		}
		fresh, err := reviewSelfObservation(root, review.Submission, entries, records)
		if err != nil {
			return err
		}
		if fresh.ReviewDigest != review.ReviewDigest {
			return ErrRevisionConflict
		}
		if fresh.CurrentState == "rejected" || fresh.CurrentState == "redacted" || fresh.CurrentState == "contradicted" || fresh.CurrentState == "expired" {
			return errors.New("self declaration is denied")
		}
		input, err := selfObservationInput(review.Submission)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if entry.ID == review.ID {
				receipt = SelfObservationIntakeReceipt{entry.ID, observationID(entry.Input), entry.State, entry.Revision}
				return nil
			}
		}
		entry := selfObservationIntake{OwnerScope: selfObservationOwnerScope(root), SchemaVersion: 1, ID: review.ID, Input: input, ReviewDigest: review.ReviewDigest, ApprovedAt: time.Now().UTC(), State: "approved", Revision: 1}
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := writeSelfObservationIntakes(root, append(entries, entry)); err != nil {
			return err
		}
		receipt = SelfObservationIntakeReceipt{entry.ID, observationID(input), entry.State, entry.Revision}
		return nil
	})
	return receipt, err
}

// The existing maintenance guard is acquired before the owner observation lock.
func CaptureSelfObservation(ctx context.Context, root, id, source string, guard func(func() error) error) (SelfObservationCapture, error) {
	if ctx == nil {
		return SelfObservationCapture{}, errors.New("self observation context is required")
	}
	if err := ctx.Err(); err != nil {
		return SelfObservationCapture{}, err
	}
	var result SelfObservationCapture
	if guard == nil {
		return result, errors.New("self capture publication guard is required")
	}
	err := guard(func() error {
		return withObservationLock(root, func(path string) error {
			records, err := readObservationRecords(root)
			if err != nil {
				return err
			}
			entries, err := readSelfObservationIntakes(root)
			if err != nil {
				return err
			}
			if selfObservationSource(root, entries, records, time.Now().UTC()).Fingerprint != source {
				return ErrRevisionConflict
			}
			for _, entry := range entries {
				if entry.ID != id {
					continue
				}
				if entry.State != "approved" || !entry.Input.ExpiresAt.After(time.Now().UTC()) {
					return errors.New("self intake is denied or expired")
				}
				if current, exists := latestObservations(records)[observationID(entry.Input)]; exists {
					result.Observation = observationReceipt(current, "capture_idempotent")
					return nil
				}
				now := time.Now().UTC()
				record := observationRecord{ObservationInput: entry.Input, ID: observationID(entry.Input), State: ObservationCaptured, RecordedAt: now, StateChangedAt: now}
				if err := ctx.Err(); err != nil {
					return err
				}
				if err := appendObservationRecord(path, record); err != nil {
					return err
				}
				result = SelfObservationCapture{Added: true, Observation: observationReceipt(record, "reviewed_owner_intake_captured")}
				return nil
			}
			return os.ErrNotExist
		})
	})
	return result, err
}
func VerifySelfObservationVersion(root, intakeID, id, revision string) error {
	return withObservationReadSnapshot(root, func(records []observationRecord) error {
		entries, err := readSelfObservationIntakes(root)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if entry.ID != intakeID || observationID(entry.Input) != id {
				continue
			}
			for _, record := range records {
				if record.ID == id && record.State == ObservationCaptured && record.SourceEvent == "owner.reviewed_declaration" && observationRevision(record) == revision && sameObservationInput(record.ObservationInput, entry.Input) && !record.RecordedAt.Before(entry.ApprovedAt) {
					return nil
				}
			}
		}
		return os.ErrNotExist
	})
}

// WithSelfObservationSource keeps the owning input/denial snapshot fixed through publication.
func WithSelfObservationSource(root, expected string, operation func() error) error {
	if operation == nil || !validDigest(expected) {
		return errors.New("self observation source fence is invalid")
	}
	return withObservationReadSnapshot(root, func(records []observationRecord) error {
		entries, err := readSelfObservationIntakes(root)
		if err != nil {
			return err
		}
		if selfObservationSource(root, entries, records, time.Now().UTC()).Fingerprint != expected {
			return ErrRevisionConflict
		}
		return operation()
	})
}

type SelfObservationControlReview struct {
	IntakeState         string                    `json:"intake_state"`
	ID                  string                    `json:"id"`
	Next                string                    `json:"next"`
	OwnerBinding        string                    `json:"owner_binding"`
	Submission          SelfObservationSubmission `json:"submission"`
	IntakeRevision      uint64                    `json:"intake_revision"`
	ObservationState    ObservationState          `json:"observation_state,omitempty"`
	ObservationRevision string                    `json:"observation_revision,omitempty"`
	SourceState         string                    `json:"source_state"`
	ReviewDigest        string                    `json:"review_digest"`
	seal                string
}

func reviewSelfObservationControl(root, id, next string, entries []selfObservationIntake, records []observationRecord) (SelfObservationControlReview, error) {
	if next != "rejected" && next != "redacted" {
		return SelfObservationControlReview{}, errors.New("self observation control is invalid")
	}
	for _, entry := range entries {
		if entry.ID != id {
			continue
		}
		i := entry.Input
		r := SelfObservationControlReview{ID: id, Next: next, OwnerBinding: selfObservationOwnerScope(root), Submission: SelfObservationSubmission{i.Signal, i.Facet, i.Claim, i.EvidenceType, i.SourceDigest, i.EpisodeID, i.Confidence, i.ExpiresAt}, IntakeRevision: entry.Revision, IntakeState: entry.State, SourceState: selfObservationSource(root, entries, records, time.Now().UTC()).Fingerprint}
		if current, exists := latestObservations(records)[observationID(i)]; exists {
			if current.State == ObservationPromoted || current.State == ObservationRedacted && next != "redacted" {
				return SelfObservationControlReview{}, errors.New("self control must preserve promoted or redacted owner state")
			}
			r.ObservationState = current.State
			r.ObservationRevision = observationRevision(current)
		}
		r.ReviewDigest = selfObservationDigest(r)
		r.seal = r.ReviewDigest
		return r, nil
	}
	return SelfObservationControlReview{}, os.ErrNotExist
}
func ReviewSelfObservationControl(root, id, next string) (SelfObservationControlReview, error) {
	var review SelfObservationControlReview
	err := withObservationReadSnapshot(root, func(records []observationRecord) error {
		entries, err := readSelfObservationIntakes(root)
		if err != nil {
			return err
		}
		review, err = reviewSelfObservationControl(root, id, next, entries, records)
		return err
	})
	return review, err
}

// The first owning write is the commit point; finish its coupled denial metadata.
func ApplySelfObservationControl(ctx context.Context, root string, review SelfObservationControlReview, confirmed bool) (SelfObservationIntakeReceipt, error) {
	if ctx == nil {
		return SelfObservationIntakeReceipt{}, errors.New("self observation context is required")
	}
	if err := ctx.Err(); err != nil {
		return SelfObservationIntakeReceipt{}, err
	}
	if !confirmed {
		return SelfObservationIntakeReceipt{}, ErrConfirmationRequired
	}
	if review.seal == "" || review.seal != review.ReviewDigest || review.OwnerBinding != selfObservationOwnerScope(root) {
		return SelfObservationIntakeReceipt{}, ErrRevisionConflict
	}
	preflight, err := ReviewSelfObservationControl(root, review.ID, review.Next)
	if err != nil {
		return SelfObservationIntakeReceipt{}, err
	}
	if preflight.ReviewDigest != review.ReviewDigest {
		return SelfObservationIntakeReceipt{}, ErrRevisionConflict
	}
	var result SelfObservationIntakeReceipt
	err = withObservationLock(root, func(path string) error {
		records, err := readObservationRecords(root)
		if err != nil {
			return err
		}
		entries, err := readSelfObservationIntakes(root)
		if err != nil {
			return err
		}
		fresh, err := reviewSelfObservationControl(root, review.ID, review.Next, entries, records)
		if err != nil {
			return err
		}
		if fresh.ReviewDigest != review.ReviewDigest {
			return ErrRevisionConflict
		}
		committed := false
		for index := range entries {
			entry := &entries[index]
			if entry.ID != review.ID {
				continue
			}
			if current, exists := latestObservations(records)[observationID(entry.Input)]; exists && current.State != ObservationState(review.Next) {
				if err := validateTransition(current, ObservationState(review.Next), records); err != nil {
					return err
				}
				previousRevision := observationRevision(current)
				current.ExpectedState = current.State
				current.ExpectedRevision = previousRevision
				current.State = ObservationState(review.Next)
				current.TransitionID = "intake-control-" + review.ReviewDigest[:40]
				current.OwnerAction = true
				current.Tombstone = review.Next == "redacted"
				current.StateChangedAt = time.Now().UTC()
				if err := ctx.Err(); err != nil {
					return err
				}
				if err := appendObservationRecord(path, current); err != nil {
					return err
				}
				committed = true
			}
			if entry.State != review.Next {
				if !committed {
					if err := ctx.Err(); err != nil {
						return err
					}
				}
				entry.State = review.Next
				entry.Revision++
				if err := writeSelfObservationIntakes(root, entries); err != nil {
					return err
				}
			}
			result = SelfObservationIntakeReceipt{entry.ID, observationID(entry.Input), entry.State, entry.Revision}
			return nil
		}
		return os.ErrNotExist
	})
	return result, err
}

func revokeSelfObservationIntakes(root string) error {
	entries, err := readSelfObservationIntakes(root)
	if err != nil {
		return err
	}
	changed := false
	for index := range entries {
		if entries[index].State == "approved" {
			entries[index].State = "redacted"
			entries[index].Revision++
			changed = true
		}
	}
	if !changed {
		return nil
	}
	return writeSelfObservationIntakes(root, entries)
}
