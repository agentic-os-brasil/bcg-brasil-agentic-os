package memory

import (
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	MaxOwnedCaptureBytes      = 8192
	OwnedCaptureKind          = "owner_reviewed"
	OwnedCaptureProducerID    = "maestro.product-memory"
	OwnedCaptureSanitizerID   = "human-reviewed-text-v1"
	OwnedCaptureSchemaVersion = 3
)

var (
	ErrCaptureConflict        = errors.New("memory capture revision conflict")
	ErrCaptureUnavailable     = errors.New("memory capture unavailable")
	ErrCaptureCapacity        = errors.New("memory capture identity capacity reached; reads and removal remain available")
	ErrConfinementUnavailable = errors.New("owner-reviewed memory requires qualified descriptor confinement")
	ownedCaptureIDPattern     = regexp.MustCompile(`^[a-f0-9]{32}$`)
	visibilityNamePattern     = regexp.MustCompile(`^[0-9]{20}\.json$`)
)

// OwnedCaptureInput is sealed by a trusted server only after exact human review.
// Its private binding prevents a reviewed replacement from being retargeted.
type OwnedCaptureInput struct {
	Text                       string
	At                         time.Time
	workspaceID, id, seal      string
	expectedRevision           uint64
	hasReviewedVisibility      bool
	reviewedVisibilityRevision uint64
}

type CaptureReceipt struct {
	Eligible           bool   `json:"eligible"`
	ID                 string `json:"id"`
	Revision           uint64 `json:"revision"`
	SourceDigest       string `json:"source_digest"`
	VisibilityRevision uint64 `json:"visibility_revision"`
}
type CaptureRecord struct {
	CaptureReceipt
	Text        string    `json:"text"`
	RecordedAt  time.Time `json:"recorded_at"`
	Kind        string    `json:"kind"`
	ProducerID  string    `json:"producer_id"`
	SanitizerID string    `json:"sanitizer_id"`
}
type MutationReceipt struct {
	VisibilityRevision uint64 `json:"visibility_revision"`
	Removed            int    `json:"removed"`
	Complete           bool   `json:"complete"`
}

type visibilityBarrier struct {
	SchemaVersion int               `json:"schema_version"`
	WorkspaceID   string            `json:"workspace_id"`
	Revision      uint64            `json:"revision"`
	Generation    uint64            `json:"generation"`
	Denied        map[string]uint64 `json:"denied"`
	LegacyThrough uint64            `json:"legacy_through,omitempty"`
}

func (engine *Engine) ownedPreflight(workspaceID string) error {
	if err := ownedIOAvailable(); err != nil {
		return err
	}
	if err := engine.validateCore(); err != nil {
		return err
	}
	if !filepath.IsAbs(engine.Root) {
		return errors.New("memory root must be absolute")
	}
	return validateWorkspaceID(workspaceID)
}

// PrepareOwnedCaptureAuthority belongs exclusively to explicit local setup.
func (engine *Engine) PrepareOwnedCaptureAuthority(workspaceID string) error {
	if err := engine.ownedPreflight(workspaceID); err != nil {
		return err
	}
	if err := Bootstrap(engine.Root, workspaceID); err != nil {
		return err
	}
	_, err := (CaptureAttestor{Root: engine.Root}).ownedKey(workspaceID, true)
	return err
}

// SealOwnedCapture does not create authority. The caller owns trusted review.
func (engine *Engine) SealOwnedCapture(workspaceID, id string, expectedRevision uint64, text string, at time.Time) (OwnedCaptureInput, error) {
	if err := engine.ownedPreflight(workspaceID); err != nil {
		return OwnedCaptureInput{}, err
	}
	if at.IsZero() || !utf8.ValidString(text) || len(text) > MaxOwnedCaptureBytes || strings.TrimSpace(text) == "" || strings.ContainsRune(text, 0) {
		return OwnedCaptureInput{}, errors.New("reviewed memory text must be bounded UTF-8 with a timestamp")
	}
	if id == "" {
		if expectedRevision != 0 {
			return OwnedCaptureInput{}, ErrCaptureConflict
		}
		id = memoryRandomID()
	} else if !ownedCaptureIDPattern.MatchString(id) || expectedRevision == 0 {
		return OwnedCaptureInput{}, ErrCaptureConflict
	}
	in := OwnedCaptureInput{Text: text, At: at.UTC(), workspaceID: workspaceID, id: id, expectedRevision: expectedRevision}
	candidate := ownedCaptureEnvelope(workspaceID, id, expectedRevision+1, in.Text, in.At)
	candidate.Attestation = strings.Repeat("0", 64)
	body, err := json.Marshal(candidate)
	if err != nil {
		return OwnedCaptureInput{}, err
	}
	if len(body) > MaxOwnedCaptureBytes+4096 {
		return OwnedCaptureInput{}, errors.New("reviewed memory text exceeds encoded persistence bound")
	}
	key, err := (CaptureAttestor{Root: engine.Root}).ownedKey(workspaceID, false)
	if err != nil {
		return OwnedCaptureInput{}, err
	}
	in.seal = ownedInputMAC(key, in)
	return in, nil
}

func ownedInputMAC(key []byte, in OwnedCaptureInput) string {
	envelope := []any{in.workspaceID, in.id, in.expectedRevision, in.Text, in.At}
	// Preserve existing unfenced seal bytes while binding every reviewed fence.
	if in.hasReviewedVisibility || in.reviewedVisibilityRevision != 0 {
		envelope = append(envelope, "reviewed-visibility-v1", in.hasReviewedVisibility, in.reviewedVisibilityRevision)
	}
	body, _ := json.Marshal(envelope)
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte("maestro-reviewed-memory-input-v1\x00"))
	_, _ = mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

// SealReviewedOwnedCapture binds a new capture to the exact reviewed visibility.
func (engine *Engine) SealReviewedOwnedCapture(workspaceID string, reviewedVisibilityRevision uint64, text string, at time.Time) (OwnedCaptureInput, error) {
	in, err := engine.SealOwnedCapture(workspaceID, "", 0, text, at)
	if err != nil {
		return OwnedCaptureInput{}, err
	}
	in.hasReviewedVisibility = true
	in.reviewedVisibilityRevision = reviewedVisibilityRevision
	key, err := (CaptureAttestor{Root: engine.Root}).ownedKey(workspaceID, false)
	if err != nil {
		return OwnedCaptureInput{}, err
	}
	in.seal = ownedInputMAC(key, in)
	return in, nil
}

func (engine *Engine) verifyOwnedInput(workspaceID, id string, revision uint64, in OwnedCaptureInput) error {
	if in.workspaceID != workspaceID || in.id != id || in.expectedRevision != revision || !ownedCaptureIDPattern.MatchString(id) || len(in.Text) > MaxOwnedCaptureBytes || !utf8.ValidString(in.Text) || strings.TrimSpace(in.Text) == "" || in.At.IsZero() {
		return errors.New("reviewed memory binding is invalid")
	}
	key, err := (CaptureAttestor{Root: engine.Root}).ownedKey(workspaceID, false)
	if err != nil {
		return err
	}
	if !hmac.Equal([]byte(in.seal), []byte(ownedInputMAC(key, in))) {
		return errors.New("memory text differs from trusted review")
	}
	return nil
}

func (engine *Engine) CaptureOwned(workspaceID string, in OwnedCaptureInput) (CaptureReceipt, error) {
	if err := engine.ownedPreflight(workspaceID); err != nil {
		return CaptureReceipt{}, err
	}
	if err := engine.verifyOwnedInput(workspaceID, in.id, 0, in); err != nil {
		return CaptureReceipt{}, err
	}
	release, err := engine.acquireCycleLock(workspaceID, "activation")
	if err != nil {
		return CaptureReceipt{}, err
	}
	defer release()
	barrier, err := engine.readVisibility(workspaceID)
	if err != nil {
		return CaptureReceipt{}, err
	}
	if in.hasReviewedVisibility && barrier.Revision != in.reviewedVisibilityRevision {
		return CaptureReceipt{}, ErrCaptureConflict
	}
	if denied := barrier.Denied[in.id]; denied > 0 {
		return CaptureReceipt{}, ErrCaptureConflict
	}
	if existing, err := engine.readOwnedCapture(workspaceID, in.id); err == nil {
		if existing.Revision == 1 && existing.Text == in.Text && existing.RecordedAt.Equal(in.At) {
			return captureReceipt(existing, barrier.Revision), nil
		}
		return CaptureReceipt{}, ErrCaptureConflict
	} else if !errors.Is(err, os.ErrNotExist) {
		return CaptureReceipt{}, err
	}
	barrier, err = engine.publishCaptureVisibility(workspaceID, in.id)
	if err != nil {
		return CaptureReceipt{}, err
	}
	c, err := engine.persistOwnedCapture(workspaceID, in, 1, true)
	if err != nil {
		return CaptureReceipt{}, err
	}
	return captureReceipt(c, barrier.Revision), nil
}

func captureReceipt(c Capture, visibility uint64) CaptureReceipt {
	return CaptureReceipt{ID: c.ID, Revision: c.Revision, SourceDigest: c.SourceDigest, VisibilityRevision: visibility, Eligible: c.SchemaVersion == AttestedCaptureSchemaVersion || c.SchemaVersion == OwnedCaptureSchemaVersion}
}

func (engine *Engine) ownedPath(workspaceID, id string) string {
	return engine.currentPath(workspaceID, "l1", filepath.Join("owned-captures", id+".json"))
}

func ownedCaptureEnvelope(workspaceID, id string, revision uint64, text string, at time.Time) Capture {
	sum := sha256.Sum256([]byte(text))
	return Capture{SchemaVersion: OwnedCaptureSchemaVersion, WorkspaceID: workspaceID, ID: id, Revision: revision, RecordedAt: at, Kind: OwnedCaptureKind, Text: text, Sanitized: true, ProducerID: OwnedCaptureProducerID, SanitizerID: OwnedCaptureSanitizerID, SourceDigest: hex.EncodeToString(sum[:])}
}

func (engine *Engine) persistOwnedCapture(workspaceID string, in OwnedCaptureInput, revision uint64, immutable bool) (Capture, error) {
	c := ownedCaptureEnvelope(workspaceID, in.id, revision, in.Text, in.At)
	key, err := (CaptureAttestor{Root: engine.Root}).ownedKey(workspaceID, false)
	if err != nil {
		return Capture{}, err
	}
	c.Attestation = ownedCaptureMAC(key, c)
	path := engine.ownedPath(workspaceID, c.ID)
	if err := memoryMkdirAll(filepath.Dir(path), 0700); err != nil {
		return Capture{}, err
	}
	body, err := json.Marshal(c)
	if err != nil {
		return Capture{}, err
	}
	if err = memoryPublish(path, body, immutable); err != nil {
		return Capture{}, err
	}
	return c, nil
}

func (engine *Engine) readOwnedCapture(workspaceID, id string) (Capture, error) {
	if !ownedCaptureIDPattern.MatchString(id) {
		return Capture{}, ErrCaptureUnavailable
	}
	body, err := memoryReadFile(engine.ownedPath(workspaceID, id), MaxOwnedCaptureBytes+4096)
	if err != nil {
		return Capture{}, err
	}
	var c Capture
	if err = decodeMemoryJSON(body, &c); err != nil {
		return Capture{}, err
	}
	if c.WorkspaceID != workspaceID || c.ID != id {
		return Capture{}, ErrCaptureUnavailable
	}
	if err = (CaptureAttestor{Root: engine.Root}).Verify(c); err != nil {
		return Capture{}, err
	}
	return c, nil
}

func (engine *Engine) ReadCapture(workspaceID, id string) (CaptureRecord, error) {
	if err := engine.ownedPreflight(workspaceID); err != nil {
		return CaptureRecord{}, err
	}
	b, err := engine.readVisibility(workspaceID)
	if err != nil {
		return CaptureRecord{}, err
	}
	c, err := engine.currentCapture(workspaceID, id)
	if errors.Is(err, os.ErrNotExist) {
		return CaptureRecord{}, ErrCaptureUnavailable
	}
	if err != nil {
		return CaptureRecord{}, err
	}
	if c.Revision <= b.Denied[id] {
		return CaptureRecord{}, ErrCaptureUnavailable
	}
	// Recheck after reading so a concurrent revocation cannot expose old text.
	after, err := engine.readVisibility(workspaceID)
	if err != nil {
		return CaptureRecord{}, err
	}
	if c.Revision <= after.Denied[id] {
		return CaptureRecord{}, ErrCaptureUnavailable
	}
	return CaptureRecord{CaptureReceipt: captureReceipt(c, after.Revision), Text: c.Text, RecordedAt: c.RecordedAt, Kind: c.Kind, ProducerID: c.ProducerID, SanitizerID: c.SanitizerID}, nil
}

func (engine *Engine) CorrectCapture(workspaceID, id string, expectedRevision uint64, in OwnedCaptureInput) (CaptureReceipt, error) {
	if err := engine.ownedPreflight(workspaceID); err != nil {
		return CaptureReceipt{}, err
	}
	if err := engine.verifyOwnedInput(workspaceID, id, expectedRevision, in); err != nil {
		return CaptureReceipt{}, err
	}
	if expectedRevision == ^uint64(0) {
		return CaptureReceipt{}, ErrCaptureConflict
	}
	release, err := engine.acquireCycleLock(workspaceID, "activation")
	if err != nil {
		return CaptureReceipt{}, err
	}
	defer release()
	current, err := engine.ReadCapture(workspaceID, id)
	if err != nil {
		return CaptureReceipt{}, err
	}
	if current.Revision != expectedRevision {
		return CaptureReceipt{}, ErrCaptureConflict
	}
	b, err := engine.publishDenial(workspaceID, map[string]uint64{id: expectedRevision})
	if err != nil {
		return CaptureReceipt{}, err
	}
	if err = engine.fault("after_visibility_barrier"); err != nil {
		return CaptureReceipt{}, err
	}
	if err = engine.purgeDenied(workspaceID, b); err != nil {
		return CaptureReceipt{}, err
	}
	c, err := engine.persistOwnedCapture(workspaceID, in, expectedRevision+1, true)
	if err != nil {
		return CaptureReceipt{}, err
	}
	return captureReceipt(c, b.Revision), nil
}

func (engine *Engine) DeleteCapture(workspaceID, id string, expectedRevision uint64) (MutationReceipt, error) {
	if err := engine.ownedPreflight(workspaceID); err != nil {
		return MutationReceipt{}, err
	}
	if !ownedCaptureIDPattern.MatchString(id) || expectedRevision == 0 {
		return MutationReceipt{}, ErrCaptureConflict
	}
	release, err := engine.acquireCycleLock(workspaceID, "activation")
	if err != nil {
		return MutationReceipt{}, err
	}
	defer release()
	b, err := engine.readVisibility(workspaceID)
	if err != nil {
		return MutationReceipt{}, err
	}
	if c, e := engine.currentCapture(workspaceID, id); e == nil && c.Revision > expectedRevision {
		return MutationReceipt{}, ErrCaptureConflict
	} else if e != nil && !errors.Is(e, os.ErrNotExist) {
		return MutationReceipt{}, e
	}
	if b.Denied[id] != expectedRevision {
		c, err := engine.ReadCapture(workspaceID, id)
		if err != nil {
			return MutationReceipt{}, err
		}
		if c.Revision != expectedRevision {
			return MutationReceipt{}, ErrCaptureConflict
		}
		b, err = engine.publishDenial(workspaceID, map[string]uint64{id: expectedRevision})
		if err != nil {
			return MutationReceipt{}, err
		}
	}
	if err = engine.fault("after_visibility_barrier"); err != nil {
		return MutationReceipt{}, err
	}
	if err = engine.purgeDenied(workspaceID, b); err != nil {
		return MutationReceipt{}, err
	}
	return MutationReceipt{VisibilityRevision: b.Revision, Removed: 1, Complete: true}, nil
}

func (engine *Engine) VisibilityRevision(workspaceID string) (uint64, error) {
	if err := engine.ownedPreflight(workspaceID); err != nil {
		return 0, err
	}
	b, err := engine.readVisibility(workspaceID)
	return b.Revision, err
}

func (engine *Engine) ApplyRetention(workspaceID string, cutoff time.Time, expectedVisibilityRevision uint64) (MutationReceipt, error) {
	if err := engine.ownedPreflight(workspaceID); err != nil {
		return MutationReceipt{}, err
	}
	if cutoff.IsZero() {
		return MutationReceipt{}, errors.New("retention requires an explicit cutoff")
	}
	release, err := engine.acquireCycleLock(workspaceID, "activation")
	if err != nil {
		return MutationReceipt{}, err
	}
	defer release()
	b, err := engine.readVisibility(workspaceID)
	if err != nil {
		return MutationReceipt{}, err
	}
	if b.Revision != expectedVisibilityRevision {
		return MutationReceipt{}, ErrCaptureConflict
	}
	captures, err := engine.allCaptures(workspaceID)
	if err != nil {
		return MutationReceipt{}, err
	}
	denied := map[string]uint64{}
	for _, c := range captures {
		if c.Revision > b.Denied[c.ID] && c.RecordedAt.Before(cutoff) {
			denied[c.ID] = c.Revision
		}
	}
	if len(denied) > 0 {
		b, err = engine.publishDenial(workspaceID, denied)
		if err != nil {
			return MutationReceipt{}, err
		}
		if err = engine.fault("after_visibility_barrier"); err != nil {
			return MutationReceipt{}, err
		}
	}
	if len(b.Denied) > 0 {
		if err = engine.purgeDenied(workspaceID, b); err != nil {
			return MutationReceipt{}, err
		}
	}
	return MutationReceipt{VisibilityRevision: b.Revision, Removed: len(denied), Complete: true}, nil
}

func (engine *Engine) ownedCaptures(workspaceID string) ([]Capture, error) {
	path := engine.currentPath(workspaceID, "l1", "owned-captures")
	entries, err := memoryReadDir(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	captures := make([]Capture, 0, len(entries))
	for _, entry := range entries {
		id := strings.TrimSuffix(entry.Name(), ".json")
		if entry.IsDir() || !ownedCaptureIDPattern.MatchString(id) || entry.Name() != id+".json" {
			return nil, errors.New("invalid owned capture entry")
		}
		c, err := engine.readOwnedCapture(workspaceID, id)
		if err != nil {
			return nil, err
		}
		captures = append(captures, c)
	}
	return captures, nil
}

func (engine *Engine) readLegacyVisibility(workspaceID string) (visibilityBarrier, error) {
	b := visibilityBarrier{SchemaVersion: 1, WorkspaceID: workspaceID, Denied: map[string]uint64{}}
	if err := validateWorkspaceID(workspaceID); err != nil {
		return b, err
	}
	directory := filepath.Join(engine.workspaceRoot(workspaceID), ".visibility")
	entries, err := memoryReadDir(directory)
	if errors.Is(err, os.ErrNotExist) {
		return b, nil
	}
	if err != nil {
		return b, err
	}
	totalBytes := 0
	for _, entry := range entries {
		if entry.IsDir() || !visibilityNamePattern.MatchString(entry.Name()) {
			return b, errors.New("invalid memory visibility barrier entry")
		}
		body, err := memoryReadFile(filepath.Join(directory, entry.Name()), 1<<20)
		if err != nil {
			return b, err
		}
		totalBytes += len(body)
		if totalBytes > 8<<20 {
			return b, errors.New("memory visibility history exceeds read bound")
		}
		var next visibilityBarrier
		if err = decodeMemoryJSON(body, &next); err != nil {
			return b, err
		}
		if next.SchemaVersion != 1 || next.WorkspaceID != workspaceID || next.Revision != b.Revision+1 || next.Generation < b.Generation || next.Generation > next.Revision || entry.Name() != fmt.Sprintf("%020d.json", next.Revision) || len(next.Denied) > 4096 {
			return b, errors.New("invalid memory visibility barrier")
		}
		for id, revision := range b.Denied {
			if next.Denied[id] < revision {
				return b, errors.New("memory visibility barrier regressed")
			}
		}
		for id, revision := range next.Denied {
			if !ownedCaptureIDPattern.MatchString(id) || revision == 0 {
				return b, errors.New("invalid denied capture identity")
			}
		}
		b = next
	}
	return b, nil
}

func (engine *Engine) purgeGenerated(workspaceID string) error {
	barrier, err := engine.readVisibility(workspaceID)
	if err != nil {
		return err
	}
	for _, name := range []string{"versions", ".transactions"} {
		directory := filepath.Join(engine.workspaceRoot(workspaceID), name)
		entries, err := memoryReadDir(directory)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if name == "versions" && entry.IsDir() {
				files, err := memoryReadDir(filepath.Join(directory, entry.Name()))
				if err != nil {
					return err
				}
				current := len(files) > 0
				for _, file := range files {
					body, err := memoryReadFile(filepath.Join(directory, entry.Name(), file.Name()), 1<<20)
					if err != nil {
						return err
					}
					var artifact Artifact
					if err = decodeMemoryJSON(body, &artifact); err != nil {
						current = false
						continue
					}
					if artifact.VisibilityRevision != barrier.Generation {
						current = false
					}
				}
				if current {
					continue
				}
			}
			if err = memoryRemoveAll(filepath.Join(directory, entry.Name())); err != nil {
				return err
			}
		}
	}
	return nil
}
func (engine *Engine) purgeDenied(workspaceID string, b visibilityBarrier) error {
	if err := engine.purgeGenerated(workspaceID); err != nil {
		return err
	}
	if err := engine.purgeDeniedJournals(workspaceID, b); err != nil {
		return err
	}
	captures, err := engine.ownedCaptures(workspaceID)
	if err != nil {
		return err
	}
	for _, c := range captures {
		if c.Revision <= b.Denied[c.ID] {
			if err = memoryRemoveAll(engine.ownedPath(workspaceID, c.ID)); err != nil {
				return err
			}
		}
	}
	return nil
}

func (attestor CaptureAttestor) ownedKey(workspaceID string, create bool) ([]byte, error) {
	if err := validateWorkspaceID(workspaceID); err != nil {
		return nil, err
	}
	if !filepath.IsAbs(attestor.Root) {
		return nil, errors.New("absolute memory root required")
	}
	directory := filepath.Join(attestor.Root, "workspaces", workspaceID, ".attestation")
	if create {
		if err := memoryMkdirAll(directory, 0700); err != nil {
			return nil, err
		}
	}
	path := filepath.Join(directory, "owner-reviewed.key")
	key, err := memoryReadFile(path, captureAttestationKeyBytes)
	if err == nil {
		if len(key) != captureAttestationKeyBytes {
			return nil, errors.New("invalid owner capture authority")
		}
		return key, nil
	}
	if !create || !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	key = make([]byte, captureAttestationKeyBytes)
	if _, err = rand.Read(key); err != nil {
		return nil, err
	}
	if err = memoryPublish(path, key, true); errors.Is(err, os.ErrExist) {
		return attestor.ownedKey(workspaceID, false)
	}
	return key, err
}

func ownedCaptureMAC(key []byte, c Capture) string {
	c.Attestation = ""
	body, _ := json.Marshal(c)
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte("maestro-owner-reviewed-capture-v1\x00"))
	_, _ = mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}
func (attestor CaptureAttestor) verifyOwned(c Capture) error {
	if c.SchemaVersion != OwnedCaptureSchemaVersion || !c.Sanitized || c.Kind != OwnedCaptureKind || c.ProducerID != OwnedCaptureProducerID || c.SanitizerID != OwnedCaptureSanitizerID || !ownedCaptureIDPattern.MatchString(c.ID) || c.Revision == 0 || c.RecordedAt.IsZero() || len(c.Text) > MaxOwnedCaptureBytes || !utf8.ValidString(c.Text) || strings.TrimSpace(c.Text) == "" {
		return errors.New("invalid owner-reviewed memory capture")
	}
	sum := sha256.Sum256([]byte(c.Text))
	if c.SourceDigest != hex.EncodeToString(sum[:]) {
		return errors.New("owner-reviewed source digest mismatch")
	}
	key, err := attestor.ownedKey(c.WorkspaceID, false)
	if err != nil {
		return err
	}
	if !hmac.Equal([]byte(c.Attestation), []byte(ownedCaptureMAC(key, c))) {
		return errors.New("owner-reviewed capture attestation mismatch")
	}
	return nil
}

// Duplicate keys are rejected before typed decoding, including nested metadata.
func decodeMemoryJSON(body []byte, value any) error {
	check := json.NewDecoder(bytes.NewReader(body))
	if err := uniqueJSONValue(check); err != nil {
		return err
	}
	if err := ensureJSONEOF(check); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	return ensureJSONEOF(decoder)
}
func uniqueJSONValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delimiter {
	case '{':
		seen := map[string]bool{}
		for decoder.More() {
			token, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := token.(string)
			if !ok || seen[key] {
				return errors.New("duplicate JSON metadata key")
			}
			seen[key] = true
			if err := uniqueJSONValue(decoder); err != nil {
				return err
			}
		}
	case '[':
		for decoder.More() {
			if err := uniqueJSONValue(decoder); err != nil {
				return err
			}
		}
	default:
		return errors.New("invalid JSON delimiter")
	}
	_, err = decoder.Token()
	if errors.Is(err, io.EOF) {
		return io.ErrUnexpectedEOF
	}
	return err
}
