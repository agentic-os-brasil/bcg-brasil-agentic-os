package basememory

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"io"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/memory"
)

//go:embed policy.json
var policyJSON []byte

//go:embed runtime.json
var runtimeJSON []byte

type RuntimeConfig struct {
	SchemaVersion                  int `json:"schema_version"`
	L1MaxRunes                     int `json:"l1_max_runes"`
	L1MaxEntries                   int `json:"l1_max_entries"`
	L1MaxInputBytes                int `json:"l1_max_input_bytes"`
	L1MaxInputEntries              int `json:"l1_max_input_entries"`
	SessionContextLifetimeMaxRunes int `json:"session_context_lifetime_max_runes"`
	SessionContextL3MaxRunes       int `json:"session_context_l3_max_runes"`
	SessionContextL2MaxRunes       int `json:"session_context_l2_max_runes"`
	SessionContextL1MaxRunes       int `json:"session_context_l1_max_runes"`
}

func (config RuntimeConfig) ContextBudgets() map[string]int {
	return map[string]int{
		"lifetime": config.SessionContextLifetimeMaxRunes,
		"L3":       config.SessionContextL3MaxRunes,
		"L2":       config.SessionContextL2MaxRunes,
		"L1":       config.SessionContextL1MaxRunes,
	}
}

func Policy() (memory.Policy, error) {
	return memory.Load(bytes.NewReader(policyJSON))
}

func Runtime() (RuntimeConfig, error) {
	decoder := json.NewDecoder(bytes.NewReader(runtimeJSON))
	decoder.DisallowUnknownFields()
	var config RuntimeConfig
	if err := decoder.Decode(&config); err != nil {
		return RuntimeConfig{}, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return RuntimeConfig{}, errors.New("memory runtime config contains multiple JSON values")
		}
		return RuntimeConfig{}, err
	}
	contextTotal := 0
	contextBudgetsValid := true
	for _, budget := range config.ContextBudgets() {
		if budget < 128 || budget > 2048 {
			contextBudgetsValid = false
		}
		contextTotal += budget
	}
	if config.SchemaVersion != 1 || config.L1MaxRunes < 1024 || config.L1MaxRunes > 65536 || config.L1MaxEntries < 1 || config.L1MaxEntries > 256 || config.L1MaxInputBytes < config.L1MaxRunes || config.L1MaxInputBytes > 4<<20 || config.L1MaxInputEntries < config.L1MaxEntries || config.L1MaxInputEntries > 1024 || !contextBudgetsValid || contextTotal > 4096 {
		return RuntimeConfig{}, errors.New("memory runtime config is outside bounded limits")
	}
	return config, nil
}
