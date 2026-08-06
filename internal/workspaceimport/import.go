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
	SchemaVersion int      `json:"schema_version"`
	RunID         string   `json:"run_id"`
	PlanID        string   `json:"plan_id"`
	PlanDigest    string   `json:"plan_digest"`
	State         string   `json:"state"`
	RecordedAt    string   `json:"recorded_at"`
	Copied        []string `json:"copied,omitempty"`
	Quarantined   []string `json:"quarantined,omitempty"`
	Excluded      []string `json:"excluded,omitempty"`
	RollbackPaths []string `json:"rollback_paths,omitempty"`
	Error         string   `json:"error,omitempty"`
}

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

// BuildPlan derives an immutable plan from a bounded inspection. It does not
// read source file bodies, create destination directories, or change either
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
		entry := PlanEntry{SourcePath: item.RelativePath, DestinationPath: destRel, Size: item.Size, Mode: item.Mode, ModifiedUnix: item.ModifiedUnix, Action: ActionCopy, Availability: "available"}
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

func copyBounded(source, destination string, expectedSize, expectedModified, maxBytes int64) error {
	info, err := os.Lstat(source)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return errors.New("source changed to an unsafe filesystem entry")
	}
	if info.Size() != expectedSize || info.ModTime().UnixNano() != expectedModified {
		return errors.New("source metadata changed after planning")
	}
	if expectedSize > maxBytes {
		return errors.New("source file exceeds import limit")
	}
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	defer out.Close()
	written, err := io.CopyN(out, in, maxBytes+1)
	if err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	if written > maxBytes {
		return errors.New("source file exceeds import limit")
	}
	if written != expectedSize {
		return errors.New("source changed while staging")
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

// Execute applies an approved plan through a private staging directory. A
// repeated execution of the same plan returns the existing receipt.
func Execute(dataRoot string, plan Plan, approval Approval) (Receipt, error) {
	if err := ValidateApproval(plan, approval); err != nil {
		return Receipt{}, err
	}
	root, err := cleanDirectory(dataRoot, "data root", false)
	if err != nil {
		return Receipt{}, err
	}
	runID := "run-" + plan.PlanDigest[:16]
	receiptPath := filepath.Join(root, "workspace-import", "receipts", runID+".json")
	if existing, readErr := ReadReceipt(receiptPath); readErr == nil {
		return existing, nil
	}
	if _, err := cleanDirectory(plan.Origin, "source", true); err != nil {
		return Receipt{}, err
	}
	if _, err := cleanDirectory(plan.Destination, "destination", true); err != nil {
		return Receipt{}, err
	}
	receipt := Receipt{SchemaVersion: SchemaVersion, RunID: runID, PlanID: plan.PlanID, PlanDigest: plan.PlanDigest, State: "staging", RecordedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	stageRoot := filepath.Join(root, "workspace-import", "staging", runID)
	if err := os.MkdirAll(stageRoot, 0o700); err != nil {
		return Receipt{}, err
	}
	cleanupStage := true
	committed := []string{}
	succeeded := false
	defer func() {
		if cleanupStage {
			_ = safeRemove(stageRoot)
		}
		if !succeeded {
			for index := len(committed) - 1; index >= 0; index-- {
				_ = removeCreated(committed[index])
			}
		}
	}()
	for _, entry := range plan.Entries {
		source, err := pathSafe(plan.Origin, entry.SourcePath)
		if err != nil {
			receipt.State, receipt.Error = "failed", err.Error()
			return receipt, err
		}
		stage, err := pathSafe(stageRoot, entry.DestinationPath)
		if err != nil {
			receipt.State, receipt.Error = "failed", err.Error()
			return receipt, err
		}
		if entry.Action == ActionExclude {
			receipt.Excluded = append(receipt.Excluded, entry.SourcePath)
			continue
		}
		if err := os.MkdirAll(filepath.Dir(stage), 0o700); err != nil {
			receipt.State, receipt.Error = "failed", err.Error()
			return receipt, err
		}
		if err := copyBounded(source, stage, entry.Size, entry.ModifiedUnix, plan.Limits.MaxFileBytes); err != nil {
			receipt.State, receipt.Error = "failed", err.Error()
			return receipt, err
		}
		if entry.Action == ActionQuarantine {
			receipt.Quarantined = append(receipt.Quarantined, entry.SourcePath)
		} else {
			receipt.Copied = append(receipt.Copied, entry.SourcePath)
		}
	}
	for _, entry := range plan.Entries {
		if entry.Action == ActionExclude {
			continue
		}
		stage, _ := pathSafe(stageRoot, entry.DestinationPath)
		destination, _ := pathSafe(plan.Destination, entry.DestinationPath)
		if entry.Action == ActionQuarantine {
			destination, _ = pathSafe(plan.Destination, filepath.ToSlash(filepath.Join(".bcgos", "import-quarantine", runID, entry.DestinationPath)))
		}
		if _, err := os.Lstat(destination); err == nil {
			receipt.State, receipt.Error = "failed", "destination changed after approval"
			return receipt, errors.New(receipt.Error)
		} else if !errors.Is(err, os.ErrNotExist) {
			receipt.State, receipt.Error = "failed", err.Error()
			return receipt, err
		}
		if err := ensureParent(plan.Destination, filepath.Dir(destination)); err != nil {
			receipt.State, receipt.Error = "failed", err.Error()
			return receipt, err
		}
		if err := os.Rename(stage, destination); err != nil {
			receipt.State, receipt.Error = "failed", err.Error()
			return receipt, err
		}
		committed = append(committed, destination)
		rollbackPath := entry.DestinationPath
		if entry.Action == ActionQuarantine {
			rollbackPath = filepath.ToSlash(filepath.Join(".bcgos", "import-quarantine", runID, entry.DestinationPath))
		}
		receipt.RollbackPaths = append(receipt.RollbackPaths, rollbackPath)
	}
	receipt.State = "executed"
	if err := writeJSONAtomic(receiptPath, receipt); err != nil {
		return receipt, err
	}
	succeeded = true
	return receipt, nil
}

// Rollback removes only paths recorded by a successful receipt and never
// touches the source tree. It is safe to replay after a partial cleanup.
func Rollback(dataRoot string, plan Plan, receipt Receipt, confirmation string) (Receipt, error) {
	if confirmation != ConfirmRollback {
		return Receipt{}, errors.New("explicit ROLLBACK confirmation is required")
	}
	if receipt.SchemaVersion != SchemaVersion || (receipt.State != "executed" && receipt.State != PlanStateRolledBack) || receipt.PlanDigest != plan.PlanDigest {
		return Receipt{}, errors.New("receipt is not rollbackable")
	}
	if receipt.State == PlanStateRolledBack {
		return receipt, nil
	}
	if _, err := cleanDirectory(dataRoot, "data root", true); err != nil {
		return Receipt{}, err
	}
	if _, err := cleanDirectory(plan.Destination, "destination", true); err != nil {
		return Receipt{}, err
	}
	for _, relative := range receipt.RollbackPaths {
		target, err := pathSafe(plan.Destination, relative)
		if err != nil {
			return Receipt{}, err
		}
		if err := removeCreated(target); err != nil {
			return Receipt{}, err
		}
	}
	receipt.State = PlanStateRolledBack
	receipt.RecordedAt = time.Now().UTC().Format(time.RFC3339Nano)
	receiptPath := filepath.Join(dataRoot, "workspace-import", "receipts", receipt.RunID+".json")
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
