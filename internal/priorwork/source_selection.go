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
)

const (
	MaximumSourceSelectionInputBytes = 128 << 10
	maximumSelectedFolders           = 32
	SourceSelectionRequired          = "selection_required"
	SourceSelected                   = "selected"
	SourceDeferred                   = "deferred"
	SourceSelectionUnavailable       = "unavailable"
)

var (
	ErrSourceSelectionTooLarge = errors.New("SharePoint source selection exceeds the safe input limit")
	workspaceIDPattern         = regexp.MustCompile(`^[a-f0-9]{32}$`)
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
	Root  string
	clock func() time.Time
}

// SelectedFolders returns the exact reviewed pointers for a workspace. It is
// intentionally callable only by a bounded local ingestion path; Session Start
// continues to expose counts and fingerprints, never these URLs.
func (store SourceSelectionStore) SelectedFolders(workspaceID string) ([]string, error) {
	if err := validateWorkspaceID(workspaceID); err != nil {
		return nil, err
	}
	status, err := store.Status(workspaceID)
	if err != nil {
		return nil, err
	}
	if status.State != SourceSelected {
		return nil, errors.New("SharePoint source folders are not selected")
	}
	active, err := loadJSONAt[sourceSelectionActive](store.Root, filepath.Join("source-selections", workspaceID, "active.json"))
	if err != nil {
		return nil, err
	}
	record, err := loadJSONAt[sourceSelection](store.Root, filepath.Join("source-selections", workspaceID, "versions", active.Version+".json"))
	if err != nil {
		return nil, err
	}
	if err := validateStoredSourceSelection(record, active, workspaceID); err != nil {
		return nil, err
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
	if err := validateWorkspaceID(workspaceID); err != nil {
		return SourceSelectionStatus{}, err
	}
	base := baseSourceSelectionStatus(workspaceID)
	if strings.TrimSpace(store.Root) == "" {
		return SourceSelectionStatus{}, errors.New("prior-work root is required")
	}
	if _, err := os.Lstat(store.Root); errors.Is(err, os.ErrNotExist) {
		return base, nil
	} else if err != nil {
		return SourceSelectionStatus{}, err
	}
	activePath := filepath.Join("source-selections", workspaceID, "active.json")
	active, err := loadJSONAt[sourceSelectionActive](store.Root, activePath)
	if errors.Is(err, os.ErrNotExist) {
		return base, nil
	}
	if err != nil {
		return SourceSelectionStatus{}, err
	}
	recordPath := filepath.Join("source-selections", workspaceID, "versions", active.Version+".json")
	record, err := loadJSONAt[sourceSelection](store.Root, recordPath)
	if err != nil {
		return SourceSelectionStatus{}, err
	}
	if err := validateStoredSourceSelection(record, active, workspaceID); err != nil {
		return SourceSelectionStatus{}, err
	}
	return statusFromSourceSelection(record, recordPath), nil
}

func (store SourceSelectionStore) Select(workspaceID string, folderURLs []string) (SourceSelectionStatus, error) {
	canonical, err := canonicalFolderURLs(folderURLs)
	if err != nil {
		return SourceSelectionStatus{}, err
	}
	return store.record(workspaceID, SourceSelected, canonical)
}

func (store SourceSelectionStore) Defer(workspaceID string) (SourceSelectionStatus, error) {
	return store.record(workspaceID, SourceDeferred, nil)
}

func (store SourceSelectionStore) record(workspaceID, state string, folderURLs []string) (SourceSelectionStatus, error) {
	if err := validateWorkspaceID(workspaceID); err != nil {
		return SourceSelectionStatus{}, err
	}
	if state != SourceSelected && state != SourceDeferred {
		return SourceSelectionStatus{}, errors.New("invalid SharePoint source selection state")
	}
	if state == SourceSelected && len(folderURLs) == 0 {
		return SourceSelectionStatus{}, errors.New("at least one exact SharePoint project folder is required")
	}
	if state == SourceDeferred && len(folderURLs) != 0 {
		return SourceSelectionStatus{}, errors.New("a deferred SharePoint source choice cannot contain folders")
	}
	record := sourceSelection{
		SchemaVersion: 1,
		WorkspaceID:   workspaceID,
		State:         state,
		Source:        "sharepoint",
		Purpose:       "prior_work_retrieval",
		FolderURLs:    append([]string(nil), folderURLs...),
	}
	fingerprint, err := sourceSelectionFingerprint(record)
	if err != nil {
		return SourceSelectionStatus{}, err
	}
	record.Fingerprint = fingerprint
	record.Version = "source-" + fingerprint[:20]
	record.RecordedAt = store.now()

	if err := (Store{Root: store.Root}).prepareRoot(); err != nil {
		return SourceSelectionStatus{}, err
	}
	root, err := openAnchoredRoot(store.Root)
	if err != nil {
		return SourceSelectionStatus{}, err
	}
	defer root.Close()
	for _, directory := range []string{
		"source-selections",
		filepath.Join("source-selections", workspaceID),
		filepath.Join("source-selections", workspaceID, "versions"),
	} {
		if err := ensurePrivateDirectoryAt(root, directory); err != nil {
			return SourceSelectionStatus{}, err
		}
	}
	recordPath := filepath.Join("source-selections", workspaceID, "versions", record.Version+".json")
	if existing, err := loadJSONRoot[sourceSelection](root, recordPath); err == nil {
		if existing.Fingerprint != record.Fingerprint || existing.WorkspaceID != record.WorkspaceID || existing.State != record.State || !equalStrings(existing.FolderURLs, record.FolderURLs) {
			return SourceSelectionStatus{}, errors.New("immutable SharePoint source selection collision")
		}
		record = existing
	} else if !errors.Is(err, os.ErrNotExist) {
		return SourceSelectionStatus{}, err
	} else if err := writePrivateExclusiveAt(root, recordPath, record); err != nil {
		return SourceSelectionStatus{}, err
	}
	active := sourceSelectionActive{SchemaVersion: 1, WorkspaceID: workspaceID, Version: record.Version, Fingerprint: record.Fingerprint}
	if err := atomicWriteAt(store.Root, filepath.Join("source-selections", workspaceID, "active.json"), active); err != nil {
		return SourceSelectionStatus{}, err
	}
	return statusFromSourceSelection(record, recordPath), nil
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
	} else {
		status.AuthorizationState = "deferred_by_owner"
	}
	return status
}

func validateStoredSourceSelection(record sourceSelection, active sourceSelectionActive, workspaceID string) error {
	if record.SchemaVersion != 1 || active.SchemaVersion != 1 || record.WorkspaceID != workspaceID || active.WorkspaceID != workspaceID ||
		(record.State != SourceSelected && record.State != SourceDeferred) || record.Source != "sharepoint" || record.Purpose != "prior_work_retrieval" ||
		record.Version == "" || record.Version != active.Version || record.Fingerprint == "" || record.Fingerprint != active.Fingerprint || record.RecordedAt.IsZero() {
		return errors.New("SharePoint source selection state is invalid")
	}
	canonical, err := canonicalFolderURLs(record.FolderURLs)
	if record.State == SourceDeferred && len(record.FolderURLs) == 0 {
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
		if err != nil || parsed.Scheme != "https" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Hostname() == "" || parsed.Port() != "" {
			return nil, errors.New("SharePoint folder pointer must be a canonical HTTPS URL without credentials, query or fragment")
		}
		host := strings.ToLower(parsed.Hostname())
		if !strings.HasSuffix(host, ".sharepoint.com") {
			return nil, errors.New("SharePoint folder pointer must use a sharepoint.com origin")
		}
		segments := strings.Split(strings.Trim(parsed.EscapedPath(), "/"), "/")
		if len(segments) < 4 || (strings.ToLower(segments[0]) != "sites" && strings.ToLower(segments[0]) != "teams") {
			return nil, errors.New("SharePoint folder pointer must identify an exact project folder below a site or team library")
		}
		parsed.Scheme = "https"
		parsed.Host = host
		parsed.Path = strings.TrimSuffix(parsed.Path, "/")
		parsed.RawPath = strings.TrimSuffix(parsed.RawPath, "/")
		value := parsed.String()
		if seen[value] {
			return nil, errors.New("SharePoint source selection contains a duplicate folder pointer")
		}
		seen[value] = true
		canonical = append(canonical, value)
	}
	sort.Strings(canonical)
	return canonical, nil
}

func ensurePrivateDirectoryAt(root *os.Root, relative string) error {
	if err := root.Mkdir(relative, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	info, err := root.Lstat(relative)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return errors.New("SharePoint source selection path must be a real directory")
	}
	if info.Mode().Perm()&0o077 != 0 {
		if err := root.Chmod(relative, 0o700); err != nil {
			return err
		}
	}
	return nil
}

func writePrivateExclusiveAt(root *os.Root, relative string, value any) error {
	body, err := marshalPrivate(value)
	if err != nil {
		return err
	}
	file, err := root.OpenFile(relative, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(file, bytes.NewReader(body)); err != nil {
		_ = file.Close()
		_ = root.Remove(relative)
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		_ = root.Remove(relative)
		return err
	}
	if err := file.Close(); err != nil {
		_ = root.Remove(relative)
		return err
	}
	return syncRootDirectory(root, filepath.Dir(relative))
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
