//go:build windows

package datahome_test

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/datahome"
	"golang.org/x/sys/windows"
)

func resolveDirectoryIdentity(t *testing.T, path string) [3]uint32 {
	t.Helper()
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	handle, err := windows.CreateFile(name, windows.FILE_READ_ATTRIBUTES, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		t.Fatal("case capability requires a real directory handle", err)
	}
	var info windows.ByHandleFileInformation
	infoErr := windows.GetFileInformationByHandle(handle, &info)
	closeErr := windows.CloseHandle(handle)
	if infoErr != nil || closeErr != nil || info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY == 0 || info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		t.Fatal("case capability directory is unavailable or aliased", infoErr, closeErr)
	}
	return [3]uint32{info.VolumeSerialNumber, info.FileIndexHigh, info.FileIndexLow}
}

func TestWindowsResolveCaseAliasesRespectLocatorAuthority(t *testing.T) {
	opts := fixture(t)
	lower, upper := filepath.Join(opts.UserHome, ".maestro"), filepath.Join(opts.UserHome, ".MAESTRO")
	for _, path := range []string{lower, upper} {
		if err := os.Mkdir(path, 0700); err != nil && !os.IsExist(err) {
			t.Fatal(err)
		}
	}
	lowerInfo, err := os.Stat(lower)
	if err != nil {
		t.Fatal(err)
	}
	upperInfo, err := os.Stat(upper)
	if err != nil {
		t.Fatal(err)
	}
	lowerIdentity, upperIdentity := resolveDirectoryIdentity(t, lower), resolveDirectoryIdentity(t, upper)
	same := os.SameFile(lowerInfo, upperInfo)
	if same != (lowerIdentity == upperIdentity) {
		t.Fatal("case capability identity sources disagree")
	}
	t.Logf("selected_directory_case_identity lower=%v upper=%v same_object=%t", lowerIdentity, upperIdentity, same)
	if same {
		assertCaseAliasLocatorContract(t, opts, lower, upper)
		return
	} else {
		selected := opts
		selected.LookupEnv = func(string) (string, bool) { return upper, true }
		if got, err := datahome.InitializeFresh(selected); err != nil || !got.Writable {
			t.Fatalf("distinct case-sensitive HOME positive control failed: %+v %v", got, err)
		}
		if err := os.Remove(filepath.Join(opts.UserHome, ".maestro-home.json")); err != nil {
			t.Fatal(err)
		}
		before := homeSnapshot(t, opts.UserHome)
		if got, err := datahome.Resolve(selected); err == nil || got.Writable || got.Diagnostic != "marker-required" {
			t.Fatalf("distinct nondefault case spelling bypassed locator: %+v %v", got, err)
		}
		if after := homeSnapshot(t, opts.UserHome); !reflect.DeepEqual(before, after) {
			t.Fatal("missing locator refusal changed case-distinct HOME")
		}
		confirm(t, lower)
		record(t, filepath.Join(opts.UserHome, ".maestro-home.json"), map[string]any{"schema_version": 1, "home": lower, "attempt_id": ""})
		opts = selected
	}
	before := homeSnapshot(t, opts.UserHome)
	got, err := datahome.Resolve(opts)
	if err == nil || got.Writable || got.Diagnostic != "marker-conflict" {
		t.Fatalf("locator authorized a different actual HOME: %+v %v", got, err)
	}
	if after := homeSnapshot(t, opts.UserHome); !reflect.DeepEqual(before, after) {
		t.Fatal("conflicting locator refusal changed profile or HOME")
	}
	recordCasePairEvidence(t, upper, lower, false)
}
