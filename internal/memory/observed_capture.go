package memory

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/lifecycle"
)

// Prepare belongs only to explicit setup; provenance grants no execution power.
func (a CaptureAttestor) Prepare(workspaceID string) error {
	_, err := a.key(workspaceID, true)
	return err
}

func signalMAC(key []byte, s lifecycle.Signal) string {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte("maestro-observed-signal-v1\x00"))
	mac.Write([]byte(s.Digest()))
	return hex.EncodeToString(mac.Sum(nil))
}

func (a CaptureAttestor) SealSignal(s lifecycle.Signal) (lifecycle.Signal, error) {
	if s.Validate() != nil {
		return lifecycle.Signal{}, errors.New("signal_metadata_invalid")
	}
	key, err := a.key(s.WorkspaceID, false)
	if err != nil {
		return lifecycle.Signal{}, err
	}
	s.Attestation = signalMAC(key, s)
	return s, nil
}

func (a CaptureAttestor) VerifySignal(s lifecycle.Signal) error {
	if s.Validate() != nil || len(s.Attestation) != 64 {
		return errors.New("signal_attestation_invalid")
	}
	key, err := a.key(s.WorkspaceID, false)
	if err != nil {
		return err
	}
	got, err := hex.DecodeString(s.Attestation)
	if err != nil {
		return errors.New("signal_attestation_invalid")
	}
	want, _ := hex.DecodeString(signalMAC(key, s))
	if !hmac.Equal(got, want) {
		return errors.New("signal_attestation_invalid")
	}
	return nil
}

func signalText(s lifecycle.Signal) string {
	return fmt.Sprintf("%s: %s/%s; observed_at=%s; source=%s; revision=%d", s.EvidenceClass, s.Runtime, s.Event, s.OccurredAt.UTC().Format("2006-01-02T15:04:05.999999999Z"), s.SourceID, s.SourceRevision)
}

type SignalCaptureReceipt struct {
	Capture    CaptureReceipt `json:"capture"`
	Added      bool           `json:"added"`
	Suppressed bool           `json:"suppressed"`
}
type signalAdmission struct {
	WorkspaceID  string `json:"workspace_id"`
	OccurrenceID string `json:"occurrence_id"`
	Digest       string `json:"digest"`
	CaptureID    string `json:"capture_id"`
}

// CaptureSignal reuses the workspace activation lock and permanent denial
// barrier. An admission records intent, never proof that its append completed.
func (e *Engine) CaptureSignal(s lifecycle.Signal) (SignalCaptureReceipt, error) {
	if err := e.ownedPreflight(s.WorkspaceID); err != nil {
		return SignalCaptureReceipt{}, err
	}
	a := CaptureAttestor{Root: e.Root}
	if err := a.VerifySignal(s); err != nil {
		return SignalCaptureReceipt{}, err
	}
	c := Capture{SchemaVersion: AttestedCaptureSchemaVersion, WorkspaceID: s.WorkspaceID, Kind: "observed_signal", Text: signalText(s), Sanitized: true, ProducerID: s.Producer, SanitizerID: lifecycle.SignalVersion, SourceDigest: s.SourceDigest, Observation: &s}
	id := journalCaptureID(c)
	release, err := e.acquireCycleLock(s.WorkspaceID, "activation")
	if err != nil {
		return SignalCaptureReceipt{}, err
	}
	defer release()
	barrier, err := e.readVisibility(s.WorkspaceID)
	if err != nil {
		return SignalCaptureReceipt{}, err
	}
	admission := signalAdmission{s.WorkspaceID, s.OccurrenceID(), s.Digest(), id}
	path := filepath.Join(e.workspaceRoot(s.WorkspaceID), "signal-admissions", s.OccurrenceID()+".json")
	body, readErr := memoryReadFile(path, 4096)
	if readErr == nil {
		var prior signalAdmission
		if decodeMemoryJSON(body, &prior) != nil || prior != admission {
			return SignalCaptureReceipt{}, errors.New("signal_occurrence_conflict")
		}
	} else if !errors.Is(readErr, os.ErrNotExist) {
		return SignalCaptureReceipt{}, readErr
	}
	if barrier.Denied[id] > 0 {
		return SignalCaptureReceipt{Capture: CaptureReceipt{ID: id, VisibilityRevision: barrier.Revision}, Suppressed: true}, nil
	}
	if prior, err := e.currentCapture(s.WorkspaceID, id); err == nil {
		if prior.Observation == nil || prior.Observation.Digest() != s.Digest() {
			return SignalCaptureReceipt{}, errors.New("signal_occurrence_conflict")
		}
		return SignalCaptureReceipt{Capture: captureReceipt(prior, barrier.Revision)}, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return SignalCaptureReceipt{}, err
	}
	publish := func() error {
		if errors.Is(readErr, os.ErrNotExist) {
			if err := memoryMkdirAll(filepath.Dir(path), 0700); err != nil {
				return err
			}
			b, _ := json.Marshal(admission)
			if err := memoryPublish(path, b, true); err != nil {
				return err
			}
		}
		if e.FaultPoint != nil {
			if err := e.FaultPoint("signal_admitted"); err != nil {
				return err
			}
		}
		c.RecordedAt = e.Now().UTC()
		if c.RecordedAt.Before(s.OccurredAt) {
			return errors.New("signal_source_in_future")
		}
		var err error
		c, err = a.Seal(c)
		if err != nil {
			return err
		}
		journal := e.currentPath(s.WorkspaceID, "l1", filepath.Join("attested-captures", c.RecordedAt.UTC().Format("2006-01-02")+".jsonl"))
		prior, err := memoryReadFile(journal, 1<<20)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		b, _ := json.Marshal(c)
		if len(prior)+len(b)+1 > 1<<20 {
			return ErrCaptureCapacity
		}
		barrier, err = e.publishCaptureVisibility(s.WorkspaceID, id)
		if err != nil {
			return err
		}
		if err := memoryMkdirAll(filepath.Dir(journal), 0700); err != nil {
			return err
		}
		if err := memoryPublish(journal, append(prior, append(b, '\n')...), false); err != nil {
			return err
		}
		if e.FaultPoint != nil {
			return e.FaultPoint("signal_published")
		}
		return nil
	}
	if e.PublicationGuard != nil {
		err = e.PublicationGuard(publish)
	} else {
		err = publish()
	}
	if err != nil {
		return SignalCaptureReceipt{}, err
	}
	return SignalCaptureReceipt{Capture: captureReceipt(identifiedJournalCapture(c), barrier.Revision), Added: true}, nil
}
