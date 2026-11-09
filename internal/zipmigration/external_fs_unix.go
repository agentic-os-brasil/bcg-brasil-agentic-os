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

type externalTree struct {
	*engine
	sourceOnly bool
}

func externalOpen(root string) (*externalTree, error) {
	if err := scheduler.ValidateDirectory(root); err != nil {
		return nil, err
	}
	r, err := os.OpenRoot(root)
	if err != nil {
		return nil, err
	}
	return &externalTree{engine: &engine{r, root}}, nil
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
		return externalSyncPublication(e, name)
	}
	if !externalMissing(err) {
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
	if err = e.mkdir(path.Dir(name)); err != nil {
		return err
	}
	if err = e.mkdir(path.Dir(temporary)); err != nil {
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
	err = errors.Join(err, f.Close())
	if err != nil {
		return err
	}
	// The owner-private namespace excludes other users; no published leaf is replaced.
	if _, err = e.root.Lstat(name); !os.IsNotExist(err) {
		return errors.New("publication target appeared")
	}
	_, err = externalPublish(e, temporary, name)
	return err
}
func externalPublish(e *externalTree, from, to string) (bool, error) {
	return externalPublishAfterRename(e, from, to, nil)
}
func externalPublishAfterRename(e *externalTree, from, to string, afterRename func(*os.File, *os.File)) (moved bool, err error) {
	if e.sourceOnly {
		return false, errors.New("source tree grants no mutation authority")
	}

	if err = e.check(from); err != nil {
		return false, err
	}
	if err = e.check(to); err != nil {
		return false, err
	}
	source, err := e.root.Open(path.Dir(from))
	if err != nil {
		return false, err
	}
	defer func() { err = errors.Join(err, source.Close()) }()
	target, err := e.root.Open(path.Dir(to))
	if err != nil {
		return false, err
	}
	defer func() { err = errors.Join(err, target.Close()) }()
	if err = externalRenameExclusive(int(source.Fd()), path.Base(from), int(target.Fd()), path.Base(to)); err != nil {
		return false, err
	}
	moved = true
	if afterRename != nil {
		afterRename(source, target)
	}
	return true, errors.Join(source.Sync(), target.Sync())
}

func externalOpenSource(root string) (*externalTree, error) {
	e, err := externalOpen(root)
	if err == nil {
		e.sourceOnly = true
	}
	return e, err
}
func externalOpenMovable(root string) (*externalTree, error)  { return externalOpen(root) }
func (e *externalTree) close() error                          { return e.root.Close() }
func (e *externalTree) stat(name string) (os.FileInfo, error) { return e.root.Lstat(name) }
func (e *externalTree) mkdir(name string) error {
	if e.sourceOnly {
		return errors.New("source tree grants no mutation authority")
	}
	if err := e.root.MkdirAll(name, 0700); err != nil {
		return err
	}
	return e.syncPath(name)
}
func (e *externalTree) walk(name string, visit fs.WalkDirFunc) error {
	return fs.WalkDir(e.root.FS(), name, visit)
}
func (e *externalTree) move(archive string) (moved bool, err error) {
	if e.sourceOnly {
		return false, errors.New("source tree grants no mutation authority")
	}

	parent, err := externalOpen(filepath.Dir(e.project))
	if err != nil {
		return false, err
	}
	defer func() { err = errors.Join(err, parent.close()) }()
	moved, err = externalPublish(parent, filepath.Base(e.project), filepath.Base(archive))
	if moved {
		e.project = archive
	}
	return moved, err
}
func (e *externalTree) removeExact(name string, expected []byte) (bool, error) {
	if e.sourceOnly {
		return false, errors.New("source tree grants no mutation authority")
	}

	body, err := externalRead(e, name, true)
	if externalMissing(err) {
		return false, e.syncPath(path.Dir(name))
	}
	if err != nil {
		return false, err
	}
	if !bytes.Equal(body, expected) {
		return false, errors.New("locator belongs to another attempt")
	}
	if err = e.root.Remove(name); err != nil {
		return false, err
	}
	return true, e.syncPath(path.Dir(name))
}
func (e *externalTree) publishRetirementUnderLease(next, name string, body []byte) error {
	if e.sourceOnly {
		return errors.New("source tree grants no mutation authority")
	}

	if err := e.root.Rename(next, name); err != nil {
		return err
	}
	return e.syncPath(name)
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

func (e *externalTree) syncPath(name string) error {
	if e.sourceOnly {
		return errors.New("source tree grants no sync authority")
	}
	if err := e.check(name); err != nil {
		return err
	}
	for {
		f, err := e.root.OpenFile(name, externalReadFlags(), 0)
		if err != nil {
			return err
		}
		err = errors.Join(f.Sync(), f.Close())
		if err != nil || name == "." {
			return err
		}
		name = path.Dir(name)
	}
}

func externalOpenHome(root string, create bool) (*externalTree, error) {
	if create {
		if _, err := os.Lstat(root); externalMissing(err) {
			if err = os.Mkdir(root, 0700); err != nil {
				return nil, err
			}
		} else if err != nil {
			return nil, err
		}
	}
	if err := externalOwnedDirectory(root, true); err != nil {
		return nil, err
	}
	return externalOpen(root)
}
func (e *externalTree) syncHomeParent() error {
	if e.sourceOnly {
		return errors.New("source tree grants no sync authority")
	}
	parent, err := externalOpen(filepath.Dir(e.project))
	if err != nil {
		return err
	}
	return errors.Join(parent.syncPath("."), parent.close())
}

func (e *externalTree) restoreHome(home string) error {
	if e.sourceOnly {
		return errors.New("source tree grants no mutation authority")
	}

	if err := os.Mkdir(home, 0700); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	entries, err := os.ReadDir(home)
	if err != nil || len(entries) != 0 {
		return errors.New("prior empty home conflicts")
	}
	return e.syncHomeParent()
}
