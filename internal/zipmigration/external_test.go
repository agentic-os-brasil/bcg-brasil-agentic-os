package zipmigration_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ownerctx"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/zipmigration"
)

func TestExternalOwnerAuthorityRequiresFreshEnrollment(t *testing.T) {
	o := externalFixture(t, "0.2.0")
	sourceBrain := filepath.Join(o.SourceRoot, "brain")
	if _, err := ownerctx.Initialize(sourceBrain); err != nil {
		t.Fatal(err)
	}
	status, err := ownerctx.Inspect(sourceBrain)
	if err != nil {
		t.Fatal(err)
	}
	var automatic []string
	for name, facet := range status.Facets {
		if facet.Refinement == "automatic_with_audit" {
			automatic = append(automatic, name)
		}
	}
	sort.Strings(automatic)
	if len(automatic) == 0 {
		t.Fatal("no public automatic facet available for authority test")
	}
	facet := automatic[0]
	facetPath := status.Facets[facet].Path
	original, err := os.ReadFile(filepath.Join(sourceBrain, facetPath))
	if err != nil {
		t.Fatal(err)
	}
	oldCapability, err := ownerctx.AuthorizeProducer(sourceBrain, "synthetic-source-adapter")
	if err != nil {
		t.Fatal(err)
	}
	// A public Submit interrupted before its audit write preserves a genuinely
	// authorized pending proposal; no registry/proposal JSON is manufactured.
	interruption := filepath.Join(sourceBrain, "owner/refinement/audits")
	externalPut(t, sourceBrain, "owner/refinement/audits", "synthetic interruption")
	input := ownerctx.RefinementInput{Facet: facet, Evidence: "synthetic reviewed source", ProposedBody: "# Reviewed style\n\n## Current\n\nPrefer concise evidence-led updates.\n", ProducerID: "synthetic-source-adapter", Capability: oldCapability, OccurrenceID: "synthetic-migration-pending"}
	if _, err = ownerctx.SubmitRefinement(sourceBrain, input); err == nil {
		t.Fatal("synthetic interrupted submit unexpectedly completed")
	}
	if err = os.Remove(interruption); err != nil {
		t.Fatal(err)
	}
	pending, err := ownerctx.SubmitRefinement(sourceBrain, input)
	if err != nil || pending.State != "proposed" {
		t.Fatal("public interrupted proposal was not retained", pending.State, err)
	}
	if current, err := os.ReadFile(filepath.Join(sourceBrain, facetPath)); err != nil || !bytes.Equal(current, original) {
		t.Fatal("interrupted source submit changed authored facet", err)
	}
	sourceControls := map[string][]byte{}
	for _, name := range []string{"owner/registry.json", "owner/refinement/proposals/" + pending.ID + ".json", "owner/refinement/versions/" + facet + "/" + pending.ID + ".before.md"} {
		body, err := os.ReadFile(filepath.Join(sourceBrain, name))
		if err != nil {
			t.Fatal("source control unavailable", name, err)
		}
		sourceControls[name] = body
	}
	p, err := zipmigration.PlanExternal(o)
	if err != nil {
		t.Fatal(err)
	}
	digest := externalDigest(t, p)
	if migrated, err := zipmigration.ApplyExternal(p, digest, nil); err != nil || !migrated.Current {
		t.Fatal("migration failed", err)
	}
	targetBrain := filepath.Join(o.Home, "brain")
	for name, original := range sourceControls {
		if _, err := os.Stat(filepath.Join(targetBrain, name)); !os.IsNotExist(err) {
			t.Error("historical owner control remained active", name)
		}
		retained, err := os.ReadFile(filepath.Join(o.Home, "lineage", p.AttemptID, "brain", name))
		if err != nil || !bytes.Equal(retained, original) {
			t.Error("historical owner control was not retained intact", name, err)
		}
	}
	if _, err = ownerctx.Initialize(targetBrain); err != nil {
		t.Fatal(err)
	}
	if applied, err := ownerctx.ApplyRefinement(targetBrain, pending.ID, false); err == nil || applied.State == "applied" {
		t.Error("historical authorized pending proposal reactivated after migration")
	}
	beforeReuse, err := os.ReadFile(filepath.Join(targetBrain, facetPath))
	if err != nil {
		t.Fatal(err)
	}
	reused, err := ownerctx.SubmitRefinement(targetBrain, ownerctx.RefinementInput{Facet: facet, Evidence: "synthetic relocated producer", ProposedBody: "# Relocated authority\n\n## Current\n\nThis historical capability must not auto-apply.\n", ProducerID: input.ProducerID, Capability: oldCapability})
	if err != nil {
		t.Fatal(err)
	}
	if reused.State == "applied" {
		t.Error("historical producer capability reactivated after migration")
	}
	afterReuse, err := os.ReadFile(filepath.Join(targetBrain, facetPath))
	if err != nil || !bytes.Equal(beforeReuse, afterReuse) || !bytes.Equal(afterReuse, original) {
		t.Error("historical owner authority changed migrated SELF", err)
	}
	// Explicit enrollment in the new brain restores the owning workflow; the
	// fresh capability stays in test memory and is never printed or persisted.
	freshCapability, err := ownerctx.AuthorizeProducer(targetBrain, input.ProducerID)
	if err != nil {
		t.Fatal(err)
	}
	fresh, err := ownerctx.SubmitRefinement(targetBrain, ownerctx.RefinementInput{Facet: facet, Evidence: "synthetic fresh enrollment", ProposedBody: "# Fresh authority\n\n## Current\n\nExplicitly enrolled after migration.\n", ProducerID: input.ProducerID, Capability: freshCapability})
	if err != nil || fresh.State != "applied" {
		t.Fatal("fresh explicit producer enrollment did not work", fresh.State, err)
	}
	if repeated, err := zipmigration.ApplyExternal(p, digest, nil); err != nil || !repeated.Current {
		t.Fatal("repeat changed migration", err)
	}
	if current, err := os.ReadFile(filepath.Join(targetBrain, facetPath)); err != nil || !strings.Contains(string(current), "Explicitly enrolled after migration.") {
		t.Error("repeat overwrote explicitly updated SELF", err)
	}
	if current, err := os.ReadFile(filepath.Join(sourceBrain, facetPath)); err != nil || !bytes.Equal(current, original) {
		t.Fatal("migration changed preserved source SELF", err)
	}
	for name, original := range sourceControls {
		body, err := os.ReadFile(filepath.Join(sourceBrain, name))
		if err != nil || !bytes.Equal(body, original) {
			t.Error("migration changed preserved source control", name, err)
		}
	}
}

func TestExternalOwnerControlsPreserveAuthoredNeighbors(t *testing.T) {
	for _, spelling := range []string{"canonical", "portable-case"} {
		t.Run(spelling, func(t *testing.T) {
			o := externalFixture(t, "0.2.0")
			controls := []string{"owner/registry.json", "owner/interview/confirmations.json", "owner/interview/drafts/review.json", "owner/refinement/proposals/pending.json", "owner/refinement/audits/prepared.json", "owner/refinement/reversions/prior.json", "owner/refinement/versions/voice/prior.before.md", "owner/observations/observations.jsonl"}
			for i, name := range controls {
				if spelling == "portable-case" {
					directory, base := filepath.Split(name)
					name = directory + strings.ToUpper(base)
					controls[i] = name
				}
				externalPut(t, o.SourceRoot, "brain/"+name, "historical machine state\n")
			}
			authored := []string{"owner/self/voice.md", "owner/operating/work-state.md", "owner/sources/assessment.json", "owner/prompt-history/entries.jsonl", "owner/observations/notes.md", "owner/registry-notes.json", "owner/interview-notes.md", "owner/refinement-notes.json"}
			for _, name := range authored {
				externalPut(t, o.SourceRoot, "brain/"+name, "authored source/history\n")
			}
			p, err := zipmigration.PlanExternal(o)
			if err != nil {
				t.Fatal(err)
			}
			digest := externalDigest(t, p)
			if _, err = zipmigration.ApplyExternal(p, digest, nil); err != nil {
				t.Fatal(err)
			}
			for _, name := range controls {
				if _, err := os.Stat(filepath.Join(o.Home, "brain", name)); !os.IsNotExist(err) {
					t.Error("owner control remained in active reader path", name)
				}
				for _, root := range []string{o.SourceRoot, filepath.Join(o.Home, "lineage", p.AttemptID)} {
					body, err := os.ReadFile(filepath.Join(root, "brain", name))
					if err != nil || string(body) != "historical machine state\n" {
						t.Error("owner machine state lost or rewritten", name, err)
					}
				}
			}
			for _, name := range authored {
				for _, root := range []string{o.SourceRoot, o.Home} {
					body, err := os.ReadFile(filepath.Join(root, "brain", name))
					if err != nil || string(body) != "authored source/history\n" {
						t.Error("authored neighbor lost or rewritten", name, err)
					}
				}
			}
			externalPut(t, o.Home, "brain/owner/self/voice.md", "new authored edit\n")
			if _, err = zipmigration.ApplyExternal(p, digest, nil); err != nil {
				t.Fatal(err)
			}
			rolled, err := zipmigration.RollbackExternal(o.Home, p.AttemptID, digest, nil)
			if err != nil || rolled.Current || rolled.RestoredRuntime {
				t.Fatal("owner migration rollback failed", err)
			}
			body, err := os.ReadFile(filepath.Join(rolled.ArchiveRoot, "brain/owner/self/voice.md"))
			if err != nil || string(body) != "new authored edit\n" {
				t.Fatal("repeat/rollback did not preserve authored edit", err)
			}
		})
	}
}

func TestExternalOwnerObservationsRequireFreshAttestation(t *testing.T) {
	o := externalFixture(t, "0.2.0")
	sourceBrain := filepath.Join(o.SourceRoot, "brain")
	if _, err := ownerctx.Initialize(sourceBrain); err != nil {
		t.Fatal(err)
	}
	input := ownerctx.ObservationInput{SchemaVersion: 1, Signal: ownerctx.SignalExplicitCorrection, Facet: "voice", Claim: "concise_updates", EvidenceType: "owner_statement", SourceEvent: "interaction.completed", SourceDigest: strings.Repeat("a", 64), EpisodeID: "synthetic-source-episode", ScopeKind: "global", ScopeID: "synthetic-owner", Confidence: 1, Sensitivity: "professional", ExpiresAt: time.Now().UTC().Add(time.Hour), AuthenticatedOwner: true, Material: true, OwnerConfirmed: true}
	first, evaluation, err := ownerctx.AppendObservation(sourceBrain, input)
	if err != nil || !first.Persisted || !evaluation.Persist {
		t.Fatal("public source observation was not persisted", err)
	}
	independent := input
	independent.EpisodeID = "synthetic-independent-episode"
	independent.SourceDigest = strings.Repeat("c", 64)
	second, _, err := ownerctx.AppendObservation(sourceBrain, independent)
	if err != nil {
		t.Fatal(err)
	}
	for _, receipt := range []ownerctx.ObservationReceipt{first, second} {
		eligible, err := ownerctx.TransitionObservation(sourceBrain, ownerctx.ObservationTransitionInput{ObservationID: receipt.ID, TransitionID: "eligible-" + receipt.EpisodeID, Next: ownerctx.ObservationEligible, ExpectedState: receipt.State, ExpectedRevision: receipt.Revision, OwnerAction: true})
		if err != nil {
			t.Fatal("public observation eligibility transition failed", err)
		}
		if receipt.ID == first.ID {
			first = eligible
		}
	}
	first, err = ownerctx.TransitionObservation(sourceBrain, ownerctx.ObservationTransitionInput{ObservationID: first.ID, TransitionID: "synthetic-corroborated", Next: ownerctx.ObservationCorroborated, ExpectedState: first.State, ExpectedRevision: first.Revision, OwnerAction: true})
	if err != nil || !ownerctx.IsYodaWeeklyEligible(first) {
		t.Fatal("public source observation did not become weekly-eligible", err)
	}
	original, err := os.ReadFile(filepath.Join(sourceBrain, "owner/observations/observations.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	externalPut(t, sourceBrain, "owner/observations/notes.md", "authored observation notes\n")
	p, err := zipmigration.PlanExternal(o)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = zipmigration.ApplyExternal(p, externalDigest(t, p), nil); err != nil {
		t.Fatal(err)
	}
	targetBrain := filepath.Join(o.Home, "brain")
	if _, err = ownerctx.Initialize(targetBrain); err != nil {
		t.Fatal(err)
	}
	if old, err := ownerctx.ListObservations(sourceBrain); err != nil || len(old) != 2 {
		t.Fatal("original owner attestation was lost", err)
	}
	if active, err := ownerctx.ListObservations(targetBrain); err != nil || len(active) != 0 {
		t.Error("historical owner attestation reactivated in new brain", err)
	}
	retained, err := os.ReadFile(filepath.Join(o.Home, "lineage", p.AttemptID, "brain/owner/observations/observations.jsonl"))
	if err != nil || !bytes.Equal(retained, original) {
		t.Error("historical observation ledger was not retained intact", err)
	}
	if body, err := os.ReadFile(filepath.Join(targetBrain, "owner/observations/notes.md")); err != nil || string(body) != "authored observation notes\n" {
		t.Error("authored observation neighbor was not preserved", err)
	}
	input.EpisodeID = "synthetic-fresh-episode"
	input.SourceDigest = strings.Repeat("b", 64)
	if receipt, _, err := ownerctx.AppendObservation(targetBrain, input); err != nil || !receipt.Persisted {
		t.Fatal("fresh explicit attestation failed", err)
	}
	if active, err := ownerctx.ListObservations(targetBrain); err != nil || len(active) != 1 || active[0].EpisodeID != input.EpisodeID {
		t.Error("fresh observation store contains historical authority", err)
	}
}

func TestExternalCurrentBrainSurvivesLegacyEvolution(t *testing.T) {
	o := externalFixture(t, "0.2.0")
	externalPut(t, o.SourceRoot, "data/profile/a.md", "old a")
	externalPut(t, o.SourceRoot, "data/profile/b.md", "deleted b")
	if state := zipmigration.Run(o.SourceRoot, zipmigration.Options{}); state.State != "committed" {
		t.Fatal(state)
	}
	externalPut(t, o.SourceRoot, "brain/owner/a.md", "edited a")
	externalPut(t, o.SourceRoot, "brain/owner/c.md", "new c")
	if err := os.Remove(filepath.Join(o.SourceRoot, "brain/owner/b.md")); err != nil {
		t.Fatal(err)
	}
	p, err := zipmigration.PlanExternal(o)
	if err != nil {
		t.Fatal(err)
	}
	status, err := zipmigration.ApplyExternal(p, externalDigest(t, p), nil)
	if err != nil || !status.Current {
		t.Fatalf("%+v %v", status, err)
	}
	for rel, want := range map[string]string{"brain/owner/a.md": "edited a", "brain/owner/c.md": "new c", "lineage/" + p.AttemptID + "/data/profile/b.md": "deleted b"} {
		b, e := os.ReadFile(filepath.Join(o.Home, rel))
		if e != nil || string(b) != want {
			t.Fatalf("%s %q %v", rel, b, e)
		}
	}
	if _, err = os.Stat(filepath.Join(o.Home, "brain/owner/b.md")); !os.IsNotExist(err) {
		t.Fatal("deleted author file resurrected")
	}
	if state := zipmigration.Run(o.SourceRoot, zipmigration.Options{Status: true}); state.State != "committed" {
		t.Fatal("original runtime broken", state)
	}
}

func TestExternalReviewedPlanCannotOmitOrRedirectSource(t *testing.T) {
	for _, change := range []string{"omit", "redirect"} {
		t.Run(change, func(t *testing.T) {
			o := externalFixture(t, "0.1.12")
			externalPut(t, o.SourceRoot, "data/profile/identity.md", "must preserve")
			p, err := zipmigration.PlanExternal(o)
			if err != nil {
				t.Fatal(err)
			}
			if change == "omit" {
				p.Entries = p.Entries[:len(p.Entries)-1]
			} else {
				p.Entries[len(p.Entries)-1].Target = "brain/owner/wrong.md"
			}
			if _, err = zipmigration.ApplyExternal(p, externalDigest(t, p), nil); err == nil {
				t.Fatal("noncanonical source mapping admitted")
			}
			if _, err = os.Stat(o.Home); !os.IsNotExist(err) {
				t.Fatal("invalid plan changed destination")
			}
		})
	}
}

func TestExternalControlStagingNeverWritesInsideSource(t *testing.T) {
	o := externalFixture(t, "0.1.12")
	o.SourceRoot = filepath.Join(o.UserHome, "brain")
	o.ProductRoot = o.SourceRoot
	if err := os.Mkdir(o.SourceRoot, 0700); err != nil {
		t.Fatal(err)
	}
	externalPut(t, o.SourceRoot, "VERSION", "0.1.12\n")
	externalPut(t, o.SourceRoot, "data/profile/identity.md", "same source product\n")
	p, err := zipmigration.PlanExternal(o)
	if err != nil {
		t.Fatal(err)
	}
	status, err := zipmigration.ApplyExternal(p, externalDigest(t, p), nil)
	if err != nil || !status.Current {
		t.Fatalf("%+v %v", status, err)
	}
	entries, err := os.ReadDir(o.SourceRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("control publication changed source: %v", entries)
	}
	if _, err = os.Stat(filepath.Join(o.SourceRoot, ".maestro")); !os.IsNotExist(err) {
		t.Fatal("locator staging wrote inside source")
	}
}

func TestExternalPhysicalWriteSetAliasesRefuseBeforeEffects(t *testing.T) {
	for _, target := range []string{"home", "profile-control"} {
		t.Run(target, func(t *testing.T) {
			o := externalFixture(t, "0.1.12")
			externalPut(t, o.SourceRoot, "data/profile/identity.md", "original")
			reviewed, err := zipmigration.PlanExternal(o)
			if err != nil {
				t.Fatal(err)
			}
			alias := filepath.Join(filepath.Dir(o.SourceRoot), "SOURCE")
			if target == "profile-control" {
				o.SourceRoot = filepath.Join(o.UserHome, ".MAESTRO-EXTERNAL-MIGRATION")
				if err := os.Mkdir(o.SourceRoot, 0700); err != nil {
					t.Fatal(err)
				}
				externalPut(t, o.SourceRoot, "VERSION", "0.1.12\n")
				externalPut(t, o.SourceRoot, "data/profile/identity.md", "original")
				alias = filepath.Join(o.UserHome, ".maestro-external-migration")
				o.ProductRoot = o.SourceRoot
			}
			a, err := os.Stat(alias)
			if os.IsNotExist(err) {
				t.Skip("filesystem distinguishes case aliases")
			}
			b, otherErr := os.Stat(o.SourceRoot)
			if err != nil || otherErr != nil || !os.SameFile(a, b) {
				t.Fatalf("alias identity was not established: %v %v", err, otherErr)
			}
			if target == "home" {
				o.Home = filepath.Join(alias, "new-home")
			}
			before, err := os.ReadDir(o.SourceRoot)
			if err != nil {
				t.Fatal(err)
			}
			if p, planErr := zipmigration.PlanExternal(o); planErr == nil {
				if status, applyErr := zipmigration.ApplyExternal(p, externalDigest(t, p), nil); applyErr == nil || status.Current {
					t.Errorf("physical overlap activated: %+v %v", status, applyErr)
				}
				t.Error("plan admitted physical write-set overlap")
			}
			reviewed.ExternalOptions = o
			reviewed.ArchiveRoot = filepath.Join(filepath.Dir(o.Home), ".maestro-rollback-"+reviewed.AttemptID)
			if status, applyErr := zipmigration.ApplyExternal(reviewed, externalDigest(t, reviewed), nil); applyErr == nil || status.Current {
				t.Errorf("imported physical overlap activated: %+v %v", status, applyErr)
			}
			after, err := os.ReadDir(o.SourceRoot)
			if err != nil || len(before) != len(after) {
				t.Fatalf("source changed: %v %v", after, err)
			}
			if _, err := os.Stat(o.Home); !os.IsNotExist(err) {
				t.Fatal("overlap created HOME")
			}
		})
	}
}

func TestExternalPrivateControlDirectoriesRefuseWithoutRepair(t *testing.T) {
	for _, directory := range []string{"control", "attempt", "locator-staging"} {
		t.Run(directory, func(t *testing.T) {
			o := externalFixture(t, "0.1.12")
			externalPut(t, o.SourceRoot, "data/profile/identity.md", "original")
			p, err := zipmigration.PlanExternal(o)
			if err != nil {
				t.Fatal(err)
			}
			name := filepath.Join(o.UserHome, ".maestro-external-migration")
			if directory == "attempt" {
				name = filepath.Join(name, p.AttemptID)
			} else if directory == "locator-staging" {
				name = filepath.Join(name, ".locator-staging")
			}
			if err = os.MkdirAll(name, 0700); err != nil {
				t.Fatal(err)
			}
			assertPublic := externalPublicControl(t, o.UserHome, name)
			if _, err = zipmigration.PlanExternal(o); err == nil && directory != "attempt" {
				t.Error("plan admitted nonprivate controls")
			}
			if status, err := zipmigration.ApplyExternal(p, externalDigest(t, p), nil); err == nil || status.Current {
				t.Errorf("apply admitted nonprivate controls: %+v %v", status, err)
			}
			assertPublic()
			for _, leaf := range []string{o.Home, filepath.Join(o.UserHome, ".maestro-home.json"), filepath.Join(o.UserHome, ".maestro-external-migration/apply.lock")} {
				if _, err := os.Stat(leaf); !os.IsNotExist(err) {
					t.Error("unsafe controls allowed first effect", leaf)
				}
			}
		})
	}
}

func TestExternalHomeCannotOverlapProfilePublication(t *testing.T) {
	o := externalFixture(t, "0.1.12")
	externalPut(t, o.SourceRoot, "data/profile/identity.md", "original")
	p, err := zipmigration.PlanExternal(o)
	if err != nil {
		t.Fatal(err)
	}
	o.Home = filepath.Join(o.UserHome, ".maestro-external-migration")
	if _, err = zipmigration.PlanExternal(o); err == nil {
		t.Error("plan admitted HOME as profile-control namespace")
	}
	p.ExternalOptions = o
	p.ArchiveRoot = filepath.Join(filepath.Dir(o.Home), ".maestro-rollback-"+p.AttemptID)
	if _, err = zipmigration.ApplyExternal(p, externalDigest(t, p), nil); err == nil {
		t.Error("apply admitted HOME as profile-control namespace")
	}
	if _, err = os.Stat(o.Home); !os.IsNotExist(err) {
		t.Fatal("overlap wrote profile control state before refusing")
	}
}

func TestExternalMissingReservedProfileAliasesRefuseBeforeEffects(t *testing.T) {
	for _, reserved := range []string{".MAESTRO-EXTERNAL-MIGRATION", ".MAESTRO-HOME.JSON"} {
		t.Run(reserved, func(t *testing.T) {
			o := externalFixture(t, "0.1.12")
			externalPut(t, o.SourceRoot, "data/profile/identity.md", "original")
			reviewed, err := zipmigration.PlanExternal(o)
			if err != nil {
				t.Fatal(err)
			}
			o.Home = filepath.Join(o.UserHome, reserved)
			if _, err := zipmigration.PlanExternal(o); err == nil {
				t.Error("plan admitted missing reserved alias")
			}
			reviewed.ExternalOptions = o
			reviewed.ArchiveRoot = filepath.Join(o.UserHome, ".maestro-rollback-"+reviewed.AttemptID)
			if _, err = zipmigration.ApplyExternal(reviewed, externalDigest(t, reviewed), nil); err == nil {
				t.Error("apply admitted missing reserved alias")
			}
			entries, err := os.ReadDir(o.UserHome)
			if err != nil || len(entries) != 0 {
				t.Fatal("reserved prospective overlap changed profile", entries, err)
			}
		})
	}
}

func TestExternalDistinctCaseSensitiveRootRemainsSupported(t *testing.T) {
	o := externalFixture(t, "0.1.12")
	externalPut(t, o.SourceRoot, "data/profile/identity.md", "original")
	o.Home = filepath.Join(filepath.Dir(o.SourceRoot), "SOURCE")
	if alias, err := os.Stat(o.Home); err == nil {
		source, sourceErr := os.Stat(o.SourceRoot)
		if sourceErr != nil || !os.SameFile(alias, source) {
			t.Fatal("case alias identity was not established", sourceErr)
		}
		t.Run("same-object-source-overlap", func(t *testing.T) { externalCaseAliasContract(t, o) })
		t.Skip("filesystem aliases the source spelling")
	} else if !os.IsNotExist(err) {
		t.Fatal(err)
	}
	p, err := zipmigration.PlanExternal(o)
	if err != nil {
		t.Fatal("distinct case-sensitive root was refused", err)
	}
	if status, err := zipmigration.ApplyExternal(p, externalDigest(t, p), nil); err != nil || !status.Current {
		t.Fatal("distinct case-sensitive root unusable", status, err)
	}
}

func TestExternalRollbackRetainedPathsUseArchive(t *testing.T) {
	o := externalFixture(t, "0.1.12")
	externalPut(t, o.SourceRoot, "data/workspaces/project/note.md", "retained namespace")
	p, err := zipmigration.PlanExternal(o)
	if err != nil {
		t.Fatal(err)
	}
	digest := externalDigest(t, p)
	if _, err = zipmigration.ApplyExternal(p, digest, nil); err != nil {
		t.Fatal(err)
	}
	stop := errors.New("interrupted after archive")
	for _, operation := range []string{"interrupt", "inspect-revoked", "rollback", "repeat", "inspect"} {
		var status zipmigration.ExternalStatus
		if operation == "inspect" || operation == "inspect-revoked" {
			status, err = zipmigration.InspectExternal(o.Home, p.AttemptID, digest)
		} else if operation == "interrupt" {
			status, err = zipmigration.RollbackExternal(o.Home, p.AttemptID, digest, func(progress zipmigration.ExternalProgress) error {
				if progress.Phase == "archived" {
					return stop
				}
				return nil
			})
		} else {
			status, err = zipmigration.RollbackExternal(o.Home, p.AttemptID, digest, nil)
		}
		if operation == "interrupt" && errors.Is(err, stop) {
			err = nil
		}
		if err != nil || status.Current || status.RestoredRuntime {
			t.Errorf("%s: %+v %v", operation, status, err)
			continue
		}
		retained := status.RetainedPaths["workspaces"]
		want := filepath.Join(p.ArchiveRoot, "lineage", p.AttemptID, "data/workspaces")
		body, readErr := os.ReadFile(filepath.Join(retained, "project/note.md"))
		if retained != want || readErr != nil || string(body) != "retained namespace" {
			t.Errorf("%s returned unreadable archived lineage: %q %q %v", operation, retained, body, readErr)
		}
	}
}

func TestExternalReservedAuthorityAliasesAreInactive(t *testing.T) {
	o := externalFixture(t, "0.2.0")
	names := []string{"memory/workspaces/a/.ATTESTATION/capture-v2.key", "memory/.DREAM-REQUESTED", "ACCOUNTS/a/CASES/c/OWNER/MAESTRO/dispatch/current.json"}
	for _, name := range names {
		externalPut(t, o.SourceRoot, "brain/"+name, "old authority")
	}
	p, err := zipmigration.PlanExternal(o)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = zipmigration.ApplyExternal(p, externalDigest(t, p), nil); err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		if _, err := os.Stat(filepath.Join(o.Home, "brain", name)); !os.IsNotExist(err) {
			t.Error("reserved alias retained active authority", name)
		}
		b, err := os.ReadFile(filepath.Join(o.Home, "lineage", p.AttemptID, "brain", name))
		if err != nil || string(b) != "old authority" {
			t.Error("reserved alias was not retained inactive", name, err)
		}
	}
}

func TestExternalProofIsSelfContainedAndStrict(t *testing.T) {
	o := externalFixture(t, "0.2.0")
	externalPut(t, o.SourceRoot, "brain/owner/identity.md", "owner")
	p, err := zipmigration.PlanExternal(o)
	if err != nil {
		t.Fatal(err)
	}
	digest := externalDigest(t, p)
	if _, err = zipmigration.ApplyExternal(p, digest, nil); err != nil {
		t.Fatal(err)
	}
	if err = os.Rename(o.SourceRoot, o.SourceRoot+"-offline"); err != nil {
		t.Fatal(err)
	}
	if err = os.Rename(o.ProductRoot, o.ProductRoot+"-offline"); err != nil {
		t.Fatal(err)
	}
	// Retired roots can change independently after commit; inspection must not
	// acquire identity or permission facts from either historical pathname.
	externalDirectoryAlias(t, o.SourceRoot, o.SourceRoot+"-offline")
	externalDirectoryAlias(t, o.ProductRoot, o.ProductRoot+"-offline")
	status, err := zipmigration.InspectExternal(o.Home, p.AttemptID, digest)
	if err != nil || !status.Current {
		t.Fatalf("proof depends on original: %+v %v", status, err)
	}
	commit := filepath.Join(o.Home, "brain/.maestro/external-migration", p.AttemptID, "commit.json")
	body, err := os.ReadFile(commit)
	if err != nil {
		t.Fatal(err)
	}
	for _, invalid := range [][]byte{[]byte(`{"schema_version":1,"schema_version":1}`), []byte(`null`), []byte(`{"schema_version":1}`)} {
		if err = os.WriteFile(commit, invalid, 0600); err != nil {
			t.Fatal(err)
		}
		if state, err := zipmigration.InspectExternal(o.Home, p.AttemptID, digest); err == nil || state.Current {
			t.Fatal("bad proof admitted", state, err)
		}
	}
	if err = os.WriteFile(commit, body, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = zipmigration.RollbackExternal(o.Home, p.AttemptID, digest, nil); err == nil {
		t.Fatal("missing original revoked active HOME")
	}
	status, err = zipmigration.InspectExternal(o.Home, p.AttemptID, digest)
	if err != nil || !status.Current {
		t.Fatal("failed rollback changed authority")
	}
	// Even a digest-concordant document cannot omit a required zero-valued field.
	planName := filepath.Join(o.Home, "brain/.maestro/external-migration", p.AttemptID, "plan.json")
	planBody, err := os.ReadFile(planName)
	if err != nil {
		t.Fatal(err)
	}
	var object map[string]any
	if err = json.Unmarshal(planBody, &object); err != nil {
		t.Fatal(err)
	}
	delete(object, "prior_home")
	altered, _ := json.Marshal(object)
	if err = os.WriteFile(planName, altered, 0600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(altered)
	if state, err := zipmigration.InspectExternal(o.Home, p.AttemptID, hex.EncodeToString(sum[:])); err == nil || state.Current {
		t.Fatal("missing required field admitted")
	}
}

func TestExternalForeignLocatorDoesNotRevokeOrMoveHome(t *testing.T) {
	o := externalFixture(t, "0.1.12")
	externalPut(t, o.SourceRoot, "data/profile/note.md", "preserve")
	p, err := zipmigration.PlanExternal(o)
	if err != nil {
		t.Fatal(err)
	}
	digest := externalDigest(t, p)
	if _, err = zipmigration.ApplyExternal(p, digest, nil); err != nil {
		t.Fatal(err)
	}
	locator := filepath.Join(o.UserHome, ".maestro-home.json")
	body, err := os.ReadFile(locator)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(locator, []byte("foreign locator\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = zipmigration.RollbackExternal(o.Home, p.AttemptID, digest, nil); err == nil {
		t.Fatal("foreign locator accepted")
	}
	if _, err = os.Stat(p.ArchiveRoot); !os.IsNotExist(err) {
		t.Fatal("conflict moved HOME")
	}
	if err = os.WriteFile(locator, body, 0600); err != nil {
		t.Fatal(err)
	}
	if state, err := zipmigration.InspectExternal(o.Home, p.AttemptID, digest); err != nil || !state.Current {
		t.Fatal("conflict revoked valid authority")
	}
}

func TestExternalPreflightConflictsHaveNoProfileOrSourceEffects(t *testing.T) {
	for _, conflict := range []string{"populated", "foreign-locator", "hardlink-source", "alias-home", "stale-source", "intent-overlap"} {
		t.Run(conflict, func(t *testing.T) {
			o := externalFixture(t, "0.1.12")
			externalPut(t, o.SourceRoot, "data/profile/identity.md", "untouched")
			if conflict == "hardlink-source" {
				if err := os.Link(filepath.Join(o.SourceRoot, "data/profile/identity.md"), filepath.Join(o.SourceRoot, "data/alias.md")); err != nil {
					t.Fatal(err)
				}
				if _, err := zipmigration.PlanExternal(o); err == nil {
					t.Fatal("source alias accepted")
				}
				return
			}
			if conflict == "intent-overlap" {
				o.SourceRoot = filepath.Join(o.UserHome, ".maestro-external-migration", "source")
				o.ProductRoot = o.SourceRoot
				if err := os.MkdirAll(o.SourceRoot, 0700); err != nil {
					t.Fatal(err)
				}
				externalPut(t, o.SourceRoot, "VERSION", "0.1.12\n")
				externalPut(t, o.SourceRoot, "data/profile/a.md", "safe")
				if _, err := zipmigration.PlanExternal(o); err == nil {
					t.Fatal("control overlap accepted")
				}
				return
			}
			p, err := zipmigration.PlanExternal(o)
			if err != nil {
				t.Fatal(err)
			}
			switch conflict {
			case "populated":
				externalPut(t, o.Home, "personal.md", "foreign destination")
			case "foreign-locator":
				externalPut(t, o.UserHome, ".maestro-home.json", "foreign selection")
			case "alias-home":
				externalDirectoryAlias(t, o.Home, o.SourceRoot)
			case "stale-source":
				externalPut(t, o.SourceRoot, "data/profile/identity.md", "new source revision")
			}
			if _, err = zipmigration.ApplyExternal(p, externalDigest(t, p), nil); err == nil {
				t.Fatal("conflict accepted")
			}
			if _, err = os.Stat(filepath.Join(o.UserHome, ".maestro-external-migration")); !os.IsNotExist(err) {
				t.Fatal("failed preflight created profile control")
			}
			want := "untouched"
			if conflict == "stale-source" {
				want = "new source revision"
			}
			b, err := os.ReadFile(filepath.Join(o.SourceRoot, "data/profile/identity.md"))
			if err != nil || string(b) != want {
				t.Fatal("source changed")
			}
		})
	}
}

func TestExternalConflictingPartialAndConcurrentAttemptStayOwned(t *testing.T) {
	o := externalFixture(t, "0.1.11")
	externalPut(t, o.SourceRoot, "data/profile/note.md", "source")
	p, err := zipmigration.PlanExternal(o)
	if err != nil {
		t.Fatal(err)
	}
	digest := externalDigest(t, p)
	_, err = zipmigration.ApplyExternal(p, digest, func(event zipmigration.ExternalProgress) error {
		if event.Phase == "pending" {
			if _, nestedErr := zipmigration.ApplyExternal(p, digest, nil); nestedErr == nil {
				t.Fatal("concurrent attempt admitted")
			}
			return errors.New("stopped")
		}
		return nil
	})
	if err == nil {
		t.Fatal("stop ignored")
	}
	externalPut(t, o.Home, "brain/owner/note.md", "unowned partial change")
	if _, err = zipmigration.ApplyExternal(p, digest, nil); err == nil {
		t.Fatal("conflicting partial overwritten")
	}
	b, err := os.ReadFile(filepath.Join(o.Home, "brain/owner/note.md"))
	if err != nil || string(b) != "unowned partial change" {
		t.Fatal("partial work lost")
	}
	if state, err := zipmigration.InspectExternal(o.Home, p.AttemptID, digest); err == nil || state.Current {
		t.Fatal("partial home admitted")
	}
}

func TestExternalWindowsUnreviewedDigestRefusesBeforeEffects(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows reviewed-digest boundary")
	}
	o := externalFixture(t, "0.1.12")
	externalPut(t, o.SourceRoot, "data/profile/note.md", "source")
	p, err := zipmigration.PlanExternal(o)
	if err != nil {
		t.Fatal(err)
	}
	digest := externalDigest(t, p)
	unreviewed := digest[:len(digest)-1] + "0"
	if unreviewed == digest {
		unreviewed = digest[:len(digest)-1] + "1"
	}
	if _, err = zipmigration.ApplyExternal(p, unreviewed, nil); err == nil {
		t.Fatal("Windows unreviewed digest admitted")
	}

	if _, err = os.Stat(o.Home); !os.IsNotExist(err) {
		t.Fatal("Windows unreviewed digest created HOME")
	}
}

func TestExternalRetirementIsFixedDenyOnlyAndInterruptedRollbackResumes(t *testing.T) {
	o := externalFixture(t, "0.1.12")
	externalPut(t, o.SourceRoot, "data/profile/identity.md", "preserve")
	if retired, err := zipmigration.RetiredExternalHome(o.UserHome, o.Home); err != nil || retired {
		t.Fatal("fresh home retired", retired, err)
	}
	if _, err := os.Stat(filepath.Join(o.UserHome, ".maestro-external-migration")); !os.IsNotExist(err) {
		t.Fatal("retirement read created control state")
	}
	for i := 0; i < 2; i++ {
		p, err := zipmigration.PlanExternal(o)
		if err != nil {
			t.Fatal(err)
		}
		digest := externalDigest(t, p)
		if _, err = zipmigration.ApplyExternal(p, digest, nil); err != nil {
			t.Fatal(err)
		}
		_, err = zipmigration.RollbackExternal(o.Home, p.AttemptID, digest, func(event zipmigration.ExternalProgress) error {
			if event.Phase == "retired" {
				return errors.New("operator interrupted")
			}
			return nil
		})
		if err == nil {
			t.Fatal("stop ignored")
		}
		if retired, err := zipmigration.RetiredExternalHome(o.UserHome, o.Home); err == nil || retired {
			t.Fatal("incomplete retirement not closed", retired, err)
		}
		if _, err = zipmigration.RollbackExternal(o.Home, p.AttemptID, digest, nil); err != nil {
			t.Fatal("rollback resume", err)
		}
		if retired, err := zipmigration.RetiredExternalHome(o.UserHome, o.Home); err != nil || !retired {
			t.Fatal("retired binding absent", retired, err)
		}
	}
	homeHash := sha256.Sum256([]byte(o.Home))
	pointer := filepath.Join(o.UserHome, ".maestro-external-migration/retired", hex.EncodeToString(homeHash[:])+".json")
	if err := os.WriteFile(pointer, []byte(`{"schema_version":1}`), 0600); err != nil {
		t.Fatal(err)
	}
	if retired, err := zipmigration.RetiredExternalHome(o.UserHome, o.Home); err == nil || retired {
		t.Fatal("malformed retirement treated as fresh", retired, err)
	}
}

func TestExternalCaseAuthorityIsArchivedOutsideActiveReaders(t *testing.T) {
	o := externalFixture(t, "0.2.0")
	for _, name := range []string{"accounts/a/cases/c/.maestro/daily-generation", "accounts/a/cases/c/memory/.dream-requested", "accounts/a/cases/c/owner/maestro/dispatch/current.json", "memory/workspaces/id/.attestation/capture-v2.key"} {
		externalPut(t, o.SourceRoot, "brain/"+name, "historical authority")
	}
	externalPut(t, o.SourceRoot, "brain/accounts/a/cases/c/notes.md", "live case notes")
	p, err := zipmigration.PlanExternal(o)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = zipmigration.ApplyExternal(p, externalDigest(t, p), nil); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"accounts/a/cases/c/.maestro/daily-generation", "accounts/a/cases/c/memory/.dream-requested", "accounts/a/cases/c/owner/maestro/dispatch/current.json", "memory/workspaces/id/.attestation/capture-v2.key"} {
		if _, err = os.Stat(filepath.Join(o.Home, "brain", name)); !os.IsNotExist(err) {
			t.Fatal("old control remained active", name)
		}
		b, err := os.ReadFile(filepath.Join(o.Home, "lineage", p.AttemptID, "brain", name))
		if err != nil || string(b) != "historical authority" {
			t.Fatal("old control lost", name, err)
		}
	}
	b, err := os.ReadFile(filepath.Join(o.Home, "brain/accounts/a/cases/c/notes.md"))
	if err != nil || string(b) != "live case notes" {
		t.Fatal("authored case content lost")
	}
}

func TestExternalInterruptionAndRollbackRetainNewWork(t *testing.T) {
	for _, phase := range []string{"prepared", "pending", "copying", "copied", "committed"} {
		t.Run(phase, func(t *testing.T) {
			o := externalFixture(t, "0.1.11")
			externalPut(t, o.SourceRoot, "data/profile/note.md", "before")
			p, err := zipmigration.PlanExternal(o)
			if err != nil {
				t.Fatal(err)
			}
			digest := externalDigest(t, p)
			_, err = zipmigration.ApplyExternal(p, digest, func(p zipmigration.ExternalProgress) error {
				if p.Phase == phase {
					return errors.New("operator stopped")
				}
				return nil
			})
			if err == nil {
				t.Fatal("stop ignored")
			}
			status, err := zipmigration.ApplyExternal(p, digest, nil)
			if err != nil || !status.Current {
				t.Fatalf("restart: %+v %v", status, err)
			}
			externalPut(t, o.Home, "brain/owner/note.md", "after migration")
			_, err = zipmigration.RollbackExternal(o.Home, p.AttemptID, digest, func(p zipmigration.ExternalProgress) error {
				if p.Phase == "archived" {
					return errors.New("operator stopped")
				}
				return nil
			})
			if err == nil {
				t.Fatal("rollback stop ignored")
			}
			status, err = zipmigration.RollbackExternal(o.Home, p.AttemptID, digest, nil)
			if err != nil || status.State != "rolled_back" || status.Current || status.RestoredRuntime {
				t.Fatalf("rollback: %+v %v", status, err)
			}
			b, err := os.ReadFile(filepath.Join(status.ArchiveRoot, "brain/owner/note.md"))
			if err != nil || string(b) != "after migration" {
				t.Fatalf("new work lost: %q %v", b, err)
			}
			b, err = os.ReadFile(filepath.Join(o.SourceRoot, "data/profile/note.md"))
			if err != nil || string(b) != "before" {
				t.Fatal("original changed")
			}
			if _, err = os.Stat(filepath.Join(o.UserHome, ".maestro-home.json")); !os.IsNotExist(err) {
				t.Fatal("owned locator remains")
			}
			if _, err = os.Stat(o.Home); !os.IsNotExist(err) {
				t.Fatal("revoked default home blocks legacy selection")
			}
		})
	}
}

func externalFixture(t *testing.T, version string) zipmigration.ExternalOptions {
	t.Helper()
	externalPlatformAccount(t)
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	o := zipmigration.ExternalOptions{SourceRoot: filepath.Join(base, "source"), ProductRoot: filepath.Join(base, "product"), UserHome: filepath.Join(base, "profile"), Home: filepath.Join(base, "profile", ".maestro")}
	for _, p := range []string{o.SourceRoot, o.ProductRoot, o.UserHome} {
		if err := os.Mkdir(p, 0700); err != nil {
			t.Fatal(err)
		}
	}
	externalPut(t, o.SourceRoot, "VERSION", version+"\n")
	return o
}

func externalDigest(t *testing.T, p zipmigration.ExternalPlan) string {
	t.Helper()
	b, err := p.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func TestExternalLegacyAuthoredStateAndInactiveAuthority(t *testing.T) {
	o := externalFixture(t, "0.1.12")
	externalPut(t, o.SourceRoot, "data/profile/identity.md", "original identity\n")
	externalPut(t, o.SourceRoot, "data/workspaces/project/note.md", "retained namespace\n")
	externalPut(t, o.SourceRoot, "data/.maestro/runtime/observed.json", "historical receipt\n")
	p, err := zipmigration.PlanExternal(o)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(o.Home); !os.IsNotExist(err) {
		t.Fatal("plan created HOME")
	}
	digest := externalDigest(t, p)
	status, err := zipmigration.ApplyExternal(p, digest, nil)
	if err != nil || !status.Current {
		t.Fatalf("apply: %+v %v", status, err)
	}
	for rel, want := range map[string]string{"brain/owner/identity.md": "original identity\n", "lineage/" + p.AttemptID + "/data/workspaces/project/note.md": "retained namespace\n", "lineage/" + p.AttemptID + "/data/.maestro/runtime/observed.json": "historical receipt\n"} {
		b, e := os.ReadFile(filepath.Join(o.Home, rel))
		if e != nil || string(b) != want {
			t.Fatalf("%s: %q %v", rel, b, e)
		}
	}
	if _, err = os.Stat(filepath.Join(o.Home, "brain/.maestro/runtime/observed.json")); !os.IsNotExist(err) {
		t.Fatal("historical authority became live")
	}
	externalPut(t, o.Home, "brain/owner/identity.md", "new owner edit\n")
	status, err = zipmigration.ApplyExternal(p, digest, nil)
	if err != nil || !status.Current || status.State != "already_migrated" {
		t.Fatalf("repeat %+v %v", status, err)
	}
	b, _ := os.ReadFile(filepath.Join(o.Home, "brain/owner/identity.md"))
	if string(b) != "new owner edit\n" {
		t.Fatal("repeat overwrote new work")
	}
	b, _ = os.ReadFile(filepath.Join(o.SourceRoot, "data/profile/identity.md"))
	if string(b) != "original identity\n" {
		t.Fatal("source changed")
	}
}

func externalPut(t *testing.T, root, name, body string) {
	t.Helper()
	p := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
}

func externalCaseAliasContract(t *testing.T, options zipmigration.ExternalOptions) {
	t.Helper()
	selectedID, counterpartID := externalPlatformIdentity(t, options.Home), externalPlatformIdentity(t, options.SourceRoot)
	if selectedID != counterpartID || options.Home == options.SourceRoot {
		t.Fatal("selected source alias identity was not established")
	}
	sourceInfo, err := os.Stat(options.SourceRoot)
	if err != nil {
		t.Fatal(err)
	}
	identity := filepath.Join(options.SourceRoot, "data/profile/identity.md")
	beforeBody, err := os.ReadFile(identity)
	if err != nil {
		t.Fatal(err)
	}
	beforeEntries, err := os.ReadDir(options.SourceRoot)
	if err != nil {
		t.Fatal(err)
	}
	baseline := options
	baseline.Home = filepath.Join(options.UserHome, ".maestro")
	reviewed, err := zipmigration.PlanExternal(baseline)
	if err != nil {
		t.Fatal("ordinary migration positive control failed", err)
	}
	if plan, err := zipmigration.PlanExternal(options); err == nil {
		if status, applyErr := zipmigration.ApplyExternal(plan, externalDigest(t, plan), nil); applyErr == nil || status.Current {
			t.Error("case alias acquired source authority", applyErr)
		}
		t.Error("plan admitted physical source overlap")
	}
	reviewed.ExternalOptions = options
	reviewed.ArchiveRoot = filepath.Join(filepath.Dir(options.Home), ".maestro-rollback-"+reviewed.AttemptID)
	if status, err := zipmigration.ApplyExternal(reviewed, externalDigest(t, reviewed), nil); err == nil || status.Current {
		t.Error("imported alias acquired source authority", err)
	}
	afterBody, err := os.ReadFile(identity)
	if err != nil || !bytes.Equal(beforeBody, afterBody) {
		t.Fatal("source alias refusal changed original bytes", err)
	}
	afterEntries, err := os.ReadDir(options.SourceRoot)
	if err != nil || len(beforeEntries) != len(afterEntries) {
		t.Fatal("source alias refusal changed original namespace", err)
	}
	afterInfo, err := os.Stat(options.SourceRoot)
	if err != nil || !os.SameFile(sourceInfo, afterInfo) || externalPlatformIdentity(t, options.SourceRoot) != counterpartID {
		t.Fatal("source alias refusal changed selected identity", err)
	}
	profile, err := os.ReadDir(options.UserHome)
	if err != nil || len(profile) != 0 {
		t.Fatal("source alias refusal changed the owner profile", err)
	}
	body, err := json.Marshal(map[string]any{"kind": "selected-directory-case-pair-v1", "scope": t.Name(), "selected": options.Home, "counterpart": options.SourceRoot, "selected_identity": selectedID, "counterpart_identity": counterpartID, "same_object": true})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("case_pair_evidence=%s", body)
}
