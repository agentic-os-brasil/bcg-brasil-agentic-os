package processidentity

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"
)

func TestCurrentIsTheStartupExecutable(t *testing.T) {
	identity, err := Current()
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(identity.Path)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(body)
	if identity.SHA256 != hex.EncodeToString(sum[:]) {
		t.Fatal("startup digest differs from actual executable")
	}
}

func TestPinnedFileNeverAdoptsAReplacement(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix atomic replacement contract; Windows held-handle contract is tested separately")
	}
	name := filepath.Join(t.TempDir(), "executable")
	if err := os.WriteFile(name, []byte("original image"), 0700); err != nil {
		t.Fatal(err)
	}
	p := pinExecutable(name)
	if p.file == nil {
		t.Fatal("fixture pin unavailable")
	}
	defer p.file.Close()
	if _, err := p.current(); err != nil {
		t.Fatal(err)
	}
	replacement := name + ".new"
	if err := os.Link(name, name+".original"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(replacement, []byte("replacement image"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(replacement, name); err != nil {
		t.Fatal(err)
	}
	if identity, err := p.current(); !errors.Is(err, ErrChanged) || identity != (Identity{}) {
		t.Fatal("replacement supplied a running identity")
	}
	if err := os.Rename(name+".original", name); err != nil {
		t.Fatal(err)
	}
	if _, err := p.current(); !errors.Is(err, ErrChanged) {
		t.Fatal("restoring old bytes revived a process requiring restart")
	}
}

func TestPinnedWindowsFileBlocksReplacement(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows held-handle sharing contract")
	}
	name := filepath.Join(t.TempDir(), "executable")
	const original = "original image"
	if err := os.WriteFile(name, []byte(original), 0700); err != nil {
		t.Fatal(err)
	}
	p := pinExecutable(name)
	if p.file == nil {
		t.Fatal("fixture pin unavailable")
	}
	defer p.file.Close()
	before, err := p.current()
	if err != nil {
		t.Fatal(err)
	}
	replacement := name + ".new"
	if err := os.WriteFile(replacement, []byte("replacement image"), 0700); err != nil {
		t.Fatal(err)
	}
	// Go's ordinary Windows open shares reads/writes, not deletion. The held
	// image cannot be atomically replaced; its original identity remains true.
	if err := os.Rename(replacement, name); err == nil {
		t.Fatal("Windows replaced an image held without delete sharing")
	}
	after, err := p.current()
	if err != nil || after != before {
		t.Fatalf("blocked replacement changed running identity: %+v %v", after, err)
	}
	body, err := os.ReadFile(name)
	if err != nil || string(body) != original {
		t.Fatal("blocked replacement changed original bytes")
	}
}

func identityTestSymlink(t *testing.T, target, name string) {
	t.Helper()
	if err := os.Symlink(target, name); err != nil {
		// ERROR_PRIVILEGE_NOT_HELD (1314) is returned without Developer Mode
		// or symlink privilege; only this Windows capability absence may skip.
		if runtime.GOOS == "windows" && (errors.Is(err, syscall.Errno(1314)) || errors.Is(err, os.ErrPermission)) {
			t.Skip("Windows symlink privilege unavailable: " + err.Error())
		}
		t.Fatal(err)
	}
}

func TestPinnedFileRejectsSymlinkRetargetAndInPlaceChange(t *testing.T) {
	for _, mode := range []string{"symlink", "in-place"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			name := filepath.Join(root, "image")
			if err := os.WriteFile(name, []byte("original image"), 0700); err != nil {
				t.Fatal(err)
			}
			hint := name
			if mode == "symlink" {
				hint = filepath.Join(root, "current")
				identityTestSymlink(t, name, hint)
			}
			p := pinExecutable(hint)
			if p.file == nil {
				t.Fatal("fixture pin unavailable")
			}
			defer p.file.Close()
			if _, err := p.current(); err != nil {
				t.Fatal(err)
			}
			if mode == "symlink" {
				other := filepath.Join(root, "other")
				if err := os.WriteFile(other, []byte("other"), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.Remove(hint); err != nil {
					t.Fatal(err)
				}
				identityTestSymlink(t, other, hint)
			} else if err := os.WriteFile(name, []byte("changed longer image"), 0700); err != nil {
				t.Fatal(err)
			}
			if _, err := p.current(); !errors.Is(err, ErrChanged) {
				t.Fatal("changed file remained current")
			}
		})
	}
}
