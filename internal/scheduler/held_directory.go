package scheduler

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// HeldDirectory reuses the private-file owner against an already retained root.
// Its relative names are application-internal, never a source of scope authority.
// In particular, no operation reopens the absolute root after admission.
type HeldDirectory struct {
	directory *secureDirectory
	path      string
}

func HoldDirectory(path string, expected *os.File) (*HeldDirectory, error) {
	d, err := openRetainedDirectory(path)
	if err != nil {
		return nil, err
	}
	if err = heldDirectoryOwned(d); err != nil {
		d.close()
		return nil, err
	}
	info, err := heldDirectoryStat(d)
	if err != nil {
		d.close()
		return nil, err
	}
	if expected != nil {
		before, e := expected.Stat()
		if e != nil || !os.SameFile(info, before) {
			d.close()
			return nil, errors.New("retained directory binding changed")
		}
	}
	return &HeldDirectory{directory: d, path: path}, nil
}
func (h *HeldDirectory) Path() string { return h.path }
func (h *HeldDirectory) Close() error { return h.directory.close() }
func (h *HeldDirectory) Relative(path string) (string, error) {
	r, e := filepath.Rel(h.path, path)
	if e != nil {
		return "", e
	}
	if _, e = heldParts(r); e != nil {
		return "", e
	}
	return r, nil
}
func heldParts(relative string) ([]string, error) {
	if relative == "" || relative == "." {
		return nil, nil
	}
	if filepath.IsAbs(relative) {
		return nil, errors.New("held path must be relative")
	}
	parts := strings.Split(filepath.ToSlash(relative), "/")
	for _, p := range parts {
		if p == "." || p == ".." || validateSecureLeaf(p) != nil {
			return nil, errors.New("invalid held relative path")
		}
	}
	return parts, nil
}
func (h *HeldDirectory) at(relative string, create bool) (*secureDirectory, func(), error) {
	parts, err := heldParts(relative)
	if err != nil {
		return nil, nil, err
	}
	d := h.directory
	owned := false
	for _, p := range parts {
		next, e := heldDirectoryChild(d, p, create)
		if owned {
			d.close()
		}
		if e != nil {
			return nil, nil, e
		}
		d = next
		owned = true
	}
	return d, func() {
		if owned {
			d.close()
		}
	}, nil
}
func (h *HeldDirectory) parent(relative string, create bool) (*secureDirectory, string, func(), error) {
	if _, e := heldParts(relative); e != nil {
		return nil, "", nil, e
	}
	name := filepath.Base(relative)
	if validateSecureLeaf(name) != nil || name == "." {
		return nil, "", nil, errors.New("held leaf required")
	}
	d, done, e := h.at(filepath.Dir(relative), create)
	return d, name, done, e
}
func (h *HeldDirectory) EnsureDirectory(relative string) error {
	_, done, e := h.at(relative, true)
	if e != nil {
		return e
	}
	done()
	return nil
}
func (h *HeldDirectory) Mkdir(relative string) error {
	d, n, done, e := h.parent(relative, false)
	if e != nil {
		return e
	}
	defer done()
	if e = heldDirectoryPrivate(d); e != nil {
		return e
	}
	return heldDirectoryMkdir(d, n)
}
func (h *HeldDirectory) StatDirectory(relative string) (os.FileInfo, error) {
	d, done, e := h.at(relative, false)
	if e != nil {
		return nil, e
	}
	defer done()
	return heldDirectoryStat(d)
}
func (h *HeldDirectory) ReadDir(relative string, maximum int) ([]os.DirEntry, error) {
	d, done, e := h.at(relative, false)
	if e != nil {
		return nil, e
	}
	defer done()
	return heldDirectoryRead(d, maximum)
}
func (h *HeldDirectory) OpenFile(relative string, flags int) (*os.File, error) {
	d, n, done, e := h.parent(relative, false)
	if e != nil {
		return nil, e
	}
	defer done()
	if flags&(os.O_WRONLY|os.O_RDWR|os.O_CREATE|os.O_TRUNC|os.O_APPEND) != 0 {
		if e = heldDirectoryPrivate(d); e != nil {
			return nil, e
		}
	}
	return d.openFile(n, flags, 0600)
}
func (h *HeldDirectory) ReadFile(relative string, maximum int64, private bool) ([]byte, error) {
	if maximum < 1 || maximum > 16<<20 {
		return nil, errors.New("held read bound invalid")
	}
	f, e := h.OpenFile(relative, os.O_RDONLY)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	if private {
		if e = ValidatePrivateFile(f, 0600); e != nil {
			return nil, e
		}
	} else if e = ValidateSingleLink(f); e != nil {
		return nil, e
	}
	body, e := io.ReadAll(io.LimitReader(f, maximum+1))
	if e != nil {
		return nil, e
	}
	if len(body) > int(maximum) || len(body) == 0 {
		return nil, errors.New("held file size invalid")
	}
	return body, nil
}

// ReadPrivateFile preserves the strict private-parent and private-leaf read
// contract while resolving all I/O through this retained directory capability.
// Unlike ReadFile, an empty private file is a valid bounded read.
func (h *HeldDirectory) ReadPrivateFile(relative string, maximum int64) ([]byte, error) {
	if maximum < 1 || maximum > 16<<20 {
		return nil, errors.New("held read bound invalid")
	}
	d, name, done, err := h.parent(relative, false)
	if err != nil {
		return nil, err
	}
	defer done()
	if err = heldDirectoryPrivate(d); err != nil {
		return nil, err
	}
	file, err := d.openFile(name, os.O_RDONLY, 0600)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	if err = ValidatePrivateFile(file, 0600); err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if info.Size() > maximum {
		return nil, errors.New("held file size invalid")
	}
	body, err := io.ReadAll(io.LimitReader(file, maximum+1))
	if err == nil && int64(len(body)) > maximum {
		return nil, errors.New("held file size invalid")
	}
	return body, err
}

func (h *HeldDirectory) UpdatePrivateFile(relative string, maximum int64, update func([]byte) ([]byte, error)) error {
	d, n, done, e := h.parent(relative, true)
	if e != nil {
		return e
	}
	defer done()
	if e = heldDirectoryPrivate(d); e != nil {
		return e
	}
	return updatePrivateFileInDirectory(d, n, maximum, nil, update, false)
}
func (h *HeldDirectory) RemoveExact(relative string, maximum int64, expected []byte, guard func(func() error) error) error {
	if len(expected) == 0 || guard == nil {
		return errors.New("exact preimage and live guard required")
	}
	d, n, done, e := h.parent(relative, false)
	if e != nil {
		return e
	}
	defer done()
	if e = heldDirectoryPrivate(d); e != nil {
		return e
	}
	return updatePrivateFileInDirectory(d, n, maximum, guard, func(current []byte) ([]byte, error) {
		if !bytes.Equal(current, expected) {
			return nil, errors.New("held removal preimage changed")
		}
		return nil, nil
	}, true)
}
func (h *HeldDirectory) PublishNewFile(relative string, body []byte) error {
	d, n, done, e := h.parent(relative, false)
	if e != nil {
		return e
	}
	defer done()
	if e = heldDirectoryPrivate(d); e != nil {
		return e
	}
	nonce := make([]byte, 12)
	if _, e = rand.Read(nonce); e != nil {
		return e
	}
	stage := ".held-publish-" + hex.EncodeToString(nonce)
	if e = d.writeNewFile(stage, body); e != nil {
		return e
	}
	defer d.removeFile(stage)
	if e = heldDirectoryRename(d, stage, d, n, false); e != nil {
		return e
	}
	return heldDirectorySync(d)
}
func (h *HeldDirectory) Rename(from, to string, replace bool) error {
	a, an, ad, e := h.parent(from, false)
	if e != nil {
		return e
	}
	defer ad()
	b, bn, bd, e := h.parent(to, false)
	if e != nil {
		return e
	}
	defer bd()
	if e = heldDirectoryPrivate(a); e != nil {
		return e
	}
	if e = heldDirectoryPrivate(b); e != nil {
		return e
	}
	if e = heldDirectoryRename(a, an, b, bn, replace); e != nil {
		return e
	}
	return errors.Join(heldDirectorySync(a), heldDirectorySync(b))
}
func (h *HeldDirectory) SyncDirectory(relative string) error {
	d, done, e := h.at(relative, false)
	if e != nil {
		return e
	}
	defer done()
	return heldDirectorySync(d)
}

// Remove is limited to an internal leaf or empty lock directory. It is not
// authored-content retirement and never traverses/removes a directory tree.
func (h *HeldDirectory) Remove(relative string) error {
	if _, e := heldParts(relative); e != nil || relative == "." || relative == "" {
		return errors.New("held removal requires relative leaf")
	}
	d, n, done, e := h.parent(relative, false)
	if e != nil {
		return e
	}
	defer done()
	if e = heldDirectoryPrivate(d); e != nil {
		return e
	}
	return heldDirectoryRemove(d, n)
}
func (h *HeldDirectory) WithPrivateLock(relative string, action func() error) error {
	if action == nil {
		return errors.New("private transition required")
	}
	d, n, done, e := h.parent(relative, true)
	if e != nil {
		return e
	}
	defer done()
	if e = heldDirectoryPrivate(d); e != nil {
		return e
	}
	guard, e := acquireLeaseGuard(d, n)
	if errors.Is(e, errLeaseGuardBusy) {
		return ErrLeaseBusy
	}
	if e != nil {
		return e
	}
	defer guard.release()
	return action()
}
