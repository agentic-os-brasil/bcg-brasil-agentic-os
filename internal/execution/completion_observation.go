package execution

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"reflect"
	"sort"
	"time"
)

// CompletionObservation witnesses historical committed results. It contains
// no original caller identity, objective, checkpoint, document or path text.
type CompletionObservation struct {
	SchemaVersion int       `json:"schema_version"`
	WorkspaceID   string    `json:"workspace_id"`
	SourceID      string    `json:"source_id"`
	Revision      int       `json:"revision"`
	OccurredAt    time.Time `json:"occurred_at"`
	WitnessDigest string    `json:"witness_digest"`
}

func observationHash(value any) string {
	body, _ := json.Marshal(value)
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

// ObserveCompletion reads the canonical revision and its passing witnesses;
// it does not execute commands or claim that artifacts are unchanged today.
func (store Store) ObserveCompletion(workspaceID, itemID string) (CompletionObservation, error) {
	item, err := store.Inspect(workspaceID, itemID)
	if err != nil {
		return CompletionObservation{}, err
	}
	if item.State.State != StateCompleted || item.Attempt == nil || item.Attempt.State != AttemptCompleted || item.State.ActiveAttemptID != "" {
		return CompletionObservation{}, errors.New("completion_observation_unavailable")
	}
	receipts, err := store.latestEvidence(workspaceID, itemID)
	if err != nil {
		return CompletionObservation{}, err
	}
	type witness struct {
		Criterion string
		Type      CriterionType
		Source    string
		Artifact  string
		Bytes     int64
		Command   string
		Tool      string
		At        time.Time
	}
	var witnesses []witness
	for _, criterion := range item.Contract.Criteria {
		r, ok := receipts[criterion.ID]
		if !ok || r.Outcome != EvidencePassed || r.WorkspaceID != workspaceID || r.ItemID != itemID || r.AttemptID != item.Attempt.AttemptID || r.Type != criterion.Type || r.ObservedAt.After(item.State.UpdatedAt) {
			return CompletionObservation{}, errors.New("completion_witness_invalid")
		}
		if r.Type == CriterionArtifactSnapshot && r.TargetRef != criterion.TargetRef {
			return CompletionObservation{}, errors.New("completion_witness_invalid")
		}
		if r.Type == CriterionCommandCheck {
			digest, err := commandDigest(criterion.Command)
			if err != nil || digest != r.CommandSHA256 || r.ExitCode == nil || *r.ExitCode != 0 {
				return CompletionObservation{}, errors.New("completion_witness_invalid")
			}
		}
		witnesses = append(witnesses, witness{observationHash(criterion.ID), r.Type, observationHash(r.TargetRef), r.ArtifactSHA256, r.ArtifactBytes, r.CommandSHA256, r.ToolSHA256, r.ObservedAt})
	}
	if len(witnesses) == 0 {
		return CompletionObservation{}, errors.New("completion_witness_unavailable")
	}
	sort.Slice(witnesses, func(i, j int) bool { return witnesses[i].Criterion < witnesses[j].Criterion })
	after, err := store.Inspect(workspaceID, itemID)
	if err != nil || !reflect.DeepEqual(item, after) {
		return CompletionObservation{}, errors.New("completion_source_changed")
	}
	return CompletionObservation{1, workspaceID, observationHash(itemID)[:32], item.State.StateRevision, item.State.UpdatedAt, observationHash(witnesses)}, nil
}
