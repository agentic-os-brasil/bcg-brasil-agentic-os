package actionconfirmation

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
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

const (
	StateFileName = "confirmation-state.json"
	KeyFileName   = "confirmation.key"
)

type State string

const (
	ChallengeRequired State = "challenge_required"
	Authorized        State = "authorized"
	Denied            State = "denied"
)

var identifierPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:@/-]{0,255}$`)
var challengePattern = regexp.MustCompile(`^[a-f0-9]{32}$`)

type Binding struct {
	Runtime     string
	WorkspaceID string
	ActorID     string
	SessionID   string
	Action      Action
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
	RuntimeHMAC   string    `json:"runtime_hmac"`
	WorkspaceHMAC string    `json:"workspace_hmac"`
	ActorHMAC     string    `json:"actor_hmac"`
	SessionHMAC   string    `json:"session_hmac"`
	Action        string    `json:"action"`
	TargetHMAC    string    `json:"target_hmac"`
	InputHMAC     string    `json:"input_hmac"`
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
	err := store.transaction(func(state *stateFile, key []byte) error {
		state.prune(now)
		macs := bindingHMACs(key, binding)
		for index := range state.Challenges {
			item := &state.Challenges[index]
			if !item.matches(macs, binding.Action.Action) || !now.Before(item.ExpiresAt) {
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
		state.Challenges = append(state.Challenges, challenge{ID: id, RuntimeHMAC: macs.runtime, WorkspaceHMAC: macs.workspace, ActorHMAC: macs.actor, SessionHMAC: macs.session, Action: binding.Action.Action, TargetHMAC: macs.target, InputHMAC: macs.input, State: "pending", CreatedAt: now, ExpiresAt: expires})
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
func (store Store) Confirm(runtimeName, workspaceID, actorID, sessionID, prompt string) (bool, error) {
	fields := strings.Fields(strings.TrimSpace(prompt))
	if len(fields) != 3 || fields[0] != "CONFIRM" || fields[1] != "MAESTRO" || !challengePattern.MatchString(fields[2]) {
		return false, nil
	}
	binding := Binding{Runtime: runtimeName, WorkspaceID: workspaceID, ActorID: actorID, SessionID: sessionID}
	if err := validateIdentity(binding); err != nil {
		return false, err
	}
	now := store.now()
	found := false
	err := store.transaction(func(state *stateFile, key []byte) error {
		state.prune(now)
		macs := bindingHMACs(key, binding)
		for index := range state.Challenges {
			item := &state.Challenges[index]
			if item.ID != fields[2] {
				continue
			}
			if !macEqual(item.RuntimeHMAC, macs.runtime) || !macEqual(item.WorkspaceHMAC, macs.workspace) || !macEqual(item.ActorHMAC, macs.actor) || !macEqual(item.SessionHMAC, macs.session) {
				return errors.New("challenge is bound to another runtime, workspace, actor or session")
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
	if err := validateIdentity(binding); err != nil {
		return err
	}
	if binding.Action.Action == "" || strings.TrimSpace(binding.Action.Target) == "" || validateDigest(binding.Action.InputDigest) != nil {
		return errors.New("external action is not canonicalizable")
	}
	return nil
}

func validateIdentity(binding Binding) error {
	if (binding.Runtime != "claude" && binding.Runtime != "codex") || !identifierPattern.MatchString(binding.WorkspaceID) || !identifierPattern.MatchString(binding.ActorID) || !identifierPattern.MatchString(binding.SessionID) {
		return errors.New("external action confirmation requires bounded runtime, workspace, actor and session identity")
	}
	return nil
}

type bindingMAC struct {
	runtime, workspace, actor, session, target, input string
}

func bindingHMACs(key []byte, binding Binding) bindingMAC {
	return bindingMAC{
		runtime: hmacValue(key, "runtime", binding.Runtime), workspace: hmacValue(key, "workspace", binding.WorkspaceID),
		actor: hmacValue(key, "actor", binding.ActorID), session: hmacValue(key, "session", binding.SessionID),
		target: hmacValue(key, "target", binding.Action.Target), input: hmacValue(key, "input", binding.Action.InputDigest),
	}
}

func (item challenge) matches(macs bindingMAC, action string) bool {
	return item.Action == action && macEqual(item.RuntimeHMAC, macs.runtime) && macEqual(item.WorkspaceHMAC, macs.workspace) && macEqual(item.ActorHMAC, macs.actor) && macEqual(item.SessionHMAC, macs.session) && macEqual(item.TargetHMAC, macs.target) && macEqual(item.InputHMAC, macs.input)
}

func hmacValue(key []byte, domain, value string) string {
	digest := hmac.New(sha256.New, key)
	digest.Write([]byte("maestro-action-confirmation-v1\x00" + domain + "\x00" + value))
	return hex.EncodeToString(digest.Sum(nil))
}

func macEqual(left, right string) bool {
	leftBytes, leftErr := hex.DecodeString(left)
	rightBytes, rightErr := hex.DecodeString(right)
	return leftErr == nil && rightErr == nil && hmac.Equal(leftBytes, rightBytes)
}

func (store Store) transaction(change func(*stateFile, []byte) error) error {
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
	key, err := loadOrCreateKey(filepath.Join(store.Root, KeyFileName))
	if err != nil {
		return err
	}

	state, err := readState(filepath.Join(store.Root, StateFileName))
	if err != nil {
		return err
	}
	if err := change(&state, key); err != nil {
		return err
	}
	return writeState(filepath.Join(store.Root, StateFileName), state)
}

func loadOrCreateKey(path string) ([]byte, error) {
	if info, err := os.Lstat(path); err == nil {
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm() != 0o600 || info.Size() != sha256.Size {
			return nil, errors.New("confirmation HMAC key is not a private regular key")
		}
		return os.ReadFile(path)
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	key := make([]byte, sha256.Size)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("create confirmation HMAC key: %w", err)
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return nil, err
	}
	if _, err = file.Write(key); err == nil {
		err = file.Sync()
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return nil, err
	}
	return key, nil
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
		return stateFile{SchemaVersion: 2}, nil
	}
	if err != nil {
		return stateFile{}, err
	}
	var state stateFile
	if err := json.Unmarshal(body, &state); err != nil || state.SchemaVersion != 2 {
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
