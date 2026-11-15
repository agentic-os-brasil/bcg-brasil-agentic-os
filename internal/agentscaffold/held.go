package agentscaffold

import (
	"errors"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
	"os"
	"path/filepath"
)

// The same canonical engine consumes either its own current root or the exact
// borrowed root retained by an admitted setup operation. No path reopen occurs.
type scaffoldRoot struct {
	files  *scheduler.HeldDirectory
	prefix string
	owned  bool
}

func (r *scaffoldRoot) rel(p string) string {
	if r.prefix == "" {
		return p
	}
	return filepath.Join(r.prefix, p)
}
func (r *scaffoldRoot) Name() string { return filepath.Join(r.files.Path(), r.prefix) }
func (r *scaffoldRoot) Close() error {
	if r.owned {
		return r.files.Close()
	}
	return nil
}
func (r *scaffoldRoot) OpenRoot(p string) (*scaffoldRoot, error) {
	prefix := r.rel(p)
	if _, e := r.files.StatDirectory(prefix); e != nil {
		return nil, e
	}
	return &scaffoldRoot{files: r.files, prefix: prefix}, nil
}
func (r *scaffoldRoot) Open(p string) (*os.File, error) {
	return r.files.OpenFile(r.rel(p), os.O_RDONLY)
}
func (r *scaffoldRoot) RemoveAll(p string) error {
	return errors.New("private non-authoritative staging is preserved")
}
