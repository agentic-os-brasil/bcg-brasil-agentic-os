//go:build windows

package memory

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

func statusDirectoryAlias(t *testing.T, alias, target string) {
	t.Helper()
	shell := filepath.Join(os.Getenv("SystemRoot"), "System32", "cmd.exe")
	if out, err := exec.Command(shell, "/c", "mklink", "/J", alias, target).CombinedOutput(); err != nil {
		t.Fatalf("required ordinary-user junction fixture unavailable: %s %v", out, err)
	}
}

func statusSecuritySnapshot(t *testing.T, path string) string {
	t.Helper()
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	return sd.String()
}
