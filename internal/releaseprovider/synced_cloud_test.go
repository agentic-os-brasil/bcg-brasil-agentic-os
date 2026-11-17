package releaseprovider

import "testing"

func TestSyncedCloudSourceTagPolicy(t *testing.T) {
	for _, directory := range []bool{false, true} {
		attributes := uint32(0)
		if directory {
			attributes = 0x10
		}
		cloud, err := syncedCloudSourceKind(attributes, 0, directory)
		if err != nil || cloud {
			t.Fatalf("ordinary source refused or relabeled: %v", err)
		}
		for variant := uint32(0); variant < 16; variant++ {
			cloud, err := syncedCloudSourceKind(attributes|0x400, 0x9000001a|(variant<<12), directory)
			if err != nil || !cloud {
				t.Fatalf("supported Cloud Files variant %x refused: %v", variant, err)
			}
		}
		for _, tag := range []uint32{0, 0xa0000003, 0xa000000c, 0x80000015, 0x80000021, 0x9001001a, 0xb000001a} {
			if _, err := syncedCloudSourceKind(attributes|0x400, tag, directory); err == nil {
				t.Fatalf("unsupported reparse tag %x admitted", tag)
			}
		}
		for _, tag := range []uint32{1, 0x9000001a} {
			if _, err := syncedCloudSourceKind(attributes, tag, directory); err == nil {
				t.Fatal("inconsistent nonreparse attributes admitted")
			}
		}
		for _, unsafe := range []uint32{0x1000, 0x40000, 0x400000} {
			if _, err := syncedCloudSourceKind(attributes|unsafe, 0, directory); err == nil {
				t.Fatal("non-cloud recall/offline source admitted")
			}
		}
		if _, err := syncedCloudSourceKind(attributes^0x10, 0, directory); err == nil {
			t.Fatal("wrong filesystem type admitted")
		}
	}
}

func TestSyncedCloudReadyRequiresHydratedVerifiedState(t *testing.T) {
	for _, state := range []uint32{1, 1 | 8, 1 | 2 | 4 | 8} {
		if !syncedCloudReady(0x400, state) {
			t.Fatalf("ready placeholder state %x denied", state)
		}
	}
	for _, state := range []uint32{0, 8, 1 | 16, 1 | 32, 1 | 16 | 32, 0xffffffff, 1 | 64} {
		if syncedCloudReady(0x400, state) {
			t.Fatalf("unready/invalid placeholder state %x admitted", state)
		}
	}
	for _, attributes := range []uint32{0x400 | 0x1000, 0x400 | 0x40000, 0x400 | 0x400000} {
		if syncedCloudReady(attributes, 1|8) {
			t.Fatalf("recall/offline attributes %x admitted", attributes)
		}
	}
}
