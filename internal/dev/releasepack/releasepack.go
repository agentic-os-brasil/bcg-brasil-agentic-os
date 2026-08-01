// Package releasepack contains factory-only release candidate tooling. It must
// never be imported by the distributed CLI or base bundle.
package releasepack

import (
	"archive/tar"
	"compress/gzip"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Allowlist struct {
	SchemaVersion int          `json:"schema_version"`
	Files         []BundleFile `json:"files"`
}

type BundleFile struct {
	Source string `json:"source"`
	Path   string `json:"path"`
}

func LoadAllowlist(path string) (Allowlist, error) {
	file, err := os.Open(path)
	if err != nil {
		return Allowlist{}, err
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	var allowlist Allowlist
	if err := decoder.Decode(&allowlist); err != nil {
		return Allowlist{}, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return Allowlist{}, errors.New("distribution allowlist contains multiple JSON values")
		}
		return Allowlist{}, err
	}
	if err := validateAllowlist(allowlist); err != nil {
		return Allowlist{}, err
	}
	return allowlist, nil
}

func BuildBundle(root string, allowlist Allowlist, output io.Writer) error {
	if err := validateAllowlist(allowlist); err != nil {
		return err
	}
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return err
	}
	files := append([]BundleFile(nil), allowlist.Files...)
	sort.Slice(files, func(left, right int) bool { return files[left].Path < files[right].Path })

	gzipWriter, err := gzip.NewWriterLevel(output, gzip.BestCompression)
	if err != nil {
		return err
	}
	gzipWriter.Header.ModTime = time.Unix(0, 0).UTC()
	gzipWriter.Header.OS = 255
	tarWriter := tar.NewWriter(gzipWriter)
	for _, entry := range files {
		source := filepath.Join(root, filepath.FromSlash(entry.Source))
		if symlinked, err := hasSymlinkComponent(root, entry.Source); err != nil {
			return closeArchive(tarWriter, gzipWriter, err)
		} else if symlinked {
			return closeArchive(tarWriter, gzipWriter, fmt.Errorf("symlinked bundle source is forbidden: %s", entry.Source))
		}
		resolvedSource, err := filepath.EvalSymlinks(source)
		if err != nil {
			return closeArchive(tarWriter, gzipWriter, err)
		}
		relative, err := filepath.Rel(resolvedRoot, resolvedSource)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return closeArchive(tarWriter, gzipWriter, fmt.Errorf("source escapes repository root: %s", entry.Source))
		}
		info, err := os.Lstat(source)
		if err != nil {
			return closeArchive(tarWriter, gzipWriter, err)
		}
		if !info.Mode().IsRegular() {
			return closeArchive(tarWriter, gzipWriter, fmt.Errorf("bundle source must be a regular file: %s", entry.Source))
		}
		file, err := os.Open(source)
		if err != nil {
			return closeArchive(tarWriter, gzipWriter, err)
		}
		header := &tar.Header{
			Name:       entry.Path,
			Mode:       0o644,
			Size:       info.Size(),
			ModTime:    time.Unix(0, 0).UTC(),
			AccessTime: time.Unix(0, 0).UTC(),
			ChangeTime: time.Unix(0, 0).UTC(),
			Uid:        0,
			Gid:        0,
			Typeflag:   tar.TypeReg,
			Format:     tar.FormatPAX,
		}
		if err := tarWriter.WriteHeader(header); err != nil {
			file.Close()
			return closeArchive(tarWriter, gzipWriter, err)
		}
		if _, err := io.Copy(tarWriter, file); err != nil {
			file.Close()
			return closeArchive(tarWriter, gzipWriter, err)
		}
		if err := file.Close(); err != nil {
			return closeArchive(tarWriter, gzipWriter, err)
		}
	}
	if err := tarWriter.Close(); err != nil {
		gzipWriter.Close()
		return err
	}
	return gzipWriter.Close()
}

func hasSymlinkComponent(root, relative string) (bool, error) {
	current := root
	for _, component := range strings.Split(relative, "/") {
		current = filepath.Join(current, component)
		info, err := os.Lstat(current)
		if err != nil {
			return false, err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return true, nil
		}
	}
	return false, nil
}

func closeArchive(tarWriter *tar.Writer, gzipWriter *gzip.Writer, cause error) error {
	_ = tarWriter.Close()
	_ = gzipWriter.Close()
	return cause
}

func validateAllowlist(allowlist Allowlist) error {
	if allowlist.SchemaVersion != 1 || len(allowlist.Files) == 0 {
		return errors.New("distribution allowlist requires schema_version 1 and at least one file")
	}
	sources := map[string]bool{}
	targets := map[string]bool{}
	for _, entry := range allowlist.Files {
		if !safeRelative(entry.Source) || !safeRelative(entry.Path) {
			return fmt.Errorf("unsafe distribution path %q -> %q", entry.Source, entry.Path)
		}
		if !strings.HasPrefix(entry.Source, "bundles/base/") && !strings.HasPrefix(entry.Source, "bundles/engineering-core/") && !strings.HasPrefix(entry.Source, "bundles/data-practice/") && !strings.HasPrefix(entry.Source, "schemas/") {
			return fmt.Errorf("distribution source is outside approved roots: %s", entry.Source)
		}
		if strings.HasSuffix(entry.Source, ".go") || entry.Source == "bundles/base/.gitkeep" {
			return fmt.Errorf("source-only file is forbidden in the bundle: %s", entry.Source)
		}
		if sources[entry.Source] || targets[entry.Path] {
			return fmt.Errorf("duplicate distribution source or target: %s", entry.Path)
		}
		sources[entry.Source] = true
		targets[entry.Path] = true
	}
	return nil
}

func safeRelative(value string) bool {
	if value == "" || strings.Contains(value, `\`) || filepath.IsAbs(value) {
		return false
	}
	clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(value)))
	return clean == value && clean != "." && clean != ".." && !strings.HasPrefix(clean, "../")
}

func Sign(payload []byte, privateKey ed25519.PrivateKey) []byte {
	return ed25519.Sign(privateKey, payload)
}

func Verify(payload, signature []byte, publicKey ed25519.PublicKey) error {
	if !ed25519.Verify(publicKey, payload, signature) {
		return errors.New("detached signature verification failed")
	}
	return nil
}

func SHA256(payload []byte) string {
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:])
}
