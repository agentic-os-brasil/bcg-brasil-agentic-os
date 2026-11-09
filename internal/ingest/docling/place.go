package docling

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releaseverify"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

// PlaceVerified materializes an already authenticated optional pack under its
// owning admitted home. It never acquires a dependency or modifies an existing
// different pack/metadata. Runtime.Status remains the independent availability
// authority, including the signed client range and qualified input closure.
func PlaceVerified(ctx context.Context, o Options, release releaseverify.VerifiedRelease) (placementErr error) {
	registry, err := releaseverify.LoadPinnedAuthorityRegistry(o.AuthorityRegistryPath, o.AuthorityRegistrySHA256, time.Now)
	if err != nil {
		return err
	}
	verified, err := releaseverify.VerifyDirectory(release.Directory, registry)
	if err != nil {
		return err
	}
	if verified.ManifestSHA256 != release.ManifestSHA256 || !verified.Manifest.Modern() {
		return errors.New("Docling placement requires the unchanged modern signed release")
	}
	compatibility, err := releasecontract.ParseVersionRange(verified.Manifest.CompatibleExecutable())
	if err != nil {
		return err
	}
	accepts, err := compatibility.Contains(o.CLIVersion)
	if err != nil || !accepts {
		return errors.New("converter client is incompatible")
	}
	var selected *releasecontract.Artifact
	for index := range verified.Manifest.Artifacts {
		a := &verified.Manifest.Artifacts[index]
		if a.Kind == "runtime_pack" && a.Adapter == "docling" && a.OS == runtime.GOOS && a.Arch == runtime.GOARCH {
			if selected != nil {
				return errors.New("ambiguous converter identity")
			}
			selected = a
		}
	}
	if selected == nil {
		return errors.New("no authenticated converter for this target")
	}
	body, _, err := readFile(filepath.Join(verified.Directory, selected.Name), selected.Size)
	if err != nil || digest(body) != selected.SHA256 {
		return errors.New("converter archive changed")
	}
	entries, err := archiveEntries(body)
	if err != nil {
		return err
	}
	if err = scheduler.ValidatePrivateDirectory(o.DataRoot); err != nil {
		return err
	}
	base := filepath.Join(o.DataRoot, "ingestion", "docling")
	if err = scheduler.EnsurePrivateDirectory(base); err != nil {
		return err
	}
	installedParent := filepath.Join(base, "installed")
	if err = scheduler.EnsurePrivateDirectory(installedParent); err != nil {
		return err
	}
	installed := filepath.Join(installedParent, selected.SHA256)
	if _, err = os.Lstat(installed); errors.Is(err, os.ErrNotExist) {
		stage, err := os.MkdirTemp(installedParent, ".placing-")
		if err != nil {
			return err
		}
		published := false
		defer func() {
			if placementErr != nil && !published {
				placementErr = fmt.Errorf("converter staging preserved for inspection at %s; no failure cleanup was attempted: %w", stage, placementErr)
			}
		}()
		handle, err := os.OpenRoot(stage)
		if err != nil {
			return err
		}
		defer handle.Close()
		paths := make([]string, 0, len(entries))
		for path := range entries {
			paths = append(paths, path)
		}
		sort.Strings(paths)
		for _, path := range paths {
			if err = ctx.Err(); err != nil {
				return err
			}
			entry := entries[path]
			if err = handle.MkdirAll(filepath.Dir(path), 0700); err != nil {
				return err
			}
			file, err := handle.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, os.FileMode(entry.mode))
			if err != nil {
				return err
			}
			_, writeErr := file.Write(entry.body)
			closeErr := file.Close()
			if writeErr != nil {
				return writeErr
			}
			if closeErr != nil {
				return closeErr
			}
		}
		var directories []string
		if err = fs.WalkDir(handle.FS(), ".", func(path string, d fs.DirEntry, e error) error {
			if e != nil {
				return e
			}
			if d.IsDir() {
				directories = append(directories, path)
			}
			return nil
		}); err != nil {
			return err
		}
		sort.Sort(sort.Reverse(sort.StringSlice(directories)))
		for _, directory := range directories {
			if err = handle.Chmod(directory, 0555); err != nil {
				return err
			}
		}
		if err = os.Rename(stage, installed); err != nil {
			return err
		}
		published = true
	} else if err != nil {
		return err
	}
	for name, content := range map[string][]byte{selected.Name: body} {
		if err = placeExact(base, name, content); err != nil {
			return err
		}
	}
	for _, name := range []string{releaseverify.ManifestName, releaseverify.ManifestSignatureName, selected.SignatureRef} {
		content, _, err := readFile(filepath.Join(verified.Directory, name), 1<<20)
		if err != nil {
			return err
		}
		if err = placeExact(base, name, content); err != nil {
			return err
		}
	}
	availability := New(o).Status(ctx)
	if availability.State != "ready" {
		return fmt.Errorf("placed converter unavailable: %s", availability.Reason)
	}
	return nil
}

func placeExact(root, name string, body []byte) error {
	path := filepath.Join(root, name)
	existing, _, err := readFile(path, int64(len(body)))
	if err == nil {
		if !bytes.Equal(existing, body) {
			return errors.New("existing converter metadata differs; preserved")
		}
		return nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return scheduler.WriteNewPrivateFile(path, body)
}
