//go:build windows

package packaging

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
	"unsafe"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
	"golang.org/x/sys/windows"
)

type nativeTreeWitness struct {
	Parent, Child               uint32
	ParentCreated, ChildCreated uint64
}

func nativeTestCreation(process windows.Handle) (uint64, error) {
	var creation, exit, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(process, &creation, &exit, &kernel, &user); err != nil {
		return 0, err
	}
	return uint64(creation.HighDateTime)<<32 | uint64(creation.LowDateTime), nil
}

func TestNativeWindowsOwnedProcessTree(t *testing.T) {
	for i, arg := range os.Args {
		if arg == "--maestro-owned-grandchild" {
			time.Sleep(30 * time.Second)
			os.Exit(0)
		}
		if arg == "--maestro-owned-helper" && i+3 < len(os.Args) {
			nativeTreeHelper(os.Args[i+1], os.Args[i+2], os.Args[i+3])
			os.Exit(0)
		}
	}
	if os.Getenv("MAESTRO_WINDOWS_CORE_PROOF") != "1" {
		t.Skip("dedicated ordinary-account Windows driver required")
	}
	admin, err := windows.CreateWellKnownSid(windows.WinBuiltinAdministratorsSid)
	if err != nil {
		t.Fatal(err)
	}
	member, err := windows.Token(0).IsMember(admin)
	if err != nil {
		t.Fatal(err)
	}
	var elevated, returned uint32
	err = windows.GetTokenInformation(windows.GetCurrentProcessToken(), windows.TokenElevation, (*byte)(unsafe.Pointer(&elevated)), 4, &returned)
	if err != nil || returned != 4 || elevated != 0 || member {
		t.Fatal("actual ordinary Windows account required")
	}
	t.Run("private-scratch", func(t *testing.T) {
		private := filepath.Join(t.TempDir(), "private")
		release, err := makeNativePrivateDirectory(private)
		if err != nil {
			t.Fatal(err)
		}
		defer release()
		if err := validateNativeScratchIdentity(private); err != nil {
			t.Fatal("actual current-user scratch refused", err)
		}
		scratch, closeScratch, err := createNativeScratch(private)
		if err != nil {
			t.Fatal(err)
		}
		defer closeScratch()
		if filepath.Dir(scratch) != private {
			t.Fatal("scratch escaped its selected root")
		}
		if err := scheduler.WriteNewPrivateFile(filepath.Join(scratch, "preserved"), []byte("original")); err != nil {
			t.Fatal(err)
		}
		if _, err := makeNativePrivateDirectory(scratch); err == nil {
			t.Fatal("exclusive native directory overwrote existing state")
		}
		if err := os.Rename(scratch, scratch+"-moved"); err == nil {
			t.Fatal("native scratch namespace was not retained")
		}
		if body, err := scheduler.ReadPrivateFile(filepath.Join(scratch, "preserved"), 128); err != nil || string(body) != "original" {
			t.Fatal("private scratch did not preserve its source")
		}
	})
	for _, mode := range []string{"cancel", "parent-exit", "output-limit", "nonzero-exit"} {
		t.Run(mode, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "owned-proof")
			release, err := makeNativePrivateDirectory(root)
			if err != nil {
				t.Fatal(err)
			}
			defer release()
			witnessPath, trigger := filepath.Join(root, "witness.json"), filepath.Join(root, "trigger")
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			type result struct {
				body []byte
				err  error
			}
			done := make(chan result, 1)
			go func() {
				body, err := runNativeCommand(ctx, os.Args[0], []string{"-test.run=^TestNativeWindowsOwnedProcessTree$", "--", "--maestro-owned-helper", mode, witnessPath, trigger}, root)
				done <- result{body, err}
			}()
			var witness nativeTreeWitness
			ready := false
			for deadline := time.Now().Add(8 * time.Second); time.Now().Before(deadline); {
				body, err := scheduler.ReadPrivateFile(witnessPath, 4096)
				if err == nil && json.Unmarshal(body, &witness) == nil {
					ready = true
					break
				}
				select {
				case out := <-done:
					t.Fatalf("owned helper exited before its witness: %v", out.err)
				default:
				}
				time.Sleep(10 * time.Millisecond)
			}
			if !ready {
				t.Fatal("owned helper did not create bounded synthetic witness")
			}
			held := []windows.Handle{}
			for _, identity := range []struct {
				pid     uint32
				created uint64
			}{{witness.Parent, witness.ParentCreated}, {witness.Child, witness.ChildCreated}} {
				h, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.PROCESS_TERMINATE, false, identity.pid)
				if err != nil {
					t.Fatal(err)
				}
				created, err := nativeTestCreation(h)
				if err != nil || created != identity.created {
					windows.CloseHandle(h)
					t.Fatal("synthetic owned process identity changed")
				}
				held = append(held, h)
				t.Cleanup(func() {
					state, _ := windows.WaitForSingleObject(h, 0)
					if state != windows.WAIT_OBJECT_0 {
						_ = windows.TerminateProcess(h, 93)
					}
					_ = windows.CloseHandle(h)
				})
			}
			started := time.Now()
			if mode == "cancel" {
				cancel()
			} else if err := scheduler.WriteNewPrivateFile(trigger, []byte("release")); err != nil {
				t.Fatal(err)
			}
			var out result
			select {
			case out = <-done:
			case <-time.After(6 * time.Second):
				t.Fatal("owned process tree did not stop within its cleanup bound")
			}
			if mode == "parent-exit" && out.err != nil {
				t.Fatalf("ordinary owned parent exit failed: %v", out.err)
			}
			if mode == "cancel" && !errors.Is(out.err, context.Canceled) {
				t.Fatalf("owned cancellation lost its cause: %v", out.err)
			}
			if (mode == "nonzero-exit" || mode == "output-limit") && out.err == nil {
				t.Fatal("owned failure was reported as success")
			}
			if len(out.body) > 2<<20 || time.Since(started) > 6*time.Second {
				t.Fatal("owned output or cleanup exceeded its bound")
			}
			for _, h := range held {
				if state, err := windows.WaitForSingleObject(h, 0); err != nil || state != windows.WAIT_OBJECT_0 {
					t.Fatal("a qualification-owned descendant survived the runner return")
				}
			}
		})
	}
	t.Run("pre-cancel", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		body, err := runNativeCommand(ctx, os.Args[0], nil, "")
		if !errors.Is(err, context.Canceled) || len(body) != 0 {
			t.Fatal("pre-cancelled command started")
		}
	})
	if !t.Failed() {
		t.Log("native_windows_owned_process ordinary_account=true atomic_job_assignment=true private_scratch=true cancellation=true parent_exit=true output_limit=true nonzero_exit=true descendants_closed_before_return=true")
	}
}

func nativeTreeHelper(mode, witnessPath, trigger string) {
	child := exec.Command(os.Args[0], "-test.run=^TestNativeWindowsOwnedProcessTree$", "--", "--maestro-owned-grandchild")
	if err := child.Start(); err != nil {
		os.Exit(94)
	}
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(child.Process.Pid))
	if err != nil {
		os.Exit(95)
	}
	childCreated, err := nativeTestCreation(h)
	windows.CloseHandle(h)
	if err != nil {
		os.Exit(95)
	}
	parentCreated, err := nativeTestCreation(windows.CurrentProcess())
	if err != nil {
		os.Exit(95)
	}
	body, _ := json.Marshal(nativeTreeWitness{Parent: uint32(os.Getpid()), Child: uint32(child.Process.Pid), ParentCreated: parentCreated, ChildCreated: childCreated})
	if err := scheduler.WriteNewPrivateFile(witnessPath, body); err != nil {
		os.Exit(96)
	}
	fmt.Fprintln(os.Stdout, "synthetic owned tree ready")
	for deadline := time.Now().Add(20 * time.Second); time.Now().Before(deadline); {
		if _, err := os.Stat(trigger); err == nil {
			switch mode {
			case "parent-exit":
				return
			case "nonzero-exit":
				os.Exit(7)
			case "output-limit":
				_, _ = os.Stdout.Write(bytes.Repeat([]byte("x"), (2<<20)+4096))
				time.Sleep(20 * time.Second)
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	os.Exit(97)
}
