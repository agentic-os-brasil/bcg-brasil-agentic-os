package docling

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

const (
	PythonVersion        = "3.12.16+20261026"
	DoclingVersion       = "2.145.0"
	LockSHA256           = "f69da0e247c73dc20870d51030344e1cbba9f5520b722c79a336b9d8b044639b"
	PythonArchive        = "cpython-3.12.16+20261026-aarch64-apple-darwin-install_only.tar.gz"
	PythonArchiveSHA256  = "fbf4afef53c2a6b72bb741bc06d5671a9dfaf87bd84e5aa73ac9b3d0d589f14e"
	maximumArchiveBytes  = 256 << 20
	maximumExpandedBytes = 512 << 20
	maximumFiles         = 20000
)

//go:embed runtime/inputs.json
var lockedInputs []byte

//go:embed runtime/pylock.toml
var rawLock []byte

//go:embed runtime/bridge.py
var bridge []byte

//go:embed runtime/parser-licenses.json
var parserLicenses []byte

type Input struct {
	Name     string `json:"name"`
	Version  string `json:"version"`
	URL      string `json:"url"`
	Filename string `json:"filename"`
	SHA256   string `json:"sha256"`
	Size     int64  `json:"size"`
}
type inventoryFile struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
	Mode   int64  `json:"mode"`
}
type packManifest struct {
	SchemaVersion       int             `json:"schema_version"`
	Adapter             string          `json:"adapter"`
	Version             string          `json:"version"`
	OS                  string          `json:"os"`
	Arch                string          `json:"arch"`
	CompatibleCLI       string          `json:"compatible_cli"`
	Python              string          `json:"python"`
	PythonArchiveSHA256 string          `json:"python_archive_sha256"`
	LockSHA256          string          `json:"lock_sha256"`
	Models              []string        `json:"models"`
	Inputs              []Input         `json:"inputs"`
	Files               []inventoryFile `json:"files"`
}
type entry struct {
	body []byte
	mode int64
}
type Assembly struct {
	ArchivePath   string `json:"archive_path"`
	ArchiveSHA256 string `json:"archive_sha256"`
	Files         int    `json:"files"`
	ExpandedBytes int64  `json:"expanded_bytes"`
}

// Assemble authenticates already acquired official archives before extracting
// them. Destination must be new; the result is unsigned until the existing
// release authority signs its archive and release manifest.
func Assemble(inputDirectory, destination string) (Assembly, error) {
	var result Assembly
	if err := scheduler.ValidateDirectory(inputDirectory); err != nil {
		return result, err
	}
	inputs := []Input{}
	if err := decode(lockedInputs, &inputs); err != nil {
		return result, err
	}
	if len(inputs) != 51 || digest(rawLock) != LockSHA256 {
		return result, errors.New("embedded_lock_invalid")
	}
	python, _, err := readFile(filepath.Join(inputDirectory, PythonArchive), maximumArchiveBytes)
	if err != nil {
		return result, err
	}
	if digest(python) != PythonArchiveSHA256 {
		return result, errors.New("python_archive_digest")
	}
	// Authenticate all raw wheels before extracting any of them.
	wheels := make([][]byte, len(inputs))
	for i, input := range inputs {
		wheels[i], _, err = readFile(filepath.Join(inputDirectory, input.Filename), input.Size)
		if err != nil {
			return result, fmt.Errorf("locked wheel %s unavailable: %w", input.Name, err)
		}
		if int64(len(wheels[i])) != input.Size || digest(wheels[i]) != input.SHA256 {
			return result, fmt.Errorf("locked wheel %s digest mismatch", input.Name)
		}
	}
	entries, err := pythonEntries(python)
	if err != nil {
		return result, err
	}
	for i, body := range wheels {
		archive, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
		if err != nil {
			return result, err
		}
		for _, file := range archive.File {
			if file.FileInfo().IsDir() {
				continue
			}
			if !relativeName(file.Name) || !file.Mode().IsRegular() || file.UncompressedSize64 > maximumExpandedBytes {
				return result, errors.New("unsafe_wheel_member")
			}
			name := "site-packages/" + file.Name
			relocatedScript := false
			if strings.Contains(file.Name, ".data/") {
				// Retain the exact authenticated xlrd CLI as inert inventory. It
				// is outside PATH and never used by the closed BIFF route.
				if inputs[i].Name != "xlrd" || inputs[i].Version != "2.0.2" || file.Name != "xlrd-2.0.2.data/scripts/runxlrd.py" {
					return result, fmt.Errorf("wheel relocation unsupported: %s", inputs[i].Name)
				}
				name, relocatedScript = "wheel-scripts/xlrd/runxlrd.py", true
			}
			if _, exists := entries[name]; exists {
				return result, errors.New("wheel_collision")
			}
			reader, err := file.Open()
			if err != nil {
				return result, err
			}
			data, err := io.ReadAll(io.LimitReader(reader, maximumExpandedBytes+1))
			closeErr := reader.Close()
			if err != nil {
				return result, err
			}
			if closeErr != nil {
				return result, closeErr
			}
			if uint64(len(data)) != file.UncompressedSize64 {
				return result, errors.New("wheel_expansion")
			}
			mode := int64(0444)
			if file.Mode().Perm()&0111 != 0 && !relocatedScript {
				mode = 0555
			}
			entries[name] = entry{data, mode}
			if err := boundEntries(entries); err != nil {
				return result, err
			}
		}
	}
	entries["bridge.py"] = entry{bridge, 0444}
	entries["pylock.toml"] = entry{rawLock, 0444}
	entries["inputs.json"] = entry{lockedInputs, 0444}
	entries["parser-licenses.json"] = entry{parserLicenses, 0444}
	manifest := packManifest{SchemaVersion: 1, Adapter: "docling", Version: DoclingVersion, OS: "darwin", Arch: "arm64", CompatibleCLI: ">=0.1.0 <0.2.0", Python: PythonVersion, PythonArchiveSHA256: PythonArchiveSHA256, LockSHA256: LockSHA256, Models: []string{}, Inputs: inputs, Files: []inventoryFile{}}
	names := entryNames(entries)
	for _, name := range names {
		e := entries[name]
		manifest.Files = append(manifest.Files, inventoryFile{name, digest(e.body), int64(len(e.body)), e.mode})
		result.ExpandedBytes += int64(len(e.body))
	}
	body, err := json.Marshal(manifest)
	if err != nil {
		return result, err
	}
	entries["pack.json"] = entry{body, 0444}
	if err := boundEntries(entries); err != nil {
		return result, err
	}
	var compressed bytes.Buffer
	gz := gzip.NewWriter(&compressed)
	tw := tar.NewWriter(gz)
	for _, name := range entryNames(entries) {
		e := entries[name]
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: e.mode, Size: int64(len(e.body)), Typeflag: tar.TypeReg}); err != nil {
			return result, err
		}
		if _, err := tw.Write(e.body); err != nil {
			return result, err
		}
	}
	if err := tw.Close(); err != nil {
		return result, err
	}
	if err := gz.Close(); err != nil {
		return result, err
	}
	if compressed.Len() > maximumArchiveBytes {
		return result, errors.New("archive_too_large")
	}
	if err := scheduler.ValidateDirectory(filepath.Dir(destination)); err != nil {
		return result, err
	}
	if err := os.Mkdir(destination, 0700); err != nil {
		return result, err
	}
	result.ArchiveSHA256 = digest(compressed.Bytes())
	result.ArchivePath = filepath.Join(destination, "docling-runtime-"+DoclingVersion+"-darwin-arm64.tar.gz")
	if err := writeExclusive(result.ArchivePath, compressed.Bytes(), 0444); err != nil {
		return result, err
	}
	installed := filepath.Join(destination, "installed", result.ArchiveSHA256)
	if err := materialize(entries, installed); err != nil {
		return result, err
	}
	if err := filepath.WalkDir(installed, func(name string, item os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if item.IsDir() {
			return os.Chmod(name, 0555)
		}
		return nil
	}); err != nil {
		return result, err
	}
	result.Files = len(entries)
	result.ExpandedBytes += int64(len(body))
	return result, nil
}
func pythonEntries(body []byte) (map[string]entry, error) {
	gz, err := gzip.NewReader(bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	entries := map[string]entry{}
	links := map[string]string{}
	seen := map[string]bool{}
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		name := strings.TrimSuffix(h.Name, "/")
		if !relativeName(name) || (!strings.HasPrefix(name, "python/") && name != "python") || seen[name] {
			return nil, errors.New("unsafe_python_member")
		}
		seen[name] = true
		if len(seen) > maximumFiles {
			return nil, errors.New("python_member_limit")
		}
		switch h.Typeflag {
		case tar.TypeDir:
			continue
		case tar.TypeSymlink:
			target := path.Join(path.Dir(name), h.Linkname)
			if path.IsAbs(h.Linkname) || !strings.HasPrefix(target, "python/") {
				return nil, errors.New("python_link_escape")
			}
			links[name] = target
		case tar.TypeReg, tar.TypeRegA:
			if h.Size < 0 || h.Size > maximumExpandedBytes {
				return nil, errors.New("python_member_limit")
			}
			data, err := io.ReadAll(io.LimitReader(tr, h.Size+1))
			if err != nil || int64(len(data)) != h.Size {
				return nil, errors.New("python_member_size")
			}
			mode := int64(0444)
			if h.Mode&0111 != 0 {
				mode = 0555
			}
			entries[name] = entry{data, mode}
		default:
			return nil, errors.New("python_special_member")
		}
		if err := boundEntries(entries); err != nil {
			return nil, err
		}
	}
	for name, target := range links {
		e, ok := entries[target]
		if !ok {
			return nil, errors.New("python_link_target")
		}
		entries[name] = e
	}
	return entries, boundEntries(entries)
}
func boundEntries(entries map[string]entry) error {
	var total int64
	if len(entries) > maximumFiles {
		return errors.New("pack_file_count")
	}
	for _, e := range entries {
		total += int64(len(e.body))
		if total > maximumExpandedBytes {
			return errors.New("pack_expanded_size")
		}
	}
	return nil
}
func entryNames(entries map[string]entry) []string {
	names := make([]string, 0, len(entries))
	for name := range entries {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
func materialize(entries map[string]entry, root string) error {
	if err := os.MkdirAll(filepath.Dir(root), 0700); err != nil {
		return err
	}
	if err := os.Mkdir(root, 0700); err != nil {
		return err
	}
	for _, name := range entryNames(entries) {
		e := entries[name]
		if err := writeExclusive(filepath.Join(root, filepath.FromSlash(name)), e.body, os.FileMode(e.mode)); err != nil {
			return err
		}
	}
	return nil
}
