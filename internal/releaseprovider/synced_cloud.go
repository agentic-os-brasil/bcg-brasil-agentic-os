package releaseprovider

import "errors"

// These finite Windows transport facts grant no publisher or state authority.
func syncedCloudSourceKind(attributes, tag uint32, directory bool) (bool, error) {
	if (attributes&0x10 != 0) != directory {
		return false, errors.New("synced source filesystem type differs")
	}
	if attributes&0x400 != 0 {
		if tag&^uint32(0x0000f000) != 0x9000001a {
			return false, errors.New("synced source reparse tag is unsupported")
		}
		return true, nil
	}
	if tag != 0 || attributes&(0x1000|0x40000|0x400000) != 0 {
		return false, errors.New("synced source is an unsupported offline or recall object")
	}
	return false, nil
}

func syncedCloudReady(attributes, state uint32) bool {
	return state&1 != 0 && state&^uint32(0x3f) == 0 && state&(0x10|0x20) == 0 && attributes&(0x1000|0x40000|0x400000) == 0
}
