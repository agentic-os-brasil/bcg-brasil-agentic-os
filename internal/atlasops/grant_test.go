package atlasops

import (
	"testing"
	"time"
)

func weeklyRetro() GrantRequest {
	return GrantRequest{
		Ritual:        "retro",
		RitualVersion: "1",
		Segment:       "development/retros/",
		Operations:    []string{"create-page", "append-entry"},
		Cadence:       "weekly",
		CatchUp:       CatchUpSingle,
		Reader:        ReaderOwnerSession,
		Retention:     "owner_managed",
	}
}

func TestCreateGrantIsInspectableAndPersists(t *testing.T) {
	engine := testEngine(t)

	grant, err := engine.CreateGrant(weeklyRetro())
	if err != nil {
		t.Fatalf("create grant: %v", err)
	}
	if grant.GrantID == "" || grant.CreatedAt.IsZero() {
		t.Fatalf("grant is not identifiable: %+v", grant)
	}
	if grant.StateAt(engine.now()) != GrantActive {
		t.Fatalf("new grant state = %q, want %q", grant.StateAt(engine.now()), GrantActive)
	}

	reopened, err := Open(engine.dataRoot, engine.now)
	if err != nil {
		t.Fatal(err)
	}
	found, ok, err := reopened.Grant(grant.GrantID)
	if err != nil || !ok {
		t.Fatalf("grant did not survive a reopen: ok=%v err=%v", ok, err)
	}
	if found.Ritual != "retro" || found.Cadence != "weekly" {
		t.Fatalf("reopened grant lost its binding: %+v", found)
	}
}

func TestGrantAuthorizesOnlyItsOwnSegmentAndOperations(t *testing.T) {
	engine := testEngine(t)
	grant, err := engine.CreateGrant(weeklyRetro())
	if err != nil {
		t.Fatal(err)
	}

	if err := engine.AuthorizeGrant(grant.GrantID, "append-entry", "development/retros/2026-08-11.md"); err != nil {
		t.Fatalf("granted operation inside the segment was refused: %v", err)
	}
	if err := engine.AuthorizeGrant(grant.GrantID, "append-entry", "learnings/elsewhere.md"); err == nil {
		t.Fatal("grant authorized a page outside its segment")
	}
	if err := engine.AuthorizeGrant(grant.GrantID, "delete", "development/retros/2026-08-11.md"); err == nil {
		t.Fatal("grant authorized an operation outside its allowed set")
	}
	if err := engine.AuthorizeGrant("grant-that-does-not-exist", "append-entry", "development/retros/x.md"); err == nil {
		t.Fatal("an unknown grant authorized a write")
	}
}

func TestGrantPauseAndResume(t *testing.T) {
	engine := testEngine(t)
	grant, err := engine.CreateGrant(weeklyRetro())
	if err != nil {
		t.Fatal(err)
	}
	page := "development/retros/2026-08-11.md"

	if err := engine.PauseGrant(grant.GrantID); err != nil {
		t.Fatalf("pause grant: %v", err)
	}
	if err := engine.AuthorizeGrant(grant.GrantID, "append-entry", page); err == nil {
		t.Fatal("a paused grant still authorized a write")
	}
	paused, _, err := engine.Grant(grant.GrantID)
	if err != nil {
		t.Fatal(err)
	}
	if paused.StateAt(engine.now()) != GrantPaused {
		t.Fatalf("paused grant state = %q, want %q", paused.StateAt(engine.now()), GrantPaused)
	}

	if err := engine.ResumeGrant(grant.GrantID); err != nil {
		t.Fatalf("resume grant: %v", err)
	}
	if err := engine.AuthorizeGrant(grant.GrantID, "append-entry", page); err != nil {
		t.Fatalf("a resumed grant refused a write: %v", err)
	}
}

func TestRevokeGrantIsPermanentAndErasesNoContent(t *testing.T) {
	engine := testEngine(t)
	grant, err := engine.CreateGrant(weeklyRetro())
	if err != nil {
		t.Fatal(err)
	}
	page := "development/retros/2026-08-11.md"
	if _, err := engine.CreatePage(CreatePageRequest{
		Page:       page,
		Body:       "# Retro\n\n## Evidence\n- written under the grant\n",
		Provenance: Provenance{Origin: OriginGrant, GrantID: grant.GrantID, OccurrenceID: "w33", IdempotencyKey: "retro-w33"},
	}); err != nil {
		t.Fatal(err)
	}

	if err := engine.RevokeGrant(grant.GrantID); err != nil {
		t.Fatalf("revoke grant: %v", err)
	}
	if err := engine.AuthorizeGrant(grant.GrantID, "append-entry", page); err == nil {
		t.Fatal("a revoked grant still authorized a write")
	}
	if err := engine.ResumeGrant(grant.GrantID); err == nil {
		t.Fatal("a revoked grant was resumed; revocation must be permanent")
	}

	// Revoking the authority must not remove what was already written under it.
	projection, err := engine.Collect(CollectRequest{
		Purpose: "confirm content survived revocation",
		Reader:  ReaderOwnerSession,
		Pages:   []string{page},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(projection.Pages) != 1 {
		t.Fatalf("revocation removed owner content: %+v", projection)
	}
}

func TestExpiredGrantStopsAuthorizing(t *testing.T) {
	engine := testEngine(t)
	request := weeklyRetro()
	expiry := engine.now().Add(-time.Hour)
	request.ExpiresAt = &expiry

	grant, err := engine.CreateGrant(request)
	if err != nil {
		t.Fatal(err)
	}
	if err := engine.AuthorizeGrant(grant.GrantID, "append-entry", "development/retros/2026-08-11.md"); err == nil {
		t.Fatal("an expired grant authorized a write")
	}
	expired, _, err := engine.Grant(grant.GrantID)
	if err != nil {
		t.Fatal(err)
	}
	if expired.StateAt(engine.now()) != GrantExpired {
		t.Fatalf("expired grant state = %q, want %q", expired.StateAt(engine.now()), GrantExpired)
	}
}

func TestCreateGrantRejectsAnIncompleteBinding(t *testing.T) {
	engine := testEngine(t)
	for name, mutate := range map[string]func(*GrantRequest){
		"no ritual":     func(r *GrantRequest) { r.Ritual = "" },
		"no segment":    func(r *GrantRequest) { r.Segment = "" },
		"no operations": func(r *GrantRequest) { r.Operations = nil },
		"no cadence":    func(r *GrantRequest) { r.Cadence = "" },
		"escaping segment": func(r *GrantRequest) {
			r.Segment = "../"
		},
		"unknown operation": func(r *GrantRequest) { r.Operations = []string{"launch-missiles"} },
	} {
		request := weeklyRetro()
		mutate(&request)
		if _, err := engine.CreateGrant(request); err == nil {
			t.Fatalf("grant accepted an incomplete binding: %s", name)
		}
	}
}
