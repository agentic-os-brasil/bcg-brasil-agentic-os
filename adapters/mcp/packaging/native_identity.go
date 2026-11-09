package packaging

import (
	"crypto/sha256"
	"debug/elf"
	"debug/macho"
	"debug/pe"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

// Host inspection does not copy a payload into a packet. Its independent bound
// admits the actual 239,695,888-byte minimum-release host without changing the
// server payload's 128 MiB limit or allocating the whole host in memory.
const maximumNativeHostBytes int64 = 512 << 20

func nativeHostIdentity(path string) (string, Target, error) {
	f, err := scheduler.OpenRegularFile(path)
	if err != nil {
		return "", Target{}, err
	}
	defer f.Close()
	before, err := f.Stat()
	// Windows file modes do not encode executable permission. Its actual PE
	// executable header is checked below; the supported CLI probe must still run.
	if err != nil || !before.Mode().IsRegular() || (runtime.GOOS != "windows" && before.Mode().Perm()&0111 == 0) || before.Size() <= 0 || before.Size() > maximumNativeHostBytes {
		return "", Target{}, errors.New("bounded executable native host required")
	}
	// Native loaders describe the format in bounded headers. Refuse unusual
	// headers rather than permitting attacker-controlled unbounded allocations.
	header := &nativeMetadataReader{source: io.NewSectionReader(f, 0, before.Size()), remaining: 32 << 20}
	var target Target
	if m, err := macho.NewFile(header); err == nil && m.Type == macho.TypeExec {
		switch m.Cpu {
		case macho.CpuArm64:
			target = Target{"darwin", "arm64"}
		case macho.CpuAmd64:
			target = Target{"darwin", "amd64"}
		}
	} else if p, err := pe.NewFile(header); err == nil && p.Characteristics&0x0002 != 0 && p.Characteristics&0x2000 == 0 {
		switch p.Machine {
		case pe.IMAGE_FILE_MACHINE_AMD64:
			target = Target{"windows", "amd64"}
		case pe.IMAGE_FILE_MACHINE_ARM64:
			target = Target{"windows", "arm64"}
		}
	} else if e, err := elf.NewFile(header); err == nil && e.Class == elf.ELFCLASS64 && (e.Type == elf.ET_EXEC || e.Type == elf.ET_DYN) {
		switch e.Machine {
		case elf.EM_X86_64:
			target = Target{"linux", "amd64"}
		case elf.EM_AARCH64:
			target = Target{"linux", "arm64"}
		}
	}
	if target.OS == "" {
		return "", Target{}, errors.New("supported bounded 64-bit native host header required")
	}
	hash := sha256.New()
	count, err := io.Copy(hash, io.LimitReader(f, maximumNativeHostBytes+1))
	if err != nil || count != before.Size() {
		return "", Target{}, errors.New("native host changed while hashing")
	}
	after, err := f.Stat()
	if err != nil || !sameHostMetadata(before, after) {
		return "", Target{}, errors.New("native host changed while inspecting")
	}
	// Reopen through the same no-follow admission to bind the selected path to
	// the held inode, including replacement by byte-identical executable files.
	selected, err := scheduler.OpenRegularFile(path)
	if err != nil {
		return "", Target{}, err
	}
	current, statErr := selected.Stat()
	closeErr := selected.Close()
	if statErr != nil || closeErr != nil || !sameHostMetadata(before, current) {
		return "", Target{}, errors.New("selected native host identity changed")
	}
	return fmt.Sprintf("%x", hash.Sum(nil)), target, nil
}

// Native symbol tables may be near the end of a large executable. Bound total
// metadata reads rather than its physical offset; a first-MiB window would
// incorrectly reject normal compiled executables, including Go test helpers.
type nativeMetadataReader struct {
	source    io.ReaderAt
	remaining int64
}

func (r *nativeMetadataReader) ReadAt(body []byte, offset int64) (int, error) {
	if int64(len(body)) > r.remaining {
		return 0, errors.New("native format metadata limit")
	}
	r.remaining -= int64(len(body))
	return r.source.ReadAt(body, offset)
}

func sameHostMetadata(a, b os.FileInfo) bool {
	return a != nil && b != nil && os.SameFile(a, b) && a.Size() == b.Size() && a.Mode() == b.Mode() && a.ModTime().Equal(b.ModTime())
}
