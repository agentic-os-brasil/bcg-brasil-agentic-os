package packaging

import (
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestWeeklyIdentityFixedHeaderCountsAndFileBounds(t *testing.T) {
	path := filepath.Join(t.TempDir(), "synthetic-header")
	body := make([]byte, 64)
	binary.LittleEndian.PutUint32(body[0:4], 0xfeedfacf)
	binary.LittleEndian.PutUint32(body[4:8], 0x0100000c)
	binary.LittleEndian.PutUint32(body[12:16], 2)
	binary.LittleEndian.PutUint32(body[16:20], 1)
	binary.LittleEndian.PutUint32(body[20:24], 8)
	if err := os.WriteFile(path, body, 0600); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	target, err := weeklyIdentityTarget(file, int64(len(body)))
	if err != nil || target != (Target{"darwin", "arm64"}) {
		t.Fatal("fixed synthetic header not recognized", err)
	}
	for _, offset := range []int{12, 16, 20} {
		mutated := append([]byte(nil), body...)
		binary.LittleEndian.PutUint32(mutated[offset:offset+4], 0xffffffff)
		if err := os.WriteFile(path, mutated, 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := weeklyIdentityTarget(file, int64(len(mutated))); err == nil {
			t.Fatal("hostile fixed header accepted", offset)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err := weeklyIdentityHash(ctx, file, weeklyIdentityMaximumBytes()+1); err == nil {
		t.Fatal("oversized image read was attempted")
	}
	if runtime.GOOS == "darwin" && weeklyIdentityMaximumBytes() != maximumNativeHostBytes {
		t.Fatal("Darwin no longer reuses bounded native host ceiling")
	}
	if runtime.GOOS == "windows" && weeklyIdentityMaximumBytes() != 256<<20 {
		t.Fatal("Windows reviewed stricter image ceiling changed")
	}
	if maximumWeeklyIdentityBytes != 512<<20 {
		t.Fatal("unexpected public native reader ceiling")
	}
}
