package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"
)

var sharedNativeQualificationChecks = []string{
	"dangerous_git_denied",
	"direct_enrollment",
	"doctor_skill_discovered",
	"doctor_skill_invoked",
	"hub_bootstrap",
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
		SchemaVersion:  1,
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

func validateNativeRuntimeParity(claude, codex report) error {
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
