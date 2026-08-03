package actionconfirmation

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestCanonicalizeProtectsExternalMutationButNotOrdinaryLocalWork(t *testing.T) {
	protected, err := Canonicalize("Bash", json.RawMessage(`{"command":"git push origin refs/heads/topic"}`))
	if err != nil || protected == nil || protected.Action != "git.push" || protected.Target != "origin:refs/heads/topic" || len(protected.InputDigest) != 64 {
		t.Fatalf("Canonicalize protected = %#v, %v", protected, err)
	}
	local, err := Canonicalize("Bash", json.RawMessage(`{"command":"go test ./internal/actionconfirmation"}`))
	if err != nil || local != nil {
		t.Fatalf("Canonicalize local = %#v, %v", local, err)
	}
	if got, err := Canonicalize("Bash", json.RawMessage(`{"command":"git push origin main && echo done"}`)); err == nil || got != nil {
		t.Fatalf("non-canonical external mutation = %#v, %v", got, err)
	}
	if got, err := Canonicalize("Bash", json.RawMessage(`{"command":"go test ./...","command":"git push origin refs/heads/topic"}`)); err == nil || got != nil {
		t.Fatalf("duplicate-key mutation = %#v, %v", got, err)
	}
}

func TestStoreRejectsPermissiveRoot(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0o755); err != nil {
		t.Fatal(err)
	}
	store := Store{Root: root}
	action := Action{Action: "git.push", Target: "origin:refs/heads/topic", InputDigest: strings.Repeat("e", 64)}
	if _, err := store.Authorize(Binding{ActorID: "owner-a", SessionID: "session-a", Action: action}); err == nil {
		t.Fatal("permissive confirmation root was accepted")
	}
}

func TestChallengeRequiresExactUserConfirmationAndIsOneShot(t *testing.T) {
	now := time.Date(2026, 8, 2, 20, 0, 0, 0, time.UTC)
	store := Store{Root: filepath.Join(t.TempDir(), "confirmation"), Now: func() time.Time { return now }, TTL: 5 * time.Minute}
	action := Action{Action: "git.push", Target: "origin:refs/heads/topic", InputDigest: strings.Repeat("a", 64)}

	first, err := store.Authorize(Binding{ActorID: "owner-a", SessionID: "session-a", Action: action})
	if err != nil || first.State != ChallengeRequired || first.ChallengeID == "" {
		t.Fatalf("first authorization = %#v, %v", first, err)
	}
	if recognized, err := store.Confirm("owner-a", "session-a", "yes, please proceed"); err != nil || recognized {
		t.Fatalf("ordinary prompt recognized = %v, %v", recognized, err)
	}
	confirmation := "CONFIRM MAESTRO " + first.ChallengeID
	if recognized, err := store.Confirm("owner-a", "session-a", confirmation); err != nil || !recognized {
		t.Fatalf("confirmation = %v, %v", recognized, err)
	}
	allowed, err := store.Authorize(Binding{ActorID: "owner-a", SessionID: "session-a", Action: action})
	if err != nil || allowed.State != Authorized {
		t.Fatalf("allowed = %#v, %v", allowed, err)
	}
	replay, err := store.Authorize(Binding{ActorID: "owner-a", SessionID: "session-a", Action: action})
	if err != nil || replay.State != ChallengeRequired || replay.ChallengeID == first.ChallengeID {
		t.Fatalf("replay = %#v, %v", replay, err)
	}
}

func TestChallengeFailsClosedAcrossIdentityExpiryAndMutation(t *testing.T) {
	now := time.Date(2026, 8, 2, 20, 0, 0, 0, time.UTC)
	store := Store{Root: filepath.Join(t.TempDir(), "confirmation"), Now: func() time.Time { return now }, TTL: time.Minute}
	action := Action{Action: "git.push", Target: "origin:refs/heads/topic", InputDigest: strings.Repeat("b", 64)}
	result, err := store.Authorize(Binding{ActorID: "owner-a", SessionID: "session-a", Action: action})
	if err != nil {
		t.Fatal(err)
	}
	confirmation := "CONFIRM MAESTRO " + result.ChallengeID
	for _, identity := range [][2]string{{"", "session-a"}, {"owner-a", ""}, {"owner-b", "session-a"}, {"owner-a", "session-b"}} {
		if recognized, err := store.Confirm(identity[0], identity[1], confirmation); err == nil || recognized {
			t.Fatalf("cross identity confirmation accepted for %#v", identity)
		}
	}
	if recognized, err := store.Confirm("owner-a", "session-a", confirmation); err != nil || !recognized {
		t.Fatal(err)
	}
	mutated := action
	mutated.InputDigest = strings.Repeat("c", 64)
	if got, err := store.Authorize(Binding{ActorID: "owner-a", SessionID: "session-a", Action: mutated}); err != nil || got.State != ChallengeRequired {
		t.Fatalf("mutated input = %#v, %v", got, err)
	}
	mutatedTarget := action
	mutatedTarget.Target = "origin:refs/heads/other"
	if got, err := store.Authorize(Binding{ActorID: "owner-a", SessionID: "session-a", Action: mutatedTarget}); err != nil || got.State != ChallengeRequired {
		t.Fatalf("mutated target = %#v, %v", got, err)
	}
	if recognized, err := store.Confirm("owner-a", "session-a", confirmation); err == nil || recognized {
		t.Fatalf("confirmation replay = %v, %v", recognized, err)
	}
	now = now.Add(2 * time.Minute)
	if got, err := store.Authorize(Binding{ActorID: "owner-a", SessionID: "session-a", Action: action}); err != nil || got.State != ChallengeRequired {
		t.Fatalf("expired confirmation = %#v, %v", got, err)
	}
}

func TestChallengeConsumptionIsAtomicAndPersistsNoRawInput(t *testing.T) {
	now := time.Date(2026, 8, 2, 20, 0, 0, 0, time.UTC)
	root := filepath.Join(t.TempDir(), "confirmation")
	store := Store{Root: root, Now: func() time.Time { return now }, TTL: time.Minute}
	action := Action{Action: "github.pull_request.create", Target: "org/private-repo", InputDigest: strings.Repeat("d", 64)}
	first, err := store.Authorize(Binding{ActorID: "owner-a", SessionID: "session-a", Action: action})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Confirm("owner-a", "session-a", "CONFIRM MAESTRO "+first.ChallengeID); err != nil {
		t.Fatal(err)
	}

	start := make(chan struct{})
	states := make(chan State, 2)
	var wait sync.WaitGroup
	for range 2 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			result, err := store.Authorize(Binding{ActorID: "owner-a", SessionID: "session-a", Action: action})
			if err != nil {
				states <- Denied
				return
			}
			states <- result.State
		}()
	}
	close(start)
	wait.Wait()
	close(states)
	authorized := 0
	for state := range states {
		if state == Authorized {
			authorized++
		}
	}
	if authorized != 1 {
		t.Fatalf("authorized count = %d", authorized)
	}

	body, err := os.ReadFile(filepath.Join(root, StateFileName))
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"owner-a", "session-a", "org/private-repo", "git push", "CONFIRM MAESTRO"} {
		if strings.Contains(string(body), forbidden) {
			t.Fatalf("state persisted raw value %q: %s", forbidden, body)
		}
	}
}
