package scheduler

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
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
	marker := quarantinePath(path)
	if _, markerErr := secureReadFile(marker); markerErr == nil {
		return Lease{}, ErrLeaseBusy
	} else if !errors.Is(markerErr, os.ErrNotExist) {
		return Lease{}, markerErr
	}
	if existing, err := readLease(path); err == nil {
		if existing.WorkspaceID != workspaceID || existing.JobID != jobID || existing.OccurrenceKey != occurrenceKey {
			return Lease{}, errors.New("scheduler lease identity mismatch")
		}
		if existing.ExpiresAt.After(now) {
			return Lease{}, ErrLeaseBusy
		}
		if err := secureRemoveFile(path); err != nil && !errors.Is(err, os.ErrNotExist) {
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
		return removeQuarantineMarker(path, lease)
	}
	if err != nil {
		return err
	}
	if !sameLeaseIdentity(current, lease) {
		return ErrLeaseLost
	}
	if err := removeQuarantineMarker(path, lease); err != nil {
		return err
	}
	if err := secureRemoveFile(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func removeQuarantineMarker(leasePath string, lease Lease) error {
	marker := quarantinePath(leasePath)
	body, err := secureReadFile(marker)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var markerData struct {
		FenceToken string `json:"fence_token"`
	}
	if err := json.Unmarshal(body, &markerData); err != nil || markerData.FenceToken != lease.FenceToken {
		return ErrLeaseLost
	}
	return secureRemoveFile(marker)
}

// ArmLease installs an active execution fence before handler side effects.
// Normal completion removes it; a crashed process leaves an explicit marker
// that becomes recoverable after the lease deadline.
func (store Store) ArmLease(lease Lease) error {
	return store.writeFenceMarker(lease, "active")
}

// QuarantineLease transitions an active fence to operator-recoverable state.
// The marker is removed only by the original fence owner when its handler
// exits and ReleaseLease succeeds; a successor can therefore never reclaim a
// live, non-cooperative execution between TTL expiry and timeout handling.
func (store Store) QuarantineLease(lease Lease) error {
	return store.writeFenceMarker(lease, "quarantined")
}

func (store Store) writeFenceMarker(lease Lease, state string) error {
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
	if err != nil {
		return err
	}
	if !sameLeaseIdentity(current, lease) {
		return ErrLeaseLost
	}
	marker := quarantinePath(path)
	if body, markerErr := secureReadFile(marker); markerErr == nil {
		var existing struct {
			FenceToken string `json:"fence_token"`
			State      string `json:"state"`
		}
		if err := json.Unmarshal(body, &existing); err != nil || existing.FenceToken != lease.FenceToken {
			return ErrLeaseLost
		}
		if existing.State == state || (existing.State == "quarantined" && state == "active") {
			return nil
		}
		body, err = json.Marshal(struct {
			FenceToken string `json:"fence_token"`
			State      string `json:"state"`
		}{FenceToken: lease.FenceToken, State: state})
		if err != nil {
			return err
		}
		return secureWriteFile(marker, append(body, '\n'))
	} else if !errors.Is(markerErr, os.ErrNotExist) {
		return markerErr
	}
	return writeNewJSON(marker, struct {
		FenceToken string `json:"fence_token"`
		State      string `json:"state"`
	}{FenceToken: lease.FenceToken, State: state})
}

// QuarantinedLeases lists timeout-marked fences and expired active fences left
// by a crashed process. Healthy active execution fences are intentionally not
// reported as orphaned quarantine. The result is metadata-only.
func (store Store) QuarantinedLeases(workspaceID string) ([]Lease, error) {
	if err := validateStoreInput(store.Root, workspaceID); err != nil {
		return nil, err
	}
	root, rootErr := lookupPrivateTree(store.Root, "workspaces", workspaceID, "leases")
	if errors.Is(rootErr, os.ErrNotExist) {
		return nil, nil
	}
	if rootErr != nil {
		return nil, rootErr
	}
	entries, err := secureReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var leases []Lease
	for _, jobEntry := range entries {
		if jobEntry.Type()&os.ModeSymlink != 0 {
			return nil, errors.New("scheduler quarantine job path cannot be a symlink")
		}
		if !jobEntry.IsDir() {
			continue
		}
		jobRoot := filepath.Join(root, jobEntry.Name())
		markers, readErr := secureReadDir(jobRoot)
		if readErr != nil {
			return nil, readErr
		}
		for _, marker := range markers {
			if marker.Type()&os.ModeSymlink != 0 {
				return nil, errors.New("scheduler quarantine marker cannot be a symlink")
			}
			if marker.IsDir() || !strings.HasSuffix(marker.Name(), ".json.quarantine") {
				continue
			}
			leasePath := filepath.Join(jobRoot, strings.TrimSuffix(marker.Name(), ".quarantine"))
			lease, leaseErr := readLease(leasePath)
			if leaseErr != nil {
				return nil, leaseErr
			}
			if lease.WorkspaceID != workspaceID {
				return nil, errors.New("scheduler quarantine workspace mismatch")
			}
			markerBody, readErr := secureReadFile(filepath.Join(jobRoot, marker.Name()))
			if readErr != nil {
				return nil, readErr
			}
			var markerState struct {
				State string `json:"state"`
			}
			if err := json.Unmarshal(markerBody, &markerState); err != nil {
				return nil, err
			}
			if markerState.State == "active" && lease.ExpiresAt.After(time.Now().UTC()) {
				continue
			}
			leases = append(leases, lease)
		}
	}
	return leases, nil
}

// RecoverQuarantinedLease is an explicit operator recovery boundary. It
// refuses to clear a quarantine while the original lease is still live; the
// caller must separately attest that the process has exited or been restarted.
func (store Store) RecoverQuarantinedLease(lease Lease, now time.Time) error {
	if now.IsZero() || !lease.ExpiresAt.Before(now.UTC()) {
		return ErrLeaseBusy
	}
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
	marker := quarantinePath(path)
	if _, err := secureReadFile(marker); err != nil {
		return err
	}
	return store.ReleaseLease(lease)
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

func quarantinePath(leasePath string) string { return leasePath + ".quarantine" }

func ScheduledOccurrenceKey(jobID string, scheduledFor time.Time) string {
	return jobID + "\x00scheduled\x00" + scheduledFor.UTC().Format(time.RFC3339Nano)
}

func readLease(path string) (Lease, error) {
	var lease Lease
	if err := readStrictJSON(path, &lease); err != nil {
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
