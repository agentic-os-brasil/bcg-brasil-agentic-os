package maestro

import (
	"encoding/json"
	"os"
	"testing"
)

func TestMaestroAdapterQualityLoopFixtureCoversFourPathsAndDirectAnswer(t *testing.T) {
	body, err := os.ReadFile("../../adapters/conformance/maestro-quality-loop.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		SchemaVersion int `json:"schema_version"`
		Paths         []struct {
			Name                 string      `json:"name"`
			IntentClass          IntentClass `json:"intent_class"`
			ExecutionOnly        bool        `json:"execution_only"`
			StrategicImplication bool        `json:"strategic_implication"`
			Materiality          Materiality `json:"materiality"`
			CaseEntry            CaseEntry   `json:"case_entry"`
			AccountValidation    bool        `json:"account_validation"`
			WalterRequired       bool        `json:"walter_required"`
		} `json:"paths"`
	}
	if err := json.Unmarshal(body, &fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.SchemaVersion != 1 || len(fixture.Paths) != 5 {
		t.Fatalf("quality-loop fixture drifted: %#v", fixture)
	}
	for _, path := range fixture.Paths {
		input := caseInput(path.ExecutionOnly)
		if path.IntentClass != "" {
			input.IntentClass = path.IntentClass
		}
		input.ExecutionOnly = path.ExecutionOnly
		input.StrategicImplication = path.StrategicImplication
		if path.Materiality != "" {
			input.Materiality = path.Materiality
		}
		plan, err := PlanFor(input)
		if err != nil {
			t.Fatalf("%s: %v", path.Name, err)
		}
		if string(plan.CaseEntry) != string(path.CaseEntry) || plan.RequiresAccountValidation != path.AccountValidation || plan.RequiresWalter != path.WalterRequired {
			t.Fatalf("%s: plan=%#v", path.Name, plan)
		}
	}
}
