//go:build windows

package scheduler

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

var securePathStepHook func(string)

// Windows does not expose a portable openat/O_NOFOLLOW equivalent through the
// standard runtime. Keep the boundary fail-closed: every component is Lstat'd,
// symlinks/reparse-like entries are rejected, and no recursive MkdirAll or
// ancestor chmod is used. Native Windows qualification remains responsible for
// stronger device-specific reparse-point evidence.
func secureEnsurePrivatePath(path string) error {
	components, current, err := windowsPathParts(path)
	if err != nil {
		return err
	}
	for index, component := range components {
		current = filepath.Join(current, component)
		info, statErr := os.Lstat(current)
		created := false
		if errors.Is(statErr, os.ErrNotExist) {
			if mkdirErr := os.Mkdir(current, 0o700); mkdirErr != nil && !errors.Is(mkdirErr, os.ErrExist) {
				return mkdirErr
			}
			info, statErr = os.Lstat(current)
			created = statErr == nil
		}
		if statErr != nil {
			return statErr
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return errors.New("scheduler storage component is not a private directory")
		}
		if created || index == len(components)-1 {
			if chmodErr := os.Chmod(current, 0o700); chmodErr != nil {
				return chmodErr
			}
		}
	}
	return nil
}

func secureLookupPrivatePath(path string) error {
	components, current, err := windowsPathParts(path)
	if err != nil {
		return err
	}
	for _, component := range components {
		current = filepath.Join(current, component)
		info, statErr := os.Lstat(current)
		if statErr != nil {
			return statErr
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return errors.New("scheduler storage component is not a private directory")
		}
	}
	return nil
}

func windowsPathParts(path string) ([]string, string, error) {
	absolute, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return nil, "", err
	}
	volume := filepath.VolumeName(absolute)
	if volume == "" || !filepath.IsAbs(absolute) {
		return nil, "", errors.New("scheduler path must be an absolute Windows path")
	}
	remainder := strings.TrimPrefix(absolute, volume)
	remainder = strings.TrimLeft(remainder, `\`+string(filepath.Separator))
	components := make([]string, 0)
	for _, component := range strings.FieldsFunc(remainder, func(r rune) bool { return r == '\\' || r == '/' }) {
		if component != "" && component != "." {
			components = append(components, component)
		}
	}
	return components, volume + string(filepath.Separator), nil
}
