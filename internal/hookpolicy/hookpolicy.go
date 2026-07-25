// Package hookpolicy validates the bounded execution contract for future
// product lifecycle adapters. It deliberately describes what a hook may do,
// before any native hook is installed.
package hookpolicy

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

type Policy struct {
	SchemaVersion int           `json:"schema_version"`
	Events        []EventPolicy `json:"events"`
}

type EventPolicy struct {
	Event            string   `json:"event"`
	Mode             string   `json:"mode"`
	MayBlock         bool     `json:"may_block"`
	MayWaitForWorker bool     `json:"may_wait_for_worker"`
	MayUseNetwork    bool     `json:"may_use_network"`
	MayCallModel     bool     `json:"may_call_model"`
	AllowedWork      []string `json:"allowed_work"`
}

type expectedEvent struct {
	mode        string
	mayBlock    bool
	allowedWork string
}

var expectedEvents = map[string]expectedEvent{
	"session_start":       {mode: "snapshot", mayBlock: false, allowedWork: "read_committed_snapshot"},
	"context_inject":      {mode: "snapshot", mayBlock: false, allowedWork: "read_committed_snapshot"},
	"pre_action_guard":    {mode: "deterministic_guard", mayBlock: true, allowedWork: "evaluate_local_guard"},
	"post_action_observe": {mode: "signal", mayBlock: false, allowedWork: "emit_idempotent_signal"},
	"stop_finalize":       {mode: "signal", mayBlock: false, allowedWork: "emit_idempotent_signal"},
}

func Parse(reader io.Reader) (Policy, error) {
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	var policy Policy
	if err := decoder.Decode(&policy); err != nil {
		return Policy{}, err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return Policy{}, errors.New("hook execution policy contains multiple JSON values")
		}
		return Policy{}, err
	}
	if err := policy.Validate(); err != nil {
		return Policy{}, err
	}
	return policy, nil
}

func (policy Policy) Validate() error {
	if policy.SchemaVersion != 1 {
		return fmt.Errorf("hook execution policy schema version %d is unsupported", policy.SchemaVersion)
	}
	if len(policy.Events) != len(expectedEvents) {
		return fmt.Errorf("hook execution policy has %d events, want %d", len(policy.Events), len(expectedEvents))
	}
	seen := map[string]bool{}
	for _, event := range policy.Events {
		expected, exists := expectedEvents[event.Event]
		if !exists || seen[event.Event] {
			return fmt.Errorf("invalid or duplicate hook event %q", event.Event)
		}
		seen[event.Event] = true
		if event.Mode != expected.mode || event.MayBlock != expected.mayBlock {
			return fmt.Errorf("hook event %s has invalid execution mode", event.Event)
		}
		if event.MayWaitForWorker || event.MayUseNetwork || event.MayCallModel {
			return fmt.Errorf("hook event %s may not wait for a worker, use network, or call a model", event.Event)
		}
		if len(event.AllowedWork) != 1 || event.AllowedWork[0] != expected.allowedWork {
			return fmt.Errorf("hook event %s has invalid allowed work", event.Event)
		}
	}
	return nil
}
