package hookpolicy_test

import (
	"strings"
	"testing"

	baseruntime "github.com/agentic-os-brasil/bcg-brasil-agentic-os/bundles/base/runtime"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/hookpolicy"
)

func TestBasePolicyMakesEveryHookNonWaiting(t *testing.T) {
	policy, err := baseruntime.HookPolicy()
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range policy.Events {
		if event.MayWaitForWorker || event.MayUseNetwork || event.MayCallModel || event.MayRetry {
			t.Fatalf("event %s permits synchronous expensive work: %#v", event.Event, event)
		}
	}
}

func TestPolicyRejectsBlockingSessionStart(t *testing.T) {
	_, err := hookpolicy.Parse(strings.NewReader(`{
  "schema_version": 1,
  "events": [
    {"event":"session_start","mode":"snapshot","may_block":true,"may_wait_for_worker":false,"may_use_network":false,"may_call_model":false,"allowed_work":["read_committed_snapshot"]},
    {"event":"context_inject","mode":"snapshot","may_block":false,"may_wait_for_worker":false,"may_use_network":false,"may_call_model":false,"allowed_work":["read_committed_snapshot"]},
    {"event":"pre_action_guard","mode":"deterministic_guard","may_block":true,"may_wait_for_worker":false,"may_use_network":false,"may_call_model":false,"allowed_work":["evaluate_local_guard"]},
    {"event":"post_action_observe","mode":"signal","may_block":false,"may_wait_for_worker":false,"may_use_network":false,"may_call_model":false,"allowed_work":["emit_idempotent_signal"]},
    {"event":"stop_finalize","mode":"signal","may_block":false,"may_wait_for_worker":false,"may_use_network":false,"may_call_model":false,"allowed_work":["emit_idempotent_signal"]}
  ]
}`))
	if err == nil || !strings.Contains(err.Error(), "session_start") {
		t.Fatalf("Parse() error = %v, want session_start rejection", err)
	}
}

func TestPolicyRejectsWorkerWaitForGuard(t *testing.T) {
	_, err := hookpolicy.Parse(strings.NewReader(`{
  "schema_version": 1,
  "events": [
    {"event":"session_start","mode":"snapshot","may_block":false,"may_wait_for_worker":false,"may_use_network":false,"may_call_model":false,"allowed_work":["read_committed_snapshot"]},
    {"event":"context_inject","mode":"snapshot","may_block":false,"may_wait_for_worker":false,"may_use_network":false,"may_call_model":false,"allowed_work":["read_committed_snapshot"]},
    {"event":"pre_action_guard","mode":"deterministic_guard","may_block":true,"may_wait_for_worker":true,"may_use_network":false,"may_call_model":false,"allowed_work":["evaluate_local_guard"]},
    {"event":"post_action_observe","mode":"signal","may_block":false,"may_wait_for_worker":false,"may_use_network":false,"may_call_model":false,"allowed_work":["emit_idempotent_signal"]},
    {"event":"stop_finalize","mode":"signal","may_block":false,"may_wait_for_worker":false,"may_use_network":false,"may_call_model":false,"allowed_work":["emit_idempotent_signal"]}
  ]
}`))
	if err == nil || !strings.Contains(err.Error(), "pre_action_guard") {
		t.Fatalf("Parse() error = %v, want pre_action_guard rejection", err)
	}
}

func TestPolicyRejectsRetryPermission(t *testing.T) {
	_, err := hookpolicy.Parse(strings.NewReader(`{
  "schema_version": 1,
  "events": [
    {"event":"session_start","mode":"snapshot","may_block":false,"may_wait_for_worker":false,"may_use_network":false,"may_call_model":false,"may_retry":true,"allowed_work":["read_committed_snapshot"]},
    {"event":"context_inject","mode":"snapshot","may_block":false,"may_wait_for_worker":false,"may_use_network":false,"may_call_model":false,"may_retry":false,"allowed_work":["read_committed_snapshot"]},
    {"event":"pre_action_guard","mode":"deterministic_guard","may_block":true,"may_wait_for_worker":false,"may_use_network":false,"may_call_model":false,"may_retry":false,"allowed_work":["evaluate_local_guard"]},
    {"event":"post_action_observe","mode":"signal","may_block":false,"may_wait_for_worker":false,"may_use_network":false,"may_call_model":false,"may_retry":false,"allowed_work":["emit_idempotent_signal"]},
    {"event":"stop_finalize","mode":"signal","may_block":false,"may_wait_for_worker":false,"may_use_network":false,"may_call_model":false,"may_retry":false,"allowed_work":["emit_idempotent_signal"]}
  ]
}`))
	if err == nil || !strings.Contains(err.Error(), "may not retry") {
		t.Fatalf("Parse() error = %v, want retry rejection", err)
	}
}
