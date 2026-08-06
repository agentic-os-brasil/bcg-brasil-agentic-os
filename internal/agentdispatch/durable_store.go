package agentdispatch

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	durablePilotSchemaVersion = 1
	maxDurablePilotRecordSize = 128 * 1024
)

// pilotRecoveryStore is the private, owner-local recovery boundary for Pilot.
// It is deliberately separate from public receipts and the execution ledger:
// packet bodies are needed to authenticate a post-restart return, but must not
// be injected into context or exposed as status metadata.
type pilotRecoveryStore struct {
	root string
}

type durablePilotRecord struct {
	SchemaVersion int                 `json:"schema_version"`
	Receipt       Receipt             `json:"receipt"`
	Packet        WorkPacket          `json:"packet"`
	Errand        *ErrandContract     `json:"errand,omitempty"`
	ErrandState   errandState         `json:"errand_state,omitempty"`
	PendingTool   *ErrandToolEnvelope `json:"pending_tool,omitempty"`
	UsedNonces    []string            `json:"used_nonces,omitempty"`
}

func newPilotRecoveryStore(root string) (*pilotRecoveryStore, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, nil
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("pilot recovery root: %w", err)
	}
	if err := rejectSymlinkAncestors(abs); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(abs, 0o700); err != nil {
		return nil, fmt.Errorf("pilot recovery root: %w", err)
	}
	info, err := os.Lstat(abs)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return nil, errors.New("pilot recovery root must be a non-symlink directory")
	}
	if err := os.Chmod(abs, 0o700); err != nil {
		return nil, fmt.Errorf("pilot recovery root permissions: %w", err)
	}
	return &pilotRecoveryStore{root: abs}, nil
}

func rejectSymlinkAncestors(path string) error {
	probe := path
	for {
		info, err := os.Lstat(probe)
		if err == nil {
			if info.Mode()&os.ModeSymlink != 0 {
				return errors.New("pilot recovery root has a symlink ancestor")
			}
			return nil
		}
		if !os.IsNotExist(err) {
			return fmt.Errorf("pilot recovery root ancestry: %w", err)
		}
		parent := filepath.Dir(probe)
		if parent == probe {
			return nil
		}
		probe = parent
	}
}

func (store *pilotRecoveryStore) path(id string) string {
	return filepath.Join(store.root, id+".json")
}

func (store *pilotRecoveryStore) write(record pilotRecord) error {
	if store == nil {
		return nil
	}
	if !validPacketID(record.receipt.DelegationID) {
		return errors.New("pilot recovery record has an invalid delegation id")
	}
	payload := durablePilotRecord{
		SchemaVersion: durablePilotSchemaVersion, Receipt: cloneReceipt(record.receipt),
		Packet: record.packet, Errand: record.errand, ErrandState: record.errandState,
		PendingTool: record.pendingTool, UsedNonces: append([]string(nil), record.consumedNonces...),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("pilot recovery record encode: %w", err)
	}
	if len(body) > maxDurablePilotRecordSize {
		return errors.New("pilot recovery record exceeds its bounded size")
	}
	path := store.path(record.receipt.DelegationID)
	tmp, err := os.CreateTemp(store.root, ".pilot-recovery-*.tmp")
	if err != nil {
		return fmt.Errorf("pilot recovery record temp: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return fmt.Errorf("pilot recovery record permissions: %w", err)
	}
	if _, err := tmp.Write(body); err != nil {
		tmp.Close()
		return fmt.Errorf("pilot recovery record write: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("pilot recovery record sync: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("pilot recovery record close: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("pilot recovery record activate: %w", err)
	}
	dir, err := os.Open(store.root)
	if err != nil {
		return fmt.Errorf("pilot recovery directory open: %w", err)
	}
	defer dir.Close()
	if err := dir.Sync(); err != nil {
		return fmt.Errorf("pilot recovery directory sync: %w", err)
	}
	return nil
}

func (store *pilotRecoveryStore) load(dispatcher *Dispatcher) (map[string]pilotRecord, map[string]bool, error) {
	records := make(map[string]pilotRecord)
	used := make(map[string]bool)
	entries, err := os.ReadDir(store.root)
	if err != nil {
		return nil, nil, fmt.Errorf("pilot recovery directory read: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".json")
		if !validPacketID(id) {
			return nil, nil, errors.New("pilot recovery directory contains an invalid record name")
		}
		path := filepath.Join(store.root, entry.Name())
		info, err := os.Lstat(path)
		if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return nil, nil, errors.New("pilot recovery record is not a regular file")
		}
		if info.Size() > maxDurablePilotRecordSize {
			return nil, nil, errors.New("pilot recovery record exceeds its bounded size")
		}
		file, err := os.Open(path)
		if err != nil {
			return nil, nil, fmt.Errorf("pilot recovery record open: %w", err)
		}
		limited := io.LimitReader(file, maxDurablePilotRecordSize+1)
		var payload durablePilotRecord
		decoder := json.NewDecoder(limited)
		decoder.DisallowUnknownFields()
		decodeErr := decoder.Decode(&payload)
		var extra struct{}
		if decodeErr == nil {
			decodeErr = decoder.Decode(&extra)
			if decodeErr == io.EOF {
				decodeErr = nil
			}
		}
		closeErr := file.Close()
		if decodeErr != nil || closeErr != nil {
			return nil, nil, errors.New("pilot recovery record is not canonical JSON")
		}
		if payload.SchemaVersion != durablePilotSchemaVersion || payload.Receipt.DelegationID != id || payload.Receipt.SchemaVersion != 1 {
			return nil, nil, errors.New("pilot recovery record schema is invalid")
		}
		record := pilotRecord{receipt: payload.Receipt, packet: payload.Packet, errand: payload.Errand,
			errandState: payload.ErrandState, pendingTool: payload.PendingTool,
			consumedNonces: append([]string(nil), payload.UsedNonces...)}
		if record.packet.PacketID != "" {
			if record.packet.PacketID != id || verifyPacketForRecovery(dispatcher, record.packet) != nil ||
				digestBody(record.packet) != record.receipt.PacketSHA256 ||
				digestBody(record.packet.DoneContract) != record.receipt.DoneContractSHA256 {
				return nil, nil, errors.New("pilot recovery packet failed authentication")
			}
			if record.receipt.TargetAgentID != record.packet.TargetAgentID || record.receipt.ScopeKind != record.packet.ScopeKind || record.receipt.ScopeID != record.packet.ScopeID {
				return nil, nil, errors.New("pilot recovery packet and receipt are inconsistent")
			}
		}
		for _, nonce := range record.consumedNonces {
			if !validPacketID(nonce) || used[nonce] {
				return nil, nil, errors.New("pilot recovery nonce state is invalid")
			}
			used[nonce] = true
		}
		records[id] = record
	}
	return records, used, nil
}

// verifyPacketForRecovery authenticates the immutable packet contract without
// treating an already-terminal receipt as corrupt merely because its original
// TTL has elapsed. Active packets are still checked against the live clock by
// verifyEnvelope before any return is accepted.
func verifyPacketForRecovery(dispatcher *Dispatcher, packet WorkPacket) error {
	copy := *dispatcher
	copy.now = func() time.Time { return packet.IssuedAt.Add(time.Nanosecond) }
	return copy.Verify(packet)
}
