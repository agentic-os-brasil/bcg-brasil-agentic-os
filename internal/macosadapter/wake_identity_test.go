package macosadapter

import (
	"strings"
	"testing"
)

func TestOwnedWakeRequiresExactLoadedArguments(t *testing.T) {
	spec := Spec{Label: "com.bcg.maestro.maintenance." + strings.Repeat("a", 64), Program: "/synthetic/Developer/maestro-install", Arguments: []string{"maintenance-wake", "--workspace", "/synthetic/Developer/Case A"}, StartInterval: 900}
	valid := "program = /synthetic/Developer/maestro-install\narguments = {\n/synthetic/Developer/maestro-install\nmaintenance-wake\n--workspace\n/synthetic/Developer/Case A\n}\n"
	if err := VerifyLoadedArguments(valid, spec); err != nil {
		t.Fatal(err)
	}
	for _, body := range []string{strings.Replace(valid, "Case A", "Case B", 1), strings.Replace(valid, "maintenance-wake\n", "other\n", 1), "program = /synthetic/Developer/maestro-install\n", valid + "arguments = {\n}\n"} {
		if VerifyLoadedArguments(body, spec) == nil {
			t.Fatal("same program with unverified foreign args accepted")
		}
	}
	for _, label := range []string{"com.bcg.maestro.maintenance", spec.Label} {
		if err := validateNativeLabel(label); err != nil {
			t.Fatal(err)
		}
	}
	for _, label := range []string{"com.bcg.maestro.maintenance.other", "foreign." + strings.Repeat("a", 64), "com.bcg.maestro.maintenance." + strings.Repeat("A", 64), strings.Repeat("a", 64)} {
		if validateNativeLabel(label) == nil {
			t.Fatal("foreign OS label admitted")
		}
	}
}
