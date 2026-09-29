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

// RuntimeConfig mirrors runtime.json, which is decoded with
// DisallowUnknownFields: every key in the file must appear here, including the
// documentation ones, or the whole decode fails.
//
// Schema 2 moved the session-context caps from "runes" to bytes. The v1 caps
// were never read by any code — they were a declaration without enforcement —
// and the proof is in their values: the lifetime cap was 1024 against a real
// measured 3197, three times under, which nothing ever noticed because nothing
// ever truncated. They are kept in the file under SupersededRunes as a record
// of the original design intent, and parsed here only so the decode succeeds.
type RuntimeConfig struct {
	SchemaVersion     int    `json:"schema_version"`
	Comment           string `json:"_comment"`
	Calibracao        string `json:"_calibracao"`
	L1MaxRunes        int    `json:"l1_max_runes"`
	L1MaxEntries      int    `json:"l1_max_entries"`
	L1MaxInputBytes   int    `json:"l1_max_input_bytes"`
	L1MaxInputEntries int    `json:"l1_max_input_entries"`

	SessionContextSelfMaxBytes      int `json:"session_context_self_max_bytes"`
	SessionContextLifetimeMaxBytes  int `json:"session_context_lifetime_max_bytes"`
	SessionContextL3MaxBytes        int `json:"session_context_l3_max_bytes"`
	SessionContextL2MaxBytes        int `json:"session_context_l2_max_bytes"`
	SessionContextL1MaxBytes        int `json:"session_context_l1_max_bytes"`
	SessionContextLearningsMaxBytes int `json:"session_context_learnings_max_bytes"`
	SessionContextCraftMaxBytes     int `json:"session_context_craft_max_bytes"`
	SessionContextTotalMaxBytes     int `json:"session_context_total_max_bytes"`

	Measured2026_09_05 map[string]any `json:"_measured_2026_09_05"`
	SupersededRunes    map[string]any `json:"_superseded_runes"`
}

// ContextBudgets returns the per-block caps the SessionStart hook enforces.
// The keys are the block names the hook uses, not the JSON field names.
func (config RuntimeConfig) ContextBudgets() map[string]int {
	return map[string]int{
		"self":      config.SessionContextSelfMaxBytes,
		"lifetime":  config.SessionContextLifetimeMaxBytes,
		"L3":        config.SessionContextL3MaxBytes,
		"L2":        config.SessionContextL2MaxBytes,
		"L1":        config.SessionContextL1MaxBytes,
		"learnings": config.SessionContextLearningsMaxBytes,
		"craft":     config.SessionContextCraftMaxBytes,
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

	// Every block cap must be a real bound, and the total must be able to hold
	// at least the largest single block — otherwise that block's cap could
	// never be reached and the number would be decoration.
	//
	// The sum of the block caps is deliberately NOT required to fit inside the
	// total: they are per-block maxima, not reservations. Today they sum to
	// 18,900 against a total of 18,000, while the measured reality is 15,589.
	// The hook enforces both independently — each block is cut at its own cap,
	// then the envelope is checked against the total — so allowing any single
	// block to grow without shrinking every other one is the point.
	contextBudgetsValid := true
	largestBlock := 0
	for _, budget := range config.ContextBudgets() {
		if budget < 128 || budget > 16384 {
			contextBudgetsValid = false
		}
		if budget > largestBlock {
			largestBlock = budget
		}
	}
	if config.SessionContextTotalMaxBytes < 1024 || config.SessionContextTotalMaxBytes > 65536 {
		contextBudgetsValid = false
	}
	if config.SessionContextTotalMaxBytes < largestBlock {
		contextBudgetsValid = false
	}

	if config.SchemaVersion != 2 ||
		config.L1MaxRunes < 1024 || config.L1MaxRunes > 65536 ||
		config.L1MaxEntries < 1 || config.L1MaxEntries > 256 ||
		config.L1MaxInputBytes < config.L1MaxRunes || config.L1MaxInputBytes > 4<<20 ||
		config.L1MaxInputEntries < config.L1MaxEntries || config.L1MaxInputEntries > 1024 ||
		!contextBudgetsValid {
		return RuntimeConfig{}, errors.New("memory runtime config is outside bounded limits")
	}
	return config, nil
}
