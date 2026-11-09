//go:build windows

package product_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"unsafe"

	product "github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/datahome"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspace"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/zipmigration"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/zipruntime"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"golang.org/x/sys/windows"
)

// This witness runs under an actual temporary standard user, not the elevated CI token.
func TestWindowsCorePilot(t *testing.T) {
	if os.Getenv("MAESTRO_WINDOWS_CORE_PROOF") != "1" {
		t.Skip("the native standard-user driver supplies this dedicated acceptance environment")
	}
	token := windows.GetCurrentProcessToken()
	admin, err := windows.CreateWellKnownSid(windows.WinBuiltinAdministratorsSid)
	if err != nil {
		t.Fatal(err)
	}
	member, err := windows.Token(0).IsMember(admin)
	var elevation, returned uint32
	elevationErr := windows.GetTokenInformation(token, windows.TokenElevation, (*byte)(unsafe.Pointer(&elevation)), uint32(unsafe.Sizeof(elevation)), &returned)
	if err != nil || elevationErr != nil || returned != uint32(unsafe.Sizeof(elevation)) || elevation != 0 || member {
		t.Fatalf("ordinary-user kernel token required: elevated=%v admin_member=%v membership_error=%v elevation_error=%v", elevation != 0, member, err, elevationErr)
	}
	user, err := token.GetTokenUser()
	if err != nil || user == nil || user.User.Sid == nil {
		t.Fatal("native user SID unavailable", err)
	}
	sidHash := sha256.Sum256([]byte(user.User.Sid.String()))
	t.Logf("native_windows_token elevated=false admin_member=false user_sid_sha256=%x", sidHash)
	server := os.Getenv("MAESTRO_WINDOWS_CORE_SERVER")
	proofRoot := os.Getenv("MAESTRO_WINDOWS_CORE_PROOF_ROOT")
	if !filepath.IsAbs(server) || !filepath.IsAbs(proofRoot) {
		t.Fatal("driver must supply absolute binary and isolated proof roots")
	}
	// The credentialed launcher must supply a temp created by this actual
	// user, rather than inheriting the elevated runner's inaccessible profile.
	expectedTemp := filepath.Join(proofRoot, "user-temp")
	for _, key := range []string{"TEMP", "TMP"} {
		if !strings.EqualFold(filepath.Clean(os.Getenv(key)), expectedTemp) {
			t.Fatalf("native environment %s does not select the isolated user temp", key)
		}
	}
	if !strings.EqualFold(filepath.Clean(os.TempDir()), expectedTemp) {
		t.Fatal("native OS temp lookup differs from the isolated user temp")
	}
	assertWindowsCorePrivateFixture(t, expectedTemp, user.User.Sid)
	t.Log("native_windows_environment private_current_user_temp=true")
	serverBody, err := os.ReadFile(server)
	if err != nil {
		t.Fatal(err)
	}
	serverHash := sha256.Sum256(serverBody)
	t.Logf("ordinary_shipping_server_sha256=%x", serverHash)
	root, err := os.MkdirTemp(proofRoot, "core-")
	if err != nil {
		t.Fatal(err)
	}
	productRoot, home, ws := filepath.Join(root, "product"), filepath.Join(root, "owner"), filepath.Join(root, "workspace")
	for _, path := range []string{productRoot, home} {
		if err = os.Mkdir(path, 0700); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
	defer cancel()
	args := []string{"--product-root", productRoot, "--user-home", home, "--workspace", ws}
	command := func(op string) {
		t.Helper()
		out, err := exec.CommandContext(ctx, server, append([]string{op}, args...)...).CombinedOutput()
		if err != nil {
			// Preserve the public failure. These read-only synthetic diagnostics
			// identify its phase without changing production error privacy.
			inspectOut, shippingInspectErr := exec.CommandContext(ctx, server, append([]string{"inspect"}, args...)...).CombinedOutput()
			t.Logf("native_init_phase shipping_inspect=%s error=%v", inspectOut, shippingInspectErr)
			envAdmission, envErr := datahome.Resolve(datahome.Options{ProductRoot: productRoot, UserHome: home})
			t.Logf("native_init_phase environment_home_diagnostic=%s selected_by=%s environment_home_error=%v", envAdmission.Diagnostic, envAdmission.SelectedBy, envErr)
			paths := zipruntime.ResolvePaths(productRoot, zipruntime.Options{UserHome: home})
			legacy := zipmigration.Run(productRoot, zipmigration.Options{Status: true})
			t.Logf("native_init_phase zip_mode=%s zip_diagnostic=%s migration_state=%s migration_errors=%v", paths.Mode, paths.Diagnostic, legacy.State, legacy.Errors)
			workspaceErr := workspace.PreflightInitialize(workspace.Options{WorkspacePath: ws, DataRoot: filepath.Join(home, ".maestro", "state")})
			preflightErr := product.Preflight(product.Options{ProductRoot: productRoot, UserHome: home, WorkspacePath: ws})
			t.Logf("native_init_phase workspace_preflight_error=%v product_preflight_error=%v", workspaceErr, preflightErr)
			admission, inspectErr := datahome.Resolve(datahome.Options{ProductRoot: productRoot, UserHome: home, LookupEnv: func(string) (string, bool) { return "", false }})
			meta, _ := json.Marshal(admission)
			t.Fatalf("public %s refused: %s %v; read-only admission=%s inspect_error=%v", op, out, err, meta, inspectErr)
		}
	}
	command("init")
	command("inspect")
	if err = os.WriteFile(filepath.Join(ws, "result.md"), []byte("synthetic Windows artifact\n"), 0600); err != nil {
		t.Fatal(err)
	}
	var approve atomic.Bool
	approve.Store(true)
	connect := func() *mcp.ClientSession {
		t.Helper()
		client := mcp.NewClient(&mcp.Implementation{Name: "windows-core-native-proof", Version: "1"}, &mcp.ClientOptions{
			ElicitationHandler: func(_ context.Context, r *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
				if approve.Load() && strings.Contains(r.Params.Message, "WINDOWS_CORE_") {
					return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"approved": true}}, nil
				}
				return &mcp.ElicitResult{Action: "decline"}, nil
			},
		})
		cs, err := client.Connect(ctx, &mcp.CommandTransport{Command: exec.CommandContext(ctx, server, append([]string{"serve"}, args...)...), TerminateDuration: time.Second}, nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { cs.Close() })
		return cs
	}
	call := func(cs *mcp.ClientSession, tool, op string, extra map[string]any, fail bool) map[string]any {
		t.Helper()
		in := map[string]any{"operation": op}
		for k, v := range extra {
			in[k] = v
		}
		out, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: tool, Arguments: in})
		if err != nil || out == nil || out.IsError != fail || len(out.Content) == 0 {
			t.Fatalf("%s result=%+v error=%v", op, out, err)
		}
		var got map[string]any
		if err = json.Unmarshal([]byte(out.Content[0].(*mcp.TextContent).Text), &got); err != nil {
			t.Fatal(err)
		}
		return got
	}
	cs := connect()
	created := call(cs, "maestro_apply", "execution.create", map[string]any{"objective": "WINDOWS_CORE_WORK", "next_step": "write synthetic result", "artifacts": []string{"result.md"}}, false)
	id := created["item_id"].(string)
	started := call(cs, "maestro_apply", "execution.start", map[string]any{"item_id": id, "expected_revision": created["state_revision"]}, false)
	cp := call(cs, "maestro_apply", "execution.checkpoint", map[string]any{"item_id": id, "expected_revision": started["state_revision"], "attempt_id": started["attempt_id"], "summary": "WINDOWS_CORE_CHECKPOINT", "next_step": "resume same native case"}, false)
	paused := call(cs, "maestro_apply", "execution.pause", map[string]any{"item_id": id, "expected_revision": cp["state_revision"], "attempt_id": started["attempt_id"]}, false)
	if err = cs.Close(); err != nil {
		t.Fatal(err)
	}
	cs = connect()
	resumed := call(cs, "maestro_apply", "execution.resume", map[string]any{"item_id": id, "expected_revision": paused["state_revision"]}, false)
	if resumed["attempt_id"] == started["attempt_id"] {
		t.Fatal("restarted resume reused the old attempt")
	}
	next := call(cs, "maestro_query", "execution.next", map[string]any{"item_id": id}, false)
	if next["next_step"] != "resume same native case" {
		t.Fatal("native checkpoint was not retained", next)
	}
	captured := call(cs, "maestro_apply", "memory.capture", map[string]any{"text": "WINDOWS_CORE_CAPTURE synthetic approved memory"}, false)
	survivor := call(cs, "maestro_apply", "memory.capture", map[string]any{"text": "WINDOWS_CORE_SURVIVOR selected surviving memory"}, false)
	day := time.Now().UTC().Format("2006-01-02")
	call(cs, "maestro_apply", "memory.consolidate", map[string]any{"cycle": "daily", "date": day}, false)
	initialContext := call(cs, "maestro_query", "memory.context", nil, false)
	if !strings.Contains(initialContext["text"].(string), "WINDOWS_CORE_CAPTURE") {
		t.Fatal("approved memory did not reach canonical consolidated context")
	}
	if err = cs.Close(); err != nil {
		t.Fatal(err)
	}
	cs = connect()
	read := call(cs, "maestro_query", "memory.read", map[string]any{"capture_id": captured["id"]}, false)
	if read["text"] != "WINDOWS_CORE_CAPTURE synthetic approved memory" {
		t.Fatal("native approved capture did not survive restart", read)
	}
	approve.Store(false)
	before := windowsCoreTree(t, home)
	call(cs, "maestro_apply", "memory.capture", map[string]any{"text": "WINDOWS_CORE_DECLINED must not persist"}, true)
	if !reflect.DeepEqual(before, windowsCoreTree(t, home)) {
		t.Fatal("declined review changed private owner bytes")
	}
	approve.Store(true)
	corrected := call(cs, "maestro_apply", "memory.correct", map[string]any{"capture_id": captured["id"], "expected_revision": captured["revision"], "text": "WINDOWS_CORE_REPLACEMENT synthetic correction"}, false)
	call(cs, "maestro_apply", "memory.consolidate", map[string]any{"cycle": "daily", "date": day}, false)
	correctedContext := call(cs, "maestro_query", "memory.context", nil, false)
	if strings.Contains(correctedContext["text"].(string), "WINDOWS_CORE_CAPTURE") || !strings.Contains(correctedContext["text"].(string), "WINDOWS_CORE_REPLACEMENT") {
		t.Fatal("canonical correction retained the removed text or lost the replacement")
	}
	call(cs, "maestro_apply", "memory.delete", map[string]any{"capture_id": corrected["id"], "expected_revision": corrected["revision"]}, false)
	// Force the existing canonical historical-manifest fallback through a corrupt
	// synthetic newest manifest, then rebuild from the surviving reviewed source.
	commits := windowsCoreCommits(t, home)
	if err = scheduler.PublishNewPrivateFile(filepath.Join(commits, "99999999T999999.999999999Z-broken.json"), []byte(`{"corrupt":true}`)); err != nil {
		t.Fatal(err)
	}
	afterFallback := call(cs, "maestro_query", "memory.context", nil, false)
	if strings.Contains(afterFallback["text"].(string), "WINDOWS_CORE_CAPTURE") || strings.Contains(afterFallback["text"].(string), "WINDOWS_CORE_REPLACEMENT") {
		t.Fatal("historical fallback resurrected removed text")
	}
	call(cs, "maestro_apply", "memory.consolidate", map[string]any{"cycle": "daily", "date": day}, false)
	rebuilt := call(cs, "maestro_query", "memory.context", nil, false)
	if !strings.Contains(rebuilt["text"].(string), "WINDOWS_CORE_SURVIVOR") || strings.Contains(rebuilt["text"].(string), "WINDOWS_CORE_REPLACEMENT") {
		t.Fatal("native rebuild lost the surviving source or resurrected the removed source")
	}
	call(cs, "maestro_apply", "memory.delete", map[string]any{"capture_id": survivor["id"], "expected_revision": survivor["revision"]}, false)
	empty := call(cs, "maestro_query", "memory.context", nil, false)
	if empty["text"] != "" {
		t.Fatal("deleting final source did not produce valid empty context")
	}
	if err = cs.Close(); err != nil {
		t.Fatal(err)
	}
	cs = connect()
	call(cs, "maestro_query", "memory.read", map[string]any{"capture_id": corrected["id"]}, true)
	search := call(cs, "maestro_query", "memory.search", map[string]any{"query": "WINDOWS_CORE_CAPTURE"}, false)
	if search["total"] != float64(0) {
		t.Fatal("deleted/corrected memory reappeared", search)
	}
	proof, _ := json.MarshalIndent(map[string]any{"native": "windows", "elevated": false, "admin_member": false, "server_sha256": hex.EncodeToString(serverHash[:]), "work_id": id, "resumed_revision": resumed["state_revision"], "memory_restart": true, "declined_bytes_unchanged": true, "deleted_not_searchable": true, "historical_fallback_no_resurrection": true, "surviving_source_rebuild": true, "final_source_empty": true, "review_host": "controlled synthetic SDK elicitation; native client loading remains separate"}, "", "  ")
	if err = os.WriteFile(filepath.Join(proofRoot, "native-core-proof.json"), append(proof, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
}

func windowsCoreTree(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	if err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if d.IsDir() {
			out[rel] = "directory"
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		h := sha256.Sum256(body)
		out[rel] = hex.EncodeToString(h[:])
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return out
}

func windowsCoreCommits(t *testing.T, home string) string {
	t.Helper()
	var found []string
	root := filepath.Join(home, ".maestro", "brain", "memory")
	if err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && d.Name() == "commits" {
			found = append(found, path)
			return filepath.SkipDir
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(found) != 1 {
		t.Fatal("synthetic single-workspace commit fixture was not uniquely identified", len(found))
	}
	return found[0]
}

// Attest the launcher fixture independently of the production private-state
// implementation, so an unavailable engine cannot become its own prerequisite.
func assertWindowsCorePrivateFixture(t *testing.T, path string, user *windows.SID) {
	t.Helper()
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	attributes, err := windows.GetFileAttributes(name)
	if err != nil || attributes&windows.FILE_ATTRIBUTE_DIRECTORY == 0 || attributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		t.Fatal("native temporary fixture must be an ordinary directory", err)
	}
	security, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal("native temporary fixture security unavailable", err)
	}
	owner, _, err := security.Owner()
	if err != nil || owner == nil || !owner.Equals(user) {
		t.Fatal("native temporary fixture must belong to the actual ordinary user", err)
	}
	acl, _, err := security.DACL()
	if err != nil || acl == nil || acl.AceCount == 0 {
		t.Fatal("native temporary fixture requires a finite private DACL", err)
	}
	ownControl := false
	requiredControl := windows.ACCESS_MASK(windows.FILE_GENERIC_READ | windows.FILE_GENERIC_WRITE | windows.FILE_GENERIC_EXECUTE | windows.DELETE | windows.WRITE_DAC | windows.WRITE_OWNER)
	for i := uint16(0); i < acl.AceCount; i++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(acl, uint32(i), &ace); err != nil || ace == nil {
			t.Fatal("native temporary fixture ACE unavailable", err)
		}
		if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE {
			t.Fatal("native temporary fixture has an unsupported ACE")
		}
		principal := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		if !principal.Equals(user) && !principal.IsWellKnown(windows.WinLocalSystemSid) && !principal.IsWellKnown(windows.WinBuiltinAdministratorsSid) {
			t.Fatal("native temporary fixture grants an unrelated principal")
		}
		if principal.Equals(user) && ace.Header.AceFlags&windows.INHERIT_ONLY_ACE == 0 && ace.Mask&requiredControl == requiredControl {
			ownControl = true
		}
	}
	if !ownControl {
		t.Fatal("native temporary fixture lacks effective current-user control")
	}
}
