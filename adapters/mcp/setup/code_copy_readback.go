package setup

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installer"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

// Current Code command sources omit folder-only fields and suffix their cache
// version with the first 12 characters of the copied contents' SHA256. Absence
// of readFromFolder alone is never copy evidence: prove the fixed COPY provider,
// selected profile cache, manifest, content-derived version and canonical bytes.
func observeCodeCommandCopy(r installer.ClientRecord, p pluginRow) (string, string, bool, error) {
	if _, err := reviewCodeCommandConsent(r); err != nil {
		return "", "", false, err
	}
	profile := r.Code.ConfigurationDirectory
	if !filepath.IsAbs(profile) || filepath.Clean(profile) != profile || p.Version == "" || p.Version == "." || p.Version == ".." || strings.ContainsAny(p.Version, `/\`) {
		return "", "", false, errors.New("explicit selected Code cache binding required")
	}
	expectedCache := filepath.Join(profile, "plugins", "cache", r.Code.Marketplace, "maestro-product", p.Version)
	if p.InstallPath != expectedCache {
		return "", "", false, errors.New("command copy is outside the exact selected Code cache")
	}
	manifestPath := filepath.Join(p.InstallPath, ".claude-plugin", "plugin.json")
	body, err := readCodeCopyFile(manifestPath, 64<<10)
	if err != nil {
		return "", "", false, err
	}
	var manifest struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	}
	if err := json.Unmarshal(body, &manifest); err != nil || manifest.Name != "maestro-product" || manifest.Version == "" {
		return "", "", false, errors.New("native Code copy manifest identity unavailable")
	}
	copy, content, err := codeCopyInventory(p.InstallPath, true)
	if err != nil {
		return "", "", false, err
	}
	if p.Version != manifest.Version+"-"+content[:12] {
		return "", "", false, errors.New("native Code cache version does not match its manifest and actual content digest")
	}
	current := manifest.Version == r.Materialization.Release
	if current {
		whole, err := installer.ClientTreeDigest(r.Materialization.CodeRoot)
		if err != nil || whole != r.Materialization.CodeTreeSHA256 {
			return "", "", false, errors.New("recorded current Code source changed")
		}
		source, sourceContent, err := codeCopyInventory(filepath.Join(r.Materialization.CodeRoot, "plugins", "maestro-product"), false)
		if err != nil || sourceContent != content || !reflect.DeepEqual(source, copy) {
			return "", "", false, errors.New("native Code copy differs from current canonical packet; preserved")
		}
		whole, err = installer.ClientTreeDigest(r.Materialization.CodeRoot)
		if err != nil || whole != r.Materialization.CodeTreeSHA256 {
			return "", "", false, errors.New("current Code source changed during copy observation")
		}
	}
	after, afterContent, err := codeCopyInventory(p.InstallPath, true)
	if err != nil || afterContent != content || !reflect.DeepEqual(after, copy) {
		return "", "", false, errors.New("native Code copy changed during observation")
	}
	return manifest.Version, content, current, nil
}

// This bounded inventory admits no aliases and compares directory topology as
// well as every plugin byte. The only host-owned exception is the root .in_use
// directory with the exact public PID-marker schema. No subtree is ignored.
func codeCopyInventory(root string, cache bool) ([]string, string, error) {
	if err := scheduler.ValidateDirectory(root); err != nil {
		return nil, "", err
	}
	var files, inventory []string
	var total int64
	entries := 0
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		entries++
		if entries > 4096 {
			return errors.New("Code copy inventory bound")
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if !utf8.ValidString(rel) {
			return errors.New("Code copy path encoding unsupported")
		}
		if d.Type()&os.ModeSymlink != 0 {
			return errors.New("Code copy alias refused")
		}
		if rel == ".in_use" {
			if !cache {
				return errors.New("canonical Code packet contains a reserved host lease role")
			}
			if err := validateCodeCopyLeases(path); err != nil {
				return err
			}
			return filepath.SkipDir
		}
		if rel == ".git" {
			return errors.New("canonical Code copy contains unsupported repository metadata")
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if info.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 {
			return errors.New("Code copy special mode refused")
		}
		if d.IsDir() {
			if err := scheduler.ValidateDirectory(path); err != nil {
				return err
			}
			inventory = append(inventory, "directory:"+rel)
			return nil
		}
		if !info.Mode().IsRegular() {
			return errors.New("Code copy entry is not a regular file")
		}
		total += info.Size()
		if info.Size() < 0 || info.Size() > 128<<20 || total > 256<<20 {
			return errors.New("Code copy content bound")
		}
		files = append(files, rel)
		return nil
	})
	if err != nil {
		return nil, "", err
	}
	// JavaScript's default string ordering is UTF-16 code-unit ordering, including
	// supplementary characters. No platform-native separator enters the hash.
	sort.Slice(files, func(i, j int) bool { return codeUTF16Less(filepath.FromSlash(files[i]), filepath.FromSlash(files[j])) })
	host := sha256.New()
	total = 0
	for _, rel := range files {
		body, err := readCodeCopyFile(filepath.Join(root, filepath.FromSlash(rel)), 128<<20)
		if err != nil {
			return nil, "", err
		}
		total += int64(len(body))
		if total > 256<<20 {
			return nil, "", errors.New("Code copy observed byte bound")
		}
		fmt.Fprintf(host, "f %s\x00%d\x00", rel, len(body))
		host.Write(body)
		inventory = append(inventory, fmt.Sprintf("file:%s:%x", rel, sha256.Sum256(body)))
	}
	sort.Strings(inventory)
	return inventory, fmt.Sprintf("%x", host.Sum(nil)), nil
}

func codeUTF16Less(a, b string) bool {
	x, y := utf16.Encode([]rune(a)), utf16.Encode([]rune(b))
	for i := 0; i < len(x) && i < len(y); i++ {
		if x[i] != y[i] {
			return x[i] < y[i]
		}
	}
	return len(x) < len(y)
}

func readCodeCopyFile(path string, limit int64) ([]byte, error) {
	f, err := scheduler.OpenRegularFile(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if err := scheduler.ValidateSingleLink(f); err != nil {
		return nil, err
	}
	before, err := f.Stat()
	if err != nil || !before.Mode().IsRegular() || before.Size() < 0 || before.Size() > limit {
		return nil, errors.New("Code copy regular file bound")
	}
	body, err := io.ReadAll(io.LimitReader(f, limit+1))
	after, statErr := f.Stat()
	if err != nil || statErr != nil {
		return nil, errors.Join(err, statErr)
	}
	if int64(len(body)) != before.Size() || !os.SameFile(before, after) || before.Size() != after.Size() || before.Mode() != after.Mode() || !before.ModTime().Equal(after.ModTime()) {
		return nil, errors.New("Code copy file changed while reading")
	}
	return body, nil
}

func validateCodeCopyLeases(path string) error {
	if err := scheduler.ValidateDirectory(path); err != nil {
		return err
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return err
	}
	if len(entries) > 128 {
		return errors.New("Code lease marker bound")
	}
	for _, entry := range entries {
		pid, err := strconv.ParseInt(entry.Name(), 10, 32)
		if err != nil || pid <= 0 || strconv.FormatInt(pid, 10) != entry.Name() || !entry.Type().IsRegular() {
			return errors.New("Code lease marker role refused")
		}
		body, err := readCodeCopyFile(filepath.Join(path, entry.Name()), 4096)
		if err != nil {
			return err
		}
		var marker map[string]json.RawMessage
		if err := json.Unmarshal(body, &marker); err != nil {
			return errors.New("Code lease marker schema unavailable")
		}
		var observedPID int64
		if err := json.Unmarshal(marker["pid"], &observedPID); err != nil || observedPID != pid {
			return errors.New("Code lease marker PID mismatch")
		}
		for key, value := range marker {
			if key == "pid" {
				continue
			}
			if key != "procStart" && key != "procStartFt" {
				return errors.New("unknown Code lease marker field")
			}
			var text string
			if err := json.Unmarshal(value, &text); err != nil || bytes.Equal(bytes.TrimSpace(value), []byte("null")) || len(text) > 256 || bytes.IndexByte([]byte(text), 0) >= 0 {
				return errors.New("Code lease marker field refused")
			}
		}
	}
	return nil
}
