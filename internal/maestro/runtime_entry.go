package maestro

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type DispatchBoundaryReceipt struct {
	SchemaVersion        int    `json:"schema_version"`
	PlanDigest           string `json:"plan_digest"`
	ChainDigest          string `json:"chain_digest"`
	PacketDigest         string `json:"packet_digest"`
	PromptDigest         string `json:"prompt_digest"`
	DraftDigest          string `json:"draft_digest"`
	AccountConsultation  bool   `json:"account_consultation_required"`
	WalterRequired       bool   `json:"walter_required"`
	HistoryCount         int    `json:"history_count"`
	DispatchID           string `json:"dispatch_id"`
	DurableDispatchEpoch uint64 `json:"durable_dispatch_epoch"`
	BindingChainDigest   string `json:"binding_chain_digest"`
	State                Stage  `json:"state"`
	Outcome              string `json:"outcome"`
}

type DispatchRecoveryMarker struct {
	SchemaVersion int       `json:"schema_version"`
	PlanDigest    string    `json:"plan_digest"`
	PromptID      string    `json:"prompt_id,omitempty"`
	ChainPath     string    `json:"chain_path,omitempty"`
	Reason        string    `json:"reason"`
	RecordedAt    time.Time `json:"recorded_at"`
}

// DispatchBoundaryState is a metadata-only local CAS record. It proves that
// Maestro prepared and closed one dispatch boundary; it is not native-agent
// authentication and never contains credentials.
type DispatchBoundaryState struct {
	SchemaVersion      int       `json:"schema_version"`
	InstallationDigest string    `json:"installation_digest"`
	DispatchID         string    `json:"dispatch_id"`
	OwnerDigest        string    `json:"owner_digest"`
	SessionDigest      string    `json:"session_digest"`
	ReceiptName        string    `json:"receipt_name"`
	Epoch              uint64    `json:"epoch"`
	PlanDigest         string    `json:"plan_digest"`
	ChainDigest        string    `json:"chain_digest"`
	BindingChainDigest string    `json:"binding_chain_digest"`
	Status             string    `json:"status"`
	ActiveSpokeCount   int       `json:"active_spoke_count"`
	StartedAt          time.Time `json:"started_at"`
	FinishedAt         time.Time `json:"finished_at"`
	StateDigest        string    `json:"state_digest"`
}

type dispatchCurrentPointer struct {
	SchemaVersion int    `json:"schema_version"`
	Epoch         uint64 `json:"epoch"`
	ReceiptName   string `json:"receipt_name"`
	PointerDigest string `json:"pointer_digest"`
}

var syncChainDirectoryFunc = syncChainDirectory
var syncDispatchDirectoryFunc = syncChainDirectory

func PersistDispatchBoundary(root, ownerID, sessionID, dispatchID string, plan Plan, chain ChainState) (DispatchBoundaryState, error) {
	if err := plan.Validate(); err != nil {
		return DispatchBoundaryState{}, err
	}
	if chain.PlanDigest != plan.PlanDigest {
		return DispatchBoundaryState{}, errors.New("dispatch boundary chain is not bound to plan")
	}
	if err := validateDispatchIdentity(ownerID, sessionID, dispatchID); err != nil {
		return DispatchBoundaryState{}, err
	}
	bindingDigest := orderedBindingDigest(plan)
	directory := filepath.Join(root, "owner", "maestro", "dispatch")
	receiptsDirectory := filepath.Join(directory, "receipts")
	if err := ensurePrivateChainTree(root, directory); err != nil {
		return DispatchBoundaryState{}, err
	}
	if err := ensurePrivateChainTree(root, receiptsDirectory); err != nil {
		return DispatchBoundaryState{}, err
	}
	unlock, err := acquireChainLock(directory)
	if err != nil {
		return DispatchBoundaryState{}, err
	}
	defer func() { _ = unlock() }()
	chainDigestValue := chainDigest(chain)
	ownerDigest := SHA256Hex(ownerID)
	sessionDigest := SHA256Hex(sessionID)
	priorPointer, priorPointerBody, err := readDispatchCurrentPointer(directory, receiptsDirectory)
	if err != nil {
		return DispatchBoundaryState{}, err
	}
	if prior, err := findDispatchReceipt(receiptsDirectory, dispatchID); err != nil {
		return DispatchBoundaryState{}, err
	} else if prior != nil {
		if prior.InstallationDigest != installationDigest(root) || prior.OwnerDigest != ownerDigest || prior.SessionDigest != sessionDigest {
			return DispatchBoundaryState{}, errors.New("dispatch occurrence is bound to another owner or session")
		}
		if prior.PlanDigest != plan.PlanDigest || prior.ChainDigest != chainDigestValue || prior.BindingChainDigest != bindingDigest || prior.Status != "finished" {
			return DispatchBoundaryState{}, errors.New("dispatch occurrence was reused with different content")
		}
		if priorPointer == nil || priorPointer.Epoch < prior.Epoch {
			repairedPointer := newDispatchCurrentPointer(*prior)
			if err := persistDispatchCurrentPointer(directory, filepath.Join(directory, "current.json"), repairedPointer, priorPointerBody); err != nil {
				return DispatchBoundaryState{}, fmt.Errorf("dispatch occurrence exists but current pointer recovery failed: %w", err)
			}
		} else if priorPointer.Epoch == prior.Epoch && priorPointer.ReceiptName != prior.ReceiptName {
			return DispatchBoundaryState{}, errors.New("durable dispatch current pointer conflicts with occurrence receipt")
		}
		return *prior, nil
	}
	epoch := uint64(1)
	if priorPointer != nil {
		priorState, err := readDispatchReceipt(receiptsDirectory, priorPointer.ReceiptName)
		if err != nil {
			return DispatchBoundaryState{}, err
		}
		if priorState.InstallationDigest != installationDigest(root) || priorState.ActiveSpokeCount != 0 || priorState.Status != "finished" {
			return DispatchBoundaryState{}, errors.New("durable dispatch current receipt is invalid")
		}
		epoch = priorPointer.Epoch + 1
	}
	state := newDispatchBoundaryState(root, ownerID, sessionID, dispatchID, epoch, plan, chain, bindingDigest)
	receiptPath := filepath.Join(receiptsDirectory, state.ReceiptName)
	if err := persistDispatchReceipt(receiptsDirectory, receiptPath, state); err != nil {
		return DispatchBoundaryState{}, err
	}
	pointer := newDispatchCurrentPointer(state)
	if err := persistDispatchCurrentPointer(directory, filepath.Join(directory, "current.json"), pointer, priorPointerBody); err != nil {
		_ = PersistDispatchRecoveryMarker(root, DispatchRecoveryMarker{SchemaVersion: 1, PlanDigest: state.PlanDigest, ChainPath: receiptPath, Reason: "dispatch current pointer durability failure"})
		return DispatchBoundaryState{}, err
	}
	return state, nil
}

func validateDispatchIdentity(ownerID, sessionID, dispatchID string) error {
	if ownerID == "" || sessionID == "" || len(ownerID) > 128 || len(sessionID) > 128 || strings.ContainsAny(ownerID+sessionID, `/\\`) {
		return errors.New("dispatch owner or session identity is invalid")
	}
	if !validClosedDispatchID(dispatchID) {
		return errors.New("dispatch occurrence identity is invalid")
	}
	return nil
}

func validClosedDispatchID(value string) bool {
	if len(value) == 64 {
		for _, char := range value {
			if !(char >= '0' && char <= '9' || char >= 'a' && char <= 'f') {
				return false
			}
		}
		return true
	}
	if len(value) < 1 || len(value) > 128 || value == "." || value == ".." {
		return false
	}
	for index, char := range value {
		if !(char >= 'a' && char <= 'z' || char >= '0' && char <= '9' || char == '-' || char == '_' || char == '.') || index == 0 && char == '.' {
			return false
		}
	}
	return true
}

func validLowerDigest(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, char := range value {
		if !(char >= '0' && char <= '9' || char >= 'a' && char <= 'f') {
			return false
		}
	}
	return true
}

func orderedBindingDigest(plan Plan) string {
	body, _ := json.Marshal(plan.Bindings)
	return SHA256Hex(string(body))
}

func chainDigest(chain ChainState) string {
	body, _ := json.Marshal(chain)
	return SHA256Hex(string(body))
}

func installationDigest(root string) string {
	abs, _ := filepath.Abs(filepath.Clean(root))
	return SHA256Hex(abs)
}

func newDispatchBoundaryState(root, ownerID, sessionID, dispatchID string, epoch uint64, plan Plan, chain ChainState, bindingDigest string) DispatchBoundaryState {
	now := time.Now().UTC()
	state := DispatchBoundaryState{SchemaVersion: 1, InstallationDigest: installationDigest(root), DispatchID: dispatchID, OwnerDigest: SHA256Hex(ownerID), SessionDigest: SHA256Hex(sessionID), ReceiptName: fmt.Sprintf("%020d-%s.json", epoch, dispatchID), Epoch: epoch, PlanDigest: plan.PlanDigest, ChainDigest: chainDigest(chain), BindingChainDigest: bindingDigest, Status: "finished", ActiveSpokeCount: 0, StartedAt: now, FinishedAt: now}
	state.StateDigest = dispatchBoundaryStateDigest(state)
	return state
}

func validDispatchBoundaryState(state DispatchBoundaryState) bool {
	return state.SchemaVersion == 1 && state.Epoch > 0 && state.DispatchID != "" && safeReceiptName(state.ReceiptName) && validLowerDigest(state.InstallationDigest) && validLowerDigest(state.OwnerDigest) && validLowerDigest(state.SessionDigest) && validLowerDigest(state.PlanDigest) && validLowerDigest(state.ChainDigest) && validLowerDigest(state.BindingChainDigest) && state.Status == "finished" && state.ActiveSpokeCount == 0 && !state.StartedAt.IsZero() && !state.FinishedAt.IsZero() && state.StateDigest == dispatchBoundaryStateDigest(state)
}

func dispatchBoundaryStateDigest(state DispatchBoundaryState) string {
	digestValue := state.StateDigest
	state.StateDigest = ""
	body, _ := json.Marshal(state)
	state.StateDigest = digestValue
	return SHA256Hex(string(body))
}

func newDispatchCurrentPointer(state DispatchBoundaryState) dispatchCurrentPointer {
	pointer := dispatchCurrentPointer{SchemaVersion: 1, Epoch: state.Epoch, ReceiptName: state.ReceiptName}
	pointer.PointerDigest = dispatchCurrentPointerDigest(pointer)
	return pointer
}

func dispatchCurrentPointerDigest(pointer dispatchCurrentPointer) string {
	digest := pointer.PointerDigest
	pointer.PointerDigest = ""
	body, _ := json.Marshal(pointer)
	pointer.PointerDigest = digest
	return SHA256Hex(string(body))
}

func validDispatchCurrentPointer(pointer dispatchCurrentPointer) bool {
	return pointer.SchemaVersion == 1 && pointer.Epoch > 0 && safeReceiptName(pointer.ReceiptName) && pointer.PointerDigest == dispatchCurrentPointerDigest(pointer)
}

func safeReceiptName(name string) bool {
	if len(name) < 27 || filepath.Base(name) != name || !strings.HasSuffix(name, ".json") || name[20] != '-' {
		return false
	}
	for _, char := range name[:20] {
		if char < '0' || char > '9' {
			return false
		}
	}
	return validClosedDispatchID(strings.TrimSuffix(name[21:], ".json"))
}

func readDispatchCurrentPointer(directory, receiptsDirectory string) (*dispatchCurrentPointer, []byte, error) {
	path := filepath.Join(directory, "current.json")
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, nil, errors.New("durable dispatch current pointer is not a regular private file")
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	var pointer dispatchCurrentPointer
	if err := json.Unmarshal(body, &pointer); err != nil || !validDispatchCurrentPointer(pointer) {
		return nil, nil, errors.New("durable dispatch current pointer is invalid")
	}
	if _, err := readDispatchReceipt(receiptsDirectory, pointer.ReceiptName); err != nil {
		return nil, nil, err
	}
	return &pointer, append([]byte(nil), body...), nil
}

func readDispatchReceipt(directory, name string) (DispatchBoundaryState, error) {
	if !safeReceiptName(name) {
		return DispatchBoundaryState{}, errors.New("durable dispatch receipt name is invalid")
	}
	path := filepath.Join(directory, name)
	info, err := os.Lstat(path)
	if err != nil {
		return DispatchBoundaryState{}, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return DispatchBoundaryState{}, errors.New("durable dispatch receipt is not a regular private file")
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return DispatchBoundaryState{}, err
	}
	var state DispatchBoundaryState
	if json.Unmarshal(body, &state) != nil || !validDispatchBoundaryState(state) || state.ReceiptName != name {
		return DispatchBoundaryState{}, errors.New("durable dispatch receipt is invalid")
	}
	return state, nil
}

func findDispatchReceipt(directory, dispatchID string) (*DispatchBoundaryState, error) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if entry.IsDir() || !safeReceiptName(entry.Name()) {
			continue
		}
		state, err := readDispatchReceipt(directory, entry.Name())
		if err != nil {
			return nil, err
		}
		if state.DispatchID == dispatchID {
			return &state, nil
		}
	}
	return nil, nil
}

func persistDispatchReceipt(directory, path string, state DispatchBoundaryState) error {
	body, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if _, err := file.Write(append(body, '\n')); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return syncDispatchDirectoryFunc(directory)
}

func persistDispatchCurrentPointer(directory, path string, pointer dispatchCurrentPointer, previous []byte) error {
	body, err := json.MarshalIndent(pointer, "", "  ")
	if err != nil {
		return err
	}
	write := func(contents []byte) error {
		temporary, err := os.CreateTemp(directory, ".dispatch-current-*")
		if err != nil {
			return err
		}
		temporaryPath := temporary.Name()
		defer os.Remove(temporaryPath)
		if err := temporary.Chmod(0o600); err != nil {
			_ = temporary.Close()
			return err
		}
		if _, err := temporary.Write(append(contents, '\n')); err != nil {
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
		return os.Rename(temporaryPath, path)
	}
	if err := write(body); err != nil {
		return err
	}
	if err := syncDispatchDirectoryFunc(directory); err != nil {
		if len(previous) > 0 {
			if restoreErr := write([]byte(strings.TrimSuffix(string(previous), "\n"))); restoreErr != nil {
				return fmt.Errorf("dispatch pointer durability failed and previous pointer restore failed: %w", err)
			} else if restoreErr = syncDispatchDirectoryFunc(directory); restoreErr != nil {
				return fmt.Errorf("dispatch pointer durability failed; previous pointer restore fsync failed: %v", restoreErr)
			}
		} else {
			_ = os.Remove(path)
			_ = syncDispatchDirectoryFunc(directory)
		}
		return fmt.Errorf("dispatch pointer durability failed: %w", err)
	}
	return nil
}

func PersistChainState(root string, state ChainState) (string, error) {
	if !validLowerDigest(state.PlanDigest) {
		return "", errors.New("cannot persist a chain without a plan digest")
	}
	body, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return "", err
	}
	storedBody := append(append([]byte(nil), body...), '\n')
	directory := filepath.Join(root, "owner", "maestro", "chains")
	if err := ensurePrivateChainTree(root, directory); err != nil {
		return "", err
	}
	path := filepath.Join(directory, state.PlanDigest+".json")
	unlock, err := acquireChainLock(directory)
	if err != nil {
		return "", err
	}
	defer func() { _ = unlock() }()
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return "", errors.New("chain state target is not a regular private file")
		}
		current, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		if string(current) == string(storedBody) {
			return path, nil
		}
		return "", errors.New("chain state conflict for plan digest")
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	temporary, err := os.CreateTemp(directory, ".chain-*")
	if err != nil {
		return "", err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return "", err
	}
	if _, err := temporary.Write(storedBody); err != nil {
		_ = temporary.Close()
		return "", err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return "", err
	}
	if err := temporary.Close(); err != nil {
		return "", err
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return "", err
	}
	if err := syncChainDirectoryFunc(directory); err != nil {
		cleanupErr := os.Remove(path)
		if cleanupErr == nil {
			cleanupErr = syncChainDirectoryFunc(directory)
		}
		if cleanupErr != nil {
			markerErr := PersistDispatchRecoveryMarker(root, DispatchRecoveryMarker{SchemaVersion: 1, PlanDigest: state.PlanDigest, ChainPath: path, Reason: "chain post-rename durability failure"})
			if markerErr != nil {
				return "", fmt.Errorf("chain durability failed: %v; cleanup failed: %v; recovery marker failed: %w", err, cleanupErr, markerErr)
			}
			return "", fmt.Errorf("chain durability failed; recovery marker recorded: %w", err)
		}
		return "", fmt.Errorf("chain durability failed after rename; chain cleanup completed: %w", err)
	}
	return path, nil
}

func RemoveChainState(root string, state ChainState) error {
	if !validLowerDigest(state.PlanDigest) {
		return errors.New("cannot remove a chain without a plan digest")
	}
	directory := filepath.Join(root, "owner", "maestro", "chains")
	if err := ensurePrivateChainTree(root, directory); err != nil {
		return err
	}
	unlock, err := acquireChainLock(directory)
	if err != nil {
		return err
	}
	defer func() { _ = unlock() }()
	path := filepath.Join(directory, state.PlanDigest+".json")
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return errors.New("chain state target is not a regular private file")
	}
	if err := os.Remove(path); err != nil {
		return err
	}
	return syncChainDirectoryFunc(directory)
}

func PersistDispatchRecoveryMarker(root string, marker DispatchRecoveryMarker) error {
	if !validLowerDigest(marker.PlanDigest) || marker.Reason == "" {
		return errors.New("dispatch recovery marker is incomplete")
	}
	directory := filepath.Join(root, "owner", "maestro", "recovery")
	if err := ensurePrivateChainTree(root, directory); err != nil {
		return err
	}
	if marker.RecordedAt.IsZero() {
		marker.RecordedAt = time.Now().UTC()
	}
	body, err := json.MarshalIndent(marker, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(directory, marker.PlanDigest+".json")
	unlock, err := acquireChainLock(directory)
	if err != nil {
		return err
	}
	defer func() { _ = unlock() }()
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return errors.New("dispatch recovery marker target is not a regular private file")
		}
		existing, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var prior DispatchRecoveryMarker
		if json.Unmarshal(existing, &prior) == nil && prior.PlanDigest == marker.PlanDigest && prior.PromptID == marker.PromptID && prior.ChainPath == marker.ChainPath && prior.Reason == marker.Reason {
			return nil
		}
		return errors.New("unresolved dispatch recovery marker already exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	temporary, err := os.CreateTemp(directory, ".recovery-*")
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
	if err := os.Rename(temporaryPath, path); err != nil {
		return err
	}
	return syncChainDirectoryFunc(directory)
}

func ensurePrivateChainTree(root, directory string) error {
	root, err := filepath.Abs(filepath.Clean(root))
	if err != nil {
		return err
	}
	rootInfo, err := os.Lstat(root)
	if err != nil {
		return err
	}
	if rootInfo.Mode()&os.ModeSymlink != 0 || !rootInfo.IsDir() {
		return errors.New("chain state root is not a private directory")
	}
	relative, err := filepath.Rel(root, directory)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return errors.New("chain state directory escapes owner root")
	}
	current := root
	for _, part := range splitRelativePath(relative) {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			if err := os.Mkdir(current, 0o700); err != nil {
				return err
			}
			info, err = os.Lstat(current)
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return errors.New("chain state parent is not a private directory")
		}
	}
	return nil
}

func splitRelativePath(value string) []string {
	if value == "." || value == "" {
		return nil
	}
	parts := []string{}
	for value != "." && value != string(filepath.Separator) {
		parts = append([]string{filepath.Base(value)}, parts...)
		value = filepath.Dir(value)
	}
	return parts
}

func acquireChainLock(directory string) (func() error, error) {
	lockPath := filepath.Join(directory, ".lock")
	deadline := time.Now().Add(2 * time.Second)
	for {
		before, err := os.Lstat(lockPath)
		missing := errors.Is(err, os.ErrNotExist)
		if err != nil && !missing {
			return nil, err
		}
		if !missing && (before.Mode()&os.ModeSymlink != 0 || !before.Mode().IsRegular()) {
			return nil, errors.New("chain state lock is not a private regular file")
		}
		flags := os.O_RDWR
		if missing {
			flags |= os.O_CREATE | os.O_EXCL
		}
		file, err := os.OpenFile(lockPath, flags, 0o600)
		if errors.Is(err, os.ErrExist) || errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err == nil {
			opened, statErr := file.Stat()
			if statErr != nil {
				_ = file.Close()
				return nil, statErr
			}
			after, statErr := os.Lstat(lockPath)
			if statErr != nil || after.Mode()&os.ModeSymlink != 0 || !after.Mode().IsRegular() || !os.SameFile(opened, after) {
				_ = file.Close()
				return nil, errors.New("chain state lock changed during secure open")
			}
			if err := tryLockDispatchFile(file); err == nil {
				return func() error {
					unlockErr := unlockDispatchFile(file)
					closeErr := file.Close()
					if unlockErr != nil {
						return unlockErr
					}
					return closeErr
				}, nil
			} else if !errors.Is(err, errDispatchLockBusy) {
				_ = file.Close()
				return nil, err
			}
			_ = file.Close()
		} else {
			return nil, err
		}
		if time.Now().After(deadline) {
			return nil, errors.New("chain state lock is busy")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func syncChainDirectory(directory string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	file, err := os.Open(directory)
	if err != nil {
		return err
	}
	defer file.Close()
	return file.Sync()
}
