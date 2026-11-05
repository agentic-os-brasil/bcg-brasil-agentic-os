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

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/datahome"
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
	call(cs, "maestro_apply", "memory.delete", map[string]any{"capture_id": corrected["id"], "expected_revision": corrected["revision"]}, false)
	call(cs, "maestro_query", "memory.read", map[string]any{"capture_id": corrected["id"]}, true)
	search := call(cs, "maestro_query", "memory.search", map[string]any{"query": "WINDOWS_CORE_CAPTURE"}, false)
	if search["total"] != float64(0) {
		t.Fatal("deleted/corrected memory reappeared", search)
	}
	proof, _ := json.MarshalIndent(map[string]any{"native": "windows", "elevated": false, "admin_member": false, "server_sha256": hex.EncodeToString(serverHash[:]), "work_id": id, "resumed_revision": resumed["state_revision"], "memory_restart": true, "declined_bytes_unchanged": true, "deleted_not_searchable": true, "review_host": "controlled synthetic SDK elicitation; native client loading remains separate"}, "", "  ")
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
