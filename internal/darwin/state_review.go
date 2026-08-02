package darwin

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	maxReviewedStateDocuments = 16
	maxStateDocumentBytes     = 12 * 1024
	maxStateDocumentLines     = 240
)

// reviewStateDocuments reads only bounded local control-plane files arranged
// as <root>/<registered-agent-id>/states.md. It retains neither filenames nor
// bodies: the health packet receives only an aggregate breach count. The
// caller supplies a Darwin-owned root, never a client workspace or dossier.
func reviewStateDocuments(root string) ProductSurface {
	if strings.TrimSpace(root) == "" {
		return ProductSurface{State: "healthy"}
	}
	info, err := os.Lstat(root)
	if errors.Is(err, os.ErrNotExist) {
		return ProductSurface{State: "healthy"}
	}
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return ProductSurface{State: "failed", Count: 1}
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) > maxReviewedStateDocuments {
		return ProductSurface{State: "failed", Count: 1}
	}
	overlong := 0
	for _, entry := range entries {
		if !idPattern.MatchString(entry.Name()) || entry.Type()&os.ModeSymlink != 0 || !entry.IsDir() {
			return ProductSurface{State: "failed", Count: 1}
		}
		path := filepath.Join(root, entry.Name(), "states.md")
		fileInfo, statErr := os.Lstat(path)
		if errors.Is(statErr, os.ErrNotExist) {
			continue
		}
		if statErr != nil || fileInfo.Mode()&os.ModeSymlink != 0 || !fileInfo.Mode().IsRegular() {
			return ProductSurface{State: "failed", Count: 1}
		}
		if fileInfo.Size() > maxStateDocumentBytes || stateDocumentHasTooManyLines(path) {
			overlong++
		}
	}
	if overlong == 0 {
		return ProductSurface{State: "healthy"}
	}
	return ProductSurface{State: "warning", Count: overlong}
}

func stateDocumentHasTooManyLines(path string) bool {
	file, err := os.Open(path)
	if err != nil {
		return true
	}
	defer file.Close()
	limited := io.LimitReader(file, maxStateDocumentBytes+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return true
	}
	return len(body) > maxStateDocumentBytes || strings.Count(string(body), "\n")+1 > maxStateDocumentLines
}
