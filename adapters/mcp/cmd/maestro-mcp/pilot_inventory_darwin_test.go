package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
