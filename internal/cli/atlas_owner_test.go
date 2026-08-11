package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func ownerCLI(t *testing.T) (func(stdin string, args ...string) (map[string]any, string, int), string) {
	t.Helper()
	dataRoot := filepath.Join(t.TempDir(), "local", "BCGOS")
	if err := os.MkdirAll(dataRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	resolve := func() (string, error) { return dataRoot, nil }
	return func(stdin string, args ...string) (map[string]any, string, int) {
		t.Helper()
		var out, errOut bytes.Buffer
		code := runAtlas(args, strings.NewReader(stdin), &out, &errOut, resolve)
		decoded := map[string]any{}
		if out.Len() > 0 {
			if err := json.Unmarshal(out.Bytes(), &decoded); err != nil {
				t.Fatalf("output for %v is not JSON: %v\n%s", args, err, out.String())
			}
		}
		return decoded, errOut.String(), code
	}, dataRoot
}

func TestAtlasOwnerRoundTripThroughTheCLI(t *testing.T) {
	run, _ := ownerCLI(t)

	if _, stderr, code := run("", "owner", "init"); code != ExitOK {
		t.Fatalf("owner init failed: code=%d stderr=%s", code, stderr)
	}

	created, stderr, code := run("# Retro\n\n## Evidence\n", "owner", "create-page",
		"--page", "development/retros/2026-08-11.md", "--key", "retro-w33", "--session", "s1", "--stdin")
	if code != ExitOK {
		t.Fatalf("create-page failed: code=%d stderr=%s", code, stderr)
	}
	if created["state"] != "written" {
		t.Fatalf("create-page state = %v, want written", created["state"])
	}

	appended, stderr, code := run("- led the steering committee", "owner", "append-entry",
		"--page", "development/retros/2026-08-11.md", "--section", "## Evidence",
		"--key", "evidence-1", "--session", "s1", "--stdin")
	if code != ExitOK {
		t.Fatalf("append-entry failed: code=%d stderr=%s", code, stderr)
	}
	if appended["state"] != "written" {
		t.Fatalf("append-entry state = %v, want written", appended["state"])
	}

	projection, stderr, code := run("", "owner", "collect",
		"--purpose", "confirm the round trip", "--reader", "owner_session",
		"--page", "development/retros/2026-08-11.md")
	if code != ExitOK {
		t.Fatalf("collect failed: code=%d stderr=%s", code, stderr)
	}
	pages, _ := projection["pages"].([]any)
	if len(pages) != 1 {
		t.Fatalf("collect returned %d pages, want 1: %v", len(pages), projection)
	}
	page, _ := pages[0].(map[string]any)
	content, _ := page["content"].(string)
	if !strings.Contains(content, "led the steering committee") {
		t.Fatalf("collect did not return the appended entry: %q", content)
	}
}

func TestAtlasOwnerWriteRefusesContentInArguments(t *testing.T) {
	run, _ := ownerCLI(t)
	if _, stderr, code := run("ignored", "owner", "create-page",
		"--page", "index.md", "--key", "k", "--session", "s1"); code == ExitOK {
		t.Fatalf("create-page accepted a write without --stdin: stderr=%s", stderr)
	}
}

func TestAtlasOwnerWriteRequiresExactlyOneAuthority(t *testing.T) {
	run, _ := ownerCLI(t)

	if _, _, code := run("body\n", "owner", "create-page",
		"--page", "index.md", "--key", "k", "--stdin"); code == ExitOK {
		t.Fatal("create-page accepted a write with no authority")
	}
	if _, _, code := run("body\n", "owner", "create-page",
		"--page", "index.md", "--key", "k", "--stdin",
		"--session", "s1", "--grant", "g1", "--occurrence", "o1"); code == ExitOK {
		t.Fatal("create-page accepted both an attended session and a standing grant")
	}
}

func TestAtlasCollectRefusesAnUnnamedProjection(t *testing.T) {
	run, _ := ownerCLI(t)
	if _, stderr, code := run("", "owner", "collect", "--purpose", "everything"); code == ExitOK {
		t.Fatalf("collect returned a projection with no named page: stderr=%s", stderr)
	}
}

func TestAtlasGrantLifecycleThroughTheCLI(t *testing.T) {
	run, _ := ownerCLI(t)

	created, stderr, code := run("", "grant", "create",
		"--ritual", "retro", "--ritual-version", "1",
		"--segment", "development/retros/", "--cadence", "weekly",
		"--operation", "create-page", "--operation", "append-entry")
	if code != ExitOK {
		t.Fatalf("grant create failed: code=%d stderr=%s", code, stderr)
	}
	if created["state"] != "active" {
		t.Fatalf("new grant state = %v, want active", created["state"])
	}
	if created["scheduled"] != false {
		t.Fatalf("grant reported scheduled = %v; nothing drives owner rituals yet", created["scheduled"])
	}
	grant, _ := created["grant"].(map[string]any)
	grantID, _ := grant["grant_id"].(string)
	if grantID == "" {
		t.Fatalf("grant create returned no identity: %v", created)
	}

	listed, _, code := run("", "grant", "list")
	if code != ExitOK {
		t.Fatal("grant list failed")
	}
	if grants, _ := listed["grants"].([]any); len(grants) != 1 {
		t.Fatalf("grant list returned %v, want exactly the one created", listed)
	}

	paused, _, code := run("", "grant", "pause", "--grant", grantID)
	if code != ExitOK || paused["state"] != "paused" {
		t.Fatalf("pause did not take: code=%d state=%v", code, paused["state"])
	}
	resumed, _, code := run("", "grant", "resume", "--grant", grantID)
	if code != ExitOK || resumed["state"] != "active" {
		t.Fatalf("resume did not take: code=%d state=%v", code, resumed["state"])
	}
	revoked, _, code := run("", "grant", "revoke", "--grant", grantID)
	if code != ExitOK || revoked["state"] != "revoked" {
		t.Fatalf("revoke did not take: code=%d state=%v", code, revoked["state"])
	}
	if _, _, code := run("", "grant", "resume", "--grant", grantID); code == ExitOK {
		t.Fatal("a revoked grant was resumed through the CLI")
	}
}
