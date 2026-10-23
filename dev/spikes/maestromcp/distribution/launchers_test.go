package distribution

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestMacLaunchersPreserveSpecialPathsAsArguments(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell argument proof")
	}
	root := filepath.Join(t.TempDir(), "space ' dollar$ & bracket[ path")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "versions", "0.0.1")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	installer := filepath.Join(dir, "installer")
	if err := os.WriteFile(installer, []byte("#!/bin/sh\nprintf '%s\\n' \"$@\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := WriteFeedLauncher(root, Manifest{Target: "darwin-arm64", Version: "0.0.1"}); err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command("/bin/sh", filepath.Join(root, "Instalar Maestro.command")).CombinedOutput()
	if err != nil {
		t.Fatalf("%v: %s", err, output)
	}
	want := []string{"install", "--feed", root, "--activate-code", "--open-desktop"}
	if got := strings.Split(strings.TrimSuffix(string(output), "\n"), "\n"); !reflect.DeepEqual(got, want) {
		t.Fatalf("args %#v", got)
	}
	r := Receipt{Home: root, Target: "darwin-arm64", Installer: installer, FeedRoot: root + "/feed ' $ &"}
	if err := WriteLocalLauncher(r); err != nil {
		t.Fatal(err)
	}
	output, err = exec.Command("/bin/sh", filepath.Join(root, "Atualizar Maestro.command")).CombinedOutput()
	if err != nil {
		t.Fatalf("%v: %s", err, output)
	}
	want = []string{"update", "--feed", r.FeedRoot, "--home", root, "--activate-code", "--open-desktop"}
	if got := strings.Split(strings.TrimSuffix(string(output), "\n"), "\n"); !reflect.DeepEqual(got, want) {
		t.Fatalf("args %#v", got)
	}
}

func TestLaunchersRefuseUnownedFileAndSymlink(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "Instalar Maestro.command")
	if err := os.WriteFile(file, []byte("owner content"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := WriteFeedLauncher(root, Manifest{Target: "darwin-arm64", Version: "0.0.1"}); err == nil {
		t.Fatal("overwrote unowned launcher")
	}
	actual, _ := os.ReadFile(file)
	if string(actual) != "owner content" {
		t.Fatal("changed owner file")
	}
	if runtime.GOOS != "windows" {
		if err := os.Remove(file); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(filepath.Join(root, "other"), file); err != nil {
			t.Fatal(err)
		}
		if err := WriteFeedLauncher(root, Manifest{Target: "darwin-arm64", Version: "0.0.1"}); err == nil {
			t.Fatal("accepted symlink")
		}
	}
}

func TestWindowsLauncherEscapesPercentAndDisablesDelayedExpansion(t *testing.T) {
	root := t.TempDir()
	r := Receipt{Home: root, Target: "windows-amd64", Installer: `C:\Developer\Pilot %x% ! &\installer.exe`, FeedRoot: `C:\Team\Feed %x% ! &`}
	if err := WriteLocalLauncher(r); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join(root, "Atualizar Maestro.cmd"))
	if err != nil {
		t.Fatal(err)
	}
	for _, piece := range []string{"setlocal DisableDelayedExpansion", `"C:\Developer\Pilot %%x%% ! &\installer.exe" update --feed "C:\Team\Feed %%x%% ! &"`} {
		if !bytes.Contains(content, []byte(piece)) {
			t.Fatalf("missing %q in %s", piece, content)
		}
	}
	for _, path := range []string{"bad\"path", "bad\npath", "bad\rpath"} {
		if _, err := batchQuote(path); err == nil {
			t.Fatal("accepted unsafe literal")
		}
	}
}

func TestGeneratedLauncherCanAdvanceWithoutReplacingUnownedFiles(t *testing.T) {
	root := t.TempDir()
	for _, version := range []string{"0.0.1", "0.0.2"} {
		if err := WriteFeedLauncher(root, Manifest{Target: "darwin-arm64", Version: version}); err != nil {
			t.Fatal(err)
		}
	}
	content, _ := os.ReadFile(filepath.Join(root, "Instalar Maestro.command"))
	if !bytes.Contains(content, []byte("/versions/0.0.2/installer")) {
		t.Fatal(string(content))
	}
}
