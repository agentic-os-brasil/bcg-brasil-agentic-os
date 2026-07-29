package priorwork

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

var (
	ErrAlreadyEnrolled    = errors.New("prior-work scope is already enrolled")
	ErrNoActiveCatalog    = errors.New("prior-work catalog is not active")
	ErrWatermarkConflict  = errors.New("snapshot previous watermark does not match the active catalog")
	ErrImmutableWatermark = errors.New("snapshot attempts to change an immutable watermark")
	ErrCollectionReplay   = errors.New("snapshot collection sequence is stale or forked")
	ErrImportLocked       = errors.New("another prior-work import holds the exclusive lock")
	ErrRevocationFence    = errors.New("prior-work queries are fenced by an incomplete revocation batch")
	versionPattern        = regexp.MustCompile(`^v-[a-f0-9]{20}$`)
)

type compileFunc func(*os.Root, Catalog) error

type AccessContext struct {
	ActorRef string
	Purpose  string
}

type Store struct {
	Root    string
	compile compileFunc
	clock   func() time.Time
}

func (store Store) now() time.Time {
	if store.clock != nil {
		return store.clock().UTC()
	}
	return time.Now().UTC()
}

func authorize(enrollment Enrollment, access AccessContext) error {
	if access.ActorRef == "" || access.ActorRef != enrollment.AuthorizedBy ||
		access.Purpose != enrollment.Purpose {
		return errors.New("prior-work actor or purpose is not authorized")
	}
	return nil
}

func (store Store) Enroll(enrollment Enrollment) error {
	if err := ValidateEnrollment(enrollment); err != nil {
		return err
	}
	var err error
	enrollment, err = canonicalEnrollment(enrollment)
	if err != nil {
		return err
	}
	if err := store.prepareRoot(); err != nil {
		return err
	}
	root, err := openAnchoredRoot(store.Root)
	if err != nil {
		return err
	}
	defer root.Close()
	if _, err := root.Lstat("enrollment.json"); err == nil {
		return ErrAlreadyEnrolled
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	body, err := marshalPrivate(enrollment)
	if err != nil {
		return err
	}
	file, err := root.OpenFile("enrollment.json", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, os.ErrExist) {
		return ErrAlreadyEnrolled
	}
	if err != nil {
		return err
	}
	closed := false
	defer func() {
		if !closed {
			_ = file.Close()
		}
	}()
	if _, err := file.Write(body); err != nil {
		_ = root.Remove("enrollment.json")
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		closed = true
		_ = root.Remove("enrollment.json")
		return err
	}
	if err := file.Close(); err != nil {
		closed = true
		_ = root.Remove("enrollment.json")
		return err
	}
	closed = true
	return syncRootDirectory(root, ".")
}

func (store Store) Apply(snapshot Snapshot, receipt ImportReceipt, access AccessContext) (ApplyReport, error) {
	if err := ValidateSnapshot(snapshot); err != nil {
		return ApplyReport{}, err
	}
	if err := ValidateImportReceipt(receipt, snapshot); err != nil {
		return ApplyReport{}, err
	}
	publishedAt := store.now()
	if publishedAt.Before(receipt.EmittedAt) {
		return ApplyReport{}, errors.New("catalog publication time predates its import receipt")
	}
	if err := store.prepareRoot(); err != nil {
		return ApplyReport{}, err
	}
	releaseLock, err := store.acquireImportLock()
	if err != nil {
		return ApplyReport{}, err
	}
	defer releaseLock()
	enrollment, err := store.loadEnrollment()
	if err != nil {
		return ApplyReport{}, err
	}
	if err := authorize(enrollment, access); err != nil {
		return ApplyReport{}, err
	}
	if err := VerifyImportReceipt(receipt, snapshot, enrollment); err != nil {
		return ApplyReport{}, err
	}
	if !publishedAt.Before(enrollment.AuthorizationExpiresAt) {
		return ApplyReport{}, errors.New("prior-work enrollment authorization has expired")
	}
	if snapshot.TenantRef != enrollment.TenantRef || !rootsEqual(snapshot.Roots, enrollment.Roots) {
		return ApplyReport{}, errors.New("snapshot tenant or roots do not match enrollment")
	}
	if len(snapshot.Items)+len(snapshot.Tombstones) > enrollment.MaxSnapshotItems {
		return ApplyReport{}, errors.New("snapshot exceeds enrolled item limit")
	}
	allowedTypes := make(map[string]bool, len(enrollment.AllowedItemTypes))
	for _, itemType := range enrollment.AllowedItemTypes {
		allowedTypes[itemType] = true
	}
	allowedOrigins := make(map[string]bool, len(enrollment.AllowedOrigins))
	for _, origin := range enrollment.AllowedOrigins {
		allowedOrigins[strings.ToLower(strings.TrimSuffix(origin, "/"))] = true
	}
	for _, item := range snapshot.Items {
		if !allowedTypes[item.Kind] || item.SizeBytes > enrollment.MaxItemBytes {
			return ApplyReport{}, errors.New("snapshot item exceeds enrolled type or size authority")
		}
		parsed, _ := url.Parse(item.SourceURL)
		origin := strings.ToLower(parsed.Scheme + "://" + parsed.Host)
		if !allowedOrigins[origin] || parsed.RawQuery != "" || parsed.Fragment != "" {
			return ApplyReport{}, errors.New("snapshot item URL is outside enrolled origins or contains query/fragment data")
		}
	}
	enrollmentFingerprint, err := fingerprintEnrollment(enrollment)
	if err != nil {
		return ApplyReport{}, err
	}
	snapshotDigest, err := fingerprintSnapshot(snapshot)
	if err != nil {
		return ApplyReport{}, err
	}

	var active Manifest
	var current Catalog
	active, current, err = store.loadActive()
	if err != nil && !errors.Is(err, ErrNoActiveCatalog) {
		return ApplyReport{}, err
	}
	hasActive := err == nil
	if !hasActive && snapshot.CollectionSequence != 1 {
		return ApplyReport{}, ErrNoActiveCatalog
	}
	if hasActive && snapshot.CollectionSequence != active.CollectionSequence &&
		snapshot.CollectionSequence != active.CollectionSequence+1 {
		return ApplyReport{}, ErrCollectionReplay
	}
	if hasActive && snapshot.CollectionSequence == active.CollectionSequence+1 &&
		snapshot.PreviousWatermark != active.Watermark {
		return ApplyReport{}, ErrWatermarkConflict
	}

	items := map[string]Item{}
	if snapshot.Mode == "delta" {
		for _, item := range current.Items {
			items[item.key()] = item
		}
	}
	for _, item := range snapshot.Items {
		items[item.key()] = item
	}
	for _, tombstone := range snapshot.Tombstones {
		delete(items, tombstone.key())
	}

	var missing []Tombstone
	if snapshot.Mode == "full" && hasActive {
		for _, oldItem := range current.Items {
			if _, found := items[oldItem.key()]; !found {
				missing = append(missing, Tombstone{
					ItemRef:    oldItem.ItemRef,
					Root:       oldItem.Root,
					Reason:     "deleted",
					ObservedAt: snapshot.GeneratedAt,
				})
			}
		}
	}

	catalog := Catalog{
		SchemaVersion:      1,
		TenantRef:          snapshot.TenantRef,
		PolicyVersion:      enrollment.PolicyVersion,
		CollectionSequence: snapshot.CollectionSequence,
		Watermark:          snapshot.Watermark,
		SnapshotDigest:     snapshotDigest,
		GeneratedAt:        snapshot.GeneratedAt,
		Roots:              append([]RootRef(nil), snapshot.Roots...),
		Items:              make([]Item, 0, len(items)),
	}
	for _, item := range items {
		catalog.Items = append(catalog.Items, item)
	}
	catalog, err = canonicalCatalog(catalog)
	if err != nil {
		return ApplyReport{}, err
	}
	fingerprint, err := catalogFingerprint(catalog)
	if err != nil {
		return ApplyReport{}, err
	}
	if hasActive && snapshot.CollectionSequence == active.CollectionSequence {
		if active.Watermark != snapshot.Watermark {
			return ApplyReport{}, ErrCollectionReplay
		}
		if active.Fingerprint != fingerprint {
			return ApplyReport{}, ErrImmutableWatermark
		}
		if err := store.clearActiveBarriers(snapshot.Items, active.CollectionSequence); err != nil {
			return ApplyReport{}, err
		}
		return ApplyReport{
			State: "unchanged", Version: active.Version, Fingerprint: active.Fingerprint,
			Watermark: active.Watermark, CollectionSequence: active.CollectionSequence, Items: active.ItemCount,
		}, nil
	}

	allTombstones := append(append([]Tombstone(nil), snapshot.Tombstones...), missing...)
	if err := store.writeBarriers(allTombstones, snapshot, enrollment, enrollmentFingerprint, snapshotDigest); err != nil {
		return ApplyReport{}, err
	}

	version := "v-" + fingerprint[:20]
	stage, err := os.MkdirTemp("", "maestro-prior-work-compile-")
	if err != nil {
		return ApplyReport{}, err
	}
	defer os.RemoveAll(stage)
	stageRoot, err := openAnchoredRoot(stage)
	if err != nil {
		return ApplyReport{}, err
	}
	defer stageRoot.Close()
	compiler := store.compile
	if compiler == nil {
		compiler = compileAt
	}
	if err := compiler(stageRoot, catalog); err != nil {
		return ApplyReport{}, fmt.Errorf("compile prior-work catalog: %w", err)
	}
	manifest := Manifest{
		SchemaVersion:         1,
		Version:               version,
		TenantRef:             catalog.TenantRef,
		CollectionSequence:    catalog.CollectionSequence,
		Watermark:             catalog.Watermark,
		Fingerprint:           fingerprint,
		PolicyVersion:         catalog.PolicyVersion,
		EnrollmentFingerprint: enrollmentFingerprint,
		SnapshotDigest:        snapshotDigest,
		CompilerVersion:       compilerVersion,
		PublishedAt:           publishedAt.UTC(),
		ItemCount:             len(catalog.Items),
	}
	if err := writePrivateFileAt(stageRoot, "manifest.json", manifest); err != nil {
		return ApplyReport{}, err
	}
	if err := validateBundleAt(stageRoot, catalog); err != nil {
		return ApplyReport{}, fmt.Errorf("validate prior-work catalog: %w", err)
	}

	root, err := openAnchoredRoot(store.Root)
	if err != nil {
		return ApplyReport{}, err
	}
	defer root.Close()
	finalRelative := filepath.Join("versions", version)
	if info, err := root.Lstat(finalRelative); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return ApplyReport{}, errors.New("immutable catalog version is not a real directory")
		}
		existing, loadErr := loadJSONAt[Manifest](store.Root, filepath.Join("versions", version, "manifest.json"))
		if loadErr != nil || !sameImmutableManifest(existing, manifest) {
			return ApplyReport{}, errors.New("immutable catalog version collision")
		}
		manifest = existing
	} else if !errors.Is(err, os.ErrNotExist) {
		return ApplyReport{}, err
	} else {
		stageRelative, err := publishBundle(root, stageRoot)
		if err != nil {
			return ApplyReport{}, err
		}
		defer root.RemoveAll(stageRelative)
		if err := root.Rename(stageRelative, finalRelative); err != nil {
			return ApplyReport{}, err
		}
	}
	if err := syncRootDirectory(root, "versions"); err != nil {
		return ApplyReport{}, err
	}
	if err := writeSnapshot(store.Root, fingerprint, snapshot); err != nil {
		return ApplyReport{}, err
	}
	if err := store.verifyActiveUnchanged(hasActive, active); err != nil {
		return ApplyReport{}, err
	}
	if err := writeActiveManifest(store.Root, manifest); err != nil {
		return ApplyReport{}, err
	}
	if err := store.clearActiveBarriers(snapshot.Items, snapshot.CollectionSequence); err != nil {
		return ApplyReport{}, err
	}
	return ApplyReport{
		State: "published", Version: version, Fingerprint: fingerprint,
		Watermark: snapshot.Watermark, CollectionSequence: snapshot.CollectionSequence,
		Items: len(catalog.Items), Removed: len(allTombstones),
	}, nil
}

func sameImmutableManifest(left, right Manifest) bool {
	left.PublishedAt = time.Time{}
	right.PublishedAt = time.Time{}
	return left == right
}

func (store Store) Status(access AccessContext) (Status, error) {
	enrollment, err := store.loadEnrollment()
	if err != nil {
		return Status{}, err
	}
	if err := authorize(enrollment, access); err != nil {
		return Status{}, err
	}
	now := store.now()
	if !now.Before(enrollment.AuthorizationExpiresAt) {
		return Status{
			State: "authorization_expired", Due: true, Stale: true,
			RefreshHours: enrollment.RefreshHours, StaleHours: enrollment.StaleHours,
		}, nil
	}
	manifest, _, err := store.loadActive()
	if errors.Is(err, ErrNoActiveCatalog) {
		return Status{
			State: "absent", Due: true, Stale: true,
			RefreshHours: enrollment.RefreshHours, StaleHours: enrollment.StaleHours,
		}, nil
	}
	if err != nil {
		return Status{}, err
	}
	enrollmentFingerprint, err := fingerprintEnrollment(enrollment)
	if err != nil {
		return Status{}, err
	}
	if manifest.PolicyVersion != enrollment.PolicyVersion ||
		manifest.EnrollmentFingerprint != enrollmentFingerprint ||
		manifest.TenantRef != enrollment.TenantRef {
		return Status{}, errors.New("active prior-work catalog is stale against enrollment")
	}
	state := freshnessState(manifest.PublishedAt, enrollment, now)
	return Status{
		State: state, Due: state != "fresh", Stale: state == "stale",
		LastSyncAt: manifest.PublishedAt, Watermark: manifest.Watermark,
		CollectionSequence: manifest.CollectionSequence,
		Fingerprint:        manifest.Fingerprint, Items: manifest.ItemCount,
		RefreshHours: enrollment.RefreshHours, StaleHours: enrollment.StaleHours,
	}, nil
}

func freshnessState(publishedAt time.Time, enrollment Enrollment, now time.Time) string {
	age := now.Sub(publishedAt)
	if age >= time.Duration(enrollment.StaleHours)*time.Hour {
		return "stale"
	}
	if age >= time.Duration(enrollment.RefreshHours)*time.Hour {
		return "due"
	}
	return "fresh"
}

func (store Store) loadEnrollment() (Enrollment, error) {
	enrollment, err := loadJSONAt[Enrollment](store.Root, "enrollment.json")
	if err != nil {
		return Enrollment{}, err
	}
	if err := ValidateEnrollment(enrollment); err != nil {
		return Enrollment{}, err
	}
	return enrollment, nil
}

func (store Store) loadActive() (Manifest, Catalog, error) {
	manifest, err := loadJSONAt[Manifest](store.Root, "active.json")
	if errors.Is(err, os.ErrNotExist) {
		return Manifest{}, Catalog{}, ErrNoActiveCatalog
	}
	if err != nil {
		return Manifest{}, Catalog{}, err
	}
	if manifest.SchemaVersion != 1 || !versionPattern.MatchString(manifest.Version) ||
		manifest.CollectionSequence == 0 || manifest.Fingerprint == "" ||
		manifest.Watermark == "" || manifest.PolicyVersion == "" ||
		manifest.EnrollmentFingerprint == "" || manifest.SnapshotDigest == "" ||
		manifest.CompilerVersion != compilerVersion || manifest.PublishedAt.IsZero() {
		return Manifest{}, Catalog{}, errors.New("invalid active prior-work manifest")
	}
	catalog, err := loadJSONAt[Catalog](store.Root, filepath.Join("versions", manifest.Version, "items.json"))
	if err != nil {
		return Manifest{}, Catalog{}, err
	}
	fingerprint, err := catalogFingerprint(catalog)
	if err != nil {
		return Manifest{}, Catalog{}, err
	}
	if fingerprint != manifest.Fingerprint || catalog.Watermark != manifest.Watermark ||
		catalog.CollectionSequence != manifest.CollectionSequence ||
		catalog.TenantRef != manifest.TenantRef || catalog.PolicyVersion != manifest.PolicyVersion ||
		catalog.SnapshotDigest != manifest.SnapshotDigest ||
		len(catalog.Items) != manifest.ItemCount {
		return Manifest{}, Catalog{}, errors.New("active prior-work manifest does not match its catalog")
	}
	return manifest, catalog, nil
}

func (store Store) writeBarriers(
	tombstones []Tombstone,
	snapshot Snapshot,
	enrollment Enrollment,
	enrollmentFingerprint string,
	snapshotDigest string,
) error {
	if len(tombstones) == 0 {
		return nil
	}
	fence := RevocationFence{
		SchemaVersion: 1, TenantRef: snapshot.TenantRef,
		CollectionSequence: snapshot.CollectionSequence,
		Watermark:          snapshot.Watermark, SnapshotDigest: snapshotDigest,
	}
	if err := atomicWriteAt(store.Root, "revocation-fence.json", fence); err != nil {
		return err
	}
	for _, tombstone := range tombstones {
		barrier := Barrier{
			SchemaVersion:         1,
			TenantRef:             snapshot.TenantRef,
			Root:                  tombstone.Root,
			ItemRef:               tombstone.ItemRef,
			Reason:                tombstone.Reason,
			ObservedAt:            tombstone.ObservedAt,
			CollectionSequence:    snapshot.CollectionSequence,
			Watermark:             snapshot.Watermark,
			PolicyVersion:         enrollment.PolicyVersion,
			EnrollmentFingerprint: enrollmentFingerprint,
			SnapshotDigest:        snapshotDigest,
		}
		if err := atomicWriteAt(store.Root, filepath.Join("barriers", opaqueFilename(tombstone.key())+".json"), barrier); err != nil {
			return err
		}
	}
	root, err := openAnchoredRoot(store.Root)
	if err != nil {
		return err
	}
	defer root.Close()
	if err := syncRootDirectory(root, "barriers"); err != nil {
		return err
	}
	if err := root.Remove("revocation-fence.json"); err != nil {
		return err
	}
	return syncRootDirectory(root, ".")
}

func (store Store) clearActiveBarriers(items []Item, publishedSequence uint64) error {
	for _, item := range items {
		relative := filepath.Join("barriers", opaqueFilename(item.key())+".json")
		barrier, err := loadJSONAt[Barrier](store.Root, relative)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		if barrier.ItemRef != item.ItemRef || barrier.Root.key() != item.Root.key() {
			return errors.New("barrier filename collision")
		}
		if barrier.CollectionSequence >= publishedSequence {
			continue
		}
		root, err := openAnchoredRoot(store.Root)
		if err != nil {
			return err
		}
		removeErr := root.Remove(relative)
		closeErr := root.Close()
		if removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			return removeErr
		}
		if closeErr != nil {
			return closeErr
		}
	}
	root, err := openAnchoredRoot(store.Root)
	if err != nil {
		return err
	}
	defer root.Close()
	return syncRootDirectory(root, "barriers")
}

func (store Store) loadBarriers() (map[string]bool, error) {
	if fence, err := loadJSONAt[RevocationFence](store.Root, "revocation-fence.json"); err == nil {
		if fence.SchemaVersion != 1 || !opaqueRefPattern.MatchString(fence.TenantRef) ||
			fence.CollectionSequence == 0 || !watermarkPattern.MatchString(fence.Watermark) ||
			!digestPattern.MatchString(fence.SnapshotDigest) {
			return nil, errors.New("invalid prior-work revocation fence")
		}
		return nil, ErrRevocationFence
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	root, err := openAnchoredRoot(store.Root)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	directory, err := root.Open("barriers")
	if err != nil {
		return nil, err
	}
	entries, err := directory.ReadDir(-1)
	closeErr := directory.Close()
	if err != nil {
		return nil, err
	}
	if closeErr != nil {
		return nil, closeErr
	}
	barriers := map[string]bool{}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		barrier, err := loadJSONAt[Barrier](store.Root, filepath.Join("barriers", entry.Name()))
		if err != nil {
			return nil, err
		}
		if barrier.SchemaVersion != 1 || !opaqueRefPattern.MatchString(barrier.TenantRef) ||
			validateRoot(barrier.Root) != nil || !opaqueRefPattern.MatchString(barrier.ItemRef) ||
			barrier.CollectionSequence == 0 || barrier.PolicyVersion == "" ||
			barrier.EnrollmentFingerprint == "" || barrier.SnapshotDigest == "" ||
			(barrier.Reason != "deleted" && barrier.Reason != "access_revoked") {
			return nil, errors.New("invalid prior-work barrier")
		}
		barriers[barrier.Root.key()+"\x00"+barrier.ItemRef] = true
	}
	return barriers, nil
}

func (store Store) prepareRoot() error {
	if strings.TrimSpace(store.Root) == "" {
		return errors.New("prior-work root is required")
	}
	absolute, err := filepath.Abs(store.Root)
	if err != nil {
		return err
	}
	if absolute != filepath.Clean(store.Root) {
		return errors.New("prior-work root must be an absolute clean path")
	}
	if _, err := os.Lstat(absolute); errors.Is(err, os.ErrNotExist) {
		parent, openErr := openAnchoredRoot(filepath.Dir(absolute))
		if openErr != nil {
			return openErr
		}
		mkdirErr := parent.Mkdir(filepath.Base(absolute), 0o700)
		closeErr := parent.Close()
		if mkdirErr != nil && !errors.Is(mkdirErr, os.ErrExist) {
			return mkdirErr
		}
		if closeErr != nil {
			return closeErr
		}
	} else if err != nil {
		return err
	}
	root, err := openAnchoredRoot(absolute)
	if err != nil {
		return err
	}
	defer root.Close()
	rootInfo, err := root.Stat(".")
	if err != nil {
		return err
	}
	if rootInfo.Mode().Perm()&0o077 != 0 {
		if err := root.Chmod(".", 0o700); err != nil {
			return err
		}
		rootInfo, err = root.Stat(".")
		if err != nil {
			return err
		}
	}
	if rootInfo.Mode().Perm()&0o077 != 0 {
		return errors.New("prior-work root permissions must not grant group or other access")
	}
	for _, child := range []string{"barriers", "snapshots", "versions"} {
		if err := root.Mkdir(child, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
			return err
		}
		info, err := root.Lstat(child)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return errors.New("prior-work storage path must be a real directory")
		}
		opened, err := root.OpenRoot(child)
		if err != nil {
			return err
		}
		openedInfo, statErr := opened.Stat(".")
		if statErr != nil {
			_ = opened.Close()
			return statErr
		}
		if !os.SameFile(info, openedInfo) {
			_ = opened.Close()
			return errors.New("prior-work storage path changed during secure open")
		}
		if openedInfo.Mode().Perm()&0o077 != 0 {
			if err := opened.Chmod(".", 0o700); err != nil {
				_ = opened.Close()
				return err
			}
			openedInfo, statErr = opened.Stat(".")
			if statErr != nil {
				_ = opened.Close()
				return statErr
			}
		}
		closeErr := opened.Close()
		if openedInfo.Mode().Perm()&0o077 != 0 {
			return errors.New("prior-work storage path permissions are not private")
		}
		if closeErr != nil {
			return closeErr
		}
	}
	if err := syncRootDirectory(root, "."); err != nil {
		return err
	}
	return nil
}

func ensurePrivateDirectory(path string) error {
	if err := os.MkdirAll(path, 0o700); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return errors.New("prior-work storage path must not be a symlink")
	}
	return os.Chmod(path, 0o700)
}

func canonicalCatalog(input Catalog) (Catalog, error) {
	body, err := json.Marshal(input)
	if err != nil {
		return Catalog{}, err
	}
	var catalog Catalog
	if err := json.Unmarshal(body, &catalog); err != nil {
		return Catalog{}, err
	}
	sort.Slice(catalog.Roots, func(i, j int) bool { return catalog.Roots[i].key() < catalog.Roots[j].key() })
	for index := range catalog.Items {
		canonicalizeItem(&catalog.Items[index])
	}
	sort.Slice(catalog.Items, func(i, j int) bool { return catalog.Items[i].key() < catalog.Items[j].key() })
	return catalog, nil
}

func canonicalizeItem(item *Item) {
	sort.Strings(item.Facets.Clients)
	sort.Strings(item.Facets.Projects)
	sort.Strings(item.Facets.Themes)
	sort.Ints(item.Facets.Years)
	sort.Strings(item.Facets.Audiences)
	sort.Strings(item.Facets.People)
	sort.Strings(item.Facets.Presenters)
	sort.Strings(item.SearchTerms)
}

func canonicalEnrollment(input Enrollment) (Enrollment, error) {
	body, err := json.Marshal(input)
	if err != nil {
		return Enrollment{}, err
	}
	var enrollment Enrollment
	if err := json.Unmarshal(body, &enrollment); err != nil {
		return Enrollment{}, err
	}
	sort.Slice(enrollment.Roots, func(i, j int) bool { return enrollment.Roots[i].key() < enrollment.Roots[j].key() })
	sort.Strings(enrollment.AllowedItemTypes)
	sort.Strings(enrollment.AllowedOrigins)
	return enrollment, nil
}

func fingerprintEnrollment(enrollment Enrollment) (string, error) {
	canonical, err := canonicalEnrollment(enrollment)
	if err != nil {
		return "", err
	}
	return fingerprintValue(canonical)
}

func fingerprintSnapshot(input Snapshot) (string, error) {
	body, err := json.Marshal(input)
	if err != nil {
		return "", err
	}
	var snapshot Snapshot
	if err := json.Unmarshal(body, &snapshot); err != nil {
		return "", err
	}
	sort.Slice(snapshot.Roots, func(i, j int) bool { return snapshot.Roots[i].key() < snapshot.Roots[j].key() })
	sort.Slice(snapshot.RootResults, func(i, j int) bool {
		return snapshot.RootResults[i].Root.key() < snapshot.RootResults[j].Root.key()
	})
	for index := range snapshot.Items {
		canonicalizeItem(&snapshot.Items[index])
	}
	sort.Slice(snapshot.Items, func(i, j int) bool { return snapshot.Items[i].key() < snapshot.Items[j].key() })
	sort.Slice(snapshot.Tombstones, func(i, j int) bool { return snapshot.Tombstones[i].key() < snapshot.Tombstones[j].key() })
	return fingerprintValue(snapshot)
}

func catalogFingerprint(catalog Catalog) (string, error) {
	catalog, err := canonicalCatalog(catalog)
	if err != nil {
		return "", err
	}
	payload := struct {
		SchemaVersion      int       `json:"schema_version"`
		TenantRef          string    `json:"tenant_ref"`
		PolicyVersion      string    `json:"policy_version"`
		CollectionSequence uint64    `json:"collection_sequence"`
		Watermark          string    `json:"watermark"`
		SnapshotDigest     string    `json:"snapshot_digest"`
		Roots              []RootRef `json:"roots"`
		Items              []Item    `json:"items"`
	}{
		SchemaVersion:      catalog.SchemaVersion,
		TenantRef:          catalog.TenantRef,
		PolicyVersion:      catalog.PolicyVersion,
		CollectionSequence: catalog.CollectionSequence,
		Watermark:          catalog.Watermark,
		SnapshotDigest:     catalog.SnapshotDigest,
		Roots:              catalog.Roots,
		Items:              catalog.Items,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:]), nil
}

func fingerprintValue(value any) (string, error) {
	body, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:]), nil
}

func opaqueFilename(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func writeSnapshot(root, fingerprint string, snapshot Snapshot) error {
	relative := filepath.Join("snapshots", fingerprint+".json")
	anchored, err := openAnchoredRoot(root)
	if err != nil {
		return err
	}
	defer anchored.Close()
	body, err := marshalPrivate(snapshot)
	if err != nil {
		return err
	}
	temp, err := randomStageName("snapshots", ".snapshot-")
	if err != nil {
		return err
	}
	file, err := anchored.OpenFile(temp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	defer anchored.Remove(temp)
	if _, err := file.Write(body); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := anchored.Link(temp, relative); err == nil {
		if err := anchored.Remove(temp); err != nil {
			return err
		}
		return syncRootDirectory(anchored, "snapshots")
	} else if !errors.Is(err, os.ErrExist) {
		return err
	}
	existing, err := loadJSONRoot[Snapshot](anchored, relative)
	if err != nil {
		return errors.New("immutable prior-work snapshot is invalid")
	}
	existingDigest, err := fingerprintSnapshot(existing)
	if err != nil {
		return err
	}
	expectedDigest, err := fingerprintSnapshot(snapshot)
	if err != nil {
		return err
	}
	if existingDigest != expectedDigest {
		return errors.New("immutable prior-work snapshot collision")
	}
	return nil
}

func marshalPrivate(value any) ([]byte, error) {
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(body, '\n'), nil
}

func writePrivateFileAt(root *os.Root, path string, value any) error {
	body, err := marshalPrivate(value)
	if err != nil {
		return err
	}
	file, err := root.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := file.Write(body); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

func atomicWriteAt(rootPath, relative string, value any) error {
	body, err := marshalPrivate(value)
	if err != nil {
		return err
	}
	root, err := openAnchoredRoot(rootPath)
	if err != nil {
		return err
	}
	defer root.Close()
	clean := filepath.Clean(relative)
	if clean == "." || filepath.IsAbs(clean) || clean == ".." ||
		strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return errors.New("prior-work write escaped its storage root")
	}
	parent := filepath.Dir(clean)
	if err := root.MkdirAll(parent, 0o700); err != nil {
		return err
	}
	temp, err := randomStageName(parent, ".write-")
	if err != nil {
		return err
	}
	file, err := root.OpenFile(temp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	defer root.Remove(temp)
	if _, err := file.Write(body); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := root.Rename(temp, clean); err != nil {
		return err
	}
	return syncRootDirectory(root, parent)
}

func randomStageName(parent, prefix string) (string, error) {
	var suffix [16]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		return "", err
	}
	return filepath.Join(parent, prefix+hex.EncodeToString(suffix[:])), nil
}

func publishBundle(root, source *os.Root) (string, error) {
	stage, err := randomStageName("versions", ".stage-")
	if err != nil {
		return "", err
	}
	if err := root.Mkdir(stage, 0o700); err != nil {
		return "", err
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = root.RemoveAll(stage)
		}
	}()
	err = fs.WalkDir(source.FS(), ".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == "." {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return errors.New("compiled prior-work bundle contains a symlink")
		}
		relative := filepath.FromSlash(path)
		target := filepath.Join(stage, relative)
		if entry.IsDir() {
			return root.Mkdir(target, 0o700)
		}
		info, err := source.Lstat(relative)
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return errors.New("compiled prior-work bundle contains a non-regular file")
		}
		input, err := source.Open(relative)
		if err != nil {
			return err
		}
		openedInfo, err := input.Stat()
		if err != nil {
			_ = input.Close()
			return err
		}
		if !os.SameFile(info, openedInfo) {
			_ = input.Close()
			return errors.New("compiled prior-work bundle changed during secure open")
		}
		output, err := root.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			_ = input.Close()
			return err
		}
		written, copyErr := io.Copy(output, io.LimitReader(input, maximumSnapshotBytes+1))
		inputCloseErr := input.Close()
		syncErr := output.Sync()
		outputCloseErr := output.Close()
		if copyErr != nil {
			return copyErr
		}
		if written > maximumSnapshotBytes {
			return errors.New("compiled prior-work bundle file exceeds the safe publication limit")
		}
		if inputCloseErr != nil {
			return inputCloseErr
		}
		if syncErr != nil {
			return syncErr
		}
		return outputCloseErr
	})
	if err != nil {
		return "", err
	}
	if err := syncRootDirectory(root, stage); err != nil {
		return "", err
	}
	cleanup = false
	return stage, nil
}

func syncRootDirectory(root *os.Root, relative string) error {
	directory, err := root.Open(relative)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}

func loadJSONAt[T any](rootPath, relative string) (T, error) {
	var zero T
	root, err := openAnchoredRoot(rootPath)
	if err != nil {
		return zero, err
	}
	defer root.Close()
	return loadJSONRoot[T](root, relative)
}

func loadJSONRoot[T any](root *os.Root, relative string) (T, error) {
	var zero T
	clean := filepath.Clean(relative)
	if clean == "." || filepath.IsAbs(clean) || clean == ".." ||
		strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return zero, errors.New("prior-work read escaped its storage root")
	}
	info, err := root.Lstat(clean)
	if err != nil {
		return zero, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return zero, errors.New("prior-work JSON path must be a regular file, not a symlink")
	}
	file, err := root.Open(clean)
	if err != nil {
		return zero, err
	}
	defer file.Close()
	openedInfo, err := file.Stat()
	if err != nil {
		return zero, err
	}
	if !os.SameFile(info, openedInfo) {
		return zero, errors.New("prior-work JSON path changed during secure open")
	}
	body, err := io.ReadAll(io.LimitReader(file, maximumSnapshotBytes+1))
	if err != nil {
		return zero, err
	}
	if len(body) > maximumSnapshotBytes {
		return zero, errors.New("prior-work JSON file exceeds the safe read limit")
	}
	return decodeStrictJSON[T](body)
}

func openAnchoredRoot(path string) (*os.Root, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("prior-work root is required")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	if absolute != filepath.Clean(path) {
		return nil, errors.New("prior-work root must be an absolute clean path")
	}
	parent, err := os.OpenRoot(filepath.Dir(absolute))
	if err != nil {
		return nil, err
	}
	defer parent.Close()
	base := filepath.Base(absolute)
	info, err := parent.Lstat(base)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return nil, errors.New("prior-work root must be a real directory")
	}
	root, err := parent.OpenRoot(base)
	if err != nil {
		return nil, err
	}
	openedInfo, err := root.Stat(".")
	if err != nil {
		_ = root.Close()
		return nil, err
	}
	if !os.SameFile(info, openedInfo) {
		_ = root.Close()
		return nil, errors.New("prior-work root changed during secure open")
	}
	return root, nil
}

func decodeStrictJSON[T any](body []byte) (T, error) {
	var zero T
	if err := rejectDuplicateJSONKeys(body); err != nil {
		return zero, err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var value T
	if err := decoder.Decode(&value); err != nil {
		return zero, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return zero, errors.New("JSON file contains multiple values")
		}
		return zero, err
	}
	return value, nil
}

func (store Store) acquireImportLock() (func(), error) {
	root, err := openAnchoredRoot(store.Root)
	if err != nil {
		return nil, err
	}
	file, err := root.OpenFile("import.lock", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, os.ErrExist) {
		_ = root.Close()
		return nil, ErrImportLocked
	}
	if err != nil {
		_ = root.Close()
		return nil, err
	}
	lock := struct {
		SchemaVersion int       `json:"schema_version"`
		CreatedAt     time.Time `json:"created_at"`
	}{SchemaVersion: 1, CreatedAt: time.Now().UTC()}
	body, err := marshalPrivate(lock)
	if err == nil {
		_, err = file.Write(body)
	}
	if err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		_ = root.Remove("import.lock")
		_ = root.Close()
		return nil, err
	}
	return func() {
		_ = root.Remove("import.lock")
		_ = root.Close()
	}, nil
}

func (store Store) verifyActiveUnchanged(hadActive bool, expected Manifest) error {
	current, _, err := store.loadActive()
	if !hadActive {
		if errors.Is(err, ErrNoActiveCatalog) {
			return nil
		}
		if err != nil {
			return err
		}
		return errors.New("active catalog appeared during import")
	}
	if err != nil {
		return err
	}
	if current.Fingerprint != expected.Fingerprint ||
		current.CollectionSequence != expected.CollectionSequence ||
		current.Watermark != expected.Watermark {
		return errors.New("active catalog changed during import")
	}
	return nil
}
