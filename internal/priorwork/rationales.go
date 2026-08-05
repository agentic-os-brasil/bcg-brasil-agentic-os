package priorwork

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	MaximumRationaleBatchBytes = 8 << 20
	MaximumRationales          = 100
	MaximumRationaleBytes      = 64 << 10
)

// RationaleBatch is the bounded hand-off from a qualified Claude SharePoint
// collector to a local Maestro workspace. It contains derived reasoning, not
// copied source bodies. The signed metadata snapshot and receipt bind each
// rationale to an authoritative SharePoint item.
type RationaleBatch struct {
	SchemaVersion              int           `json:"schema_version"`
	WorkspaceID                string        `json:"workspace_id"`
	SourceSelectionFingerprint string        `json:"source_selection_fingerprint"`
	Snapshot                   Snapshot      `json:"snapshot"`
	Receipt                    ImportReceipt `json:"receipt"`
	Rationales                 []Rationale   `json:"rationales"`
}

// Rationale is a concise, derived synthesis. SourceURL is retained so a user
// can re-open the authoritative SharePoint item; raw document bodies are not
// accepted here.
type Rationale struct {
	ItemRef       string    `json:"item_ref"`
	Root          RootRef   `json:"root"`
	SourceURL     string    `json:"source_url"`
	Name          string    `json:"name"`
	ModifiedAt    time.Time `json:"modified_at"`
	ContentDigest string    `json:"content_digest"`
	Text          string    `json:"text"`
}

type RationaleReport struct {
	SchemaVersion   int      `json:"schema_version"`
	State           string   `json:"state"`
	WorkspacePath   string   `json:"workspace_path"`
	RationaleCount  int      `json:"rationale_count"`
	Priority        string   `json:"priority"`
	SourceAuthority string   `json:"source_authority"`
	LocalProjection string   `json:"local_projection"`
	Items           []string `json:"items"`
}

func ParseRationaleBatch(reader io.Reader) (RationaleBatch, error) {
	body, err := io.ReadAll(io.LimitReader(reader, MaximumRationaleBatchBytes+1))
	if err != nil {
		return RationaleBatch{}, err
	}
	if len(body) > MaximumRationaleBatchBytes {
		return RationaleBatch{}, errors.New("SharePoint rationale batch exceeds the safe input limit")
	}
	if err := rejectDuplicateJSONKeys(body); err != nil {
		return RationaleBatch{}, fmt.Errorf("decode SharePoint rationale batch: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var batch RationaleBatch
	if err := decoder.Decode(&batch); err != nil {
		return RationaleBatch{}, fmt.Errorf("decode SharePoint rationale batch: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return RationaleBatch{}, errors.New("SharePoint rationale batch contains multiple JSON values")
		}
		return RationaleBatch{}, err
	}
	return batch, nil
}

// ValidateRationaleBatch verifies syntax and the relationship between the
// derived items and the signed metadata snapshot. Receipt authentication and
// enrollment binding are performed by MaterializeRationales.
func ValidateRationaleBatch(batch RationaleBatch) error {
	if batch.SchemaVersion != 1 || !workspaceIDPattern.MatchString(batch.WorkspaceID) || !isDigest(batch.SourceSelectionFingerprint) {
		return errors.New("rationale batch requires schema 1, workspace ID and selection fingerprint")
	}
	if err := ValidateSnapshot(batch.Snapshot); err != nil {
		return err
	}
	if err := ValidateImportReceipt(batch.Receipt, batch.Snapshot); err != nil {
		return err
	}
	if len(batch.Rationales) == 0 || len(batch.Rationales) > MaximumRationales {
		return fmt.Errorf("rationale batch must contain between 1 and %d items", MaximumRationales)
	}
	items := make(map[string]Item, len(batch.Snapshot.Items))
	for _, item := range batch.Snapshot.Items {
		items[item.key()] = item
	}
	seen := make(map[string]bool, len(batch.Rationales))
	for _, rationale := range batch.Rationales {
		rationaleKey := rationale.Root.key() + "\x00" + rationale.ItemRef
		if rationale.ItemRef == "" || seen[rationaleKey] {
			return errors.New("rationale batch contains a duplicate or empty item reference")
		}
		seen[rationaleKey] = true
		item, ok := items[rationale.Root.key()+"\x00"+rationale.ItemRef]
		if !ok || item.Kind != "file" {
			return errors.New("rationale item is not present in the signed SharePoint snapshot")
		}
		if rationale.SourceURL != item.SourceURL || rationale.Name != item.Name || !rationale.ModifiedAt.Equal(item.ModifiedAt) || !isDigest(rationale.ContentDigest) {
			return errors.New("rationale provenance does not match the signed SharePoint item")
		}
		if strings.TrimSpace(rationale.Text) == "" || len([]byte(rationale.Text)) > MaximumRationaleBytes || strings.ContainsRune(rationale.Text, '\x00') {
			return errors.New("rationale text is empty, oversized or contains invalid bytes")
		}
	}
	return nil
}

// MaterializeRationales validates the signed adapter hand-off and atomically
// writes only derived Markdown into the workspace. Re-running the same batch
// is idempotent; an item with a different digest replaces its prior rationale.
func MaterializeRationales(workspacePath string, batch RationaleBatch, folderURLs []string, enrollment Enrollment) (RationaleReport, error) {
	if err := ValidateRationaleBatch(batch); err != nil {
		return RationaleReport{}, err
	}
	if !time.Now().UTC().Before(enrollment.AuthorizationExpiresAt) || batch.Receipt.EmittedAt.After(enrollment.AuthorizationExpiresAt) {
		return RationaleReport{}, errors.New("prior-work enrollment authorization has expired")
	}
	if batch.Snapshot.TenantRef != enrollment.TenantRef || !rootsEqual(batch.Snapshot.Roots, enrollment.Roots) {
		return RationaleReport{}, errors.New("snapshot tenant or roots do not match enrollment")
	}
	if err := VerifyImportReceipt(batch.Receipt, batch.Snapshot, enrollment); err != nil {
		return RationaleReport{}, err
	}
	selectionFingerprint, err := rationaleSelectionFingerprint(batch.WorkspaceID, folderURLs)
	if err != nil {
		return RationaleReport{}, err
	}
	if batch.SourceSelectionFingerprint != selectionFingerprint {
		return RationaleReport{}, errors.New("rationale batch does not bind the selected SharePoint folders")
	}
	if err := validateRationaleFolders(batch.Rationales, folderURLs); err != nil {
		return RationaleReport{}, err
	}
	workspacePath, err = filepath.Abs(workspacePath)
	if err != nil {
		return RationaleReport{}, err
	}
	manifestPath := filepath.Join(workspacePath, ".bcgos", "workspace.json")
	if info, err := os.Stat(manifestPath); err != nil || info.IsDir() {
		return RationaleReport{}, errors.New("rationale materialization requires an initialized Maestro workspace")
	}
	manifestBody, err := os.ReadFile(manifestPath)
	if err != nil {
		return RationaleReport{}, err
	}
	var manifest struct {
		SchemaVersion int    `json:"schema_version"`
		WorkspaceID   string `json:"workspace_id"`
	}
	decoder := json.NewDecoder(bytes.NewReader(manifestBody))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil || manifest.SchemaVersion != 1 || manifest.WorkspaceID != batch.WorkspaceID {
		return RationaleReport{}, errors.New("rationale batch does not bind the initialized workspace manifest")
	}
	target := filepath.Join(workspacePath, "brain", "knowledge", "sharepoint-rationales")
	if err := ensureWorkspaceDirectory(workspacePath, filepath.Join("brain", "knowledge", "sharepoint-rationales")); err != nil {
		return RationaleReport{}, err
	}
	sorted := append([]Rationale(nil), batch.Rationales...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].ModifiedAt.Equal(sorted[j].ModifiedAt) {
			return sorted[i].ItemRef < sorted[j].ItemRef
		}
		return sorted[i].ModifiedAt.After(sorted[j].ModifiedAt)
	})
	items := make([]string, 0, len(sorted))
	for rank, rationale := range sorted {
		id := rationaleID(rationale.ItemRef)
		name := fmt.Sprintf("%02d-%s.md", rank+1, id)
		path := filepath.Join(target, name)
		body := renderRationale(rank+1, rationale)
		if err := atomicWriteWorkspaceFile(path, body); err != nil {
			return RationaleReport{}, err
		}
		items = append(items, name)
	}
	index := renderRationaleIndex(batch, sorted, items)
	if err := atomicWriteWorkspaceFile(filepath.Join(target, "index.md"), index); err != nil {
		return RationaleReport{}, err
	}
	return RationaleReport{
		SchemaVersion: 1, State: "materialized", WorkspacePath: workspacePath,
		RationaleCount: len(sorted), Priority: "source_modified_descending",
		SourceAuthority: "sharepoint", LocalProjection: "derived_rationales_with_source_pointers", Items: items,
	}, nil
}

func rationaleSelectionFingerprint(workspaceID string, folderURLs []string) (string, error) {
	canonical, err := canonicalFolderURLs(folderURLs)
	if err != nil {
		return "", err
	}
	return sourceSelectionFingerprint(sourceSelection{
		SchemaVersion: 1,
		WorkspaceID:   workspaceID,
		State:         SourceSelected,
		Source:        "sharepoint",
		Purpose:       "prior_work_retrieval",
		FolderURLs:    canonical,
	})
}

func validateRationaleFolders(rationales []Rationale, folders []string) error {
	if len(folders) == 0 {
		return errors.New("rationale materialization requires at least one selected SharePoint folder")
	}
	canonical, err := canonicalFolderURLs(folders)
	if err != nil {
		return err
	}
	for _, rationale := range rationales {
		if !sourceURLUnderFolder(rationale.SourceURL, canonical) {
			return errors.New("rationale source is outside the selected SharePoint folders")
		}
	}
	return nil
}

func sourceURLUnderFolder(raw string, folders []string) bool {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return false
	}
	origin := strings.ToLower(parsed.Scheme + "://" + parsed.Hostname())
	path := strings.TrimSuffix(parsed.Path, "/")
	for _, folder := range folders {
		base, _ := url.Parse(folder)
		if strings.ToLower(base.Scheme+"://"+base.Hostname()) == origin && (path == strings.TrimSuffix(base.Path, "/") || strings.HasPrefix(path, strings.TrimSuffix(base.Path, "/")+"/")) {
			return true
		}
	}
	return false
}

func isDigest(value string) bool {
	if len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func rationaleID(itemRef string) string {
	digest := sha256.Sum256([]byte("maestro-rationale-v1\x00" + itemRef))
	return fmt.Sprintf("%x", digest[:8])
}

func renderRationale(rank int, rationale Rationale) string {
	return fmt.Sprintf("---\nschema_version: 1\nrank: %d\nsource: sharepoint\nsource_item_ref: %s\nsource_root: %s / %s / %s\nsource_url: %s\nsource_modified_at: %s\ncontent_digest: %s\nauthority: sharepoint\nprojection: derived_rationale\n---\n\n# %s\n\n%s\n", rank, yamlScalar(rationale.ItemRef), yamlScalar(rationale.Root.SiteRef), yamlScalar(rationale.Root.DriveRef), yamlScalar(rationale.Root.FolderRef), yamlScalar(rationale.SourceURL), rationale.ModifiedAt.UTC().Format(time.RFC3339), rationale.ContentDigest, rationale.Name, strings.TrimSpace(rationale.Text))
}

func renderRationaleIndex(batch RationaleBatch, rationales []Rationale, files []string) string {
	var builder strings.Builder
	builder.WriteString("# Racionais derivados do SharePoint\n\n")
	builder.WriteString("Esta é uma projeção interna, derivada de materiais autorizados e priorizada pela data de modificação da fonte. O SharePoint continua sendo a autoridade; abra o ponteiro de cada item para consultar a versão vigente. Nenhum corpo bruto foi copiado para este workspace.\n\n")
	fmt.Fprintf(&builder, "- Seleção: `%s`\n- Snapshot: `%s`\n- Atualizado em: `%s`\n- Ordem: materiais mais recentes primeiro; desempate por `item_ref`\n\n", batch.SourceSelectionFingerprint, batch.Receipt.SnapshotDigest, batch.Snapshot.GeneratedAt.UTC().Format(time.RFC3339))
	for i, rationale := range rationales {
		fmt.Fprintf(&builder, "%d. [%s](%s) — modificado em %s\n", i+1, rationale.Name, files[i], rationale.ModifiedAt.UTC().Format(time.RFC3339))
	}
	return builder.String()
}

func yamlScalar(value string) string {
	return `"` + strings.ReplaceAll(strings.ReplaceAll(value, `\`, `\\`), `"`, `\"`) + `"`
}

func ensureWorkspaceDirectory(workspacePath, relative string) error {
	rootInfo, err := os.Lstat(workspacePath)
	if err != nil {
		return err
	}
	if rootInfo.Mode()&os.ModeSymlink != 0 || !rootInfo.IsDir() {
		return errors.New("rationale workspace path must be a real directory")
	}
	current := workspacePath
	for _, part := range strings.Split(filepath.Clean(relative), string(filepath.Separator)) {
		if part == "" || part == "." {
			continue
		}
		current = filepath.Join(current, part)
		info, statErr := os.Lstat(current)
		if errors.Is(statErr, os.ErrNotExist) {
			if err := os.Mkdir(current, 0o700); err != nil {
				return err
			}
			continue
		}
		if statErr != nil {
			return statErr
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return errors.New("rationale workspace path must not traverse symlinks")
		}
	}
	return nil
}

func ensureRealDirectory(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return errors.New("rationale workspace path must be a real directory")
	}
	return nil
}

func atomicWriteWorkspaceFile(path, body string) error {
	if err := ensureRealDirectory(filepath.Dir(path)); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".rationale-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := io.WriteString(tmp, body); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}
