// Package workspaceimport implements the bounded, transactional import path for
// external workspace sources. It is deliberately separate from document
// ingestion: workspace migration moves explicitly approved files, while
// document conversion remains owned by internal/ingest.
package workspaceimport

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const SchemaVersion = 1

const (
	ClassificationMaestroNative = "maestro_native"
	ClassificationMaestroLegacy = "maestro_legacy"
	ClassificationKowalski      = "kowalski"
	ClassificationForeign       = "foreign"
	ClassificationUnsupported   = "unsupported"
)

const (
	ActionCopy       = "copy"
	ActionQuarantine = "quarantine"
	ActionExclude    = "exclude"
)

const (
	PlanStatePlanned    = "planned"
	PlanStateApproved   = "approved"
	PlanStateExecuted   = "executed"
	PlanStateRolledBack = "rolled_back"
)

const (
	ConfirmImport   = "IMPORT"
	ConfirmRollback = "ROLLBACK"
)

type Limits struct {
	MaxEntries    int   `json:"max_entries"`
	MaxDepth      int   `json:"max_depth"`
	MaxFileBytes  int64 `json:"max_file_bytes"`
	MaxTotalBytes int64 `json:"max_total_bytes"`
}

func DefaultLimits() Limits {
	return Limits{MaxEntries: 4096, MaxDepth: 32, MaxFileBytes: 50 << 20, MaxTotalBytes: 250 << 20}
}

type Inspection struct {
	SchemaVersion        int              `json:"schema_version"`
	State                string           `json:"state"`
	Classification       string           `json:"classification"`
	ClassificationReason string           `json:"classification_reason"`
	EntryCount           int              `json:"entry_count"`
	FileCount            int              `json:"file_count"`
	DirectoryCount       int              `json:"directory_count"`
	SymlinkCount         int              `json:"symlink_count"`
	TotalBytes           int64            `json:"total_bytes"`
	Entries              []InventoryEntry `json:"entries"`
	Warnings             []string         `json:"warnings,omitempty"`
	Limits               Limits           `json:"limits"`
	ReadOnly             bool             `json:"read_only"`
	Bounded              bool             `json:"bounded"`
}

type InventoryEntry struct {
	RelativePath string `json:"path"`
	Kind         string `json:"kind"`
	Size         int64  `json:"size,omitempty"`
	Mode         uint32 `json:"mode,omitempty"`
	ModifiedUnix int64  `json:"modified_unix,omitempty"`
	Unsafe       bool   `json:"unsafe,omitempty"`
}

type Exclusion struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

type Conflict struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

type PlanEntry struct {
	SourcePath      string `json:"source_path"`
	DestinationPath string `json:"destination_path"`
	Action          string `json:"action"`
	Availability    string `json:"availability"`
	Reason          string `json:"reason,omitempty"`
	Size            int64  `json:"size,omitempty"`
	Mode            uint32 `json:"mode,omitempty"`
	ModifiedUnix    int64  `json:"modified_unix,omitempty"`
	ContentDigest   string `json:"content_digest"`
}

type Plan struct {
	SchemaVersion        int         `json:"schema_version"`
	PlanID               string      `json:"plan_id"`
	Origin               string      `json:"origin"`
	Destination          string      `json:"destination"`
	Classification       string      `json:"classification"`
	ClassificationReason string      `json:"classification_reason"`
	CreatedAt            string      `json:"created_at"`
	Entries              []PlanEntry `json:"entries"`
	Exclusions           []Exclusion `json:"exclusions,omitempty"`
	Conflicts            []Conflict  `json:"conflicts,omitempty"`
	PlanDigest           string      `json:"plan_digest"`
	State                string      `json:"state"`
	Limits               Limits      `json:"limits"`
}

type Approval struct {
	SchemaVersion int    `json:"schema_version"`
	PlanID        string `json:"plan_id"`
	PlanDigest    string `json:"plan_digest"`
	ApprovedBy    string `json:"approved_by"`
	ApprovedAt    string `json:"approved_at"`
	Confirmation  string `json:"confirmation"`
}

type Receipt struct {
	SchemaVersion   int               `json:"schema_version"`
	RunID           string            `json:"run_id"`
	PlanID          string            `json:"plan_id"`
	PlanDigest      string            `json:"plan_digest"`
	State           string            `json:"state"`
	RecordedAt      string            `json:"recorded_at"`
	Copied          []string          `json:"copied,omitempty"`
	Quarantined     []string          `json:"quarantined,omitempty"`
	Excluded        []string          `json:"excluded,omitempty"`
	RollbackPaths   []string          `json:"rollback_paths,omitempty"`
	RollbackDigests map[string]string `json:"rollback_digests,omitempty"`
	Error           string            `json:"error,omitempty"`
}

type JournalEntry struct {
	SourcePath      string `json:"source_path"`
	StagePath       string `json:"stage_path"`
	DestinationPath string `json:"destination_path"`
	Action          string `json:"action"`
	ContentDigest   string `json:"content_digest"`
}

type Journal struct {
	SchemaVersion int            `json:"schema_version"`
	RunID         string         `json:"run_id"`
	PlanID        string         `json:"plan_id"`
	PlanDigest    string         `json:"plan_digest"`
	State         string         `json:"state"`
	RecordedAt    string         `json:"recorded_at"`
	Entries       []JournalEntry `json:"entries"`
	Committed     []string       `json:"committed,omitempty"`
}

// Test seams remain nil in production. They make the filesystem and lease
// boundaries observable without injecting real workspace data.
var secureCommitParentHook func(string)
var executeLockHook func()

func normalizeLimits(limits Limits) (Limits, error) {
	defaults := DefaultLimits()
	if limits.MaxEntries == 0 {
		limits.MaxEntries = defaults.MaxEntries
	}
	if limits.MaxDepth == 0 {
		limits.MaxDepth = defaults.MaxDepth
	}
	if limits.MaxFileBytes == 0 {
		limits.MaxFileBytes = defaults.MaxFileBytes
	}
	if limits.MaxTotalBytes == 0 {
		limits.MaxTotalBytes = defaults.MaxTotalBytes
	}
	if limits.MaxEntries < 1 || limits.MaxDepth < 1 || limits.MaxFileBytes < 1 || limits.MaxTotalBytes < 1 {
		return Limits{}, errors.New("workspace import limits must be positive")
	}
	return limits, nil
}

func cleanDirectory(path, label string, mustExist bool) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("%s path is required", label)
	}
	abs, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", label, err)
	}
	info, err := os.Lstat(abs)
	if errors.Is(err, os.ErrNotExist) && !mustExist {
		return abs, nil
	}
	if err != nil {
		return "", fmt.Errorf("inspect %s: %w", label, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return "", fmt.Errorf("%s must be a non-symlink directory", label)
	}
	return abs, nil
}

func pathSafe(root, relative string) (string, error) {
	if relative == "" || filepath.IsAbs(relative) || strings.Contains(relative, `\`) {
		return "", errors.New("relative path is unsafe")
	}
	clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(relative)))
	if clean != relative || clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", errors.New("relative path escapes root")
	}
	return filepath.Join(root, filepath.FromSlash(clean)), nil
}

func pathWithin(root, candidate string) bool {
	rel, err := filepath.Rel(filepath.Clean(root), filepath.Clean(candidate))
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != "."
}

func containsMarker(root string, markers ...string) bool {
	for _, marker := range markers {
		if _, err := os.Lstat(filepath.Join(root, marker)); err == nil {
			return true
		}
	}
	return false
}

func classify(root string) (string, string) {
	if containsMarker(root, ".bcgos", "workspace.json") {
		return ClassificationMaestroNative, "initialized Maestro workspace metadata is present; native workspaces are not migrated"
	}
	if containsMarker(root, ".maestro", ".maestro-workspace", "maestro.json", "maestro.yaml") {
		return ClassificationMaestroLegacy, "legacy Maestro marker is present"
	}
	if containsMarker(root, ".kowalski", "kowalski.json", "kowalski.yaml") {
		return ClassificationKowalski, "Kowalski marker is present"
	}
	return ClassificationForeign, "no recognized Maestro or Kowalski marker is present"
}

func inspectDirectory(root string, limits Limits) (Inspection, error) {
	classification, reason := classify(root)
	result := Inspection{SchemaVersion: SchemaVersion, State: "ready", Classification: classification, ClassificationReason: reason, Limits: limits, ReadOnly: true, Bounded: true, Entries: []InventoryEntry{}}
	var walk func(string, string, int) error
	walk = func(current, relative string, depth int) error {
		if depth > limits.MaxDepth {
			result.State = "bounded"
			result.Warnings = append(result.Warnings, "maximum inventory depth reached")
			return nil
		}
		entries, err := os.ReadDir(current)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if len(result.Entries) >= limits.MaxEntries {
				result.State = "bounded"
				result.Warnings = append(result.Warnings, "maximum inventory entry count reached")
				return nil
			}
			entryRel := entry.Name()
			if relative != "" {
				entryRel = filepath.ToSlash(filepath.Join(relative, entry.Name()))
			}
			info, err := os.Lstat(filepath.Join(current, entry.Name()))
			if err != nil {
				return fmt.Errorf("inspect %s: %w", entryRel, err)
			}
			item := InventoryEntry{RelativePath: entryRel, Mode: uint32(info.Mode()), ModifiedUnix: info.ModTime().UnixNano()}
			switch {
			case info.Mode()&os.ModeSymlink != 0:
				item.Kind, item.Unsafe, result.SymlinkCount = "symlink", true, result.SymlinkCount+1
				result.Warnings = append(result.Warnings, "symlink excluded: "+entryRel)
			case info.IsDir():
				item.Kind = "directory"
				result.DirectoryCount++
			case info.Mode().IsRegular():
				item.Kind, item.Size = "file", info.Size()
				result.FileCount++
				result.TotalBytes += info.Size()
				if info.Size() > limits.MaxFileBytes {
					result.Warnings = append(result.Warnings, "file exceeds limit: "+entryRel)
				}
				if result.TotalBytes > limits.MaxTotalBytes {
					result.State = "bounded"
					result.Warnings = append(result.Warnings, "maximum inventory byte count reached")
				}
			default:
				item.Kind, item.Unsafe = "unsupported", true
				result.Warnings = append(result.Warnings, "unsupported filesystem entry: "+entryRel)
			}
			result.Entries = append(result.Entries, item)
			if item.Kind == "directory" && !shouldSkipDirectory(entryRel) && result.State != "bounded" {
				if err := walk(filepath.Join(current, entry.Name()), entryRel, depth+1); err != nil {
					return err
				}
			}
			if result.State == "bounded" {
				return nil
			}
		}
		return nil
	}
	if err := walk(root, "", 0); err != nil {
		return result, err
	}
	sort.Slice(result.Entries, func(i, j int) bool { return result.Entries[i].RelativePath < result.Entries[j].RelativePath })
	result.EntryCount = len(result.Entries)
	if classification == ClassificationMaestroNative {
		result.State = "blocked"
	}
	return result, nil
}

func shouldSkipDirectory(path string) bool {
	base := filepath.Base(path)
	return base == ".git" || base == ".bcgos" || base == ".claude" || base == ".maestro" || base == ".kowalski"
}

// Inspect performs a read-only metadata inventory. It never opens a regular
// source file and never follows symlinks.
func Inspect(source string, limits Limits) (Inspection, error) {
	limits, err := normalizeLimits(limits)
	if err != nil {
		return Inspection{}, err
	}
	root, err := cleanDirectory(source, "source", true)
	if err != nil {
		return Inspection{SchemaVersion: SchemaVersion, State: "unsupported", Classification: ClassificationUnsupported, ClassificationReason: err.Error(), Entries: []InventoryEntry{}, ReadOnly: true, Bounded: true, Limits: limits}, err
	}
	return inspectDirectory(root, limits)
}

func canonicalPlanBytes(plan Plan) ([]byte, error) {
	unsigned := struct {
		SchemaVersion        int         `json:"schema_version"`
		PlanID               string      `json:"plan_id"`
		Origin               string      `json:"origin"`
		Destination          string      `json:"destination"`
		Classification       string      `json:"classification"`
		ClassificationReason string      `json:"classification_reason"`
		CreatedAt            string      `json:"created_at"`
		Entries              []PlanEntry `json:"entries"`
		Exclusions           []Exclusion `json:"exclusions,omitempty"`
		Conflicts            []Conflict  `json:"conflicts,omitempty"`
		Limits               Limits      `json:"limits"`
	}{plan.SchemaVersion, plan.PlanID, plan.Origin, plan.Destination, plan.Classification, plan.ClassificationReason, plan.CreatedAt, plan.Entries, plan.Exclusions, plan.Conflicts, plan.Limits}
	return json.Marshal(unsigned)
}

func digestPlan(plan Plan) (string, error) {
	body, err := canonicalPlanBytes(plan)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:]), nil
}

func idForPlan(plan Plan) (string, error) {
	digest, err := digestPlan(plan)
	if err != nil {
		return "", err
	}
	return "wimp-" + digest[:16], nil
}

func documentExtension(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".doc", ".docx", ".pdf", ".ppt", ".pptx", ".xls", ".xlsx", ".odt", ".ods", ".odp", ".epub", ".zip":
		return true
	}
	return false
}

func migratableText(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".md", ".txt", ".json", ".yaml", ".yml", ".toml", ".ini", ".csv", ".html", ".htm", ".xml", ".go", ".py", ".js", ".ts", ".sql":
		return true
	}
	return false
}

// BuildPlan derives an immutable plan from a bounded inspection. It hashes
// each allowlisted source file so execution can fail closed if content changes
// after approval. It does not create destination directories or change either
// tree.
func BuildPlan(source, destination string, limits Limits) (Plan, error) {
	limits, err := normalizeLimits(limits)
	if err != nil {
		return Plan{}, err
	}
	origin, err := cleanDirectory(source, "source", true)
	if err != nil {
		return Plan{}, err
	}
	target, err := cleanDirectory(destination, "destination", true)
	if err != nil {
		return Plan{}, err
	}
	if same, _ := filepath.Abs(origin); same == target || pathWithin(origin, target) || pathWithin(target, origin) {
		return Plan{}, errors.New("source and destination must be separate")
	}
	inspection, err := inspectDirectory(origin, limits)
	if err != nil {
		return Plan{}, err
	}
	if inspection.State == "bounded" {
		return Plan{}, errors.New("source inventory reached a bound; raise limits before creating an import plan")
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	plan := Plan{SchemaVersion: SchemaVersion, Origin: origin, Destination: target, Classification: inspection.Classification, ClassificationReason: inspection.ClassificationReason, CreatedAt: now, State: PlanStatePlanned, Limits: limits, Entries: []PlanEntry{}, Exclusions: []Exclusion{}, Conflicts: []Conflict{}}
	plan.PlanID, err = idForPlan(plan)
	if err != nil {
		return Plan{}, err
	}
	for _, item := range inspection.Entries {
		if item.Kind == "directory" {
			continue
		}
		if item.Unsafe || item.Kind != "file" {
			plan.Exclusions = append(plan.Exclusions, Exclusion{Path: item.RelativePath, Reason: "unsafe filesystem entry; symlinks and special files are never followed"})
			continue
		}
		if item.RelativePath == ".bcgos" || strings.HasPrefix(item.RelativePath, ".bcgos/") {
			plan.Exclusions = append(plan.Exclusions, Exclusion{Path: item.RelativePath, Reason: "managed Maestro metadata is not migrated"})
			continue
		}
		if inspection.Classification == ClassificationMaestroNative {
			plan.Exclusions = append(plan.Exclusions, Exclusion{Path: item.RelativePath, Reason: "native Maestro workspace migration is unsupported"})
			continue
		}
		destRel := filepath.ToSlash(item.RelativePath)
		destPath, pathErr := pathSafe(target, destRel)
		if pathErr != nil {
			plan.Exclusions = append(plan.Exclusions, Exclusion{Path: item.RelativePath, Reason: pathErr.Error()})
			continue
		}
		if _, statErr := os.Lstat(destPath); statErr == nil {
			plan.Conflicts = append(plan.Conflicts, Conflict{Path: destRel, Reason: "destination path already exists"})
			continue
		} else if !errors.Is(statErr, os.ErrNotExist) {
			plan.Conflicts = append(plan.Conflicts, Conflict{Path: destRel, Reason: "destination path cannot be inspected"})
			continue
		}
		contentDigest, digestErr := digestSourceFile(origin, item.RelativePath, item.Size, item.ModifiedUnix, limits.MaxFileBytes)
		if digestErr != nil {
			return Plan{}, fmt.Errorf("digest source %s: %w", item.RelativePath, digestErr)
		}
		entry := PlanEntry{SourcePath: item.RelativePath, DestinationPath: destRel, Size: item.Size, Mode: item.Mode, ModifiedUnix: item.ModifiedUnix, Action: ActionCopy, Availability: "available", ContentDigest: contentDigest}
		if documentExtension(item.RelativePath) {
			entry.Action, entry.Availability, entry.Reason = ActionQuarantine, "unavailable", "document conversion runtime is unavailable; migration does not ingest documents"
		} else if !migratableText(item.RelativePath) {
			entry.Action, entry.Availability, entry.Reason = ActionQuarantine, "unsupported", "file format is not allowlisted for workspace migration"
		}
		plan.Entries = append(plan.Entries, entry)
	}
	sort.Slice(plan.Entries, func(i, j int) bool { return plan.Entries[i].SourcePath < plan.Entries[j].SourcePath })
	sort.Slice(plan.Exclusions, func(i, j int) bool { return plan.Exclusions[i].Path < plan.Exclusions[j].Path })
	sort.Slice(plan.Conflicts, func(i, j int) bool { return plan.Conflicts[i].Path < plan.Conflicts[j].Path })
	plan.PlanDigest, err = digestPlan(plan)
	if err != nil {
		return Plan{}, err
	}
	return plan, nil
}

func Approve(plan Plan, approvedBy, confirmation string) (Approval, error) {
	if err := ValidatePlan(plan); err != nil {
		return Approval{}, err
	}
	if strings.TrimSpace(approvedBy) == "" {
		return Approval{}, errors.New("approver is required")
	}
	if confirmation != ConfirmImport {
		return Approval{}, errors.New("explicit IMPORT confirmation is required")
	}
	return Approval{SchemaVersion: SchemaVersion, PlanID: plan.PlanID, PlanDigest: plan.PlanDigest, ApprovedBy: approvedBy, ApprovedAt: time.Now().UTC().Format(time.RFC3339Nano), Confirmation: confirmation}, nil
}

func ValidatePlan(plan Plan) error {
	if plan.SchemaVersion != SchemaVersion || plan.State != PlanStatePlanned && plan.State != PlanStateApproved {
		return errors.New("workspace import plan is invalid or already executed")
	}
	digest, err := digestPlan(plan)
	if err != nil {
		return err
	}
	if digest != plan.PlanDigest {
		return errors.New("workspace import plan digest mismatch")
	}
	if plan.Classification == ClassificationMaestroNative {
		return errors.New("native Maestro workspace migration is unsupported")
	}
	if len(plan.Conflicts) > 0 {
		return errors.New("workspace import plan has unresolved conflicts")
	}
	for _, entry := range plan.Entries {
		if entry.Action != ActionExclude && !isSHA256(entry.ContentDigest) {
			return fmt.Errorf("workspace import entry %s has no valid content digest", entry.SourcePath)
		}
	}
	return nil
}

func ValidateApproval(plan Plan, approval Approval) error {
	if err := ValidatePlan(plan); err != nil {
		return err
	}
	if approval.SchemaVersion != SchemaVersion || approval.PlanID != plan.PlanID || approval.PlanDigest != plan.PlanDigest || approval.Confirmation != ConfirmImport {
		return errors.New("approval does not match the immutable workspace import plan")
	}
	return nil
}

func expectedReceiptPaths(plan Plan) (copied, quarantined, excluded []string, rollback map[string]string, err error) {
	rollback = map[string]string{}
	for _, entry := range plan.Entries {
		switch entry.Action {
		case ActionCopy:
			copied = append(copied, entry.SourcePath)
		case ActionQuarantine:
			quarantined = append(quarantined, entry.SourcePath)
		case ActionExclude:
			excluded = append(excluded, entry.SourcePath)
		default:
			return nil, nil, nil, nil, fmt.Errorf("receipt validation encountered unsupported plan action %q", entry.Action)
		}
		if entry.Action != ActionExclude {
			rollbackPath := entry.DestinationPath
			if entry.Action == ActionQuarantine {
				rollbackPath = filepath.ToSlash(filepath.Join(".bcgos", "import-quarantine", "run-"+plan.PlanDigest[:16], entry.DestinationPath))
			}
			if _, exists := rollback[rollbackPath]; exists {
				return nil, nil, nil, nil, fmt.Errorf("duplicate rollback path in plan: %s", rollbackPath)
			}
			rollback[rollbackPath] = entry.ContentDigest
		}
	}
	return copied, quarantined, excluded, rollback, nil
}

func validateReceiptPathList(label string, expected, actual []string) error {
	if len(expected) != len(actual) {
		return fmt.Errorf("receipt %s paths do not match the plan", label)
	}
	seen := make(map[string]struct{}, len(actual))
	for _, path := range actual {
		if _, exists := seen[path]; exists {
			return fmt.Errorf("receipt %s contains duplicate path %q", label, path)
		}
		seen[path] = struct{}{}
	}
	for _, path := range expected {
		if _, exists := seen[path]; !exists {
			return fmt.Errorf("receipt %s is missing path %q", label, path)
		}
	}
	return nil
}

// ValidateReceipt proves that a terminal receipt belongs to this exact plan
// and describes exactly the entries and digests that execution committed.
func ValidateReceipt(plan Plan, receipt Receipt) error {
	if err := ValidatePlan(plan); err != nil {
		return err
	}
	if receipt.SchemaVersion != SchemaVersion || receipt.RunID != "run-"+plan.PlanDigest[:16] || receipt.PlanID != plan.PlanID || receipt.PlanDigest != plan.PlanDigest {
		return errors.New("receipt identity does not match the immutable workspace import plan")
	}
	if receipt.State != PlanStateExecuted && receipt.State != PlanStateRolledBack {
		return errors.New("receipt is not a terminal workspace import receipt")
	}
	if strings.TrimSpace(receipt.Error) != "" {
		return errors.New("terminal receipt cannot contain an error")
	}
	if _, err := time.Parse(time.RFC3339Nano, receipt.RecordedAt); err != nil {
		return errors.New("receipt recorded_at is invalid")
	}
	copied, quarantined, excluded, rollback, err := expectedReceiptPaths(plan)
	if err != nil {
		return err
	}
	if err := validateReceiptPathList("copied", copied, receipt.Copied); err != nil {
		return err
	}
	if err := validateReceiptPathList("quarantined", quarantined, receipt.Quarantined); err != nil {
		return err
	}
	if err := validateReceiptPathList("excluded", excluded, receipt.Excluded); err != nil {
		return err
	}
	if err := validateReceiptPathList("rollback", mapKeys(rollback), receipt.RollbackPaths); err != nil {
		return err
	}
	if len(receipt.RollbackDigests) != len(rollback) {
		return errors.New("receipt rollback digests do not match the plan")
	}
	for path, expectedDigest := range rollback {
		actualDigest, exists := receipt.RollbackDigests[path]
		if !exists || actualDigest != expectedDigest || !isSHA256(actualDigest) {
			return fmt.Errorf("receipt rollback digest does not match path %q", path)
		}
	}
	for path := range receipt.RollbackDigests {
		if _, exists := rollback[path]; !exists {
			return fmt.Errorf("receipt contains an unknown rollback path %q", path)
		}
	}
	return nil
}

func mapKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	return keys
}

func journalPath(root, runID string) string {
	return filepath.Join(root, "workspace-import", "journal", runID+".json")
}

func buildJournal(plan Plan, stageRoot, runID string) Journal {
	journal := Journal{SchemaVersion: SchemaVersion, RunID: runID, PlanID: plan.PlanID, PlanDigest: plan.PlanDigest, State: "prepared", RecordedAt: time.Now().UTC().Format(time.RFC3339Nano), Entries: []JournalEntry{}, Committed: []string{}}
	for _, entry := range plan.Entries {
		if entry.Action == ActionExclude {
			continue
		}
		destinationPath := entry.DestinationPath
		if entry.Action == ActionQuarantine {
			destinationPath = filepath.ToSlash(filepath.Join(".bcgos", "import-quarantine", runID, entry.DestinationPath))
		}
		journal.Entries = append(journal.Entries, JournalEntry{SourcePath: entry.SourcePath, StagePath: entry.DestinationPath, DestinationPath: destinationPath, Action: entry.Action, ContentDigest: entry.ContentDigest})
	}
	return journal
}

func ValidateJournal(plan Plan, journal Journal) error {
	if err := ValidatePlan(plan); err != nil {
		return err
	}
	if journal.SchemaVersion != SchemaVersion || journal.RunID != "run-"+plan.PlanDigest[:16] || journal.PlanID != plan.PlanID || journal.PlanDigest != plan.PlanDigest {
		return errors.New("journal identity does not match the immutable workspace import plan")
	}
	switch journal.State {
	case "prepared", "committing", "failed", "reconciled":
	default:
		return errors.New("journal has an unsupported state")
	}
	if _, err := time.Parse(time.RFC3339Nano, journal.RecordedAt); err != nil {
		return errors.New("journal recorded_at is invalid")
	}
	expected := buildJournal(plan, filepath.Join("/", "workspace-import", "staging", journal.RunID), journal.RunID)
	if len(journal.Entries) != len(expected.Entries) {
		return errors.New("journal entries do not match the plan")
	}
	for index, entry := range expected.Entries {
		if journal.Entries[index] != entry {
			return fmt.Errorf("journal entry %d does not match the plan", index)
		}
	}
	committed := make(map[string]struct{}, len(journal.Committed))
	for _, path := range journal.Committed {
		if _, exists := committed[path]; exists {
			return fmt.Errorf("journal contains duplicate committed path %q", path)
		}
		committed[path] = struct{}{}
	}
	allowed := map[string]struct{}{}
	for _, entry := range journal.Entries {
		allowed[entry.DestinationPath] = struct{}{}
	}
	for path := range committed {
		if _, exists := allowed[path]; !exists {
			return fmt.Errorf("journal contains unknown committed path %q", path)
		}
	}
	return nil
}

func writeJournal(path string, journal Journal) error {
	if err := writeJSONAtomic(path, journal); err != nil {
		return err
	}
	return syncDirectoryPath(filepath.Dir(path))
}

func journalHasCommitted(journal Journal, path string) bool {
	for _, committed := range journal.Committed {
		if committed == path {
			return true
		}
	}
	return false
}

func reconcilePendingJournal(root string, plan Plan) error {
	runID := "run-" + plan.PlanDigest[:16]
	path := journalPath(root, runID)
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return errors.New("workspace import journal is a symlink")
	}
	var journal Journal
	if err := readJSON(path, &journal); err != nil {
		return fmt.Errorf("workspace import journal is invalid: %w", err)
	}
	if err := ValidateJournal(plan, journal); err != nil {
		return fmt.Errorf("workspace import journal failed validation: %w", err)
	}
	if journal.State == "failed" {
		return nil
	}
	receiptPath := filepath.Join(root, "workspace-import", "receipts", runID+".json")
	if receiptInfo, statErr := os.Lstat(receiptPath); statErr == nil {
		if receiptInfo.Mode()&os.ModeSymlink != 0 {
			return errors.New("workspace import receipt is a symlink during journal recovery")
		}
		var existing Receipt
		if err := readJSON(receiptPath, &existing); err != nil {
			return fmt.Errorf("workspace import receipt is invalid during journal recovery: %w", err)
		}
		if existing.State == PlanStateExecuted || existing.State == PlanStateRolledBack {
			if err := ValidateReceipt(plan, existing); err != nil {
				return fmt.Errorf("workspace import receipt failed validation during journal recovery: %w", err)
			}
			journal.State = "reconciled"
			journal.RecordedAt = time.Now().UTC().Format(time.RFC3339Nano)
			return writeJournal(path, journal)
		}
	}
	journal.State = "committing"
	journal.RecordedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if err := writeJournal(path, journal); err != nil {
		return err
	}
	stageRoot := filepath.Join(root, "workspace-import", "staging", runID)
	for _, entry := range journal.Entries {
		destinationDigest, destinationExists, err := digestFileSecure(plan.Destination, entry.DestinationPath, plan.Limits.MaxFileBytes)
		if err != nil {
			return fmt.Errorf("reconcile destination %s: %w", entry.DestinationPath, err)
		}
		stageDigest, stageExists, err := digestFileSecure(stageRoot, entry.StagePath, plan.Limits.MaxFileBytes)
		if err != nil {
			return fmt.Errorf("reconcile stage %s: %w", entry.StagePath, err)
		}
		if destinationExists {
			if destinationDigest != entry.ContentDigest {
				return fmt.Errorf("reconcile found changed destination %s", entry.DestinationPath)
			}
			if stageExists {
				if stageDigest != entry.ContentDigest {
					return fmt.Errorf("reconcile found changed stage %s", entry.StagePath)
				}
				if err := removeFileIfDigestSecure(stageRoot, entry.StagePath, entry.ContentDigest, plan.Limits.MaxFileBytes); err != nil {
					return fmt.Errorf("reconcile stage cleanup %s: %w", entry.StagePath, err)
				}
			}
		} else {
			if !stageExists || stageDigest != entry.ContentDigest {
				return fmt.Errorf("reconcile cannot prove staged content for %s", entry.StagePath)
			}
			installed, err := secureCommitFile(stageRoot, entry.StagePath, plan.Destination, entry.DestinationPath)
			if installed {
				stageExists = false
			}
			if err != nil {
				return fmt.Errorf("reconcile commit %s: %w", entry.DestinationPath, err)
			}
			if !installed {
				return fmt.Errorf("reconcile did not install %s", entry.DestinationPath)
			}
		}
		if !journalHasCommitted(journal, entry.DestinationPath) {
			journal.Committed = append(journal.Committed, entry.DestinationPath)
			journal.RecordedAt = time.Now().UTC().Format(time.RFC3339Nano)
			if err := writeJournal(path, journal); err != nil {
				return err
			}
		}
	}
	copied, quarantined, excluded, rollback, err := expectedReceiptPaths(plan)
	if err != nil {
		return err
	}
	receipt := Receipt{SchemaVersion: SchemaVersion, RunID: runID, PlanID: plan.PlanID, PlanDigest: plan.PlanDigest, State: PlanStateExecuted, RecordedAt: time.Now().UTC().Format(time.RFC3339Nano), Copied: copied, Quarantined: quarantined, Excluded: excluded, RollbackPaths: mapKeys(rollback), RollbackDigests: rollback}
	if err := ValidateReceipt(plan, receipt); err != nil {
		return err
	}
	if err := writeJSONAtomic(receiptPath, receipt); err != nil {
		return err
	}
	journal.State = "reconciled"
	journal.RecordedAt = time.Now().UTC().Format(time.RFC3339Nano)
	return writeJournal(path, journal)
}

func writeJSONAtomic(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	tmp, err := os.CreateTemp(filepath.Dir(path), ".workspace-import-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(body); err != nil {
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
	return os.Rename(tmpName, path)
}

func ReadPlan(path string) (Plan, error) {
	var plan Plan
	if err := readJSON(path, &plan); err != nil {
		return plan, err
	}
	if err := ValidatePlan(plan); err != nil {
		return plan, err
	}
	return plan, nil
}
func ReadApproval(path string) (Approval, error) {
	var approval Approval
	if err := readJSON(path, &approval); err != nil {
		return approval, err
	}
	return approval, nil
}
func ReadReceipt(path string) (Receipt, error) {
	var receipt Receipt
	if err := readJSON(path, &receipt); err != nil {
		return receipt, err
	}
	return receipt, nil
}

func readJSON(path string, target any) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	decoder := json.NewDecoder(io.LimitReader(file, 2<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("JSON artifact has trailing data")
	}
	return nil
}

// SavePlan stores a plan only after revalidating its immutable digest.
func SavePlan(path string, plan Plan) error {
	if err := ValidatePlan(plan); err != nil {
		return err
	}
	return writeJSONAtomic(path, plan)
}
func SaveApproval(path string, approval Approval) error {
	if approval.SchemaVersion != SchemaVersion || approval.Confirmation != ConfirmImport {
		return errors.New("invalid approval")
	}
	return writeJSONAtomic(path, approval)
}

func isSHA256(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func digestSourceFile(root, relative string, expectedSize, expectedModified, maxBytes int64) (string, error) {
	file, err := openSourceFileSecure(root, relative)
	if err != nil {
		return "", err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return "", err
	}
	if info.Size() != expectedSize || info.ModTime().UnixNano() != expectedModified {
		return "", errors.New("source metadata changed while planning")
	}
	if info.Size() > maxBytes {
		return "", errors.New("source file exceeds import limit")
	}
	hasher := sha256.New()
	written, err := io.Copy(hasher, io.LimitReader(file, maxBytes+1))
	if err != nil {
		return "", err
	}
	if written != expectedSize {
		return "", errors.New("source changed while hashing")
	}
	finalInfo, err := file.Stat()
	if err != nil {
		return "", err
	}
	if finalInfo.Size() != expectedSize || finalInfo.ModTime().UnixNano() != expectedModified {
		return "", errors.New("source metadata changed while hashing")
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func copyBounded(sourceRoot, sourceRelative, destination string, expectedSize, expectedModified, maxBytes int64, expectedDigest string) error {
	in, err := openSourceFileSecure(sourceRoot, sourceRelative)
	if err != nil {
		return err
	}
	defer in.Close()
	info, err := in.Stat()
	if err != nil {
		return err
	}
	if info.Size() != expectedSize || info.ModTime().UnixNano() != expectedModified {
		return errors.New("source metadata changed after planning")
	}
	if expectedSize > maxBytes {
		return errors.New("source file exceeds import limit")
	}
	out, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	defer out.Close()
	hasher := sha256.New()
	written, err := io.Copy(out, io.LimitReader(io.TeeReader(in, hasher), maxBytes+1))
	if err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	if written > maxBytes {
		return errors.New("source file exceeds import limit")
	}
	if written != expectedSize {
		return errors.New("source changed while staging")
	}
	if hex.EncodeToString(hasher.Sum(nil)) != expectedDigest {
		return errors.New("source content changed after planning")
	}
	finalInfo, err := in.Stat()
	if err != nil {
		return err
	}
	if finalInfo.Size() != expectedSize || finalInfo.ModTime().UnixNano() != expectedModified {
		return errors.New("source metadata changed while staging")
	}
	return out.Sync()
}

func safeRemove(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return os.Remove(path)
	}
	return os.RemoveAll(path)
}

func removeCreated(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.IsDir() && info.Mode()&os.ModeSymlink == 0 {
		return errors.New("refusing to remove a directory during import rollback")
	}
	return os.Remove(path)
}

func ensureParent(root, path string) error {
	relative, err := filepath.Rel(root, path)
	if err != nil || strings.HasPrefix(relative, "..") {
		return errors.New("destination parent escapes root")
	}
	current := root
	for _, component := range strings.Split(relative, string(filepath.Separator)) {
		if component == "" || component == "." {
			continue
		}
		current = filepath.Join(current, component)
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
			return errors.New("destination parent contains an unsafe filesystem entry")
		}
	}
	return nil
}

type committedImport struct {
	root   string
	rel    string
	digest string
}

func removeCreatedExpected(root, relative, expectedDigest string, maxBytes int64) error {
	return removeFileIfDigestSecure(root, relative, expectedDigest, maxBytes)
}

// Execute applies an approved plan through a private staging directory. A
// repeated execution of the same plan returns the existing receipt. Every
// post-staging failure is recorded before the transaction is cleaned up.
func Execute(dataRoot string, plan Plan, approval Approval) (Receipt, error) {
	if err := ValidateApproval(plan, approval); err != nil {
		return Receipt{}, err
	}
	root, err := cleanDirectory(dataRoot, "data root", false)
	if err != nil {
		return Receipt{}, err
	}
	lock, err := acquirePlanLock(root, plan.PlanDigest)
	if err != nil {
		return Receipt{}, err
	}
	defer lock.release()
	if executeLockHook != nil {
		executeLockHook()
	}
	runID := "run-" + plan.PlanDigest[:16]
	receiptPath := filepath.Join(root, "workspace-import", "receipts", runID+".json")
	if err := reconcilePendingJournal(root, plan); err != nil {
		return Receipt{}, err
	}
	if info, statErr := os.Lstat(receiptPath); statErr == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return Receipt{}, errors.New("existing workspace import receipt is a symlink")
		}
		existing, readErr := ReadReceipt(receiptPath)
		if readErr != nil {
			return Receipt{}, fmt.Errorf("existing workspace import receipt is invalid: %w", readErr)
		}
		if existing.SchemaVersion != SchemaVersion || existing.RunID != runID || existing.PlanID != plan.PlanID || existing.PlanDigest != plan.PlanDigest {
			return Receipt{}, errors.New("existing workspace import receipt identity does not match the plan")
		}
		switch existing.State {
		case PlanStateExecuted, PlanStateRolledBack:
			if err := ValidateReceipt(plan, existing); err != nil {
				return Receipt{}, fmt.Errorf("existing workspace import receipt failed validation: %w", err)
			}
			return existing, nil
		case "staging", "failed":
			// A same-plan non-terminal receipt may be resumed.
		default:
			return Receipt{}, errors.New("existing workspace import receipt has an unsupported state")
		}
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return Receipt{}, statErr
	}
	if _, err := cleanDirectory(plan.Origin, "source", true); err != nil {
		return Receipt{}, err
	}
	if _, err := cleanDirectory(plan.Destination, "destination", true); err != nil {
		return Receipt{}, err
	}
	receipt := Receipt{SchemaVersion: SchemaVersion, RunID: runID, PlanID: plan.PlanID, PlanDigest: plan.PlanDigest, State: "staging", RecordedAt: time.Now().UTC().Format(time.RFC3339Nano), RollbackDigests: map[string]string{}}
	stageRoot := filepath.Join(root, "workspace-import", "staging", runID)
	if err := writeJSONAtomic(receiptPath, receipt); err != nil {
		return receipt, err
	}
	if err := os.MkdirAll(stageRoot, 0o700); err != nil {
		receipt.State, receipt.Error = "failed", err.Error()
		_ = writeJSONAtomic(receiptPath, receipt)
		return receipt, err
	}
	cleanupStage := true
	committed := []committedImport{}
	journal := Journal{}
	journalFile := journalPath(root, runID)
	defer func() {
		if cleanupStage {
			_ = safeRemove(stageRoot)
		}
	}()
	abort := func(cause error) (Receipt, error) {
		cleanupErrors := []error{}
		for index := len(committed) - 1; index >= 0; index-- {
			item := committed[index]
			if err := removeCreatedExpected(item.root, item.rel, item.digest, plan.Limits.MaxFileBytes); err != nil {
				cleanupErrors = append(cleanupErrors, err)
			}
		}
		receipt.State, receipt.Error = "failed", cause.Error()
		if len(cleanupErrors) > 0 {
			receipt.Error = errors.Join(cause, errors.Join(cleanupErrors...)).Error()
		}
		if journal.RunID != "" {
			journal.State = "failed"
			journal.RecordedAt = time.Now().UTC().Format(time.RFC3339Nano)
			_ = writeJournal(journalFile, journal)
		}
		receipt.RecordedAt = time.Now().UTC().Format(time.RFC3339Nano)
		if persistErr := writeJSONAtomic(receiptPath, receipt); persistErr != nil {
			return receipt, errors.Join(cause, persistErr)
		}
		return receipt, cause
	}
	for _, entry := range plan.Entries {
		_, err := pathSafe(plan.Origin, entry.SourcePath)
		if err != nil {
			return abort(err)
		}
		stage, err := pathSafe(stageRoot, entry.DestinationPath)
		if err != nil {
			return abort(err)
		}
		if entry.Action == ActionExclude {
			receipt.Excluded = append(receipt.Excluded, entry.SourcePath)
			continue
		}
		if err := os.MkdirAll(filepath.Dir(stage), 0o700); err != nil {
			return abort(err)
		}
		if err := copyBounded(plan.Origin, entry.SourcePath, stage, entry.Size, entry.ModifiedUnix, plan.Limits.MaxFileBytes, entry.ContentDigest); err != nil {
			return abort(err)
		}
		if entry.Action == ActionQuarantine {
			receipt.Quarantined = append(receipt.Quarantined, entry.SourcePath)
		} else {
			receipt.Copied = append(receipt.Copied, entry.SourcePath)
		}
		if err := writeJSONAtomic(receiptPath, receipt); err != nil {
			return abort(err)
		}
	}
	journal = buildJournal(plan, stageRoot, runID)
	if err := writeJournal(journalFile, journal); err != nil {
		return abort(err)
	}
	for _, entry := range plan.Entries {
		if entry.Action == ActionExclude {
			continue
		}
		rollbackPath := entry.DestinationPath
		if entry.Action == ActionQuarantine {
			rollbackPath = filepath.ToSlash(filepath.Join(".bcgos", "import-quarantine", runID, entry.DestinationPath))
		}
		installed, commitErr := secureCommitFile(stageRoot, entry.DestinationPath, plan.Destination, rollbackPath)
		if installed {
			committed = append(committed, committedImport{root: plan.Destination, rel: rollbackPath, digest: entry.ContentDigest})
			if !journalHasCommitted(journal, rollbackPath) {
				journal.Committed = append(journal.Committed, rollbackPath)
				journal.State = "committing"
				journal.RecordedAt = time.Now().UTC().Format(time.RFC3339Nano)
				if err := writeJournal(journalFile, journal); err != nil {
					return abort(err)
				}
			}
		}
		if commitErr != nil {
			return abort(commitErr)
		}
		receipt.RollbackPaths = append(receipt.RollbackPaths, rollbackPath)
		receipt.RollbackDigests[rollbackPath] = entry.ContentDigest
		if err := writeJSONAtomic(receiptPath, receipt); err != nil {
			return abort(err)
		}
	}
	receipt.State = "executed"
	if err := ValidateReceipt(plan, receipt); err != nil {
		return abort(err)
	}
	if err := writeJSONAtomic(receiptPath, receipt); err != nil {
		return receipt, err
	}
	journal.State = "reconciled"
	journal.RecordedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if err := writeJournal(journalFile, journal); err != nil {
		return receipt, err
	}
	return receipt, nil
}

// Rollback removes only paths recorded by a successful receipt and never
// touches the source tree. It is safe to replay after a partial cleanup.
func Rollback(dataRoot string, plan Plan, receipt Receipt, confirmation string) (Receipt, error) {
	if confirmation != ConfirmRollback {
		return Receipt{}, errors.New("explicit ROLLBACK confirmation is required")
	}
	if err := ValidateReceipt(plan, receipt); err != nil {
		return Receipt{}, fmt.Errorf("receipt is not rollbackable: %w", err)
	}
	if receipt.State == PlanStateRolledBack {
		return receipt, nil
	}
	root, err := cleanDirectory(dataRoot, "data root", true)
	if err != nil {
		return Receipt{}, err
	}
	lock, err := acquirePlanLock(root, plan.PlanDigest)
	if err != nil {
		return Receipt{}, err
	}
	defer lock.release()
	if _, err := cleanDirectory(plan.Destination, "destination", true); err != nil {
		return Receipt{}, err
	}
	for _, relative := range receipt.RollbackPaths {
		digest := receipt.RollbackDigests[relative]
		if err := removeCreatedExpected(plan.Destination, relative, digest, plan.Limits.MaxFileBytes); err != nil {
			return Receipt{}, err
		}
	}
	receipt.State = PlanStateRolledBack
	receipt.RecordedAt = time.Now().UTC().Format(time.RFC3339Nano)
	receiptPath := filepath.Join(root, "workspace-import", "receipts", receipt.RunID+".json")
	if err := writeJSONAtomic(receiptPath, receipt); err != nil {
		return Receipt{}, err
	}
	return receipt, nil
}

// RollbackPlan removes only files created by Execute for this plan.
func RollbackPlan(dataRoot string, plan Plan, receipt Receipt, confirmation string) (Receipt, error) {
	return Rollback(dataRoot, plan, receipt, confirmation)
}

func JSON(value any) ([]byte, error) {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}
