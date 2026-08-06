package actionconfirmation

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func testBinding(action Action) Binding {
	return Binding{Runtime: "claude", WorkspaceID: "workspace-a", ActorID: "owner-a", SessionID: "session-a", Action: action}
}

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
	for _, test := range []struct {
		tool, raw, action string
	}{
		{"mcp__github__create_pull_request", `{"repository":"org/repo","title":"title"}`, "github.pull_request.create"},
		{"mcp__outlook_email__send_email", `{"to":"person@example.com","subject":"subject"}`, "email.send"},
		{"mcp__teams__send_message", `{"channel":"channel-a","message":"hello"}`, "teams.message.send"},
		{"mcp__slack__send_message", `{"channel":"channel-a","message":"hello"}`, "slack.message.send"},
	} {
		got, err := Canonicalize(test.tool, json.RawMessage(test.raw))
		if err != nil || got == nil || got.Action != test.action {
			t.Fatalf("Canonicalize(%s) = %#v, %v", test.tool, got, err)
		}
	}
	for _, tool := range []string{"collaboration.send_message", "mcp__internal__send_message", "mcp__workspace_agents__send_message"} {
		got, err := Canonicalize(tool, json.RawMessage(`{"target":"internal-agent","message":"hello"}`))
		if err != nil || got != nil {
			t.Fatalf("internal tool %s treated as external = %#v, %v", tool, got, err)
		}
	}
}

func TestReadOnlyBCGOSDiagnosticsUseAClosedSimpleCommandGrammar(t *testing.T) {
	installed := filepath.Join(t.TempDir(), "Maestro", "bin", "bcgos")
	if err := os.MkdirAll(filepath.Dir(installed), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(installed, []byte("installed"), 0o700); err != nil {
		t.Fatal(err)
	}
	allowed := []string{
		fmt.Sprintf(`{"command":%q}`, fmt.Sprintf("%q doctor", installed)),
		fmt.Sprintf(`{"command":%q}`, fmt.Sprintf("%q version", installed)),
		fmt.Sprintf(`{"command":%q}`, fmt.Sprintf("%q status '/Users/example/Developer/maestro-os'", installed)),
		fmt.Sprintf(`{"command":%q}`, fmt.Sprintf("%q owner onboarding status", installed)),
	}
	for _, raw := range allowed {
		if !IsReadOnlyBCGOSDiagnostic("Bash", json.RawMessage(raw), installed) {
			t.Fatalf("read-only diagnostic was not recognized: %s", raw)
		}
	}
	arbitrary := filepath.Join(t.TempDir(), "bcgos")
	if err := os.WriteFile(arbitrary, []byte("attacker"), 0o700); err != nil {
		t.Fatal(err)
	}
	denied := []string{
		fmt.Sprintf(`{"command":%q}`, fmt.Sprintf("%q doctor", arbitrary)),
		`{"command":"C:\\\\tmp\\\\bcgos.exe doctor"}`,
		fmt.Sprintf(`{"command":%q}`, fmt.Sprintf("%q init /tmp/workspace", installed)),
		fmt.Sprintf(`{"command":%q}`, fmt.Sprintf("%q doctor && touch /tmp/marker", installed)),
		fmt.Sprintf(`{"command":%q}`, fmt.Sprintf("%q owner init", installed)),
		fmt.Sprintf(`{"command":%q}`, fmt.Sprintf("%q update", installed)),
	}
	for _, raw := range denied {
		if IsReadOnlyBCGOSDiagnostic("Bash", json.RawMessage(raw), installed) {
			t.Fatalf("mutating or compound command was recognized as read-only: %s", raw)
		}
	}
}

func TestReadOnlyDiagnosticsAllowObservedBoundedShellForms(t *testing.T) {
	installed := filepath.Join(t.TempDir(), "Maestro", "bin", "bcgos")
	if err := os.MkdirAll(filepath.Dir(installed), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(installed, []byte("installed"), 0o700); err != nil {
		t.Fatal(err)
	}
	bcgos := fmt.Sprintf("%q skills index 2>/dev/null | head -60", installed)
	if !IsReadOnlyBCGOSDiagnostic("Bash", json.RawMessage(fmt.Sprintf(`{"command":%q}`, bcgos)), installed) {
		t.Fatalf("bounded skills index pipeline was not recognized: %s", bcgos)
	}
	lsCommand := `ls /Users/example/Developer/maestro-os/owner/self/ 2>/dev/null || echo "dir not found"`
	if !IsReadOnlyBoundedDiagnostic("Bash", json.RawMessage(fmt.Sprintf(`{"command":%q}`, lsCommand))) {
		t.Fatalf("bounded ls diagnostic was not recognized: %s", lsCommand)
	}
	denied := []string{
		fmt.Sprintf("%q skills index 2>/dev/null | head -60; touch /tmp/marker", installed),
		`ls /Users/example/Developer/maestro-os/owner/self/ || rm -rf /`,
		fmt.Sprintf("%q skills index 2>/dev/null | head -1000", installed),
	}
	for _, command := range denied {
		raw := json.RawMessage(fmt.Sprintf(`{"command":%q}`, command))
		if IsReadOnlyBCGOSDiagnostic("Bash", raw, installed) || IsReadOnlyBoundedDiagnostic("Bash", raw) {
			t.Fatalf("unsafe diagnostic was recognized as read-only: %s", command)
		}
	}
}

func TestStoreRejectsPermissiveRoot(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0o755); err != nil {
		t.Fatal(err)
	}
	store := Store{Root: root}
	action := Action{Action: "git.push", Target: "origin:refs/heads/topic", InputDigest: strings.Repeat("e", 64)}
	if _, err := store.Authorize(testBinding(action)); err == nil {
		t.Fatal("permissive confirmation root was accepted")
	}
}

func TestChallengeRequiresExactUserConfirmationAndIsOneShot(t *testing.T) {
	now := time.Date(2026, 8, 2, 20, 0, 0, 0, time.UTC)
	store := Store{Root: filepath.Join(t.TempDir(), "confirmation"), Now: func() time.Time { return now }, TTL: 5 * time.Minute}
	action := Action{Action: "git.push", Target: "origin:refs/heads/topic", InputDigest: strings.Repeat("a", 64)}

	first, err := store.Authorize(testBinding(action))
	if err != nil || first.State != ChallengeRequired || first.ChallengeID == "" {
		t.Fatalf("first authorization = %#v, %v", first, err)
	}
	if recognized, err := store.Confirm("claude", "workspace-a", "owner-a", "session-a", "yes, please proceed"); err != nil || recognized {
		t.Fatalf("ordinary prompt recognized = %v, %v", recognized, err)
	}
	confirmation := "CONFIRM MAESTRO " + first.ChallengeID
	if recognized, err := store.Confirm("claude", "workspace-a", "owner-a", "session-a", confirmation); err != nil || !recognized {
		t.Fatalf("confirmation = %v, %v", recognized, err)
	}
	allowed, err := store.Authorize(testBinding(action))
	if err != nil || allowed.State != Authorized {
		t.Fatalf("allowed = %#v, %v", allowed, err)
	}
	replay, err := store.Authorize(testBinding(action))
	if err != nil || replay.State != ChallengeRequired || replay.ChallengeID == first.ChallengeID {
		t.Fatalf("replay = %#v, %v", replay, err)
	}
}

func TestChallengeFailsClosedAcrossIdentityExpiryAndMutation(t *testing.T) {
	now := time.Date(2026, 8, 2, 20, 0, 0, 0, time.UTC)
	store := Store{Root: filepath.Join(t.TempDir(), "confirmation"), Now: func() time.Time { return now }, TTL: time.Minute}
	action := Action{Action: "git.push", Target: "origin:refs/heads/topic", InputDigest: strings.Repeat("b", 64)}
	result, err := store.Authorize(testBinding(action))
	if err != nil {
		t.Fatal(err)
	}
	confirmation := "CONFIRM MAESTRO " + result.ChallengeID
	for _, identity := range [][2]string{{"", "session-a"}, {"owner-a", ""}, {"owner-b", "session-a"}, {"owner-a", "session-b"}} {
		if recognized, err := store.Confirm("claude", "workspace-a", identity[0], identity[1], confirmation); err == nil || recognized {
			t.Fatalf("cross identity confirmation accepted for %#v", identity)
		}
	}
	if recognized, err := store.Confirm("codex", "workspace-a", "owner-a", "session-a", confirmation); err == nil || recognized {
		t.Fatalf("cross-runtime confirmation = %v, %v", recognized, err)
	}
	if recognized, err := store.Confirm("claude", "workspace-b", "owner-a", "session-a", confirmation); err == nil || recognized {
		t.Fatalf("cross-workspace confirmation = %v, %v", recognized, err)
	}
	if recognized, err := store.Confirm("claude", "workspace-a", "owner-a", "session-a", confirmation); err != nil || !recognized {
		t.Fatal(err)
	}
	mutated := action
	mutated.InputDigest = strings.Repeat("c", 64)
	mutatedBinding := testBinding(mutated)
	if got, err := store.Authorize(mutatedBinding); err != nil || got.State != ChallengeRequired {
		t.Fatalf("mutated input = %#v, %v", got, err)
	}
	mutatedTarget := action
	mutatedTarget.Target = "origin:refs/heads/other"
	if got, err := store.Authorize(testBinding(mutatedTarget)); err != nil || got.State != ChallengeRequired {
		t.Fatalf("mutated target = %#v, %v", got, err)
	}
	if recognized, err := store.Confirm("claude", "workspace-a", "owner-a", "session-a", confirmation); err == nil || recognized {
		t.Fatalf("confirmation replay = %v, %v", recognized, err)
	}
	now = now.Add(2 * time.Minute)
	if got, err := store.Authorize(testBinding(action)); err != nil || got.State != ChallengeRequired {
		t.Fatalf("expired confirmation = %#v, %v", got, err)
	}
}

func TestChallengeConsumptionIsAtomicAndPersistsNoRawInput(t *testing.T) {
	now := time.Date(2026, 8, 2, 20, 0, 0, 0, time.UTC)
	root := filepath.Join(t.TempDir(), "confirmation")
	store := Store{Root: root, Now: func() time.Time { return now }, TTL: time.Minute}
	action := Action{Action: "github.pull_request.create", Target: "org/private-repo", InputDigest: strings.Repeat("d", 64)}
	first, err := store.Authorize(testBinding(action))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Confirm("claude", "workspace-a", "owner-a", "session-a", "CONFIRM MAESTRO "+first.ChallengeID); err != nil {
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
			result, err := store.Authorize(testBinding(action))
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
	for _, rawDigest := range []string{digest("owner-a"), digest("session-a"), digest("workspace-a"), digest("org/private-repo"), strings.Repeat("d", 64)} {
		if strings.Contains(string(body), rawDigest) {
			t.Fatalf("state persisted an unkeyed digest %q: %s", rawDigest, body)
		}
	}
	keyInfo, err := os.Stat(filepath.Join(root, KeyFileName))
	if err != nil || keyInfo.Mode().Perm() != 0o600 || keyInfo.Size() != sha256.Size {
		t.Fatalf("HMAC key = %#v, %v", keyInfo, err)
	}
}

func TestTamperedBindingCannotAuthorize(t *testing.T) {
	root := filepath.Join(t.TempDir(), "confirmation")
	store := Store{Root: root}
	action := Action{Action: "git.push", Target: "origin:refs/heads/topic", InputDigest: strings.Repeat("f", 64)}
	first, err := store.Authorize(testBinding(action))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Confirm("claude", "workspace-a", "owner-a", "session-a", "CONFIRM MAESTRO "+first.ChallengeID); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, StateFileName)
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	body = []byte(strings.Replace(string(body), `"input_hmac": "`, `"input_hmac": "tampered`, 1))
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := store.Authorize(testBinding(action))
	if err == nil || result.State != Denied {
		t.Fatalf("tampered state did not fail closed = %#v, %v", result, err)
	}
}

func TestPendingStateEditCannotAuthorizeWithoutRecordKey(t *testing.T) {
	now := time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)
	root := filepath.Join(t.TempDir(), "confirmation")
	store := Store{Root: root, Now: func() time.Time { return now }, TTL: 5 * time.Minute}
	action := Action{Action: "git.push", Target: "origin:refs/heads/topic", InputDigest: strings.Repeat("a", 64)}
	first, err := store.Authorize(testBinding(action))
	if err != nil || first.State != ChallengeRequired {
		t.Fatalf("first authorization = %#v, %v", first, err)
	}

	path := filepath.Join(root, StateFileName)
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var state stateFile
	if err := json.Unmarshal(body, &state); err != nil {
		t.Fatal(err)
	}
	state.Challenges[0].State = "confirmed"
	state.Challenges[0].ConfirmedAt = now
	body, err = json.MarshalIndent(state, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}

	result, err := store.Authorize(testBinding(action))
	if err == nil || result.State != Denied {
		t.Fatalf("edited pending challenge authorized = %#v, %v", result, err)
	}
}

func TestChallengeTimestampEditFailsClosed(t *testing.T) {
	now := time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)
	root := filepath.Join(t.TempDir(), "confirmation")
	store := Store{Root: root, Now: func() time.Time { return now }, TTL: time.Minute}
	action := Action{Action: "git.push", Target: "origin:refs/heads/topic", InputDigest: strings.Repeat("b", 64)}
	if _, err := store.Authorize(testBinding(action)); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(root, StateFileName)
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var state stateFile
	if err := json.Unmarshal(body, &state); err != nil {
		t.Fatal(err)
	}
	state.Challenges[0].ExpiresAt = state.Challenges[0].ExpiresAt.Add(24 * time.Hour)
	body, err = json.MarshalIndent(state, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}

	result, err := store.Authorize(testBinding(action))
	if err == nil || result.State != Denied {
		t.Fatalf("edited expiry remained trusted = %#v, %v", result, err)
	}
}

func TestConfirmedRecordDuplicationCannotReplayAuthorization(t *testing.T) {
	now := time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)
	root := filepath.Join(t.TempDir(), "confirmation")
	store := Store{Root: root, Now: func() time.Time { return now }, TTL: 5 * time.Minute}
	action := Action{Action: "git.push", Target: "origin:refs/heads/topic", InputDigest: strings.Repeat("c", 64)}
	first, err := store.Authorize(testBinding(action))
	if err != nil {
		t.Fatal(err)
	}
	if confirmed, err := store.Confirm("claude", "workspace-a", "owner-a", "session-a", "CONFIRM MAESTRO "+first.ChallengeID); err != nil || !confirmed {
		t.Fatalf("confirmation = %v, %v", confirmed, err)
	}

	path := filepath.Join(root, StateFileName)
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var state stateFile
	if err := json.Unmarshal(body, &state); err != nil {
		t.Fatal(err)
	}
	state.Challenges = append(state.Challenges, state.Challenges[0])
	body, err = json.MarshalIndent(state, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}

	result, err := store.Authorize(testBinding(action))
	if err == nil || result.State != Denied {
		t.Fatalf("duplicated confirmed record remained authorized = %#v, %v", result, err)
	}
}
