//go:build windows

package releaseprovider

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unsafe"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/winprivate"
	"golang.org/x/sys/windows"
)

var syncedCloudDLL = windows.NewLazySystemDLL("CldApi.dll")
var syncedCloudState = syncedCloudDLL.NewProc("CfGetPlaceholderStateFromAttributeTag")
var syncedCloudHydrate = syncedCloudDLL.NewProc("CfHydratePlaceholder")

// Only distribution input admits Cloud Files; private-state readers stay strict.
// Relative OPEN_REPARSE_POINT handles inspect each object without following a link.
func openSyncedFile(path string, maximum int64) (*os.File, error) {
	volume := filepath.VolumeName(path)
	if !filepath.IsAbs(path) || filepath.Clean(path) != path || len(volume) != 2 || volume[1] != ':' || maximum < 0 {
		return nil, errors.New("synced source requires a bounded canonical local path")
	}
	names := strings.Split(strings.TrimPrefix(path, volume+`\`), `\`)
	if len(names) > 128 {
		return nil, errors.New("synced source path exceeds component bound")
	}
	for _, name := range names {
		if err := winprivate.Leaf(name); err != nil {
			return nil, err
		}
	}
	current, err := openSyncedWindowsChild(0, `\??\`+volume+`\`, volume+`\`, true)
	if err != nil {
		return nil, err
	}
	for i, name := range names {
		directory := i < len(names)-1
		next, err := openSyncedWindowsChild(windows.Handle(current.Fd()), name, filepath.Join(current.Name(), name), directory)
		_ = current.Close()
		if err != nil {
			return nil, err
		}
		current = next
	}
	ready := false
	defer func() {
		if !ready {
			_ = current.Close()
		}
	}()
	info, err := current.Stat()
	if err != nil || info.Size() < 0 || info.Size() > maximum {
		return nil, errors.New("synced source exceeds declared byte bound")
	}
	cloud, attributes, state, err := inspectSyncedWindowsObject(current, false)
	if err != nil {
		return nil, err
	}
	if cloud && !syncedCloudReady(attributes, state) {
		if err := hydrateSyncedWindowsFile(current, info.Size()); err != nil {
			return nil, err
		}
	}
	if !syncedSingleLink(current, info) {
		return nil, errors.New("synced source remains partial, offline or aliased")
	}
	ready = true
	return current, nil
}

func openSyncedWindowsChild(parent windows.Handle, name, path string, directory bool) (*os.File, error) {
	nativeName, err := windows.NewNTUnicodeString(name)
	if err != nil {
		return nil, err
	}
	attributes := windows.OBJECT_ATTRIBUTES{RootDirectory: parent, ObjectName: nativeName, Attributes: windows.OBJ_CASE_INSENSITIVE}
	attributes.Length = uint32(unsafe.Sizeof(attributes))
	options := uint32(windows.FILE_OPEN_REPARSE_POINT | windows.FILE_SYNCHRONOUS_IO_NONALERT | windows.FILE_NON_DIRECTORY_FILE | windows.FILE_OPEN_NO_RECALL)
	access := uint32(windows.FILE_READ_ATTRIBUTES | windows.FILE_READ_DATA | windows.SYNCHRONIZE)
	if directory {
		options = windows.FILE_OPEN_REPARSE_POINT | windows.FILE_SYNCHRONOUS_IO_NONALERT | windows.FILE_DIRECTORY_FILE
		access = windows.FILE_READ_ATTRIBUTES | windows.FILE_LIST_DIRECTORY | windows.FILE_TRAVERSE | windows.SYNCHRONIZE
	}
	var status windows.IO_STATUS_BLOCK
	var allocation int64
	var handle windows.Handle
	err = windows.NtCreateFile(&handle, access, &attributes, &status, &allocation, 0, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, windows.FILE_OPEN, options, 0, 0)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(handle), path)
	if file == nil {
		_ = windows.CloseHandle(handle)
		return nil, errors.New("synced source handle unavailable")
	}
	_, _, _, err = inspectSyncedWindowsObject(file, directory)
	if err == nil {
		var final string
		final, err = winprivate.FinalLocalPath(file)
		if err == nil && !strings.EqualFold(final, path) {
			err = errors.New("synced source canonical path differs")
		}
	}
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	return file, nil
}

func inspectSyncedWindowsObject(file *os.File, directory bool) (cloud bool, attributes, state uint32, err error) {
	defer runtime.KeepAlive(file)
	handle := windows.Handle(file.Fd())
	kind, err := windows.GetFileType(handle)
	if err != nil || kind != windows.FILE_TYPE_DISK {
		return false, 0, 0, errors.New("synced source requires a disk object")
	}
	var info windows.ByHandleFileInformation
	if err = windows.GetFileInformationByHandle(handle, &info); err != nil {
		return false, 0, 0, err
	}
	var tag struct{ Attributes, Tag uint32 }
	if err = windows.GetFileInformationByHandleEx(handle, windows.FileAttributeTagInfo, (*byte)(unsafe.Pointer(&tag)), uint32(unsafe.Sizeof(tag))); err != nil {
		return false, 0, 0, err
	}
	if tag.Attributes != info.FileAttributes || (!directory && info.NumberOfLinks != 1) {
		return false, 0, 0, errors.New("synced source attributes changed or file is aliased")
	}
	cloud, err = syncedCloudSourceKind(tag.Attributes, tag.Tag, directory)
	if err != nil || !cloud {
		return cloud, tag.Attributes, 0, err
	}
	if err = syncedCloudState.Find(); err != nil {
		return false, 0, 0, errors.New("Cloud Files source API unavailable")
	}
	value, _, _ := syncedCloudState.Call(uintptr(tag.Attributes), uintptr(tag.Tag))
	state = uint32(value)
	if state&1 == 0 || state&^uint32(0x3f) != 0 {
		return false, 0, 0, errors.New("Cloud Files source state unavailable")
	}
	return true, tag.Attributes, state, nil
}

func hydrateSyncedWindowsFile(file *os.File, size int64) error {
	if runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64" {
		return errors.New("Cloud Files source architecture unavailable")
	}
	if err := syncedCloudHydrate.Find(); err != nil {
		return errors.New("Cloud Files hydration API unavailable")
	}
	// Exact measured bytes, never CF_EOF. Provider latency is synchronous, not a
	// wall-time guarantee; errors/partial data never admit a release snapshot.
	result, _, _ := syncedCloudHydrate.Call(uintptr(file.Fd()), 0, uintptr(size), 0, 0)
	runtime.KeepAlive(file)
	if uint32(result) != 0 {
		return fmt.Errorf("synced Cloud Files hydration unavailable: HRESULT %08x", uint32(result))
	}
	return nil
}

func syncedSingleLink(file *os.File, _ os.FileInfo) bool {
	cloud, attributes, state, err := inspectSyncedWindowsObject(file, false)
	return err == nil && (!cloud || syncedCloudReady(attributes, state))
}
