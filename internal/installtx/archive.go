package installtx

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	maximumBundleFiles = 10_000
	maximumBundleBytes = 512 << 20
)

func extractBundle(reader io.Reader, target string) error {
	gzipReader, err := gzip.NewReader(reader)
	if err != nil {
		return err
	}
	defer gzipReader.Close()
	tarReader := tar.NewReader(gzipReader)
	files := 0
	var total int64
	for {
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		if !safeArchivePath(header.Name) {
			return fmt.Errorf("unsafe bundle path %q", header.Name)
		}
		files++
		total += header.Size
		if files > maximumBundleFiles || header.Size < 0 || total > maximumBundleBytes {
			return errors.New("bundle extraction limits exceeded")
		}
		destination := filepath.Join(target, filepath.FromSlash(header.Name))
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(destination, 0o755); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
				return err
			}
			file, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
			if err != nil {
				return err
			}
			written, copyErr := io.CopyN(file, tarReader, header.Size)
			closeErr := file.Close()
			if copyErr != nil || written != header.Size {
				return fmt.Errorf("extract %s: %w", header.Name, copyErr)
			}
			if closeErr != nil {
				return closeErr
			}
		default:
			return fmt.Errorf("unsupported bundle entry type for %s", header.Name)
		}
	}
	if files == 0 {
		return errors.New("bundle archive is empty")
	}
	return nil
}

func safeArchivePath(value string) bool {
	if value == "" || strings.Contains(value, `\`) || filepath.IsAbs(value) {
		return false
	}
	clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(value)))
	return clean == value && clean != "." && clean != ".." && !strings.HasPrefix(clean, "../")
}
