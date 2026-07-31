package maintenance

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMaintenanceConformanceFixtureCoversCatalogAndKeepsAdaptersUnqualified(t *testing.T) {
	catalog, err := LoadFile(filepath.Join("../../bundles/base/runtime", "maintenance.json"))
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join("../../adapters/conformance", "maintenance.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		CatalogState   string `json:"catalog_state"`
		NativeEvidence string `json:"native_evidence"`
		Adapters       map[string]struct {
			State string `json:"state"`
		} `json:"adapters"`
		Jobs []string `json:"jobs"`
	}
	if err := json.Unmarshal(body, &fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.CatalogState != CatalogOnly || fixture.NativeEvidence != "pending" {
		t.Fatalf("fixture promoted capability: %#v", fixture)
	}
	for _, platform := range []string{"claude", "codex"} {
		if fixture.Adapters[platform].State != Unavailable {
			t.Fatalf("%s adapter must remain unavailable", platform)
		}
	}
	if fixture.Adapters["macos"].State != "template_only" || fixture.Adapters["windows"].State != "template_only" {
		t.Fatal("native scheduler adapters must remain template_only")
	}
	if len(fixture.Jobs) != len(catalog.Jobs) {
		t.Fatalf("fixture jobs = %d, catalog jobs = %d", len(fixture.Jobs), len(catalog.Jobs))
	}
	seen := map[string]bool{}
	for _, id := range fixture.Jobs {
		seen[id] = true
	}
	for _, job := range catalog.Jobs {
		if !seen[job.ID] {
			t.Fatalf("fixture missing %s", job.ID)
		}
	}
	macTemplate, err := os.ReadFile(filepath.Join("../../adapters/macos/launchd", "com.bcg.maestro.maintenance.plist.tmpl"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(macTemplate), "<key>Disabled</key>\n  <true/>") {
		t.Fatal("macOS template must be disabled until executor qualification")
	}
	windowsTemplate, err := os.ReadFile(filepath.Join("../../adapters/windows/task-scheduler", "BCGOS-Maestro-Maintenance.xml.tmpl"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(windowsTemplate), "<Enabled>false</Enabled>") != 2 {
		t.Fatal("Windows template triggers must remain disabled until executor qualification")
	}
}

func TestDarwinCadenceFixtureKeepsHooksNonBlockingAndPromotionUnavailable(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("../../adapters/conformance", "darwin-cadence.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		RuntimeNeutral  bool `json:"runtime_neutral"`
		ContinuousEvent struct {
			Trigger         string `json:"trigger"`
			MapsTo          string `json:"maps_to"`
			RequiresEventID bool   `json:"requires_event_id"`
			HookBehavior    string `json:"hook_behavior"`
		} `json:"continuous_event"`
		Cadences []struct {
			Name    string `json:"name"`
			CatchUp string `json:"catch_up"`
			Worker  string `json:"worker"`
		} `json:"cadences"`
		Worker struct {
			Authority          string `json:"authority"`
			DeadlineRequired   bool   `json:"deadline_required"`
			MaxDeadlineMinutes int    `json:"max_deadline_minutes"`
			Reentrancy         string `json:"reentrancy"`
			BusyReceipt        string `json:"busy_receipt"`
			LeaseKey           string `json:"lease_key"`
			LeaseFencing       string `json:"lease_fencing"`
			Occurrence         string `json:"occurrence_idempotency"`
			Receipts           string `json:"receipts"`
			Structural         string `json:"structural_evolution"`
			Promotion          string `json:"capability_promotion"`
		} `json:"worker_contract"`
		Native map[string]string `json:"native"`
	}
	if err := json.Unmarshal(body, &fixture); err != nil {
		t.Fatal(err)
	}
	if !fixture.RuntimeNeutral || fixture.ContinuousEvent.Trigger != "continuous" || fixture.ContinuousEvent.MapsTo != "event" || !fixture.ContinuousEvent.RequiresEventID || fixture.ContinuousEvent.HookBehavior != "signal_only" {
		t.Fatalf("continuous event contract = %#v", fixture.ContinuousEvent)
	}
	if len(fixture.Cadences) != 3 || fixture.Worker.Authority != "runtime_qualified_catalog_attendance_and_exact_occurrence" || fixture.Worker.MaxDeadlineMinutes != 15 || !fixture.Worker.DeadlineRequired || fixture.Worker.Reentrancy != "busy_without_wait" || fixture.Worker.BusyReceipt != "ephemeral_nonterminal" || fixture.Worker.LeaseKey != "canonical_occurrence" || fixture.Worker.LeaseFencing != "unique_attempt_token_and_guarded_execution_plus_finalization" || fixture.Worker.Occurrence != "terminal_occurrence_digest" || fixture.Worker.Receipts != "metadata_only" || fixture.Worker.Structural != "proposal_only" || fixture.Worker.Promotion != "native_evidence_only" {
		t.Fatalf("worker cadence contract = %#v", fixture)
	}
	for _, state := range fixture.Native {
		if state != "unavailable" && state != "template_only" {
			t.Fatalf("native state was promoted: %#v", fixture.Native)
		}
	}
}
