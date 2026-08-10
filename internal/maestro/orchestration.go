package maestro

import "errors"

// AgentEvent is the metadata-only return from one bounded agent invocation.
// The model body never enters this contract; ContentDigest binds the event to
// the result that the native adapter already received.
type AgentEvent struct {
	AgentID       string               `json:"agent_id"`
	Decision      string               `json:"decision"`
	ContentDigest string               `json:"content_digest,omitempty"`
	ReasonCode    string               `json:"reason_code,omitempty"`
	IntentReceipt *IntentReviewReceipt `json:"intent_receipt,omitempty"`
}

const MaximumAgentEvents = 8

// ExecuteAgentEvents applies a complete, ordered set of agent returns to one
// plan. It is deliberately runtime-neutral: Claude/Codex adapters obtain the
// event and this function is the single deterministic transition authority.
// An empty event list is not execution; callers must keep that route
// explicitly unavailable rather than claiming that a model call happened.
func ExecuteAgentEvents(plan Plan, events []AgentEvent, policy LoopPolicy) (ChainState, error) {
	if len(events) == 0 {
		return ChainState{}, errors.New("agent orchestration requires at least one bounded agent event")
	}
	if len(events) > MaximumAgentEvents {
		return ChainState{}, errors.New("agent orchestration exceeds the bounded event budget")
	}
	state, err := NewChain(plan, policy)
	if err != nil {
		return ChainState{}, err
	}
	for _, event := range events {
		state, _, err = state.Advance(plan, policy, "maestro", Event{
			AgentID: event.AgentID, Decision: event.Decision,
			ContentDigest: event.ContentDigest, ReasonCode: event.ReasonCode,
			IntentReceipt: event.IntentReceipt,
		})
		if err != nil {
			return ChainState{}, err
		}
	}
	return state, nil
}
