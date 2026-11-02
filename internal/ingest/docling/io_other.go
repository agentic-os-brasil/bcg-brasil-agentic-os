//go:build !darwin && !linux

package docling

import "os"

func singleLink(os.FileInfo) bool { return false }
