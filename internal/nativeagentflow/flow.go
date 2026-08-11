// Package nativeagentflow enforces the minimum Maestro native-agent sequence
// across Claude hook processes. It stores metadata only: agent identities and
// transition state, never prompts, tool inputs, results or client content.
package nativeagentflow

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/processwait"
)

const maximumStateBytes = 8 << 10

type State struct {
	SchemaVersion     int    `json:"schema_version"`
	WorkspaceID       string `json:"workspace_id"`
	SessionDigest     string `json:"session_digest"`
	ActiveAgentID     string `json:"active_agent_id,omitempty"`
	ActiveAgentType   string `json:"active_agent_type,omitempty"`
	AccountRoute      bool   `json:"account_route"`
	AccountFramed     bool   `json:"account_framed"`
	CaseCompleted     bool   `json:"case_completed"`
	AccountValidated  bool   `json:"account_validated"`
	SystemHealthRoute bool   `json:"system_health_route"`
	TransitionCount   int    `json:"transition_count"`
	LastEvent         string `json:"last_event,omitempty"`
	LastAgentID       string `json:"last_agent_id,omitempty"`
	LastAgentType     string `json:"last_agent_type,omitempty"`
}

type Store struct {
	root        string
	workspaceID string
}

func New(dataRoot, workspaceID string) (Store, error) {
	if strings.TrimSpace(dataRoot) == "" || strings.TrimSpace(workspaceID) == "" {
		return Store{}, errors.New("native agent flow requires data root and workspace identity")
	}
	return Store{root: filepath.Join(dataRoot, "native-agent-flow", digest(workspaceID)), workspaceID: workspaceID}, nil
}

func (store Store) BeginTurn(sessionID string) error {
	return store.update(sessionID, func(state *State) error {
		// UserPromptSubmit is synchronous and runs before any specialist starts.
		// A hook retry therefore sees a pristine state and must be a no-op. Never
		// erase an active route: a second prompt arriving while a specialist is
		// running is an ordering error, not a new turn boundary.
		if state.ActiveAgentID != "" {
			return errors.New("cannot begin a new Maestro turn while a managed specialist is active")
		}
		if state.TransitionCount == 0 && !state.AccountRoute && !state.AccountFramed && !state.CaseCompleted && !state.AccountValidated && !state.SystemHealthRoute {
			return nil
		}
		*state = store.empty(sessionID)
		return nil
	})
}

func (store Store) Start(sessionID, agentID, agentType string) error {
	return store.update(sessionID, func(state *State) error {
		if state.ActiveAgentID != "" {
			if state.ActiveAgentID == agentID && state.ActiveAgentType == agentType && state.LastEvent == "start" {
				return nil
			}
			return fmt.Errorf("Maestro allows one managed specialist at a time; %s is still active", state.ActiveAgentType)
		}
		if state.TransitionCount >= 32 {
			return errors.New("managed specialist transition limit reached for this turn")
		}
		switch agentType {
		case "pa-expert":
			if state.SystemHealthRoute {
				return errors.New("PA Expert cannot join a Darwin system-health route")
			}
		case "darwin":
			if state.AccountRoute || state.CaseCompleted || state.AccountFramed {
				return errors.New("Darwin system-health work cannot be mixed with client execution in the same turn")
			}
			state.SystemHealthRoute = true
		case "client-account-agent":
			if state.SystemHealthRoute {
				return errors.New("Client Account Agent cannot join a Darwin system-health route")
			}
			if !state.AccountRoute {
				if state.CaseCompleted {
					return errors.New("Client Account Agent must be selected before Case Agent on the strategic route")
				}
				state.AccountRoute = true
			} else if !state.AccountFramed || !state.CaseCompleted || state.AccountValidated {
				return errors.New("Client Account Agent may return only after Case Agent completes the selected strategic route")
			}
		case "case-agent":
			if state.SystemHealthRoute {
				return errors.New("Case Agent cannot join a Darwin system-health route")
			}
			if state.CaseCompleted {
				return errors.New("Case Agent already completed this turn")
			}
			if state.AccountRoute && !state.AccountFramed {
				return errors.New("Case Agent must wait for Client Account Agent framing on the selected strategic route")
			}
		case "walter":
			if state.SystemHealthRoute || !state.CaseCompleted || (state.AccountRoute && !state.AccountValidated) {
				return errors.New("Walter may refine only after the selected Case route and any required Client Account validation complete")
			}
		default:
			return fmt.Errorf("agent type %q is not managed by Maestro", agentType)
		}
		state.ActiveAgentID = agentID
		state.ActiveAgentType = agentType
		state.LastEvent = "start"
		state.LastAgentID = agentID
		state.LastAgentType = agentType
		state.TransitionCount++
		return nil
	})
}

func (store Store) Stop(sessionID, agentID, agentType string) error {
	return store.update(sessionID, func(state *State) error {
		if state.ActiveAgentID == "" && state.LastEvent == "stop" && state.LastAgentID == agentID && state.LastAgentType == agentType {
			return nil
		}
		if state.ActiveAgentID != agentID || state.ActiveAgentType != agentType {
			return errors.New("managed specialist stop does not match the active specialist")
		}
		switch agentType {
		case "client-account-agent":
			if !state.AccountFramed {
				state.AccountFramed = true
			} else {
				state.AccountValidated = true
			}
		case "case-agent":
			state.CaseCompleted = true
		}
		state.ActiveAgentID = ""
		state.ActiveAgentType = ""
		state.LastEvent = "stop"
		state.LastAgentID = agentID
		state.LastAgentType = agentType
		state.TransitionCount++
		return nil
	})
}

func (store Store) Finalize(sessionID string) (bool, string, error) {
	state, err := store.read(sessionID)
	if errors.Is(err, os.ErrNotExist) {
		return true, "", nil
	}
	if err != nil {
		return false, "", err
	}
	if state.ActiveAgentID != "" {
		return false, "Maestro cannot finish while " + state.ActiveAgentType + " is still active", nil
	}
	if state.AccountRoute && !state.CaseCompleted {
		return false, "Maestro selected the strategic route; call Case Agent after Client Account Agent framing before finishing", nil
	}
	if state.AccountRoute && !state.AccountValidated {
		return false, "Maestro selected the strategic route; return the Case result to Client Account Agent for stakeholder validation before finishing", nil
	}
	return true, "", nil
}

func (store Store) empty(sessionID string) State {
	return State{SchemaVersion: 1, WorkspaceID: store.workspaceID, SessionDigest: digest(sessionID)}
}

func (store Store) statePath(sessionID string) string {
	return filepath.Join(store.root, digest(sessionID)+".json")
}

func (store Store) read(sessionID string) (State, error) {
	body, err := os.ReadFile(store.statePath(sessionID))
	if err != nil {
		return State{}, err
	}
	if len(body) == 0 || len(body) > maximumStateBytes {
		return State{}, errors.New("native agent flow state is outside its bounded size")
	}
	var state State
	if err := json.Unmarshal(body, &state); err != nil {
		return State{}, fmt.Errorf("decode native agent flow: %w", err)
	}
	if state.SchemaVersion != 1 || state.WorkspaceID != store.workspaceID || state.SessionDigest != digest(sessionID) || state.TransitionCount < 0 || state.TransitionCount > 32 {
		return State{}, errors.New("native agent flow state is invalid")
	}
	return state, nil
}

func (store Store) update(sessionID string, mutate func(*State) error) error {
	if strings.TrimSpace(sessionID) == "" {
		return errors.New("native agent flow requires a session identity")
	}
	if err := os.MkdirAll(store.root, 0o700); err != nil {
		return err
	}
	lockPath := store.statePath(sessionID) + ".lock"
	unlock, err := acquireLock(lockPath)
	if err != nil {
		return err
	}
	defer unlock()
	state, err := store.read(sessionID)
	if errors.Is(err, os.ErrNotExist) {
		state = store.empty(sessionID)
	} else if err != nil {
		return err
	}
	if err := mutate(&state); err != nil {
		return err
	}
	body, err := json.Marshal(state)
	if err != nil {
		return err
	}
	if len(body) > maximumStateBytes {
		return errors.New("native agent flow state exceeds its bounded size")
	}
	temporary, err := os.CreateTemp(store.root, ".flow-*.tmp")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(body); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, store.statePath(sessionID))
}

func digest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func acquireLock(path string) (func(), error) {
	body := []byte(strconv.Itoa(os.Getpid()) + "\n")
	create := func() (func(), error) {
		file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err != nil {
			return nil, err
		}
		if _, err := file.Write(body); err != nil {
			file.Close()
			os.Remove(path)
			return nil, err
		}
		if err := file.Close(); err != nil {
			os.Remove(path)
			return nil, err
		}
		return func() { _ = os.Remove(path) }, nil
	}
	unlock, err := create()
	if err == nil {
		return unlock, nil
	}
	if !errors.Is(err, os.ErrExist) {
		return nil, err
	}
	current, readErr := os.ReadFile(path)
	if readErr != nil || len(current) == 0 || len(current) > 32 || !bytes.HasSuffix(current, []byte("\n")) {
		return nil, errors.New("native agent flow lock is busy or malformed")
	}
	pid, parseErr := strconv.Atoi(strings.TrimSuffix(string(current), "\n"))
	if parseErr != nil || pid <= 0 || pid == os.Getpid() {
		return nil, errors.New("native agent flow lock has an invalid owner")
	}
	if waitErr := processwait.UntilExit(pid, 100*time.Millisecond); waitErr != nil {
		return nil, errors.New("native agent flow is busy; retry the hook once")
	}
	verified, readErr := os.ReadFile(path)
	if readErr != nil || !bytes.Equal(verified, current) {
		return nil, errors.New("native agent flow lock changed during stale-owner verification")
	}
	if err := os.Remove(path); err != nil {
		return nil, fmt.Errorf("remove stale native agent flow lock: %w", err)
	}
	return create()
}
