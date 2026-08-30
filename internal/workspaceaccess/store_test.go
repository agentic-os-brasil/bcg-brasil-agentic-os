package workspaceaccess

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	basememory "github.com/agentic-os-brasil/bcg-brasil-agentic-os/bundles/base/memory"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/memory"
)

func TestGrantRequiresExplicitBoundedAuthority(t *testing.T) {
	now := time.Date(2026, 8, 29, 15, 0, 0, 0, time.UTC)
	store := Store{Root: t.TempDir(), Clock: func() time.Time { return now }, Random: bytes.NewReader(bytes.Repeat([]byte{0x31}, 64))}
	source := testAuthority("a", "1")
	target := testAuthority("b", "2")
	identity := DeriveIdentity("owner", "device")

	request := GrantRequest{
		Runtime: "claude", Source: source, Target: target,
		Purpose: PurposeReferenceContext, Sources: []string{SourceContext, SourceMemory}, TTL: 30 * time.Minute,
	}
	if _, err := store.Grant(request, identity); !errors.Is(err, ErrConfirmationRequired) {
		t.Fatalf("grant without confirmation error = %v", err)
	}
	request.Confirmed = true
	grant, err := store.Grant(request, identity)
	if err != nil {
		t.Fatal(err)
	}
	if grant.State != StateActive || grant.SourceWorkspaceID != source.WorkspaceID || grant.TargetWorkspaceID != target.WorkspaceID || grant.ExpiresAt.Sub(grant.IssuedAt) != 30*time.Minute {
		t.Fatalf("unexpected grant: %#v", grant)
	}

	for _, invalid := range []GrantRequest{
		{Runtime: "claude", Source: source, Target: source, Purpose: PurposeReferenceContext, Sources: []string{SourceContext}, TTL: 30 * time.Minute, Confirmed: true},
		{Runtime: "claude", Source: source, Target: target, Purpose: "arbitrary", Sources: []string{SourceContext}, TTL: 30 * time.Minute, Confirmed: true},
		{Runtime: "claude", Source: source, Target: target, Purpose: PurposeReferenceContext, Sources: []string{"checkout"}, TTL: 30 * time.Minute, Confirmed: true},
		{Runtime: "claude", Source: source, Target: target, Purpose: PurposeReferenceContext, Sources: []string{SourceContext}, TTL: 4 * time.Minute, Confirmed: true},
		{Runtime: "claude", Source: source, Target: target, Purpose: PurposeReferenceContext, Sources: []string{SourceContext}, TTL: 121 * time.Minute, Confirmed: true},
	} {
		if _, err := store.Grant(invalid, identity); err == nil {
			t.Fatalf("invalid grant was accepted: %#v", invalid)
		}
	}
}

func TestReadIsBoundedPrivateProjectionAndNeverCheckoutAccess(t *testing.T) {
	now := time.Date(2026, 8, 29, 15, 0, 0, 0, time.UTC)
	root := t.TempDir()
	store := Store{Root: root, Clock: func() time.Time { return now }, Random: bytes.NewReader(bytes.Repeat([]byte{0x42}, 64))}
	source := testAuthority("a", "1")
	target := testAuthority("b", "2")
	identity := DeriveIdentity("owner", "device")
	privateRoot := filepath.Join(root, "workspaces", target.WorkspaceID)
	writePrivate(t, filepath.Join(privateRoot, "context", "session-context.md"), "target-context-sentinel")
	writePrivate(t, filepath.Join(privateRoot, "continuity", "active.md"), "target-continuity-sentinel")
	checkout := filepath.Join(root, "synthetic-target-checkout")
	writePrivate(t, filepath.Join(checkout, "secret.txt"), "checkout-secret-must-not-leak")

	grant, err := store.Grant(GrantRequest{
		Runtime: "codex", Source: source, Target: target, Purpose: PurposeCompareImplementation,
		Sources: []string{SourceContext, SourceContinuity}, TTL: 30 * time.Minute, Confirmed: true,
	}, identity)
	if err != nil {
		t.Fatal(err)
	}
	resolve := func(_ context.Context, runtimeName, workspaceID string) (Authority, error) {
		if runtimeName != "codex" || workspaceID != target.WorkspaceID {
			return Authority{}, errors.New("unexpected target")
		}
		return target, nil
	}
	result, err := store.Read(context.Background(), "codex", source, grant.GrantID, identity, resolve)
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	for _, wanted := range []string{"target-context-sentinel", "target-continuity-sentinel"} {
		if !strings.Contains(text, wanted) {
			t.Fatalf("read omitted %q: %s", wanted, text)
		}
	}
	for _, forbidden := range []string{"checkout-secret-must-not-leak", root, checkout} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("read leaked %q: %s", forbidden, text)
		}
	}
	if len(body) > MaximumResponseBytes {
		t.Fatalf("response size = %d", len(body))
	}
}

func TestGrantFailsClosedOnTamperExpiryRevocationAndReplay(t *testing.T) {
	now := time.Date(2026, 8, 29, 15, 0, 0, 0, time.UTC)
	root := t.TempDir()
	random := append(bytes.Repeat([]byte{0x53}, 48), bytes.Repeat([]byte{0x54}, 16)...)
	store := Store{Root: root, Clock: func() time.Time { return now }, Random: bytes.NewReader(random)}
	source := testAuthority("a", "1")
	target := testAuthority("b", "2")
	identity := DeriveIdentity("owner", "device")
	grant, err := store.Grant(GrantRequest{Runtime: "claude", Source: source, Target: target, Purpose: PurposeReuseLearning, Sources: []string{SourceContext}, TTL: 5 * time.Minute, Confirmed: true}, identity)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := store.Status("codex", source, grant.GrantID, identity); err == nil {
		t.Fatal("cross-runtime replay was accepted")
	}
	if _, err := store.Status("claude", testAuthority("c", "3"), grant.GrantID, identity); err == nil {
		t.Fatal("cross-source replay was accepted")
	}
	if _, err := store.Status("claude", source, grant.GrantID, DeriveIdentity("other", "device")); err == nil {
		t.Fatal("cross-principal replay was accepted")
	}

	now = now.Add(5 * time.Minute)
	status, err := store.Status("claude", source, grant.GrantID, identity)
	if err != nil || status.State != StateExpired {
		t.Fatalf("expiry status=%#v err=%v", status, err)
	}
	now = now.Add(-time.Minute)
	status, err = store.Revoke("claude", source, grant.GrantID, identity)
	if err != nil || status.State != StateRevoked {
		t.Fatalf("revoke status=%#v err=%v", status, err)
	}
	if _, err := store.Read(context.Background(), "claude", source, grant.GrantID, identity, func(context.Context, string, string) (Authority, error) { return target, nil }); !errors.Is(err, ErrRevoked) {
		t.Fatalf("revoked read error=%v", err)
	}

	second, err := store.Grant(GrantRequest{Runtime: "claude", Source: source, Target: target, Purpose: PurposeReuseLearning, Sources: []string{SourceContext}, TTL: 5 * time.Minute, Confirmed: true}, identity)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "workspaces", source.WorkspaceID, "access", "grants", second.GrantID+".json")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	body = bytes.Replace(body, []byte(PurposeReuseLearning), []byte(PurposeReferenceContext), 1)
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Status("claude", source, second.GrantID, identity); err == nil || !strings.Contains(err.Error(), "integrity") {
		t.Fatalf("tampered grant error=%v", err)
	}
}

func TestGrantRecordAndPrivateReadersExcludeRawIdentityAndFailClosed(t *testing.T) {
	now := time.Date(2026, 8, 29, 15, 0, 0, 0, time.UTC)
	root := t.TempDir()
	store := Store{Root: root, Clock: func() time.Time { return now }, Random: bytes.NewReader(bytes.Repeat([]byte{0x61}, 64))}
	source := testAuthority("a", "1")
	target := testAuthority("b", "2")
	identity := DeriveIdentity("raw-owner-name", "raw-device-name")
	grant, err := store.Grant(GrantRequest{Runtime: "claude", Source: source, Target: target, Purpose: PurposeDependencyCoordination, Sources: []string{SourceContext}, TTL: 30 * time.Minute, Confirmed: true}, identity)
	if err != nil {
		t.Fatal(err)
	}
	recordPath := filepath.Join(root, "workspaces", source.WorkspaceID, "access", "grants", grant.GrantID+".json")
	record, err := os.ReadFile(recordPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"raw-owner-name", "raw-device-name", root} {
		if strings.Contains(string(record), forbidden) {
			t.Fatalf("grant record leaked %q: %s", forbidden, record)
		}
	}

	external := filepath.Join(root, "external-context.md")
	if err := os.WriteFile(external, []byte("symlink-secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	contextDir := filepath.Join(root, "workspaces", target.WorkspaceID, "context")
	if err := os.MkdirAll(contextDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, filepath.Join(contextDir, "session-context.md")); err == nil {
		_, err = store.Read(context.Background(), "claude", source, grant.GrantID, identity, func(context.Context, string, string) (Authority, error) { return target, nil })
		if err == nil || !strings.Contains(err.Error(), "symlink") {
			t.Fatalf("symlink source error=%v", err)
		}
	}
	if err := os.Remove(filepath.Join(contextDir, "session-context.md")); err == nil {
		if err := os.WriteFile(filepath.Join(contextDir, "session-context.md"), bytes.Repeat([]byte("x"), 2049), 0o600); err != nil {
			t.Fatal(err)
		}
		_, err = store.Read(context.Background(), "claude", source, grant.GrantID, identity, func(context.Context, string, string) (Authority, error) { return target, nil })
		if err == nil || !strings.Contains(err.Error(), "bounded") {
			t.Fatalf("oversize source error=%v", err)
		}
	}
}

func TestReadUsesCanonicalGeneratedMemoryRatherThanLegacyMemoryFile(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	root := t.TempDir()
	store := Store{Root: root, Clock: func() time.Time { return now }, Random: bytes.NewReader(bytes.Repeat([]byte{0x71}, 64))}
	source := testAuthority("a", "1")
	target := testAuthority("b", "2")
	identity := DeriveIdentity("owner", "device")
	policy, err := basememory.Policy()
	if err != nil {
		t.Fatal(err)
	}
	runtimeConfig, err := basememory.Runtime()
	if err != nil {
		t.Fatal(err)
	}
	engine := memory.Engine{Root: root, Policy: policy, Budgets: runtimeConfig.ContextBudgets(), Synthesizer: memorySynthesizer("canonical-memory-sentinel"), SynthesizerID: "workspace-access-test-v1", Now: func() time.Time { return now }}
	if _, err := engine.Capture(memory.Capture{WorkspaceID: target.WorkspaceID, RecordedAt: now, Kind: "decision", Text: "sanitized", Sanitized: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.DreamDaily(context.Background(), target.WorkspaceID, now); err != nil {
		t.Fatal(err)
	}
	writePrivate(t, filepath.Join(root, "workspaces", target.WorkspaceID, "memory", "session-context.md"), "legacy-memory-must-not-leak")
	grant, err := store.Grant(GrantRequest{Runtime: "codex", Source: source, Target: target, Purpose: PurposeReuseLearning, Sources: []string{SourceMemory}, TTL: 30 * time.Minute, Confirmed: true}, identity)
	if err != nil {
		t.Fatal(err)
	}
	result, err := store.Read(context.Background(), "codex", source, grant.GrantID, identity, func(context.Context, string, string) (Authority, error) { return target, nil })
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(result)
	if !strings.Contains(string(body), "canonical-memory-sentinel") || strings.Contains(string(body), "legacy-memory-must-not-leak") {
		t.Fatalf("memory result did not use canonical assembler: %s", body)
	}
	store.Clock = nil
	if _, err := store.Read(context.Background(), "codex", source, grant.GrantID, identity, func(context.Context, string, string) (Authority, error) { return target, nil }); err != nil {
		t.Fatalf("production nil-clock memory read failed: %v", err)
	}
}

func TestReadRejectsTargetIdentityDriftAndMalformedRecord(t *testing.T) {
	store := Store{Root: t.TempDir(), Random: bytes.NewReader(bytes.Repeat([]byte{0x81}, 64))}
	source := testAuthority("a", "1")
	target := testAuthority("b", "2")
	identity := DeriveIdentity("owner", "device")
	grant, err := store.Grant(GrantRequest{Runtime: "claude", Source: source, Target: target, Purpose: PurposeReferenceContext, Sources: []string{SourceContext}, TTL: 30 * time.Minute, Confirmed: true}, identity)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Read(context.Background(), "claude", source, grant.GrantID, identity, func(context.Context, string, string) (Authority, error) {
		return Authority{WorkspaceID: target.WorkspaceID, RepositoryID: strings.Repeat("9", 32)}, nil
	}); err == nil || !strings.Contains(err.Error(), "identity changed") {
		t.Fatalf("target identity drift error=%v", err)
	}
	path := filepath.Join(store.Root, "workspaces", source.WorkspaceID, "access", "grants", grant.GrantID+".json")
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString("{}\n"); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Status("claude", source, grant.GrantID, identity); err == nil || !strings.Contains(err.Error(), "trailing") {
		t.Fatalf("trailing record error=%v", err)
	}
}

func TestGrantStoreCapacityIsBoundedAndCrashTemporaryDoesNotConsumeGrantSlot(t *testing.T) {
	root := t.TempDir()
	store := Store{Root: root, Random: bytes.NewReader(bytes.Repeat([]byte{0x91}, 64))}
	source := testAuthority("a", "1")
	target := testAuthority("b", "2")
	identity := DeriveIdentity("owner", "device")
	directory := filepath.Join(root, "workspaces", source.WorkspaceID, "access", "grants")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, ".workspace-access-crash.tmp"), []byte("partial"), 0o600); err != nil {
		t.Fatal(err)
	}
	for index := 0; index < maximumGrantCount-1; index++ {
		name := fmt.Sprintf("%032x.json", index+1)
		if err := os.WriteFile(filepath.Join(directory, name), []byte("{}"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.Grant(GrantRequest{Runtime: "codex", Source: source, Target: target, Purpose: PurposeReferenceContext, Sources: []string{SourceContext}, TTL: 30 * time.Minute, Confirmed: true}, identity); err != nil {
		t.Fatalf("temporary file incorrectly consumed capacity: %v", err)
	}
	store.Random = bytes.NewReader(bytes.Repeat([]byte{0x92}, 16))
	if _, err := store.Grant(GrantRequest{Runtime: "codex", Source: source, Target: target, Purpose: PurposeReferenceContext, Sources: []string{SourceContext}, TTL: 30 * time.Minute, Confirmed: true}, identity); err == nil || !strings.Contains(err.Error(), "capacity") {
		t.Fatalf("capacity error=%v", err)
	}
}

func TestUTF8ProjectionTruncationPreservesValidText(t *testing.T) {
	value := strings.Repeat("á", 2048)
	truncated := truncateUTF8Bytes(value, 3071)
	if len(truncated) > 3071 || !utf8.ValidString(truncated) || strings.ContainsRune(truncated, utf8.RuneError) {
		t.Fatalf("invalid UTF-8 truncation: bytes=%d valid=%t", len(truncated), utf8.ValidString(truncated))
	}
}

type memorySynthesizer string

func (value memorySynthesizer) Synthesize(context.Context, memory.SynthesisRequest) (string, error) {
	return string(value), nil
}

func testAuthority(workspaceByte, repositoryByte string) Authority {
	return Authority{WorkspaceID: strings.Repeat(workspaceByte, 32), RepositoryID: strings.Repeat(repositoryByte, 32)}
}

func writePrivate(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}
