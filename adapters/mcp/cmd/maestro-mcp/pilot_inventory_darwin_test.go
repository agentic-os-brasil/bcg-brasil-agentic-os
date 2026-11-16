package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestMacOSPilotInventoryRejectsHashProducerFailure(t *testing.T) {
	source := filepath.Join("..", "..", "..", "..", "dev", "pilot")
	setup, err := os.ReadFile(filepath.Join(source, "setup-macos.command"))
	if err != nil {
		t.Fatal(err)
	}
	launcher, err := os.ReadFile(filepath.Join(source, "test-macos.command"))
	if err != nil {
		t.Fatal(err)
	}
	for _, caller := range []string{"setup", "launcher"} {
		unavailable := false
		t.Run(caller, func(t *testing.T) {
			home, err := os.UserHomeDir()
			if err != nil {
				t.Fatal(err)
			}
			for _, directory := range []string{filepath.Join(home, "Developer"), filepath.Join(home, "Developer", "tmp")} {
				if err := os.Mkdir(directory, 0700); err != nil && !os.IsExist(err) {
					t.Fatal(err)
				}
				if info, err := os.Lstat(directory); err != nil || !info.IsDir() {
					t.Fatal("fixture parent must be a physical directory", err)
				}
			}
			developer, err := filepath.EvalSymlinks(filepath.Join(home, "Developer"))
			if err != nil {
				t.Fatal(err)
			}
			temporary, err := filepath.EvalSymlinks(filepath.Join(developer, "tmp"))
			if err != nil || !strings.HasPrefix(temporary, developer+string(filepath.Separator)) {
				t.Fatal("fixture temporary directory must remain physically under Developer", err)
			}
			// Keep the actual preparer's per-ancestor ownership checks small.
			base, err := os.MkdirTemp(temporary, "pilot-inventory-")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.RemoveAll(base) })
			root, kit := filepath.Join(base, "prepared space"), filepath.Join(base, "kit")
			run := func(cwd, path string, args ...string) ([]byte, error) {
				ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
				defer cancel()
				cmd := exec.CommandContext(ctx, path, args...)
				cmd.Dir = cwd
				cmd.WaitDelay = time.Second
				for _, entry := range os.Environ() {
					if !strings.HasPrefix(entry, "MAESTRO_HOME=") {
						cmd.Env = append(cmd.Env, entry)
					}
				}
				cmd.Env = append(cmd.Env, "PATH="+filepath.Join(base, "tools")+":"+os.Getenv("PATH"), "PILOT_INVENTORY_WITNESS="+filepath.Join(root, "hash-output"))
				cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
				cmd.Cancel = func() error {
					if cmd.Process == nil {
						return os.ErrProcessDone
					}
					return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
				}
				defer func() {
					if cmd.Process != nil {
						_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
					}
				}()
				started := time.Now()
				out, err := cmd.CombinedOutput()
				t.Logf("%s target completed after %s: %v\n%s", caller, time.Since(started).Round(time.Millisecond), err, strings.ReplaceAll(string(out), base, "<fixture>"))
				if ctx.Err() != nil {
					unavailable = true
					t.Fatalf("bounded %s target: %v", caller, ctx.Err())
				}
				return out, err
			}
			binary := []byte("#!/bin/bash\nprintf reached > \"${0%/*}/../checker-reached\"\nexit 17\n")
			prepare := filepath.Join(kit, "Preparar Maestro.command")
			image := "maestro-mcp-darwin-" + runtime.GOARCH
			writePilotFixture(t, prepare, setup)
			writePilotFixture(t, filepath.Join(kit, "bin", image), binary)
			kitInventory := []byte(fmt.Sprintf("%x  ./Preparar Maestro.command\n%x  ./bin/%s\n", sha256.Sum256(setup), sha256.Sum256(binary), image))
			kitID := fmt.Sprintf("%x\n", sha256.Sum256(kitInventory))
			writePilotFixture(t, filepath.Join(kit, "SHA256SUMS"), kitInventory)
			writePilotFixture(t, filepath.Join(kit, "KIT-ID.txt"), []byte(kitID))
			writePilotFixture(t, filepath.Join(root, ".maestro-pilot-owned"), []byte(kitID))
			writePilotFixture(t, filepath.Join(root, ".maestro-pilot-complete"), []byte(kitID))
			checker := filepath.Join(root, "product", "maestro-mcp")
			testScript := filepath.Join(root, "launchers", "Testar Maestro.command")
			writePilotFixture(t, checker, binary)
			writePilotFixture(t, testScript, launcher)
			managed := []byte("managed bytes\n")
			writePilotFixture(t, filepath.Join(root, "exports", "-literal file with spaces.txt"), managed)
			for _, path := range []string{checker, testScript} {
				if err := os.Chmod(path, 0700); err != nil {
					t.Fatal(err)
				}
			}
			writePilotFixture(t, filepath.Join(root, "pilot-binary.sha256"), []byte(fmt.Sprintf("%x  product/maestro-mcp\n", sha256.Sum256(binary))))
			// BSD find -s sorts the initial roots as well as their contents.
			expected := []byte(fmt.Sprintf("%x  exports/-literal file with spaces.txt\n%x  launchers/Testar Maestro.command\n%x  product/maestro-mcp\n", sha256.Sum256(managed), sha256.Sum256(launcher), sha256.Sum256(binary)))
			writePilotFixture(t, filepath.Join(root, "managed-files.sha256"), expected)
			review := fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("ordinary-engineering-rehearsal-v1\n%s\ndarwin-%s\n%s\n", strings.TrimSpace(kitID), runtime.GOARCH, root))))
			shim := filepath.Join(base, "tools", "shasum")
			// Inject only the producer failure; keep bootstrap hashing real.
			shimBody := fmt.Sprintf(`#!/bin/bash
case "${3:-}" in
  product/maestro-mcp|"exports/-literal file with spaces.txt"|"launchers/Testar Maestro.command")
    shift 2
    for path do
      case "$path" in
        product/maestro-mcp) record='%x  product/maestro-mcp';;
        "exports/-literal file with spaces.txt") record='%x  exports/-literal file with spaces.txt';;
        "launchers/Testar Maestro.command") record='%x  launchers/Testar Maestro.command';;
        *) exit 2;;
      esac
      printf '%%s\n' "$record"
      printf '%%s\n' "$record" >> "$PILOT_INVENTORY_WITNESS"
    done
    exit 1;;
esac
exec /usr/bin/shasum "$@"
`, sha256.Sum256(binary), sha256.Sum256(managed), sha256.Sum256(launcher))
			writePilotFixture(t, shim, []byte(shimBody))
			if err := os.Chmod(shim, 0700); err != nil {
				t.Fatal(err)
			}
			if caller == "setup" {
				_, err = run(root, "/bin/bash", prepare, root, "--confirm", review)
			} else {
				_, err = run(root, "/bin/bash", testScript, "--check-only")
			}
			witness, readErr := os.ReadFile(filepath.Join(root, "hash-output"))
			inventoryCount := len(witness) / len(expected)
			if readErr != nil || inventoryCount == 0 || !bytes.Equal(witness, bytes.Repeat(expected, inventoryCount)) {
				t.Fatalf("producer did not emit the complete original inventory: %v\ngot: %q\nwant: %q", readErr, witness, expected)
			}
			if err == nil {
				t.Error("failed hash producer accepted")
			}
			_, statErr := os.Stat(filepath.Join(root, "checker-reached"))
			t.Logf("inventory_output_complete=true injected_producer_exit=1 caller_exit=%v checker_called=%t", err, !os.IsNotExist(statErr))
			if !os.IsNotExist(statErr) {
				t.Error("producer exited 1 after complete output, but verification called the checker")
			}
			retained, readErr := os.ReadFile(filepath.Join(root, "managed-files.sha256"))
			if readErr != nil || !bytes.Equal(retained, expected) {
				t.Fatal("verification changed original inventory")
			}
		})
		if unavailable {
			return
		}
	}
}

func TestMacOSPilotCodePromptHandoff(t *testing.T) {
	source := filepath.Join("..", "..", "..", "..", "dev", "pilot")
	base := t.TempDir()
	root := filepath.Join(base, "prepared space ' literal")
	tools := filepath.Join(base, "tools")
	witness := filepath.Join(base, "argv")
	prompt := "Use exact \"quoted\" text, $HOME and $(touch NEVER_CREATED).\nSecond line."
	report, _ := json.Marshal(map[string]any{"schema_version": 1, "status": "ready", "markdown": "synthetic report", "code_prompt": prompt + " complete", "desktop_prompt": "desktop", "code_ready_prompt": prompt + " connection", "desktop_ready_prompt": "desktop connection"})
	checker := []byte(`#!/bin/bash
case "$1" in
  pilot-check) cat "$PILOT_REPORT";;
  inspect) printf '{"diagnostic":"ready","packet_mode":"mcp_lifecycle","packet_server_sha256":"%s"}\n' "$PILOT_SERVER_SHA";;
  native-review) printf '%s\0' "$@" > "$PILOT_REVIEW_ARGS"; printf '{"plan":{"state":"reviewable","complete_plan_marker":"FULL_CANONICAL_PLAN"},"review_digest":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}\n';;
  native-qualify-and-build) printf '%s\0' "$@" > "$PILOT_QUALIFY_ARGS"; printf '{"state":"unavailable","reason":"synthetic_fixture_only"}\n';;
  *) exit 21;;
esac
`)
	for path, body := range map[string][]byte{
		filepath.Join(root, "product", "maestro-mcp"):                    checker,
		filepath.Join(root, "exports", "code", "scripts", "maestro-mcp"): checker,
		filepath.Join(root, ".maestro-pilot-owned"):                      []byte("synthetic"),
		filepath.Join(root, "pilot-binary.sha256"):                       []byte(fmt.Sprintf("%x  product/maestro-mcp\n", sha256.Sum256(checker))),
		filepath.Join(base, "report.json"):                               report,
		filepath.Join(tools, "claude"):                                   []byte("#!/bin/bash\nprintf '%s\\0' \"$PWD\" \"$CLAUDE_CONFIG_DIR\" \"$@\" > \"$PILOT_ARGS\"\n"),
		filepath.Join(tools, "open"):                                     []byte("#!/bin/bash\nexit 0\n"),
		filepath.Join(tools, "pbcopy"):                                   []byte("#!/bin/bash\necho 'Code must not use Clipboard' >&2\nexit 19\n"),
	} {
		writePilotFixture(t, path, body)
		if strings.Contains(string(body), "#!/bin/bash") {
			if err := os.Chmod(path, 0700); err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, name := range []string{"workspace", "owner", "client/code"} {
		if err := os.MkdirAll(filepath.Join(root, name), 0700); err != nil {
			t.Fatal(err)
		}
	}
	for original, name := range map[string]string{"open-code-macos.command": "Abrir Code.command", "test-macos.command": "Testar Maestro.command"} {
		body, err := os.ReadFile(filepath.Join(source, original))
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(root, "launchers", name)
		writePilotFixture(t, path, body)
		if err := os.Chmod(path, 0700); err != nil {
			t.Fatal(err)
		}
	}
	run := func(args []string, input string) ([]byte, error) {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, args[0], args[1:]...)
		cmd.Env = append(os.Environ(), "PATH="+tools+":"+os.Getenv("PATH"), "PILOT_REPORT="+filepath.Join(base, "report.json"), "PILOT_ARGS="+witness, "PILOT_REVIEW_ARGS="+filepath.Join(base, "review-args"), "PILOT_QUALIFY_ARGS="+filepath.Join(base, "qualification-args"), fmt.Sprintf("PILOT_SERVER_SHA=%x", sha256.Sum256(checker)))
		if input == "" {
			return cmd.CombinedOutput()
		}
		stdin, err := cmd.StdinPipe()
		if err != nil {
			t.Fatal(err)
		}
		defer stdin.Close()
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			t.Fatal(err)
		}
		cmd.Stderr = cmd.Stdout
		if err := cmd.Start(); err != nil {
			return nil, err
		}
		reader := bufio.NewReader(stdout)
		var output bytes.Buffer
		for !strings.Contains(output.String(), "Escolha:") {
			part, err := reader.ReadBytes(':')
			output.Write(part)
			if err != nil {
				_ = cmd.Wait()
				return output.Bytes(), err
			}
		}
		if _, err := io.WriteString(stdin, input); err != nil {
			t.Fatal(err)
		}
		rest, err := io.ReadAll(reader)
		output.Write(rest)
		waitErr := cmd.Wait()
		if err != nil {
			return output.Bytes(), err
		}
		return output.Bytes(), waitErr
	}
	assertArgs := func(want ...string) {
		t.Helper()
		body, err := os.ReadFile(witness)
		if err != nil {
			t.Fatal(err)
		}
		got := strings.Split(strings.TrimSuffix(string(body), "\x00"), "\x00")
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("literal launcher argv differs: got %q want %q", got, want)
		}
		if err := os.Remove(witness); err != nil {
			t.Fatal(err)
		}
	}
	launcher := filepath.Join(root, "launchers", "Abrir Code.command")
	prefix := []string{filepath.Join(root, "workspace"), filepath.Join(root, "client", "code"), "--plugin-dir", filepath.Join(root, "exports", "code")}
	if out, err := run([]string{"/bin/bash", launcher}, ""); err != nil {
		t.Fatalf("ordinary open: %s %v", out, err)
	}
	assertArgs(prefix...)
	if out, err := run([]string{"/bin/bash", launcher, prompt}, ""); err != nil {
		t.Fatalf("prompt open: %s %v", out, err)
	}
	assertArgs(append(append([]string{}, prefix...), "--", prompt)...)
	for _, invalid := range [][]string{{""}, {"one", "two"}} {
		if out, err := run(append([]string{"/bin/bash", launcher}, invalid...), ""); err == nil {
			t.Fatalf("invalid launcher arguments accepted: %s", out)
		}
	}
	menu := filepath.Join(root, "launchers", "Testar Maestro.command")
	if out, err := run([]string{"/bin/bash", menu, "--check-only"}, ""); err != nil {
		t.Fatalf("check-only: %s %v", out, err)
	}
	if _, err := os.Stat(witness); !os.IsNotExist(err) {
		t.Fatal("check-only or invalid arguments opened Code")
	}
	for _, choice := range []string{"1", "3"} {
		transcript := filepath.Join(base, "menu-"+choice)
		if out, err := run([]string{"/usr/bin/script", "-q", transcript, "/bin/bash", menu}, choice+"\n"); err != nil {
			t.Fatalf("menu %s: %s %v", choice, out, err)
		}
		suffix := " complete"
		if choice == "3" {
			suffix = " connection"
		}
		assertArgs(append(append([]string{}, prefix...), "--", prompt+suffix)...)
	}
	for _, file := range []string{"review-args", "qualification-args"} {
		if _, err := os.Stat(filepath.Join(base, file)); !os.IsNotExist(err) {
			t.Fatal("ordinary menu started native qualification", file)
		}
	}
	for _, test := range []struct {
		name, input         string
		reviewed, qualified bool
	}{
		{"budget cancelled", "5\n\n", false, false},
		{"plan cancelled", "5\n0.25\nCANCELAR\n", true, false},
		{"explicitly confirmed unavailable", "5\n0.25\nQUALIFICAR\n", true, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			for _, file := range []string{"review-args", "qualification-args"} {
				_ = os.Remove(filepath.Join(base, file))
			}
			out, err := run([]string{"/usr/bin/script", "-q", filepath.Join(base, "qualification-menu"), "/bin/bash", menu}, test.input)
			if (err != nil) != test.qualified {
				t.Fatalf("unexpected qualification menu outcome: %v %s", err, out)
			}
			for file, want := range map[string]bool{"review-args": test.reviewed, "qualification-args": test.qualified} {
				body, readErr := os.ReadFile(filepath.Join(base, file))
				if (readErr == nil) != want {
					t.Fatalf("%s called=%t want=%t: %v %s", file, readErr == nil, want, readErr, out)
				}
				if file == "qualification-args" && want && !bytes.Contains(body, []byte("--review-digest\x00"+strings.Repeat("a", 64)+"\x00--confirm\x00")) {
					t.Fatal("qualification omitted exact reviewed confirmation")
				}
			}
			if test.reviewed && !bytes.Contains(out, []byte("FULL_CANONICAL_PLAN")) {
				t.Fatal("complete returned plan was not displayed")
			}
			if _, err := os.Stat(witness); !os.IsNotExist(err) {
				t.Fatal("cancelled or unavailable qualification opened Code")
			}
		})
	}
	if _, err := os.Stat(filepath.Join(root, "workspace", "NEVER_CREATED")); !os.IsNotExist(err) {
		t.Fatal("prompt was interpreted by a shell")
	}
}

func TestMacOSPilotLaunchesQualifiedCodeWithSessionSelection(t *testing.T) {
	source := filepath.Join("..", "..", "..", "..", "dev", "pilot")
	base := t.TempDir()
	root := filepath.Join(base, "prepared space ' quote")
	tools := filepath.Join(base, "tools")
	witness := filepath.Join(base, "argv")
	checker := []byte("#!/bin/bash\n[[ $1 == inspect ]] || exit 21\n[[ ${PILOT_REFUSE:-} != yes ]] || exit 19\nmode=mcp_lifecycle\nsha=${PILOT_BASE_SHA:-$PILOT_SERVER_SHA}\nfor arg do if [[ $arg == */code-native-qualified ]]; then mode=${PILOT_NATIVE_MODE:-native}; sha=$PILOT_SERVER_SHA; fi; done\nprintf '{\"diagnostic\":\"ready\",\"packet_mode\":\"%s\",\"packet_server_sha256\":\"%s\"}\\n' \"$mode\" \"$sha\"\n")
	for path, body := range map[string][]byte{
		filepath.Join(root, "product", "maestro-mcp"):                                     checker,
		filepath.Join(root, "exports", "code", "scripts", "maestro-mcp"):                  checker,
		filepath.Join(root, "exports", "code-native-qualified", "scripts", "maestro-mcp"): checker,
		filepath.Join(tools, "claude"):                                                    []byte("#!/bin/bash\nprintf '%s\\0' \"$PWD\" \"$CLAUDE_CONFIG_DIR\" \"$@\" > \"$PILOT_ARGS\"\n"),
	} {
		writePilotFixture(t, path, body)
		if err := os.Chmod(path, 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(root, "workspace"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "client", "code"), 0700); err != nil {
		t.Fatal(err)
	}
	writePilotFixture(t, filepath.Join(root, "pilot-binary.sha256"), []byte(fmt.Sprintf("%x  product/maestro-mcp\n", sha256.Sum256(checker))))
	body, err := os.ReadFile(filepath.Join(source, "open-code-macos.command"))
	if err != nil {
		t.Fatal(err)
	}
	launcher := filepath.Join(root, "launchers", "Abrir Code.command")
	writePilotFixture(t, launcher, body)
	profile := filepath.Join(root, "client", "code", "settings.json")
	writePilotFixture(t, profile, []byte("{\"existing\":true}\n"))
	run := func(extra ...string) ([]byte, error) {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "/bin/bash", launcher)
		cmd.Env = append(os.Environ(), "PATH="+tools+":"+os.Getenv("PATH"), "PILOT_ARGS="+witness, fmt.Sprintf("PILOT_SERVER_SHA=%x", sha256.Sum256(checker)))
		cmd.Env = append(cmd.Env, extra...)
		return cmd.CombinedOutput()
	}
	if out, err := run(); err != nil {
		t.Fatalf("Code opener: %v %s", err, out)
	}
	got, err := os.ReadFile(witness)
	if err != nil {
		t.Fatal(err)
	}
	args := strings.Split(strings.TrimSuffix(string(got), "\x00"), "\x00")
	if len(args) != 6 || args[2] != "--plugin-dir" || args[3] != filepath.Join(root, "exports", "code-native-qualified") || args[4] != "--settings" {
		t.Fatalf("colleague opener did not choose the qualified packet and session selection: %q", args)
	}
	selection, err := os.ReadFile(args[5])
	var value struct {
		Plugins map[string]struct {
			Options map[string]string `json:"options"`
		} `json:"pluginConfigs"`
	}
	if err != nil || json.Unmarshal(selection, &value) != nil {
		t.Fatal("session selection is not a regular JSON file", err)
	}
	options := value.Plugins["maestro-product@inline"].Options
	if len(value.Plugins) != 1 || len(options) != 2 || options["user_home"] != filepath.Join(root, "owner") || options["workspace"] != filepath.Join(root, "workspace") {
		t.Fatal("session selection changed the exact prepared scope")
	}
	for _, test := range []struct {
		name     string
		env      []string
		fallback bool
	}{
		{"mcp_only fallback", []string{"PILOT_NATIVE_MODE=mcp_only"}, true},
		{"engineering fixture refused", []string{"PILOT_NATIVE_MODE=native_engineering_fixture"}, false},
		{"other executable refused", []string{"PILOT_SERVER_SHA=" + strings.Repeat("0", 64)}, false},
		{"canonical inspection refused", []string{"PILOT_REFUSE=yes"}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			_ = os.Remove(witness)
			out, err := run(test.env...)
			got, readErr := os.ReadFile(witness)
			if test.fallback {
				args := strings.Split(strings.TrimSuffix(string(got), "\x00"), "\x00")
				if err != nil || readErr != nil || len(args) != 4 || args[3] != filepath.Join(root, "exports", "code") {
					t.Fatalf("fallback changed: %v %s %q", err, out, args)
				}
			} else if err == nil || !os.IsNotExist(readErr) {
				t.Fatalf("refused packet launched Code: %v %s", err, out)
			}
		})
	}
	for _, test := range []struct {
		name     string
		fallback bool
	}{
		{"foreign baseline refused", false}, {"foreign fallback refused", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			native := filepath.Join(root, "exports", "code-native-qualified")
			if !test.fallback {
				if err := os.Rename(native, native+".retained"); err != nil {
					t.Fatal(err)
				}
				defer func() {
					if err := os.Rename(native+".retained", native); err != nil {
						t.Fatal(err)
					}
				}()
			}
			_ = os.Remove(witness)
			out, err := run("PILOT_BASE_SHA="+strings.Repeat("0", 64), "PILOT_NATIVE_MODE=mcp_only")
			if _, readErr := os.Stat(witness); err == nil || !os.IsNotExist(readErr) {
				t.Fatalf("foreign selected packet launched Code: %v %s", err, out)
			}
		})
	}
	for _, name := range []string{"client", "client/code"} {
		for _, mode := range []string{"native", "mcp_only"} {
			t.Run("profile alias refused/"+name+"/"+mode, func(t *testing.T) {
				alias := filepath.Join(root, filepath.FromSlash(name))
				retained := filepath.Join(base, "retained-profile")
				if err := os.Rename(alias, retained); err != nil {
					t.Fatal(err)
				}
				foreign := t.TempDir()
				target := foreign
				if name == "client" {
					target = filepath.Join(foreign, "code")
				}
				writePilotFixture(t, filepath.Join(target, "settings.json"), []byte("preserve foreign settings"))
				if err := os.Symlink(foreign, alias); err != nil {
					t.Fatal(err)
				}
				defer func() {
					if err := os.Remove(alias); err != nil {
						t.Error(err)
					}
					if err := os.Rename(retained, alias); err != nil {
						t.Error(err)
					}
				}()
				_ = os.Remove(witness)
				out, err := run("PILOT_NATIVE_MODE=" + mode)
				if _, readErr := os.Stat(witness); err == nil || !os.IsNotExist(readErr) {
					t.Errorf("aliased profile launched Code: %v %s", err, out)
				}
				entries, err := os.ReadDir(target)
				if err != nil || len(entries) != 1 || entries[0].Name() != "settings.json" {
					t.Errorf("launcher wrote through profile alias: %v %v", entries, err)
				}
				if body, err := os.ReadFile(filepath.Join(target, "settings.json")); err != nil || string(body) != "preserve foreign settings" {
					t.Error("foreign settings changed", err)
				}
			})
		}
	}
	if got, err := os.ReadFile(profile); err != nil || string(got) != "{\"existing\":true}\n" {
		t.Fatal("launcher changed existing profile", err)
	}
}
