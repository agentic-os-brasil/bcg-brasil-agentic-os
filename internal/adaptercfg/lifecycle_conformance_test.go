package adaptercfg

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

type lifecycleAdapterFixture struct {
	Events []lifecycleAdapterFixtureEvent `json:"events"`
}

type lifecycleAdapterFixtureEvent struct {
	Claude lifecycleAdapterFixtureRuntime `json:"claude"`
	Codex  lifecycleAdapterFixtureRuntime `json:"codex"`
}

type lifecycleAdapterFixtureRuntime struct {
	Binding        string `json:"binding"`
	Implementation string `json:"implementation"`
}

func TestLifecycleConformanceFixtureMatchesInstalledBindingTopology(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "..", "adapters", "conformance", "lifecycle.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixture lifecycleAdapterFixture
	if err := json.Unmarshal(body, &fixture); err != nil {
		t.Fatal(err)
	}
	for _, runtimeName := range []string{"claude", "codex"} {
		bindings, err := bindingsFor(runtimeName, "/opt/maestro/bcgos")
		if err != nil {
			t.Fatal(err)
		}
		byEvent := map[string]binding{}
		for _, binding := range bindings {
			byEvent[binding.NativeEvent] = binding
		}
		for _, row := range fixture.Events {
			contract := row.Claude
			if runtimeName == "codex" {
				contract = row.Codex
			}
			if contract.Implementation == "not_implemented" {
				if contract.Binding != "none" {
					t.Fatalf("%s unimplemented fixture has binding %q", runtimeName, contract.Binding)
				}
				continue
			}
			binding, found := byEvent[contract.Binding]
			if !found || binding.Command == "" {
				t.Fatalf("%s binding %q is absent from installed topology", runtimeName, contract.Binding)
			}
			wantAsync := contract.Implementation == "configured_async"
			if binding.Async != wantAsync {
				t.Fatalf("%s binding %q async=%v, want %v", runtimeName, contract.Binding, binding.Async, wantAsync)
			}
		}
	}
}
