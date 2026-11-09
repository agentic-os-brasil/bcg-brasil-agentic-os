//go:build windows

package zipmigration

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWindowsExternalCaseCapabilityPreservesOverlapGuards(t *testing.T) {
	o := externalNativeTransactionFixture(t)
	reviewed, _ := externalNativePlan(t, o)
	source := externalNativeSnapshot(t, o.SourceRoot)
	profile := externalNativeSnapshot(t, o.UserHome)
	alias := filepath.Join(filepath.Dir(o.SourceRoot), strings.ToUpper(filepath.Base(o.SourceRoot)))
	if alias == o.SourceRoot {
		t.Fatal("case fixture did not change the source spelling")
	}
	aliasInfo, err := os.Stat(alias)
	if errors.Is(err, os.ErrNotExist) {
		if err := os.Mkdir(alias, 0700); err != nil {
			t.Fatal("case-sensitive positive fixture unavailable", err)
		}
		aliasInfo, err = os.Stat(alias)
		sourceInfo, sourceErr := os.Stat(o.SourceRoot)
		aliasIdentity := externalNativeSnapshot(t, alias)["."].Identity
		if err != nil || sourceErr != nil || os.SameFile(aliasInfo, sourceInfo) || aliasIdentity == source["."].Identity {
			t.Fatal("case-distinct source/HOME objects were not established", err, sourceErr)
		}
		externalNativeCaseEvidence(t, alias, o.SourceRoot, aliasIdentity, source["."].Identity, false)
		if err := os.Remove(alias); err != nil {
			t.Fatal(err)
		}
		o.Home = alias
		plan, digest := externalNativePlan(t, o)
		if status, err := ApplyExternal(plan, digest, nil); err != nil || !status.Current {
			t.Fatal("distinct case-sensitive root unusable", status.State, err)
		}
		externalNativeUnchanged(t, o.SourceRoot, source)
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	sourceInfo, err := os.Stat(o.SourceRoot)
	if err != nil || !os.SameFile(aliasInfo, sourceInfo) {
		t.Fatal("case alias exists but identifies a different uncreated object", err)
	}
	aliasIdentity := externalNativeSnapshot(t, alias)["."].Identity
	if aliasIdentity != source["."].Identity {
		t.Fatal("case alias native identities disagree")
	}
	externalNativeCaseEvidence(t, alias, o.SourceRoot, aliasIdentity, source["."].Identity, true)
	o.Home = alias
	if plan, err := PlanExternal(o); err == nil {
		if status, err := ApplyExternal(plan, hashPlan(t, plan), nil); err == nil || status.Current {
			t.Error("case alias acquired source write authority", status.State, err)
		}
		t.Error("plan admitted a source/HOME physical alias")
	}
	reviewed.ExternalOptions = o
	reviewed.ArchiveRoot = filepath.Join(filepath.Dir(o.Home), ".maestro-rollback-"+reviewed.AttemptID)
	if status, err := ApplyExternal(reviewed, hashPlan(t, reviewed), nil); err == nil || status.Current {
		t.Error("imported case alias acquired source write authority", status.State, err)
	}
	externalNativeUnchanged(t, o.SourceRoot, source)
	externalNativeUnchanged(t, o.UserHome, profile)
}

func hashPlan(t *testing.T, plan ExternalPlan) string {
	t.Helper()
	body, err := plan.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	return hash(body)
}

func externalNativeCaseEvidence(t *testing.T, selected, counterpart string, selectedID, counterpartID [3]uint32, same bool) {
	t.Helper()
	body, err := json.Marshal(map[string]any{"kind": "selected-directory-case-pair-v1", "scope": t.Name(), "selected": selected, "counterpart": counterpart, "selected_identity": selectedID, "counterpart_identity": counterpartID, "same_object": same})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("case_pair_evidence=%s", body)
}
