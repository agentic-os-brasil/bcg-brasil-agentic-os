package maestro

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

type DispatchBoundaryReceipt struct {
	SchemaVersion       int    `json:"schema_version"`
	PlanDigest          string `json:"plan_digest"`
	ChainDigest         string `json:"chain_digest"`
	PacketDigest        string `json:"packet_digest"`
	PromptDigest        string `json:"prompt_digest"`
	DraftDigest         string `json:"draft_digest"`
	AccountConsultation bool   `json:"account_consultation_required"`
	WalterRequired      bool   `json:"walter_required"`
	HistoryCount        int    `json:"history_count"`
	State               Stage  `json:"state"`
	Outcome             string `json:"outcome"`
}

func PersistChainState(root string, state ChainState) (string, error) {
	if state.PlanDigest == "" {
		return "", errors.New("cannot persist a chain without a plan digest")
	}
	body, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return "", err
	}
	directory := filepath.Join(root, "owner", "maestro", "chains")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return "", err
	}
	path := filepath.Join(directory, state.PlanDigest+".json")
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
	if _, err := temporary.Write(append(body, '\n')); err != nil {
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
	return path, nil
}
