package priorworksync

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"time"
)

type presenceClaim struct {
	root      *os.Root
	name      string
	file      *os.File
	info      os.FileInfo
	guardName string
	guard     *os.File
	guardInfo os.FileInfo
}

type claimRecord struct {
	SchemaVersion int       `json:"schema_version"`
	PID           int       `json:"pid"`
	Token         string    `json:"token"`
	CreatedAt     time.Time `json:"created_at"`
}

func acquirePresenceClaim(root string) (*presenceClaim, error) {
	schedulerRoot, err := os.OpenRoot(root)
	if err != nil {
		return nil, err
	}
	defer schedulerRoot.Close()
	if err := schedulerRoot.Mkdir("claims", 0o700); err != nil && !errors.Is(err, os.ErrExist) {
		return nil, err
	}
	claimsRoot, err := schedulerRoot.OpenRoot("claims")
	if err != nil {
		return nil, err
	}
	cleanupRoot := true
	defer func() {
		if cleanupRoot {
			_ = claimsRoot.Close()
		}
	}()

	guardName := JobID + ".guard"
	guard, guardInfo, err := openClaimGuard(claimsRoot, guardName)
	if err != nil {
		return nil, err
	}
	if err := tryLockClaimGuard(guard); err != nil {
		_ = guard.Close()
		if errors.Is(err, errClaimGuardBusy) {
			return nil, ErrPresenceClaimed
		}
		return nil, err
	}
	cleanupGuard := true
	defer func() {
		if cleanupGuard {
			_ = unlockClaimGuard(guard)
			_ = guard.Close()
		}
	}()

	name := JobID + ".lock"
	if err := claimsRoot.Remove(name); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	claim, err := createPresenceClaim(claimsRoot, name)
	if err != nil {
		return nil, err
	}
	claim.root = claimsRoot
	claim.name = name
	claim.guardName = guardName
	claim.guard = guard
	claim.guardInfo = guardInfo
	cleanupRoot = false
	cleanupGuard = false
	return claim, nil
}

func createPresenceClaim(root *os.Root, name string) (*presenceClaim, error) {
	var tokenBytes [16]byte
	if _, err := rand.Read(tokenBytes[:]); err != nil {
		return nil, err
	}
	record := claimRecord{
		SchemaVersion: 1, PID: os.Getpid(),
		Token: hex.EncodeToString(tokenBytes[:]), CreatedAt: time.Now().UTC(),
	}
	body, err := json.Marshal(record)
	if err != nil {
		return nil, err
	}
	file, err := root.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return nil, err
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = file.Close()
			_ = root.Remove(name)
		}
	}()
	if _, err := file.Write(append(body, '\n')); err != nil {
		return nil, err
	}
	if err := file.Sync(); err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	cleanup = false
	return &presenceClaim{file: file, info: info}, nil
}

func openClaimGuard(root *os.Root, name string) (*os.File, os.FileInfo, error) {
	for range 3 {
		before, err := root.Lstat(name)
		missing := errors.Is(err, os.ErrNotExist)
		if err != nil && !missing {
			return nil, nil, err
		}
		if !missing && (before.Mode()&os.ModeSymlink != 0 || !before.Mode().IsRegular()) {
			return nil, nil, errors.New("invalid prior-work scheduler claim guard")
		}
		flags := os.O_RDWR
		if missing {
			flags |= os.O_CREATE | os.O_EXCL
		}
		file, err := root.OpenFile(name, flags, 0o600)
		if errors.Is(err, os.ErrExist) || errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, nil, err
		}
		info, err := file.Stat()
		if err != nil {
			_ = file.Close()
			return nil, nil, err
		}
		after, err := root.Lstat(name)
		if err != nil {
			_ = file.Close()
			return nil, nil, err
		}
		if after.Mode()&os.ModeSymlink != 0 || !after.Mode().IsRegular() || !os.SameFile(info, after) {
			_ = file.Close()
			return nil, nil, errors.New("prior-work scheduler claim guard changed during secure open")
		}
		return file, info, nil
	}
	return nil, nil, ErrPresenceClaimed
}

func (claim *presenceClaim) Valid() error {
	if claim == nil || claim.root == nil || claim.file == nil || claim.guard == nil {
		return ErrPresenceClaimed
	}
	pathInfo, err := claim.root.Lstat(claim.name)
	if err != nil {
		return ErrPresenceClaimed
	}
	fileInfo, err := claim.file.Stat()
	if err != nil || !os.SameFile(pathInfo, fileInfo) || !os.SameFile(claim.info, fileInfo) {
		return ErrPresenceClaimed
	}
	guardPathInfo, err := claim.root.Lstat(claim.guardName)
	if err != nil {
		return ErrPresenceClaimed
	}
	guardFileInfo, err := claim.guard.Stat()
	if err != nil || !os.SameFile(guardPathInfo, guardFileInfo) || !os.SameFile(claim.guardInfo, guardFileInfo) {
		return ErrPresenceClaimed
	}
	return nil
}

func (claim *presenceClaim) Release() {
	if claim == nil || claim.file == nil {
		return
	}
	if claim.Valid() == nil {
		_ = claim.root.Remove(claim.name)
	}
	_ = claim.file.Close()
	_ = unlockClaimGuard(claim.guard)
	_ = claim.guard.Close()
	_ = claim.root.Close()
	claim.file = nil
	claim.guard = nil
	claim.root = nil
}
