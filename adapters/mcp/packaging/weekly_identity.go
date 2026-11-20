package packaging

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/processidentity"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

const maximumWeeklyIdentityBytes int64 = maximumNativeHostBytes

func weeklyIdentityMaximumBytes() int64 {
	if runtime.GOOS == "windows" {
		return 256 << 20
	}
	return maximumWeeklyIdentityBytes
}

var weeklyIdentitySHA = regexp.MustCompile(`^[a-f0-9]{64}$`)
var errWeeklyIdentityUnavailable = errors.New("weekly supported runtime identity unavailable")
var errWeeklyIdentityStaticSignature = errors.New("weekly static code identity verification failed")

// WeeklyRuntimeIdentity owns a held, statically verified image, never a model
// qualification or execution grant. Only concrete attempt composition may use it.
type WeeklyRuntimeIdentity struct {
	mu                 sync.Mutex
	file               *os.File
	info               os.FileInfo
	hint, path, sha256 string
	target             Target
	declaration        processidentity.WeeklyNativeDeclaration
	closed             bool
}

func InspectWeeklyRuntime(ctx context.Context, path, expectedSHA256 string, declaration processidentity.WeeklyNativeDeclaration) (result *WeeklyRuntimeIdentity, err error) {
	if err := weeklyIdentityContext(ctx); err != nil {
		return nil, err
	}
	if declaration.Validate() != nil || !weeklyIdentitySHA.MatchString(expectedSHA256) || !filepath.IsAbs(path) || filepath.Clean(path) != path || strings.ContainsAny(path, "\x00\r\n") || len(path) > 32768*4 {
		return nil, errWeeklyIdentityUnavailable
	}
	if declaration.Architecture != runtime.GOARCH || declaration.Topology == processidentity.WeeklyNativeDarwinStandalone && runtime.GOOS != "darwin" || declaration.Topology == processidentity.WeeklyNativeWindowsStandalone && runtime.GOOS != "windows" {
		return nil, errWeeklyIdentityUnavailable
	}
	f, err := scheduler.OpenRegularFile(path)
	if err != nil {
		return nil, errWeeklyIdentityUnavailable
	}
	accepted := false
	defer func() {
		if !accepted {
			err = errors.Join(err, f.Close())
		}
	}()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() < 1 || info.Size() > weeklyIdentityMaximumBytes() || runtime.GOOS != "windows" && info.Mode().Perm()&0111 == 0 {
		return nil, errWeeklyIdentityUnavailable
	}
	if err := scheduler.ValidateSingleLink(f); err != nil {
		return nil, errWeeklyIdentityUnavailable
	}
	canonical, err := weeklyIdentityCanonicalPath(f)
	if err != nil || !weeklyIdentityPathsMatch(path, canonical) {
		return nil, errWeeklyIdentityUnavailable
	}
	target, err := weeklyIdentityTarget(f, info.Size())
	if err != nil || target.OS != runtime.GOOS || target.Arch != declaration.Architecture {
		return nil, errWeeklyIdentityUnavailable
	}
	actual, err := weeklyIdentityHash(ctx, f, info.Size())
	if err != nil || actual != expectedSHA256 {
		return nil, errors.Join(errWeeklyIdentityUnavailable, err)
	}
	identity := &WeeklyRuntimeIdentity{file: f, info: info, hint: path, path: canonical, sha256: actual, target: target, declaration: declaration}
	if err := identity.verifyCurrentLocked(ctx); err != nil {
		return nil, err
	}
	if err := verifyWeeklyIdentitySignature(ctx, identity); err != nil {
		return nil, errors.Join(errWeeklyIdentityStaticSignature, err)
	}
	if err := identity.verifyCurrentLocked(ctx); err != nil {
		return nil, err
	}
	accepted = true
	return identity, nil
}

func (i *WeeklyRuntimeIdentity) Close() error {
	if i == nil {
		return nil
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.closed {
		return nil
	}
	i.closed = true
	if i.file == nil {
		return nil
	}
	return i.file.Close()
}
func (i *WeeklyRuntimeIdentity) VerifyCurrent(ctx context.Context) error {
	if i == nil {
		return errWeeklyIdentityUnavailable
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.verifyCurrentLocked(ctx)
}
func (i *WeeklyRuntimeIdentity) verifyCurrentLocked(ctx context.Context) error {
	if err := weeklyIdentityContext(ctx); err != nil {
		return err
	}
	if i == nil || i.closed || i.file == nil || i.info == nil || i.declaration.Validate() != nil || !weeklyIdentitySHA.MatchString(i.sha256) {
		return errWeeklyIdentityUnavailable
	}
	info, err := i.file.Stat()
	if err != nil || !sameHostMetadata(i.info, info) {
		return errWeeklyIdentityUnavailable
	}
	if err := i.verifyImagePathLocked(ctx, i.hint); err != nil {
		return err
	}
	actual, err := weeklyIdentityHash(ctx, i.file, i.info.Size())
	if err != nil || actual != i.sha256 {
		return errors.Join(errWeeklyIdentityUnavailable, err)
	}
	after, err := i.file.Stat()
	if err != nil || !sameHostMetadata(i.info, after) {
		return errWeeklyIdentityUnavailable
	}
	return weeklyIdentityContext(ctx)
}
func (i *WeeklyRuntimeIdentity) verifyImagePathLocked(ctx context.Context, path string) (resultErr error) {
	if i == nil || i.closed || i.file == nil || i.info == nil {
		return errWeeklyIdentityUnavailable
	}
	if err := weeklyIdentityContext(ctx); err != nil {
		return err
	}
	if !filepath.IsAbs(path) || filepath.Clean(path) != path || strings.ContainsAny(path, "\x00\r\n") {
		return errWeeklyIdentityUnavailable
	}
	f, err := scheduler.OpenRegularFile(path)
	if err != nil {
		return errWeeklyIdentityUnavailable
	}
	defer func() { resultErr = errors.Join(resultErr, f.Close()) }()
	info, err := f.Stat()
	if err != nil || !sameHostMetadata(i.info, info) {
		return errWeeklyIdentityUnavailable
	}
	canonical, err := weeklyIdentityCanonicalPath(f)
	if err != nil || !weeklyIdentityPathsMatch(path, canonical) || !weeklyIdentityPathsMatch(i.path, canonical) {
		return errWeeklyIdentityUnavailable
	}
	target, err := weeklyIdentityTarget(f, info.Size())
	if err != nil || target != i.target {
		return errWeeklyIdentityUnavailable
	}
	actual, err := weeklyIdentityHash(ctx, f, info.Size())
	if err != nil || actual != i.sha256 {
		return errors.Join(errWeeklyIdentityUnavailable, err)
	}
	after, err := f.Stat()
	if err != nil || !sameHostMetadata(i.info, after) {
		return errWeeklyIdentityUnavailable
	}
	return weeklyIdentityContext(ctx)
}
func weeklyIdentityContext(ctx context.Context) error {
	if ctx == nil {
		return errWeeklyIdentityUnavailable
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	deadline, ok := ctx.Deadline()
	if !ok || time.Until(deadline) > 120*time.Second {
		return errWeeklyIdentityUnavailable
	}
	if time.Until(deadline) <= 0 {
		return context.DeadlineExceeded
	}
	return nil
}
func weeklyIdentityHash(ctx context.Context, f *os.File, size int64) (string, error) {
	if size < 1 || size > weeklyIdentityMaximumBytes() {
		return "", errWeeklyIdentityUnavailable
	}
	hash := sha256.New()
	buffer := make([]byte, 64<<10)
	for offset := int64(0); offset < size; {
		if err := weeklyIdentityContext(ctx); err != nil {
			return "", err
		}
		want := min(int64(len(buffer)), size-offset)
		n, err := f.ReadAt(buffer[:want], offset)
		if n != int(want) || err != nil {
			return "", errWeeklyIdentityUnavailable
		}
		_, _ = hash.Write(buffer[:n])
		offset += int64(n)
	}
	return hex.EncodeToString(hash.Sum(nil)), weeklyIdentityContext(ctx)
}

// Fixed headers avoid allocation from attacker-chosen Mach-O load-command or PE
// section counts. Full platform signature verification remains separate.
func weeklyIdentityTarget(f *os.File, size int64) (Target, error) {
	var header [64]byte
	if size < 64 {
		return Target{}, errWeeklyIdentityUnavailable
	}
	if _, err := f.ReadAt(header[:], 0); err != nil {
		return Target{}, errWeeklyIdentityUnavailable
	}
	if binary.LittleEndian.Uint32(header[:4]) == 0xfeedfacf {
		cpu := binary.LittleEndian.Uint32(header[4:8])
		commands := binary.LittleEndian.Uint32(header[16:20])
		commandBytes := binary.LittleEndian.Uint32(header[20:24])
		if binary.LittleEndian.Uint32(header[12:16]) != 2 || commands < 1 || commands > 4096 || commandBytes < commands*8 || commandBytes > 32<<20 || int64(commandBytes)+32 > size {
			return Target{}, errWeeklyIdentityUnavailable
		}
		switch cpu {
		case 0x01000007:
			return Target{"darwin", "amd64"}, nil
		case 0x0100000c:
			return Target{"darwin", "arm64"}, nil
		}
	}
	if header[0] == 'M' && header[1] == 'Z' {
		offset := int64(binary.LittleEndian.Uint32(header[60:64]))
		if offset < 64 || offset > size-26 {
			return Target{}, errWeeklyIdentityUnavailable
		}
		var pe [26]byte
		if _, err := f.ReadAt(pe[:], offset); err != nil {
			return Target{}, errWeeklyIdentityUnavailable
		}
		sections := binary.LittleEndian.Uint16(pe[6:8])
		characteristics := binary.LittleEndian.Uint16(pe[22:24])
		if string(pe[:4]) != "PE\x00\x00" || sections < 1 || sections > 96 || characteristics&2 == 0 || characteristics&0x2000 != 0 || binary.LittleEndian.Uint16(pe[24:26]) != 0x20b {
			return Target{}, errWeeklyIdentityUnavailable
		}
		switch binary.LittleEndian.Uint16(pe[4:6]) {
		case 0x8664:
			return Target{"windows", "amd64"}, nil
		case 0xaa64:
			return Target{"windows", "arm64"}, nil
		}
	}
	return Target{}, errWeeklyIdentityUnavailable
}
