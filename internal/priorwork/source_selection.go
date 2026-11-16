package priorwork

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

const (
	MaximumSourceSelectionInputBytes  = 128 << 10
	maximumSourceSelectionRecordBytes = 1 << 20 // Stored URLs may expand sixfold when JSON escapes HTML characters.
	maximumSelectedFolders            = 32
	SourceSelectionRequired           = "selection_required"
	SourceSelected                    = "selected"
	SourceDeferred                    = "deferred"
	SourceRevoked                     = "revoked"
	SourceSelectionUnavailable        = "unavailable"
)

var (
	ErrSourceSelectionTooLarge      = errors.New("SharePoint source selection exceeds the safe input limit")
	ErrLibraryRootNeedsProjectScope = errors.New("SharePoint library root needs a project scope before collection")
	workspaceIDPattern              = regexp.MustCompile(`^[a-f0-9]{32}$`)
)

// SourceSelectionInput is the only body accepted from a guided runtime. It
// carries exact source pointers but no credentials, document body or inferred
// scope. The workspace identity is derived by the CLI and cannot be supplied
// by prompt text.
type SourceSelectionInput struct {
	SchemaVersion int      `json:"schema_version"`
	FolderURLs    []string `json:"folder_urls"`
}

type sourceSelection struct {
	SchemaVersion int       `json:"schema_version"`
	WorkspaceID   string    `json:"workspace_id"`
	State         string    `json:"state"`
	Source        string    `json:"source"`
	Purpose       string    `json:"purpose"`
	FolderURLs    []string  `json:"folder_urls"`
	Fingerprint   string    `json:"fingerprint"`
	Version       string    `json:"version"`
	RecordedAt    time.Time `json:"recorded_at"`
}

type sourceSelectionActive struct {
	SchemaVersion int    `json:"schema_version"`
	WorkspaceID   string `json:"workspace_id"`
	Version       string `json:"version"`
	Fingerprint   string `json:"fingerprint"`
}

// SourceSelectionStatus is safe for Session Start. Exact folder URLs remain
// behind Pointer in private local storage and are never serialized here.
type SourceSelectionStatus struct {
	SchemaVersion        int    `json:"schema_version"`
	State                string `json:"state"`
	WorkspaceID          string `json:"workspace_id,omitempty"`
	Version              string `json:"version,omitempty"`
	Fingerprint          string `json:"fingerprint,omitempty"`
	Pointer              string `json:"pointer,omitempty"`
	FolderCount          int    `json:"folder_count"`
	SourceAuthority      string `json:"source_authority"`
	LocalProjection      string `json:"local_projection"`
	AuthorizationState   string `json:"authorization_state"`
	CollectionRuntime    string `json:"collection_runtime"`
	CollectionState      string `json:"collection_state"`
	CodexCollectionState string `json:"codex_collection_state"`
}

type SourceSelectionStore struct {
	Root string
	// CustodyRoot is the admitted owner directory; it is never supplied by a model.
	CustodyRoot string
	clock       func() time.Time
}

// SelectedFolders returns the exact reviewed pointers for a workspace. It is
// intentionally callable only by a bounded local ingestion path; Session Start
// continues to expose counts and fingerprints, never these URLs.
func (store SourceSelectionStore) SelectedFolders(workspaceID string) ([]string, error) {
	if err := validateWorkspaceID(workspaceID); err != nil {
		return nil, err
	}
	review, err := store.PrepareSelection(workspaceID, SourceDeferred, nil)
	if err != nil {
		return nil, err
	}
	defer review.Close()
	if review.CurrentStatus.State != SourceSelected {
		return nil, errors.New("SharePoint source folders are not selected")
	}
	record := review.Current
	for _, folder := range record.FolderURLs {
		if isSharePointLibraryRoot(folder) {
			// Keep the owner's pointer and let the conversational UI continue,
			// but never hand a whole document library to a collector implicitly.
			return nil, ErrLibraryRootNeedsProjectScope
		}
	}
	return append([]string(nil), record.FolderURLs...), nil
}

func ParseSourceSelectionInput(reader io.Reader) (SourceSelectionInput, error) {
	body, err := io.ReadAll(io.LimitReader(reader, MaximumSourceSelectionInputBytes+1))
	if err != nil {
		return SourceSelectionInput{}, err
	}
	if len(body) > MaximumSourceSelectionInputBytes {
		return SourceSelectionInput{}, ErrSourceSelectionTooLarge
	}
	if err := rejectDuplicateJSONKeys(body); err != nil {
		return SourceSelectionInput{}, fmt.Errorf("decode SharePoint source selection: %w", err)
	}
	input, err := decodeStrictJSON[SourceSelectionInput](body)
	if err != nil {
		return SourceSelectionInput{}, fmt.Errorf("decode SharePoint source selection: %w", err)
	}
	if input.SchemaVersion != 1 {
		return SourceSelectionInput{}, errors.New("SharePoint source selection schema_version must be 1")
	}
	canonical, err := canonicalFolderURLs(input.FolderURLs)
	if err != nil {
		return SourceSelectionInput{}, err
	}
	input.FolderURLs = canonical
	return input, nil
}

func (store SourceSelectionStore) now() time.Time {
	if store.clock != nil {
		return store.clock().UTC()
	}
	return time.Now().UTC()
}

func (store SourceSelectionStore) Status(workspaceID string) (SourceSelectionStatus, error) {
	review, err := store.PrepareSelection(workspaceID, SourceDeferred, nil)
	if err != nil {
		return SourceSelectionStatus{}, err
	}
	defer review.Close()
	return review.CurrentStatus, nil
}

func (store SourceSelectionStore) Select(workspaceID string, folderURLs []string) (SourceSelectionStatus, error) {
	return store.record(workspaceID, SourceSelected, folderURLs)
}
func (store SourceSelectionStore) Defer(workspaceID string) (SourceSelectionStatus, error) {
	return store.record(workspaceID, SourceDeferred, nil)
}
func (store SourceSelectionStore) record(workspaceID, state string, folderURLs []string) (SourceSelectionStatus, error) {
	review, err := store.PrepareSelection(workspaceID, state, folderURLs)
	if err != nil {
		return SourceSelectionStatus{}, err
	}
	defer review.Close()
	return store.CommitSelection(review, review.Digest, nil)
}

func baseSourceSelectionStatus(workspaceID string) SourceSelectionStatus {
	return SourceSelectionStatus{
		SchemaVersion:        1,
		State:                SourceSelectionRequired,
		WorkspaceID:          workspaceID,
		SourceAuthority:      "sharepoint",
		LocalProjection:      "metadata_and_source_pointers_only",
		AuthorizationState:   "not_selected",
		CollectionRuntime:    "claude",
		CollectionState:      "unavailable",
		CodexCollectionState: "unavailable/corporate_policy",
	}
}

func statusFromSourceSelection(record sourceSelection, recordPath string) SourceSelectionStatus {
	status := baseSourceSelectionStatus(record.WorkspaceID)
	status.State = record.State
	status.Version = record.Version
	status.Fingerprint = record.Fingerprint
	status.Pointer = filepath.ToSlash(recordPath)
	status.FolderCount = len(record.FolderURLs)
	if record.State == SourceSelected {
		status.AuthorizationState = "pending_signed_enrollment"
	} else if record.State == SourceRevoked {
		status.AuthorizationState = "revoked_by_owner"
	} else {
		status.AuthorizationState = "deferred_by_owner"
	}
	return status
}

func validateStoredSourceSelection(record sourceSelection, active sourceSelectionActive, workspaceID string) error {
	if record.SchemaVersion != 1 || active.SchemaVersion != 1 || record.WorkspaceID != workspaceID || active.WorkspaceID != workspaceID ||
		(record.State != SourceSelected && record.State != SourceDeferred && record.State != SourceRevoked) || record.Source != "sharepoint" || record.Purpose != "prior_work_retrieval" ||
		record.Version == "" || record.Version != active.Version || record.Fingerprint == "" || record.Fingerprint != active.Fingerprint || record.RecordedAt.IsZero() {
		return errors.New("SharePoint source selection state is invalid")
	}
	canonical, err := canonicalFolderURLs(record.FolderURLs)
	if (record.State == SourceDeferred || record.State == SourceRevoked) && len(record.FolderURLs) == 0 {
		canonical, err = nil, nil
	}
	if err != nil || !equalStrings(canonical, record.FolderURLs) {
		return errors.New("SharePoint source selection pointers are invalid")
	}
	expected, err := sourceSelectionFingerprint(record)
	if err != nil || expected != record.Fingerprint || record.Version != "source-"+record.Fingerprint[:20] {
		return errors.New("SharePoint source selection fingerprint is invalid")
	}
	return nil
}

func sourceSelectionFingerprint(record sourceSelection) (string, error) {
	return fingerprintValue(struct {
		SchemaVersion int      `json:"schema_version"`
		WorkspaceID   string   `json:"workspace_id"`
		State         string   `json:"state"`
		Source        string   `json:"source"`
		Purpose       string   `json:"purpose"`
		FolderURLs    []string `json:"folder_urls"`
	}{
		SchemaVersion: record.SchemaVersion,
		WorkspaceID:   record.WorkspaceID,
		State:         record.State,
		Source:        record.Source,
		Purpose:       record.Purpose,
		FolderURLs:    record.FolderURLs,
	})
}

func validateWorkspaceID(workspaceID string) error {
	if !workspaceIDPattern.MatchString(workspaceID) {
		return errors.New("SharePoint source selection requires the exact initialized workspace ID")
	}
	return nil
}

func canonicalFolderURLs(values []string) ([]string, error) {
	if len(values) == 0 {
		return nil, errors.New("at least one exact SharePoint project folder is required")
	}
	if len(values) > maximumSelectedFolders {
		return nil, fmt.Errorf("SharePoint source selection exceeds %d folders", maximumSelectedFolders)
	}
	seen := map[string]bool{}
	canonical := make([]string, 0, len(values))
	for _, raw := range values {
		if len(raw) == 0 || len(raw) > 4096 || strings.TrimSpace(raw) != raw {
			return nil, errors.New("SharePoint folder pointer is empty, oversized or not canonical")
		}
		parsed, err := url.Parse(raw)
		if err != nil || parsed.Scheme != "https" || parsed.User != nil || parsed.Fragment != "" || parsed.Hostname() == "" || parsed.Port() != "" {
			return nil, errors.New("SharePoint folder pointer must use HTTPS without credentials, fragment or a non-standard port")
		}
		host := strings.ToLower(parsed.Hostname())
		if !strings.HasSuffix(host, ".sharepoint.com") {
			return nil, errors.New("SharePoint folder pointer must use a sharepoint.com origin")
		}
		pathValue, err := normalizeSharePointPath(parsed)
		if err != nil {
			return nil, err
		}
		parsed.Scheme = "https"
		parsed.Host = host
		parsed.Path = pathValue
		parsed.RawPath = ""
		parsed.RawQuery = ""
		parsed.ForceQuery = false
		value := parsed.String()
		if len(value) > 4096 {
			return nil, errors.New("canonical SharePoint folder pointer exceeds the safe limit")
		}
		if seen[value] {
			return nil, errors.New("SharePoint source selection contains a duplicate folder pointer")
		}
		seen[value] = true
		canonical = append(canonical, value)
	}
	sort.Strings(canonical)
	return canonical, nil
}

// normalizeSharePointPath accepts both a library/folder URL and the common
// SharePoint AllItems.aspx view URL. A library root is retained as an owner
// pointer, but SelectedFolders refuses to hand that broad scope to collection
// until a project folder is explicitly narrowed.
func normalizeSharePointPath(parsed *url.URL) (string, error) {
	pathValue := strings.TrimSuffix(parsed.Path, "/")
	viewSuffix := "/Forms/AllItems.aspx"
	if len(pathValue) >= len(viewSuffix) && strings.EqualFold(pathValue[len(pathValue)-len(viewSuffix):], viewSuffix) {
		viewRoot := strings.TrimSuffix(pathValue[:len(pathValue)-len(viewSuffix)], "/")
		pathValue = viewRoot
		if parsed.RawQuery != "" {
			query, err := url.ParseQuery(parsed.RawQuery)
			if err != nil {
				return "", errors.New("SharePoint view link could not be read")
			}
			if len(query["id"]) > 1 {
				return "", errors.New("SharePoint view link contains multiple folder scopes")
			}
			if id := strings.TrimSpace(query.Get("id")); id != "" {
				target := strings.TrimSuffix(id, "/")
				rootSegments, err := validatedSharePointPathSegments(viewRoot)
				if err != nil {
					return "", err
				}
				targetSegments, err := validatedSharePointPathSegments(target)
				if err != nil {
					return "", err
				}
				for index := 0; index < 3; index++ {
					if !strings.EqualFold(rootSegments[index], targetSegments[index]) {
						return "", errors.New("SharePoint view folder must remain inside the displayed site and document library")
					}
				}
				pathValue = target
			}
		}
	} else if parsed.RawQuery != "" {
		return "", errors.New("SharePoint folder pointer may not contain a query string")
	}
	segments, err := validatedSharePointPathSegments(pathValue)
	if err != nil {
		return "", err
	}
	return "/" + strings.Join(segments, "/"), nil
}

func validatedSharePointPathSegments(pathValue string) ([]string, error) {
	segments := strings.Split(strings.Trim(pathValue, "/"), "/")
	if len(segments) < 3 || (strings.ToLower(segments[0]) != "sites" && strings.ToLower(segments[0]) != "teams") {
		return nil, errors.New("SharePoint folder pointer must identify a site/team and document library")
	}
	for _, segment := range segments {
		if segment == "" || segment == "." || segment == ".." {
			return nil, errors.New("SharePoint folder pointer contains an unsafe path segment")
		}
	}
	return segments, nil
}

func isSharePointLibraryRoot(value string) bool {
	parsed, err := url.Parse(value)
	if err != nil {
		return false
	}
	segments := strings.Split(strings.Trim(parsed.EscapedPath(), "/"), "/")
	return len(segments) == 3 && (strings.EqualFold(segments[0], "sites") || strings.EqualFold(segments[0], "teams"))
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

var ErrSourceSelectionChanged = errors.New("SharePoint source selection changed; review the current selection again")

// SourceSelectionReview retains the admitted directory without taking a lock
// while a native client presents the complete current and proposed records.
type SourceSelectionReview struct {
	CurrentStatus SourceSelectionStatus `json:"current_status"`
	Current       *sourceSelection      `json:"current,omitempty"`
	Proposed      sourceSelection       `json:"proposed"`
	Digest        string                `json:"digest"`
	held          *scheduler.HeldDirectory
	anchor        os.FileInfo
	directories   map[string]os.FileInfo
	relative      string
	activeBody    []byte
}

func (review *SourceSelectionReview) Close() error { return review.held.Close() }

func (store SourceSelectionStore) PrepareSelection(workspaceID, state string, folders []string) (*SourceSelectionReview, error) {
	if err := validateWorkspaceID(workspaceID); err != nil {
		return nil, err
	}
	if state != SourceSelected && state != SourceDeferred && state != SourceRevoked {
		return nil, errors.New("invalid SharePoint selection state")
	}
	var err error
	if state == SourceSelected {
		folders, err = canonicalFolderURLs(folders)
		if err != nil {
			return nil, err
		}
	} else if len(folders) != 0 {
		return nil, errors.New("inactive SharePoint selection cannot contain folders")
	}
	custody := store.CustodyRoot
	if custody == "" {
		custody = filepath.Dir(store.Root)
	}
	if !filepath.IsAbs(store.Root) || filepath.Clean(store.Root) != store.Root || store.Root == custody {
		return nil, errors.New("SharePoint source root is invalid")
	}
	held, err := scheduler.HoldDirectory(custody, nil)
	if err != nil {
		return nil, err
	}
	review := &SourceSelectionReview{held: held, directories: map[string]os.FileInfo{}, CurrentStatus: baseSourceSelectionStatus(workspaceID)}
	fail := func(err error) (*SourceSelectionReview, error) { held.Close(); return nil, err }
	review.anchor, err = held.StatDirectory(".")
	if err != nil {
		return fail(err)
	}
	review.relative, err = held.Relative(store.Root)
	if err != nil {
		return fail(err)
	}
	scope := filepath.Join(review.relative, "source-selections", workspaceID)
	directories := []string{}
	current := ""
	for _, segment := range strings.Split(filepath.ToSlash(scope), "/") {
		current = filepath.Join(current, segment)
		directories = append(directories, current)
	}
	directories = append(directories, filepath.Join(scope, "versions"))
	missing := false
	for _, directory := range directories {
		if missing {
			break
		}
		info, e := held.StatDirectory(directory)
		if errors.Is(e, os.ErrNotExist) {
			missing = true
			break
		}
		if e != nil {
			return fail(e)
		}
		review.directories[directory] = info
	}
	if _, exists := review.directories[scope]; exists {
		body, e := held.ReadFile(filepath.Join(scope, "active.json"), MaximumSourceSelectionInputBytes, true)
		if e != nil {
			return fail(e)
		}
		active, e := decodeStrictJSON[sourceSelectionActive](body)
		if e != nil {
			return fail(e)
		}
		if !regexp.MustCompile(`^source-[a-f0-9]{20}$`).MatchString(active.Version) {
			return fail(errors.New("SharePoint selection pointer is invalid"))
		}
		review.activeBody = append([]byte(nil), body...)
		body, e = held.ReadFile(filepath.Join(scope, "versions", active.Version+".json"), maximumSourceSelectionRecordBytes, true)
		if e != nil {
			return fail(e)
		}
		record, e := decodeStrictJSON[sourceSelection](body)
		if e != nil {
			return fail(e)
		}
		if e = validateStoredSourceSelection(record, active, workspaceID); e != nil {
			return fail(e)
		}
		review.Current = &record
		review.CurrentStatus = statusFromSourceSelection(record, filepath.Join("source-selections", workspaceID, "versions", active.Version+".json"))
	}
	review.Proposed = sourceSelection{SchemaVersion: 1, WorkspaceID: workspaceID, State: state, Source: "sharepoint", Purpose: "prior_work_retrieval", FolderURLs: append([]string(nil), folders...)}
	review.Proposed.Fingerprint, err = sourceSelectionFingerprint(review.Proposed)
	if err != nil {
		return fail(err)
	}
	review.Proposed.Version = "source-" + review.Proposed.Fingerprint[:20]
	review.Digest, err = fingerprintValue(review)
	if err != nil {
		return fail(err)
	}
	return review, nil
}

// CommitSelection never grants signed enrollment or external collection. A
// revoked local choice fences SelectedFolders and preserves immutable versions.
func (store SourceSelectionStore) CommitSelection(review *SourceSelectionReview, digest string, guard func() error) (SourceSelectionStatus, error) {
	var status SourceSelectionStatus
	if review == nil || review.held == nil || digest == "" || review.Digest != digest {
		return status, ErrSourceSelectionChanged
	}
	err := review.held.WithPrivateLock(filepath.Join(review.relative, ".sharepoint-source-"+review.Proposed.WorkspaceID+".lock"), func() error {
		if guard != nil {
			if err := guard(); err != nil {
				return err
			}
		}
		currentAnchor, err := scheduler.HoldDirectory(review.held.Path(), nil)
		if err != nil {
			return err
		}
		info, err := currentAnchor.StatDirectory(".")
		currentAnchor.Close()
		if err != nil {
			return err
		}
		if !os.SameFile(info, review.anchor) {
			return ErrSourceSelectionChanged
		}
		for path, before := range review.directories {
			now, e := review.held.StatDirectory(path)
			if e != nil {
				return e
			}
			if !os.SameFile(before, now) {
				return ErrSourceSelectionChanged
			}
		}
		fresh, err := store.PrepareSelection(review.Proposed.WorkspaceID, review.Proposed.State, review.Proposed.FolderURLs)
		if err != nil {
			return err
		}
		defer fresh.Close()
		if fresh.Digest != digest {
			return ErrSourceSelectionChanged
		}
		record := review.Proposed
		if review.Current != nil && review.Current.Fingerprint == record.Fingerprint {
			status = review.CurrentStatus
			return nil
		}
		record.RecordedAt = store.now()
		scope := filepath.Join(review.relative, "source-selections", record.WorkspaceID)
		if err = review.held.EnsureDirectory(filepath.Join(scope, "versions")); err != nil {
			return err
		}
		versionPath := filepath.Join(scope, "versions", record.Version+".json")
		body, err := review.held.ReadFile(versionPath, maximumSourceSelectionRecordBytes, true)
		if err == nil {
			existing, e := decodeStrictJSON[sourceSelection](body)
			active := sourceSelectionActive{SchemaVersion: 1, WorkspaceID: record.WorkspaceID, Version: record.Version, Fingerprint: record.Fingerprint}
			if e != nil {
				return e
			}
			if e = validateStoredSourceSelection(existing, active, record.WorkspaceID); e != nil {
				return e
			}
			record = existing
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		} else {
			body, err = marshalPrivate(record)
			if err != nil {
				return err
			}
			if err = review.held.PublishNewFile(versionPath, body); err != nil {
				return err
			}
		}
		active := sourceSelectionActive{SchemaVersion: 1, WorkspaceID: record.WorkspaceID, Version: record.Version, Fingerprint: record.Fingerprint}
		body, err = marshalPrivate(active)
		if err != nil {
			return err
		}
		if guard != nil {
			if err = guard(); err != nil {
				return err
			}
		}
		if err = review.held.UpdatePrivateFile(filepath.Join(scope, "active.json"), MaximumSourceSelectionInputBytes, func(current []byte) ([]byte, error) {
			if !bytes.Equal(current, review.activeBody) {
				return nil, ErrSourceSelectionChanged
			}
			return body, nil
		}); err != nil {
			return err
		}
		status = statusFromSourceSelection(record, filepath.Join("source-selections", record.WorkspaceID, "versions", record.Version+".json"))
		return nil
	})
	return status, err
}
