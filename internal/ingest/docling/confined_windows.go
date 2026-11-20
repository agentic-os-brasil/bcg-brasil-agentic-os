//go:build windows

package docling

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"unicode/utf16"
	"unsafe"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/userlevel"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/winprivate"
	"golang.org/x/sys/windows"
)

var userenv = windows.NewLazySystemDLL("userenv.dll")
var createAppContainerProfile = userenv.NewProc("CreateAppContainerProfile")
var deleteAppContainerProfile = userenv.NewProc("DeleteAppContainerProfile")

const securityCapabilitiesAttribute = 0x00020009
const childProcessPolicyAttribute = 0x0002000e

type securityCapabilities struct {
	SID             *windows.SID
	Capabilities    *windows.SIDAndAttributes
	Count, Reserved uint32
}

func runConfinedBridge(ctx context.Context, work, snapshot string, request []byte, stdout, stderr *boundedOutput) error {
	return runWindowsConfined(ctx, work, filepath.Join(snapshot, "python", "python.exe"), []string{"-I", "-S", "-B", filepath.Join(snapshot, "bridge.py")}, request, stdout, stderr)
}

// runWindowsConfined creates only a unique per-user AppContainer profile and
// execution-local ACLs. It never changes the installed pack or source documents.
func runWindowsConfined(ctx context.Context, work, executable string, args []string, request []byte, stdout, stderr io.Writer) (result error) {
	if err := userlevel.EnsureNotElevated(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	custody, err := winprivate.OpenPinnedTree(work)
	if err != nil {
		return err
	}
	defer custody.Close()
	// Admit the complete freshly-created staging tree before granting the child SID.
	handles, paths, err := holdExecutionTree(work)
	if err != nil {
		return err
	}
	defer func() {
		for _, h := range handles {
			windows.CloseHandle(h)
		}
	}()
	moniker, err := windows.UTF16PtrFromString("Maestro.Docling." + winprivate.Random())
	if err != nil {
		return err
	}
	var sid *windows.SID
	hr, _, _ := createAppContainerProfile.Call(uintptr(unsafe.Pointer(moniker)), uintptr(unsafe.Pointer(moniker)), uintptr(unsafe.Pointer(moniker)), 0, 0, uintptr(unsafe.Pointer(&sid)))
	if int32(hr) < 0 || sid == nil {
		return fmt.Errorf("create conversion AppContainer: HRESULT 0x%x", hr)
	}
	defer windows.FreeSid(sid)
	defer func() {
		hr, _, _ := deleteAppContainerProfile.Call(uintptr(unsafe.Pointer(moniker)))
		if int32(hr) < 0 {
			result = errors.Join(result, fmt.Errorf("remove conversion AppContainer: HRESULT 0x%x", hr))
		}
	}()
	defer func() { result = errors.Join(result, restoreExecutionTree(handles)) }()
	if err := grantExecutionTree(work, handles, paths, sid); err != nil {
		return err
	}
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(job)
	limits := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE | windows.JOB_OBJECT_LIMIT_ACTIVE_PROCESS | windows.JOB_OBJECT_LIMIT_PROCESS_MEMORY
	limits.BasicLimitInformation.ActiveProcessLimit = 1
	limits.ProcessMemoryLimit = 1 << 30
	if _, err = windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits))); err != nil {
		return err
	}
	attrs, err := windows.NewProcThreadAttributeList(3)
	if err != nil {
		return err
	}
	defer attrs.Delete()
	capabilities := securityCapabilities{SID: sid}
	if err = attrs.Update(securityCapabilitiesAttribute, unsafe.Pointer(&capabilities), unsafe.Sizeof(capabilities)); err != nil {
		return err
	}
	childPolicy := uint32(1) // PROCESS_CREATION_CHILD_PROCESS_RESTRICTED.
	if err = attrs.Update(childProcessPolicyAttribute, unsafe.Pointer(&childPolicy), unsafe.Sizeof(childPolicy)); err != nil {
		return err
	}
	inRead, inWrite, err := windowsPipe()
	if err != nil {
		return err
	}
	defer inRead.Close()
	defer inWrite.Close()
	outRead, outWrite, err := windowsPipe()
	if err != nil {
		return err
	}
	defer outRead.Close()
	defer outWrite.Close()
	errRead, errWrite, err := windowsPipe()
	if err != nil {
		return err
	}
	defer errRead.Close()
	defer errWrite.Close()
	inherited := []windows.Handle{windows.Handle(inRead.Fd()), windows.Handle(outWrite.Fd()), windows.Handle(errWrite.Fd())}
	for _, h := range inherited {
		if err = windows.SetHandleInformation(h, windows.HANDLE_FLAG_INHERIT, windows.HANDLE_FLAG_INHERIT); err != nil {
			return err
		}
	}
	if err = attrs.Update(windows.PROC_THREAD_ATTRIBUTE_HANDLE_LIST, unsafe.Pointer(&inherited[0]), uintptr(len(inherited))*unsafe.Sizeof(inherited[0])); err != nil {
		return err
	}
	startup := windows.StartupInfoEx{ProcThreadAttributeList: attrs.List()}
	startup.Cb = uint32(unsafe.Sizeof(startup))
	startup.Flags = windows.STARTF_USESTDHANDLES
	startup.StdInput, startup.StdOutput, startup.StdErr = inherited[0], inherited[1], inherited[2]
	app, err := windows.UTF16PtrFromString(executable)
	if err != nil {
		return err
	}
	commandLine, err := windows.UTF16PtrFromString(windows.ComposeCommandLine(append([]string{executable}, args...)))
	if err != nil {
		return err
	}
	directory, err := windows.UTF16PtrFromString(work)
	if err != nil {
		return err
	}
	environment := bridgeEnvironment(work)
	// System DLL discovery is needed by CPython; no ambient user environment survives.
	systemDirectory, err := windows.GetSystemWindowsDirectory()
	if err != nil {
		return err
	}
	environment[0] = "PATH=" + filepath.Join(systemDirectory, "System32")
	sentinel, err := os.CreateTemp(filepath.Dir(work), ".conversion-denied-")
	if err != nil {
		return err
	}
	sentinelPath := sentinel.Name()
	if _, err = sentinel.WriteString("synthetic confinement sentinel"); err != nil {
		sentinel.Close()
		os.Remove(sentinelPath)
		return err
	}
	if err = sentinel.Close(); err != nil {
		os.Remove(sentinelPath)
		return err
	}
	defer os.Remove(sentinelPath)
	environment = append(environment, "MAESTRO_CONFINEMENT_SENTINEL="+sentinelPath, "SystemRoot="+systemDirectory, "TEMP="+filepath.Join(work, "tmp"), "TMP="+filepath.Join(work, "tmp"), "USERPROFILE="+filepath.Join(work, "home"), "APPDATA="+filepath.Join(work, "config"), "LOCALAPPDATA="+filepath.Join(work, "data"))
	sort.Slice(environment, func(i, j int) bool { return strings.ToUpper(environment[i]) < strings.ToUpper(environment[j]) })
	block := utf16.Encode([]rune(strings.Join(environment, "\x00") + "\x00\x00"))
	process := windows.ProcessInformation{}
	flags := uint32(windows.CREATE_SUSPENDED | windows.CREATE_UNICODE_ENVIRONMENT | windows.EXTENDED_STARTUPINFO_PRESENT | windows.CREATE_NO_WINDOW)
	if err = windows.CreateProcess(app, commandLine, nil, nil, true, flags, &block[0], directory, &startup.StartupInfo, &process); err != nil {
		return err
	}
	runtime.KeepAlive(capabilities)
	runtime.KeepAlive(inherited)
	runtime.KeepAlive(block)
	defer windows.CloseHandle(process.Process)
	defer windows.CloseHandle(process.Thread)
	// Nothing executes before the Job owns the process; failures terminate the suspended child.
	if err = windows.AssignProcessToJobObject(job, process.Process); err != nil {
		windows.TerminateProcess(process.Process, 1)
		return err
	}
	if err = validateConfinedToken(process.Process, sid); err != nil {
		windows.TerminateJobObject(job, 1)
		return err
	}
	if _, err = windows.ResumeThread(process.Thread); err != nil {
		windows.TerminateJobObject(job, 1)
		return err
	}
	inRead.Close()
	outWrite.Close()
	errWrite.Close()
	var ioWait sync.WaitGroup
	ioErrors := make(chan error, 3)
	for _, copy := range []struct {
		destination io.Writer
		source      io.Reader
	}{{inWrite, bytes.NewReader(request)}, {stdout, outRead}, {stderr, errRead}} {
		ioWait.Add(1)
		go func(destination io.Writer, source io.Reader) {
			defer ioWait.Done()
			_, e := io.Copy(destination, source)
			ioErrors <- e
			if destination == inWrite {
				inWrite.Close()
			}
		}(copy.destination, copy.source)
	}
	done := make(chan struct{})
	watcherFinished := make(chan struct{})
	go func() {
		defer close(watcherFinished)
		select {
		case <-ctx.Done():
			windows.TerminateJobObject(job, 1)
		case <-done:
		}
	}()
	_, waitErr := windows.WaitForSingleObject(process.Process, windows.INFINITE)
	close(done)
	<-watcherFinished
	// Job kill also closes any unexpected inherited pipe copies before readers wait.
	windows.TerminateJobObject(job, 1)
	inWrite.Close()
	ioWait.Wait()
	close(ioErrors)
	for copyErr := range ioErrors {
		if copyErr != nil {
			result = errors.Join(result, copyErr)
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if waitErr != nil {
		return waitErr
	}
	var code uint32
	if err = windows.GetExitCodeProcess(process.Process, &code); err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("confined converter exit %d (status=0x%08x pid=%d creation_flags=0x%08x)", code, code, process.ProcessId, flags)
	}
	return result
}

func windowsPipe() (*os.File, *os.File, error) {
	var reader, writer windows.Handle
	if err := windows.CreatePipe(&reader, &writer, nil, 0); err != nil {
		return nil, nil, err
	}
	return os.NewFile(uintptr(reader), "conversion-pipe-read"), os.NewFile(uintptr(writer), "conversion-pipe-write"), nil
}

// Every handle remains held without delete/write sharing until the process exits.
func holdExecutionTree(root string) ([]windows.Handle, []string, error) {
	var handles []windows.Handle
	var paths []string
	err := filepath.WalkDir(root, func(name string, item os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if len(handles) >= maximumFiles+maximumFiles {
			return errors.New("execution_entry_limit")
		}
		path, err := windows.UTF16PtrFromString(name)
		if err != nil {
			return err
		}
		flags := uint32(windows.FILE_FLAG_OPEN_REPARSE_POINT)
		if item.IsDir() {
			flags |= windows.FILE_FLAG_BACKUP_SEMANTICS
		}
		sharing := uint32(windows.FILE_SHARE_READ)
		if item.IsDir() {
			sharing |= windows.FILE_SHARE_WRITE
		}
		h, err := windows.CreateFile(path, windows.FILE_GENERIC_READ|windows.READ_CONTROL|windows.WRITE_DAC|windows.WRITE_OWNER, sharing, nil, windows.OPEN_EXISTING, flags, 0)
		if err != nil {
			return err
		}
		if err = winprivate.Validate(h, item.IsDir()); err != nil {
			windows.CloseHandle(h)
			return err
		}
		handles = append(handles, h)
		paths = append(paths, name)
		return nil
	})
	if err != nil {
		for _, h := range handles {
			windows.CloseHandle(h)
		}
		return nil, nil, err
	}
	return handles, paths, nil
}

func grantExecutionTree(root string, handles []windows.Handle, paths []string, sid *windows.SID) error {
	owner, err := winprivate.CurrentUser()
	if err != nil {
		return err
	}
	for i, h := range handles {
		relative, err := filepath.Rel(root, paths[i])
		if err != nil {
			return err
		}
		writable := false
		for _, directory := range []string{"home", "cache", "models", "tmp", "config", "data"} {
			if relative == directory || strings.HasPrefix(relative, directory+string(filepath.Separator)) {
				writable = true
				break
			}
		}
		access := "FRFX"
		inheritance := ""
		if writable {
			access = "FA"
			inheritance = "OICI"
		}
		sd, err := windows.SecurityDescriptorFromString("D:P(A;;FA;;;SY)(A;;FA;;;BA)(A;;FA;;;" + owner.String() + ")(A;" + inheritance + ";" + access + ";;;" + sid.String() + ")")
		if err != nil {
			return err
		}
		dacl, _, err := sd.DACL()
		if err != nil {
			return err
		}
		if err = windows.SetSecurityInfo(h, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, dacl, nil); err != nil {
			return err
		}
		if writable {
			label, err := windows.SecurityDescriptorFromString("S:(ML;OICI;NW;;;LW)")
			if err != nil {
				return err
			}
			sacl, _, err := label.SACL()
			if err != nil {
				return err
			}
			if err = windows.SetSecurityInfo(h, windows.SE_FILE_OBJECT, windows.LABEL_SECURITY_INFORMATION, nil, nil, nil, sacl); err != nil {
				return err
			}
		}
	}
	return nil
}

// Reset execution-only ACLs between JSON classification and conversion phases.
func restoreExecutionTree(handles []windows.Handle) error {
	owner, err := winprivate.CurrentUser()
	if err != nil {
		return err
	}
	sd, err := windows.SecurityDescriptorFromString("D:P(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)(A;OICI;FA;;;" + owner.String() + ")S:(ML;OICI;NW;;;ME)")
	if err != nil {
		return err
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	sacl, _, err := sd.SACL()
	if err != nil {
		return err
	}
	var result error
	for _, h := range handles {
		err = windows.SetSecurityInfo(h, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION|windows.LABEL_SECURITY_INFORMATION, nil, nil, dacl, sacl)
		result = errors.Join(result, err)
	}
	return result
}

func validateConfinedToken(process windows.Handle, expected *windows.SID) error {
	var token windows.Token
	if err := windows.OpenProcessToken(process, windows.TOKEN_QUERY, &token); err != nil {
		return err
	}
	defer token.Close()
	var isContainer, returned uint32
	if err := windows.GetTokenInformation(token, 29, (*byte)(unsafe.Pointer(&isContainer)), 4, &returned); err != nil {
		return err
	}
	if isContainer != 1 {
		return errors.New("converter_token_not_appcontainer")
	}
	// TOKEN_APPCONTAINER_INFORMATION includes the variable-length SID after its pointer.
	container := make([]byte, 256)
	if err := windows.GetTokenInformation(token, 31, &container[0], uint32(len(container)), &returned); err != nil {
		return err
	}
	if returned < uint32(unsafe.Sizeof(uintptr(0))) || returned > uint32(len(container)) {
		return errors.New("converter_token_invalid_appcontainer")
	}
	app := (*struct{ SID *windows.SID })(unsafe.Pointer(&container[0]))
	if app.SID == nil || !app.SID.IsValid() || !app.SID.Equals(expected) {
		return errors.New("converter_token_wrong_appcontainer")
	}
	capabilities := make([]byte, 64<<10)
	if err := windows.GetTokenInformation(token, 30, &capabilities[0], uint32(len(capabilities)), &returned); err != nil {
		return err
	}
	if returned < 4 || *(*uint32)(unsafe.Pointer(&capabilities[0])) != 0 {
		return errors.New("converter_token_has_capabilities")
	}
	return nil
}
