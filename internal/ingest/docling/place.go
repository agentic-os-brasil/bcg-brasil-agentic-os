package docling

import (
	"bytes"
	"context"
	"crypto/rand"
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
func PlaceVerified(ctx context.Context, o Options, release releaseverify.VerifiedRelease) error {
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
		if err = placeEntries(ctx, installedParent, selected.SHA256, entries); err != nil {
			return err
		}
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

func placeEntries(ctx context.Context, installedParent, name string, entries map[string]entry) (result error) {
	parent, err := openPlacementDirectory(installedParent)
	if err != nil {
		return err
	}
	defer parent.Close()
	confined, err := os.OpenRoot(installedParent)
	if err != nil {
		return err
	}
	defer confined.Close()
	opened, err := confined.Open(".")
	if err != nil {
		return err
	}
	parentInfo, parentErr := parent.Stat()
	confinedInfo, confinedErr := opened.Stat()
	_ = opened.Close()
	if parentErr != nil || confinedErr != nil || !os.SameFile(parentInfo, confinedInfo) {
		return errors.New("converter placement parent changed")
	}
	stage := ".placing-" + rand.Text()
	if err = confined.Mkdir(stage, 0700); err != nil {
		return err
	}
	handle, err := confined.OpenRoot(stage)
	if err != nil {
		return err
	}
	defer handle.Close()
	root, err := openPlacementLeaf(parent, stage)
	if err != nil {
		return err
	}
	defer root.Close()
	opened, err = handle.Open(".")
	if err != nil {
		return err
	}
	rootInfo, rootErr := root.Stat()
	extractionInfo, extractionErr := opened.Stat()
	_ = opened.Close()
	if rootErr != nil || extractionErr != nil || !os.SameFile(rootInfo, extractionInfo) {
		return errors.New("converter placement staging root changed")
	}
	defer func() {
		if err := placementLeafMatches(parent, root, stage); errors.Is(err, os.ErrNotExist) {
			return
		} else if err != nil {
			result = errors.Join(result, err)
			return
		}
		err := fs.WalkDir(handle.FS(), ".", func(path string, d fs.DirEntry, e error) error {
			if e == nil && d.IsDir() {
				return handle.Chmod(path, 0700)
			}
			return e
		})
		if err == nil {
			err = confined.RemoveAll(stage)
		}
		result = errors.Join(result, err)
	}()
	if err := placementLeafMatches(parent, root, stage); err != nil {
		return err
	}
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
		if writeErr == nil {
			writeErr = file.Sync()
		}
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
		if d.IsDir() && path != "." {
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
	return publishPlacement(parent, root, stage, name)
}

// Test-only interruption seam after exclusive publication and before sealing.
var placementAfterPublish func() error

func publishPlacement(parent, root *os.File, stage, installed string) (result error) {
	if err := placementLeafMatches(parent, root, stage); err != nil {
		return err
	}
	err := scheduler.RenamePrivateFileExclusive(parent, stage, installed)
	if err != nil && placementLeafMatches(parent, root, installed) != nil {
		return err
	}
	defer func() {
		if result == nil {
			return
		}
		// Restore only this invocation's held tree to its absent staging leaf.
		rollback := placementLeafMatches(parent, root, installed)
		if rollback == nil {
			rollback = root.Chmod(0700)
		}
		if rollback == nil {
			rollback = scheduler.RenamePrivateFileExclusive(parent, installed, stage)
		}
		if rollback == nil {
			rollback = syncPlacementDirectory(parent)
		}
		if rollback != nil {
			result = errors.Join(result, fmt.Errorf("converter placement recovery required: %w", rollback))
		}
	}()
	if err != nil {
		return err
	}
	if placementAfterPublish != nil {
		if err := placementAfterPublish(); err != nil {
			return err
		}
	}
	// Seal the held root only after exclusive publication completes.
	if err := root.Chmod(0555); err != nil {
		return err
	}
	if err := syncPlacementDirectory(root); err != nil {
		return err
	}
	if err := placementLeafMatches(parent, root, installed); err != nil {
		return err
	}
	return syncPlacementDirectory(parent)
}

func placementLeafMatches(parent, root *os.File, name string) error {
	leaf, err := openPlacementLeaf(parent, name)
	if err != nil {
		return err
	}
	defer leaf.Close()
	heldInfo, heldErr := root.Stat()
	leafInfo, leafErr := leaf.Stat()
	if heldErr != nil || leafErr != nil || !os.SameFile(heldInfo, leafInfo) {
		return errors.New("converter placement directory changed")
	}
	return nil
}
