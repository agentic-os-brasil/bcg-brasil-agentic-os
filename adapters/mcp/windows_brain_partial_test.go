//go:build windows

package product

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/atlasops"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
	"golang.org/x/sys/windows"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/datahome"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ownerctx"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspace"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestWindowsAuthoredBrainPartialPreparationPilot(t *testing.T) {
	if os.Getenv("MAESTRO_WINDOWS_CORE_PROOF") != "1" {
		t.Skip("dedicated actual ordinary-account driver required")
	}
	admin, err := windows.CreateWellKnownSid(windows.WinBuiltinAdministratorsSid)
	if err != nil {
		t.Fatal(err)
	}
	member, err := windows.Token(0).IsMember(admin)
	if err != nil || member || windows.GetCurrentProcessToken().IsElevated() {
		t.Fatal("actual ordinary nonadmin Windows token required", err, member)
	}
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	options := Options{ProductRoot: filepath.Join(base, "product"), UserHome: filepath.Join(base, "user"), WorkspacePath: filepath.Join(base, "workspace"), LookupEnv: func(string) (string, bool) { return "", false }}
	for _, path := range []string{options.ProductRoot, options.UserHome} {
		if err = scheduler.EnsurePrivateDirectory(path); err != nil {
			t.Fatal(err)
		}
	}
	initialized, err := datahome.InitializeFresh(datahome.Options{ProductRoot: options.ProductRoot, UserHome: options.UserHome, LookupEnv: options.LookupEnv})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = workspace.Initialize(workspace.Options{WorkspacePath: options.WorkspacePath, DataRoot: filepath.Join(initialized.Home, "state")}); err != nil {
		t.Fatal(err)
	}
	if _, err = ownerctx.Initialize(filepath.Join(initialized.Home, "brain")); err != nil {
		t.Fatal(err)
	}
	newer := filepath.Join(options.WorkspacePath, "brain", "projects", "template-project.md")
	const newerBody = "# Preserve native newer hand-edited template\n\n## Decisions\n"
	if err = scheduler.EnsurePrivateDirectory(filepath.Dir(newer)); err != nil {
		t.Fatal(err)
	}
	if err = scheduler.WritePrivateFileAtomic(newer, []byte(newerBody)); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(base, "outside.md")
	const outsideBody = "# Preserve native outside canary\n"
	if err = scheduler.WritePrivateFileAtomic(outside, []byte(outsideBody)); err != nil {
		t.Fatal(err)
	}
	snapshot := func(root string) map[string]string {
		values := map[string]string{}
		if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			info, err := entry.Info()
			if err != nil {
				return err
			}
			value := fmt.Sprint(info.Mode())
			if !entry.IsDir() {
				body, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				value += string(body)
			}
			values[strings.TrimPrefix(path, root)] = value
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		return values
	}
	ownerBefore, productBefore := snapshot(filepath.Join(initialized.Home, "brain")), snapshot(options.ProductRoot)
	bound, err := Open(options)
	if err != nil {
		t.Fatal(err)
	}
	bound.afterBrainPrepareEffect = func() error { return errors.New("synthetic interruption after one actual prepared page effect") }
	a, z := mcp.NewInMemoryTransports()
	server, err := bound.Server().Connect(context.Background(), a, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "partial-brain-sdk-proof", Version: "1"}, &mcp.ClientOptions{ElicitationHandler: func(context.Context, *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"approved": true}}, nil
	}})
	session, err := client.Connect(context.Background(), z, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	call := func(tool, operation string) (map[string]any, bool) {
		out, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: tool, Arguments: map[string]any{"operation": operation}})
		if err != nil {
			t.Fatal(err)
		}
		var value map[string]any
		if len(out.Content) != 1 {
			t.Fatal("unexpected partial result", out)
		}
		if err = json.Unmarshal([]byte(out.Content[0].(*mcp.TextContent).Text), &value); err != nil {
			t.Fatal(err)
		}
		return value, out.IsError
	}
	status, failed := call("maestro_query", "brain.status")
	if failed {
		t.Fatal(status)
	}
	firstMissing := status["missing"].([]any)[0].(string)
	partial, failed := call("maestro_apply", "brain.prepare")
	firstPath := filepath.Join(options.WorkspacePath, "brain", filepath.FromSlash(firstMissing))
	actual, err := os.ReadFile(firstPath)
	if err != nil || len(actual) == 0 {
		t.Fatal("test interruption did not follow an actual prepared page publication", err)
	}
	if !failed || partial["code"] != "brain_prepare_incomplete" || partial["state"] != "incomplete" {
		t.Fatal("partial publication was reported as an opaque failure or success", partial)
	}
	completed, pending := partial["completed"].([]any), partial["pending"].([]any)
	if len(completed) == 0 || len(pending) == 0 {
		t.Fatal("partial preparation lost confirmed or unconfirmed page effects", partial)
	}
	confirmed := false
	for _, entry := range completed {
		result := entry.(map[string]any)
		if result["page"] == firstMissing && result["state"] == "written" {
			confirmed = true
		}
	}
	if !confirmed {
		t.Fatal("actual first prepared page missing from confirmed effects", partial)
	}
	bound.afterBrainPrepareEffect = nil
	retried, failed := call("maestro_apply", "brain.prepare")
	if failed || retried["state"] != "ready" {
		t.Fatal("explicit reviewed retry could not finish preparation", retried)
	}
	preserved, err := os.ReadFile(firstPath)
	if err != nil || string(preserved) != string(actual) {
		t.Fatal("explicit retry overwrote the prior completed page", err)
	}
	if got, err := os.ReadFile(newer); err != nil || string(got) != newerBody {
		t.Fatal("partial retry overwrote newer native template", err)
	}
	if got, err := os.ReadFile(outside); err != nil || string(got) != outsideBody {
		t.Fatal("partial preparation changed outside bytes", err)
	}
	if !reflect.DeepEqual(ownerBefore, snapshot(filepath.Join(initialized.Home, "brain"))) || !reflect.DeepEqual(productBefore, snapshot(options.ProductRoot)) {
		t.Fatal("partial preparation changed owner or managed product bytes/modes")
	}
	engine, err := bound.knowledgeEngine()
	if err != nil {
		t.Fatal(err)
	}
	current, err := engine.ReviewPage(atlasops.CollectRequest{Purpose: "Keep actual native completed page after malformed replacement", Reader: atlasops.ReaderOwnerSession, Pages: []string{firstMissing}})
	if err != nil {
		t.Fatal(err)
	}
	workspaceBefore := snapshot(options.WorkspacePath)
	for _, invalid := range []string{string([]byte{0xff}), string([]byte{0xfe})} {
		if _, err := engine.UpdatePage(atlasops.UpdatePageRequest{Page: firstMissing, Body: "# " + invalid, ExpectedRevision: current.Revision, Provenance: atlasops.Provenance{Origin: atlasops.OriginAttended, SessionID: "native-partial-utf8", IdempotencyKey: "same-native-malformed-update"}}); err == nil {
			t.Fatal("native owning Update normalized invalid UTF8")
		}
		if !reflect.DeepEqual(workspaceBefore, snapshot(options.WorkspacePath)) {
			t.Fatal("native malformed update changed completed page, journal or grants")
		}
	}
	t.Log("native_authored_partial actual_first_page_effect=true confirmed_receipt=true remaining_unconfirmed=true explicit_retry=true existing_newer_preserved=true owner_and_product_unchanged=true outside_unchanged=true malformed_update_utf8_refused=true")
	final, failed := call("maestro_query", "brain.status")
	if failed || final["prepared"] != true {
		t.Fatal("finished preparation remained incomplete", final)
	}
}
