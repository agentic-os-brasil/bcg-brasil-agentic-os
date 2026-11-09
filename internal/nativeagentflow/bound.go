package nativeagentflow

import (
	"bytes"
	"encoding/json"
	"errors"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/claudeagents"
	"io"
	"math"
	"os"
	"regexp"
	"slices"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/lifecycle"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

const maximumReplayIdentities = 96

var ErrTurnResetUnavailable = errors.New("turn_reset_unavailable")
var ErrTransition = errors.New("native_transition_unavailable")
var replayIdentity = regexp.MustCompile(`^[a-f0-9]{32}$`)
var boundedIdentity = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:-]{0,127}$`)

// NewBound uses the existing flow store under an admitted private owner root.
// It does not create directories. Schema-1/legacy observations cannot grant authority.
func NewBound(dataRoot, workspaceID string) (Store, error) {
	if err := scheduler.ValidatePrivateDirectory(dataRoot); err != nil {
		return Store{}, err
	}
	return New(dataRoot, workspaceID)
}
func key(parts ...string) string        { return lifecycle.IdempotencyKey(parts...) }
func seen(state *State, id string) bool { return slices.Contains(state.Seen, id) }
func remember(state *State, id string) error {
	if seen(state, id) {
		return nil
	}
	if len(state.Seen) >= maximumReplayIdentities {
		return ErrTransition
	}
	state.Seen = append(state.Seen, id)
	return nil
}
func (store Store) decodeBound(body []byte, session string) (State, error) {
	if len(body) == 0 {
		state := store.empty(session)
		state.SchemaVersion = 2
		state.Profile = "maestro_planner"
		return state, nil
	}
	// Existing bytes must supply their own authority identifiers. Defaults
	// belong only to the absent-file path, never to missing/null stored fields.
	var state State
	d := json.NewDecoder(bytes.NewReader(body))
	d.DisallowUnknownFields()
	if err := d.Decode(&state); err != nil {
		return State{}, ErrTransition
	}
	if d.Decode(new(any)) != io.EOF || state.SchemaVersion != 2 || state.Profile != "maestro_planner" || state.WorkspaceID != store.workspaceID || state.SessionDigest != digest(session) || state.Revision == 0 || len(state.Seen) > maximumReplayIdentities || state.TransitionCount < 0 || state.TransitionCount > 32 {
		return State{}, ErrTransition
	}
	if (state.PendingTool == "") != (state.PendingType == "") || (state.ActiveAgentID == "") != (state.ActiveAgentType == "") || (state.PendingTool != "" && state.ActiveAgentID != "") {
		return State{}, ErrTransition
	}
	if state.Revision == math.MaxUint64 || state.TurnDigest == "" {
		return State{}, ErrTransition
	}
	identities := map[string]bool{}
	for _, id := range state.Seen {
		if (id != "initial" && !replayIdentity.MatchString(id)) || identities[id] {
			return State{}, ErrTransition
		}
		identities[id] = true
	}
	if !identities[state.TurnDigest] {
		return State{}, ErrTransition
	}
	if state.PendingTool != "" {
		canonical, ok := claudeagents.CanonicalAgentID(state.PendingType)
		if !ok || canonical != state.PendingType || !identities[state.PendingTool] {
			return State{}, ErrTransition
		}
	}
	if state.ActiveAgentID != "" {
		canonical, ok := claudeagents.CanonicalAgentID(state.ActiveAgentType)
		if !ok || canonical != state.ActiveAgentType || !identities[state.ActiveAgentID] {
			return State{}, ErrTransition
		}
	}
	if state.Finalized && (state.PendingTool != "" || state.ActiveAgentID != "") {
		return State{}, ErrTransition
	}
	return state, nil
}
func (store Store) updateBound(session string, mutate func(*State) error) error {
	if !boundedIdentity.MatchString(session) {
		return ErrTransition
	}
	return scheduler.UpdatePrivateFile(store.statePath(session), maximumStateBytes, func(body []byte) ([]byte, error) {
		state, err := store.decodeBound(body, session)
		if err != nil {
			return nil, err
		}
		before, _ := json.Marshal(state)
		expected := state.Revision
		if err := mutate(&state); err != nil {
			return nil, err
		}
		if state.Revision != expected {
			return nil, ErrTransition
		}
		after, _ := json.Marshal(state)
		if len(body) > 0 && bytes.Equal(before, after) {
			return nil, nil
		}
		state.Revision++
		return json.Marshal(state)
	})
}
func (store Store) readBound(session string) (State, error) {
	body, err := scheduler.ReadPrivateStateFile(store.statePath(session), maximumStateBytes)
	if err != nil {
		return State{}, err
	}
	if len(body) == 0 {
		return State{}, ErrTransition
	}
	return store.decodeBound(body, session)
}
func checkOccurrence(state *State, prompt string) error {
	if state.TurnDigest == "" {
		return ErrTransition
	}
	if prompt != "" && state.TurnDigest != key("prompt", prompt) {
		return ErrTransition
	}
	return nil
}

// BeginOccurrence never hashes prompt bodies and never guesses a new occurrence.
func (store Store) BeginOccurrence(session, prompt string) error {
	if prompt != "" && !boundedIdentity.MatchString(prompt) {
		return ErrTransition
	}
	return store.updateBound(session, func(state *State) error {
		occurrence := "initial"
		if prompt != "" {
			occurrence = key("prompt", prompt)
		}
		if state.TurnDigest == occurrence {
			if prompt == "" && (state.TransitionCount > 0 || state.Finalized || state.PendingTool != "") {
				return ErrTurnResetUnavailable
			}
			return nil
		}
		if state.TurnDigest != "" {
			if prompt == "" {
				return ErrTurnResetUnavailable
			}
			if state.ActiveAgentID != "" || state.PendingTool != "" {
				return ErrTransition
			}
			if seen(state, occurrence) {
				return ErrTransition
			}
		}
		if err := remember(state, occurrence); err != nil {
			return err
		}
		prior := *state
		*state = store.empty(session)
		state.SchemaVersion = 2
		state.Profile = prior.Profile
		state.Revision = prior.Revision
		state.Seen = prior.Seen
		state.TurnDigest = occurrence
		return nil
	})
}

// Admit reserves the sole pending target before the host can create it.
func (store Store) Admit(session, prompt, tool, target string) error {
	if !boundedIdentity.MatchString(tool) {
		return ErrTransition
	}
	return store.updateBound(session, func(state *State) error {
		if err := checkOccurrence(state, prompt); err != nil {
			return err
		}
		token := key("dispatch", tool, target)
		if state.PendingTool == token && state.PendingType == target && !state.Finalized {
			return nil
		}
		if state.Finalized || state.PendingTool != "" || state.ActiveAgentID != "" || seen(state, token) {
			return ErrTransition
		}
		trial := *state
		if err := startState(&trial, "pending", target); err != nil {
			return ErrTransition
		}
		if err := remember(state, token); err != nil {
			return err
		}
		state.PendingTool = token
		state.PendingType = target
		return nil
	})
}

// Correlate receives no host dispatch ID. It matches only the sole pending type.
func (store Store) Correlate(session, prompt, agent, target string) error {
	if !boundedIdentity.MatchString(agent) {
		return ErrTransition
	}
	return store.updateBound(session, func(state *State) error {
		if err := checkOccurrence(state, prompt); err != nil {
			return err
		}
		native := key("native", agent)
		if state.ActiveAgentID == native && state.ActiveAgentType == target {
			return nil
		}
		if state.Finalized || state.PendingTool == "" || state.PendingType != target || seen(state, native) {
			return ErrTransition
		}
		if err := remember(state, native); err != nil {
			return err
		}
		if err := startState(state, native, target); err != nil {
			return ErrTransition
		}
		state.PendingTool = ""
		state.PendingType = ""
		return nil
	})
}
func (store Store) Authorize(session, prompt, agent, target string) error {
	state, err := store.readBound(session)
	if err != nil {
		return ErrTransition
	}
	if err := checkOccurrence(&state, prompt); err != nil {
		return err
	}
	if state.Finalized || state.ActiveAgentID != key("native", agent) || state.ActiveAgentType != target {
		return ErrTransition
	}
	return nil
}
func (store Store) ObserveStop(session, prompt, agent, target string) error {
	return store.updateBound(session, func(state *State) error {
		if err := checkOccurrence(state, prompt); err != nil {
			return err
		}
		token := key("stopped", agent, target)
		if seen(state, token) {
			return nil
		}
		native := key("native", agent)
		if state.ActiveAgentID != native || state.ActiveAgentType != target || state.TransitionCount >= 32 {
			return ErrTransition
		}
		if err := remember(state, token); err != nil {
			return err
		}
		switch target {
		case "case-agent":
			state.CaseStopObserved = true
		case "client-account-agent":
			if !state.AccountFramingObserved {
				state.AccountFramingObserved = true
			} else {
				state.AccountReturnObserved = true
			}
		}
		state.ActiveAgentID = ""
		state.ActiveAgentType = ""
		state.LastEvent = "stop"
		state.LastAgentID = native
		state.LastAgentType = target
		state.TransitionCount++
		return nil
	})
}
func (store Store) ObserveFinalize(session, prompt string) error {
	return store.updateBound(session, func(state *State) error {
		if err := checkOccurrence(state, prompt); err != nil {
			return err
		}
		if state.PendingTool != "" || state.ActiveAgentID != "" || (state.AccountRoute && (!state.CaseStopObserved || !state.AccountReturnObserved)) {
			return ErrTransition
		}
		state.Finalized = true
		return nil
	})
}

// CheckMain refuses omitted child identity while a dispatch or child is unresolved.
func (store Store) CheckMain(session, prompt string) error {
	state, err := store.readBound(session)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return ErrTransition
	}
	if state.PendingTool != "" || state.ActiveAgentID != "" {
		return ErrTransition
	}
	if prompt != "" && state.TurnDigest != "" && state.TurnDigest != key("prompt", prompt) {
		return ErrTransition
	}
	return nil
}
