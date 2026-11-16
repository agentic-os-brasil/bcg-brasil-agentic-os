package acquisition

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ingest/docling"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releaseverify"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

// Acquire is an explicit, separately confirmed preparation effect. It downloads
// public inputs and assembles bytes; it never executes the acquired runtime or
// installs it. The caller must separately review the complete returned release.
func Acquire(ctx context.Context, o Options, reviewed Plan) (releaseverify.VerifiedRelease, error) {
	var result releaseverify.VerifiedRelease
	reviewedAt, err := time.Parse(time.RFC3339, reviewed.ReviewedAt)
	if err != nil {
		return result, errors.New("acquisition_review_invalid")
	}
	current, err := reviewAt(ctx, o, reviewedAt, time.Now().UTC())
	if err != nil {
		return result, err
	}
	if current.SHA256 != reviewed.SHA256 {
		return result, errors.New("acquisition_review_changed")
	}
	if runtime.GOOS != "windows" || runtime.GOARCH != "amd64" {
		return result, errors.New("acquisition_native_windows_required")
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Minute)
	defer cancel()
	if err = privateDirectory(o.CacheRoot, true); err != nil {
		return result, errors.New("acquisition_cache_unsafe")
	}
	held, err := holdPrivateDirectory(o.CacheRoot)
	if err != nil {
		return result, errors.New("acquisition_cache_unsafe")
	}
	defer held.Close()
	err = scheduler.WithPrivateLockContext(ctx, filepath.Join(o.CacheRoot, ".acquisition.lock"), func() error {
		current, err := reviewAt(ctx, o, reviewedAt, time.Now().UTC())
		if err != nil {
			return err
		}
		if current.SHA256 != reviewed.SHA256 {
			return errors.New("acquisition_review_changed")
		}
		result, err = prepare(ctx, o, current)
		return err
	})
	return result, err
}

func prepare(ctx context.Context, o Options, p Plan) (releaseverify.VerifiedRelease, error) {
	var empty releaseverify.VerifiedRelease
	a, err := authenticate(ctx, o, time.Now().UTC())
	if err != nil {
		return empty, err
	}
	if err = privateDirectory(filepath.Join(o.CacheRoot, "blobs"), true); err != nil {
		return empty, errors.New("acquisition_cache_unsafe")
	}
	attempt := filepath.Join(o.CacheRoot, fmt.Sprintf("attempt-%d", p.Attempt))
	if err = mkdirPrivateNew(attempt); err != nil {
		return empty, errors.New("acquisition_attempt_conflict_preserved")
	}
	inputDirectory := filepath.Join(attempt, "inputs")
	if err = mkdirPrivateNew(inputDirectory); err != nil {
		return empty, errors.New("acquisition_attempt_conflict_preserved")
	}
	for _, state := range p.Inputs {
		if err = stillAuthorized(ctx, o, p); err != nil {
			return empty, err
		}
		cached, err := cachedInput(o.CacheRoot, state.Input)
		if err != nil {
			return empty, err
		}
		if state.Cached && !cached {
			return empty, errors.New("acquisition_review_cache_changed")
		}
		if !cached {
			pending := filepath.Join(attempt, state.Filename+".part")
			if err = fetchInput(ctx, state.Input, pending, publicClient(state.URL)); err != nil {
				return empty, err
			}
			if err = stillAuthorized(ctx, o, p); err != nil {
				return empty, err
			}
			if err = publishBlob(o.CacheRoot, state.Input, pending); err != nil {
				return empty, err
			}
		}
		if err = copyPinnedInput(filepath.Join(o.CacheRoot, "blobs", state.SHA256), filepath.Join(inputDirectory, state.Filename), state.Input); err != nil {
			return empty, err
		}
	}
	if err = stillAuthorized(ctx, o, p); err != nil {
		return empty, err
	}
	assembly, err := docling.AssembleProfile(inputDirectory, filepath.Join(attempt, "assembled"), a.recipe.OS, a.recipe.Arch, a.recipe.Profile)
	if err != nil {
		return empty, errors.New("acquisition_canonical_assembly_failed_preserved")
	}
	// The canonical producer's identity is checked again against signed metadata;
	// successful extraction alone never authenticates a release artifact.
	if assembly.ArchiveSHA256 != a.recipe.RuntimeArtifact.SHA256 {
		return empty, errors.New("acquisition_assembled_archive_mismatch_preserved")
	}
	if err = stillAuthorized(ctx, o, p); err != nil {
		return empty, err
	}
	return completeSnapshot(ctx, o, p, attempt, assembly.ArchivePath)
}

func stillAuthorized(ctx context.Context, o Options, p Plan) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	expires, err := time.Parse(time.RFC3339, p.ValidUntil)
	if err != nil || !time.Now().Before(expires) {
		return errors.New("acquisition_review_expired")
	}
	a, err := authenticate(ctx, o, time.Now().UTC())
	if err != nil {
		return err
	}
	expected, _ := releasecontract.CanonicalJSON(p.Recipe)
	current, _ := releasecontract.CanonicalJSON(a.recipe)
	if string(current) != string(expected) {
		return errors.New("acquisition_review_changed")
	}
	return nil
}

func fetchInput(ctx context.Context, input docling.Input, destination string, client *http.Client) error {
	if _, err := publicURL(input.URL, input.URL); err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, input.URL, nil)
	if err != nil {
		return errors.New("acquisition_request_refused")
	}
	request.Header.Set("User-Agent", "Maestro-Engineering-Runtime/1")
	response, err := client.Do(request)
	if err != nil {
		return errors.New("acquisition_upstream_unavailable")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK || response.Header.Get("Content-Encoding") != "" || (response.ContentLength >= 0 && response.ContentLength != input.Size) {
		return errors.New("acquisition_upstream_response_refused")
	}
	return writeInput(destination, input, response.Body)
}

func copyPinnedInput(source, destination string, input docling.Input) error {
	file, err := scheduler.OpenRegularFile(source)
	if err != nil {
		return errors.New("acquisition_input_unavailable")
	}
	defer file.Close()
	if scheduler.ValidatePrivateFile(file, 0600) != nil || scheduler.ValidateSingleLink(file) != nil {
		return errors.New("acquisition_cache_unsafe")
	}
	return writeInput(destination, input, file)
}

func completeSnapshot(ctx context.Context, o Options, p Plan, attempt, archive string) (releaseverify.VerifiedRelease, error) {
	var empty releaseverify.VerifiedRelease
	a, err := authenticate(ctx, o, time.Now().UTC())
	if err != nil {
		return empty, err
	}
	expected, _ := releasecontract.CanonicalJSON(p.Recipe)
	current, _ := releasecontract.CanonicalJSON(a.recipe)
	if string(expected) != string(current) {
		return empty, errors.New("acquisition_review_changed")
	}
	directory := filepath.Join(attempt, "release")
	if err = mkdirPrivateNew(directory); err != nil {
		return empty, errors.New("acquisition_snapshot_conflict_preserved")
	}
	names := make([]string, 0, len(a.carrier))
	for name := range a.carrier {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if err = ctx.Err(); err != nil {
			return empty, err
		}
		if err = scheduler.WriteNewPrivateFile(filepath.Join(directory, name), a.carrier[name]); err != nil {
			return empty, errors.New("acquisition_snapshot_conflict_preserved")
		}
	}
	artifact := a.recipe.RuntimeArtifact
	file, err := scheduler.OpenRegularFile(archive)
	if err != nil {
		return empty, errors.New("acquisition_archive_unavailable")
	}
	defer file.Close()
	if scheduler.ValidateSingleLink(file) != nil {
		return empty, errors.New("acquisition_archive_unsafe")
	}
	if err = writeInput(filepath.Join(directory, artifact.Name), docling.Input{SHA256: artifact.SHA256, Size: artifact.Size}, file); err != nil {
		return empty, err
	}
	if err = stillAuthorized(ctx, o, p); err != nil {
		return empty, err
	}
	// This is deliberately the ordinary complete-directory verifier, without a
	// partial-directory mode or a weaker acquisition-specific trust result.
	verified, err := releaseverify.VerifyDirectory(directory, a.registry)
	if err != nil {
		return empty, errors.New("acquisition_complete_release_verification_failed_preserved")
	}
	if verified.ManifestSHA256 != p.Recipe.ManifestSHA256 {
		return empty, errors.New("acquisition_complete_release_changed")
	}
	if err = stillAuthorized(ctx, o, p); err != nil {
		return empty, err
	}
	return verified, nil
}

// retainedBytes counts only the owned cache, refuses links/special entries and
// bounds traversal before admitting another explicit attempt.
func retainedBytes(root string) (int64, error) {
	var total int64
	count := 0
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return errors.New("acquisition_cache_unsafe")
		}
		count++
		if count > 100000 || entry.Type()&os.ModeSymlink != 0 {
			return errors.New("acquisition_cache_limit")
		}
		if entry.IsDir() {
			return scheduler.ValidateDirectory(path)
		}
		f, err := scheduler.OpenRegularFile(path)
		if err != nil {
			return errors.New("acquisition_cache_unsafe")
		}
		defer f.Close()
		info, err := f.Stat()
		if err != nil || !info.Mode().IsRegular() || scheduler.ValidateSingleLink(f) != nil {
			return errors.New("acquisition_cache_unsafe")
		}
		total += info.Size()
		if total > 2<<30 {
			return errors.New("acquisition_cache_limit")
		}
		return nil
	})
	return total, err
}
