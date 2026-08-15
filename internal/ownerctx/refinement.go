package ownerctx

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const maximumOwnerRegistryBytes = 64 << 10

var ErrConfirmationRequired = errors.New("owner confirmation is required by this facet policy")
var ErrRevisionConflict = errors.New("owner facet has changed since this audit; refusing to overwrite newer content")

var refinementWriteJSON = writePrivateJSON

// RefinementInput is produced by an approved observation or synthesis adapter.
// The engine records it locally and never derives content from raw work itself.
type RefinementInput struct {
	Facet        string
	Evidence     string
	ProposedBody string
	ProducerID   string
	Capability   string
	// OccurrenceID binds a periodic proposal to one execution occurrence. When
	// present, retries return the same proposal instead of creating another.
	OccurrenceID               string
	YodaReviewRequestSHA256  string
	YodaReviewProposalID     string
	YodaReviewProposalSHA256 string
	YodaReviewSensitivity    string
	YodaReviewReaders        []string
	YodaReviewRefinement     string
	YodaReviewConfirmation   string
	YodaReviewAdapterID      string
	YodaReviewAuthorityID    string
	YodaReviewFencingToken   string
}

// RefinementReceipt is safe to expose in CLI output: it deliberately omits the
// proposed text and evidence body.
type RefinementReceipt struct {
	ID                   string   `json:"id"`
	Facet                string   `json:"facet"`
	State                string   `json:"state"`
	Policy               string   `json:"policy"`
	Sensitivity          string   `json:"sensitivity"`
	Readers              []string `json:"readers"`
	ProposalSHA256       string   `json:"proposal_sha256"`
	OccurrenceID         string   `json:"occurrence_id,omitempty"`
	YodaRequestSHA256  string   `json:"yoda_request_sha256,omitempty"`
	YodaProposalID     string   `json:"yoda_proposal_id,omitempty"`
	YodaProposalSHA256 string   `json:"yoda_proposal_sha256,omitempty"`
	YodaSensitivity    string   `json:"yoda_sensitivity,omitempty"`
	YodaReaders        []string `json:"yoda_readers,omitempty"`
	YodaRefinement     string   `json:"yoda_refinement,omitempty"`
	YodaConfirmation   string   `json:"yoda_confirmation,omitempty"`
	YodaAdapterID      string   `json:"yoda_adapter_id,omitempty"`
	YodaAuthorityID    string   `json:"yoda_authority_id,omitempty"`
	YodaFencingToken   string   `json:"yoda_fencing_token,omitempty"`
	AuditID              string   `json:"audit_id,omitempty"`
}

type proposal struct {
	ID                   string    `json:"id"`
	Facet                string    `json:"facet"`
	Sensitivity          string    `json:"sensitivity"`
	Readers              []string  `json:"readers"`
	SourceSHA256         string    `json:"source_sha256"`
	Evidence             string    `json:"evidence"`
	ProposedBody         string    `json:"proposed_body"`
	Policy               string    `json:"policy"`
	ProducerID           string    `json:"producer_id"`
	AutoApproved         bool      `json:"auto_approved"`
	OccurrenceID         string    `json:"occurrence_id,omitempty"`
	YodaRequestSHA256  string    `json:"yoda_request_sha256,omitempty"`
	YodaProposalID     string    `json:"yoda_proposal_id,omitempty"`
	YodaProposalSHA256 string    `json:"yoda_proposal_sha256,omitempty"`
	YodaSensitivity    string    `json:"yoda_sensitivity,omitempty"`
	YodaReaders        []string  `json:"yoda_readers,omitempty"`
	YodaRefinement     string    `json:"yoda_refinement,omitempty"`
	YodaConfirmation   string    `json:"yoda_confirmation,omitempty"`
	YodaAdapterID      string    `json:"yoda_adapter_id,omitempty"`
	YodaAuthorityID    string    `json:"yoda_authority_id,omitempty"`
	YodaFencingToken   string    `json:"yoda_fencing_token,omitempty"`
	CreatedAt            time.Time `json:"created_at"`
	State                string    `json:"state"`
	AuditID              string    `json:"audit_id,omitempty"`
}

type audit struct {
	ID             string    `json:"id"`
	ProposalID     string    `json:"proposal_id"`
	Facet          string    `json:"facet"`
	EvidenceSHA256 string    `json:"evidence_sha256"`
	BeforePath     string    `json:"before_path"`
	BeforeSHA256   string    `json:"before_sha256"`
	AfterSHA256    string    `json:"after_sha256"`
	AppliedAt      time.Time `json:"applied_at"`
	State          string    `json:"state"`
}

type reversion struct {
	ID             string    `json:"id"`
	AuditID        string    `json:"audit_id"`
	Facet          string    `json:"facet"`
	CurrentSHA256  string    `json:"current_sha256"`
	RestoredSHA256 string    `json:"restored_sha256"`
	CreatedAt      time.Time `json:"created_at"`
	State          string    `json:"state"`
}

// AuthorizeProducer creates a capability for a managed runtime adapter. Only
// its hash remains in the owner registry; the caller must retain the returned
// value in a private credential surface. Direct CLI submission has no token.
func AuthorizeProducer(root, id string) (string, error) {
	if strings.TrimSpace(id) == "" {
		return "", errors.New("producer id is required")
	}
	value, err := readRegistry(root)
	if err != nil {
		return "", err
	}
	capability, err := randomCapability()
	if err != nil {
		return "", err
	}
	if value.Producers == nil {
		value.Producers = map[string]producerRecord{}
	}
	value.Producers[id] = producerRecord{CapabilitySHA256: digest(capability), AuthorizedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	if err := writeOwnerRegistry(root, value); err != nil {
		return "", err
	}
	return capability, nil
}

// SubmitRefinement persists a provenance-bearing proposal. Automatic policies
// run only when the producer presents an authorized capability; otherwise the
// owner may still explicitly confirm the proposal.
func SubmitRefinement(root string, input RefinementInput) (RefinementReceipt, error) {
	definition, err := facetDefinition(root, input.Facet)
	if err != nil {
		return RefinementReceipt{}, err
	}
	if strings.TrimSpace(input.Evidence) == "" || strings.TrimSpace(input.ProposedBody) == "" {
		return RefinementReceipt{}, errors.New("refinement evidence and proposed body are required")
	}
	current, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(definition.Path)))
	if err != nil {
		return RefinementReceipt{}, err
	}
	autoApproved, err := authorizedProducer(root, input.ProducerID, input.Capability)
	if err != nil {
		return RefinementReceipt{}, err
	}
	created := time.Now().UTC()
	id := refinementID(input.Facet, input.Evidence, input.ProposedBody, created)
	if strings.TrimSpace(input.OccurrenceID) != "" {
		id = "proposal-yoda-weekly-" + digest("yoda-self-review-weekly\x00" + input.OccurrenceID)[:32]
		// The occurrence identity, not wall-clock time, is the durable retry
		// identity. This keeps the ownerctx proposal digest stable if a process
		// crashes between proposal commit and receipt finalization.
		created = time.Unix(0, 0).UTC()
	}
	p := proposal{ID: id, Facet: input.Facet, Sensitivity: definition.Sensitivity, Readers: append([]string(nil), definition.Readers...), SourceSHA256: digest(string(current)), Evidence: input.Evidence, ProposedBody: input.ProposedBody, Policy: definition.Refinement, ProducerID: input.ProducerID, AutoApproved: autoApproved, OccurrenceID: input.OccurrenceID, YodaRequestSHA256: input.YodaReviewRequestSHA256, YodaProposalID: input.YodaReviewProposalID, YodaProposalSHA256: input.YodaReviewProposalSHA256, YodaSensitivity: input.YodaReviewSensitivity, YodaReaders: append([]string(nil), input.YodaReviewReaders...), YodaRefinement: input.YodaReviewRefinement, YodaConfirmation: input.YodaReviewConfirmation, YodaAdapterID: input.YodaReviewAdapterID, YodaAuthorityID: input.YodaReviewAuthorityID, YodaFencingToken: input.YodaReviewFencingToken, CreatedAt: created, State: "proposed"}
	if existing, readErr := readProposal(root, id); readErr == nil {
		if !sameProposalBinding(existing, p) {
			return RefinementReceipt{}, errors.New("owner refinement occurrence is already bound to different content")
		}
		return receipt(existing), nil
	} else if !errors.Is(readErr, os.ErrNotExist) {
		return RefinementReceipt{}, readErr
	}
	if err := writePrivateJSONIfAbsent(proposalPath(root, id), p); err != nil {
		if !errors.Is(err, os.ErrExist) {
			return RefinementReceipt{}, err
		}
		existing, readErr := readProposal(root, id)
		if readErr != nil {
			return RefinementReceipt{}, readErr
		}
		if !sameProposalBinding(existing, p) {
			return RefinementReceipt{}, errors.New("owner refinement occurrence is already bound to different content")
		}
		return receipt(existing), nil
	}
	if definition.Refinement == "automatic_with_audit" && p.AutoApproved {
		return apply(root, p, definition, true)
	}
	return receipt(p), nil
}

func ApplyRefinement(root, id string, confirmed bool) (RefinementReceipt, error) {
	p, err := readProposal(root, id)
	if err != nil {
		return RefinementReceipt{}, err
	}
	definition, err := facetDefinition(root, p.Facet)
	if err != nil {
		return RefinementReceipt{}, err
	}
	if p.State == "applied" {
		return receipt(p), nil
	}
	return apply(root, p, definition, confirmed)
}

func RevertRefinement(root, auditID string, confirmed bool) (RefinementReceipt, error) {
	if !confirmed {
		return RefinementReceipt{}, ErrConfirmationRequired
	}
	item, err := readAudit(root, auditID)
	if err != nil {
		return RefinementReceipt{}, err
	}
	before, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(item.BeforePath)))
	if err != nil {
		return RefinementReceipt{}, err
	}
	definition, err := facetDefinition(root, item.Facet)
	if err != nil {
		return RefinementReceipt{}, err
	}
	currentPath := filepath.Join(root, filepath.FromSlash(definition.Path))
	current, err := os.ReadFile(currentPath)
	if err != nil {
		return RefinementReceipt{}, err
	}
	if subtle.ConstantTimeCompare([]byte(digest(string(current))), []byte(item.AfterSHA256)) != 1 {
		return RefinementReceipt{}, ErrRevisionConflict
	}
	reversionID := "revert-" + auditID
	event := reversion{ID: reversionID, AuditID: auditID, Facet: item.Facet, CurrentSHA256: digest(string(current)), RestoredSHA256: digest(string(before)), CreatedAt: time.Now().UTC(), State: "prepared"}
	if err := writePrivateJSON(reversionPath(root, reversionID), event); err != nil {
		return RefinementReceipt{}, err
	}
	if err := atomicPrivateWrite(currentPath, before); err != nil {
		return RefinementReceipt{}, err
	}
	event.State = "applied"
	if err := writePrivateJSON(reversionPath(root, reversionID), event); err != nil {
		return RefinementReceipt{}, err
	}
	return RefinementReceipt{ID: reversionID, Facet: item.Facet, State: "reverted", Policy: definition.Refinement, Sensitivity: definition.Sensitivity, Readers: append([]string(nil), definition.Readers...), AuditID: auditID}, nil
}

func apply(root string, p proposal, definition facetRecord, confirmed bool) (RefinementReceipt, error) {
	if (definition.Refinement != "automatic_with_audit" || !p.AutoApproved) && !confirmed {
		return RefinementReceipt{}, ErrConfirmationRequired
	}
	currentPath := filepath.Join(root, filepath.FromSlash(definition.Path))
	before, err := os.ReadFile(currentPath)
	if err != nil {
		return RefinementReceipt{}, err
	}
	currentSHA := digest(string(before))
	auditID := "audit-" + p.ID
	if p.SourceSHA256 != "" && currentSHA != p.SourceSHA256 && currentSHA == digest(p.ProposedBody) {
		item, readErr := readAudit(root, auditID)
		if readErr != nil || (item.State != "prepared" && item.State != "applied") || item.ID != auditID || item.ProposalID != p.ID || item.Facet != p.Facet || item.BeforeSHA256 != p.SourceSHA256 || item.AfterSHA256 != currentSHA || item.EvidenceSHA256 != digest(p.Evidence) {
			return RefinementReceipt{}, ErrRevisionConflict
		}
		item.State = "applied"
		if err := refinementWriteJSON(auditPath(root, auditID), item); err != nil {
			return RefinementReceipt{}, err
		}
		p.State, p.AuditID = "applied", auditID
		if err := refinementWriteJSON(proposalPath(root, p.ID), p); err != nil {
			return RefinementReceipt{}, err
		}
		return receipt(p), nil
	}
	if p.SourceSHA256 == "" || currentSHA != p.SourceSHA256 {
		return RefinementReceipt{}, ErrRevisionConflict
	}
	beforePath := filepath.ToSlash(filepath.Join("owner", "refinement", "versions", p.Facet, p.ID+".before.md"))
	if err := writePrivateFile(filepath.Join(root, filepath.FromSlash(beforePath)), before); err != nil {
		return RefinementReceipt{}, err
	}
	item := audit{ID: auditID, ProposalID: p.ID, Facet: p.Facet, EvidenceSHA256: digest(p.Evidence), BeforePath: beforePath, BeforeSHA256: digest(string(before)), AfterSHA256: digest(p.ProposedBody), AppliedAt: time.Now().UTC(), State: "prepared"}
	if err := refinementWriteJSON(auditPath(root, auditID), item); err != nil {
		return RefinementReceipt{}, err
	}
	if err := atomicPrivateWrite(currentPath, []byte(p.ProposedBody)); err != nil {
		return RefinementReceipt{}, err
	}
	item.State = "applied"
	if err := refinementWriteJSON(auditPath(root, auditID), item); err != nil {
		return RefinementReceipt{}, err
	}
	p.State, p.AuditID = "applied", auditID
	if err := refinementWriteJSON(proposalPath(root, p.ID), p); err != nil {
		return RefinementReceipt{}, err
	}
	return receipt(p), nil
}

func facetDefinition(root, id string) (facetRecord, error) {
	value, err := readRegistry(root)
	if err != nil {
		return facetRecord{}, err
	}
	definition, ok := value.Facets[id]
	if !ok {
		return facetRecord{}, fmt.Errorf("unknown owner facet %q", id)
	}
	return definition, nil
}

func authorizedProducer(root, id, capability string) (bool, error) {
	if strings.TrimSpace(id) == "" || strings.TrimSpace(capability) == "" {
		return false, nil
	}
	value, err := readRegistry(root)
	if err != nil {
		return false, err
	}
	record, ok := value.Producers[id]
	if !ok {
		return false, nil
	}
	return subtle.ConstantTimeCompare([]byte(record.CapabilitySHA256), []byte(digest(capability))) == 1, nil
}

func readRegistry(root string) (registry, error) {
	path := filepath.Join(root, "owner", "registry.json")
	info, err := os.Lstat(path)
	if err != nil {
		return registry{}, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return registry{}, errors.New("owner context registry must be a regular non-symlink file")
	}
	if info.Size() <= 0 || info.Size() > maximumOwnerRegistryBytes || (runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0) {
		return registry{}, errors.New("owner context registry must be a bounded owner-only file")
	}
	file, err := os.Open(path)
	if err != nil {
		return registry{}, err
	}
	defer file.Close()
	body, err := io.ReadAll(io.LimitReader(file, maximumOwnerRegistryBytes+1))
	if err != nil {
		return registry{}, err
	}
	if int64(len(body)) > maximumOwnerRegistryBytes {
		return registry{}, errors.New("owner context registry exceeds the bounded JSON limit")
	}
	var value registry
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil || (value.SchemaVersion != 2 && value.SchemaVersion != 3) {
		return registry{}, errors.New("owner context registry is invalid")
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return registry{}, errors.New("owner context registry contains multiple JSON values")
	}
	if value.SchemaVersion == 2 && value.OnboardingTrack == "" {
		value.OnboardingTrack = OnboardingTrackComplete
	}
	return value, nil
}

func writeOwnerRegistry(root string, value registry) error {
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	if len(body) > maximumOwnerRegistryBytes {
		return errors.New("owner context registry exceeds the bounded JSON limit")
	}
	return atomicPrivateWrite(filepath.Join(root, "owner", "registry.json"), body)
}

func readProposal(root, id string) (proposal, error) {
	var p proposal
	if err := readPrivateJSON(proposalPath(root, id), &p); err != nil {
		return proposal{}, err
	}
	return p, nil
}

func readAudit(root, id string) (audit, error) {
	var item audit
	if err := readPrivateJSON(auditPath(root, id), &item); err != nil {
		return audit{}, err
	}
	return item, nil
}

func receipt(p proposal) RefinementReceipt {
	return RefinementReceipt{ID: p.ID, Facet: p.Facet, State: p.State, Policy: p.Policy, Sensitivity: p.Sensitivity, Readers: append([]string(nil), p.Readers...), ProposalSHA256: digestJSON(p), OccurrenceID: p.OccurrenceID, YodaRequestSHA256: p.YodaRequestSHA256, YodaProposalID: p.YodaProposalID, YodaProposalSHA256: p.YodaProposalSHA256, YodaSensitivity: p.YodaSensitivity, YodaReaders: append([]string(nil), p.YodaReaders...), YodaRefinement: p.YodaRefinement, YodaConfirmation: p.YodaConfirmation, YodaAdapterID: p.YodaAdapterID, YodaAuthorityID: p.YodaAuthorityID, YodaFencingToken: p.YodaFencingToken, AuditID: p.AuditID}
}

func sameProposalBinding(left, right proposal) bool {
	return left.Facet == right.Facet && left.Sensitivity == right.Sensitivity && sameStrings(left.Readers, right.Readers) && left.SourceSHA256 == right.SourceSHA256 && left.Evidence == right.Evidence && left.ProposedBody == right.ProposedBody && left.Policy == right.Policy && left.OccurrenceID == right.OccurrenceID && left.YodaRequestSHA256 == right.YodaRequestSHA256 && left.YodaProposalID == right.YodaProposalID && left.YodaProposalSHA256 == right.YodaProposalSHA256 && left.YodaSensitivity == right.YodaSensitivity && sameStrings(left.YodaReaders, right.YodaReaders) && left.YodaRefinement == right.YodaRefinement && left.YodaConfirmation == right.YodaConfirmation && left.YodaAdapterID == right.YodaAdapterID && left.YodaAuthorityID == right.YodaAuthorityID && left.YodaFencingToken == right.YodaFencingToken
}

// FindOccurrenceRefinement discovers the deterministic Yoda artifact without
// invoking a model. It is the recovery seam after ownerctx commit.
func FindOccurrenceRefinement(root, occurrenceID string) (RefinementReceipt, bool, error) {
	if strings.TrimSpace(occurrenceID) == "" {
		return RefinementReceipt{}, false, errors.New("occurrence id is required")
	}
	id := "proposal-yoda-weekly-" + digest("yoda-self-review-weekly\x00" + occurrenceID)[:32]
	p, err := readProposal(root, id)
	if errors.Is(err, os.ErrNotExist) {
		return RefinementReceipt{}, false, nil
	}
	if err != nil {
		return RefinementReceipt{}, false, err
	}
	if p.OccurrenceID != occurrenceID {
		return RefinementReceipt{}, false, errors.New("ownerctx occurrence artifact binding is invalid")
	}
	return receipt(p), true, nil
}

func digestJSON(value any) string {
	body, _ := json.Marshal(value)
	return digest(string(body))
}

func proposalPath(root, id string) string {
	return filepath.Join(root, "owner", "refinement", "proposals", id+".json")
}

func auditPath(root, id string) string {
	return filepath.Join(root, "owner", "refinement", "audits", id+".json")
}

func reversionPath(root, id string) string {
	return filepath.Join(root, "owner", "refinement", "reversions", id+".json")
}

func refinementID(facet, evidence, body string, created time.Time) string {
	return created.Format("20060102T150405.000000000Z") + "-" + digest(facet + "\x00" + evidence + "\x00" + body)[:12]
}

func digest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func randomCapability() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

func writePrivateJSON(path string, value any) error {
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return atomicPrivateWrite(path, append(body, '\n'))
}

func writePrivateJSONIfAbsent(path string, value any) error {
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	if err := validatePrivateParents(directory); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := file.Write(append(body, '\n')); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return syncPrivateDirectory(directory)
}

func readPrivateJSON(path string, target any) error {
	body, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(body, target)
}

func writePrivateFile(path string, body []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, body, 0o600)
}

func atomicPrivateWrite(path string, body []byte) error {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	if err := validatePrivateParents(directory); err != nil {
		return err
	}
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return fmt.Errorf("private target is not a regular file: %s", path)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	temporary, err := os.CreateTemp(directory, ".private-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(body); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return err
	}
	return syncPrivateDirectory(directory)
}

func validatePrivateParents(directory string) error {
	abs, err := filepath.Abs(directory)
	if err != nil {
		return err
	}
	// Collect the components by walking up to the filesystem root, stopping at
	// the fixed point of filepath.Dir rather than at a hardcoded separator. On
	// Windows the root is a volume whose parent is itself, so a separator-only
	// guard is never reached and the walk does not terminate. Starting the
	// descent from that same fixed point keeps the traversal correct on both
	// platforms.
	parts := []string{}
	current := abs
	for {
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		parts = append([]string{filepath.Base(current)}, parts...)
		current = parent
	}
	for _, part := range parts {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			// macOS uses these compatibility links for the system temporary
			// tree. They are outside the owner-private boundary; links below
			// them are still rejected by this walk.
			if runtime.GOOS == "darwin" && (current == "/var" || current == "/tmp") {
				continue
			}
			return fmt.Errorf("private parent is a symlink: %s", current)
		}
		if !info.IsDir() {
			return fmt.Errorf("private parent is not a directory: %s", current)
		}
	}
	return nil
}

func syncPrivateDirectory(directory string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	directoryFile, err := os.Open(directory)
	if err != nil {
		return err
	}
	defer directoryFile.Close()
	return directoryFile.Sync()
}
