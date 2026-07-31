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
	if err := os.MkdirAll(root, 0o700); err != nil {
		return "", err
	}
	if err := validatePrivateDirectory(root); err != nil {
		return "", err
	}
	current := filepath.Clean(root)
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
