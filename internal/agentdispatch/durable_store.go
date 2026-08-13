package agentdispatch

import (
	"bytes"
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
	durablePilotSchemaVersion      = 1
	maxDurablePilotRecordSize      = 128 * 1024
	durablePilotTransactionName    = ".pilot-recovery-transaction.json"
	maxDurablePilotTransactionSize = maxDurablePilotRecordSize*2 + 4096
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

func (store *pilotRecoveryStore) transactionPath() string {
	return filepath.Join(store.root, durablePilotTransactionName)
}

type durablePilotTransaction struct {
	SchemaVersion int                  `json:"schema_version"`
	Records       []durablePilotRecord `json:"records"`
}

func encodeStrictJSON(value any, maximum int) ([]byte, error) {
	body, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	if len(body) > maximum {
		return nil, errors.New("durable recovery payload exceeds its bounded size")
	}
	return body, nil
}

func decodeStrictJSON(body []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra json.RawMessage
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return errors.New("durable recovery payload contains trailing JSON")
		}
		return err
	}
	return nil
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
	return syncDirectory(store.root)
}

// writeBatch uses a small owner-local journal so a Walter leaf and its producer
// projection are recovered together after a process or filesystem interruption.
// The journal is replayed before normal records are loaded and is removed only
// after every record has been durably written.
func (store *pilotRecoveryStore) writeBatch(records ...pilotRecord) error {
	if len(records) == 0 {
		return nil
	}
	payload := durablePilotTransaction{SchemaVersion: durablePilotSchemaVersion}
	for _, record := range records {
		encoded := durablePilotRecord{
			SchemaVersion: durablePilotSchemaVersion,
			Receipt:       cloneReceipt(record.receipt),
			Packet:        record.packet,
			Errand:        record.errand,
			ErrandState:   record.errandState,
			PendingTool:   record.pendingTool,
			UsedNonces:    append([]string(nil), record.consumedNonces...),
		}
		payload.Records = append(payload.Records, encoded)
	}
	body, err := encodeStrictJSON(payload, maxDurablePilotTransactionSize)
	if err != nil {
		return fmt.Errorf("pilot recovery transaction encode: %w", err)
	}
	if err := writeDurableFile(store.transactionPath(), body); err != nil {
		return err
	}
	for _, record := range records {
		if err := store.write(record); err != nil {
			return err
		}
	}
	if err := os.Remove(store.transactionPath()); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("pilot recovery transaction cleanup: %w", err)
	}
	return syncDirectory(store.root)
}

func writeDurableFile(path string, body []byte) error {
	directory := filepath.Dir(path)
	tmp, err := os.CreateTemp(directory, ".pilot-recovery-*.tmp")
	if err != nil {
		return fmt.Errorf("pilot recovery temp: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return fmt.Errorf("pilot recovery temp permissions: %w", err)
	}
	if _, err := tmp.Write(body); err != nil {
		tmp.Close()
		return fmt.Errorf("pilot recovery temp write: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("pilot recovery temp sync: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("pilot recovery temp close: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("pilot recovery file activate: %w", err)
	}
	return syncDirectory(directory)
}

func (store *pilotRecoveryStore) replayTransaction(dispatcher *Dispatcher) error {
	body, err := os.ReadFile(store.transactionPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("pilot recovery transaction read: %w", err)
	}
	if len(body) > maxDurablePilotTransactionSize {
		return errors.New("pilot recovery transaction exceeds its bounded size")
	}
	var transaction durablePilotTransaction
	if err := decodeStrictJSON(body, &transaction); err != nil || transaction.SchemaVersion != durablePilotSchemaVersion || len(transaction.Records) == 0 || len(transaction.Records) > 2 {
		return errors.New("pilot recovery transaction is invalid")
	}
	records := make([]pilotRecord, 0, len(transaction.Records))
	for _, payload := range transaction.Records {
		record, err := hydrateDurableRecord(payload, dispatcher)
		if err != nil {
			return err
		}
		records = append(records, record)
	}
	for _, record := range records {
		if err := store.write(record); err != nil {
			return err
		}
	}
	if err := os.Remove(store.transactionPath()); err != nil {
		return fmt.Errorf("pilot recovery transaction cleanup: %w", err)
	}
	return syncDirectory(store.root)
}

func (store *pilotRecoveryStore) load(dispatcher *Dispatcher) (map[string]pilotRecord, map[string]bool, error) {
	records := make(map[string]pilotRecord)
	used := make(map[string]bool)
	if err := store.replayTransaction(dispatcher); err != nil {
		return nil, nil, err
	}
	entries, err := os.ReadDir(store.root)
	if err != nil {
		return nil, nil, fmt.Errorf("pilot recovery directory read: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") || entry.Name() == durablePilotTransactionName {
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
		body, readErr := io.ReadAll(limited)
		closeErr := file.Close()
		if readErr != nil || closeErr != nil || len(body) > maxDurablePilotRecordSize || decodeStrictJSON(body, &payload) != nil {
			return nil, nil, errors.New("pilot recovery record is not canonical JSON")
		}
		record, err := hydrateDurableRecord(payload, dispatcher)
		if err != nil || record.receipt.DelegationID != id {
			return nil, nil, errors.New("pilot recovery record schema or authentication is invalid")
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

func hydrateDurableRecord(payload durablePilotRecord, dispatcher *Dispatcher) (pilotRecord, error) {
	if payload.SchemaVersion != durablePilotSchemaVersion || payload.Receipt.SchemaVersion != 1 || !validPacketID(payload.Receipt.DelegationID) ||
		payload.Receipt.OwnerAgentID != "maestro" || payload.Receipt.Runtime == "" || !validState(payload.Receipt.State) {
		return pilotRecord{}, errors.New("pilot recovery record schema or receipt invariants are invalid")
	}
	record := pilotRecord{receipt: payload.Receipt, packet: payload.Packet, errand: payload.Errand,
		errandState: payload.ErrandState, pendingTool: payload.PendingTool,
		consumedNonces: append([]string(nil), payload.UsedNonces...)}
	if record.packet.PacketID == "" {
		if record.receipt.State != StateFailed && record.receipt.State != StateUnavailable {
			return pilotRecord{}, errors.New("pilot recovery record without a packet has an invalid state")
		}
		return record, nil
	}
	if record.packet.PacketID != record.receipt.DelegationID || verifyPacketForRecovery(dispatcher, record.packet) != nil ||
		digestBody(record.packet) != record.receipt.PacketSHA256 || digestBody(record.packet.DoneContract) != record.receipt.DoneContractSHA256 ||
		record.receipt.TargetAgentID != record.packet.TargetAgentID || record.receipt.ScopeKind != record.packet.ScopeKind || record.receipt.ScopeID != record.packet.ScopeID {
		return pilotRecord{}, errors.New("pilot recovery packet failed authentication")
	}
	return record, nil
}

func validState(state State) bool {
	switch state {
	case StateDelegated, StatePendingReview, StateCompleted, StateFailed, StateUnavailable:
		return true
	default:
		return false
	}
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
