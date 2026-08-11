package scheduler

import (
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
)

// CanonicalPrivatePath resolves only the platform's fixed temporary-directory
// alias (for example macOS /var -> /private/var). It never follows an
// application-controlled symlink and is safe to use before the no-follow walk.
func CanonicalPrivatePath(path string) (string, error) {
	return canonicalSchedulerRoot(path)
}

// EnsurePrivateDirectory creates or opens one directory through the scheduler's
// descriptor-anchored, no-follow filesystem boundary. On Unix, the resulting
// directory is normalized to owner-only permissions.
func EnsurePrivateDirectory(path string) error {
	return secureEnsurePrivatePath(path)
}

// ValidatePrivateDirectory proves that the complete path is traversable
// without following links or Windows reparse points. Unix callers additionally
// require an owner-only directory; Windows relies on the native protected data
// root ACL rather than portable mode bits.
func ValidatePrivateDirectory(path string) error {
	if err := secureLookupPrivatePath(path); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return errors.New("private path must be a non-symlink directory")
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o700 {
		return fmt.Errorf("private directory permissions are %04o, want 0700", info.Mode().Perm())
	}
	return nil
}

// ReadPrivateFile opens a bounded regular file relative to descriptor-anchored
// directories. It never follows the leaf or an ancestor link/reparse point.
func ReadPrivateFile(path string, maximum int64) ([]byte, error) {
	if maximum <= 0 {
		return nil, errors.New("private file maximum must be positive")
	}
	file, err := secureOpenFile(path, os.O_RDONLY, 0)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > maximum {
		return nil, errors.New("private file must be a bounded regular file")
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		return nil, fmt.Errorf("private file permissions are %04o, want 0600", info.Mode().Perm())
	}
	body, err := io.ReadAll(io.LimitReader(file, maximum+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > maximum {
		return nil, errors.New("private file exceeds bounded size")
	}
	return body, nil
}

// WriteNewPrivateFile creates a new owner-private regular file without
// following links. Existing leaves fail closed.
func WriteNewPrivateFile(path string, body []byte) error {
	return secureWriteNewFile(path, body)
}

// ReplacePrivateFile rewrites the body of an existing owner-private regular
// file without following the leaf or an ancestor link. An absent leaf fails
// closed rather than being created, so creation stays with
// WriteNewPrivateFile and a replace can never bring a page into existence.
//
// The write is not atomic on its own. Callers that must survive a crash
// mid-write are responsible for a journaled transition around it.
func ReplacePrivateFile(path string, body []byte) error {
	file, err := secureOpenFile(path, os.O_WRONLY|os.O_TRUNC, 0)
	if err != nil {
		return err
	}
	if _, err := file.Write(body); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}
