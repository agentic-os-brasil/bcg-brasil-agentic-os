//go:build !darwin && !linux && !windows

package docling

import "os"

func fileReadSupported() bool               { return false }
func singleLink(*os.File, os.FileInfo) bool { return false }
