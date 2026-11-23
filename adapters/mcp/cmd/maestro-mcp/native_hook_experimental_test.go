//go:build maestro_experimental_installed_seed

package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	product "github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/datahome"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installer"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installtx"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/processidentity"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspace"
)

func TestInstalledNativeHookValidatesBindingOnce(t *testing.T) {
	args := nativeHookSelectionArgs(t)
	args[1] = "PreToolUse"
	root, err := filepath.EvalSymlinks(args[7])
	if err != nil {
		t.Fatal(err)
	}
	args[7], args[9], args[11] = filepath.Join(root, "product"), filepath.Join(root, "owner"), filepath.Join(root, "workspace")
	args[19] = args[7]
	s, err := parseInstalledNativeHook(args)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{s.ProductRoot, s.UserHome} {
		if err := os.Mkdir(path, 0700); err != nil {
			t.Fatal(err)
		}
	}
	reads := 0
	o := product.Options{ProductRoot: s.ProductRoot, UserHome: s.UserHome, WorkspacePath: s.Workspace,
		LookupEnv: func(key string) (string, bool) {
			if key == "MAESTRO_HOME" {
				reads++
			}
			return "", false
		}}
	h, err := datahome.InitializeFresh(datahome.Options{ProductRoot: o.ProductRoot, UserHome: o.UserHome, LookupEnv: o.LookupEnv})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := workspace.Initialize(workspace.Options{WorkspacePath: o.WorkspacePath, DataRoot: filepath.Join(h.Home, "state")}); err != nil {
		t.Fatal(err)
	}
	reads = 0
	if _, err := product.Open(o); err != nil {
		t.Fatal(err)
	}
	oneValidationReads := reads
	if oneValidationReads == 0 {
		t.Fatal("binding observation was not exercised")
	}
	// The existing environment resolver observes this operation only; no
	// replacement validator, persistent counter or shipping option is added.
	reads = 0
	admissions := 0
	var output bytes.Buffer
	input := fmt.Sprintf(`{"session_id":"one-shot","prompt_id":"turn-1","cwd":%q,"hook_event_name":"PreToolUse","tool_name":"Read","tool_use_id":"read-1","tool_input":{"file_path":%q}}`, s.Workspace, filepath.Join(s.Workspace, "synthetic.md"))
	err = handleInstalledNativeHook(context.Background(), args, strings.NewReader(input), &output,
		func(_ context.Context, selected installedNativeHookSelection, input io.Reader, output io.Writer) error {
			admissions++
			return product.HandleClaudeHook(o, selected.Event, input, output)
		})
	if err != nil || strings.TrimSpace(output.String()) != "{}" || admissions != 1 {
		t.Fatalf("real hook entry failed: err=%v output=%q admissions=%d", err, output.String(), admissions)
	}
	if reads != oneValidationReads {
		t.Fatalf("hook repeated full binding validation: resolver observations=%d; one full validation=%d", reads, oneValidationReads)
	}
	t.Logf("resolver observations=%d; one full validation=%d; admissions=%d", reads, oneValidationReads, admissions)
}

func nativeHookSelectionArgs(t *testing.T) []string {
	t.Helper()
	root := t.TempDir()
	return []string{"--event", "UserPromptSubmit", "--binding-sha256", strings.Repeat("a", 64), "--carrier-id", strings.Repeat("b", 32), "--product-root", root, "--user-home", filepath.Join(root, "owner"), "--workspace", filepath.Join(root, "workspace"), "--installed-manifest", strings.Repeat("c", 64), "--installed-client-inputs", strings.Repeat("d", 64), "--experimental-seed-sha256", strings.Repeat("e", 64), "--experimental-installed-root", root}
}

func TestInstalledNativeHookClosedSelection(t *testing.T) {
	base := nativeHookSelectionArgs(t)
	for _, event := range []string{"SessionStart", "UserPromptSubmit", "PreToolUse", "PostToolUse", "SubagentStart", "SubagentStop", "Stop"} {
		args := append([]string(nil), base...)
		args[1] = event
		if _, err := parseInstalledNativeHook(args); err != nil {
			t.Fatalf("canonical %s: %v", event, err)
		}
	}
	for _, change := range []string{"missing", "duplicate", "unknown", "equals", "empty", "relative", "alias", "foreign-root", "binding", "manifest", "client-inputs", "admission", "carrier", "event", "control", "oversize"} {
		t.Run(change, func(t *testing.T) {
			args := append([]string(nil), base...)
			switch change {
			case "missing":
				args = args[:len(args)-2]
			case "duplicate":
				args[2] = "--event"
			case "unknown":
				args[0] = "--packet"
			case "equals":
				args[0] = "--event=Stop"
			case "empty":
				args[1] = ""
			case "relative":
				args[7] = "relative"
			case "alias":
				args[7] = args[7] + string(filepath.Separator) + "."
			case "foreign-root":
				args[19] = filepath.Join(args[19], "other")
			case "binding":
				args[3] = "bad"
			case "manifest":
				args[13] = "bad"
			case "client-inputs":
				args[15] = "bad"
			case "admission":
				args[17] = "bad"
			case "carrier":
				args[5] = "bad"
			case "event":
				args[1] = "foreign"
			case "control":
				args[9] += "\n"
			case "oversize":
				args[11] = strings.Repeat("x", 4097)
			}
			called := false
			var output bytes.Buffer
			err := handleInstalledNativeHook(context.Background(), args, strings.NewReader("PRIVATE_PAYLOAD"), &output, func(context.Context, installedNativeHookSelection, io.Reader, io.Writer) error {
				called = true
				return nil
			})
			if err == nil || called || output.Len() != 0 {
				t.Fatalf("invalid selection reached admission/handler: err=%v called=%v output=%s", err, called, output.String())
			}
		})
	}
}

func TestInstalledNativeHookRequiresActualCurrentImageAndPins(t *testing.T) {
	s, err := parseInstalledNativeHook(nativeHookSelectionArgs(t))
	if err != nil {
		t.Fatal(err)
	}
	name := "maestro-mcp"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	path := filepath.Join(s.ProductRoot, "bin", name)
	state := installtx.State{TargetOS: runtime.GOOS, TargetArch: runtime.GOARCH, Binding: installtx.ReleaseBinding{ManifestSHA256: s.ManifestSHA256, ClientInputsSHA256: s.ClientInputsSHA256}}
	state.Server.SHA256 = strings.Repeat("f", 64)
	state.Server.Version = "0.1.57"
	launch := installer.QualifiedLaunch{Executable: path, State: state}
	actual := processidentity.Identity{Path: path, SHA256: state.Server.SHA256}
	if err := validateInstalledNativeHookCurrent(s, launch, actual, "0.1.57"); err != nil {
		t.Fatal(err)
	}
	for _, change := range []string{"manifest", "client-inputs", "root", "image-path", "image-hash", "selected-image", "version", "target"} {
		t.Run(change, func(t *testing.T) {
			selected, current, image, version := s, launch, actual, "0.1.57"
			switch change {
			case "manifest":
				selected.ManifestSHA256 = strings.Repeat("0", 64)
			case "client-inputs":
				selected.ClientInputsSHA256 = strings.Repeat("0", 64)
			case "root":
				selected.ProductRoot = filepath.Join(s.ProductRoot, "foreign")
			case "image-path":
				image.Path = filepath.Join(s.ProductRoot, "scripts", name)
			case "image-hash":
				image.SHA256 = strings.Repeat("0", 64)
			case "selected-image":
				current.Executable = filepath.Join(s.ProductRoot, "foreign", name)
			case "version":
				version = "0.1.58"
			case "target":
				current.State.TargetArch = "foreign"
			}
			if validateInstalledNativeHookCurrent(selected, current, image, version) == nil {
				t.Fatal("changed current image/release admitted")
			}
		})
	}
}

func TestInstalledNativeHookEntryIndependentlyRefusesAbsentAdmission(t *testing.T) {
	args := nativeHookSelectionArgs(t)
	before, err := os.ReadDir(args[7])
	if err != nil {
		t.Fatal(err)
	}
	input := &nativeHookUnreadWitness{Reader: strings.NewReader(fmt.Sprintf(`{"session_id":"no-admission","prompt_id":"turn-1","cwd":%q,"hook_event_name":"UserPromptSubmit"}`, args[11]))}
	var output bytes.Buffer
	if err := runInstalledNativeHook(context.Background(), args, input, &output); err == nil || input.reads != 0 || output.Len() != 0 {
		t.Fatalf("missing signed custody reached event input: err=%v reads=%d output=%s", err, input.reads, output.String())
	}
	after, err := os.ReadDir(args[7])
	if err != nil || len(before) != len(after) {
		t.Fatal("refusal created private state")
	}
}

type nativeHookUnreadWitness struct {
	io.Reader
	reads int
}

func (r *nativeHookUnreadWitness) Read(body []byte) (int, error) {
	r.reads++
	return r.Reader.Read(body)
}

func TestInstalledNativeHookCancellationBeforeAdmissionReadsNoEvent(t *testing.T) {
	args := nativeHookSelectionArgs(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	called := false
	input := &nativeHookUnreadWitness{Reader: strings.NewReader(fmt.Sprintf(`{"session_id":"cancelled","prompt_id":"turn-1","cwd":%q,"hook_event_name":"UserPromptSubmit"}`, args[11]))}
	var output bytes.Buffer
	err := handleInstalledNativeHook(ctx, args, input, &output, func(context.Context, installedNativeHookSelection, io.Reader, io.Writer) error {
		called = true
		return nil
	})
	if !errors.Is(err, context.Canceled) || called || input.reads != 0 || output.Len() != 0 {
		t.Fatalf("cancelled entry reached admission/input: err=%v called=%v reads=%d output=%q", err, called, input.reads, output.String())
	}
}
