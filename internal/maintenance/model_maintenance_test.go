package maintenance

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func weeklyDescriptorFixture() WeeklyModelDescriptor {
	executable, _ := filepath.Abs("synthetic-claude")
	return WeeklyModelDescriptor{SchemaVersion: 1, JobID: YodaSelfReviewWeeklyJobID, Runtime: "claude_code_cli", RuntimeVersion: "synthetic-version", Executable: executable, ExecutableSHA256: strings.Repeat("a", 64), ConfigurationSHA256: strings.Repeat("b", 64), ManagedPolicySHA256: strings.Repeat("c", 64), ProfileReferenceSHA256: strings.Repeat("d", 64), TransportSHA256: strings.Repeat("e", 64), Provider: "explicit-synthetic-provider", Model: "explicit-synthetic-model", CredentialMode: "runtime_owned_session", ValidatorVersion: "weekly-self-v1", Limits: WeeklyModelLimits{MaxInputBytes: 32768, MaxOutputBytes: 32768, MaxStderrBytes: 4096, MaxOutputTokens: 4096, TimeoutSeconds: 60, Attempts: 1, AgenticTurns: 1}}
}

func TestWeeklyModelDescriptorClosedAndExact(t *testing.T) {
	d := weeklyDescriptorFixture()
	if err := d.Validate(); err != nil {
		t.Fatal(err)
	}
	before := d.Digest()
	mutations := []func(*WeeklyModelDescriptor){func(v *WeeklyModelDescriptor) { v.Provider = "" }, func(v *WeeklyModelDescriptor) { v.Model = "" }, func(v *WeeklyModelDescriptor) { v.JobID = "memory-daily-synthesis" }, func(v *WeeklyModelDescriptor) { v.CredentialMode = "api_key" }, func(v *WeeklyModelDescriptor) { v.Executable = "claude" }, func(v *WeeklyModelDescriptor) { v.ManagedPolicySHA256 = "" }, func(v *WeeklyModelDescriptor) { v.Limits.Attempts = 2 }, func(v *WeeklyModelDescriptor) { v.Limits.AgenticTurns = 2 }, func(v *WeeklyModelDescriptor) { v.Limits.MaxInputBytes = 32769 }, func(v *WeeklyModelDescriptor) { v.Limits.MaxOutputTokens = 0 }}
	for i, mutate := range mutations {
		t.Run(string(rune('a'+i)), func(t *testing.T) {
			other := d
			mutate(&other)
			if other.Validate() == nil {
				t.Fatal("unsafe/incomplete tuple accepted")
			}
			if before == other.Digest() {
				t.Fatal("descriptor drift not bound")
			}
		})
	}
	body, _ := json.Marshal(d)
	if _, err := ParseWeeklyModelDescriptor(body); err != nil {
		t.Fatal(err)
	}
	for _, body := range []string{strings.TrimSuffix(string(body), "}") + `,"tools":true}`, strings.Replace(string(body), `"schema_version":1`, `"schema_version":1,"schema_version":1`, 1), string(body) + ` {}`} {
		if _, err := ParseWeeklyModelDescriptor([]byte(body)); err == nil {
			t.Fatal("non-strict descriptor accepted")
		}
	}
}

func TestWeeklyModelGrantCannotUseOrdinaryJobAuthority(t *testing.T) {
	d := weeklyDescriptorFixture()
	if err := ValidateWeeklyModelExecution(nil, Command{}, d, strings.Repeat("e", 64), strings.Repeat("f", 64)); err == nil {
		t.Fatal("absent grant accepted")
	}
	// A descriptor is data, never qualification, enrollment or occurrence authority.
	if _, err := ParseWeeklyModelDescriptor([]byte(`{"qualified":true}`)); err == nil {
		t.Fatal("constructor/model qualification claim accepted")
	}
}

func TestWeeklyModelClosedProbeModeKeepsLiveGuardAndDeniesDomainWrites(t *testing.T) {
	now := time.Now().UTC()
	command := Command{SchemaVersion: 1, CommandID: "synthetic-weekly", JobID: YodaSelfReviewWeeklyJobID, WorkspaceID: "owner-synthetic", Trigger: TriggerWeekly, ScheduledFor: now, RequestedAt: now, Deadline: now.Add(time.Minute), WeeklyMode: WeeklyNativeProbe}
	issued, err := newExecutionGrant(command)
	if err != nil {
		t.Fatal(err)
	}
	grant := issued.(executionGrant)
	calls := 0
	revoked := false
	grant.publication = func(write func() error) error {
		calls++
		if revoked {
			return ErrProductConflict
		}
		return write()
	}
	descriptor := weeklyDescriptorFixture()
	input, policy := strings.Repeat("a", 64), strings.Repeat("b", 64)
	grant.weekly = &weeklyModelBinding{Mode: WeeklyNativeProbe, OwnerScope: command.WorkspaceID, DescriptorSHA256: descriptor.Digest(), InputSHA256: input, PolicySHA256: policy, ProbeIntentSHA256: strings.Repeat("c", 64), TerminalOperation: "qualification_evidence_only"}
	if err := ValidateWeeklyModelExecution(grant, command, descriptor, input, policy); err != nil {
		t.Fatal(err)
	}
	if calls == 0 {
		t.Fatal("probe lost live control/lease validation")
	}
	wrote := false
	if err := PublishExecutionGrant(context.Background(), grant, command, func() error { wrote = true; return nil }); err == nil || wrote {
		t.Fatal("evidence-only probe published domain state")
	}
	revoked = true
	if err := ValidateWeeklyModelExecution(grant, command, descriptor, input, policy); !errors.Is(err, ErrProductConflict) {
		t.Fatal("revoked probe retained authority", err)
	}
}

func TestWeeklyModelClosedModeRejectsMixedOrChangedBindings(t *testing.T) {
	now := time.Now().UTC()
	command := Command{SchemaVersion: 1, CommandID: "synthetic-weekly", JobID: YodaSelfReviewWeeklyJobID, WorkspaceID: "owner-synthetic", Trigger: TriggerWeekly, ScheduledFor: now, RequestedAt: now, Deadline: now.Add(time.Minute), WeeklyMode: WeeklyWorking}
	issued, _ := newExecutionGrant(command)
	grant := issued.(executionGrant)
	grant.publication = func(write func() error) error { return write() }
	d := weeklyDescriptorFixture()
	input, policy := strings.Repeat("a", 64), strings.Repeat("b", 64)
	initial := weeklyModelBinding{Mode: WeeklyWorking, OwnerScope: command.WorkspaceID, DescriptorSHA256: d.Digest(), InputSHA256: input, PolicySHA256: policy, QualificationSHA256: strings.Repeat("c", 64), ActivationSHA256: strings.Repeat("d", 64), TerminalOperation: "working_projection_only"}
	grant.weekly = &initial
	legacy := command
	legacy.WeeklyMode = ""
	legacy.ProposalOnly = true
	if ValidateWeeklyModelExecution(grant, legacy, d, input, policy) == nil {
		t.Fatal("weekly model grant accepted downgraded legacy proposal command")
	}
	if err := ValidateWeeklyModelExecution(grant, command, d, input, policy); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*weeklyModelBinding){func(b *weeklyModelBinding) { b.Mode = "unknown" }, func(b *weeklyModelBinding) { b.ProbeIntentSHA256 = strings.Repeat("e", 64) }, func(b *weeklyModelBinding) { b.OwnerScope = "another-owner" }, func(b *weeklyModelBinding) { b.InputSHA256 = strings.Repeat("f", 64) }, func(b *weeklyModelBinding) { b.PolicySHA256 = strings.Repeat("e", 64) }, func(b *weeklyModelBinding) { b.TerminalOperation = "qualification_evidence_only" }, func(b *weeklyModelBinding) { b.QualificationSHA256 = "" }, func(b *weeklyModelBinding) { b.ActivationSHA256 = "" }} {
		copy := initial
		change(&copy)
		grant.weekly = &copy
		if err := ValidateWeeklyModelExecution(grant, command, d, input, policy); err == nil {
			t.Fatal("mixed/stale/cross-owner mode accepted")
		}
	}
	grant.weekly = nil
	if err := ValidateWeeklyModelExecution(grant, command, d, input, policy); err == nil {
		t.Fatal("ordinary grant acquired model authority")
	}
}
