//go:build !windows

package zipmigration

import (
	"bytes"
	"errors"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
)

type externalTree struct{ *engine }

func externalOpen(root string) (*externalTree, error) {
	if err := scheduler.ValidateDirectory(root); err != nil {
		return nil, err
	}
	r, err := os.OpenRoot(root)
	if err != nil {
		return nil, err
	}
	return &externalTree{&engine{r, root}}, nil
}
func externalReadLimit(e *externalTree, relative string, private bool, maximum int64) ([]byte, error) {
	if err := e.check(relative); err != nil {
		return nil, err
	}
	info, err := e.root.Lstat(relative)
	if err != nil {
		return nil, err
	}
	if err = externalRegular(info, private); err != nil {
		return nil, err
	}
	f, err := e.root.OpenFile(relative, externalReadFlags(), 0)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !os.SameFile(info, opened) {
		return nil, errors.New("external file identity changed")
	}
	if err = externalRegular(opened, private); err != nil {
		return nil, err
	}
	if opened.Size() > maximum {
		return nil, errors.New("external file exceeds bounded size")
	}
	b, err := io.ReadAll(io.LimitReader(f, maximum+1))
	if int64(len(b)) > maximum {
		return nil, errors.New("external file exceeds bounded size")
	}
	return b, err
}
func externalEnsure(e *externalTree, name string, body []byte) error {
	current, err := externalReadLimit(e, name, true, int64(len(body)))
	if err == nil {
		if !bytes.Equal(current, body) {
			return errors.New("foreign external control or target content")
		}
		return nil
	}
	if !os.IsNotExist(err) {
		return err
	}
	if err = e.check(name); err != nil {
		return err
	}
	temporary := externalTemporary(name)
	if err = e.check(temporary); err != nil {
		return err
	}
	for _, parent := range []string{path.Dir(name), path.Dir(temporary)} {
		if err = externalPrivateParents(e, parent); err != nil {
			return err
		}
	}
	if err = e.root.MkdirAll(path.Dir(name), 0700); err != nil {
		return err
	}
	if err = e.root.MkdirAll(path.Dir(temporary), 0700); err != nil {
		return err
	}
	partial, err := externalReadLimit(e, temporary, true, int64(len(body)))
	if os.IsNotExist(err) {
		partial = nil
	} else if err != nil {
		return err
	}
	if !bytes.HasPrefix(body, partial) {
		return errors.New("foreign partial publication")
	}
	f, err := e.root.OpenFile(temporary, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0600)
	if err != nil {
		return err
	}
	info, err := f.Stat()
	if err == nil {
		err = externalRegular(info, true)
	}
	if err == nil {
		_, err = f.Write(body[len(partial):])
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	// The owner-private namespace excludes other users; no published leaf is replaced.
	if _, err = e.root.Lstat(name); !os.IsNotExist(err) {
		return errors.New("publication target appeared")
	}
	if err = externalPublish(e, temporary, name); err != nil {
		return err
	}
	directory, err := e.root.Open(path.Dir(name))
	if err != nil {
		return err
	}
	err = directory.Sync()
	closeErr = directory.Close()
	if err == nil {
		err = closeErr
	}
	return err
}
func externalPublish(e *externalTree, from, to string) error {
	if err := e.check(from); err != nil {
		return err
	}
	if err := e.check(to); err != nil {
		return err
	}
	source, err := e.root.Open(path.Dir(from))
	if err != nil {
		return err
	}
	defer source.Close()
	target, err := e.root.Open(path.Dir(to))
	if err != nil {
		return err
	}
	defer target.Close()
	return externalRenameExclusive(int(source.Fd()), path.Base(from), int(target.Fd()), path.Base(to))
}

func externalOpenSource(root string) (*externalTree, error)   { return externalOpen(root) }
func externalOpenMovable(root string) (*externalTree, error)  { return externalOpen(root) }
func (e *externalTree) close() error                          { return e.root.Close() }
func (e *externalTree) stat(name string) (os.FileInfo, error) { return e.root.Lstat(name) }
func (e *externalTree) mkdir(name string) error               { return e.root.MkdirAll(name, 0700) }
func (e *externalTree) walk(name string, visit fs.WalkDirFunc) error {
	return fs.WalkDir(e.root.FS(), name, visit)
}
func (e *externalTree) move(archive string) (bool, error) {
	parent, err := externalOpen(filepath.Dir(e.project))
	if err != nil {
		return false, err
	}
	defer parent.close()
	if err = externalPublish(parent, filepath.Base(e.project), filepath.Base(archive)); err != nil {
		return false, err
	}
	e.project = archive
	return true, nil
}
func (e *externalTree) removeExact(name string, expected []byte) (bool, error) {
	body, err := externalRead(e, name, true)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !bytes.Equal(body, expected) {
		return false, errors.New("locator belongs to another attempt")
	}
	err = e.root.Remove(name)
	return err == nil, err
}
func (e *externalTree) publishRetirementUnderLease(next, name string, body []byte) error {
	if err := e.root.Rename(next, name); err != nil {
		return err
	}
	directory, err := e.root.Open(path.Dir(name))
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}

func externalPathInfo(name string, held ...*externalTree) (os.FileInfo, error) { return os.Lstat(name) }
func externalPathEntries(name string) ([]os.DirEntry, error)                   { return os.ReadDir(name) }
func externalCreateHome(name string) error                                     { return os.Mkdir(name, 0700) }

func externalV1Committed(e *externalTree, _, _ []record) error {
	if status := Run(e.project, Options{Status: true}); status.State != "committed" {
		return errors.New("v1 source is not committed")
	}
	return nil
}
