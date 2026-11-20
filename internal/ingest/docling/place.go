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
	required, err := New(o).currentRequirement(ctx)
	if err != nil {
		return err
	}
	if o.ResolveRequirement != nil {
		if required.ManifestSHA256 != verified.ManifestSHA256 || required.ServerVersion != verified.Manifest.Server.Version {
			return errors.New("reviewed converter release is no longer current")
		}
		o.CLIVersion = required.ServerVersion
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
	if selected == nil || o.ResolveRequirement != nil && selected.SHA256 != required.PackSHA256 {
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
	// Reject unsafe/unknown selectors before any new payload or metadata effect.
	if _, err = metadataBase(o); err != nil {
		return err
	}
	metadata := base
	if o.ResolveRequirement != nil {
		metadata = filepath.Join(base, "metadata", verified.ManifestSHA256)
		if err = scheduler.EnsurePrivateDirectory(metadata); err != nil {
			return err
		}
	}
	installedParent := filepath.Join(base, "installed")
	if err = scheduler.EnsurePrivateDirectory(installedParent); err != nil {
		return err
	}
	parentHandle, err := scheduler.OpenDirectory(installedParent)
	if err != nil {
		return err
	}
	defer parentHandle.Close()
	heldParent, err := scheduler.HoldDirectory(installedParent, parentHandle)
	if err != nil {
		return err
	}
	defer heldParent.Close()
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
			var modeErr error
			if writeErr == nil {
				// Creation honors the caller's private umask; the authenticated
				// archive requires these exact sealed modes on this new handle.
				modeErr = file.Chmod(os.FileMode(entry.mode))
			}
			closeErr := file.Close()
			if writeErr != nil {
				return writeErr
			}
			if modeErr != nil {
				return modeErr
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
			if directory == "." {
				// Darwin requires the source directory to remain writable for
				// rename. Keep only this private root unsealed until publication.
				continue
			}
			if err = handle.Chmod(directory, 0555); err != nil {
				return err
			}
		}
		if err = heldParent.Rename(filepath.Base(stage), selected.SHA256, false); err != nil {
			return err
		}
		published = true
		// Seal through the same retained root, before metadata, qualification
		// or selection can make this newly published payload available.
		if err = handle.Chmod(".", 0555); err != nil {
			return fmt.Errorf("published converter root could not be sealed; current selector preserved: %w", err)
		}
	} else if err != nil {
		return err
	}
	for name, content := range map[string][]byte{selected.Name: body} {
		if err = placeExact(metadata, name, content); err != nil {
			return err
		}
	}
	for _, name := range []string{releaseverify.ManifestName, releaseverify.ManifestSignatureName, selected.SignatureRef} {
		content, _, err := readFile(filepath.Join(verified.Directory, name), 1<<20)
		if err != nil {
			return err
		}
		if err = placeExact(metadata, name, content); err != nil {
			return err
		}
	}
	// Verify and qualify the candidate without changing the selected generation.
	if o.ResolveRequirement != nil {
		o.metadataGeneration = verified.ManifestSHA256
	}
	if err = qualifyPlaced(ctx, o); err != nil {
		return err
	}
	availability := New(o).Status(ctx)
	if availability.State != "ready" {
		return fmt.Errorf("placed converter unavailable: %s", availability.Reason)
	}
	// Selection is the only mutable runtime record. Its final publication is
	// serialized with current installation custody by the owning caller.
	if o.ResolveRequirement == nil {
		return nil
	}
	return selectMetadata(ctx, o, required)
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
