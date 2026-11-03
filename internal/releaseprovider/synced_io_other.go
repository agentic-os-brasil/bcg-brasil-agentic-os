//go:build !darwin && !linux && !windows

package releaseprovider

import "os"

func syncedSingleLink(*os.File, os.FileInfo) bool { return false }
