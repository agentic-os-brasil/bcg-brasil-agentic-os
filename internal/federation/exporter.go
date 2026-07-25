package federation

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	// PilotContractVersion is the single enrollment contract that authorizes
	// automatic export of the closed Batch vocabulary during the pilot.
	PilotContractVersion = "maestro-pilot-v1"
	// MaximumDeliveryAttempts prevents an unattended device from retrying a
	// failing endpoint without bound. Exhausted batches stay local for support
	// inspection and are never silently rewritten as successful.
	MaximumDeliveryAttempts = 3
)

var (
	ErrNotEnrolled   = errors.New("federation export is not enrolled")
	ErrExportRevoked = errors.New("federation export is revoked")
)

// Bridge is the only outbound seam available to a pilot installation. Its
// implementation receives an already validated Batch and never requires or
// exposes GitHub App credentials on the device.
type Bridge interface {
	Submit(context.Context, Batch) error
}

// Enrollment records the pilot contract locally. It has no participant,
// workspace or GitHub identity and contains no transport credential.
type Enrollment struct {
	SchemaVersion   int       `json:"schema_version"`
	InstallationID  string    `json:"installation_id"`
	BridgeEndpoint  string    `json:"bridge_endpoint"`
	ContractVersion string    `json:"contract_version"`
	AcceptedAt      time.Time `json:"accepted_at"`
	AutomaticExport bool      `json:"automatic_export"`
	RevokedAt       time.Time `json:"revoked_at,omitempty"`
}

// QueuedBatch is a durable structural batch waiting for the bridge. No remote
// response body or error string is retained, because those can be
// content-bearing and do not help retry scheduling.
type QueuedBatch struct {
	SchemaVersion int       `json:"schema_version"`
	Batch         Batch     `json:"batch"`
	QueuedAt      time.Time `json:"queued_at"`
	Attempts      int       `json:"attempts"`
	NextAttemptAt time.Time `json:"next_attempt_at"`
	Exhausted     bool      `json:"exhausted"`
}

type QueuedPortableSkill struct {
	SchemaVersion int                  `json:"schema_version"`
	Package       PortableSkillPackage `json:"package"`
	QueuedAt      time.Time            `json:"queued_at"`
	Attempts      int                  `json:"attempts"`
	NextAttemptAt time.Time            `json:"next_attempt_at"`
	Exhausted     bool                 `json:"exhausted"`
}

type FlushReport struct {
	Delivered int
	Retained  int
	Exhausted int
}

// ExportStore owns only user-local export state below the product data root.
// Callers must not point it at a workspace.
type ExportStore struct {
	Root string
}

// NewInstallationID creates the opaque identifier used at the bridge ingress.
// It is not derived from a workspace, person, machine or customer.
func NewInstallationID() (string, error) {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}

func (store ExportStore) Enroll(enrollment Enrollment) error {
	if err := store.validateRoot(); err != nil {
		return err
	}
	enrollment.SchemaVersion = SchemaVersion
	if err := enrollment.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(store.outboxRoot(), 0o700); err != nil {
		return err
	}
	if err := writeNewJSON(store.enrollmentPath(), enrollment); err != nil {
		if errors.Is(err, os.ErrExist) {
			return errors.New("federation export is already enrolled")
		}
		return err
	}
	return nil
}

func (store ExportStore) Enrollment() (Enrollment, error) {
	if err := store.validateRoot(); err != nil {
		return Enrollment{}, err
	}
	enrollment, err := store.readEnrollment()
	if errors.Is(err, os.ErrNotExist) {
		return Enrollment{}, ErrNotEnrolled
	}
	return enrollment, err
}

func (store ExportStore) Revoke(revokedAt time.Time) error {
	if revokedAt.IsZero() {
		return errors.New("federation revocation time is required")
	}
	enrollment, err := store.Enrollment()
	if err != nil {
		return err
	}
	if !enrollment.RevokedAt.IsZero() {
		return nil
	}
	enrollment.RevokedAt = revokedAt.UTC()
	if err := enrollment.Validate(); err != nil {
		return err
	}
	return writeAtomicJSON(store.enrollmentPath(), enrollment)
}

// Enqueue validates before persistence and is idempotent for an identical
// typed batch. The deterministic filename is derived solely from the already
// approved wire payload.
func (store ExportStore) Enqueue(batch Batch, queuedAt time.Time) error {
	if queuedAt.IsZero() {
		return errors.New("federation queue time is required")
	}
	if err := batch.Validate(); err != nil {
		return err
	}
	enrollment, err := store.Enrollment()
	if err != nil {
		return err
	}
	if !enrollment.RevokedAt.IsZero() {
		return ErrExportRevoked
	}
	if batch.InstallationID != enrollment.InstallationID {
		return errors.New("federation batch does not belong to this installation")
	}
	encoded, err := json.Marshal(batch)
	if err != nil {
		return err
	}
	digest := sha256.Sum256(encoded)
	item := QueuedBatch{
		SchemaVersion: SchemaVersion,
		Batch:         batch,
		QueuedAt:      queuedAt.UTC(),
		NextAttemptAt: queuedAt.UTC(),
	}
	path := filepath.Join(store.outboxRoot(), hex.EncodeToString(digest[:])+".json")
	if err := writeNewJSON(path, item); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	return nil
}

func (store ExportStore) Pending() ([]QueuedBatch, error) {
	if err := store.validateRoot(); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(store.outboxRoot())
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	pending := make([]QueuedBatch, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			return nil, fmt.Errorf("invalid federation outbox entry %q", entry.Name())
		}
		var item QueuedBatch
		if err := readStrictJSON(filepath.Join(store.outboxRoot(), entry.Name()), &item); err != nil {
			return nil, err
		}
		if err := item.Validate(); err != nil {
			return nil, err
		}
		pending = append(pending, item)
	}
	sort.Slice(pending, func(left, right int) bool {
		if pending[left].NextAttemptAt.Equal(pending[right].NextAttemptAt) {
			return pending[left].QueuedAt.Before(pending[right].QueuedAt)
		}
		return pending[left].NextAttemptAt.Before(pending[right].NextAttemptAt)
	})
	return pending, nil
}

// Flush delivers due batches without prompting a pilot user. Failures remain
// local and receive a bounded retry schedule; arbitrary error details are
// intentionally neither returned nor persisted in the local state.
func (store ExportStore) Flush(ctx context.Context, bridge Bridge, now time.Time) (FlushReport, error) {
	if bridge == nil {
		return FlushReport{}, errors.New("federation bridge is required")
	}
	if now.IsZero() {
		return FlushReport{}, errors.New("federation flush time is required")
	}
	enrollment, err := store.Enrollment()
	if err != nil {
		return FlushReport{}, err
	}
	if !enrollment.RevokedAt.IsZero() {
		return FlushReport{}, ErrExportRevoked
	}
	pending, err := store.Pending()
	if err != nil {
		return FlushReport{}, err
	}
	report := FlushReport{}
	for _, item := range pending {
		if item.Exhausted {
			report.Exhausted++
			continue
		}
		if item.NextAttemptAt.After(now) {
			report.Retained++
			continue
		}
		path, err := store.queuedPath(item.Batch)
		if err != nil {
			return FlushReport{}, err
		}
		if err := bridge.Submit(ctx, item.Batch); err == nil {
			if err := os.Remove(path); err != nil {
				return FlushReport{}, err
			}
			report.Delivered++
			continue
		}
		item.Attempts++
		if item.Attempts >= MaximumDeliveryAttempts {
			item.Attempts = MaximumDeliveryAttempts
			item.Exhausted = true
			item.NextAttemptAt = time.Time{}
		} else {
			item.NextAttemptAt = now.UTC().Add(retryDelay(item.Attempts))
		}
		if err := item.Validate(); err != nil {
			return FlushReport{}, err
		}
		if err := writeAtomicJSON(path, item); err != nil {
			return FlushReport{}, err
		}
		report.Retained++
	}
	return report, nil
}

// FlushHTTP is the unattended pilot entry point. It constructs the narrow
// HTTPS bridge from the one-time enrollment state; callers cannot inject a
// different destination for a normal automatic run.
func (store ExportStore) FlushHTTP(ctx context.Context, client *http.Client, now time.Time) (FlushReport, error) {
	enrollment, err := store.Enrollment()
	if err != nil {
		return FlushReport{}, err
	}
	if !enrollment.RevokedAt.IsZero() {
		return FlushReport{}, ErrExportRevoked
	}
	bridge, err := NewHTTPBridge(enrollment.BridgeEndpoint, client)
	if err != nil {
		return FlushReport{}, err
	}
	return store.Flush(ctx, bridge, now)
}

// EnqueuePortable keeps content-bearing packages separate from typed batches.
// The package has already passed the born-portable collector's root, manifest
// and hash checks; it still cannot enter a workspace batch.
func (store ExportStore) EnqueuePortable(packageValue PortableSkillPackage, queuedAt time.Time) error {
	if queuedAt.IsZero() {
		return errors.New("portable skill queue time is required")
	}
	if err := packageValue.Validate(); err != nil {
		return err
	}
	enrollment, err := store.Enrollment()
	if err != nil {
		return err
	}
	if !enrollment.RevokedAt.IsZero() {
		return ErrExportRevoked
	}
	if err := os.MkdirAll(store.portableOutboxRoot(), 0o700); err != nil {
		return err
	}
	path, err := store.portableQueuedPath(packageValue)
	if err != nil {
		return err
	}
	item := QueuedPortableSkill{SchemaVersion: SchemaVersion, Package: packageValue, QueuedAt: queuedAt.UTC(), NextAttemptAt: queuedAt.UTC()}
	if err := writeNewJSON(path, item); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	return nil
}

func (store ExportStore) PendingPortable() ([]QueuedPortableSkill, error) {
	if err := store.validateRoot(); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(store.portableOutboxRoot())
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	pending := make([]QueuedPortableSkill, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			return nil, fmt.Errorf("invalid portable skill outbox entry %q", entry.Name())
		}
		var item QueuedPortableSkill
		if err := readStrictJSON(filepath.Join(store.portableOutboxRoot(), entry.Name()), &item); err != nil {
			return nil, err
		}
		if err := item.Validate(); err != nil {
			return nil, err
		}
		pending = append(pending, item)
	}
	sort.Slice(pending, func(left, right int) bool { return pending[left].NextAttemptAt.Before(pending[right].NextAttemptAt) })
	return pending, nil
}

func (store ExportStore) FlushPortable(ctx context.Context, bridge PortableSkillBridge, now time.Time) (FlushReport, error) {
	if bridge == nil || now.IsZero() {
		return FlushReport{}, errors.New("portable skill bridge and flush time are required")
	}
	enrollment, err := store.Enrollment()
	if err != nil {
		return FlushReport{}, err
	}
	if !enrollment.RevokedAt.IsZero() {
		return FlushReport{}, ErrExportRevoked
	}
	pending, err := store.PendingPortable()
	if err != nil {
		return FlushReport{}, err
	}
	report := FlushReport{}
	for _, item := range pending {
		if item.Exhausted {
			report.Exhausted++
			continue
		}
		if item.NextAttemptAt.After(now) {
			report.Retained++
			continue
		}
		path, err := store.portableQueuedPath(item.Package)
		if err != nil {
			return FlushReport{}, err
		}
		if err := bridge.SubmitPortable(ctx, enrollment.InstallationID, item.Package); err == nil {
			if err := os.Remove(path); err != nil {
				return FlushReport{}, err
			}
			report.Delivered++
			continue
		}
		item.Attempts++
		if item.Attempts >= MaximumDeliveryAttempts {
			item.Attempts = MaximumDeliveryAttempts
			item.Exhausted = true
			item.NextAttemptAt = time.Time{}
		} else {
			item.NextAttemptAt = now.UTC().Add(retryDelay(item.Attempts))
		}
		if err := item.Validate(); err != nil {
			return FlushReport{}, err
		}
		if err := writeAtomicJSON(path, item); err != nil {
			return FlushReport{}, err
		}
		report.Retained++
	}
	return report, nil
}

// LocalState exists for support and tests. It deliberately exposes only the
// enrollment record, never pending payloads or remote errors.
func (store ExportStore) LocalState() (string, error) {
	contents, err := os.ReadFile(store.enrollmentPath())
	if errors.Is(err, os.ErrNotExist) {
		return "", ErrNotEnrolled
	}
	return string(contents), err
}

func (enrollment Enrollment) Validate() error {
	if enrollment.SchemaVersion != SchemaVersion || !installationIDPattern.MatchString(enrollment.InstallationID) || enrollment.ContractVersion != PilotContractVersion || enrollment.AcceptedAt.IsZero() || !enrollment.AutomaticExport {
		return errors.New("invalid federation enrollment")
	}
	if err := validateBridgeEndpoint(enrollment.BridgeEndpoint); err != nil {
		return err
	}
	if !enrollment.RevokedAt.IsZero() && enrollment.RevokedAt.Before(enrollment.AcceptedAt) {
		return errors.New("federation revocation cannot precede enrollment")
	}
	return nil
}

func (item QueuedBatch) Validate() error {
	if item.SchemaVersion != SchemaVersion || item.QueuedAt.IsZero() || item.Attempts < 0 || item.Attempts > MaximumDeliveryAttempts {
		return errors.New("invalid federation queued batch")
	}
	if err := item.Batch.Validate(); err != nil {
		return err
	}
	if item.Exhausted {
		if item.Attempts != MaximumDeliveryAttempts || !item.NextAttemptAt.IsZero() {
			return errors.New("invalid exhausted federation queued batch")
		}
		return nil
	}
	if item.NextAttemptAt.IsZero() {
		return errors.New("federation queued batch requires next attempt")
	}
	return nil
}

func (item QueuedPortableSkill) Validate() error {
	if item.SchemaVersion != SchemaVersion || item.QueuedAt.IsZero() || item.Attempts < 0 || item.Attempts > MaximumDeliveryAttempts {
		return errors.New("invalid queued portable skill")
	}
	if err := item.Package.Validate(); err != nil {
		return err
	}
	if item.Exhausted {
		if item.Attempts != MaximumDeliveryAttempts || !item.NextAttemptAt.IsZero() {
			return errors.New("invalid exhausted queued portable skill")
		}
		return nil
	}
	if item.NextAttemptAt.IsZero() {
		return errors.New("queued portable skill requires next attempt")
	}
	return nil
}

func (store ExportStore) readEnrollment() (Enrollment, error) {
	var enrollment Enrollment
	if err := readStrictJSON(store.enrollmentPath(), &enrollment); err != nil {
		return Enrollment{}, err
	}
	if err := enrollment.Validate(); err != nil {
		return Enrollment{}, err
	}
	return enrollment, nil
}

func (store ExportStore) queuedPath(batch Batch) (string, error) {
	encoded, err := json.Marshal(batch)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return filepath.Join(store.outboxRoot(), hex.EncodeToString(digest[:])+".json"), nil
}

func (store ExportStore) portableQueuedPath(packageValue PortableSkillPackage) (string, error) {
	encoded, err := json.Marshal(packageValue)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return filepath.Join(store.portableOutboxRoot(), hex.EncodeToString(digest[:])+".json"), nil
}

func (store ExportStore) validateRoot() error {
	if strings.TrimSpace(store.Root) == "" {
		return errors.New("federation store root is required")
	}
	return nil
}

func (store ExportStore) enrollmentPath() string { return filepath.Join(store.Root, "enrollment.json") }
func (store ExportStore) outboxRoot() string     { return filepath.Join(store.Root, "outbox") }
func (store ExportStore) portableOutboxRoot() string {
	return filepath.Join(store.Root, "portable-outbox")
}

func retryDelay(attempt int) time.Duration {
	switch attempt {
	case 1:
		return 15 * time.Minute
	case 2:
		return time.Hour
	default:
		return 0
	}
}

func validateBridgeEndpoint(value string) error {
	endpoint, err := parseHTTPSURL(value)
	if err != nil || endpoint.RawQuery != "" || endpoint.Fragment != "" || endpoint.User != nil {
		return errors.New("federation bridge endpoint must be a credential-free HTTPS URL")
	}
	return nil
}

// ValidateExportStateSchemaFile keeps the local enrollment contract visible to
// release tooling without adding a JSON Schema implementation to the runtime.
func ValidateExportStateSchemaFile(path string) error {
	var schema map[string]any
	if err := readStrictJSON(path, &schema); err != nil {
		return err
	}
	if schema["$schema"] != "https://json-schema.org/draft/2020-12/schema" {
		return errors.New("federation export state schema must use JSON Schema draft 2020-12")
	}
	if schema["$id"] != "urn:bcg-brasil-agentic-os:schema:federation-export-state:v1" {
		return errors.New("federation export state schema has an unexpected identifier")
	}
	return nil
}

func readStrictJSON(path string, target any) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("federation state contains multiple JSON values")
		}
		return err
	}
	return nil
}

func writeNewJSON(path string, value any) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(file)
	if err := encoder.Encode(value); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

func writeAtomicJSON(path string, value any) error {
	temporary, err := os.CreateTemp(filepath.Dir(path), ".federation-*.json")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := json.NewEncoder(temporary).Encode(value); err != nil {
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
	return os.Rename(temporaryName, path)
}
