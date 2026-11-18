package lifecycle

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

const (
	SignalVersion     = "body-free-observation-v1"
	LifecycleProducer = "maestro.lifecycle"
	ExecutionProducer = "maestro.execution"
	ObservedOnly      = "observed_only"
	CommittedEffect   = "committed_effect"
	ExecutionComplete = "execution_complete"
	MaximumSignals    = 1024
)

// Signal contains only closed event labels and opaque source identity. Its
// attestation is provenance, never native invocation or execution authority.
type Signal struct {
	SchemaVersion  int       `json:"schema_version"`
	WorkspaceID    string    `json:"workspace_id"`
	Producer       string    `json:"producer"`
	Version        string    `json:"version"`
	Runtime        string    `json:"runtime"`
	Event          string    `json:"event"`
	EvidenceClass  string    `json:"evidence_class"`
	SourceID       string    `json:"source_id"`
	SourceRevision int       `json:"source_revision"`
	SourceDigest   string    `json:"source_digest"`
	OccurredAt     time.Time `json:"occurred_at"`
	Attestation    string    `json:"attestation,omitempty"`
}

func (s Signal) Validate() error {
	if s.SchemaVersion != 1 || !workspaceIDPattern.MatchString(s.WorkspaceID) || s.Version != SignalVersion || !idempotencyPattern.MatchString(s.SourceID) || s.SourceRevision < 1 || len(s.SourceDigest) != 64 || s.OccurredAt.IsZero() {
		return errors.New("signal_metadata_invalid")
	}
	if _, err := hex.DecodeString(s.SourceDigest); err != nil {
		return errors.New("signal_digest_invalid")
	}
	switch s.Producer {
	case LifecycleProducer:
		if (s.Runtime != "claude" && s.Runtime != "codex") || s.Event != StopFinalize || s.EvidenceClass != ObservedOnly || s.SourceRevision != 1 {
			return errors.New("signal_evidence_invalid")
		}
	case ExecutionProducer:
		if s.Runtime != "canonical" || s.Event != ExecutionComplete || s.EvidenceClass != CommittedEffect {
			return errors.New("signal_evidence_invalid")
		}
	default:
		return errors.New("signal_producer_invalid")
	}
	return nil
}

func (s Signal) OccurrenceID() string {
	return IdempotencyKey(s.Producer, s.Runtime, s.Event, s.SourceID)
}

func (s Signal) Digest() string {
	s.Attestation = ""
	body, _ := json.Marshal(s)
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

func SignalSourceDigest(v any) string {
	body, _ := json.Marshal(v)
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

func SignalFromReceipt(workspaceID string, r Receipt) (Signal, error) {
	if err := validateReceipt(r); err != nil {
		return Signal{}, errors.New("signal_source_invalid")
	}
	s := Signal{SchemaVersion: 1, WorkspaceID: workspaceID, Producer: LifecycleProducer, Version: SignalVersion, Runtime: r.Runtime, Event: r.Event, EvidenceClass: ObservedOnly, SourceID: r.IdempotencyKey, SourceRevision: 1, SourceDigest: SignalSourceDigest(r), OccurredAt: r.OccurredAt}
	return s, s.Validate()
}

func signalRoot(dataRoot, workspaceID string) (string, error) {
	root, err := validatedReceiptRoot(dataRoot, workspaceID)
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(filepath.Dir(root)), "capture-signals", workspaceID), nil
}

func decodeSignal(body []byte) (Signal, error) {
	var s Signal
	d := json.NewDecoder(bytes.NewReader(body))
	d.DisallowUnknownFields()
	if err := d.Decode(&s); err != nil {
		return s, errors.New("signal_record_invalid")
	}
	if d.Decode(new(any)) != io.EOF || s.Validate() != nil || len(s.Attestation) != 64 {
		return s, errors.New("signal_record_invalid")
	}
	if _, err := hex.DecodeString(s.Attestation); err != nil {
		return s, errors.New("signal_attestation_invalid")
	}
	return s, nil
}

// RecordSignalConfined queues an already-sealed observation, not a memory
// capture. Repeating an occurrence can never replace different metadata.
func RecordSignalConfined(dataRoot string, s Signal) error {
	if s.Validate() != nil || len(s.Attestation) != 64 {
		return errors.New("signal_record_invalid")
	}
	root, err := signalRoot(dataRoot, s.WorkspaceID)
	if err != nil {
		return err
	}
	if err = scheduler.ValidatePrivateDirectory(dataRoot); err != nil {
		return err
	}
	return scheduler.UpdatePrivateFile(filepath.Join(root, s.OccurrenceID()+".json"), maximumReceiptBytes, func(body []byte) ([]byte, error) {
		if len(body) > 0 {
			prior, err := decodeSignal(body)
			if err != nil || prior != s {
				return nil, errors.New("signal_occurrence_conflict")
			}
			return nil, nil
		}
		return json.Marshal(s)
	})
}

func Signals(dataRoot, workspaceID string) ([]Signal, error) {
	root, err := signalRoot(dataRoot, workspaceID)
	if err != nil {
		return nil, err
	}
	dir, err := scheduler.OpenDirectory(root)
	if errors.Is(err, os.ErrNotExist) {
		return []Signal{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer dir.Close()
	entries, err := dir.ReadDir(2*MaximumSignals + 1)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	if len(entries) > 2*MaximumSignals {
		return nil, errors.New("signal_capacity")
	}
	out := []Signal{}
	for _, e := range entries {
		if e.IsDir() || e.Type()&os.ModeSymlink != 0 {
			return nil, errors.New("signal_entry_invalid")
		}
		if filepath.Ext(e.Name()) != ".json" {
			continue
		}
		body, err := scheduler.ReadPrivateStateFile(filepath.Join(root, e.Name()), maximumReceiptBytes)
		if err != nil {
			return nil, err
		}
		s, err := decodeSignal(body)
		if err != nil || s.WorkspaceID != workspaceID || e.Name() != s.OccurrenceID()+".json" {
			return nil, errors.New("signal_scope_invalid")
		}
		out = append(out, s)
		if len(out) > MaximumSignals {
			return nil, errors.New("signal_capacity")
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].OccurrenceID() < out[j].OccurrenceID() })
	return out, nil
}

// ReceiptSource returns the same confined original used by the lifecycle
// producer so ingestion can reject a changed or removed source.
func ReceiptSource(dataRoot, workspaceID, event, sourceID string) (Receipt, error) {
	root, err := validatedReceiptRoot(dataRoot, workspaceID)
	if err != nil {
		return Receipt{}, err
	}
	if !validEvents[event] || !idempotencyPattern.MatchString(sourceID) {
		return Receipt{}, errors.New("signal_source_invalid")
	}
	body, err := scheduler.ReadPrivateStateFile(filepath.Join(root, event+"-"+sourceID+".json"), maximumReceiptBytes)
	if err != nil {
		return Receipt{}, err
	}
	var r Receipt
	d := json.NewDecoder(bytes.NewReader(body))
	d.DisallowUnknownFields()
	if d.Decode(&r) != nil || d.Decode(new(any)) != io.EOF || validateReceipt(r) != nil || r.Event != event || r.IdempotencyKey != sourceID {
		return Receipt{}, errors.New("signal_source_invalid")
	}
	return r, nil
}
