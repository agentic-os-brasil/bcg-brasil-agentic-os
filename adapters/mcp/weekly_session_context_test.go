package product

import (
	"encoding/json"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/datahome"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/sessionhook"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspace"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWeeklySessionMergePreservesCanonicalWhenUnavailable(t *testing.T) {
	canonical := strings.Repeat("a", 64)
	for _, row := range []struct {
		text, gen, accepted, status string
		maximum                     int
	}{
		{"optional", strings.Repeat("b", 64), canonical, "unavailable", 4096},
		{"optional", strings.Repeat("b", 64), strings.Repeat("c", 64), "available", 4096},
		{"optional", strings.Repeat("b", 64), canonical, "available", 5},
		{"", strings.Repeat("b", 64), canonical, "available", 4096},
	} {
		body, gen, applied := mergeWeeklySessionWorking("canonical", canonical, row.text, row.gen, row.accepted, row.status, row.maximum)
		if body != "canonical" || gen != canonical || applied {
			t.Fatal("optional view damaged canonical context")
		}
	}
}
func TestWeeklySessionMergePinsCombinedGenerationForPaging(t *testing.T) {
	canonical := strings.Repeat("a", 64)
	derived := strings.Repeat("b", 64)
	body, gen, applied := mergeWeeklySessionWorking("canonical", canonical, "derived advisory", derived, canonical, "available", 4096)
	if !applied || body != "canonical\nderived advisory" || len(gen) != 64 || gen == canonical {
		t.Fatal("derived context lost independent generation")
	}
	_, same, _ := mergeWeeklySessionWorking("canonical", canonical, "derived advisory", derived, canonical, "available", 4096)
	_, changed, _ := mergeWeeklySessionWorking("canonical", canonical, "different advisory", strings.Repeat("c", 64), canonical, "available", 4096)
	if same != gen || changed == gen {
		t.Fatal("paging generation did not bind exact derived snapshot")
	}
}

func TestWeeklySessionHookPreservesNormalProjectionAndBudget(t *testing.T) {
	root := t.TempDir()
	prod, owner, work := filepath.Join(root, "product"), filepath.Join(root, "owner"), filepath.Join(root, "workspace")
	for _, path := range []string{prod, owner} {
		if err := os.Mkdir(path, 0700); err != nil {
			t.Fatal(err)
		}
	}
	lookup := func(string) (string, bool) { return "", false }
	home, err := datahome.InitializeFresh(datahome.Options{ProductRoot: prod, UserHome: owner, LookupEnv: lookup})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := workspace.Initialize(workspace.Options{WorkspacePath: work, DataRoot: filepath.Join(home.Home, "state")}); err != nil {
		t.Fatal(err)
	}
	bound, err := Open(Options{ProductRoot: prod, UserHome: owner, WorkspacePath: work, LookupEnv: lookup})
	if err != nil {
		t.Fatal(err)
	}
	original, err := bound.claudeLifecycleContext("SessionStart")
	if err != nil {
		t.Fatal(err)
	}
	if original.HookSpecificOutput.HookEventName != "SessionStart" || !strings.Contains(original.HookSpecificOutput.AdditionalContext, "MAESTRO PRODUCT CONTEXT") {
		t.Fatal("normal original SessionStart projection missing")
	}
	canonical, derived := strings.Repeat("a", 64), strings.Repeat("b", 64)
	text := "MAESTRO DERIVED YODA WORKING CONTEXT\nSynthetic advisory profile data only."
	got := appendWeeklySessionHook(original, "SessionStart", text, derived, canonical, canonical, "available")
	if got.HookSpecificOutput.HookEventName != original.HookSpecificOutput.HookEventName || got.HookSpecificOutput.AdditionalContext != original.HookSpecificOutput.AdditionalContext+"\n"+text {
		t.Fatal("working data displaced normal SessionStart instructions or memory")
	}
	for _, row := range []struct{ event, accepted, status string }{
		{"SessionStart", strings.Repeat("c", 64), "available"},
		{"SessionStart", canonical, "unavailable"},
		{"UserPromptSubmit", canonical, "available"},
	} {
		if got := appendWeeklySessionHook(original, row.event, text, derived, row.accepted, canonical, row.status); got != original {
			t.Fatal("stale/refused/other-event working data changed legacy output")
		}
	}
	if got := appendWeeklySessionHook(original, "SessionStart", strings.Repeat("x", sessionhook.MaximumAdditionalContextBytes), derived, canonical, canonical, "available"); got != original {
		t.Fatal("overflow displaced original context")
	}
	escaped := original
	escaped.HookSpecificOutput.AdditionalContext = strings.Repeat("\x01", 4000)
	if encoded, err := json.Marshal(escaped); err != nil || len(encoded) > 24<<10 {
		t.Fatal("overflow fallback fixture must start inside original JSON cap")
	}
	if got := appendWeeklySessionHook(escaped, "SessionStart", strings.Repeat("x", 1000), derived, canonical, canonical, "available"); got != escaped {
		t.Fatal("escaped JSON overflow exceeded original outer cap")
	}
	if got := bound.withWeeklySessionHook(original, "SessionStart"); got != original {
		t.Fatal("unconfirmed/unqualified ordinary startup changed legacy projection")
	}
}
