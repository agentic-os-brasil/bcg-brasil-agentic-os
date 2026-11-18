package packaging

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"path/filepath"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/claudeagents"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

// VerifyDesktopMethodFrontend uses this executable's canonical method payload,
// never a caller-supplied inventory, and grants no host installation evidence.
func VerifyDesktopMethodFrontend(path, version string) (DesktopFrontendReport, error) {
	return verifyDesktopMethodFrontend(path, version, false)
}

func VerifyDesktopLiveMethodFrontend(path, version string) (DesktopFrontendReport, error) {
	return verifyDesktopMethodFrontend(path, version, true)
}

func verifyDesktopMethodFrontend(path, version string, live bool) (DesktopFrontendReport, error) {
	report := DesktopFrontendReport{SchemaVersion: 1, Name: claudeagents.DesktopMethodNamespace, Version: version, Mode: "instructions_only", NativeQualification: "unavailable"}
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return report, errors.New("absolute Desktop methods archive required")
	}
	files, err := desktopFrontendPayload(version)
	if err == nil && live {
		files, err = desktopLiveFrontendPayload(version)
		report.BootstrapProtocol = DesktopLiveMethodProtocol
	}
	if err != nil {
		return report, err
	}
	f, err := scheduler.OpenRegularFile(path)
	if err != nil {
		return report, err
	}
	if err := scheduler.ValidateSingleLink(f); err != nil {
		f.Close()
		return report, err
	}
	body, err := io.ReadAll(io.LimitReader(f, 16<<20+1))
	closeErr := f.Close()
	if err != nil || closeErr != nil {
		return report, errors.Join(err, closeErr)
	}
	if len(body) > 16<<20 {
		return report, errors.New("Desktop methods archive exceeds bound")
	}
	z, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		return report, err
	}
	if len(z.File) != len(files) {
		return report, errors.New("Desktop methods inventory differs")
	}
	seen := map[string]bool{}
	for _, entry := range z.File {
		name := entry.Name
		prefix := report.Name + "/"
		if len(name) <= len(prefix) || name[:len(prefix)] != prefix || seen[name] || entry.Mode().Perm() != 0644 || !entry.Mode().IsRegular() {
			return report, errors.New("unexpected Desktop method entry")
		}
		seen[name] = true
		expected, ok := files[name[len(prefix):]]
		if !ok || entry.UncompressedSize64 != uint64(len(expected)) {
			return report, errors.New("Desktop method identity differs")
		}
		r, err := entry.Open()
		if err != nil {
			return report, err
		}
		actual, err := io.ReadAll(io.LimitReader(r, int64(len(expected))+1))
		closeErr := r.Close()
		if err != nil || closeErr != nil {
			return report, errors.Join(err, closeErr)
		}
		if !bytes.Equal(actual, expected) {
			return report, errors.New("Desktop method bytes differ from current canonical payload")
		}
		report.PayloadBytes += len(actual)
	}
	report.FileCount = len(files)
	report.Archive = filepath.Base(path)
	report.ArchiveSHA256 = fmt.Sprintf("%x", sha256.Sum256(body))
	return report, nil
}
