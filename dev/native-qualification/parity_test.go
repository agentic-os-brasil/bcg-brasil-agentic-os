package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

var sharedNativeQualificationChecks = []string{
	"canonical_skill_projection",
	"dangerous_git_denied",
	"direct_enrollment",
	"distinct_worktree_context_isolated",
	"distinct_worktree_identity",
	"distinct_worktree_removal_independent",
	"doctor_skill_discovered",
	"doctor_skill_invoked",
	"hub_bootstrap",
	"memory_context_injected",
	"memory_skill_discovered",
	"memory_source_preserved",
	"native_resume",
	"operator_skill_discovered",
	"specialist_topology_projected",
	"supported_hooks_complete",
	"transparent_maestro_identity",
	"owner_context_injected",
	"owner_sensitive_context_excluded",
	"projection_git_clean",
	"projection_removal",
	"receipt_post_action_observe",
	"receipt_stop_finalize",
	"removal_git_clean",
	"second_session_continuity",
	"workspace_write",
}

func TestLatestClaudeAndCodexNativeEvidenceQualifiesSameArtifactContract(t *testing.T) {
	releaseBody, err := os.ReadFile(filepath.Join("..", "..", "VERSION"))
	if err != nil {
		t.Fatal(err)
	}
	releaseVersion := string(releaseBody)
	for len(releaseVersion) > 0 && (releaseVersion[len(releaseVersion)-1] == '\n' || releaseVersion[len(releaseVersion)-1] == '\r') {
		releaseVersion = releaseVersion[:len(releaseVersion)-1]
	}

	reports := map[string]report{}
	paths, err := filepath.Glob(filepath.Join("..", "..", "docs", "evidence", "*-direct-*.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		body, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatal(readErr)
		}
		var candidate report
		if decodeErr := json.Unmarshal(body, &candidate); decodeErr != nil {
			t.Fatalf("decode %s: %v", path, decodeErr)
		}
		if candidate.ReleaseVersion != releaseVersion || candidate.Runtime != "claude" && candidate.Runtime != "codex" {
			continue
		}
		current, found := reports[candidate.Runtime]
		if !found || candidate.ObservedAt > current.ObservedAt {
			reports[candidate.Runtime] = candidate
		}
	}
	if err := validateNativeRuntimeParity(reports["claude"], reports["codex"]); err != nil {
		t.Fatal(err)
	}
}

func TestNativeRuntimeParityGateRejectsOneSidedSuccess(t *testing.T) {
	base := report{
		SchemaVersion:  2,
		Result:         "pass",
		ObservedAt:     time.Now().UTC().Format(time.RFC3339),
		OS:             "darwin",
		Arch:           "arm64",
		ReleaseVersion: "0.1.11",
		ArtifactSHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Checks:         map[string]bool{},
	}
	for _, check := range sharedNativeQualificationChecks {
		base.Checks[check] = true
	}
	claude := cloneParityReport(base)
	claude.Runtime = "claude"
	codex := cloneParityReport(base)
	codex.Runtime = "codex"
	delete(codex.Checks, "workspace_write")
	if err := validateNativeRuntimeParity(claude, codex); err == nil {
		t.Fatal("parity gate accepted a capability proven only for Claude")
	}
	codex.Checks["workspace_write"] = true
	delete(codex.Checks, "owner_context_injected")
	if err := validateNativeRuntimeParity(claude, codex); err == nil {
		t.Fatal("parity gate accepted reviewed owner context only for Claude")
	}
	codex.Checks["owner_context_injected"] = true
	delete(codex.Checks, "transparent_maestro_identity")
	if err := validateNativeRuntimeParity(claude, codex); err == nil {
		t.Fatal("parity gate accepted transparent Maestro identity only for Claude")
	}
	codex.Checks["transparent_maestro_identity"] = true
	codex.ArtifactSHA256 = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	if err := validateNativeRuntimeParity(claude, codex); err == nil {
		t.Fatal("parity gate accepted different platform artifacts")
	}
}

func TestNativeRuntimeParityGateRejectsEveryOneSidedExpandedMatrixCell(t *testing.T) {
	base := report{SchemaVersion: 2, Result: "pass", Runtime: "claude", OS: "darwin", Arch: "arm64", ReleaseVersion: "0.1.13", ArtifactSHA256: strings.Repeat("a", 64), Checks: map[string]bool{}}
	for _, check := range sharedNativeQualificationChecks {
		base.Checks[check] = true
	}
	for _, check := range []string{"canonical_skill_projection", "memory_context_injected", "native_resume", "distinct_worktree_identity", "distinct_worktree_context_isolated", "specialist_topology_projected", "supported_hooks_complete"} {
		t.Run(check, func(t *testing.T) {
			claude := cloneParityReport(base)
			codex := cloneParityReport(base)
			codex.Runtime = "codex"
			delete(codex.Checks, check)
			if err := validateNativeRuntimeParity(claude, codex); err == nil {
				t.Fatalf("parity accepted one-sided %s", check)
			}
		})
	}
}

func TestNativeRuntimeTopologyRejectsInventedOrMissingHostSpecificCells(t *testing.T) {
	claude := report{Runtime: "claude", Checks: map[string]bool{"hub_session_start": true, "hub_doctor_skill": true, "native_agents_discovered": true, "managed_agent_flow": true}}
	codex := report{Runtime: "codex", Checks: map[string]bool{"hub_activation_only": true, "agent_orchestration_declared_unavailable": true}}
	if err := validateNativeRuntimeTopology(claude, codex); err != nil {
		t.Fatal(err)
	}
	codex.Checks["hub_session_start"] = true
	if err := validateNativeRuntimeTopology(claude, codex); err == nil {
		t.Fatal("invented Codex native Hub was accepted")
	}
	delete(codex.Checks, "hub_session_start")
	delete(codex.Checks, "agent_orchestration_declared_unavailable")
	if err := validateNativeRuntimeTopology(claude, codex); err == nil {
		t.Fatal("missing Codex agent limitation was accepted")
	}
}

func validateNativeRuntimeParity(claude, codex report) error {
	if claude.SchemaVersion != 2 || codex.SchemaVersion != 2 {
		return fmt.Errorf("paired native matrix requires schema version 2 reports")
	}
	if claude.Runtime != "claude" || codex.Runtime != "codex" {
		return fmt.Errorf("native parity requires one Claude and one Codex report")
	}
	if claude.Result != "pass" || codex.Result != "pass" {
		return fmt.Errorf("native parity requires passing reports: claude=%q codex=%q", claude.Result, codex.Result)
	}
	if claude.ReleaseVersion != codex.ReleaseVersion || claude.OS != codex.OS || claude.Arch != codex.Arch || claude.ArtifactSHA256 != codex.ArtifactSHA256 {
		return fmt.Errorf("native runtime tuple drift: claude=%s/%s/%s/%s codex=%s/%s/%s/%s", claude.ReleaseVersion, claude.OS, claude.Arch, claude.ArtifactSHA256, codex.ReleaseVersion, codex.OS, codex.Arch, codex.ArtifactSHA256)
	}
	var missing []string
	for _, check := range sharedNativeQualificationChecks {
		if !claude.Checks[check] || !codex.Checks[check] {
			missing = append(missing, fmt.Sprintf("%s(claude=%t,codex=%t)", check, claude.Checks[check], codex.Checks[check]))
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return fmt.Errorf("shared native qualification checks diverged: %v", missing)
	}
	return validateNativeRuntimeTopology(claude, codex)
}

func TestNativeRuntimeParityAcceptsCompleteSupportedTopology(t *testing.T) {
	base := report{SchemaVersion: 2, Result: "pass", OS: "darwin", Arch: "arm64", ReleaseVersion: "0.1.13", ArtifactSHA256: strings.Repeat("a", 64), Checks: map[string]bool{}}
	for _, check := range sharedNativeQualificationChecks {
		base.Checks[check] = true
	}
	claude := cloneParityReport(base)
	claude.Runtime = "claude"
	for _, check := range []string{"hub_session_start", "hub_doctor_skill", "native_agents_discovered", "managed_agent_flow"} {
		claude.Checks[check] = true
	}
	codex := cloneParityReport(base)
	codex.Runtime = "codex"
	codex.Checks["hub_activation_only"] = true
	codex.Checks["agent_orchestration_declared_unavailable"] = true
	if err := validateNativeRuntimeParity(claude, codex); err != nil {
		t.Fatal(err)
	}
}

func validateNativeRuntimeTopology(claude, codex report) error {
	for _, check := range []string{"hub_session_start", "hub_doctor_skill", "native_agents_discovered", "managed_agent_flow"} {
		if !claude.Checks[check] {
			return fmt.Errorf("Claude native topology omitted %s", check)
		}
	}
	for _, check := range []string{"hub_activation_only", "agent_orchestration_declared_unavailable"} {
		if !codex.Checks[check] {
			return fmt.Errorf("Codex declared topology omitted %s", check)
		}
	}
	for _, invented := range []string{"hub_session_start", "hub_doctor_skill", "native_agents_discovered", "managed_agent_flow"} {
		if codex.Checks[invented] {
			return fmt.Errorf("Codex report invented unsupported native topology cell %s", invented)
		}
	}
	return nil
}

func cloneParityReport(source report) report {
	cloned := source
	cloned.Checks = map[string]bool{}
	for name, passed := range source.Checks {
		cloned.Checks[name] = passed
	}
	return cloned
}
