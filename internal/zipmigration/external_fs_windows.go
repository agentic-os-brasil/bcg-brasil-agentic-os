//go:build windows

package zipmigration

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/winprivate"
	"golang.org/x/sys/windows"
)

// Each transaction owns its admitted roots; no pathname registry carries custody.
type externalTree struct {
	project   string
	source    *winprivate.SourceTree
	directory *winprivate.Directory
	movable   *winprivate.MovableDirectory
}

func externalOpen(root string) (*externalTree, error) {
	d, err := winprivate.OpenPinnedTree(root)
	if err != nil {
		return nil, err
	}
	return &externalTree{project: root, directory: d}, nil
}
func externalOpenSource(root string) (*externalTree, error) {
	d, err := winprivate.InspectPinnedTree(root)
	if err != nil {
		return nil, err
	}
	return &externalTree{project: root, source: d}, nil
}
func externalOpenMovable(root string) (*externalTree, error) {
	d, err := winprivate.OpenMovableDirectory(root)
	if err != nil {
		return nil, err
	}
	return &externalTree{project: root, movable: d}, nil
}
func (e *externalTree) privateRoot() *winprivate.Directory {
	if e.movable != nil {
		return e.movable.Directory()
	}
	return e.directory
}
func (e *externalTree) close() error {
	if e.movable != nil {
		return e.movable.Close()
	}
	if e.source != nil {
		return e.source.Close()
	}
	return e.directory.Close()
}
func externalRelative(name string) error {
	if name == "." {
		return nil
	}
	if !fs.ValidPath(name) {
		return errors.New("unsafe external relative path")
	}
	_, err := collisionKey(name)
	return err
}
func (e *externalTree) privateDirectory(name string, create bool) (*winprivate.Directory, func(), error) {
	if err := externalRelative(name); err != nil {
		return nil, nil, err
	}
	d := e.privateRoot()
	if d == nil {
		return nil, nil, errors.New("source tree grants no private mutation authority")
	}
	owned := false
	for _, component := range strings.Split(name, "/") {
		if component == "." {
			continue
		}
		next, err := d.PinnedChild(component, create, false)
		if owned {
			d.Close()
		}
		if err != nil {
			return nil, nil, err
		}
		d, owned = next, true
	}
	return d, func() {
		if owned {
			d.Close()
		}
	}, nil
}
func (e *externalTree) sourceDirectory(name string) (*winprivate.SourceTree, func(), error) {
	if err := externalRelative(name); err != nil {
		return nil, nil, err
	}
	d := e.source
	if d == nil {
		return nil, nil, errors.New("read-only source admission missing")
	}
	owned := false
	for _, component := range strings.Split(name, "/") {
		if component == "." {
			continue
		}
		next, err := d.Child(component)
		if owned {
			d.Close()
		}
		if err != nil {
			return nil, nil, err
		}
		d, owned = next, true
	}
	return d, func() {
		if owned {
			d.Close()
		}
	}, nil
}
func (e *externalTree) readDirectory(name string) ([]os.DirEntry, error) {
	if e.source != nil {
		d, close, err := e.sourceDirectory(name)
		if err != nil {
			return nil, err
		}
		defer close()
		return d.ReadDir(100000)
	}
	d, close, err := e.privateDirectory(name, false)
	if err != nil {
		return nil, err
	}
	defer close()
	return d.ReadDir(100000)
}
func (e *externalTree) openFile(name string, private bool) (*os.File, func(), error) {
	if err := externalRelative(name); err != nil {
		return nil, nil, err
	}
	if e.source != nil {
		if private {
			return nil, nil, errors.New("source cannot supply private state authority")
		}
		d, close, err := e.sourceDirectory(path.Dir(name))
		if err != nil {
			return nil, nil, err
		}
		f, err := d.OpenFile(path.Base(name))
		if err != nil {
			close()
			return nil, nil, err
		}
		return f, close, nil
	}
	d, close, err := e.privateDirectory(path.Dir(name), false)
	if err != nil {
		return nil, nil, err
	}
	f, err := d.OpenFile(path.Base(name), os.O_RDONLY)
	if err != nil {
		close()
		return nil, nil, err
	}
	return f, close, nil
}
func (e *externalTree) stat(name string) (os.FileInfo, error) {
	if err := externalRelative(name); err != nil {
		return nil, err
	}
	if name == "." {
		if e.source != nil {
			return e.source.Stat()
		}
		d := e.privateRoot()
		if err := winprivate.Validate(d.Handle(), true); err != nil {
			return nil, err
		}
		return d.File.Stat()
	}
	entries, err := e.readDirectory(path.Dir(name))
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if !strings.EqualFold(entry.Name(), path.Base(name)) {
			continue
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return nil, errors.New("external reparse entry refused")
		}
		if entry.IsDir() {
			if e.source != nil {
				d, close, err := e.sourceDirectory(name)
				if err != nil {
					return nil, err
				}
				defer close()
				return d.Stat()
			}
			d, close, err := e.privateDirectory(name, false)
			if err != nil {
				return nil, err
			}
			defer close()
			return d.File.Stat()
		}
		f, close, err := e.openFile(name, e.source == nil)
		if err != nil {
			return nil, err
		}
		defer close()
		defer f.Close()
		return f.Stat()
	}
	return nil, &os.PathError{Op: "external stat", Path: name, Err: os.ErrNotExist}
}
func (e *externalTree) check(name string) error {
	if err := externalRelative(name); err != nil {
		return err
	}
	current := ""
	for _, part := range strings.Split(name, "/") {
		current = path.Join(current, part)
		_, err := e.stat(current)
		if externalMissing(err) {
			return nil
		}
		if err != nil {
			return err
		}
	}
	return nil
}
func externalReadLimit(e *externalTree, name string, private bool, maximum int64) ([]byte, error) {
	f, close, err := e.openFile(name, private)
	if err != nil {
		return nil, err
	}
	defer close()
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if info.Size() > maximum {
		return nil, errors.New("external file exceeds bounded size")
	}
	body, err := io.ReadAll(io.LimitReader(f, maximum+1))
	if int64(len(body)) > maximum {
		return nil, errors.New("external file exceeds bounded size")
	}
	return body, err
}
func (e *externalTree) walk(name string, visit fs.WalkDirFunc) error {
	info, err := e.stat(name)
	if err != nil {
		return visit(name, nil, err)
	}
	var walk func(string, os.FileInfo) error
	walk = func(current string, info os.FileInfo) error {
		if err := visit(current, fs.FileInfoToDirEntry(info), nil); err != nil {
			if err == fs.SkipDir && info.IsDir() {
				return nil
			}
			return err
		}
		if !info.IsDir() {
			return nil
		}
		entries, err := e.readDirectory(current)
		if err != nil {
			return visit(current, fs.FileInfoToDirEntry(info), err)
		}
		for _, entry := range entries {
			child := path.Join(current, entry.Name())
			info, err := e.stat(child)
			if err != nil {
				return visit(child, nil, err)
			}
			if err = walk(child, info); err != nil {
				return err
			}
		}
		return nil
	}
	return walk(name, info)
}
func (e *externalTree) mkdir(name string) error {
	_, close, err := e.privateDirectory(name, true)
	if err != nil {
		return err
	}
	close()
	return nil
}
func externalEnsure(e *externalTree, name string, body []byte) (err error) {
	current, err := externalReadLimit(e, name, true, int64(len(body)))
	if err == nil {
		if !bytes.Equal(current, body) {
			return errors.New("foreign external control or target content")
		}
		return nil
	}
	if !externalMissing(err) {
		return err
	}
	target, closeTarget, err := e.privateDirectory(path.Dir(name), true)
	if err != nil {
		return err
	}
	defer closeTarget()
	temporary := externalTemporary(name)
	staging, closeStaging, err := e.privateDirectory(path.Dir(temporary), true)
	if err != nil {
		return err
	}
	defer closeStaging()
	stage, err := staging.OpenStage(path.Base(temporary))
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, stage.Close()) }()
	info, err := stage.Stat()
	if err != nil {
		return err
	}
	if info.Size() > int64(len(body)) {
		return errors.New("foreign partial publication")
	}
	partial, err := io.ReadAll(io.LimitReader(stage, int64(len(body))+1))
	if err != nil {
		return err
	}
	if !bytes.HasPrefix(body, partial) {
		return errors.New("foreign partial publication")
	}
	if _, err = stage.Write(body[len(partial):]); err != nil {
		return err
	}
	if err = stage.Sync(); err != nil {
		return err
	}
	// Publish retains the exact file whose prefix was validated above.
	_, err = stage.Publish(target, path.Base(name))
	return err
}
func (e *externalTree) move(archive string) (bool, error) {
	if e.movable == nil || filepath.Dir(archive) != filepath.Dir(e.project) {
		return false, errors.New("rollback requires initial movable root custody")
	}
	moved, err := e.movable.MoveTo(filepath.Base(archive))
	if moved {
		e.project = archive
	}
	return moved, err
}
func (e *externalTree) removeExact(name string, expected []byte) (bool, error) {
	d, close, err := e.privateDirectory(path.Dir(name), false)
	if err != nil {
		return false, err
	}
	defer close()
	marked, err := d.RemoveExact(path.Base(name), expected)
	if !marked && externalMissing(err) {
		return false, nil
	}
	if marked && err == nil {
		// A sharing reader may retain the old name delete-pending; do not declare rollback complete.
		_, probeErr := e.stat(name)
		if probeErr == nil {
			return true, errors.New("locator retirement is pending; close readers and resume rollback")
		}
		if !externalMissing(probeErr) {
			return true, probeErr
		}
	}
	return marked, err
}

// The domain compared its fixed pointer under the profile lease. This is a
// cooperative metadata replacement, not an OS compare-and-swap guarantee.
func (e *externalTree) publishRetirementUnderLease(next, name string, body []byte) (err error) {
	source, closeSource, err := e.privateDirectory(path.Dir(next), false)
	if err != nil {
		return err
	}
	defer closeSource()
	target, closeTarget, err := e.privateDirectory(path.Dir(name), false)
	if err != nil {
		return err
	}
	defer closeTarget()
	stage, err := source.OpenStage(path.Base(next))
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, stage.Close()) }()
	info, err := stage.Stat()
	if err != nil {
		return err
	}
	if info.Size() != int64(len(body)) {
		return errors.New("retirement stage changed")
	}
	actual, err := io.ReadAll(io.LimitReader(stage, int64(len(body))+1))
	if err != nil {
		return err
	}
	if !bytes.Equal(actual, body) {
		return errors.New("retirement stage changed")
	}
	_, err = stage.PublishReplacingUnderLease(target, path.Base(name))
	return err
}

func externalPathInfo(name string, held ...*externalTree) (os.FileInfo, error) {
	for _, tree := range held {
		if tree != nil && name == tree.project {
			return tree.stat(".")
		}
	}
	if filepath.Dir(name) == name {
		tree, err := externalOpenSource(name)
		if err != nil {
			return nil, err
		}
		defer tree.close()
		return tree.stat(".")
	}
	e, err := externalOpenSource(filepath.Dir(name))
	if err != nil {
		return nil, err
	}
	defer e.close()
	return e.stat(filepath.Base(name))
}
func externalPathEntries(name string) ([]os.DirEntry, error) {
	e, err := externalOpenSource(name)
	if err != nil {
		return nil, err
	}
	defer e.close()
	return e.readDirectory(".")
}
func externalCreateHome(name string) error {
	parent, err := winprivate.OpenPinnedTree(filepath.Dir(name))
	if err != nil {
		return err
	}
	defer parent.Close()
	child, err := parent.PinnedChild(filepath.Base(name), true, true)
	if err != nil {
		return err
	}
	return child.Close()
}
func externalAcquire(e *externalTree, attempt string) (func(), error) {
	if err := externalProfileControls(e.project, attempt); err != nil {
		return nil, err
	}
	d, close, err := e.privateDirectory(externalIntentControl, true)
	if err != nil {
		return nil, err
	}
	f, err := d.OpenLock("apply.lock")
	if err != nil {
		close()
		return nil, err
	}
	var overlapped windows.Overlapped
	err = windows.LockFileEx(windows.Handle(f.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &overlapped)
	if err != nil {
		f.Close()
		close()
		return nil, err
	}
	release := func() {
		var o windows.Overlapped
		windows.UnlockFileEx(windows.Handle(f.Fd()), 0, 1, 0, &o)
		f.Close()
		close()
	}
	info, err := f.Stat()
	if err != nil {
		release()
		return nil, err
	}
	if info.Size() != 0 {
		release()
		return nil, fmt.Errorf("migration lock contains unexpected data; preserve it and close older clients")
	}
	return release, nil
}

// Inspect the existing v1 receipt through the admitted read-only source; no old
// writer or pathname-based status operation participates in external migration.
func externalV1Committed(e *externalTree, data, brain []record) error {
	body, err := externalRead(e, control+"/state.json", false)
	if err != nil {
		return err
	}
	var state Result
	if err = json.Unmarshal(body, &state); err != nil {
		return err
	}
	if state.State != "committed" || len(state.AttemptID) != 32 {
		return errors.New("v1 source is not committed")
	}
	if _, err = hex.DecodeString(state.AttemptID); err != nil {
		return err
	}
	body, err = externalRead(e, control+"/attempts/"+state.AttemptID+"/plan.json", false)
	if err != nil {
		return err
	}
	if hash(body) != state.PlanSHA256 {
		return errors.New("plan_drift")
	}
	var plan Plan
	if err = json.Unmarshal(body, &plan); err != nil {
		return err
	}
	if plan.SchemaVersion != 1 || plan.Project != e.project || plan.AttemptID != state.AttemptID {
		return errors.New("plan_binding_mismatch")
	}
	body, err = externalRead(e, control+"/attempts/"+state.AttemptID+"/receipt.json", false)
	if err != nil {
		return err
	}
	var receipt Result
	if err = json.Unmarshal(body, &receipt); err != nil {
		return err
	}
	before, _ := json.Marshal(state)
	after, _ := json.Marshal(receipt)
	if !bytes.Equal(before, after) {
		return errors.New("receipt_mismatch")
	}
	original := make([]record, 0, len(data))
	for _, r := range data {
		r.Path = strings.TrimPrefix(r.Path, "data/")
		original = append(original, r)
	}
	if fingerprint(original) != plan.SourceFingerprint {
		return errors.New("source_drift")
	}
	found := map[string]record{}
	for _, r := range brain {
		r.Path = strings.TrimPrefix(r.Path, "brain/")
		found[r.Path] = r
	}
	for _, entry := range plan.Entries {
		if entry.Action != "copy" {
			continue
		}
		if err := externalRelative(entry.Target); err != nil {
			return err
		}
		for ancestor := path.Dir(entry.Target); ancestor != "."; ancestor = path.Dir(ancestor) {
			if current, ok := found[ancestor]; ok && current.Kind != "directory" {
				return fmt.Errorf("target_ancestor_type_changed:%s", ancestor)
			}
		}
		if current, ok := found[entry.Target]; ok && current.Kind != entry.Kind {
			return fmt.Errorf("target_type_changed:%s", entry.Target)
		}
	}
	return nil
}
