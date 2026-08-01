package ownerctx

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

var ErrConfirmationRequired = errors.New("owner confirmation is required by this facet policy")
var ErrRevisionConflict = errors.New("owner facet has changed since this audit; refusing to overwrite newer content")

// RefinementInput is produced by an approved observation or synthesis adapter.
// The engine records it locally and never derives content from raw work itself.
type RefinementInput struct {
	Facet        string
	Evidence     string
	ProposedBody string
	ProducerID   string
	Capability   string
}

// RefinementReceipt is safe to expose in CLI output: it deliberately omits the
// proposed text and evidence body.
type RefinementReceipt struct {
	ID      string `json:"id"`
	Facet   string `json:"facet"`
	State   string `json:"state"`
	Policy  string `json:"policy"`
	AuditID string `json:"audit_id,omitempty"`
}

type proposal struct {
	ID           string    `json:"id"`
	Facet        string    `json:"facet"`
	SourceSHA256 string    `json:"source_sha256"`
	Evidence     string    `json:"evidence"`
	ProposedBody string    `json:"proposed_body"`
	Policy       string    `json:"policy"`
	ProducerID   string    `json:"producer_id"`
	AutoApproved bool      `json:"auto_approved"`
	CreatedAt    time.Time `json:"created_at"`
	State        string    `json:"state"`
	AuditID      string    `json:"audit_id,omitempty"`
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
	if err := writePrivateJSON(filepath.Join(root, "owner", "registry.json"), value); err != nil {
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
	p := proposal{ID: id, Facet: input.Facet, SourceSHA256: digest(string(current)), Evidence: input.Evidence, ProposedBody: input.ProposedBody, Policy: definition.Refinement, ProducerID: input.ProducerID, AutoApproved: autoApproved, CreatedAt: created, State: "proposed"}
	if err := writePrivateJSON(proposalPath(root, id), p); err != nil {
		return RefinementReceipt{}, err
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
	return RefinementReceipt{ID: reversionID, Facet: item.Facet, State: "reverted", Policy: definition.Refinement, AuditID: auditID}, nil
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
	if p.SourceSHA256 == "" || digest(string(before)) != p.SourceSHA256 {
		return RefinementReceipt{}, ErrRevisionConflict
	}
	beforePath := filepath.ToSlash(filepath.Join("owner", "refinement", "versions", p.Facet, p.ID+".before.md"))
	if err := writePrivateFile(filepath.Join(root, filepath.FromSlash(beforePath)), before); err != nil {
		return RefinementReceipt{}, err
	}
	auditID := "audit-" + p.ID
	item := audit{ID: auditID, ProposalID: p.ID, Facet: p.Facet, EvidenceSHA256: digest(p.Evidence), BeforePath: beforePath, BeforeSHA256: digest(string(before)), AfterSHA256: digest(p.ProposedBody), AppliedAt: time.Now().UTC(), State: "prepared"}
	if err := writePrivateJSON(auditPath(root, auditID), item); err != nil {
		return RefinementReceipt{}, err
	}
	if err := atomicPrivateWrite(currentPath, []byte(p.ProposedBody)); err != nil {
		return RefinementReceipt{}, err
	}
	item.State = "applied"
	if err := writePrivateJSON(auditPath(root, auditID), item); err != nil {
		return RefinementReceipt{}, err
	}
	p.State, p.AuditID = "applied", auditID
	if err := writePrivateJSON(proposalPath(root, p.ID), p); err != nil {
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
	file, err := os.ReadFile(filepath.Join(root, "owner", "registry.json"))
	if err != nil {
		return registry{}, err
	}
	var value registry
	if err := json.Unmarshal(file, &value); err != nil || value.SchemaVersion != 2 {
		return registry{}, errors.New("owner context registry is invalid")
	}
	return value, nil
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
	return RefinementReceipt{ID: p.ID, Facet: p.Facet, State: p.State, Policy: p.Policy, AuditID: p.AuditID}
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
	parts := []string{}
	for current := abs; current != "." && current != string(filepath.Separator); current = filepath.Dir(current) {
		parts = append([]string{filepath.Base(current)}, parts...)
	}
	current := string(filepath.Separator)
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
