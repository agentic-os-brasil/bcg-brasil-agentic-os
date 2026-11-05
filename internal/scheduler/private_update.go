package scheduler

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

// ReadPrivateStateFile reads an existing authority file with the same held-
// descriptor ownership, single-link, mode and size checks as UpdatePrivateFile.
// It does not create a lock, directory or file, or change permissions.
func ReadPrivateStateFile(path string, maximum int64) ([]byte, error) {
	if maximum < 1 || maximum > 1<<20 {
		return nil, errors.New("invalid private state limit")
	}
	file, err := secureOpenFile(path, os.O_RDONLY, 0)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return readPrivateStateFile(file, maximum)
}

func readPrivateStateFile(file *os.File, maximum int64) ([]byte, error) {
	if err := privateUpdateFile(file); err != nil {
		return nil, err
	}
	body, err := io.ReadAll(io.LimitReader(file, maximum+1))
	if err != nil {
		return nil, err
	}
	if len(body) == 0 {
		return nil, errors.New("private state file is empty")
	}
	if int64(len(body)) > maximum {
		return nil, errors.New("private state size")
	}
	return body, nil
}

// UpdatePrivateFile holds the existing OS lock and no-follow directory through
// read, validation and atomic publication. A nil result leaves bytes unchanged.
// It is a small file primitive, not a source of application admission authority.
func UpdatePrivateFile(path string, maximum int64, update func([]byte) ([]byte, error)) error {
	return updatePrivateFileGuarded(path, maximum, nil, update, false)
}

// UpdatePrivateFileGuarded preserves the existing private update transaction,
// placing its final CAS and visible rename inside the caller's live authority.
// The file lock is acquired before guard: domain -> control -> installation -> lease.
func UpdatePrivateFileGuarded(path string, maximum int64, guard func(func() error) error, update func([]byte) ([]byte, error)) error {
	if guard == nil {
		return errors.New("private publication guard required")
	}
	return updatePrivateFileGuarded(path, maximum, guard, update, false)
}

// RestorePrivateFileGuarded compensates only the exact bytes just published.
// A nil previous value restores absence; it never deletes a different parent.
func RestorePrivateFileGuarded(path string, maximum int64, published, previous []byte, guard func(func() error) error) error {
	if len(published) == 0 || guard == nil {
		return errors.New("exact compensation and guard required")
	}
	return updatePrivateFileGuarded(path, maximum, guard, func(current []byte) ([]byte, error) {
		if !bytes.Equal(current, published) {
			return nil, errors.New("private compensation parent changed")
		}
		return previous, nil
	}, previous == nil)
}

func updatePrivateFileGuarded(path string, maximum int64, guard func(func() error) error, update func([]byte) ([]byte, error), remove bool) error {
	if maximum < 1 || maximum > 1<<20 {
		return errors.New("invalid private update limit")
	}
	directory, err := openSecureDirectory(filepath.Dir(path), true)
	if err != nil {
		return err
	}
	defer directory.close()
	name := filepath.Base(path)
	if err := validateSecureLeaf(name); err != nil {
		return err
	}
	lock, err := directory.openFile(name+".lock", os.O_RDWR|os.O_CREATE, 0600)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err := privateUpdateFile(lock); err != nil {
		return err
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		err = tryLockLeaseGuard(lock)
		if err == nil {
			break
		}
		if !errors.Is(err, errLeaseGuardBusy) || time.Now().After(deadline) {
			return err
		}
		time.Sleep(5 * time.Millisecond)
	}
	defer unlockLeaseGuard(lock)
	read := func() ([]byte, error) {
		file, err := directory.openFile(name, os.O_RDONLY, 0)
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		defer file.Close()
		return readPrivateStateFile(file, maximum)
	}
	before, err := read()
	if err != nil {
		return err
	}
	after, err := update(append([]byte(nil), before...))
	if err != nil {
		return err
	}
	if (!remove && after == nil) || bytes.Equal(before, after) {
		return nil
	}
	if int64(len(after)) > maximum {
		return errors.New("private update size")
	}
	publish := func() error {
		current, err := read()
		if err != nil {
			return err
		}
		if !bytes.Equal(before, current) {
			return errors.New("private update revision conflict")
		}
		if remove {
			return directory.removePrivateReplacement(name)
		}
		token := make([]byte, 12)
		if _, err := rand.Read(token); err != nil {
			return err
		}
		temporary := ".update-" + hex.EncodeToString(token)
		if err := directory.writeNewFile(temporary, after); err != nil {
			return err
		}
		defer directory.removeFile(temporary)
		return directory.publishReplacement(temporary, name)
	}
	if guard != nil {
		return guard(publish)
	}
	return publish()
}

func privateUpdateFile(file *os.File) error {
	info, err := file.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || (runtime.GOOS != "windows" && info.Mode().Perm() != 0600) {
		return errors.New("private update file is not owner-private")
	}
	return validatePrivateUpdateIdentity(file)
}
