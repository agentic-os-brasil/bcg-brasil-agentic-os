// Package installerbundle implements the portable payload format used by the
// single-file Windows installer wrapper. It is intentionally independent of
// the release verifier: the wrapper authenticates the release only after the
// existing maestro-installer bridge has been launched.
package installerbundle

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	payloadFooterSize = 16 + 8 + sha256.Size
	maximumPayload    = int64(2 << 30)
	maximumFiles      = 100_000
	maximumFileBytes  = int64(512 << 20)
	maximumTotalBytes = int64(2 << 30)
)

// ValidateWindowsGUIExecutable verifies the PE subsystem used by a Windows
// visual entrypoint. A self-contained installer must not be built from a
// console-subsystem executable: that produces a transient console window and
// hides the wizard's startup failure from the owner.
func ValidateWindowsGUIExecutable(path string) error {
	file, err := openRegular(path)
	if err != nil {
		return err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	if info.Size() < 64 {
		return errors.New("Windows executable is too small to be a PE file")
	}
	read16 := func(offset int64) (uint16, error) {
		if offset < 0 || offset > info.Size()-2 {
			return 0, errors.New("PE field lies outside the Windows executable")
		}
		var body [2]byte
		if _, err := file.ReadAt(body[:], offset); err != nil {
			return 0, err
		}
		return binary.LittleEndian.Uint16(body[:]), nil
	}
	read32 := func(offset int64) (uint32, error) {
		if offset < 0 || offset > info.Size()-4 {
			return 0, errors.New("PE field lies outside the Windows executable")
		}
		var body [4]byte
		if _, err := file.ReadAt(body[:], offset); err != nil {
			return 0, err
		}
		return binary.LittleEndian.Uint32(body[:]), nil
	}
	var dos [2]byte
	if _, err := file.ReadAt(dos[:], 0); err != nil || dos != [2]byte{'M', 'Z'} {
		return errors.New("Windows executable is missing the PE DOS signature")
	}
	peOffset, err := read32(0x3c)
	if err != nil || int64(peOffset) < 64 || int64(peOffset) > info.Size()-24 {
		return errors.New("Windows executable has an invalid PE header offset")
	}
	var signature [4]byte
	if _, err := file.ReadAt(signature[:], int64(peOffset)); err != nil || signature != [4]byte{'P', 'E', 0, 0} {
		return errors.New("Windows executable is missing the PE signature")
	}
	optionalSize, err := read16(int64(peOffset) + 20)
	if err != nil {
		return err
	}
	optionalOffset := int64(peOffset) + 24
	if optionalSize < 70 || optionalOffset+int64(optionalSize) > info.Size() {
		return errors.New("Windows executable has an invalid PE optional header")
	}
	magic, err := read16(optionalOffset)
	if err != nil {
		return err
	}
	if magic != 0x10b && magic != 0x20b {
		return errors.New("Windows executable uses an unsupported PE optional-header format")
	}
	subsystem, err := read16(optionalOffset + 68)
	if err != nil {
		return err
	}
	if subsystem != 2 { // IMAGE_SUBSYSTEM_WINDOWS_GUI
		return fmt.Errorf("Windows executable uses PE subsystem %d; expected Windows GUI (2)", subsystem)
	}
	return nil
}

var payloadMagic = [16]byte{'M', 'A', 'E', 'S', 'T', 'R', 'O', '-', 'S', 'F', 'X', '-', 'V', '1', 0, 0}

// PayloadInfo describes the compressed payload appended to an executable.
type PayloadInfo struct {
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

// PackDirectory writes a deterministic gzip-compressed tar archive of root.
// Symlinks, reparse-like entries, unsupported file types and oversized input
// are rejected before they can become part of an installer executable.
func PackDirectory(root string, output io.Writer) (PayloadInfo, error) {
	rootInfo, err := os.Lstat(root)
	if err != nil {
		return PayloadInfo{}, err
	}
	if !rootInfo.IsDir() || rootInfo.Mode()&os.ModeSymlink != 0 {
		return PayloadInfo{}, errors.New("installer payload root must be a regular directory")
	}
	hash := sha256.New()
	counting := &countingWriter{writer: io.MultiWriter(output, hash)}
	gzipWriter, err := gzip.NewWriterLevel(counting, gzip.BestCompression)
	if err != nil {
		return PayloadInfo{}, err
	}
	gzipWriter.Header.ModTime = time.Unix(0, 0).UTC()
	gzipWriter.Header.OS = 255
	tarWriter := tar.NewWriter(gzipWriter)
	files := 0
	var total int64
	writeErr := filepath.WalkDir(root, func(current string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if current == root {
			return nil
		}
		files++
		if files > maximumFiles {
			return errors.New("installer payload contains too many entries")
		}
		relative, err := filepath.Rel(root, current)
		if err != nil {
			return err
		}
		archiveName := filepath.ToSlash(relative)
		if !safeArchivePath(archiveName) {
			return fmt.Errorf("unsafe installer payload path %q", archiveName)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("installer payload contains symlink: %s", archiveName)
		}
		header := &tar.Header{
			Name:       archiveName,
			Mode:       0o700,
			ModTime:    time.Unix(0, 0).UTC(),
			AccessTime: time.Unix(0, 0).UTC(),
			ChangeTime: time.Unix(0, 0).UTC(),
			Uid:        0,
			Gid:        0,
			Format:     tar.FormatPAX,
		}
		if entry.IsDir() {
			header.Name += "/"
			header.Typeflag = tar.TypeDir
			return tarWriter.WriteHeader(header)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("installer payload contains unsupported file: %s", archiveName)
		}
		if info.Size() < 0 || info.Size() > maximumFileBytes {
			return fmt.Errorf("installer payload file exceeds size limit: %s", archiveName)
		}
		total += info.Size()
		if total > maximumTotalBytes {
			return errors.New("installer payload exceeds extracted size limit")
		}
		header.Typeflag = tar.TypeReg
		header.Mode = 0o600
		header.Size = info.Size()
		if err := tarWriter.WriteHeader(header); err != nil {
			return err
		}
		file, err := os.Open(current)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(tarWriter, file)
		closeErr := file.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	})
	if writeErr == nil {
		writeErr = tarWriter.Close()
	}
	if closeErr := gzipWriter.Close(); writeErr == nil {
		writeErr = closeErr
	}
	if writeErr != nil {
		return PayloadInfo{}, writeErr
	}
	if counting.count == 0 || counting.count > maximumPayload {
		return PayloadInfo{}, errors.New("installer payload exceeds compressed size limit")
	}
	return PayloadInfo{Size: counting.count, SHA256: hex.EncodeToString(hash.Sum(nil))}, nil
}

// AppendPayload creates output by copying the executable base, appending the
// compressed payload and writing a fixed-size footer. Existing output is never
// replaced.
func AppendPayload(basePath, payloadPath, outputPath string) (PayloadInfo, error) {
	if filepath.Clean(basePath) == filepath.Clean(outputPath) || filepath.Clean(payloadPath) == filepath.Clean(outputPath) {
		return PayloadInfo{}, errors.New("installer output must differ from its inputs")
	}
	base, err := openRegular(basePath)
	if err != nil {
		return PayloadInfo{}, fmt.Errorf("open installer base: %w", err)
	}
	defer base.Close()
	payload, err := openRegular(payloadPath)
	if err != nil {
		return PayloadInfo{}, fmt.Errorf("open installer payload: %w", err)
	}
	defer payload.Close()
	payloadInfo, err := fileDigest(payload)
	if err != nil {
		return PayloadInfo{}, fmt.Errorf("hash installer payload: %w", err)
	}
	if payloadInfo.Size == 0 || payloadInfo.Size > maximumPayload {
		return PayloadInfo{}, errors.New("installer payload has invalid size")
	}
	if _, err := payload.Seek(0, io.SeekStart); err != nil {
		return PayloadInfo{}, err
	}
	output, err := os.OpenFile(outputPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o755)
	if err != nil {
		return PayloadInfo{}, fmt.Errorf("create single-file installer: %w", err)
	}
	succeeded := false
	defer func() {
		_ = output.Close()
		if !succeeded {
			_ = os.Remove(outputPath)
		}
	}()
	if _, err := io.Copy(output, base); err != nil {
		return PayloadInfo{}, err
	}
	if _, err := io.Copy(output, payload); err != nil {
		return PayloadInfo{}, err
	}
	var footer [payloadFooterSize]byte
	copy(footer[:16], payloadMagic[:])
	binary.LittleEndian.PutUint64(footer[16:24], uint64(payloadInfo.Size))
	digest, err := hex.DecodeString(payloadInfo.SHA256)
	if err != nil || len(digest) != sha256.Size {
		return PayloadInfo{}, errors.New("installer payload digest is invalid")
	}
	copy(footer[24:], digest)
	if _, err := output.Write(footer[:]); err != nil {
		return PayloadInfo{}, err
	}
	if err := output.Sync(); err != nil {
		return PayloadInfo{}, err
	}
	succeeded = true
	return payloadInfo, nil
}

// ExtractExecutable verifies and extracts the appended payload into a fresh
// user-temporary directory. The caller owns cleanup and may call it repeatedly.
func ExtractExecutable(executablePath string) (string, func(), error) {
	file, err := openRegular(executablePath)
	if err != nil {
		return "", func() {}, fmt.Errorf("open self-contained installer: %w", err)
	}
	defer file.Close()
	footer, payloadOffset, err := readFooter(file)
	if err != nil {
		return "", func() {}, err
	}
	if err := verifyPayloadDigest(file, payloadOffset, footer.size, footer.digest); err != nil {
		return "", func() {}, err
	}
	target, err := os.MkdirTemp("", "maestro-installer-sfx-")
	if err != nil {
		return "", func() {}, fmt.Errorf("create temporary installer directory: %w", err)
	}
	cleanup := cleanupFunc(target)
	reader := io.NewSectionReader(file, payloadOffset, footer.size)
	if err := extractArchive(reader, target); err != nil {
		cleanup()
		return "", func() {}, err
	}
	bridge := filepath.Join(target, "maestro-installer.exe")
	bridgeInfo, err := os.Lstat(bridge)
	if err != nil {
		cleanup()
		return "", func() {}, fmt.Errorf("installer_package_incomplete: embedded bridge is missing: %w", err)
	}
	if !bridgeInfo.Mode().IsRegular() || bridgeInfo.Mode()&os.ModeSymlink != 0 {
		cleanup()
		return "", func() {}, errors.New("installer_package_incomplete: embedded bridge is not a regular file")
	}
	return target, cleanup, nil
}

type footer struct {
	size   int64
	digest [sha256.Size]byte
}

func readFooter(file *os.File) (footer, int64, error) {
	stat, err := file.Stat()
	if err != nil {
		return footer{}, 0, err
	}
	if stat.Size() <= payloadFooterSize {
		return footer{}, 0, errors.New("self-contained installer payload footer is missing")
	}
	if _, err := file.Seek(-payloadFooterSize, io.SeekEnd); err != nil {
		return footer{}, 0, err
	}
	var encoded [payloadFooterSize]byte
	if _, err := io.ReadFull(file, encoded[:]); err != nil {
		return footer{}, 0, err
	}
	if !bytesEqual(encoded[:16], payloadMagic[:]) {
		return footer{}, 0, errors.New("self-contained installer payload footer is invalid")
	}
	size := int64(binary.LittleEndian.Uint64(encoded[16:24]))
	if size <= 0 || size > maximumPayload || size > stat.Size()-payloadFooterSize {
		return footer{}, 0, errors.New("self-contained installer payload size is invalid")
	}
	var digest [sha256.Size]byte
	copy(digest[:], encoded[24:])
	return footer{size: size, digest: digest}, stat.Size() - payloadFooterSize - size, nil
}

func verifyPayloadDigest(file *os.File, offset, size int64, expected [sha256.Size]byte) error {
	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		return err
	}
	hash := sha256.New()
	if _, err := io.CopyN(hash, file, size); err != nil {
		return err
	}
	if !bytesEqual(hash.Sum(nil), expected[:]) {
		return errors.New("self-contained installer payload digest mismatch")
	}
	return nil
}

func extractArchive(reader io.Reader, target string) error {
	gzipReader, err := gzip.NewReader(reader)
	if err != nil {
		return fmt.Errorf("read self-contained installer payload: %w", err)
	}
	defer gzipReader.Close()
	tarReader := tar.NewReader(gzipReader)
	seen := make(map[string]struct{})
	files := 0
	var total int64
	for {
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("read self-contained installer archive: %w", err)
		}
		name := strings.TrimSuffix(header.Name, "/")
		if !safeArchivePath(name) {
			return fmt.Errorf("unsafe self-contained installer path %q", header.Name)
		}
		if _, exists := seen[name]; exists {
			return fmt.Errorf("duplicate self-contained installer entry %q", name)
		}
		seen[name] = struct{}{}
		files++
		if files > maximumFiles {
			return errors.New("self-contained installer contains too many entries")
		}
		destination := filepath.Join(target, filepath.FromSlash(name))
		switch header.Typeflag {
		case tar.TypeDir:
			if header.Size != 0 {
				return fmt.Errorf("directory entry has content: %s", name)
			}
			if err := os.MkdirAll(destination, 0o700); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if header.Size < 0 || header.Size > maximumFileBytes {
				return fmt.Errorf("self-contained installer file exceeds size limit: %s", name)
			}
			total += header.Size
			if total > maximumTotalBytes {
				return errors.New("self-contained installer extracted size limit exceeded")
			}
			if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
				return err
			}
			file, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o700)
			if err != nil {
				return err
			}
			written, copyErr := io.CopyN(file, tarReader, header.Size)
			closeErr := file.Close()
			if copyErr != nil {
				return fmt.Errorf("extract %s: %w", name, copyErr)
			}
			if closeErr != nil {
				return closeErr
			}
			if written != header.Size {
				return fmt.Errorf("extract %s: short file", name)
			}
		case tar.TypeSymlink, tar.TypeLink:
			return fmt.Errorf("unsafe self-contained installer entry %s", name)
		default:
			return fmt.Errorf("unsupported self-contained installer entry %s", name)
		}
	}
	if files == 0 {
		return errors.New("self-contained installer archive is empty")
	}
	return nil
}

func safeArchivePath(value string) bool {
	if value == "" || strings.Contains(value, `\`) || strings.ContainsRune(value, 0) || strings.HasPrefix(value, "/") {
		return false
	}
	first := strings.Split(value, "/")[0]
	if strings.Contains(first, ":") {
		return false
	}
	clean := path.Clean(value)
	if clean != value || clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return false
	}
	for _, component := range strings.Split(clean, "/") {
		if component == "" || component == "." || component == ".." {
			return false
		}
	}
	return true
}

func openRegular(path string) (*os.File, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("path must be a regular file")
	}
	return os.Open(path)
}

func fileDigest(file *os.File) (PayloadInfo, error) {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return PayloadInfo{}, err
	}
	hash := sha256.New()
	count, err := io.Copy(hash, file)
	if err != nil {
		return PayloadInfo{}, err
	}
	return PayloadInfo{Size: count, SHA256: hex.EncodeToString(hash.Sum(nil))}, nil
}

func bytesEqual(left, right []byte) bool {
	if len(left) != len(right) {
		return false
	}
	var result byte
	for index := range left {
		result |= left[index] ^ right[index]
	}
	return result == 0
}

type countingWriter struct {
	writer io.Writer
	count  int64
}

func (writer *countingWriter) Write(payload []byte) (int, error) {
	count, err := writer.writer.Write(payload)
	writer.count += int64(count)
	return count, err
}

func cleanupFunc(target string) func() {
	var once sync.Once
	return func() {
		once.Do(func() { _ = os.RemoveAll(target) })
	}
}
