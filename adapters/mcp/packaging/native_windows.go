//go:build windows

package packaging

import (
	"errors"
	"fmt"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/winprivate"
	"golang.org/x/sys/windows"
	"os"
	"path/filepath"
)

func validateNativeScratchIdentity(path string) error {
	return scheduler.ValidatePrivateDirectory(path)
}

func nativeManagedDirectory() (string, error) {
	root, err := windows.KnownFolderPath(windows.FOLDERID_ProgramFiles, 0)
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "ClaudeCode"), nil
}
func nativeSourceDirectoryIdentity(file *os.File) (string, error) {
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(windows.Handle(file.Fd()), &info); err != nil {
		return "", err
	}
	if info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY == 0 {
		return "", errors.New("native source directory identity unavailable")
	}
	return fmt.Sprintf("%d:%d:%d", info.VolumeSerialNumber, info.FileIndexHigh, info.FileIndexLow), nil
}

func makeNativePrivateDirectory(path string) (func(), error) {
	// Fresh bootstraps missing components but deliberately admits an existing
	// directory. Native controls require a genuinely new leaf under a held parent.
	parent, err := winprivate.OpenPinnedTree(filepath.Dir(path))
	if err != nil {
		return nil, err
	}
	defer parent.Close()
	fresh, err := parent.Child(filepath.Base(path), true, true)
	if err != nil {
		return nil, err
	}
	defer fresh.Close()
	pinned, err := winprivate.OpenPinnedTree(path)
	if err != nil {
		return nil, err
	}
	a, err := fresh.File.Stat()
	if err != nil {
		pinned.Close()
		return nil, err
	}
	b, err := pinned.File.Stat()
	if err != nil || !os.SameFile(a, b) {
		pinned.Close()
		return nil, errors.New("native private directory identity changed")
	}
	return func() { _ = pinned.Close() }, nil
}
func nativeProcessBoundary() string {
	return "Each Windows command is associated with its owned Job Object during process creation. Cancellation, exit and output refusal terminate its owned tree and require zero active job processes before return. External process creation outside that job is not attested; failure never qualifies. No global process cleanup occurs."
}
