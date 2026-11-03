package maintenance

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

const (
	MemoryCheckpointSchemaVersion = 1
	CheckpointSchedulerProvenance = "scheduler_receipts_v1"
)

var ErrCheckpointConflict = errors.New("memory checkpoint parent changed")

// MemoryCheckpoint is a body-free continuity watermark. It can prove that a
// bounded set of already-durable local scheduler metadata was checkpointed;
// it never claims that memory synthesis or context capture occurred.
type MemoryCheckpoint struct {
	SchemaVersion            int       `json:"schema_version"`
	WorkspaceID              string    `json:"workspace_id"`
	Revision                 int       `json:"revision"`
	Provenance               string    `json:"provenance"`
	SourceWatermarkSHA256    string    `json:"source_watermark_sha256"`
	SourceCount              int       `json:"source_count"`
	LatestSourceJobID        string    `json:"latest_source_job_id"`
	LatestSourceAt           time.Time `json:"latest_source_at"`
	CheckpointedAt           time.Time `json:"checkpointed_at"`
	PreviousCheckpointSHA256 string    `json:"previous_checkpoint_sha256,omitempty"`
}

type checkpointPointer struct {
	SchemaVersion    int    `json:"schema_version"`
	WorkspaceID      string `json:"workspace_id"`
	Revision         int    `json:"revision"`
	Artifact         string `json:"artifact"`
	CheckpointSHA256 string `json:"checkpoint_sha256"`
}

type checkpointSource struct {
	JobID        string                 `json:"job_id"`
	ScheduledFor time.Time              `json:"scheduled_for"`
	AttemptedAt  time.Time              `json:"attempted_at"`
	State        scheduler.ReceiptState `json:"state"`
}

type ContinuityCheckpointStore struct {
	Root             string
	FaultPoint       func(string) error
	PublicationGuard func(func() error) error
}

func (store ContinuityCheckpointStore) CommitSchedulerReceipts(workspaceID string, receipts []scheduler.Receipt, now time.Time) (MemoryCheckpoint, bool, error) {
	if now.IsZero() || !commandIDPattern.MatchString(workspaceID) {
		return MemoryCheckpoint{}, false, errors.New("memory checkpoint requires workspace and time")
	}
	sources := make([]checkpointSource, 0, len(receipts))
	for _, receipt := range receipts {
		if receipt.JobID == MemoryCheckpointJobID || receipt.State == scheduler.Suppressed {
			continue
		}
		if receipt.ScheduledFor.IsZero() || receipt.AttemptedAt.IsZero() || !validID(receipt.JobID) {
			return MemoryCheckpoint{}, false, errors.New("memory checkpoint source metadata is invalid")
		}
		sources = append(sources, checkpointSource{JobID: receipt.JobID, ScheduledFor: receipt.ScheduledFor.UTC(), AttemptedAt: receipt.AttemptedAt.UTC(), State: receipt.State})
	}
	if len(sources) == 0 {
		return MemoryCheckpoint{}, false, scheduler.ErrCapabilityUnavailable
	}
	sort.Slice(sources, func(left, right int) bool {
		if sources[left].AttemptedAt.Equal(sources[right].AttemptedAt) {
			if sources[left].JobID == sources[right].JobID {
				return sources[left].ScheduledFor.Before(sources[right].ScheduledFor)
			}
			return sources[left].JobID < sources[right].JobID
		}
		return sources[left].AttemptedAt.Before(sources[right].AttemptedAt)
	})
	sourceBody, err := json.Marshal(sources)
	if err != nil {
		return MemoryCheckpoint{}, false, err
	}
	sourceDigest := sha256.Sum256(sourceBody)
	checkpoint := MemoryCheckpoint{
		SchemaVersion: MemoryCheckpointSchemaVersion, WorkspaceID: workspaceID, Revision: 1,
		Provenance: CheckpointSchedulerProvenance, SourceWatermarkSHA256: hex.EncodeToString(sourceDigest[:]),
		SourceCount: len(sources), LatestSourceJobID: sources[len(sources)-1].JobID,
		LatestSourceAt: sources[len(sources)-1].AttemptedAt, CheckpointedAt: now.UTC(),
	}
	current, loadErr := store.Load(workspaceID)
	if loadErr == nil {
		if current.SourceWatermarkSHA256 == checkpoint.SourceWatermarkSHA256 {
			return current, false, nil
		}
		currentDigest, digestErr := checkpointDigest(current)
		if digestErr != nil {
			return MemoryCheckpoint{}, false, digestErr
		}
		checkpoint.Revision = current.Revision + 1
		checkpoint.PreviousCheckpointSHA256 = currentDigest
	} else if !errors.Is(loadErr, os.ErrNotExist) {
		return MemoryCheckpoint{}, false, loadErr
	}
	if err := validateMemoryCheckpoint(checkpoint); err != nil {
		return MemoryCheckpoint{}, false, err
	}
	digest, err := checkpointDigest(checkpoint)
	if err != nil {
		return MemoryCheckpoint{}, false, err
	}
	root := filepath.Join(store.Root, "workspaces", workspaceID, "memory-checkpoints")
	err = scheduler.EnsurePrivateDirectory(root)
	if err != nil {
		return MemoryCheckpoint{}, false, err
	}
	versions := filepath.Join(root, "versions")
	err = scheduler.EnsurePrivateDirectory(versions)
	if err != nil {
		return MemoryCheckpoint{}, false, err
	}
	artifact := fmt.Sprintf("%020d-%s.json", checkpoint.CheckpointedAt.UnixNano(), digest[:16])
	if err := writeCheckpointImmutable(filepath.Join(versions, artifact), checkpoint); err != nil {
		return MemoryCheckpoint{}, false, err
	}
	if err := syncCheckpointDirectory(versions); err != nil {
		return MemoryCheckpoint{}, false, err
	}
	if store.FaultPoint != nil {
		if err := store.FaultPoint("after_version"); err != nil {
			return MemoryCheckpoint{}, false, err
		}
	}
	pointer := checkpointPointer{SchemaVersion: MemoryCheckpointSchemaVersion, WorkspaceID: workspaceID, Revision: checkpoint.Revision, Artifact: artifact, CheckpointSHA256: digest}
	publish := func() error {
		// Distinct occurrences (including direct/Canary callers) share this
		// pointer. Keep parent comparison and replacement under one short lock,
		// inside any external control/lease guard and after private preparation.
		return scheduler.WithPrivateLock(filepath.Join(root, "publication.guard"), func() error {
			parent, err := store.Load(workspaceID)
			parentDigest := ""
			if err == nil {
				parentDigest, err = checkpointDigest(parent)
			} else if errors.Is(err, os.ErrNotExist) {
				err = nil
			}
			if err != nil {
				return err
			}
			if parentDigest != checkpoint.PreviousCheckpointSHA256 {
				return ErrCheckpointConflict
			}
			return writeCheckpointAtomic(filepath.Join(root, "current.json"), pointer)
		})
	}
	if store.PublicationGuard != nil {
		unguarded := publish
		publish = func() error { return store.PublicationGuard(unguarded) }
	}
	if err := publish(); err != nil {
		return MemoryCheckpoint{}, false, err
	}
	if err := syncCheckpointDirectory(root); err != nil {
		return MemoryCheckpoint{}, false, err
	}
	return checkpoint, true, nil
}

func (store ContinuityCheckpointStore) Load(workspaceID string) (MemoryCheckpoint, error) {
	if !commandIDPattern.MatchString(workspaceID) {
		return MemoryCheckpoint{}, errors.New("invalid memory checkpoint workspace")
	}
	root := filepath.Join(store.Root, "workspaces", workspaceID, "memory-checkpoints")
	err := scheduler.ValidatePrivateDirectory(root)
	if err != nil {
		return MemoryCheckpoint{}, err
	}
	var pointer checkpointPointer
	if err := readCheckpointJSON(filepath.Join(root, "current.json"), &pointer); err != nil {
		return MemoryCheckpoint{}, err
	}
	if pointer.SchemaVersion != MemoryCheckpointSchemaVersion || pointer.WorkspaceID != workspaceID || pointer.Revision < 1 || !digestPattern.MatchString(pointer.CheckpointSHA256) || filepath.Base(pointer.Artifact) != pointer.Artifact || filepath.Ext(pointer.Artifact) != ".json" {
		return MemoryCheckpoint{}, errors.New("invalid memory checkpoint pointer")
	}
	var checkpoint MemoryCheckpoint
	if err := readCheckpointJSON(filepath.Join(root, "versions", pointer.Artifact), &checkpoint); err != nil {
		return MemoryCheckpoint{}, err
	}
	if err := validateMemoryCheckpoint(checkpoint); err != nil {
		return MemoryCheckpoint{}, err
	}
	digest, err := checkpointDigest(checkpoint)
	if err != nil {
		return MemoryCheckpoint{}, err
	}
	if checkpoint.WorkspaceID != workspaceID || checkpoint.Revision != pointer.Revision || digest != pointer.CheckpointSHA256 {
		return MemoryCheckpoint{}, errors.New("memory checkpoint pointer binding is invalid")
	}
	return checkpoint, nil
}

type MemoryCheckpointHandler struct {
	Scheduler                 scheduler.Store
	Store                     ContinuityCheckpointStore
	ExpectedSourceFingerprint string
}

func (handler MemoryCheckpointHandler) ExecuteAuthorized(ctx context.Context, command Command, grant ExecutionGrant) (HandlerResult, error) {
	if err := ValidateExecutionGrant(grant, command); err != nil {
		return HandlerResult{}, err
	}
	if command.JobID != MemoryCheckpointJobID || command.WorkspaceID == "" || command.Trigger != TriggerPresence {
		return HandlerResult{}, errors.New("memory checkpoint command is outside its workspace continuity boundary")
	}
	receipts, err := handler.Scheduler.Receipts(command.WorkspaceID)
	if err != nil {
		return HandlerResult{}, err
	}
	if handler.ExpectedSourceFingerprint != "" && handler.ExpectedSourceFingerprint != productDigest(receipts) {
		return HandlerResult{}, ErrProductConflict
	}
	store := handler.Store
	store.PublicationGuard = func(publish func() error) error { return publishWithGrant(ctx, grant, command, publish) }
	_, changed, err := store.CommitSchedulerReceipts(command.WorkspaceID, receipts, command.RequestedAt)
	if err != nil {
		return HandlerResult{}, err
	}
	if !changed {
		return HandlerResult{State: ReceiptReviewedNoChange, ReasonCode: ReasonReviewedNoChange}, nil
	}
	return HandlerResult{State: ReceiptSucceeded, ReasonCode: ReasonCompleted}, nil
}

func validateMemoryCheckpoint(checkpoint MemoryCheckpoint) error {
	if checkpoint.SchemaVersion != MemoryCheckpointSchemaVersion || !commandIDPattern.MatchString(checkpoint.WorkspaceID) || checkpoint.Revision < 1 || checkpoint.Provenance != CheckpointSchedulerProvenance || !digestPattern.MatchString(checkpoint.SourceWatermarkSHA256) || checkpoint.SourceCount < 1 || !validID(checkpoint.LatestSourceJobID) || checkpoint.LatestSourceAt.IsZero() || checkpoint.CheckpointedAt.IsZero() || checkpoint.LatestSourceAt.After(checkpoint.CheckpointedAt) {
		return errors.New("invalid memory checkpoint")
	}
	if checkpoint.Revision == 1 && checkpoint.PreviousCheckpointSHA256 != "" {
		return errors.New("initial memory checkpoint cannot have a predecessor")
	}
	if checkpoint.Revision > 1 && !digestPattern.MatchString(checkpoint.PreviousCheckpointSHA256) {
		return errors.New("memory checkpoint revision requires a predecessor digest")
	}
	return nil
}

func checkpointDigest(checkpoint MemoryCheckpoint) (string, error) {
	body, err := json.Marshal(checkpoint)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:]), nil
}

func writeCheckpointImmutable(path string, value any) error {
	body, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return scheduler.WriteNewPrivateFile(path, append(body, '\n'))
}

func writeCheckpointAtomic(path string, value any) error {
	body, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return scheduler.WritePrivateFileAtomic(path, append(body, '\n'))
}

func readCheckpointJSON(path string, target any) error {
	body, err := scheduler.ReadPrivateFile(path, 64<<10)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("memory checkpoint file contains multiple JSON values")
		}
		return err
	}
	return nil
}
