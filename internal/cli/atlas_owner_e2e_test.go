package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The vertical has to prove three things end to end: a ritual writes a
// learning, a later session starts with nothing carried over in memory, and
// that session recovers what was written. Each invocation below goes through
// RunWithInput, the same entry point cmd/bcgos calls, so nothing here shares an
// engine, a cache or a handle with the invocation before it.
//
// This is a session-boundary proof, not a scheduler-driven one. No scheduled
// occurrence exists yet: a standing grant authorizes an occurrence and reports
// Scheduled false, so the ritual is woken by the owner. The test says so rather
// than implying a cadence the system does not have.
func TestOwnerAtlasSurvivesASessionBoundary(t *testing.T) {
	home := t.TempDir()
	dataRoot := filepath.Join(home, "local", "BCGOS")
	if err := os.MkdirAll(dataRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LOCALAPPDATA", filepath.Join(home, "local"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, "local"))
	t.Setenv("HOME", home)

	// Each call is one session: a fresh argv through the real entry point.
	session := func(stdin string, args ...string) (map[string]any, string, int) {
		t.Helper()
		var out, errOut bytes.Buffer
		code := RunWithInput(args, strings.NewReader(stdin), &out, &errOut)
		decoded := map[string]any{}
		if out.Len() > 0 {
			if err := json.Unmarshal(out.Bytes(), &decoded); err != nil {
				t.Fatalf("session %v produced non-JSON output: %v\n%s", args, err, out.String())
			}
		}
		return decoded, errOut.String(), code
	}

	// Session 1 — the owner authorizes a weekly ritual over their retros.
	granted, stderr, code := session("", "atlas", "grant", "create",
		"--ritual", "retro", "--ritual-version", "1",
		"--segment", "development/retros/", "--cadence", "weekly",
		"--operation", "create-page", "--operation", "append-entry")
	if code != ExitOK {
		t.Fatalf("session 1 (grant create) failed: code=%d stderr=%s", code, stderr)
	}
	grant, _ := granted["grant"].(map[string]any)
	grantID, _ := grant["grant_id"].(string)
	if grantID == "" {
		t.Fatalf("session 1 returned no grant identity: %v", granted)
	}
	if granted["scheduled"] != false {
		t.Fatalf("grant claims to be scheduled; nothing wakes owner rituals yet: %v", granted["scheduled"])
	}

	// Session 2 — an occurrence of the ritual writes the retrospective.
	page := "development/retros/2026-08-11.md"
	written, stderr, code := session("# Retro — week 33\n\n## Learning\n", "atlas", "owner", "create-page",
		"--page", page, "--key", "retro-2026-W33",
		"--grant", grantID, "--occurrence", "2026-W33", "--stdin")
	if code != ExitOK {
		t.Fatalf("session 2 (create-page) failed: code=%d stderr=%s", code, stderr)
	}
	if written["state"] != "written" {
		t.Fatalf("session 2 state = %v, want written", written["state"])
	}

	// Session 3 — the same occurrence records the learning itself.
	learning := "- procurement transformations stall at the category-owner layer"
	appended, stderr, code := session(learning, "atlas", "owner", "append-entry",
		"--page", page, "--section", "## Learning", "--key", "retro-2026-W33-learning",
		"--grant", grantID, "--occurrence", "2026-W33", "--stdin")
	if code != ExitOK {
		t.Fatalf("session 3 (append-entry) failed: code=%d stderr=%s", code, stderr)
	}
	if appended["state"] != "written" {
		t.Fatalf("session 3 state = %v, want written", appended["state"])
	}

	// Session 4 — a later session, holding nothing from the ones before,
	// recovers the learning.
	projection, stderr, code := session("", "atlas", "owner", "collect",
		"--purpose", "recover the learning in a new session",
		"--reader", "owner_session", "--page", page)
	if code != ExitOK {
		t.Fatalf("session 4 (collect) failed: code=%d stderr=%s", code, stderr)
	}
	pages, _ := projection["pages"].([]any)
	if len(pages) != 1 {
		t.Fatalf("session 4 recovered %d pages, want 1: %v", len(pages), projection)
	}
	recovered, _ := pages[0].(map[string]any)
	content, _ := recovered["content"].(string)
	if !strings.Contains(content, "category-owner layer") {
		t.Fatalf("the learning did not survive the session boundary: %q", content)
	}

	// Session 5 — replaying the occurrence must not duplicate the learning.
	replayed, _, code := session(learning, "atlas", "owner", "append-entry",
		"--page", page, "--section", "## Learning", "--key", "retro-2026-W33-learning",
		"--grant", grantID, "--occurrence", "2026-W33", "--stdin")
	if code != ExitOK {
		t.Fatal("session 5 (replay) failed")
	}
	if replayed["state"] != "unchanged" {
		t.Fatalf("replaying the occurrence reported %v, want unchanged", replayed["state"])
	}
	after, _, code := session("", "atlas", "owner", "collect",
		"--purpose", "confirm the replay changed nothing",
		"--reader", "owner_session", "--page", page)
	if code != ExitOK {
		t.Fatal("session 5 (collect) failed")
	}
	afterPages, _ := after["pages"].([]any)
	afterPage, _ := afterPages[0].(map[string]any)
	afterContent, _ := afterPage["content"].(string)
	if occurrences := strings.Count(afterContent, "category-owner layer"); occurrences != 1 {
		t.Fatalf("the learning appears %d times after a replay, want 1:\n%s", occurrences, afterContent)
	}

	// Session 6 — the owner takes the authority back. The content stays.
	revoked, _, code := session("", "atlas", "grant", "revoke", "--grant", grantID)
	if code != ExitOK || revoked["state"] != "revoked" {
		t.Fatalf("session 6 (revoke) failed: code=%d state=%v", code, revoked["state"])
	}
	if _, stderr, code := session("more\n", "atlas", "owner", "append-entry",
		"--page", page, "--section", "## Learning", "--key", "after-revocation",
		"--grant", grantID, "--occurrence", "2026-W34", "--stdin"); code == ExitOK {
		t.Fatalf("a write citing a revoked grant was accepted: stderr=%s", stderr)
	}
	survived, _, code := session("", "atlas", "owner", "collect",
		"--purpose", "confirm revocation removed no content",
		"--reader", "owner_session", "--page", page)
	if code != ExitOK {
		t.Fatal("session 6 (collect) failed")
	}
	if survivedPages, _ := survived["pages"].([]any); len(survivedPages) != 1 {
		t.Fatalf("revoking the grant removed owner content: %v", survived)
	}
}
