//go:build windows

package winprivate

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

const pinnedSharing = windows.FILE_SHARE_READ | windows.FILE_SHARE_WRITE

// SourceTree grants reads of ordinary local source assets, not private write
// authority. Keep it open while using returned files; Close releases custody.
type SourceTree struct{ directory *Directory }

func InspectPinnedTree(path string) (*SourceTree, error) {
	d, err := walk(path, false, false, false, nil, true)
	if err != nil {
		return nil, err
	}
	if err = validateLocalVolume(d.Handle()); err != nil {
		d.Close()
		return nil, err
	}
	return &SourceTree{directory: d}, nil
}

func (s *SourceTree) Close() error { return s.directory.Close() }
func (s *SourceTree) Path() string { return s.directory.File.Name() }
func (s *SourceTree) Stat() (os.FileInfo, error) {
	if err := validateType(s.directory.Handle(), true, false); err != nil {
		return nil, err
	}
	return s.directory.File.Stat()
}
func (s *SourceTree) ReadDir(maximum int) ([]os.DirEntry, error) {
	if err := validateType(s.directory.Handle(), true, false); err != nil {
		return nil, err
	}
	return s.directory.ReadDir(maximum)
}

func (s *SourceTree) Child(name string) (*SourceTree, error) {
	if err := Leaf(name); err != nil {
		return nil, err
	}
	if err := validateType(s.directory.Handle(), true, false); err != nil {
		return nil, err
	}
	pins, err := duplicatePins(s.directory)
	if err != nil {
		return nil, err
	}
	h, err := ntOpenShared(s.directory.Handle(), name, readDirAccess, windows.FILE_OPEN, dirOptions, pinnedSharing)
	if err == nil {
		err = validateType(h, true, false)
	}
	if err == nil {
		err = validateLocalVolume(h)
	}
	if err != nil {
		if h != windows.InvalidHandle {
			_ = windows.CloseHandle(h)
		}
		closePins(pins)
		return nil, err
	}
	d := &Directory{File: os.NewFile(uintptr(h), filepath.Join(s.Path(), name)), ancestors: pins, pinnedTree: true}
	return &SourceTree{directory: d}, nil
}

// OpenFile withholds write/delete sharing until the admitted source is closed.
// The caller supplies its domain byte bound when consuming this read-only file.
func (s *SourceTree) OpenFile(name string) (*os.File, error) {
	if err := Leaf(name); err != nil {
		return nil, err
	}
	if err := validateType(s.directory.Handle(), true, false); err != nil {
		return nil, err
	}
	h, err := ntOpenShared(s.directory.Handle(), name, windows.FILE_GENERIC_READ|windows.READ_CONTROL|windows.SYNCHRONIZE, windows.FILE_OPEN, fileOptions, windows.FILE_SHARE_READ)
	if err != nil {
		return nil, err
	}
	if err = validateType(h, false, true); err == nil {
		err = validateLocalVolume(h)
	}
	if err != nil {
		windows.CloseHandle(h)
		return nil, err
	}
	return os.NewFile(uintptr(h), filepath.Join(s.Path(), name)), nil
}

func closePins(pins []windows.Handle) {
	for i := len(pins) - 1; i >= 0; i-- {
		_ = windows.CloseHandle(pins[i])
	}
}

// Duplicating the held objects preserves custody without reopening a pathname.
func duplicatePins(d *Directory) ([]windows.Handle, error) {
	if d == nil || d.File == nil || !d.pinnedTree {
		return nil, errors.New("operation requires a held pinned directory tree")
	}
	var copies []windows.Handle
	for _, source := range append(append([]windows.Handle(nil), d.ancestors...), d.Handle()) {
		var copy windows.Handle
		process := windows.CurrentProcess()
		if err := windows.DuplicateHandle(process, source, process, &copy, 0, false, windows.DUPLICATE_SAME_ACCESS); err != nil {
			closePins(copies)
			return nil, err
		}
		copies = append(copies, copy)
	}
	return copies, nil
}

func clonePinnedDirectory(d *Directory) (*Directory, error) {
	if d == nil || d.File == nil {
		return nil, errors.New("held private directory is missing")
	}
	if err := Validate(d.Handle(), true); err != nil {
		return nil, err
	}
	pins, err := duplicatePins(d)
	if err != nil {
		return nil, err
	}
	last := len(pins) - 1
	return &Directory{File: os.NewFile(uintptr(pins[last]), d.File.Name()), ancestors: pins[:last], pinnedTree: true}, nil
}

// PinnedChild preserves the existing Child contract and additionally retains
// all ancestors and the child name for the lifetime of the returned directory.
func (d *Directory) PinnedChild(name string, create, exclusive bool) (*Directory, error) {
	if err := Leaf(name); err != nil {
		return nil, err
	}
	if err := Validate(d.Handle(), true); err != nil {
		return nil, err
	}
	pins, err := duplicatePins(d)
	if err != nil {
		return nil, err
	}
	disposition := uint32(windows.FILE_OPEN)
	if create {
		disposition = windows.FILE_OPEN_IF
	}
	if exclusive {
		disposition = windows.FILE_CREATE
	}
	h, err := ntOpenShared(d.Handle(), name, privateDirAccess, disposition, dirOptions|windows.FILE_WRITE_THROUGH, pinnedSharing)
	if err == nil {
		err = Validate(h, true)
	}
	if err != nil {
		if h != windows.InvalidHandle {
			windows.CloseHandle(h)
		}
		closePins(pins)
		return nil, err
	}
	child := &Directory{File: os.NewFile(uintptr(h), filepath.Join(d.File.Name(), name)), ancestors: pins, pinnedTree: true}
	if create {
		if err := d.Sync(); err != nil {
			child.Close()
			return nil, err
		}
	}
	return child, nil
}

// Stage retains a deterministic private file and its parent for domain-owned
// resumption. Closing an unpublished stage never deletes its partial bytes.
type Stage struct {
	*os.File
	parent    *Directory
	published bool
}

func (d *Directory) OpenStage(name string) (*Stage, error) {
	if err := Leaf(name); err != nil {
		return nil, err
	}
	parent, err := clonePinnedDirectory(d)
	if err != nil {
		return nil, err
	}
	access := uint32(windows.FILE_GENERIC_READ | windows.FILE_GENERIC_WRITE | windows.DELETE | windows.READ_CONTROL)
	h, err := ntOpenShared(parent.Handle(), name, access, windows.FILE_OPEN_IF, fileOptions|windows.FILE_WRITE_THROUGH, windows.FILE_SHARE_READ)
	if err == nil {
		err = Validate(h, false)
	}
	if err != nil {
		if h != windows.InvalidHandle {
			windows.CloseHandle(h)
		}
		parent.Close()
		return nil, err
	}
	stage := &Stage{File: os.NewFile(uintptr(h), filepath.Join(parent.File.Name(), name)), parent: parent}
	if err := parent.Sync(); err != nil {
		stage.Close()
		return nil, err
	}
	return stage, nil
}

func (s *Stage) Close() error {
	if s == nil || s.File == nil {
		return nil
	}
	err := errors.Join(s.File.Close(), s.parent.Close())
	s.File, s.parent = nil, nil
	return err
}

func relabelHeldFile(file *os.File, name string) (*os.File, error) {
	var handle windows.Handle
	process := windows.CurrentProcess()
	if err := windows.DuplicateHandle(process, windows.Handle(file.Fd()), process, &handle, 0, false, windows.DUPLICATE_SAME_ACCESS); err != nil {
		return file, err
	}
	next := os.NewFile(uintptr(handle), name)
	return next, file.Close()
}

// Publish returns moved=true once the namespace changed, even if a later flush
// fails. Domain recovery must preserve that complete object rather than recopy.
func (s *Stage) Publish(target *Directory, name string) (moved bool, err error) {
	return s.publish(target, name, false)
}

// PublishReplacingUnderLease atomically replaces existing cooperative private
// metadata with this same held stage. The caller must hold its exclusive profile
// migration lease, validate the retired archive authority, and compare the prior
// record bytes before calling. This is not an expected-object CAS: the target's
// admission view closes before rename. Initial publication must use Publish.
func (s *Stage) PublishReplacingUnderLease(target *Directory, name string) (moved bool, err error) {
	return s.publish(target, name, true)
}

func (s *Stage) publish(target *Directory, name string, replace bool) (moved bool, err error) {
	if s == nil || s.File == nil || s.published {
		return false, errors.New("stage is closed or already published")
	}
	if err := Leaf(name); err != nil {
		return false, err
	}
	nextParent, err := clonePinnedDirectory(target)
	if err != nil {
		return false, err
	}
	if err := Validate(windows.Handle(s.File.Fd()), false); err != nil {
		nextParent.Close()
		return false, err
	}
	if replace {
		old, err := nextParent.OpenFile(name, os.O_RDONLY)
		if err != nil {
			return false, errors.Join(err, nextParent.Close())
		}
		var attributes windows.ByHandleFileInformation
		err = windows.GetFileInformationByHandle(windows.Handle(old.Fd()), &attributes)
		if err == nil && attributes.FileAttributes&windows.FILE_ATTRIBUTE_READONLY != 0 {
			err = errors.New("leased publication refuses a read-only destination")
		}
		err = errors.Join(err, old.Close())
		if err != nil {
			return false, errors.Join(err, nextParent.Close())
		}
	}
	if err := s.File.Sync(); err != nil {
		nextParent.Close()
		return false, err
	}
	moved, err = renameHandle(s.File, nextParent, name, replace)
	if !moved {
		return false, errors.Join(err, nextParent.Close())
	}
	s.published = true
	oldParent := s.parent
	s.parent = nextParent
	var labelErr error
	s.File, labelErr = relabelHeldFile(s.File, filepath.Join(nextParent.File.Name(), name))
	return true, errors.Join(err, labelErr, oldParent.Sync(), oldParent.Close())
}

// MovableDirectory owns the rename-capable root and pinned private parent.
// Other independently opened no-DELETE-sharing root handles must be closed
// before admission; failure never falls back to reopening a lexical name.
type MovableDirectory struct {
	directory *Directory
	parent    *Directory
	name      string
}

func OpenMovableDirectory(path string) (*MovableDirectory, error) {
	if _, _, err := parts(path); err != nil {
		return nil, err
	}
	name := filepath.Base(path)
	if err := Leaf(name); err != nil {
		return nil, err
	}
	parent, err := OpenPinnedTree(filepath.Dir(path))
	if err != nil {
		return nil, err
	}
	pins, err := duplicatePins(parent)
	if err != nil {
		parent.Close()
		return nil, err
	}
	h, err := ntOpenShared(parent.Handle(), name, privateDirAccess|windows.DELETE, windows.FILE_OPEN, dirOptions|windows.FILE_WRITE_THROUGH, pinnedSharing)
	if err == nil {
		err = Validate(h, true)
	}
	if err != nil {
		if h != windows.InvalidHandle {
			windows.CloseHandle(h)
		}
		closePins(pins)
		parent.Close()
		return nil, err
	}
	d := &Directory{File: os.NewFile(uintptr(h), path), ancestors: pins, pinnedTree: true}
	return &MovableDirectory{directory: d, parent: parent, name: name}, nil
}

func (m *MovableDirectory) Directory() *Directory { return m.directory }
func (m *MovableDirectory) Path() string          { return filepath.Join(m.parent.File.Name(), m.name) }
func (m *MovableDirectory) Close() error          { return errors.Join(m.directory.Close(), m.parent.Close()) }

// MoveTo is intentionally same-parent and exclusive: rollback cannot replace an
// occupied archive. The held root remains usable for receipts after transfer.
func (m *MovableDirectory) MoveTo(name string) (moved bool, err error) {
	if err := Leaf(name); err != nil {
		return false, err
	}
	if strings.EqualFold(m.name, name) {
		return false, errors.New("archive must differ from the current root")
	}
	moved, err = renameHandle(m.directory.File, m.parent, name, false)
	if !moved {
		return false, err
	}
	m.name = name
	var labelErr error
	m.directory.File, labelErr = relabelHeldFile(m.directory.File, m.Path())
	return true, errors.Join(err, labelErr)
}

const exactRemovalMaximum = 16 << 20

// RemoveExact retires only the complete expected metadata in the admitted leaf.
// marked reports a successful delete disposition, not immediate physical absence:
// independent sharing readers may keep the object/name pending until they close.
func (d *Directory) RemoveExact(name string, expected []byte) (marked bool, err error) {
	return d.removeExact(name, expected, nil)
}

// afterMark is a private deny-only test seam; it cannot bypass admission or the
// exact comparison, and production callers never supply it.
func (d *Directory) removeExact(name string, expected []byte, afterMark func(*os.File) error) (marked bool, err error) {
	if len(expected) > exactRemovalMaximum {
		return false, errors.New("exact removal expected bytes exceed the metadata bound")
	}
	if err := Leaf(name); err != nil {
		return false, err
	}
	parent, err := clonePinnedDirectory(d)
	if err != nil {
		return false, err
	}
	defer func() { err = errors.Join(err, parent.Close()) }()
	access := uint32(windows.FILE_GENERIC_READ | windows.DELETE | windows.READ_CONTROL | windows.SYNCHRONIZE)
	h, err := ntOpenShared(parent.Handle(), name, access, windows.FILE_OPEN, fileOptions, windows.FILE_SHARE_READ)
	if err != nil {
		return false, err
	}
	f := os.NewFile(uintptr(h), filepath.Join(parent.File.Name(), name))
	defer func() {
		err = errors.Join(err, f.Close())
		if marked {
			err = errors.Join(err, parent.Sync())
		}
	}()
	if err := Validate(h, false); err != nil {
		return false, err
	}
	var attributes windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(h, &attributes); err != nil {
		return false, err
	}
	if attributes.FileAttributes&windows.FILE_ATTRIBUTE_READONLY != 0 {
		return false, errors.New("exact removal refuses a read-only leaf")
	}
	info, err := f.Stat()
	if err != nil {
		return false, err
	}
	if info.Size() != int64(len(expected)) {
		return false, errors.New("exact removal bytes differ from the expected metadata")
	}
	body, err := io.ReadAll(io.LimitReader(f, int64(len(expected))+1))
	if err != nil {
		return false, err
	}
	if !bytes.Equal(body, expected) {
		return false, errors.New("exact removal bytes differ from the expected metadata")
	}
	if err := markDelete(h); err != nil {
		return false, err
	}
	marked = true
	if afterMark != nil {
		err = afterMark(f)
	}
	return marked, err
}
