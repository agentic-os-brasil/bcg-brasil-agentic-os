package setup_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installer"
)

// Exercise shipping defaults on the actual retained installation, independently
// of the explicitly engineering-only native-positive process constructor.
func TestCurrentClientShippingNativeRefusal(t *testing.T) {
	root := os.Getenv("MAESTRO_CLIENT_REGISTRATION_TEST_ROOT")
	if root == "" {
		t.Skip("explicit actual current-client fixture required")
	}
	var fixture struct{ Self, ManagedRoot, UserHome, Workspace string }
	must(t, json.Unmarshal(read(t, filepath.Join(root, "native-process.json")), &fixture))
	records, err := filepath.Glob(filepath.Join(fixture.UserHome, ".maestro-install/clients/*.json"))
	must(t, err)
	var record installer.ClientRecord
	selected := 0
	for _, path := range records {
		var candidate installer.ClientRecord
		must(t, json.Unmarshal(read(t, path), &candidate))
		if candidate.Materialization.CodeRoot == filepath.Join(root, "stable-code") {
			record = candidate
			selected++
		}
	}
	if selected != 1 {
		t.Fatal("one exact primary actual finite current record required")
	}
	request := map[string]any{"managed_root": fixture.ManagedRoot, "user_home": fixture.UserHome, "client": installer.ClientRequest{CarrierID: record.CarrierID, Workspace: record.Binding.Workspace, CodeRoot: record.Materialization.CodeRoot, DesktopPath: record.Materialization.DesktopPath, CodeExecutable: record.Code.Executable, CodeScope: record.Code.Scope, Reconcile: true}}
	body, _ := json.Marshal(request)
	fixed := []string{"--product-root", fixture.ManagedRoot, "--user-home", fixture.UserHome, "--workspace", fixture.Workspace, "--carrier-id", record.CarrierID}
	cases := [][]string{append([]string{"stable-launch"}, fixed...), {"clients-review"}, {"clients-apply", "--confirm", "--review-digest", strings.Repeat("f", 64)}, {"clients-reconcile", "--confirm", "--review-digest", strings.Repeat("f", 64)}, append([]string{"clients-status"}, fixed...), append([]string{"code-review"}, fixed...)}
	before := snapshot(t, root)
	var results []any
	for _, args := range cases {
		command := exec.Command(fixture.Self, args...)
		command.Stdin = bytes.NewReader(body)
		output, e := command.CombinedOutput()
		var exit *exec.ExitError
		if !errors.As(e, &exit) || exit.ExitCode() != 1 || !bytes.Contains(output, []byte("native installer self-trust")) {
			t.Fatalf("shipping %v did not preserve actual native-default refusal: %s %v", args, output, e)
		}
		results = append(results, map[string]any{"argv": append([]string{fixture.Self}, args...), "exit_code": exit.ExitCode(), "output": string(output)})
	}
	if !reflect.DeepEqual(before, snapshot(t, root)) {
		t.Fatal("shipping native refusals changed retained installation/owner/workspace")
	}
	proof, _ := json.MarshalIndent(map[string]any{"actual_shipping_installer_sha256": sum(read(t, fixture.Self)), "cases": results, "complete_fixture_bytes_modes_unchanged": true, "native_override": false}, "", "  ")
	must(t, os.WriteFile(root+".shipping-native-negative.json", append(proof, '\n'), 0600))
}
