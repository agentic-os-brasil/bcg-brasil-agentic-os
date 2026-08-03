package actionconfirmation

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const StateFileName = "confirmation-state.json"

type State string

const (
	ChallengeRequired State = "challenge_required"
	Authorized        State = "authorized"
	Denied            State = "denied"
)

var identifierPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:@/-]{0,255}$`)
var challengePattern = regexp.MustCompile(`^[a-f0-9]{32}$`)

type Binding struct {
	ActorID   string
	SessionID string
	Action    Action
}

type Result struct {
	State       State
	ChallengeID string
	Action      string
	Target      string
	ExpiresAt   time.Time
}

type Store struct {
	Root string
	Now  func() time.Time
	TTL  time.Duration
}

type stateFile struct {
	SchemaVersion int         `json:"schema_version"`
	Challenges    []challenge `json:"challenges"`
}

type challenge struct {
	ID            string    `json:"id"`
	ActorDigest   string    `json:"actor_digest"`
	SessionDigest string    `json:"session_digest"`
	Action        string    `json:"action"`
	TargetDigest  string    `json:"target_digest"`
	InputDigest   string    `json:"input_digest"`
	State         string    `json:"state"`
	CreatedAt     time.Time `json:"created_at"`
	ExpiresAt     time.Time `json:"expires_at"`
	ConfirmedAt   time.Time `json:"confirmed_at,omitempty"`
	ConsumedAt    time.Time `json:"consumed_at,omitempty"`
}

func (store Store) Authorize(binding Binding) (Result, error) {
	if err := validateBinding(binding); err != nil {
		return Result{State: Denied}, err
	}
	now := store.now()
	var result Result
	err := store.transaction(func(state *stateFile) error {
		state.prune(now)
		actorDigest, sessionDigest, targetDigest := digest(binding.ActorID), digest(binding.SessionID), digest(binding.Action.Target)
		for index := range state.Challenges {
			item := &state.Challenges[index]
			if item.ActorDigest != actorDigest || item.SessionDigest != sessionDigest || item.Action != binding.Action.Action || item.TargetDigest != targetDigest || item.InputDigest != binding.Action.InputDigest || !now.Before(item.ExpiresAt) {
				continue
			}
			switch item.State {
			case "pending":
				result = Result{State: ChallengeRequired, ChallengeID: item.ID, Action: binding.Action.Action, Target: binding.Action.Target, ExpiresAt: item.ExpiresAt}
				return nil
			case "confirmed":
				item.State = "consumed"
				item.ConsumedAt = now
				result = Result{State: Authorized, Action: binding.Action.Action, Target: binding.Action.Target, ExpiresAt: item.ExpiresAt}
				return nil
			}
		}
		id, err := randomID()
		if err != nil {
			return err
		}
		expires := now.Add(store.ttl())
		state.Challenges = append(state.Challenges, challenge{ID: id, ActorDigest: actorDigest, SessionDigest: sessionDigest, Action: binding.Action.Action, TargetDigest: targetDigest, InputDigest: binding.Action.InputDigest, State: "pending", CreatedAt: now, ExpiresAt: expires})
		result = Result{State: ChallengeRequired, ChallengeID: id, Action: binding.Action.Action, Target: binding.Action.Target, ExpiresAt: expires}
		return nil
	})
	if err != nil {
		return Result{State: Denied}, err
	}
	return result, nil
}

// Confirm accepts only the exact explicit phrase for an existing pending
// challenge. Ordinary prompts are ignored and never persisted.
func (store Store) Confirm(actorID, sessionID, prompt string) (bool, error) {
	fields := strings.Fields(strings.TrimSpace(prompt))
	if len(fields) != 3 || fields[0] != "CONFIRM" || fields[1] != "MAESTRO" || !challengePattern.MatchString(fields[2]) {
		return false, nil
	}
	if !identifierPattern.MatchString(actorID) || !identifierPattern.MatchString(sessionID) {
		return false, errors.New("confirmation requires bounded actor and session identity")
	}
	now := store.now()
	found := false
	err := store.transaction(func(state *stateFile) error {
		state.prune(now)
		for index := range state.Challenges {
			item := &state.Challenges[index]
			if item.ID != fields[2] {
				continue
			}
			if item.ActorDigest != digest(actorID) || item.SessionDigest != digest(sessionID) {
				return errors.New("challenge is bound to another actor or session")
			}
			if item.State != "pending" || !now.Before(item.ExpiresAt) {
				return errors.New("challenge is expired, replayed or not pending")
			}
			item.State = "confirmed"
			item.ConfirmedAt = now
			found = true
			return nil
		}
		return errors.New("pending challenge was not found")
	})
	return found, err
}

func validateBinding(binding Binding) error {
	if !identifierPattern.MatchString(binding.ActorID) || !identifierPattern.MatchString(binding.SessionID) {
		return errors.New("external action confirmation requires bounded actor and session identity")
	}
	if binding.Action.Action == "" || strings.TrimSpace(binding.Action.Target) == "" || validateDigest(binding.Action.InputDigest) != nil {
		return errors.New("external action is not canonicalizable")
	}
	return nil
}

func (store Store) transaction(change func(*stateFile) error) error {
	if strings.TrimSpace(store.Root) == "" || !filepath.IsAbs(store.Root) {
		return errors.New("confirmation store requires an absolute local root")
	}
	if err := os.MkdirAll(store.Root, 0o700); err != nil {
		return err
	}
	if info, err := os.Lstat(store.Root); err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0 {
		return errors.New("confirmation store root is not a private directory")
	}
	lock := filepath.Join(store.Root, ".confirmation.lock")
	if err := os.Mkdir(lock, 0o700); err != nil {
		if errors.Is(err, os.ErrExist) {
			return errors.New("confirmation state lock is busy")
		}
		return err
	}
	defer os.Remove(lock)

	state, err := readState(filepath.Join(store.Root, StateFileName))
	if err != nil {
		return err
	}
	if err := change(&state); err != nil {
		return err
	}
	return writeState(filepath.Join(store.Root, StateFileName), state)
}

func readState(path string) (stateFile, error) {
	info, statErr := os.Lstat(path)
	if statErr == nil && (!info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0) {
		return stateFile{}, errors.New("confirmation state is not a private regular file")
	}
	if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
		return stateFile{}, statErr
	}
	body, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return stateFile{SchemaVersion: 1}, nil
	}
	if err != nil {
		return stateFile{}, err
	}
	var state stateFile
	if err := json.Unmarshal(body, &state); err != nil || state.SchemaVersion != 1 {
		return stateFile{}, errors.New("confirmation state is invalid")
	}
	return state, nil
}

func writeState(path string, state stateFile) error {
	body, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	directory := filepath.Dir(path)
	temporary, err := os.CreateTemp(directory, ".confirmation-state-*")
	if err != nil {
		return err
	}
	name := temporary.Name()
	defer os.Remove(name)
	if err = temporary.Chmod(0o600); err == nil {
		_, err = temporary.Write(body)
	}
	if err == nil {
		err = temporary.Sync()
	}
	if closeErr := temporary.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if info, statErr := os.Lstat(path); statErr == nil && (info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular()) {
		return errors.New("confirmation state target is not a regular file")
	} else if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
		return statErr
	}
	return os.Rename(name, path)
}

func (state *stateFile) prune(now time.Time) {
	kept := state.Challenges[:0]
	for _, item := range state.Challenges {
		if item.State == "consumed" && now.Sub(item.ConsumedAt) > 24*time.Hour || !item.ExpiresAt.IsZero() && now.Sub(item.ExpiresAt) > 24*time.Hour {
			continue
		}
		kept = append(kept, item)
	}
	state.Challenges = kept
}

func (store Store) now() time.Time {
	if store.Now != nil {
		return store.Now().UTC()
	}
	return time.Now().UTC()
}

func (store Store) ttl() time.Duration {
	if store.TTL <= 0 || store.TTL > 15*time.Minute {
		return 5 * time.Minute
	}
	return store.TTL
}

func randomID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", fmt.Errorf("create challenge ID: %w", err)
	}
	return hex.EncodeToString(value[:]), nil
}
