// Package processidentity pins one executable at process startup. The initial
// pathname is assumed to identify the loaded image during package initialization;
// this is a bounded file identity contract, not kernel image attestation.
package processidentity

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
)

var ErrUnavailable = errors.New("running executable identity unavailable")
var ErrChanged = errors.New("running executable changed; restart required")

const maximumExecutableBytes = 1 << 30

type Identity struct {
	Path   string
	SHA256 string
}

type executablePin struct {
	hint     string
	identity Identity
	file     *os.File
	info     os.FileInfo
	err      error
	status   *pinStatus
}

type pinStatus struct {
	mu      sync.Mutex
	changed bool
}

// Initialization precedes main and all product effects, including the first
// maintenance operation. A lazy first-job cache would adopt a replaced path.
var running = startupPin()

func startupPin() executablePin {
	name, err := os.Executable()
	if err != nil {
		return executablePin{err: ErrUnavailable}
	}
	return pinExecutable(name)
}

func pinExecutable(name string) executablePin {
	p := executablePin{hint: name, err: ErrUnavailable, status: &pinStatus{}}
	resolved, err := filepath.EvalSymlinks(name)
	if err != nil || !filepath.IsAbs(resolved) {
		return p
	}
	f, err := os.Open(resolved)
	if err != nil {
		return p
	}
	accepted := false
	defer func() {
		if !accepted {
			f.Close()
		}
	}()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() < 1 || info.Size() > maximumExecutableBytes {
		return p
	}
	h := sha256.New()
	n, err := io.Copy(h, io.LimitReader(f, maximumExecutableBytes+1))
	if err != nil || n != info.Size() {
		return p
	}
	p.identity = Identity{Path: resolved, SHA256: hex.EncodeToString(h.Sum(nil))}
	p.file, p.info, p.err = f, info, nil
	if _, err := p.current(); err != nil {
		p.file, p.info, p.err = nil, nil, ErrUnavailable
		return p
	}
	accepted = true // One bounded read-only descriptor lives with this process.
	return p
}

func sameFileVersion(left, right os.FileInfo) bool {
	return right != nil && right.Mode().IsRegular() && os.SameFile(left, right) && left.Size() == right.Size() && left.ModTime().Equal(right.ModTime()) && left.Mode() == right.Mode()
}

func (p executablePin) current() (Identity, error) {
	if p.err != nil || p.file == nil || p.info == nil || p.status == nil {
		return Identity{}, ErrUnavailable
	}
	p.status.mu.Lock()
	defer p.status.mu.Unlock()
	if p.status.changed {
		return Identity{}, ErrChanged
	}
	resolved, err := filepath.EvalSymlinks(p.hint)
	if err != nil || resolved != p.identity.Path {
		p.status.changed = true
		return Identity{}, ErrChanged
	}
	pathInfo, err := os.Stat(resolved)
	if err != nil || !sameFileVersion(p.info, pathInfo) {
		p.status.changed = true
		return Identity{}, ErrChanged
	}
	heldInfo, err := p.file.Stat()
	if err != nil || !sameFileVersion(p.info, heldInfo) {
		p.status.changed = true
		return Identity{}, ErrChanged
	}
	return p.identity, nil
}

// Current returns the startup fingerprint only while its canonical pathname and
// held regular-file version remain unchanged. Replacement requires a new process;
// it never refreshes the fingerprint to the newly installed bytes.
func Current() (Identity, error) { return running.current() }
