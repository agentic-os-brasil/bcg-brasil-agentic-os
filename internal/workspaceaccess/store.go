// Package workspaceaccess implements explicit, temporary and read-only access
// to bounded Maestro-owned context from another enrolled workspace. It never
// grants access to the target checkout.
package workspaceaccess

import (
	"context"
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

	basememory "github.com/agentic-os-brasil/bcg-brasil-agentic-os/bundles/base/memory"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/memory"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/privatelock"
)

const (
	SchemaVersion        = 1
	DefaultTTL           = 30 * time.Minute
	MinimumTTL           = 5 * time.Minute
	MaximumTTL           = 2 * time.Hour
	MaximumResponseBytes = 8 << 10
	maximumGrantCount    = 128
	maximumGrantBytes    = 32 << 10

	StateActive  = "active"
	StateExpired = "expired"
	StateRevoked = "revoked"

	PurposeReferenceContext       = "reference_context"
	PurposeCompareImplementation  = "compare_implementation"
	PurposeReuseLearning          = "reuse_learning"
	PurposeDependencyCoordination = "dependency_coordination"

	SourceContext    = "context"
	SourceMemory     = "memory"
	SourceContinuity = "continuity"
)

var (
	ErrConfirmationRequired = errors.New("explicit confirmation is required")
	ErrExpired              = errors.New("cross-workspace context grant expired")
	ErrRevoked              = errors.New("cross-workspace context grant revoked")
	idPattern               = regexp.MustCompile(`^[a-f0-9]{32}$`)
	digestPattern           = regexp.MustCompile(`^[a-f0-9]{64}$`)
	grantIDPattern          = regexp.MustCompile(`^[a-f0-9]{32}$`)
)

type Authority struct {
	WorkspaceID  string `json:"workspace_id"`
	RepositoryID string `json:"repository_id"`
}

type Identity struct {
	PrincipalRef string
	DeviceRef    string
}

type GrantRequest struct {
	Runtime   string
	Source    Authority
	Target    Authority
	Purpose   string
	Sources   []string
	TTL       time.Duration
	Confirmed bool
}

type Status struct {
	SchemaVersion     int       `json:"schema_version"`
	State             string    `json:"state"`
	GrantID           string    `json:"grant_id"`
	Runtime           string    `json:"runtime"`
	SourceWorkspaceID string    `json:"source_workspace_id"`
	TargetWorkspaceID string    `json:"target_workspace_id"`
	Purpose           string    `json:"purpose"`
	Sources           []string  `json:"sources"`
	IssuedAt          time.Time `json:"issued_at"`
	ExpiresAt         time.Time `json:"expires_at"`
}

type Section struct {
	Source  string `json:"source"`
	State   string `json:"state"`
	Content string `json:"content,omitempty"`
}

type ReadResult struct {
	SchemaVersion     int       `json:"schema_version"`
	GrantID           string    `json:"grant_id"`
	Runtime           string    `json:"runtime"`
	SourceWorkspaceID string    `json:"source_workspace_id"`
	TargetWorkspaceID string    `json:"target_workspace_id"`
	Purpose           string    `json:"purpose"`
	ExpiresAt         time.Time `json:"expires_at"`
	Sections          []Section `json:"sections"`
}

type TargetResolver func(context.Context, string, string) (Authority, error)

type Store struct {
	Root   string
	Clock  func() time.Time
	Random io.Reader
}

type grantRecord struct {
	SchemaVersion      int        `json:"schema_version"`
	GrantID            string     `json:"grant_id"`
	Runtime            string     `json:"runtime"`
	SourceWorkspaceID  string     `json:"source_workspace_id"`
	SourceRepositoryID string     `json:"source_repository_id"`
	TargetWorkspaceID  string     `json:"target_workspace_id"`
	TargetRepositoryID string     `json:"target_repository_id"`
	PrincipalRef       string     `json:"principal_ref"`
	DeviceRef          string     `json:"device_ref"`
	Purpose            string     `json:"purpose"`
	Sources            []string   `json:"sources"`
	IssuedAt           time.Time  `json:"issued_at"`
	ExpiresAt          time.Time  `json:"expires_at"`
	RevokedAt          *time.Time `json:"revoked_at,omitempty"`
	Integrity          string     `json:"integrity"`
}

func DeriveIdentity(principal, device string) Identity {
	return Identity{
		PrincipalRef: digest("maestro-workspace-access-principal", principal),
		DeviceRef:    digest("maestro-workspace-access-device", device),
	}
}

func (store Store) Grant(request GrantRequest, identity Identity) (Status, error) {
	if request.TTL == 0 {
		request.TTL = DefaultTTL
	}
	sources, err := validateGrantRequest(request, identity)
	if err != nil {
		return Status{}, err
	}
	if !request.Confirmed {
		return Status{}, ErrConfirmationRequired
	}
	unlock, err := privatelock.Acquire(store.lockPath(request.Source.WorkspaceID))
	if err != nil {
		return Status{}, fmt.Errorf("lock cross-workspace grant store: %w", err)
	}
	defer unlock()
	if err := store.ensureGrantCapacity(request.Source.WorkspaceID); err != nil {
		return Status{}, err
	}
	grantID, err := store.nextGrantID(request.Source.WorkspaceID)
	if err != nil {
		return Status{}, fmt.Errorf("create opaque grant id: %w", err)
	}
	now := store.now()
	record := grantRecord{
		SchemaVersion: SchemaVersion, GrantID: grantID, Runtime: request.Runtime,
		SourceWorkspaceID: request.Source.WorkspaceID, SourceRepositoryID: request.Source.RepositoryID,
		TargetWorkspaceID: request.Target.WorkspaceID, TargetRepositoryID: request.Target.RepositoryID,
		PrincipalRef: identity.PrincipalRef, DeviceRef: identity.DeviceRef,
		Purpose: request.Purpose, Sources: sources, IssuedAt: now, ExpiresAt: now.Add(request.TTL),
	}
	key, err := store.integrityKey(true)
	if err != nil {
		return Status{}, err
	}
	record.Integrity, err = signRecord(record, key)
	if err != nil {
		return Status{}, err
	}
	if err := writeJSONAtomic(store.Root, store.grantPath(request.Source.WorkspaceID, grantID), record); err != nil {
		return Status{}, err
	}
	return statusFromRecord(record, StateActive), nil
}

func (store Store) Status(runtimeName string, source Authority, grantID string, identity Identity) (Status, error) {
	record, err := store.authenticatedRecord(runtimeName, source, grantID, identity)
	if err != nil {
		return Status{}, err
	}
	return statusFromRecord(record, store.state(record)), nil
}

func (store Store) Revoke(runtimeName string, source Authority, grantID string, identity Identity) (Status, error) {
	if err := validateAuthority(source); err != nil {
		return Status{}, err
	}
	unlock, err := privatelock.Acquire(store.lockPath(source.WorkspaceID))
	if err != nil {
		return Status{}, fmt.Errorf("lock cross-workspace grant store: %w", err)
	}
	defer unlock()
	record, err := store.authenticatedRecord(runtimeName, source, grantID, identity)
	if err != nil {
		return Status{}, err
	}
	if record.RevokedAt == nil {
		now := store.now()
		record.RevokedAt = &now
		key, keyErr := store.integrityKey(false)
		if keyErr != nil {
			return Status{}, keyErr
		}
		record.Integrity, err = signRecord(record, key)
		if err != nil {
			return Status{}, err
		}
		if err := writeJSONAtomic(store.Root, store.grantPath(source.WorkspaceID, grantID), record); err != nil {
			return Status{}, err
		}
	}
	return statusFromRecord(record, StateRevoked), nil
}

func (store Store) Read(ctx context.Context, runtimeName string, source Authority, grantID string, identity Identity, resolve TargetResolver) (ReadResult, error) {
	if resolve == nil {
		return ReadResult{}, errors.New("target enrollment resolver is required")
	}
	record, err := store.authenticatedRecord(runtimeName, source, grantID, identity)
	if err != nil {
		return ReadResult{}, err
	}
	switch store.state(record) {
	case StateExpired:
		return ReadResult{}, ErrExpired
	case StateRevoked:
		return ReadResult{}, ErrRevoked
	}
	target, err := resolve(ctx, runtimeName, record.TargetWorkspaceID)
	if err != nil {
		return ReadResult{}, fmt.Errorf("revalidate target enrollment: %w", err)
	}
	if target.WorkspaceID != record.TargetWorkspaceID || target.RepositoryID != record.TargetRepositoryID {
		return ReadResult{}, errors.New("target enrollment identity changed")
	}
	result := ReadResult{
		SchemaVersion: SchemaVersion, GrantID: record.GrantID, Runtime: record.Runtime,
		SourceWorkspaceID: record.SourceWorkspaceID, TargetWorkspaceID: record.TargetWorkspaceID,
		Purpose: record.Purpose, ExpiresAt: record.ExpiresAt,
	}
	for _, sourceName := range record.Sources {
		section, readErr := store.readSection(record.TargetWorkspaceID, sourceName)
		if readErr != nil {
			return ReadResult{}, readErr
		}
		result.Sections = append(result.Sections, section)
	}
	body, err := json.Marshal(result)
	if err != nil {
		return ReadResult{}, err
	}
	if len(body) > MaximumResponseBytes {
		return ReadResult{}, errors.New("cross-workspace context response exceeds 8 KiB")
	}
	return result, nil
}

func (store Store) readSection(workspaceID, sourceName string) (Section, error) {
	switch sourceName {
	case SourceContext:
		body, err := store.readPrivateFile(workspaceID, filepath.Join("context", "session-context.md"), 2048)
		return sectionFromBody(sourceName, body), err
	case SourceContinuity:
		body, err := store.readPrivateFile(workspaceID, filepath.Join("continuity", "active.md"), 1536)
		return sectionFromBody(sourceName, body), err
	case SourceMemory:
		policy, err := basememory.Policy()
		if err != nil {
			return Section{}, fmt.Errorf("load memory policy: %w", err)
		}
		runtimeConfig, err := basememory.Runtime()
		if err != nil {
			return Section{}, fmt.Errorf("load memory runtime: %w", err)
		}
		engine := memory.Engine{Root: store.Root, Policy: policy, Budgets: runtimeConfig.ContextBudgets(), Now: store.now}
		bundle, err := engine.AssembleContext(workspaceID)
		if err != nil {
			return Section{Source: sourceName, State: "unavailable"}, nil
		}
		var builder strings.Builder
		for _, part := range bundle.Sections {
			candidate := "[" + part.Layer + "]\n" + part.Content + "\n"
			remaining := 3072 - builder.Len()
			if remaining <= 0 {
				break
			}
			if len(candidate) > remaining {
				candidate = truncateUTF8Bytes(candidate, remaining)
			}
			builder.WriteString(candidate)
		}
		return sectionFromBody(sourceName, strings.TrimSpace(builder.String())), nil
	default:
		return Section{}, errors.New("unknown cross-workspace context source")
	}
}

func sectionFromBody(sourceName, body string) Section {
	state := "available"
	if body == "" {
		state = "empty"
	}
	return Section{Source: sourceName, State: state, Content: body}
}

func validateGrantRequest(request GrantRequest, identity Identity) ([]string, error) {
	if request.Runtime != "claude" && request.Runtime != "codex" {
		return nil, errors.New("runtime must be claude or codex")
	}
	if err := validateAuthority(request.Source); err != nil {
		return nil, fmt.Errorf("source authority: %w", err)
	}
	if err := validateAuthority(request.Target); err != nil {
		return nil, fmt.Errorf("target authority: %w", err)
	}
	if request.Source.WorkspaceID == request.Target.WorkspaceID {
		return nil, errors.New("source and target workspaces must be distinct")
	}
	if request.Purpose != PurposeReferenceContext && request.Purpose != PurposeCompareImplementation && request.Purpose != PurposeReuseLearning && request.Purpose != PurposeDependencyCoordination {
		return nil, errors.New("purpose is outside the closed cross-workspace registry")
	}
	if request.TTL < MinimumTTL || request.TTL > MaximumTTL {
		return nil, errors.New("TTL must be between 5 minutes and 2 hours")
	}
	if !digestPattern.MatchString(identity.PrincipalRef) || !digestPattern.MatchString(identity.DeviceRef) {
		return nil, errors.New("local principal and device identity are required")
	}
	seen := map[string]bool{}
	for _, source := range request.Sources {
		if source != SourceContext && source != SourceMemory && source != SourceContinuity {
			return nil, errors.New("source is outside the closed cross-workspace registry")
		}
		seen[source] = true
	}
	if len(seen) == 0 {
		return nil, errors.New("at least one context source is required")
	}
	sources := make([]string, 0, len(seen))
	for _, source := range []string{SourceContext, SourceMemory, SourceContinuity} {
		if seen[source] {
			sources = append(sources, source)
		}
	}
	return sources, nil
}

func validateAuthority(authority Authority) error {
	if !idPattern.MatchString(authority.WorkspaceID) || !idPattern.MatchString(authority.RepositoryID) {
		return errors.New("workspace and repository identities must be opaque 32-character values")
	}
	return nil
}

func (store Store) authenticatedRecord(runtimeName string, source Authority, grantID string, identity Identity) (grantRecord, error) {
	if err := validateAuthority(source); err != nil {
		return grantRecord{}, err
	}
	if !grantIDPattern.MatchString(grantID) {
		return grantRecord{}, errors.New("grant id is invalid")
	}
	record, err := store.readRecord(source.WorkspaceID, grantID)
	if err != nil {
		return grantRecord{}, err
	}
	key, err := store.integrityKey(false)
	if err != nil {
		return grantRecord{}, err
	}
	want, err := signRecord(record, key)
	if err != nil || !hmac.Equal([]byte(record.Integrity), []byte(want)) {
		return grantRecord{}, errors.New("cross-workspace grant integrity verification failed")
	}
	if err := validateRecord(record); err != nil {
		return grantRecord{}, err
	}
	if record.Runtime != runtimeName || record.SourceWorkspaceID != source.WorkspaceID || record.SourceRepositoryID != source.RepositoryID {
		return grantRecord{}, errors.New("cross-workspace grant scope changed")
	}
	if record.PrincipalRef != identity.PrincipalRef || record.DeviceRef != identity.DeviceRef {
		return grantRecord{}, errors.New("cross-workspace grant identity changed")
	}
	return record, nil
}

func validateRecord(record grantRecord) error {
	if record.SchemaVersion != SchemaVersion || !grantIDPattern.MatchString(record.GrantID) || record.Runtime == "" || !idPattern.MatchString(record.SourceWorkspaceID) || !idPattern.MatchString(record.SourceRepositoryID) || !idPattern.MatchString(record.TargetWorkspaceID) || !idPattern.MatchString(record.TargetRepositoryID) || !digestPattern.MatchString(record.PrincipalRef) || !digestPattern.MatchString(record.DeviceRef) || !digestPattern.MatchString(record.Integrity) {
		return errors.New("cross-workspace grant record is invalid")
	}
	if !record.ExpiresAt.After(record.IssuedAt) || record.ExpiresAt.Sub(record.IssuedAt) < MinimumTTL || record.ExpiresAt.Sub(record.IssuedAt) > MaximumTTL {
		return errors.New("cross-workspace grant lifetime is invalid")
	}
	_, err := validateGrantRequest(GrantRequest{Runtime: record.Runtime, Source: Authority{record.SourceWorkspaceID, record.SourceRepositoryID}, Target: Authority{record.TargetWorkspaceID, record.TargetRepositoryID}, Purpose: record.Purpose, Sources: record.Sources, TTL: record.ExpiresAt.Sub(record.IssuedAt)}, Identity{record.PrincipalRef, record.DeviceRef})
	return err
}

func statusFromRecord(record grantRecord, state string) Status {
	return Status{SchemaVersion: SchemaVersion, State: state, GrantID: record.GrantID, Runtime: record.Runtime, SourceWorkspaceID: record.SourceWorkspaceID, TargetWorkspaceID: record.TargetWorkspaceID, Purpose: record.Purpose, Sources: append([]string(nil), record.Sources...), IssuedAt: record.IssuedAt, ExpiresAt: record.ExpiresAt}
}

func (store Store) state(record grantRecord) string {
	if record.RevokedAt != nil {
		return StateRevoked
	}
	if !store.now().Before(record.ExpiresAt) {
		return StateExpired
	}
	return StateActive
}

func (store Store) readRecord(workspaceID, grantID string) (grantRecord, error) {
	path := store.grantPath(workspaceID, grantID)
	info, err := os.Lstat(path)
	if err != nil {
		return grantRecord{}, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 || info.Size() > maximumGrantBytes {
		return grantRecord{}, errors.New("cross-workspace grant must be a bounded regular non-symlink file")
	}
	file, err := os.Open(path)
	if err != nil {
		return grantRecord{}, err
	}
	defer file.Close()
	decoder := json.NewDecoder(io.LimitReader(file, maximumGrantBytes+1))
	decoder.DisallowUnknownFields()
	var record grantRecord
	if err := decoder.Decode(&record); err != nil {
		return grantRecord{}, fmt.Errorf("decode cross-workspace grant: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return grantRecord{}, errors.New("cross-workspace grant contains trailing data")
	}
	return record, nil
}

func (store Store) readPrivateFile(workspaceID, relative string, limit int64) (string, error) {
	path := filepath.Join(store.Root, "workspaces", workspaceID, relative)
	if err := rejectExistingSymlink(store.Root, path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", err
	}
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Size() > limit {
		return "", errors.New("cross-workspace context source must be a bounded regular non-symlink file")
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	if !utf8.Valid(body) {
		return "", errors.New("cross-workspace context source is not valid UTF-8")
	}
	return strings.TrimSpace(string(body)), nil
}

func (store Store) ensureGrantCapacity(workspaceID string) error {
	directory := store.grantsDir(workspaceID)
	if err := rejectExistingSymlink(store.Root, directory); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	entries, err := os.ReadDir(directory)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if len(entries) > maximumGrantCount*2 {
		return errors.New("cross-workspace grant store exceeds its inspection limit")
	}
	grantCount := 0
	for _, entry := range entries {
		if entry.Type()&os.ModeSymlink != 0 {
			return errors.New("cross-workspace grant store contains a symlink")
		}
		name := entry.Name()
		if strings.HasPrefix(name, ".workspace-access-") && strings.HasSuffix(name, ".tmp") && entry.Type().IsRegular() {
			continue
		}
		if !strings.HasSuffix(name, ".json") || !grantIDPattern.MatchString(strings.TrimSuffix(name, ".json")) || !entry.Type().IsRegular() {
			return errors.New("cross-workspace grant store contains an invalid entry")
		}
		grantCount++
	}
	if grantCount >= maximumGrantCount {
		return errors.New("cross-workspace grant store reached its bounded capacity")
	}
	return nil
}

func (store Store) integrityKey(create bool) ([]byte, error) {
	if strings.TrimSpace(store.Root) == "" {
		return nil, errors.New("workspace-access root is required")
	}
	path := filepath.Join(store.Root, "workspace-access", "integrity.key")
	info, err := os.Lstat(path)
	if err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 || info.Size() != 32 {
			return nil, errors.New("workspace-access integrity key is invalid")
		}
		return os.ReadFile(path)
	}
	if !errors.Is(err, os.ErrNotExist) || !create {
		return nil, fmt.Errorf("read workspace-access integrity key: %w", err)
	}
	if err := rejectExistingSymlink(store.Root, filepath.Dir(path)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	unlock, err := privatelock.Acquire(filepath.Join(filepath.Dir(path), ".key.lock"))
	if err != nil {
		return nil, fmt.Errorf("lock workspace-access integrity key: %w", err)
	}
	defer unlock()
	if info, err = os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 || info.Size() != 32 {
			return nil, errors.New("workspace-access integrity key is invalid")
		}
		return os.ReadFile(path)
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	key := make([]byte, 32)
	if _, err := io.ReadFull(store.random(), key); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, os.ErrExist) {
		return store.integrityKey(false)
	}
	if err != nil {
		return nil, err
	}
	if _, err := file.Write(key); err != nil {
		file.Close()
		return nil, err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return nil, err
	}
	if err := file.Close(); err != nil {
		return nil, err
	}
	return key, nil
}

func signRecord(record grantRecord, key []byte) (string, error) {
	record.Integrity = ""
	body, err := json.Marshal(record)
	if err != nil {
		return "", err
	}
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil)), nil
}

func writeJSONAtomic(root, path string, value any) error {
	if err := rejectExistingSymlink(root, path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".workspace-access-*.tmp")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(append(body, '\n')); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, path)
}

func rejectExistingSymlink(root, path string) error {
	root, err := filepath.Abs(filepath.Clean(root))
	if err != nil {
		return err
	}
	absolute, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return err
	}
	relative, err := filepath.Rel(root, absolute)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return errors.New("cross-workspace private path escaped its data root")
	}
	for current := absolute; ; current = filepath.Dir(current) {
		info, statErr := os.Lstat(current)
		if statErr == nil && info.Mode()&os.ModeSymlink != 0 {
			return errors.New("cross-workspace private path contains a symlink")
		}
		if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
			return statErr
		}
		if current == root {
			break
		}
		parent := filepath.Dir(current)
		if parent == current {
			return errors.New("cross-workspace private path did not reach its data root")
		}
	}
	return nil
}

func (store Store) now() time.Time {
	if store.Clock != nil {
		return store.Clock().UTC()
	}
	return time.Now().UTC()
}

func (store Store) random() io.Reader {
	if store.Random != nil {
		return store.Random
	}
	return rand.Reader
}

func (store Store) randomHex(bytesCount int) (string, error) {
	body := make([]byte, bytesCount)
	if _, err := io.ReadFull(store.random(), body); err != nil {
		return "", err
	}
	return hex.EncodeToString(body), nil
}

func (store Store) nextGrantID(workspaceID string) (string, error) {
	for attempt := 0; attempt < 4; attempt++ {
		grantID, err := store.randomHex(16)
		if err != nil {
			return "", err
		}
		if _, err := os.Lstat(store.grantPath(workspaceID, grantID)); errors.Is(err, os.ErrNotExist) {
			return grantID, nil
		} else if err != nil {
			return "", err
		}
	}
	return "", errors.New("could not allocate a unique opaque grant id")
}

func (store Store) grantsDir(workspaceID string) string {
	return filepath.Join(store.Root, "workspaces", workspaceID, "access", "grants")
}

func (store Store) grantPath(workspaceID, grantID string) string {
	return filepath.Join(store.grantsDir(workspaceID), grantID+".json")
}

func (store Store) lockPath(workspaceID string) string {
	return filepath.Join(store.Root, "workspaces", workspaceID, "access", ".transition.lock")
}

func digest(domain, value string) string {
	sum := sha256.Sum256([]byte(domain + "\x00" + value))
	return hex.EncodeToString(sum[:])
}

func truncateUTF8Bytes(value string, maximum int) string {
	if len(value) <= maximum {
		return value
	}
	value = value[:maximum]
	for !utf8.ValidString(value) {
		value = value[:len(value)-1]
	}
	return value
}
