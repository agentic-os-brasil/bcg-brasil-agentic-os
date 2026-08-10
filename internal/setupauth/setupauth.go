// Package setupauth persists the one-and-done local setup authority described
// by COFS. It stores only opaque identity and scope digests; it never stores
// source URLs, credentials, client content or raw OS identity values.
package setupauth

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	SchemaVersion              = 1
	PolicyVersion              = "cofs-v1"
	DefaultValidity            = 365 * 24 * time.Hour
	StateAuthorizationRequired = "authorization_required"
	StateActive                = "active"
	StateScopeChanged          = "scope_changed"
	StateIdentityChanged       = "identity_changed"
	StateExpired               = "expired"
)

var (
	ErrAuthorizationRequired = errors.New("one setup authorization is required")
	workspaceIDPattern       = regexp.MustCompile(`^[a-f0-9]{32}$`)
	digestPattern            = regexp.MustCompile(`^[a-f0-9]{64}$`)
	allowedActions           = []string{
		"diagnostic_probe",
		"local_reversible_repair",
		"rollback_recovery",
		"runtime_adapter_install",
		"runtime_projection_repair",
		"selected_source_rationale_projection",
		"signed_managed_component_activation",
		"workspace_initialize",
	}
)

type Identity struct {
	PrincipalRef string
	DeviceRef    string
}

type Request struct {
	WorkspaceID       string
	WorkspacePath     string
	SourceFingerprint string
}

type Status struct {
	SchemaVersion  int       `json:"schema_version"`
	State          string    `json:"state"`
	PolicyVersion  string    `json:"policy_version"`
	WorkspaceID    string    `json:"workspace_id"`
	GrantDigest    string    `json:"grant_digest,omitempty"`
	AllowedActions []string  `json:"allowed_actions,omitempty"`
	IssuedAt       time.Time `json:"issued_at,omitempty"`
	ExpiresAt      time.Time `json:"expires_at,omitempty"`
}

type grant struct {
	SchemaVersion       int       `json:"schema_version"`
	PolicyVersion       string    `json:"policy_version"`
	WorkspaceID         string    `json:"workspace_id"`
	WorkspacePathDigest string    `json:"workspace_path_digest"`
	PrincipalRef        string    `json:"principal_ref"`
	DeviceRef           string    `json:"device_ref"`
	SourceFingerprint   string    `json:"source_fingerprint"`
	AllowedActions      []string  `json:"allowed_actions"`
	IssuedAt            time.Time `json:"issued_at"`
	ExpiresAt           time.Time `json:"expires_at"`
	GrantDigest         string    `json:"grant_digest"`
}

type Store struct {
	Root  string
	Clock func() time.Time
}

func DeriveIdentity(principal, device string) Identity {
	return Identity{
		PrincipalRef: digest("maestro-setup-principal", principal),
		DeviceRef:    digest("maestro-setup-device", device),
	}
}

func (store Store) Authorize(request Request, identity Identity, confirmed bool) (Status, error) {
	if err := validateRequest(request, identity); err != nil {
		return Status{}, err
	}
	current, err := store.Status(request, identity)
	if err != nil {
		return Status{}, err
	}
	if current.State == StateActive {
		return current, nil
	}
	if !confirmed {
		return current, ErrAuthorizationRequired
	}
	now := store.now()
	record := grant{
		SchemaVersion:       SchemaVersion,
		PolicyVersion:       PolicyVersion,
		WorkspaceID:         request.WorkspaceID,
		WorkspacePathDigest: digest("maestro-setup-workspace-path", canonicalPath(request.WorkspacePath)),
		PrincipalRef:        identity.PrincipalRef,
		DeviceRef:           identity.DeviceRef,
		SourceFingerprint:   normalizedSourceFingerprint(request.SourceFingerprint),
		AllowedActions:      append([]string(nil), allowedActions...),
		IssuedAt:            now,
		ExpiresAt:           now.Add(DefaultValidity),
	}
	record.GrantDigest, err = grantDigest(record)
	if err != nil {
		return Status{}, err
	}
	if err := store.write(record); err != nil {
		return Status{}, err
	}
	return statusFromGrant(StateActive, record), nil
}

// BindSelectedSource narrows an existing active setup grant to the exact
// source fingerprint the owner just selected. It cannot create authority from
// a source-selection action and cannot replace one previously bound scope.
func (store Store) BindSelectedSource(request Request, identity Identity) (Status, error) {
	if err := validateRequest(request, identity); err != nil {
		return Status{}, err
	}
	if request.SourceFingerprint == "" {
		return Status{}, errors.New("selected-source binding requires a SHA-256 fingerprint")
	}
	record, err := store.read(request.WorkspaceID)
	if errors.Is(err, os.ErrNotExist) {
		return Status{SchemaVersion: SchemaVersion, State: StateAuthorizationRequired, PolicyVersion: PolicyVersion, WorkspaceID: request.WorkspaceID}, ErrAuthorizationRequired
	}
	if err != nil {
		return Status{}, err
	}
	if err := validateGrant(record); err != nil {
		return Status{}, err
	}
	if record.PolicyVersion != PolicyVersion || record.WorkspacePathDigest != digest("maestro-setup-workspace-path", canonicalPath(request.WorkspacePath)) {
		return statusFromGrant(StateScopeChanged, record), errors.New("setup authorization workspace or policy changed")
	}
	if record.PrincipalRef != identity.PrincipalRef || record.DeviceRef != identity.DeviceRef {
		return statusFromGrant(StateIdentityChanged, record), errors.New("setup authorization identity changed")
	}
	if !store.now().Before(record.ExpiresAt) {
		return statusFromGrant(StateExpired, record), errors.New("setup authorization expired")
	}
	if record.SourceFingerprint != "none" && record.SourceFingerprint != request.SourceFingerprint {
		return statusFromGrant(StateScopeChanged, record), errors.New("setup authorization is bound to a different selected-source scope")
	}
	if record.SourceFingerprint == request.SourceFingerprint {
		return statusFromGrant(StateActive, record), nil
	}
	record.SourceFingerprint = request.SourceFingerprint
	record.GrantDigest, err = grantDigest(record)
	if err != nil {
		return Status{}, err
	}
	if err := store.write(record); err != nil {
		return Status{}, err
	}
	return statusFromGrant(StateActive, record), nil
}

func (store Store) Status(request Request, identity Identity) (Status, error) {
	if err := validateRequest(request, identity); err != nil {
		return Status{}, err
	}
	base := Status{SchemaVersion: SchemaVersion, State: StateAuthorizationRequired, PolicyVersion: PolicyVersion, WorkspaceID: request.WorkspaceID}
	record, err := store.read(request.WorkspaceID)
	if errors.Is(err, os.ErrNotExist) {
		return base, nil
	}
	if err != nil {
		return Status{}, err
	}
	if err := validateGrant(record); err != nil {
		return Status{}, err
	}
	state := StateActive
	if record.PolicyVersion != PolicyVersion || record.WorkspacePathDigest != digest("maestro-setup-workspace-path", canonicalPath(request.WorkspacePath)) || record.SourceFingerprint != normalizedSourceFingerprint(request.SourceFingerprint) {
		state = StateScopeChanged
	} else if record.PrincipalRef != identity.PrincipalRef || record.DeviceRef != identity.DeviceRef {
		state = StateIdentityChanged
	} else if !store.now().Before(record.ExpiresAt) {
		state = StateExpired
	}
	return statusFromGrant(state, record), nil
}

func (store Store) path(workspaceID string) string {
	return filepath.Join(store.Root, "setup-authorizations", workspaceID+".json")
}

func (store Store) now() time.Time {
	if store.Clock != nil {
		return store.Clock().UTC()
	}
	return time.Now().UTC()
}

func (store Store) read(workspaceID string) (grant, error) {
	path := store.path(workspaceID)
	info, err := os.Lstat(path)
	if err != nil {
		return grant{}, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return grant{}, errors.New("setup authorization must be a regular non-symlink file")
	}
	file, err := os.Open(path)
	if err != nil {
		return grant{}, err
	}
	defer file.Close()
	decoder := json.NewDecoder(io.LimitReader(file, 32<<10))
	decoder.DisallowUnknownFields()
	var record grant
	if err := decoder.Decode(&record); err != nil {
		return grant{}, fmt.Errorf("decode setup authorization: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return grant{}, errors.New("setup authorization contains trailing data")
	}
	return record, nil
}

func (store Store) write(record grant) error {
	if strings.TrimSpace(store.Root) == "" {
		return errors.New("setup authorization root is required")
	}
	if err := rejectSymlink(store.Root); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(store.Root, 0o700); err != nil {
		return err
	}
	directory := filepath.Join(store.Root, "setup-authorizations")
	if err := rejectSymlink(directory); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	body, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	temporary, err := os.CreateTemp(directory, ".setup-authorization-*.tmp")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(append(body, '\n')); err != nil {
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
	return os.Rename(temporaryPath, store.path(record.WorkspaceID))
}

func validateRequest(request Request, identity Identity) error {
	if !workspaceIDPattern.MatchString(request.WorkspaceID) {
		return errors.New("setup authorization requires a canonical workspace ID")
	}
	if !filepath.IsAbs(request.WorkspacePath) || canonicalPath(request.WorkspacePath) == string(filepath.Separator) {
		return errors.New("setup authorization requires an absolute workspace path")
	}
	if request.SourceFingerprint != "" && !digestPattern.MatchString(request.SourceFingerprint) {
		return errors.New("setup authorization source fingerprint must be empty or SHA-256")
	}
	if !digestPattern.MatchString(identity.PrincipalRef) || !digestPattern.MatchString(identity.DeviceRef) {
		return errors.New("setup authorization requires opaque principal and device references")
	}
	return nil
}

func validateGrant(record grant) error {
	if record.SchemaVersion != SchemaVersion || !workspaceIDPattern.MatchString(record.WorkspaceID) || !digestPattern.MatchString(record.WorkspacePathDigest) || !digestPattern.MatchString(record.PrincipalRef) || !digestPattern.MatchString(record.DeviceRef) || (record.SourceFingerprint != "none" && !digestPattern.MatchString(record.SourceFingerprint)) {
		return errors.New("setup authorization has invalid identity or scope fields")
	}
	if !equalStrings(record.AllowedActions, allowedActions) || record.IssuedAt.IsZero() || !record.ExpiresAt.After(record.IssuedAt) || !digestPattern.MatchString(record.GrantDigest) {
		return errors.New("setup authorization has invalid policy fields")
	}
	want, err := grantDigest(record)
	if err != nil {
		return err
	}
	if want != record.GrantDigest {
		return errors.New("setup authorization digest mismatch")
	}
	return nil
}

func grantDigest(record grant) (string, error) {
	record.GrantDigest = ""
	body, err := json.Marshal(record)
	if err != nil {
		return "", err
	}
	return digest("maestro-setup-grant", string(body)), nil
}

func statusFromGrant(state string, record grant) Status {
	return Status{SchemaVersion: SchemaVersion, State: state, PolicyVersion: record.PolicyVersion, WorkspaceID: record.WorkspaceID, GrantDigest: record.GrantDigest, AllowedActions: append([]string(nil), record.AllowedActions...), IssuedAt: record.IssuedAt, ExpiresAt: record.ExpiresAt}
}

func normalizedSourceFingerprint(value string) string {
	if value == "" {
		return "none"
	}
	return value
}

func digest(domain, value string) string {
	sum := sha256.Sum256([]byte(domain + "\x00" + value))
	return hex.EncodeToString(sum[:])
}

func canonicalPath(path string) string {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	return filepath.Clean(absolute)
}

func rejectSymlink(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("setup authorization path is a symlink: %s", path)
	}
	if !info.IsDir() {
		return fmt.Errorf("setup authorization path is not a directory: %s", path)
	}
	return nil
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	leftCopy := append([]string(nil), left...)
	rightCopy := append([]string(nil), right...)
	sort.Strings(leftCopy)
	sort.Strings(rightCopy)
	for index := range leftCopy {
		if leftCopy[index] != rightCopy[index] {
			return false
		}
	}
	return true
}
