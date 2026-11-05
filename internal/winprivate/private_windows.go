//go:build windows

// Package winprivate implements the finite Windows private-state operations used
// by the scheduler, fresh HOME bootstrap, work ledger, memory and owner context.
// Every operation uses a held native directory; paths are diagnostic labels only
// after admission. It introduces no domain authority or transaction policy.
package winprivate

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Directory opens use the native directory option set; OPEN_NO_RECALL is a
// file option. Open reparse objects themselves, then refuse their held attributes.
const dirOptions = windows.FILE_DIRECTORY_FILE | windows.FILE_SYNCHRONOUS_IO_NONALERT | windows.FILE_OPEN_REPARSE_POINT
const fileOptions = windows.FILE_NON_DIRECTORY_FILE | windows.FILE_SYNCHRONOUS_IO_NONALERT | windows.FILE_OPEN_NO_RECALL
const readDirAccess = windows.FILE_GENERIC_READ | windows.FILE_TRAVERSE | windows.READ_CONTROL
const privateDirAccess = readDirAccess | windows.FILE_GENERIC_WRITE
const unsafeAttributes = windows.FILE_ATTRIBUTE_REPARSE_POINT | windows.FILE_ATTRIBUTE_OFFLINE | 0x00040000 | 0x00400000

// Directory owns one no-follow native directory handle. Close releases it.
type Directory struct{ File *os.File }

func CurrentUser() (*windows.SID, error) {
	u, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return nil, err
	}
	if u == nil || u.User.Sid == nil || !u.User.Sid.IsValid() {
		return nil, errors.New("Windows process owner SID unavailable")
	}
	return u.User.Sid, nil
}

// Validate applies the existing finite owner/DACL policy to the held object.
func Validate(h windows.Handle, directory bool) error {
	if err := validateType(h, directory, !directory); err != nil {
		return err
	}
	if err := validateLocalVolume(h); err != nil {
		return err
	}
	sd, err := windows.GetSecurityInfo(h, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return fmt.Errorf("inspect held private security descriptor: %w", err)
	}
	if sd == nil || !sd.IsValid() {
		return errors.New("private object security descriptor is invalid")
	}
	owner, _, err := sd.Owner()
	if err != nil || owner == nil || !owner.IsValid() {
		return errors.New("private object owner is invalid")
	}
	user, err := CurrentUser()
	if err != nil {
		return err
	}
	if !owner.Equals(user) {
		return errors.New("private object belongs to a different Windows owner")
	}
	acl, _, err := sd.DACL()
	if err != nil || acl == nil {
		return errors.New("private object has no valid non-null DACL")
	}
	for i := uint16(0); i < acl.AceCount; i++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(acl, uint32(i), &ace); err != nil {
			return err
		}
		if ace == nil {
			return errors.New("private object contains an empty ACE")
		}
		if ace.Header.AceType == windows.ACCESS_DENIED_ACE_TYPE || ace.Header.AceFlags&windows.INHERIT_ONLY_ACE != 0 {
			continue
		}
		if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE {
			return errors.New("private object contains an unsupported effective ACE")
		}
		if ace.Mask == 0 {
			continue
		}
		sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		if !sid.IsValid() || (!sid.Equals(user) && !sid.IsWellKnown(windows.WinLocalSystemSid) && !sid.IsWellKnown(windows.WinBuiltinAdministratorsSid)) {
			return errors.New("private object grants access to an unapproved Windows principal")
		}
	}
	return nil
}

func validateType(h windows.Handle, directory, single bool) error {
	kind, err := windows.GetFileType(h)
	if err != nil || kind != windows.FILE_TYPE_DISK {
		return errors.New("private object must be a local disk object")
	}
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(h, &info); err != nil {
		return err
	}
	if info.FileAttributes&unsafeAttributes != 0 {
		return errors.New("private object is a reparse, offline or recall object")
	}
	if (info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0) != directory {
		return errors.New("private object has the wrong filesystem type")
	}
	if single && info.NumberOfLinks != 1 {
		return errors.New("private file must have exactly one link")
	}
	return nil
}

func descriptor() (*windows.SECURITY_DESCRIPTOR, error) {
	sid, err := CurrentUser()
	if err != nil {
		return nil, err
	}
	return windows.SecurityDescriptorFromString("O:" + sid.String() + "D:P(A;OICI;FA;;;" + sid.String() + ")(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)")
}

func ntOpen(parent windows.Handle, name string, access, disposition, options uint32) (windows.Handle, error) {
	n, err := windows.NewNTUnicodeString(name)
	if err != nil {
		return windows.InvalidHandle, err
	}
	oa := windows.OBJECT_ATTRIBUTES{RootDirectory: parent, ObjectName: n, Attributes: windows.OBJ_CASE_INSENSITIVE | windows.OBJ_DONT_REPARSE}
	oa.Length = uint32(unsafe.Sizeof(oa))
	if disposition != windows.FILE_OPEN {
		oa.SecurityDescriptor, err = descriptor()
		if err != nil {
			return windows.InvalidHandle, err
		}
	}
	var h windows.Handle
	var iosb windows.IO_STATUS_BLOCK
	var allocation int64
	err = windows.NtCreateFile(&h, access, &oa, &iosb, &allocation, 0, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, disposition, options, 0, 0)
	runtime.KeepAlive(oa.SecurityDescriptor)
	if err != nil {
		if status, ok := err.(windows.NTStatus); ok {
			err = status.Errno()
		}
		return windows.InvalidHandle, &os.PathError{Op: "NtCreateFile", Path: name, Err: err}
	}
	return h, nil
}

var queryVolume = windows.NewLazySystemDLL("ntdll.dll").NewProc("NtQueryVolumeInformationFile")

// Drive letters and FILE_TYPE_DISK also describe mapped network drives. The
// held object's kernel volume facts, rather than its spelling, admit locality.
func validateLocalVolume(h windows.Handle) error {
	if err := queryVolume.Find(); err != nil {
		return err
	}
	var information struct {
		DeviceType      uint32
		Characteristics uint32
	}
	var iosb windows.IO_STATUS_BLOCK
	const fileFsDeviceInformation = 4
	const fileDeviceDisk = 7
	const fileRemoteDevice = 0x10
	status, _, _ := queryVolume.Call(uintptr(h), uintptr(unsafe.Pointer(&iosb)), uintptr(unsafe.Pointer(&information)), unsafe.Sizeof(information), fileFsDeviceInformation)
	if status != 0 {
		return fmt.Errorf("inspect held private volume: %w", windows.NTStatus(uint32(status)).Errno())
	}
	if iosb.Information != unsafe.Sizeof(information) {
		return errors.New("held private volume facts are incomplete")
	}
	if information.DeviceType != fileDeviceDisk || information.Characteristics&fileRemoteDevice != 0 {
		return errors.New("private state requires a local disk volume")
	}
	return nil
}

func Leaf(name string) error {
	if name == "" || name == "." || name == ".." || filepath.Base(name) != name || strings.ContainsAny(name, "\\/:\x00\r\n") || strings.TrimRight(name, " .") != name {
		return errors.New("invalid private Windows leaf")
	}
	base := strings.ToUpper(strings.SplitN(name, ".", 2)[0])
	if base == "CON" || base == "PRN" || base == "AUX" || base == "NUL" || len(base) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) && base[3] >= '1' && base[3] <= '9' {
		return errors.New("reserved Windows leaf")
	}
	return nil
}

func parts(path string) (string, []string, error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return "", nil, errors.New("private Windows path must be clean and absolute")
	}
	vol := filepath.VolumeName(path)
	if len(vol) != 2 || vol[1] != ':' {
		return "", nil, errors.New("private Windows path must use a local drive")
	}
	tail := strings.TrimLeft(strings.TrimPrefix(path, vol), "\\/")
	var names []string
	if tail != "" {
		names = strings.Split(tail, "\\")
	}
	for _, name := range names {
		if err := Leaf(name); err != nil {
			return "", nil, err
		}
	}
	return vol + "\\", names, nil
}

// Inspect traverses existing directories without granting private ownership.
// It is for ordinary ancestors and explicitly unowned read-only source APIs.
func Inspect(path string) (*Directory, error) { return walk(path, false, false, false, nil) }

// Open admits a private directory, creating missing components with private ACLs.
func Open(path string, create bool) (*Directory, error) { return walk(path, create, true, false, nil) }

// Fresh refuses a concurrent winner when a missing bootstrap component is created.
func Fresh(path string) (*Directory, error) { return walk(path, true, true, true, nil) }

// OpenSteps preserves the scheduler's existing adversarial path-walk seam.
func OpenSteps(path string, create bool, step func(string)) (*Directory, error) {
	return walk(path, create, true, false, step)
}

func walk(path string, create, private, fresh bool, step func(string)) (*Directory, error) {
	volume, names, err := parts(path)
	if err != nil {
		return nil, err
	}
	h, err := ntOpen(0, "\\??\\"+volume, readDirAccess, windows.FILE_OPEN, dirOptions)
	if err != nil {
		return nil, err
	}
	writable := false
	for i, name := range names {
		// Request mutation rights at the original relative open. Ordinary ancestors
		// may be traversable but not writable; their read-only handles grant no
		// creation authority. A private parent is never reopened by pathname.
		access := uint32(readDirAccess)
		wantWrite := create || (i == len(names)-1 && private)
		if wantWrite {
			access = privateDirAccess
		}
		next, e := ntOpen(h, name, access, windows.FILE_OPEN, dirOptions)
		nextWritable := wantWrite
		if create && i < len(names)-1 && errors.Is(e, windows.ERROR_ACCESS_DENIED) {
			next, e = ntOpen(h, name, readDirAccess, windows.FILE_OPEN, dirOptions)
			nextWritable = false
		}
		if create && errors.Is(e, os.ErrNotExist) {
			if !writable {
				windows.CloseHandle(h)
				return nil, errors.New("private creation requires write access on the held parent")
			}
			if err = Validate(h, true); err != nil {
				windows.CloseHandle(h)
				return nil, err
			}
			disposition := uint32(windows.FILE_OPEN_IF)
			if fresh {
				disposition = windows.FILE_CREATE
			}
			next, e = ntOpen(h, name, privateDirAccess, disposition, dirOptions|windows.FILE_WRITE_THROUGH)
			nextWritable = true
			if e == nil {
				e = Validate(next, true)
			}
			if e == nil {
				e = windows.FlushFileBuffers(h)
			}
		}
		if e == nil {
			e = validateType(next, true, false)
		}
		windows.CloseHandle(h)
		if e != nil {
			if next != windows.InvalidHandle {
				windows.CloseHandle(next)
			}
			return nil, e
		}
		h = next
		writable = nextWritable
		if step != nil {
			step(name)
		}
	}
	if private {
		if err = Validate(h, true); err != nil {
			windows.CloseHandle(h)
			return nil, err
		}
	}
	return &Directory{File: os.NewFile(uintptr(h), path)}, nil
}

func (d *Directory) Close() error {
	if d == nil || d.File == nil {
		return nil
	}
	return d.File.Close()
}
func (d *Directory) Handle() windows.Handle { return windows.Handle(d.File.Fd()) }
func (d *Directory) Sync() error {
	if err := Validate(d.Handle(), true); err != nil {
		return err
	}
	if err := windows.FlushFileBuffers(d.Handle()); err != nil {
		return fmt.Errorf("flush held private directory: %w", err)
	}
	return nil
}
func Sync(path string) error {
	d, e := Open(path, false)
	if e != nil {
		return e
	}
	defer d.Close()
	return d.Sync()
}

func (d *Directory) Child(name string, create, exclusive bool) (*Directory, error) {
	if err := Leaf(name); err != nil {
		return nil, err
	}
	if err := Validate(d.Handle(), true); err != nil {
		return nil, err
	}
	disposition := uint32(windows.FILE_OPEN)
	if create {
		disposition = windows.FILE_OPEN_IF
	}
	if exclusive {
		disposition = windows.FILE_CREATE
	}
	h, err := ntOpen(d.Handle(), name, privateDirAccess, disposition, dirOptions|windows.FILE_WRITE_THROUGH)
	if err != nil {
		return nil, err
	}
	if err = Validate(h, true); err != nil {
		windows.CloseHandle(h)
		return nil, err
	}
	child := &Directory{File: os.NewFile(uintptr(h), filepath.Join(d.File.Name(), name))}
	if create {
		if err = d.Sync(); err != nil {
			child.Close()
			return nil, err
		}
	}
	return child, nil
}

func (d *Directory) OpenFile(name string, flags int) (*os.File, error) {
	return d.openFile(name, flags, true)
}
func (d *Directory) openFile(name string, flags int, private bool) (*os.File, error) {
	if err := Leaf(name); err != nil {
		return nil, err
	}
	if private {
		if err := Validate(d.Handle(), true); err != nil {
			return nil, err
		}
	}
	access := uint32(windows.FILE_GENERIC_READ)
	if flags&os.O_RDWR != 0 {
		access |= windows.FILE_GENERIC_WRITE
	} else if flags&os.O_WRONLY != 0 {
		access = windows.FILE_GENERIC_WRITE | windows.FILE_READ_ATTRIBUTES | windows.READ_CONTROL
	}
	if flags&os.O_APPEND != 0 {
		access &^= windows.FILE_WRITE_DATA
		access |= windows.FILE_APPEND_DATA
	}
	access |= windows.READ_CONTROL | windows.SYNCHRONIZE
	if flags&os.O_CREATE != 0 {
		access |= windows.DELETE
	}
	disposition := uint32(windows.FILE_OPEN)
	if flags&os.O_CREATE != 0 {
		disposition = windows.FILE_OPEN_IF
		if flags&os.O_EXCL != 0 {
			disposition = windows.FILE_CREATE
		}
	}
	options := uint32(fileOptions)
	if flags&(os.O_WRONLY|os.O_RDWR) != 0 {
		options |= windows.FILE_WRITE_THROUGH
	}
	h, err := ntOpen(d.Handle(), name, access, disposition, options)
	if err != nil {
		return nil, err
	}
	if private {
		err = Validate(h, false)
	} else {
		err = validateType(h, false, false)
	}
	if err != nil {
		windows.CloseHandle(h)
		return nil, err
	}
	f := os.NewFile(uintptr(h), filepath.Join(d.File.Name(), name))
	// Never request FILE_OVERWRITE: validate custody before the first truncation.
	if flags&os.O_TRUNC != 0 {
		if err = f.Truncate(0); err != nil {
			f.Close()
			return nil, err
		}
	}
	return f, nil
}

func OpenFile(path string, flags int) (*os.File, error) {
	d, err := Open(filepath.Dir(path), false)
	if err != nil {
		return nil, err
	}
	defer d.Close()
	return d.OpenFile(filepath.Base(path), flags)
}
func OpenRegular(path string) (*os.File, error) {
	d, err := Inspect(filepath.Dir(path))
	if err != nil {
		return nil, err
	}
	defer d.Close()
	return d.openFile(filepath.Base(path), os.O_RDONLY, false)
}
func ReadFile(path string, maximum int64) ([]byte, error) {
	f, err := OpenFile(path, os.O_RDONLY)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return Read(f, maximum)
}
func Read(f *os.File, maximum int64) ([]byte, error) {
	if maximum <= 0 {
		return nil, errors.New("private read bound must be positive")
	}
	if err := Validate(windows.Handle(f.Fd()), false); err != nil {
		return nil, err
	}
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if st.Size() > maximum {
		return nil, errors.New("private read exceeds byte bound")
	}
	b, err := io.ReadAll(io.LimitReader(f, maximum+1))
	if err == nil && int64(len(b)) > maximum {
		err = errors.New("private read exceeds byte bound")
	}
	return b, err
}

func (d *Directory) ReadDir(maximum int) ([]os.DirEntry, error) {
	if maximum < 1 {
		return nil, errors.New("directory entry bound must be positive")
	}
	// Duplicate the held capability; enumeration never reopens its pathname.
	var h windows.Handle
	process := windows.CurrentProcess()
	if err := windows.DuplicateHandle(process, d.Handle(), process, &h, 0, false, windows.DUPLICATE_SAME_ACCESS); err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(h), d.File.Name())
	defer f.Close()
	entries, err := f.ReadDir(maximum + 1)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	if len(entries) > maximum {
		return nil, errors.New("private directory exceeds entry bound")
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	return entries, nil
}
func ReadDir(path string) ([]os.DirEntry, error) {
	d, e := Open(path, false)
	if e != nil {
		return nil, e
	}
	defer d.Close()
	return d.ReadDir(4096)
}
func MkdirAll(path string) error {
	d, e := Open(path, true)
	if e != nil {
		return e
	}
	defer d.Close()
	return d.Sync()
}
func Mkdir(path string) error {
	d, e := Open(filepath.Dir(path), false)
	if e != nil {
		return e
	}
	defer d.Close()
	c, e := d.Child(filepath.Base(path), true, true)
	if e != nil {
		return e
	}
	return c.Close()
}
func Random() string {
	var b [16]byte
	if _, e := rand.Read(b[:]); e != nil {
		panic(e)
	}
	return hex.EncodeToString(b[:])
}
func MkdirTemp(path, prefix string) (string, error) {
	name := filepath.Join(path, prefix+Random())
	return name, Mkdir(name)
}
func StatDirectory(path string) (os.FileInfo, error) {
	d, e := Open(path, false)
	if e != nil {
		return nil, e
	}
	defer d.Close()
	return d.File.Stat()
}

func markDelete(h windows.Handle) error {
	b := byte(1)
	var iosb windows.IO_STATUS_BLOCK
	e := windows.NtSetInformationFile(h, &iosb, &b, 1, windows.FileDispositionInformation)
	if s, ok := e.(windows.NTStatus); ok {
		return s.Errno()
	}
	return e
}
func (d *Directory) Remove(name string, recursive bool) error {
	if err := Leaf(name); err != nil {
		return err
	}
	if err := Validate(d.Handle(), true); err != nil {
		return err
	}
	h, err := ntOpen(d.Handle(), name, windows.DELETE|windows.FILE_GENERIC_READ|windows.FILE_GENERIC_WRITE|windows.READ_CONTROL, windows.FILE_OPEN, windows.FILE_SYNCHRONOUS_IO_NONALERT|windows.FILE_OPEN_NO_RECALL)
	if err != nil {
		return err
	}
	f := os.NewFile(uintptr(h), filepath.Join(d.File.Name(), name))
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return err
	}
	if err = Validate(h, st.IsDir()); err != nil {
		return err
	}
	if st.IsDir() && recursive {
		child := &Directory{File: f}
		entries, e := child.ReadDir(4096)
		if e != nil {
			return e
		}
		for _, entry := range entries {
			if e = child.Remove(entry.Name(), true); e != nil {
				return e
			}
		}
	}
	if err = markDelete(h); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return d.Sync()
}
func Remove(path string, recursive bool) error {
	d, e := Open(filepath.Dir(path), false)
	if errors.Is(e, os.ErrNotExist) && recursive {
		return nil
	}
	if e != nil {
		return e
	}
	defer d.Close()
	e = d.Remove(filepath.Base(path), recursive)
	if errors.Is(e, os.ErrNotExist) && recursive {
		return nil
	}
	return e
}

func renameHandle(source *os.File, destination *Directory, target string, replace bool) (renamed bool, err error) {
	if err := Leaf(target); err != nil {
		return false, err
	}
	if err := Validate(destination.Handle(), true); err != nil {
		return false, err
	}
	st, err := source.Stat()
	if err != nil {
		return false, err
	}
	if err = Validate(windows.Handle(source.Fd()), st.IsDir()); err != nil {
		return false, err
	}
	if replace {
		old, e := destination.OpenFile(target, os.O_RDONLY)
		if e == nil {
			e = old.Close()
		}
		if e != nil && !errors.Is(e, os.ErrNotExist) {
			return false, e
		}
	}
	names, err := windows.UTF16FromString(target)
	if err != nil {
		return false, err
	}
	names = names[:len(names)-1]
	type renameInfo struct {
		Replace uint32
		Root    windows.Handle
		Length  uint32
		Name    [1]uint16
	}
	size := int(unsafe.Offsetof(renameInfo{}.Name)) + len(names)*2
	buf := make([]byte, size)
	info := (*renameInfo)(unsafe.Pointer(&buf[0]))
	if replace {
		info.Replace = 1
	}
	info.Root = destination.Handle()
	info.Length = uint32(len(names) * 2)
	copy(unsafe.Slice(&info.Name[0], len(names)), names)
	var iosb windows.IO_STATUS_BLOCK
	err = windows.NtSetInformationFile(windows.Handle(source.Fd()), &iosb, &buf[0], uint32(size), windows.FileRenameInformation)
	runtime.KeepAlive(buf)
	if s, ok := err.(windows.NTStatus); ok {
		err = s.Errno()
	}
	if err != nil {
		return false, err
	}
	renamed = true
	// The native namespace transition is now visible. Every later error must
	// preserve this complete object for the domain recovery protocol.

	if !st.IsDir() {
		if err = source.Sync(); err != nil {
			return renamed, err
		}
	}
	return renamed, destination.Sync()
}

// Publish stages, flushes and publishes one complete private file in a held
// parent. Cleanup acts on the held stage, never on a re-resolved stage name.
func (d *Directory) Publish(name string, body []byte, exclusive bool) error {
	if err := Leaf(name); err != nil {
		return err
	}
	stage := ".publish-" + Random()
	f, err := d.OpenFile(stage, os.O_CREATE|os.O_EXCL|os.O_RDWR)
	if err != nil {
		return err
	}
	published := false
	defer func() {
		if !published {
			_ = markDelete(windows.Handle(f.Fd()))
		}
		_ = f.Close()
	}()
	if _, err = f.Write(body); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	published, err = renameHandle(f, d, name, !exclusive)
	if err != nil {
		return err
	}
	return f.Close()
}
func Publish(path string, body []byte, exclusive bool) error {
	d, e := Open(filepath.Dir(path), false)
	if e != nil {
		return e
	}
	defer d.Close()
	return d.Publish(filepath.Base(path), body, exclusive)
}
func (d *Directory) Rename(source string, target *Directory, name string, replace bool) error {
	if err := Leaf(source); err != nil {
		return err
	}
	if err := Validate(d.Handle(), true); err != nil {
		return err
	}
	h, err := ntOpen(d.Handle(), source, windows.DELETE|windows.FILE_GENERIC_READ|windows.FILE_GENERIC_WRITE|windows.READ_CONTROL, windows.FILE_OPEN, windows.FILE_SYNCHRONOUS_IO_NONALERT|windows.FILE_OPEN_NO_RECALL|windows.FILE_WRITE_THROUGH)
	if err != nil {
		return err
	}
	f := os.NewFile(uintptr(h), filepath.Join(d.File.Name(), source))
	defer f.Close()
	if _, err = renameHandle(f, target, name, replace); err != nil {
		return err
	}
	return d.Sync()
}
func Rename(source, target string, replace bool) error {
	a, e := Open(filepath.Dir(source), false)
	if e != nil {
		return e
	}
	defer a.Close()
	b, e := Open(filepath.Dir(target), false)
	if e != nil {
		return e
	}
	defer b.Close()
	return a.Rename(filepath.Base(source), b, filepath.Base(target), replace)
}
