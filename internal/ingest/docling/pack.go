package docling

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"io"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releaseverify"
)

type verifiedPack struct {
	entries          map[string]entry
	sha, manifestSHA string
}

func (r *Runtime) verify(ctx context.Context) (verifiedPack, string) {
	var empty verifiedPack
	o := r.options
	if len(o.AuthorityRegistrySHA256) != 64 || o.AuthorityRegistryPath == "" {
		return empty, "authority_unavailable"
	}
	registryBytes, _, err := readFile(o.AuthorityRegistryPath, 1<<20)
	if err != nil || digest(registryBytes) != o.AuthorityRegistrySHA256 {
		return empty, "authority_unavailable"
	}
	registry, err := releaseverify.ParseAuthorityRegistry(bytes.NewReader(registryBytes), time.Now)
	if err != nil || !registry.HasActiveKey() {
		return empty, "authority_unavailable"
	}
	if runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" {
		return empty, "platform_unavailable"
	}
	if !filepath.IsAbs(o.DataRoot) || filepath.Clean(o.DataRoot) != o.DataRoot {
		return empty, "pack_unavailable"
	}
	base := filepath.Join(o.DataRoot, "ingestion", "docling")
	manifestBody, _, err := readFile(filepath.Join(base, releaseverify.ManifestName), 1<<20)
	if err != nil {
		return empty, "pack_unavailable"
	}
	signature, _, err := readFile(filepath.Join(base, releaseverify.ManifestSignatureName), ed25519.SignatureSize)
	if err != nil {
		return empty, "pack_unavailable"
	}
	manifest, key, err := releaseverify.VerifyManifest(manifestBody, signature, registry)
	if err != nil {
		return empty, "pack_untrusted"
	}
	compatibility, err := releasecontract.ParseVersionRange(manifest.Bundle.CompatibleCLI)
	if err != nil {
		return empty, "pack_incompatible"
	}
	ok, err := compatibility.Contains(o.CLIVersion)
	if err != nil || !ok {
		return empty, "pack_incompatible"
	}
	var selected *releasecontract.Artifact
	for i := range manifest.Artifacts {
		a := &manifest.Artifacts[i]
		if a.Kind == "runtime_pack" && a.OS == runtime.GOOS && a.Arch == runtime.GOARCH {
			if selected != nil {
				return empty, "pack_untrusted"
			}
			selected = a
		}
	}
	if selected == nil || selected.Size > maximumArchiveBytes {
		return empty, "pack_unavailable"
	}
	body, _, err := readFile(filepath.Join(base, selected.Name), selected.Size)
	if err != nil || int64(len(body)) != selected.Size || digest(body) != selected.SHA256 {
		return empty, "pack_untrusted"
	}
	signature, _, err = readFile(filepath.Join(base, selected.SignatureRef), ed25519.SignatureSize)
	if err != nil || !ed25519.Verify(key, body, signature) {
		return empty, "pack_untrusted"
	}
	if ctx.Err() != nil {
		return empty, "conversion_cancelled"
	}
	entries, err := archiveEntries(body)
	if err != nil {
		return empty, "pack_untrusted"
	}
	metadata, ok := entries["pack.json"]
	if !ok || len(metadata.body) > 8<<20 {
		return empty, "pack_untrusted"
	}
	var pack packManifest
	if decode(metadata.body, &pack) != nil {
		return empty, "pack_untrusted"
	}
	canonical, _ := json.Marshal(pack)
	var expected []Input
	_ = decode(lockedInputs, &expected)
	if !bytes.Equal(canonical, metadata.body) || pack.SchemaVersion != 1 || pack.Adapter != "docling" || pack.Version != DoclingVersion || pack.OS != runtime.GOOS || pack.Arch != runtime.GOARCH || pack.Python != PythonVersion || pack.PythonArchiveSHA256 != PythonArchiveSHA256 || pack.LockSHA256 != LockSHA256 || pack.Models == nil || len(pack.Models) != 0 || !reflect.DeepEqual(pack.Inputs, expected) {
		return empty, "pack_untrusted"
	}
	compatibility, err = releasecontract.ParseVersionRange(pack.CompatibleCLI)
	if err != nil {
		return empty, "pack_incompatible"
	}
	ok, err = compatibility.Contains(o.CLIVersion)
	if err != nil || !ok {
		return empty, "pack_incompatible"
	}
	if len(pack.Files) != len(entries)-1 {
		return empty, "pack_untrusted"
	}
	seen := map[string]bool{}
	for _, file := range pack.Files {
		e, exists := entries[file.Path]
		if !exists || seen[file.Path] || file.Path == "pack.json" || file.Size != int64(len(e.body)) || file.Mode != e.mode || file.SHA256 != digest(e.body) {
			return empty, "pack_untrusted"
		}
		seen[file.Path] = true
	}
	if !bytes.Equal(entries["bridge.py"].body, bridge) || !bytes.Equal(entries["pylock.toml"].body, rawLock) || !bytes.Equal(entries["inputs.json"].body, lockedInputs) {
		return empty, "pack_untrusted"
	}
	installed := filepath.Join(base, "installed", selected.SHA256)
	if err := verifyInstalled(ctx, installed, entries); err != nil {
		return empty, "installed_pack_changed"
	}
	return verifiedPack{entries: entries, sha: selected.SHA256, manifestSHA: digest(manifestBody)}, ""
}
func archiveEntries(body []byte) (map[string]entry, error) {
	compressed := bytes.NewReader(body)
	gz, err := gzip.NewReader(compressed)
	if err != nil {
		return nil, err
	}
	defer gz.Close()
	gz.Multistream(false)
	limited := &io.LimitedReader{R: gz, N: maximumExpandedBytes + 32<<20}
	tr := tar.NewReader(limited)
	entries := map[string]entry{}
	aliases := map[string]bool{}
	var total int64
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		if !relativeName(h.Name) || (h.Typeflag != tar.TypeReg && h.Typeflag != tar.TypeRegA) || h.Size < 0 || h.Size > maximumExpandedBytes || (h.Mode != 0444 && h.Mode != 0555) {
			return nil, errors.New("archive_member_invalid")
		}
		alias := strings.ToLower(h.Name)
		if aliases[alias] {
			return nil, errors.New("archive_member_alias")
		}
		aliases[alias] = true
		total += h.Size
		if total > maximumExpandedBytes || len(entries) >= maximumFiles {
			return nil, errors.New("archive_expansion_limit")
		}
		data, err := io.ReadAll(io.LimitReader(tr, h.Size+1))
		if err != nil || int64(len(data)) != h.Size {
			return nil, errors.New("archive_member_size")
		}
		entries[h.Name] = entry{data, h.Mode}
	}
	tail, err := io.ReadAll(io.LimitReader(gz, 4097))
	if err != nil || len(tail) > 4096 || len(bytes.Trim(tail, "\x00")) != 0 || compressed.Len() != 0 || limited.N <= 0 {
		return nil, errors.New("archive_trailing_data")
	}
	return entries, nil
}
