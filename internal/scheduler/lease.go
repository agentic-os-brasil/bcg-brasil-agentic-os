package scheduler

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var ErrLeaseBusy = errors.New("scheduler lease is already held")
var ErrLeaseLost = errors.New("scheduler lease fencing token is no longer current")

// Lease is a short-lived worker claim. Hooks never acquire this lease; only a
// bounded worker does. A busy or malformed lease is surfaced immediately so a
// lifecycle event cannot wait for another process.
type Lease struct {
	SchemaVersion int       `json:"schema_version"`
	WorkspaceID   string    `json:"workspace_id"`
	JobID         string    `json:"job_id"`
	OccurrenceKey string    `json:"occurrence_key"`
	OwnerID       string    `json:"owner_id"`
	FenceToken    string    `json:"fence_token"`
	AcquiredAt    time.Time `json:"acquired_at"`
	ExpiresAt     time.Time `json:"expires_at"`
}

func (store Store) TryAcquireLease(workspaceID, jobID, occurrenceKey, ownerID string, now time.Time, ttl time.Duration) (Lease, error) {
	if err := validateStoreInput(store.Root, workspaceID); err != nil {
		return Lease{}, err
	}
	if !jobIDPattern.MatchString(jobID) || strings.TrimSpace(occurrenceKey) == "" || len([]byte(occurrenceKey)) > 256 || strings.TrimSpace(ownerID) == "" || len([]byte(ownerID)) > 128 {
		return Lease{}, errors.New("invalid scheduler lease identity")
	}
	if now.IsZero() || ttl <= 0 || ttl > 15*time.Minute {
		return Lease{}, errors.New("scheduler lease requires a bounded positive TTL")
	}
	directory, err := ensurePrivateTree(store.Root, "workspaces", workspaceID, "leases", jobID)
	if err != nil {
		return Lease{}, err
	}
	path := filepath.Join(directory, safeLeaseName(occurrenceKey)+".json")
	guard, err := acquireLeaseGuard(filepath.Join(directory, safeLeaseName(occurrenceKey)+".guard"))
	if err != nil {
		if errors.Is(err, errLeaseGuardBusy) {
			return Lease{}, ErrLeaseBusy
		}
		return Lease{}, err
	}
	defer guard.release()
	if existing, err := readLease(path); err == nil {
		if existing.WorkspaceID != workspaceID || existing.JobID != jobID || existing.OccurrenceKey != occurrenceKey {
			return Lease{}, errors.New("scheduler lease identity mismatch")
		}
		if existing.ExpiresAt.After(now) {
			return Lease{}, ErrLeaseBusy
		}
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return Lease{}, err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return Lease{}, err
	}
	fenceToken, err := newFenceToken()
	if err != nil {
		return Lease{}, err
	}
	lease := Lease{SchemaVersion: 1, WorkspaceID: workspaceID, JobID: jobID, OccurrenceKey: occurrenceKey, OwnerID: ownerID, FenceToken: fenceToken, AcquiredAt: now.UTC(), ExpiresAt: now.Add(ttl).UTC()}
	if err := writeNewJSON(path, lease); errors.Is(err, os.ErrExist) {
		return Lease{}, ErrLeaseBusy
	} else if err != nil {
		return Lease{}, err
	}
	return lease, nil
}

func (store Store) ReleaseLease(lease Lease) error {
	if err := validateStoreInput(store.Root, lease.WorkspaceID); err != nil {
		return err
	}
	if err := validateLeaseIdentity(lease); err != nil {
		return err
	}
	directory, err := ensurePrivateTree(store.Root, "workspaces", lease.WorkspaceID, "leases", lease.JobID)
	if err != nil {
		return err
	}
	path := filepath.Join(directory, safeLeaseName(lease.OccurrenceKey)+".json")
	guard, err := acquireLeaseGuard(filepath.Join(directory, safeLeaseName(lease.OccurrenceKey)+".guard"))
	if err != nil {
		if errors.Is(err, errLeaseGuardBusy) {
			return ErrLeaseBusy
		}
		return err
	}
	defer guard.release()
	current, err := readLease(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !sameLeaseIdentity(current, lease) {
		return ErrLeaseLost
	}
	return os.Remove(path)
}

// LeaseCurrent is the side-effect/finalization fence. A worker must still own
// the exact immutable token before it publishes a terminal result.
func (store Store) LeaseCurrent(lease Lease, now time.Time) error {
	return store.WithCurrentLease(lease, now, func() error { return nil })
}

// WithCurrentLease holds the per-occurrence OS guard across terminal
// publication, closing the check-then-write takeover race.
func (store Store) WithCurrentLease(lease Lease, now time.Time, publish func() error) error {
	if err := validateStoreInput(store.Root, lease.WorkspaceID); err != nil {
		return err
	}
	if err := validateLeaseIdentity(lease); err != nil {
		return err
	}
	if now.IsZero() || publish == nil {
		return errors.New("scheduler lease validation time is required")
	}
	directory, err := ensurePrivateTree(store.Root, "workspaces", lease.WorkspaceID, "leases", lease.JobID)
	if err != nil {
		return err
	}
	name := safeLeaseName(lease.OccurrenceKey)
	path := filepath.Join(directory, name+".json")
	guard, err := acquireLeaseGuard(filepath.Join(directory, name+".guard"))
	if err != nil {
		if errors.Is(err, errLeaseGuardBusy) {
			return ErrLeaseBusy
		}
		return err
	}
	defer guard.release()
	current, err := readLease(path)
	if err != nil {
		return err
	}
	if !sameLeaseIdentity(current, lease) || !current.ExpiresAt.After(now) {
		return ErrLeaseLost
	}
	return publish()
}

func safeLeaseName(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}

func readLease(path string) (Lease, error) {
	before, err := os.Lstat(path)
	if err != nil {
		return Lease{}, err
	}
	if before.Mode()&os.ModeSymlink != 0 || !before.Mode().IsRegular() {
		return Lease{}, fmt.Errorf("invalid scheduler lease %s", path)
	}
	file, err := os.Open(path)
	if err != nil {
		return Lease{}, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(before, opened) {
		return Lease{}, fmt.Errorf("scheduler lease changed during secure open: %s", path)
	}
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	var lease Lease
	if err := decoder.Decode(&lease); err != nil {
		return Lease{}, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return Lease{}, fmt.Errorf("scheduler lease contains multiple JSON values: %s", path)
		}
		return Lease{}, err
	}
	if err := validateLeaseIdentity(lease); err != nil || !lease.ExpiresAt.After(lease.AcquiredAt) || lease.ExpiresAt.Sub(lease.AcquiredAt) > 15*time.Minute {
		return Lease{}, fmt.Errorf("invalid scheduler lease %s", path)
	}
	return lease, nil
}

func validateLeaseIdentity(lease Lease) error {
	if lease.SchemaVersion != 1 || !workspaceIDPattern.MatchString(lease.WorkspaceID) || !jobIDPattern.MatchString(lease.JobID) || strings.TrimSpace(lease.OccurrenceKey) == "" || len([]byte(lease.OccurrenceKey)) > 256 || strings.TrimSpace(lease.OwnerID) == "" || len([]byte(lease.OwnerID)) > 128 || !attemptTokenPattern.MatchString(lease.FenceToken) || lease.AcquiredAt.IsZero() || lease.ExpiresAt.IsZero() {
		return errors.New("invalid scheduler lease identity")
	}
	return nil
}

func sameLeaseIdentity(left, right Lease) bool {
	return left.WorkspaceID == right.WorkspaceID &&
		left.JobID == right.JobID &&
		left.OccurrenceKey == right.OccurrenceKey &&
		left.OwnerID == right.OwnerID &&
		left.FenceToken == right.FenceToken
}

func newFenceToken() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(value[:]), nil
}
