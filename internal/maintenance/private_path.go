package maintenance

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ensurePrivateTree establishes store.Root as the trust boundary and then
// creates/validates each descendant one component at a time. This prevents a
// pre-created workspace or receipts symlink from redirecting metadata writes.
func ensurePrivateTree(root string, relative ...string) (string, error) {
	if strings.TrimSpace(root) == "" {
		return "", errors.New("maintenance receipt root is required")
	}
	canonicalRoot, err := canonicalPrivateRoot(root)
	if err != nil {
		return "", err
	}
	if err := rejectPrivateSymlinkAncestors(canonicalRoot); err != nil {
		return "", err
	}
	if err := os.MkdirAll(canonicalRoot, 0o700); err != nil {
		return "", err
	}
	if err := validatePrivateDirectory(canonicalRoot); err != nil {
		return "", err
	}
	current := filepath.Clean(canonicalRoot)
	for _, component := range relative {
		if component == "" || component == "." || component == ".." || filepath.Base(component) != component {
			return "", errors.New("invalid maintenance storage component")
		}
		current = filepath.Join(current, component)
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			if err := os.Mkdir(current, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
				return "", err
			}
			info, err = os.Lstat(current)
		}
		if err != nil {
			return "", err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return "", fmt.Errorf("maintenance storage component is not a private directory: %s", current)
		}
		if info.Mode().Perm() != 0o700 {
			if err := os.Chmod(current, 0o700); err != nil {
				return "", err
			}
		}
	}
	return current, nil
}

// canonicalPrivateRoot normalizes only the operating system's temporary
// directory alias (for example /tmp -> /private/tmp on macOS). Symlinks in
// user-controlled portions remain visible and are rejected before creation.
func canonicalPrivateRoot(root string) (string, error) {
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

func rejectPrivateSymlinkAncestors(path string) error {
	absolute, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return err
	}
	volume := filepath.VolumeName(absolute)
	remainder := strings.TrimPrefix(absolute, volume)
	remainder = strings.TrimPrefix(remainder, string(filepath.Separator))
	current := volume + string(filepath.Separator)
	for _, component := range strings.Split(remainder, string(filepath.Separator)) {
		if component == "" || component == "." {
			continue
		}
		current = filepath.Join(current, component)
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return errors.New("maintenance storage path cannot traverse symlinked ancestors")
		}
	}
	return nil
}

func validatePrivateDirectory(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return errors.New("maintenance receipt root must be a private local directory")
	}
	if info.Mode().Perm() != 0o700 {
		return os.Chmod(path, 0o700)
	}
	return nil
}
