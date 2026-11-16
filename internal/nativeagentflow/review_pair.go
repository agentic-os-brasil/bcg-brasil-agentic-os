package nativeagentflow

import "github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/claudeagents"

// ReviewPair has exactly one slot per axis, ordered Spec then Standards. It is
// optional so existing schema-2 sequential state retains its exact authority.
type ReviewPair struct {
	Slots []ReviewWorker `json:"slots"`
}
type ReviewWorker struct {
	Dispatch string `json:"dispatch,omitempty"`
	Agent    string `json:"agent,omitempty"`
	Stopped  bool   `json:"stopped,omitempty"`
}

func reviewSlot(target string) int {
	if target == "repository-spec-reviewer" {
		return 0
	}
	if target == "repository-standards-reviewer" {
		return 1
	}
	return -1
}
func (pair *ReviewPair) unresolved() bool {
	return pair != nil && (!pair.Slots[0].Stopped || !pair.Slots[1].Stopped)
}
func (pair *ReviewPair) validate(state *State, identities map[string]bool) error {
	if pair == nil {
		return nil
	}
	if len(pair.Slots) != 2 {
		return ErrTransition
	}
	if pair.unresolved() && (state.PendingTool != "" || state.ActiveAgentID != "" || state.Finalized || state.SystemHealthRoute || (state.AccountRoute && (!state.CaseStopObserved || !state.AccountReturnObserved))) {
		return ErrTransition
	}
	nonempty := false
	used := map[string]bool{}
	for _, slot := range pair.Slots {
		if slot.Dispatch == "" {
			if slot.Agent != "" || slot.Stopped {
				return ErrTransition
			}
			continue
		}
		nonempty = true
		if !replayIdentity.MatchString(slot.Dispatch) || !identities[slot.Dispatch] || used[slot.Dispatch] {
			return ErrTransition
		}
		used[slot.Dispatch] = true
		if slot.Agent != "" {
			if !replayIdentity.MatchString(slot.Agent) || !identities[slot.Agent] || used[slot.Agent] {
				return ErrTransition
			}
			used[slot.Agent] = true
		} else if slot.Stopped {
			return ErrTransition
		}
	}
	if !nonempty {
		return ErrTransition
	}
	if (pair.Slots[0].Stopped || pair.Slots[1].Stopped) && (pair.Slots[0].Agent == "" || pair.Slots[1].Agent == "") {
		return ErrTransition
	}
	return nil
}
func admitReview(state *State, token, target string) error {
	index := reviewSlot(target)
	if index < 0 || state.Finalized || state.PendingTool != "" || state.ActiveAgentID != "" || state.SystemHealthRoute || (state.AccountRoute && (!state.CaseStopObserved || !state.AccountReturnObserved)) {
		return ErrTransition
	}
	if state.ReviewPair == nil {
		if state.TransitionCount > 28 {
			return ErrTransition
		}
		state.ReviewPair = &ReviewPair{Slots: make([]ReviewWorker, 2)}
	}
	slot := &state.ReviewPair.Slots[index]
	if slot.Dispatch == token && slot.Agent == "" {
		return nil
	}
	if slot.Dispatch != "" || seen(state, token) {
		return ErrTransition
	}
	if err := remember(state, token); err != nil {
		return err
	}
	slot.Dispatch = token
	return nil
}
func correlateReview(state *State, agent, target string) error {
	index := reviewSlot(target)
	if index < 0 || state.Finalized || state.ReviewPair == nil {
		return ErrTransition
	}
	slot := &state.ReviewPair.Slots[index]
	native := key("native", agent)
	if slot.Agent == native && !slot.Stopped {
		return nil
	}
	if slot.Dispatch == "" || slot.Agent != "" || seen(state, native) || state.TransitionCount >= 32 {
		return ErrTransition
	}
	if err := remember(state, native); err != nil {
		return err
	}
	slot.Agent = native
	state.TransitionCount++
	return nil
}
func authorizeReview(state *State, agent, target string) error {
	index := reviewSlot(target)
	if index < 0 || state.Finalized || state.ReviewPair == nil {
		return ErrTransition
	}
	slot := state.ReviewPair.Slots[index]
	if slot.Agent != key("native", agent) || slot.Stopped {
		return ErrTransition
	}
	return nil
}
func stopReview(state *State, agent, target string) error {
	if authorizeReview(state, agent, target) != nil || state.TransitionCount >= 32 {
		return ErrTransition
	}
	// Both starts establish the bounded parallel pair before either return.
	for _, slot := range state.ReviewPair.Slots {
		if slot.Agent == "" {
			return ErrTransition
		}
	}
	state.ReviewPair.Slots[reviewSlot(target)].Stopped = true
	state.TransitionCount++
	return nil
}

// ObserveReviewReturn admits only the exact completed foreground call while its
// peer remains active. It grants no main-session file/tool access.
func (store Store) ObserveReviewReturn(session, prompt, tool, target string) error {
	if !boundedIdentity.MatchString(tool) || !claudeagents.RepositoryReviewer(target) {
		return ErrTransition
	}
	state, err := store.readBound(session)
	if err != nil || checkOccurrence(&state, prompt) != nil || state.Finalized || state.ReviewPair == nil {
		return ErrTransition
	}
	slot := state.ReviewPair.Slots[reviewSlot(target)]
	if slot.Dispatch != key("dispatch", tool, target) || !slot.Stopped {
		return ErrTransition
	}
	return nil
}
