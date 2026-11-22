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
	"runtime"
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
	assertWindowsQualificationFailureFactsPrivacy(t)
	assertRegistryReadCapabilityPolicy(t)
	r17AssertProtocol(t)
	work, child := nativeWindowsFixture(t)
	var registryFixtures *r17FixtureSet
	var registryErr error
	if r17HostedAssessment(os.Getenv("MAESTRO_WINDOWS_DOCLING_QUALIFY"), os.Getenv("GITHUB_ACTIONS"), os.Getenv("RUNNER_ENVIRONMENT")) {
		registryFixtures, registryErr = r17Prepare(work)
		defer registryFixtures.finish(t)
		if registryErr == nil {
			registryErr = r17CreateMarker(work)
		}
		if registryErr != nil {
			r17Diagnostic(t, registryErr)
			t.FailNow()
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	var stdout, stderr bytes.Buffer
	if registryFixtures != nil {
		registryFixtures.childrenStopped = false
	}
	err := runWindowsConfined(ctx, work, child, []string{"-test.run=^TestWindowsConfinementChild$", "-test.v"}, []byte("probe"), &stdout, &stderr)
	if err != nil {
		logWindowsConfinementFailureContext(t)
		t.Fatalf("native confinement failed: %v stderr=%s stdout=%s", err, stderr.String(), stdout.String())
	}
	if !strings.Contains(stdout.String(), "NATIVE_WINDOWS_CONFINEMENT_PASSED") {
		t.Fatalf("real child did not finish controls: %s", stdout.String())
	}
	var first []r17Row
	if registryFixtures != nil {
		registryFixtures.childrenStopped = true
		first, registryErr = r17ParseChild(stdout.String())
		if registryErr == nil {
			registryErr = r17RequireMarker(work)
		}
		if registryErr == nil {
			registryErr = registryFixtures.validate()
		}
		if registryErr != nil {
			r17Diagnostic(t, registryErr)
			t.FailNow()
		}
	}
	// Classification and conversion require a second fresh profile over the same staging.
	stdout.Reset()
	stderr.Reset()
	if registryFixtures != nil {
		registryFixtures.childrenStopped = false
	}
	if err = runWindowsConfined(ctx, work, child, []string{"-test.run=^TestWindowsConfinementChild$"}, []byte("probe"), &stdout, &stderr); err != nil {
		t.Fatalf("repeated confined phase: %v stderr=%s", err, stderr.String())
	}
	if registryFixtures == nil {
		return // Ordinary local suites preserve the original two-call proof.
	}
	registryFixtures.childrenStopped = true
	second, registryErr := r17ParseChild(stdout.String())
	if registryErr == nil {
		registryErr = r17RequireMarker(work)
	}
	if registryErr == nil {
		registryErr = registryFixtures.validate()
	}
	if registryErr == nil {
		registryErr = registryFixtures.cleanup()
	}
	if registryErr != nil {
		r17Diagnostic(t, registryErr)
		t.FailNow()
	}
	fmt.Fprintln(os.Stdout, "R17_REGISTRY_CLEANUP state=complete")
	// Publish only after both actual children, unchanged fixtures and cleanup.
	r17Publish(first, second)
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
		logWindowsConfinementFailureContext(t)
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

// Observe only the parent's post-failure context, never the child's failed DLL.
// Do not load USER32: its initialization could alter later parents' inherited
// desktop context. Inspection failures never change confinement or the verdict.
func logWindowsConfinementFailureContext(t *testing.T) {
	t.Helper()
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	var session uint32
	err := windows.ProcessIdToSessionId(windows.GetCurrentProcessId(), &session)
	t.Logf("confinement parent-after-failure session_id=%d inspect_error=%v", session, err)
	// Kernel32 returns the parent's original requested desktop, not the child's
	// current desktop. Keep nil, empty and bounded/truncated names distinct.
	var startup windows.StartupInfo
	if err := windows.GetStartupInfo(&startup); err != nil {
		t.Logf("confinement parent_original_startup_desktop=unavailable inspect_error=%v", err)
	} else if startup.Desktop == nil {
		t.Log("confinement parent_original_startup_desktop=nil")
	} else {
		name := make([]uint16, 0, 256)
		terminated := false
		for i := 0; i < 256; i++ {
			unit := *(*uint16)(unsafe.Add(unsafe.Pointer(startup.Desktop), uintptr(i)*unsafe.Sizeof(uint16(0))))
			if unit == 0 {
				terminated = true
				break
			}
			name = append(name, unit)
		}
		t.Logf("confinement parent_original_startup_desktop=%q terminated_within_bound=%t", windows.UTF16ToString(name), terminated)
	}
	integrity := make([]byte, 256)
	var returned uint32
	err = windows.GetTokenInformation(windows.GetCurrentProcessToken(), windows.TokenIntegrityLevel, &integrity[0], uint32(len(integrity)), &returned)
	if err == nil && returned >= uint32(unsafe.Sizeof(windows.Tokenmandatorylabel{})) && returned <= uint32(len(integrity)) {
		label := (*windows.Tokenmandatorylabel)(unsafe.Pointer(&integrity[0]))
		if label.Label.Sid != nil && label.Label.Sid.IsValid() && label.Label.Sid.SubAuthorityCount() > 0 {
			t.Logf("confinement parent-after-failure token_integrity_rid=0x%x", label.Label.Sid.SubAuthority(uint32(label.Label.Sid.SubAuthorityCount()-1)))
		} else {
			t.Log("confinement parent-after-failure token_integrity=invalid")
		}
	} else {
		t.Logf("confinement parent-after-failure token_integrity=unavailable bytes=%d inspect_error=%v", returned, err)
	}
	runtime.KeepAlive(integrity)
	moduleName, err := windows.UTF16PtrFromString("user32.dll")
	if err != nil {
		t.Logf("confinement parent-after-failure desktop_inspection=unavailable inspect_error=%v", err)
		return
	}
	var user32 windows.Handle
	// Flags 0 retain an existing module reference, but never load an absent DLL.
	if err := windows.GetModuleHandleEx(0, moduleName, &user32); err != nil {
		t.Logf("confinement parent-after-failure desktop_inspection=omitted user32_not_loaded inspect_error=%v", err)
		return
	}
	defer windows.FreeLibrary(user32)
	information, err := windows.GetProcAddress(user32, "GetUserObjectInformationW")
	if err != nil {
		t.Logf("confinement parent-after-failure desktop_inspection=unavailable inspect_error=%v", err)
		return
	}
	for _, object := range []struct {
		kind, procedure string
		arguments       []uintptr
	}{{"window_station", "GetProcessWindowStation", nil}, {"desktop", "GetThreadDesktop", []uintptr{uintptr(windows.GetCurrentThreadId())}}} {
		procedure, err := windows.GetProcAddress(user32, object.procedure)
		if err != nil {
			t.Logf("confinement parent-after-failure %s=unavailable inspect_error=%v", object.kind, err)
			continue
		}
		handle, _, inspectErr := syscall.SyscallN(procedure, object.arguments...)
		if handle == 0 {
			t.Logf("confinement parent-after-failure %s=unavailable inspect_error=%v", object.kind, inspectErr)
			continue
		}
		// Borrowed handles belong to the process/thread; do not close or mutate them.
		var name [256]uint16
		ok, _, nameErr := syscall.SyscallN(information, handle, 2 /* UOI_NAME */, uintptr(unsafe.Pointer(&name[0])), unsafe.Sizeof(name), uintptr(unsafe.Pointer(&returned)))
		if ok != 0 && returned <= uint32(unsafe.Sizeof(name)) {
			t.Logf("confinement parent-after-failure %s_name=%q", object.kind, windows.UTF16ToString(name[:]))
		} else {
			t.Logf("confinement parent-after-failure %s_name=unavailable inspect_error=%v", object.kind, nameErr)
		}
		// LABEL_SECURITY_INFORMATION returns only mandatory-label information,
		// not the owner's SID or the discretionary access list.
		sd, labelErr := windows.GetSecurityInfo(windows.Handle(handle), windows.SE_WINDOW_OBJECT, windows.LABEL_SECURITY_INFORMATION)
		if labelErr == nil && sd != nil {
			t.Logf("confinement parent-after-failure %s_mandatory_label=%q (empty means implicit medium)", object.kind, sd.String())
		} else {
			t.Logf("confinement parent-after-failure %s_mandatory_label=unavailable inspect_error=%v", object.kind, labelErr)
		}
	}
}

type nativeChildReadyOutput struct {
	buffer bytes.Buffer
	ready  chan struct{}
	once   sync.Once
	pid    uint32
}

func (output *nativeChildReadyOutput) Write(body []byte) (int, error) {
	n, err := output.buffer.Write(body)
	marker := regexp.MustCompile(`NATIVE_CONFINED_CHILD_READY ([0-9]+)`).FindStringSubmatch(output.buffer.String())
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
	scratchPath, scratchErr := windows.UTF16PtrFromString(scratch)
	if scratchErr != nil {
		t.Fatal(scratchErr)
	}
	// Test file permission through file APIs: failed Winsock initialization in
	// Go 1.26 can leave os.File writes using an uninitialized network FD kind.
	scratchFile, scratchErr := windows.CreateFile(scratchPath, windows.GENERIC_WRITE, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.CREATE_ALWAYS, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if scratchErr != nil {
		t.Fatal(scratchErr)
	}
	scratchBody := []byte("synthetic")
	var written uint32
	writeErr := windows.WriteFile(scratchFile, scratchBody, &written, nil)
	closeErr := windows.CloseHandle(scratchFile)
	if writeErr != nil {
		t.Fatal(writeErr)
	}
	if written != uint32(len(scratchBody)) {
		t.Fatalf("scratch file write incomplete: got %d bytes, want %d", written, len(scratchBody))
	}
	if closeErr != nil {
		t.Fatal(closeErr)
	}
	if err := os.Remove(scratch); err != nil {
		t.Fatal(err)
	}
	networkDenied := true
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
				networkDenied = false
				t.Errorf("%s %s not kernel-denied: %v", protocol, address, err)
			}
		}
	}
	if !networkDenied {
		// Observe startup only after every original probe has completed. Neither
		// this result nor a successful retry can turn those failures into denials.
		var data windows.WSAData
		if startupErr := windows.WSAStartup(0x0202, &data); startupErr != nil {
			var code syscall.Errno
			if errors.As(startupErr, &code) {
				t.Logf("NATIVE_WINSOCK_FAILURE_OBSERVATION operation=startup result=%d", uint32(code))
			} else {
				t.Log("NATIVE_WINSOCK_FAILURE_OBSERVATION operation=startup result=unavailable")
			}
		} else {
			t.Log("NATIVE_WINSOCK_FAILURE_OBSERVATION operation=startup result=0")
			cleanupErr := windows.WSACleanup()
			var code syscall.Errno
			if cleanupErr == nil {
				t.Log("NATIVE_WINSOCK_FAILURE_OBSERVATION operation=cleanup result=0")
			} else if errors.As(cleanupErr, &code) {
				t.Logf("NATIVE_WINSOCK_FAILURE_OBSERVATION operation=cleanup result=%d", uint32(code))
			} else {
				t.Log("NATIVE_WINSOCK_FAILURE_OBSERVATION operation=cleanup result=unavailable")
			}
		}
		t.FailNow()
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
	if r17ChildAssessmentSelected(t) {
		r17ObserveChild(t)
		// Direct output preserves the same marker in the non-verbose phase.
		fmt.Fprintln(os.Stdout, "NATIVE_WINDOWS_CONFINEMENT_PASSED", string(proof))
	} else {
		t.Log("NATIVE_WINDOWS_CONFINEMENT_PASSED", string(proof))
	}
}

// Pure SID/count/attribute checks complement, never replace, the actual child token query.
func assertRegistryReadCapabilityPolicy(t *testing.T) {
	t.Helper()
	expected, err := registryReadCapabilitySID()
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := windows.CreateWellKnownSid(windows.WinWorldSid)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name              string
		count, attributes uint32
		sid               *windows.SID
		short             bool
		want              bool
	}{
		{"exact", 1, windows.SE_GROUP_ENABLED, expected, false, true},
		{"missing", 0, windows.SE_GROUP_ENABLED, expected, false, false},
		{"extra", 2, windows.SE_GROUP_ENABLED, expected, false, false},
		{"foreign", 1, windows.SE_GROUP_ENABLED, foreign, false, false},
		{"nil", 1, windows.SE_GROUP_ENABLED, nil, false, false},
		{"disabled", 1, 0, expected, false, false},
		{"extra_attributes", 1, windows.SE_GROUP_ENABLED | windows.SE_GROUP_ENABLED_BY_DEFAULT, expected, false, false},
		{"truncated", 1, windows.SE_GROUP_ENABLED, expected, true, false},
	} {
		t.Run("registry_read_policy_"+tc.name, func(t *testing.T) {
			groups := windows.Tokengroups{GroupCount: tc.count, Groups: [1]windows.SIDAndAttributes{{Sid: tc.sid, Attributes: tc.attributes}}}
			body := unsafe.Slice((*byte)(unsafe.Pointer(&groups)), int(unsafe.Sizeof(groups)))
			returned := uint32(len(body))
			if tc.short {
				returned--
			}
			if got := validateRegistryReadCapability(body, returned, expected) == nil; got != tc.want {
				t.Fatalf("exact registryRead policy accepted=%t, want %t", got, tc.want)
			}
			if tc.want && validateRegistryReadCapability(body, returned, nil) == nil {
				t.Fatal("missing expected registryRead SID admitted")
			}
		})
	}
}
