//go:build !windows

package scheduler

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"golang.org/x/sys/unix"
	"os"
	"path/filepath"
)

// PublishNewPrivateFile stages and fsyncs private bytes, then atomically publishes
// an absent leaf inside one held no-follow directory. Existing leaves survive.
func PublishNewPrivateFile(path string, body []byte) error {
	parent, name, err := unixFileParent(path)
	if err != nil {
		return err
	}
	directory, err := openSecureDirectory(parent, false)
	if err != nil {
		return err
	}
	defer directory.close()
	var stat unix.Stat_t
	if err = unix.Fstat(directory.fd, &stat); err != nil {
		return err
	}
	if stat.Uid != uint32(os.Getuid()) || stat.Mode&0777 != 0700 {
		return errors.New("publication requires owner-private directory")
	}
	random := make([]byte, 16)
	if _, err = rand.Read(random); err != nil {
		return err
	}
	stage := ".publish-" + hex.EncodeToString(random)
	if err = directory.writeNewFile(stage, body); err != nil {
		return err
	}
	defer directory.removeFile(stage)
	if err = renamePrivateExclusive(directory.fd, stage, filepath.Base(name)); err != nil {
		return err
	}
	return unix.Fsync(directory.fd)
}
