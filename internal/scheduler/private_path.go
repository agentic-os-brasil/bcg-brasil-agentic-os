package scheduler

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// ensurePrivateTree treats Store.Root as the configured trust boundary and
// validates every scheduler-owned descendant before it is read or written.
// Platform implementations walk the directory tree relative to an opened
// no-follow descriptor on Unix, with a bounded reparse-point check on Windows.
func ensurePrivateTree(root string, relative ...string) (string, error) {
	if strings.TrimSpace(root) == "" {
		return "", errors.New("scheduler root is required")
	}
	var err error
	root, err = canonicalSchedulerRoot(root)
	if err != nil {
		return "", err
	}
	current := filepath.Clean(root)
	for _, component := range relative {
		if component == "" || component == "." || component == ".." || filepath.Base(component) != component {
			return "", errors.New("invalid scheduler storage component")
		}
		current = filepath.Join(current, component)
	}
	if err := secureEnsurePrivatePath(current); err != nil {
		return "", err
	}
	return current, nil
}

// lookupPrivateTree is the non-mutating sibling used by authority preflight.
// It refuses missing components instead of creating an enrollment boundary.
func lookupPrivateTree(root string, relative ...string) (string, error) {
	if strings.TrimSpace(root) == "" {
		return "", errors.New("scheduler root is required")
	}
	canonical, err := canonicalSchedulerRoot(root)
	if err != nil {
		return "", err
	}
	current := filepath.Clean(canonical)
	for _, component := range relative {
		if component == "" || component == "." || component == ".." || filepath.Base(component) != component {
			return "", errors.New("invalid scheduler storage component")
		}
		current = filepath.Join(current, component)
	}
	if err := secureLookupPrivatePath(current); err != nil {
		return "", err
	}
	return current, nil
}

func canonicalSchedulerRoot(root string) (string, error) {
	absolute, err := filepath.Abs(filepath.Clean(root))
	if err != nil {
		return "", err
	}
	temporary, err := filepath.Abs(filepath.Clean(os.TempDir()))
	if err != nil {
		return "", err
	}
	physicalTemporary, err := filepath.EvalSymlinks(temporary)
	if err != nil {
		return "", err
	}
	relative, err := filepath.Rel(temporary, absolute)
	if err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return filepath.Join(physicalTemporary, relative), nil
	}
	return absolute, nil
}
