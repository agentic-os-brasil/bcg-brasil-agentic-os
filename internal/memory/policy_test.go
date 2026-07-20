package memory

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestRepositoryPolicyIsValid(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate test file")
	}
	path := filepath.Join(filepath.Dir(currentFile), "..", "..", "bundles", "base", "memory", "policy.json")

	policy, err := LoadFile(path)
	if err != nil {
		t.Fatalf("load policy: %v", err)
	}
	if err := policy.Validate(); err != nil {
		t.Fatalf("validate policy: %v", err)
	}
}

func TestRepositorySchemaIsValid(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate test file")
	}
	path := filepath.Join(filepath.Dir(currentFile), "..", "..", "schemas", "memory-policy.schema.json")
	if err := ValidateSchemaFile(path); err != nil {
		t.Fatalf("validate schema: %v", err)
	}
	artifactPath := filepath.Join(filepath.Dir(currentFile), "..", "..", "schemas", "memory-artifact.schema.json")
	if err := ValidateArtifactSchemaFile(artifactPath); err != nil {
		t.Fatalf("validate artifact schema: %v", err)
	}
	commitPath := filepath.Join(filepath.Dir(currentFile), "..", "..", "schemas", "memory-commit.schema.json")
	if err := ValidateCommitSchemaFile(commitPath); err != nil {
		t.Fatalf("validate commit schema: %v", err)
	}
}

func TestLoadFileRejectsUnknownFieldsAndTrailingValues(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "unknown field", body: `{"schema_version":1,"unexpected":true}`},
		{name: "trailing value", body: `{}` + "\n" + `{}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "policy.json")
			if err := os.WriteFile(path, []byte(test.body), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := LoadFile(path); err == nil {
				t.Fatal("expected decode error")
			}
		})
	}
}

func TestPolicyRejectsUnsafeMemoryContracts(t *testing.T) {
	valid := validPolicyForTest()

	tests := []struct {
		name   string
		mutate func(*Policy)
	}{
		{
			name: "wrong layer order",
			mutate: func(policy *Policy) {
				policy.Layers[0], policy.Layers[1] = policy.Layers[1], policy.Layers[0]
			},
		},
		{
			name: "lifetime overwrite",
			mutate: func(policy *Policy) {
				policy.Lifetime.DirectOverwrite = true
			},
		},
		{
			name: "unversioned lifetime update",
			mutate: func(policy *Policy) {
				policy.Lifetime.VersionedUpdates = false
			},
		},
		{
			name: "automatic lifetime update without eligibility policy",
			mutate: func(policy *Policy) {
				policy.Lifetime.Eligibility = ""
			},
		},
		{
			name: "daily lifetime write",
			mutate: func(policy *Policy) {
				policy.Cycles.Daily.LifetimeWrite = true
			},
		},
		{
			name: "weekly without lifetime write",
			mutate: func(policy *Policy) {
				policy.Cycles.Weekly.LifetimeWrite = false
			},
		},
		{
			name: "source mutation",
			mutate: func(policy *Policy) {
				policy.Dreaming.MutateSources = true
			},
		},
		{
			name: "activation without validation",
			mutate: func(policy *Policy) {
				policy.Dreaming.ValidateBeforeActivation = false
			},
		},
		{
			name: "user data in managed core",
			mutate: func(policy *Policy) {
				policy.Storage.ManagedCoreContainsUserData = true
			},
		},
		{
			name: "raw unbounded fallback",
			mutate: func(policy *Policy) {
				policy.ContextInjection.RawUnboundedFallback = true
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			policy := valid
			policy.Layers = append([]Layer(nil), valid.Layers...)
			policy.ContextInjection.Order = append([]string(nil), valid.ContextInjection.Order...)
			test.mutate(&policy)
			if err := policy.Validate(); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func validPolicyForTest() Policy {
	return Policy{
		SchemaVersion: 1,
		Storage: StoragePolicy{
			Scope:                       "user_local",
			WorkspaceIsolation:          true,
			ManagedCoreContainsUserData: false,
		},
		Layers: []Layer{
			{ID: "L1", Role: "daily", Input: "sanitized_signals", SourceMode: "append_only", Budget: "required_runtime_configuration", DrillDown: true},
			{ID: "L2", Role: "weekly", Input: "L1", SourceMode: "immutable", Budget: "required_runtime_configuration", DrillDown: true},
			{ID: "L3", Role: "medium_term_thematic", Input: "L2", SourceMode: "immutable", Budget: "required_runtime_configuration", DrillDown: true},
		},
		Lifetime: LifetimePolicy{Input: "L3", Promotion: "weekly_deep_dream", Automatic: true, Eligibility: "required_policy", VersionedUpdates: true, DirectOverwrite: false, Budget: "required_runtime_configuration", DrillDown: true},
		Cycles: DreamingCycles{
			Daily:  DreamingCycle{Depth: "light", Outputs: []string{"L1"}, LifetimeWrite: false},
			Weekly: DreamingCycle{Depth: "deep", Outputs: []string{"L2", "L3", "lifetime"}, LifetimeWrite: true},
		},
		Dreaming: DreamingPolicy{
			SourceSelection:          "deterministic",
			Idempotency:              "source_fingerprint",
			StageBeforeActivation:    true,
			ValidateBeforeActivation: true,
			AtomicActivation:         true,
			PreserveLastKnownGood:    true,
			MutateSources:            false,
			ProvenanceRequired:       true,
		},
		ContextInjection: ContextInjectionPolicy{
			Order:                  []string{"lifetime", "L3", "L2", "L1"},
			PerLayerBudgetRequired: true,
			MissingLayer:           "skip_with_diagnostic",
			RawUnboundedFallback:   false,
		},
	}
}
