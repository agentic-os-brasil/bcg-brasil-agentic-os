package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/selfmodel"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/walterselfreview"
)

func TestWalterSelfReviewCommandFailsUnavailableAndReturnsMetadataReceipt(t *testing.T) {
	now := time.Unix(10, 0).UTC()
	snapshot, err := selfmodel.NewCanonicalSnapshot(1, map[string]string{"voice": selfmodel.Digest("voice")}, now)
	if err != nil {
		t.Fatal(err)
	}
	original := "Please review this week's learning."
	working := "Review this week's learning."
	request := walterselfreview.Request{
		SchemaVersion: walterselfreview.SchemaVersion, WeekID: "2026-W31",
		PromptWindow: walterselfreview.PromptWindow{SchemaVersion: walterselfreview.SchemaVersion, Entries: []walterselfreview.PromptWindowEntry{{
			Sequence: 1, OriginalText: original, OriginalSHA256: walterselfreview.Digest(original), WorkingText: working,
			WorkingSHA256: walterselfreview.Digest(working), SourceEventSHA256: walterselfreview.Digest("prompt"), OccurredAt: now, Current: true,
		}}},
		CanonicalSnapshot: snapshot,
	}
	body, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	code := RunWithInput([]string{"agent", "walter", "self-review", "--stdin"}, bytes.NewReader(body), &output, &output)
	if code != ExitUnavailable || !strings.Contains(output.String(), `"state": "unavailable"`) || strings.Contains(output.String(), original) || strings.Contains(output.String(), working) {
		t.Fatalf("Walter self-review command = %d, output=%s", code, output.String())
	}
}

func TestWalterSelfReviewCommandRequiresStdin(t *testing.T) {
	var output bytes.Buffer
	if code := RunWithInput([]string{"agent", "walter", "self-review"}, strings.NewReader("{}"), &output, &output); code != ExitUsage || !strings.Contains(output.String(), "--stdin") {
		t.Fatalf("missing stdin guard = %d, output=%s", code, output.String())
	}
}
