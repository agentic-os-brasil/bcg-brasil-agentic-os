package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

func TestHomePathsCompiledCLI(t *testing.T) {
	// Same bytes at the same name must still expose an atomic replacement.
	// This also proves the snapshot releases its handle before the transition.
	controlRoot := homeTestDirectory(t)
	control, replacement := filepath.Join(controlRoot, "origin"), filepath.Join(controlRoot, "replacement")
	for _, path := range []string{control, replacement} {
		if err := scheduler.WriteNewPrivateFile(path, []byte("identity-control")); err != nil {
			t.Fatal(err)
		}
	}
	controlInfo := homeFileIdentitySnapshot(t, control)
	if err := os.Rename(replacement, control); err != nil {
		t.Fatal("closed identity snapshot prevented replacement control", err)
	}
	if os.SameFile(controlInfo, homeFileIdentitySnapshot(t, control)) {
		t.Fatal("identity oracle accepted a same-byte replacement")
	}
	user, product, binaries := homeTestDirectory(t), homeTestDirectory(t), homeTestDirectory(t)
	helper := filepath.Join(binaries, "maestro-runtime")
	if runtime.GOOS == "windows" {
		helper += ".exe"
	}
	if output, err := exec.Command("go", "build", "-o", helper, ".").CombinedOutput(); err != nil {
		t.Fatalf("build helper: %v %s", err, output)
	}
	home := filepath.Join(user, "canonical owner")
	invoke := func(command string, extra ...string) (int, string) {
		t.Helper()
		args := append([]string{"home", command, "--product-root", product, "--user-home", user}, extra...)
		cmd := exec.Command(helper, args...)
		for _, entry := range os.Environ() {
			if !strings.HasPrefix(entry, "MAESTRO_HOME=") {
				cmd.Env = append(cmd.Env, entry)
			}
		}
		cmd.Env = append(cmd.Env, "MAESTRO_HOME="+home)
		var out, diagnostic bytes.Buffer
		cmd.Stdout, cmd.Stderr = &out, &diagnostic
		err := cmd.Run()
		if err == nil {
			return 0, out.String()
		}
		if exit, ok := err.(*exec.ExitError); ok {
			return exit.ExitCode(), out.String()
		}
		t.Fatal(err)
		return -1, ""
	}
	if code, body := invoke("paths"); code != 2 || !strings.Contains(body, `"mode":"initialization_required"`) || strings.Contains(body, `"home"`) {
		t.Fatalf("unconfirmed: %d %s", code, body)
	}
	if output, err := exec.Command(helper, "preflight").CombinedOutput(); err != nil || !bytes.Contains(output, []byte(`"state":"non_elevated"`)) {
		t.Fatalf("compiled home readiness requires the actual non-elevated platform process: %s %v", output, err)
	}
	code, body := invoke("init")
	var ready struct {
		Home       string `json:"home"`
		Diagnostic string `json:"diagnostic"`
		Writable   bool   `json:"writable"`
	}
	if code != 0 || json.Unmarshal([]byte(body), &ready) != nil || ready.Home != home || ready.Diagnostic != "ready" || !ready.Writable {
		t.Fatalf("init readiness: %d %s", code, body)
	}
	origin := filepath.Join(home, "brain", ".maestro", "origin.json")
	beforeInfo := homeFileIdentitySnapshot(t, origin)
	before, err := scheduler.ReadPrivateFile(origin, 16<<10)
	if err != nil {
		t.Fatal("ready origin lacks actual private admission", err)
	}
	for _, directory := range []string{home, filepath.Join(home, "brain"), filepath.Dir(origin)} {
		if err := scheduler.ValidatePrivateDirectory(directory); err != nil {
			t.Fatal("ready home lacks actual private directory admission", err)
		}
	}
	if code, body := invoke("init"); code != 0 {
		t.Fatalf("idempotent compiled init: %d %s", code, body)
	}
	afterInfo := homeFileIdentitySnapshot(t, origin)
	after, err := scheduler.ReadPrivateFile(origin, 16<<10)
	if err != nil || !os.SameFile(beforeInfo, afterInfo) || !bytes.Equal(before, after) {
		t.Fatal("compiled restart/init replaced or changed private origin", err)
	}
	if code, body := invoke("paths", "--field", "owner-scope"); code != 0 || body != home+"\n" {
		t.Fatalf("owner identity: %d %q", code, body)
	}
	if code, body := invoke("paths", "--field", "state"); code != 0 || body != filepath.Join(home, "state")+"\n" {
		t.Fatalf("state path: %d %q", code, body)
	}
	entries, err := os.ReadDir(product)
	if err != nil || len(entries) != 0 {
		t.Fatalf("compiled helper changed product: %v %v", entries, err)
	}
}

func TestHomePathsRequiresExplicitInitAndKeepsProductUntouched(t *testing.T) {
	user, product := homeTestDirectory(t), homeTestDirectory(t)
	t.Setenv("MAESTRO_HOME", filepath.Join(user, "owner with spaces"))
	invoke := func(command string, extra ...string) (int, string) {
		var out, diagnostic bytes.Buffer
		args := append([]string{"home", command, "--product-root", product, "--user-home", user}, extra...)
		code := run(args, strings.NewReader(""), &out, &diagnostic)
		return code, out.String()
	}
	code, body := invoke("paths")
	var result map[string]any
	if json.Unmarshal([]byte(body), &result) != nil || code != 2 || result["mode"] != "initialization_required" || len(result) != 3 {
		t.Fatalf("uninitialized: code=%d body=%s", code, body)
	}
	if code, body = invoke("paths", "--field", "brain"); code != 2 || body != "" {
		t.Fatalf("unavailable scalar %d %q", code, body)
	}
	if code, body = invoke("init"); code != 0 {
		t.Fatalf("init: %d %s", code, body)
	}
	code, body = invoke("paths")
	if json.Unmarshal([]byte(body), &result) != nil || code != 0 || result["mode"] != "external" || result["owner_scope_root"] != filepath.Join(user, "owner with spaces") {
		t.Fatalf("admitted: %d %s", code, body)
	}
	if code, body = invoke("paths", "--field", "brain"); code != 0 || body != filepath.Join(user, "owner with spaces", "brain")+"\n" {
		t.Fatalf("scalar %d %q", code, body)
	}
	entries, err := os.ReadDir(product)
	if err != nil || len(entries) != 0 {
		t.Fatalf("product changed: %v %v", entries, err)
	}
}

func homeTestDirectory(t *testing.T) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func TestHomeInspectReportsUnconfirmedHomeWithoutCreatingIt(t *testing.T) {
	user := homeTestDirectory(t)
	product := homeTestDirectory(t)
	home := filepath.Join(user, "Maestro state")
	t.Setenv("HOME", user)
	t.Setenv("USERPROFILE", user)
	t.Setenv("MAESTRO_HOME", home)
	var out, diagnostic bytes.Buffer
	code := run([]string{"home", "inspect", "--product-root", product}, bytes.NewReader(nil), &out, &diagnostic)
	var result struct {
		Home       string `json:"home"`
		SelectedBy string `json:"selected_by"`
		Origin     string `json:"origin"`
		Writable   bool   `json:"writable"`
	}
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("code=%d diagnostic=%s output=%s: %v", code, &diagnostic, &out, err)
	}
	if code != 0 || result.Home != home || result.SelectedBy != "env" || result.Origin != "fresh-unconfirmed" || result.Writable {
		t.Fatalf("code=%d result=%+v diagnostic=%s", code, result, &diagnostic)
	}
	if _, err := os.Stat(home); !os.IsNotExist(err) {
		t.Fatalf("inspection created HOME or hid an error: %v", err)
	}
	for _, root := range []string{user, product} {
		entries, err := os.ReadDir(root)
		if err != nil || len(entries) != 0 {
			t.Fatalf("inspection changed %s: %v %v", root, entries, err)
		}
	}
}

func TestHomeInspectRejectsInvalidSelectionAndArguments(t *testing.T) {
	t.Setenv("MAESTRO_HOME", "relative-state")
	product := homeTestDirectory(t)
	for _, args := range [][]string{
		{"home", "inspect", "--product-root", product},
		{"home", "inspect"},
		{"home", "activate", "--product-root", product},
		{"home", "inspect", "--product-root", product, "unexpected"},
	} {
		var out, diagnostic bytes.Buffer
		if code := run(args, bytes.NewReader(nil), &out, &diagnostic); code != 2 || diagnostic.Len() == 0 || out.Len() != 0 {
			t.Fatalf("args=%v code=%d output=%s diagnostic=%s", args, code, &out, &diagnostic)
		}
	}
}

func TestHomeInspectBoundsDiagnostics(t *testing.T) {
	user := homeTestDirectory(t)
	t.Setenv("HOME", user)
	t.Setenv("USERPROFILE", user)
	t.Setenv("MAESTRO_HOME", filepath.Join(user, ".maestro"))
	oversized := strings.Repeat("sensitive-path-", 4096)
	for _, args := range [][]string{
		{"home", "inspect", "--product-root", filepath.Join(user, oversized)},
		{"home", "inspect", "--" + oversized},
	} {
		var out, diagnostic bytes.Buffer
		code := run(args, bytes.NewReader(nil), &out, &diagnostic)
		if code != 2 || out.Len() != 0 || diagnostic.Len() == 0 || diagnostic.Len() > 1024 {
			t.Fatalf("unbounded error: code=%d stdout=%d stderr=%d", code, out.Len(), diagnostic.Len())
		}
	}
}

// File.Stat captures the held object's Windows ID eagerly; pathname Stat may
// defer that lookup until SameFile. Close before init so the oracle adds no lock.
func homeFileIdentitySnapshot(t *testing.T, path string) os.FileInfo {
	t.Helper()
	file, err := scheduler.OpenRegularFile(path)
	if err != nil {
		t.Fatal(err)
	}
	info, statErr := file.Stat()
	if err := errors.Join(statErr, file.Close()); err != nil {
		t.Fatal(err)
	}
	return info
}
