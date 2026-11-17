package product

import (
	"encoding/json"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ownerctx"
	"strings"
	"testing"
	"time"
)

func TestHistoryReviewMaximumEnvelopeBound(t *testing.T) {
	receipt := ownerctx.PromptHistoryReceipt{ID: strings.Repeat("I", 128), OwnerID: strings.Repeat("O", 128), RecordedAt: time.Date(9999, 12, 31, 23, 59, 59, 999999999, time.FixedZone("", 23*3600+59*60)), Language: "abcdefgh-abcdefgh", Source: "claude", SessionID: strings.Repeat("S", 128), ScopeKind: ownerctx.PromptScopeWorkspace, ScopeID: strings.Repeat("W", 128), SHA256: strings.Repeat("f", 64), Bytes: 64 << 10}
	removed := make([]ownerctx.PromptHistoryReceipt, 10000)
	for i := range removed {
		removed[i] = receipt
	}
	config := ownerctx.PromptHistoryConfig{SchemaVersion: 1, OwnerID: strings.Repeat("O", 128), MaxEntries: 10000, MaxBytes: 8 << 20, MaxAgeSeconds: 10 * 365 * 86400}
	envelope := historyReviewEnvelope{Binding: historyBinding{Home: strings.Repeat("<", 4096), Workspace: strings.Repeat("<", 4096), WorkspaceID: strings.Repeat("a", 32), IdentityRevision: strings.Repeat("f", 64), ScopeKind: "workspace", ScopeID: strings.Repeat("W", 128), SetupRevision: strings.Repeat("f", 64)}, Review: ownerctx.PromptHistoryReview{Root: strings.Repeat("<", 4096), Operation: "reset", BeforeRevision: strings.Repeat("f", 64), AfterRevision: strings.Repeat("f", 64), CurrentConfig: config, ProposedConfig: config, Removed: removed, Digest: strings.Repeat("f", 64)}}
	envelope.Review.Record = &ownerctx.PromptHistoryEntry{SchemaVersion: 1, ID: strings.Repeat("I", 128), OwnerID: strings.Repeat("O", 128), OccurrenceID: strings.Repeat("e", 128), RecordedAt: receipt.RecordedAt, Language: receipt.Language, Source: receipt.Source, SessionID: receipt.SessionID, ScopeKind: receipt.ScopeKind, ScopeID: receipt.ScopeID, SHA256: receipt.SHA256, ContentKind: "user_prompt", Prompt: strings.Repeat("\x00", 32<<10)}
	body, err := json.MarshalIndent(envelope, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("maximal removed metadata envelope: %d bytes", len(body))
	if len(body) > maximumHistoryReviewBytes {
		t.Fatalf("full legal store cannot be reviewed: %d > %d", len(body), maximumHistoryReviewBytes)
	}
}
