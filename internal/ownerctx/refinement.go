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
	OccurrenceID             string
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
	ID                 string   `json:"id"`
	Facet              string   `json:"facet"`
	State              string   `json:"state"`
	Policy             string   `json:"policy"`
	Sensitivity        string   `json:"sensitivity"`
	Readers            []string `json:"readers"`
	ProposalSHA256     string   `json:"proposal_sha256"`
	OccurrenceID       string   `json:"occurrence_id,omitempty"`
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
	AuditID            string   `json:"audit_id,omitempty"`
}

type proposal struct {
	ID                 string    `json:"id"`
	Facet              string    `json:"facet"`
	Sensitivity        string    `json:"sensitivity"`
	Readers            []string  `json:"readers"`
	SourceSHA256       string    `json:"source_sha256"`
	Evidence           string    `json:"evidence"`
	ProposedBody       string    `json:"proposed_body"`
	Policy             string    `json:"policy"`
	ProducerID         string    `json:"producer_id"`
	AutoApproved       bool      `json:"auto_approved"`
	OccurrenceID       string    `json:"occurrence_id,omitempty"`
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
	CreatedAt          time.Time `json:"created_at"`
	State              string    `json:"state"`
	AuditID            string    `json:"audit_id,omitempty"`
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
	var result string
	err := withOwnerTransition(root, func() error { var err error; result, err = authorizeProducerLocked(root, id); return err })
	return result, err
}

func authorizeProducerLocked(root, id string) (string, error) {
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
	var result RefinementReceipt
	err := withOwnerTransition(root, func() error { var err error; result, err = submitRefinementLocked(root, input); return err })
	return result, err
}

func submitRefinementLocked(root string, input RefinementInput) (RefinementReceipt, error) {
	definition, err := facetDefinition(root, input.Facet)
	if err != nil {
		return RefinementReceipt{}, err
	}
	if strings.TrimSpace(input.Evidence) == "" || strings.TrimSpace(input.ProposedBody) == "" {
		return RefinementReceipt{}, errors.New("refinement evidence and proposed body are required")
	}
	if strings.ContainsRune(input.ProposedBody, 0) || strings.ContainsRune(input.Evidence, 0) {
		return RefinementReceipt{}, errors.New("owner refinement text cannot contain NUL")
	}
	if len(input.Evidence) > maximumOwnerEvidenceBytes || len(input.ProposedBody) > maximumOwnerFacetBytes {
		return RefinementReceipt{}, errors.New("owner refinement exceeds bounded text size")
	}
	current, err := readOwnerFacet(filepath.Join(root, filepath.FromSlash(definition.Path)))
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
	var result RefinementReceipt
	err := withOwnerTransition(root, func() error { var err error; result, err = applyRefinementLocked(root, id, confirmed); return err })
	return result, err
}

func applyRefinementLocked(root, id string, confirmed bool) (RefinementReceipt, error) {
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
	var result RefinementReceipt
	err := withOwnerTransition(root, func() error {
		var err error
		result, err = revertRefinementLocked(root, auditID, confirmed)
		return err
	})
	return result, err
}

func revertRefinementLocked(root, auditID string, confirmed bool) (RefinementReceipt, error) {
	if !confirmed {
		return RefinementReceipt{}, ErrConfirmationRequired
	}
	item, err := readAudit(root, auditID)
	if err != nil {
		return RefinementReceipt{}, err
	}
	expectedBefore := filepath.ToSlash(filepath.Join("owner", "refinement", "versions", item.Facet, item.ProposalID+".before.md"))
	if item.BeforePath != expectedBefore {
		return RefinementReceipt{}, errors.New("owner audit version path is invalid")
	}
	before, err := readOwnerFacet(filepath.Join(root, filepath.FromSlash(item.BeforePath)))
	if err != nil {
		return RefinementReceipt{}, err
	}
	if digest(string(before)) != item.BeforeSHA256 {
		return RefinementReceipt{}, ErrRevisionConflict
	}
	definition, err := facetDefinition(root, item.Facet)
	if err != nil {
		return RefinementReceipt{}, err
	}
	currentPath := filepath.Join(root, filepath.FromSlash(definition.Path))
	current, err := readOwnerFacet(currentPath)
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
	before, err := readOwnerFacet(currentPath)
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
	body, err := readOwnerFile(filepath.Join(root, "owner", "registry.json"), maximumOwnerRegistryBytes)
	if err != nil {
		return registry{}, err
	}
	var value registry
	if err := decodePrivateJSON(body, &value); err != nil || (value.SchemaVersion != 2 && value.SchemaVersion != 3) {
		return registry{}, errors.New("owner context registry is invalid")
	}
	for id, record := range value.Facets {
		template, ok := facets[id]
		if !ok || record.Path != template.Record.Path || record.Sensitivity != template.Record.Sensitivity || record.Refinement != template.Record.Refinement || !sameStrings(record.Readers, template.Record.Readers) {
			return registry{}, errors.New("owner context registry facet binding is invalid")
		}
	}
	if value.OperatingState != statePath {
		return registry{}, errors.New("owner context registry operating path is invalid")
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
	if !validOwnerArtifactID(id) {
		return proposal{}, errors.New("owner proposal identity is invalid")
	}
	var p proposal
	if err := readPrivateJSON(proposalPath(root, id), &p); err != nil {
		return proposal{}, err
	}
	if p.ID != id || !validOnboardingDigest(p.SourceSHA256) || strings.TrimSpace(p.Evidence) == "" || strings.TrimSpace(p.ProposedBody) == "" || strings.ContainsRune(p.ProposedBody, 0) || strings.ContainsRune(p.Evidence, 0) || len(p.Evidence) > maximumOwnerEvidenceBytes || len(p.ProposedBody) > maximumOwnerFacetBytes || (p.State != "proposed" && p.State != "applied") {
		return proposal{}, errors.New("owner proposal binding is invalid")
	}
	return p, nil
}

func readAudit(root, id string) (audit, error) {
	if !validOwnerArtifactID(id) {
		return audit{}, errors.New("owner audit identity is invalid")
	}
	var item audit
	if err := readPrivateJSON(auditPath(root, id), &item); err != nil {
		return audit{}, err
	}
	if item.ID != id || !validOwnerArtifactID(item.ProposalID) || facets[item.Facet].Record.Path == "" {
		return audit{}, errors.New("owner audit binding is invalid")
	}
	return item, nil
}

func validOwnerArtifactID(id string) bool {
	if id == "" || len(id) > 160 {
		return false
	}
	for _, r := range id {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '.') {
			return false
		}
	}
	return id != "." && id != ".."
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
	if len(body)+1 > maximumOwnerRegistryBytes {
		return errors.New("owner proposal exceeds bounded JSON size")
	}
	return publishOwnerFile(path, append(body, '\n'), true)
}

func readPrivateJSON(path string, target any) error {
	body, err := readOwnerFile(path, maximumOwnerRegistryBytes)
	if err != nil {
		return err
	}
	return decodePrivateJSON(body, target)
}

func decodePrivateJSON(body []byte, target any) error {
	if err := checkJSONKeys(json.NewDecoder(bytes.NewReader(body))); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("owner JSON contains trailing values")
	}
	return nil
}

func checkJSONKeys(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	if delim != '{' && delim != '[' {
		return errors.New("invalid owner JSON")
	}
	keys := map[string]bool{}
	for decoder.More() {
		if delim == '{' {
			token, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := token.(string)
			if !ok || keys[key] {
				return errors.New("owner JSON contains duplicate keys")
			}
			keys[key] = true
		}
		if err := checkJSONKeys(decoder); err != nil {
			return err
		}
	}
	_, err = decoder.Token()
	return err
}

func writePrivateFile(path string, body []byte) error   { return atomicPrivateWrite(path, body) }
func atomicPrivateWrite(path string, body []byte) error { return publishOwnerFile(path, body, false) }

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
