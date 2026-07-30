package releasepack

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

func TestBuildICOUsesPNGEntriesAndDeterministicOffsets(t *testing.T) {
	pngs := [][]byte{
		{0x89, 'P', 'N', 'G', 0x00},
		{0x89, 'P', 'N', 'G', 0x01, 0x02},
		{0x89, 'P', 'N', 'G', 0x03, 0x04, 0x05},
		{0x89, 'P', 'N', 'G', 0x06, 0x07, 0x08, 0x09},
		{0x89, 'P', 'N', 'G', 0x0a, 0x0b, 0x0c, 0x0d, 0x0e},
	}
	got := buildICO(pngs)
	if binary.LittleEndian.Uint16(got[0:2]) != 0 || binary.LittleEndian.Uint16(got[2:4]) != 1 || binary.LittleEndian.Uint16(got[4:6]) != uint16(len(pngs)) {
		t.Fatalf("invalid ICO header: %v", got[:6])
	}
	offset := uint32(6 + 16*len(pngs))
	for index, png := range pngs {
		entry := got[6+index*16 : 6+(index+1)*16]
		if binary.LittleEndian.Uint32(entry[8:12]) != uint32(len(png)) || binary.LittleEndian.Uint32(entry[12:16]) != offset {
			t.Fatalf("entry %d has size/offset %d/%d, want %d/%d", index, binary.LittleEndian.Uint32(entry[8:12]), binary.LittleEndian.Uint32(entry[12:16]), len(png), offset)
		}
		if got[offset : offset+uint32(len(png))][0] != png[0] {
			t.Fatalf("entry %d payload is not PNG data", index)
		}
		offset += uint32(len(png))
	}
	if int(offset) != len(got) {
		t.Fatalf("ICO length = %d, final offset = %d", len(got), offset)
	}
}

func TestFileSHA256PropagatesMissingFile(t *testing.T) {
	_, err := fileSHA256(filepath.Join(t.TempDir(), "missing"))
	if err == nil {
		t.Fatal("fileSHA256 accepted a missing file")
	}
	path := filepath.Join(t.TempDir(), "icon.svg")
	if err := os.WriteFile(path, []byte("<svg/>"), 0o600); err != nil {
		t.Fatal(err)
	}
	digest, err := fileSHA256(path)
	if err != nil || digest == "" {
		t.Fatalf("fileSHA256 = %q, %v", digest, err)
	}
}
