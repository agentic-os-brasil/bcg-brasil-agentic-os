package scheduler

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var ErrLeaseBusy = errors.New("scheduler lease is already held")

// Lease is a short-lived worker claim. Hooks never acquire this lease; only a
// bounded worker does. A busy or malformed lease is surfaced immediately so a
// lifecycle event cannot wait for another process.
type Lease struct {
	SchemaVersion int       `json:"schema_version"`
	WorkspaceID   string    `json:"workspace_id"`
	JobID         string    `json:"job_id"`
	OccurrenceKey string    `json:"occurrence_key"`
	OwnerID       string    `json:"owner_id"`
	AcquiredAt    time.Time `json:"acquired_at"`
	ExpiresAt     time.Time `json:"expires_at"`
}

func (store Store) TryAcquireLease(workspaceID, jobID, occurrenceKey, ownerID string, now time.Time, ttl time.Duration) (Lease, error) {
	if err := validateStoreInput(store.Root, workspaceID); err != nil {
		return Lease{}, err
	}
	if !jobIDPattern.MatchString(jobID) || strings.TrimSpace(occurrenceKey) == "" || strings.TrimSpace(ownerID) == "" {
		return Lease{}, errors.New("invalid scheduler lease identity")
	}
	if now.IsZero() || ttl <= 0 || ttl > 15*time.Minute {
		return Lease{}, errors.New("scheduler lease requires a bounded positive TTL")
	}
	path := store.leasePath(workspaceID, jobID, occurrenceKey)
	if existing, err := readLease(path); err == nil {
		if existing.ExpiresAt.After(now) {
			return Lease{}, ErrLeaseBusy
		}
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return Lease{}, err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return Lease{}, err
	}
	lease := Lease{SchemaVersion: 1, WorkspaceID: workspaceID, JobID: jobID, OccurrenceKey: occurrenceKey, OwnerID: ownerID, AcquiredAt: now.UTC(), ExpiresAt: now.Add(ttl).UTC()}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return Lease{}, err
	}
	if info, err := os.Lstat(filepath.Dir(path)); err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		if err != nil {
			return Lease{}, err
		}
		return Lease{}, errors.New("scheduler lease directory must be a private local directory")
	}
	if err := writeNewJSON(path, lease); errors.Is(err, os.ErrExist) {
		return Lease{}, ErrLeaseBusy
	} else if err != nil {
		return Lease{}, err
	}
	return lease, nil
}

func (store Store) ReleaseLease(workspaceID, jobID, occurrenceKey, ownerID string) error {
	if err := validateStoreInput(store.Root, workspaceID); err != nil {
		return err
	}
	path := store.leasePath(workspaceID, jobID, occurrenceKey)
	lease, err := readLease(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if lease.OwnerID != ownerID {
		return ErrLeaseBusy
	}
	return os.Remove(path)
}

func (store Store) leasePath(workspaceID, jobID, occurrenceKey string) string {
	return filepath.Join(store.workspaceRoot(workspaceID), "leases", jobID, safeLeaseName(occurrenceKey)+".json")
}

func safeLeaseName(value string) string {
	var builder strings.Builder
	for _, character := range value {
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') || (character >= '0' && character <= '9') || character == '-' || character == '_' {
			builder.WriteRune(character)
		} else {
			builder.WriteByte('_')
		}
	}
	return builder.String()
}

func readLease(path string) (Lease, error) {
	file, err := os.Open(path)
	if err != nil {
		return Lease{}, err
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	var lease Lease
	if err := decoder.Decode(&lease); err != nil {
		return Lease{}, err
	}
	if lease.SchemaVersion != 1 || !workspaceIDPattern.MatchString(lease.WorkspaceID) || !jobIDPattern.MatchString(lease.JobID) || strings.TrimSpace(lease.OccurrenceKey) == "" || strings.TrimSpace(lease.OwnerID) == "" || lease.AcquiredAt.IsZero() || lease.ExpiresAt.Before(lease.AcquiredAt) {
		return Lease{}, fmt.Errorf("invalid scheduler lease %s", path)
	}
	return lease, nil
}
