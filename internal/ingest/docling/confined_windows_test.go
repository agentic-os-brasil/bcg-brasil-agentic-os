//go:build windows

package docling

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/winprivate"
	"golang.org/x/sys/windows"
)

func nativeWindowsFixture(t *testing.T) (string, string) {
	t.Helper()
	if windows.GetCurrentProcessToken().IsElevated() {
		if os.Getenv("MAESTRO_WINDOWS_DOCLING_QUALIFY") == "1" {
			t.Fatal("native qualification requires an ordinary non-elevated Windows user")
		}
		t.Skip("ordinary-user native Windows proof is required")
	}
	root := filepath.Join(t.TempDir(), "private")
	held, err := winprivate.Fresh(root)
	if err != nil {
		t.Fatal(err)
	}
	held.Close()
	work := filepath.Join(root, "execution")
	for _, directory := range []string{work, filepath.Join(work, "pack"), filepath.Join(work, "home"), filepath.Join(work, "cache"), filepath.Join(work, "models"), filepath.Join(work, "tmp"), filepath.Join(work, "config"), filepath.Join(work, "data")} {
		if err := winprivate.MkdirAll(directory); err != nil {
			t.Fatal(err)
		}
	}
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(self)
	if err != nil {
		t.Fatal(err)
	}
	child := filepath.Join(work, "pack", "child.exe")
	if err = os.WriteFile(child, body, 0444); err != nil {
		t.Fatal(err)
	}
	return work, child
}

func TestWindowsNativeConfinement(t *testing.T) {
	work, child := nativeWindowsFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	var stdout, stderr bytes.Buffer
	err := runWindowsConfined(ctx, work, child, []string{"-test.run=^TestWindowsConfinementChild$", "-test.v"}, []byte("probe"), &stdout, &stderr)
	if err != nil {
		t.Fatalf("native confinement failed: %v stderr=%s stdout=%s", err, stderr.String(), stdout.String())
	}
	if !strings.Contains(stdout.String(), "NATIVE_WINDOWS_CONFINEMENT_PASSED") {
		t.Fatalf("real child did not finish controls: %s", stdout.String())
	}
	// Classification and conversion require a second fresh profile over the same staging.
	stdout.Reset()
	stderr.Reset()
	if err = runWindowsConfined(ctx, work, child, []string{"-test.run=^TestWindowsConfinementChild$"}, []byte("probe"), &stdout, &stderr); err != nil {
		t.Fatalf("repeated confined phase: %v stderr=%s", err, stderr.String())
	}
}

func TestWindowsNativeConfinementDeadline(t *testing.T) {
	work, child := nativeWindowsFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stdout := &nativeChildReadyOutput{ready: make(chan struct{})}
	var stderr bytes.Buffer
	result := make(chan error, 1)
	go func() {
		result <- runWindowsConfined(ctx, work, child, []string{"-test.run=^TestWindowsConfinementSleepingChild$"}, nil, stdout, &stderr)
	}()
	select {
	case <-stdout.ready:
	case err := <-result:
		t.Fatalf("native child failed before readiness: %v stderr=%s", err, stderr.String())
	case <-time.After(20 * time.Second):
		cancel()
		<-result
		t.Fatal("actual confined child never reached its ready marker")
	}
	process, err := windows.OpenProcess(windows.SYNCHRONIZE, false, stdout.pid)
	if err != nil {
		cancel()
		<-result
		t.Fatalf("hold actual ready child for termination proof: %v", err)
	}
	defer windows.CloseHandle(process)
	if state, err := windows.WaitForSingleObject(process, 0); err != nil || state != uint32(windows.WAIT_TIMEOUT) {
		cancel()
		<-result
		t.Fatal("ready child was not actually running")
	}
	started := time.Now()
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("in-flight cancellation failed: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("in-flight native Job did not terminate promptly")
	}
	if state, err := windows.WaitForSingleObject(process, 0); err != nil || state != windows.WAIT_OBJECT_0 {
		t.Fatalf("actual confined child survived cancellation: state=%d err=%v", state, err)
	}
	t.Logf("actual ready child terminated by Job in %s", time.Since(started))
}

type nativeChildReadyOutput struct {
	bytes.Buffer
	ready chan struct{}
	once  sync.Once
	pid   uint32
}

func (output *nativeChildReadyOutput) Write(body []byte) (int, error) {
	n, err := output.Buffer.Write(body)
	marker := regexp.MustCompile(`NATIVE_CONFINED_CHILD_READY ([0-9]+)`).FindStringSubmatch(output.String())
	if len(marker) == 2 {
		pid, e := strconv.ParseUint(marker[1], 10, 32)
		if e == nil {
			output.once.Do(func() { output.pid = uint32(pid); close(output.ready) })
		}
	}
	return n, err
}

func TestWindowsConfinementSleepingChild(t *testing.T) {
	if os.Getenv("MAESTRO_CONFINEMENT_SENTINEL") == "" {
		t.Skip("launched only by native confinement parent")
	}
	fmt.Fprintln(os.Stdout, "NATIVE_CONFINED_CHILD_READY", os.Getpid())
	time.Sleep(30 * time.Second)
}

func TestWindowsConfinementChild(t *testing.T) {
	sentinel := os.Getenv("MAESTRO_CONFINEMENT_SENTINEL")
	if sentinel == "" {
		t.Skip("launched only by native confinement parent")
	}
	var appContainer uint32
	var returned uint32
	if err := windows.GetTokenInformation(windows.GetCurrentProcessToken(), 29, (*byte)(unsafe.Pointer(&appContainer)), 4, &returned); err != nil || appContainer != 1 {
		t.Fatalf("kernel token is not AppContainer: flag=%d err=%v", appContainer, err)
	}
	for _, name := range []string{sentinel, os.Args[0]} {
		file, err := os.OpenFile(name, os.O_WRONLY|os.O_APPEND, 0)
		if file != nil {
			file.Close()
		}
		if !errors.Is(err, syscall.ERROR_ACCESS_DENIED) {
			t.Fatalf("outside/snapshot write not denied: %v", err)
		}
	}
	if _, err := os.ReadFile(sentinel); !errors.Is(err, syscall.ERROR_ACCESS_DENIED) {
		t.Fatalf("outside read not denied: %v", err)
	}
	scratch := filepath.Join(os.Getenv("TMP"), "proof")
	if err := os.WriteFile(scratch, []byte("synthetic"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(scratch); err != nil {
		t.Fatal(err)
	}
	for _, address := range []string{"127.0.0.1:9", "[::1]:9", "198.51.100.1:9", "[2001:db8::1]:9"} {
		for _, protocol := range []string{"tcp", "udp"} {
			c, err := net.DialTimeout(protocol, address, time.Second)
			if err == nil && protocol == "udp" {
				_, err = c.Write([]byte("synthetic-confinement-probe"))
			}
			if c != nil {
				c.Close()
			}
			if !errors.Is(err, windows.WSAEACCES) {
				t.Fatalf("%s %s not kernel-denied: %v", protocol, address, err)
			}
		}
	}
	err := exec.Command(os.Args[0], "-test.run=^$").Run()
	if err == nil {
		t.Fatal("child-process policy allowed a descendant")
	}
	var pathErr *os.PathError
	if !errors.As(err, &pathErr) {
		t.Fatalf("child denial was not a process creation failure: %v", err)
	}
	proof, _ := json.Marshal(map[string]bool{"app_container": true, "network_denied": true, "outside_read_denied": true, "outside_write_denied": true, "snapshot_write_denied": true, "scratch_writable": true, "children_denied": true})
	t.Log("NATIVE_WINDOWS_CONFINEMENT_PASSED", string(proof))
}
