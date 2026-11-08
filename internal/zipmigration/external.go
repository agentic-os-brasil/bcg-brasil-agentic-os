package zipmigration

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/userlevel"
)

const externalControl = "brain/.maestro/external-migration"
const externalMaximum = 16 << 20
const externalIntentControl = ".maestro-external-migration"

var externalAttempt = regexp.MustCompile(`^[0-9a-f]{32}$`)
var externalDigestPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

type ExternalOptions struct {
	SourceRoot  string `json:"source_root"`
	ProductRoot string `json:"product_root"`
	UserHome    string `json:"user_home"`
	Home        string `json:"home"`
}

type ExternalEntry struct {
	Source       string `json:"source"`
	Target       string `json:"target"`
	Kind         string `json:"kind"`
	Disposition  string `json:"disposition"`
	SHA256       string `json:"sha256"`
	TargetSHA256 string `json:"target_sha256"`
	Transform    string `json:"transform"`
}

type ExternalPlan struct {
	SchemaVersion int    `json:"schema_version"`
	AttemptID     string `json:"attempt_id"`
	ExternalOptions
	SourceVersion     string          `json:"source_version"`
	SourceLayout      string          `json:"source_layout"`
	SourceFingerprint string          `json:"source_fingerprint"`
	ArchiveRoot       string          `json:"archive_root"`
	PriorHome         bool            `json:"prior_home"`
	Entries           []ExternalEntry `json:"entries"`
}

func (p ExternalPlan) Bytes() ([]byte, error) {
	b, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return nil, err
	}
	if len(b)+1 > externalMaximum {
		return nil, errors.New("external plan exceeds metadata bound")
	}
	return append(b, '\n'), nil
}

type ExternalProgress struct {
	Phase         string
	Copied, Total int
}
type ExternalStatus struct {
	Current         bool              `json:"current"`
	State           string            `json:"state"`
	Diagnostic      string            `json:"diagnostic"`
	Home            string            `json:"home"`
	SourceRoot      string            `json:"source_root"`
	ProductRoot     string            `json:"product_root"`
	UserHome        string            `json:"user_home"`
	AttemptID       string            `json:"attempt_id"`
	PlanSHA256      string            `json:"plan_sha256"`
	ArchiveRoot     string            `json:"archive_root"`
	RetainedPaths   map[string]string `json:"retained_paths"`
	InactiveCount   int               `json:"inactive_count"`
	RestoredRuntime bool              `json:"restored_runtime"`
}

type externalRecord struct {
	SchemaVersion int    `json:"schema_version"`
	AttemptID     string `json:"attempt_id"`
	PlanSHA256    string `json:"plan_sha256"`
	Phase         string `json:"phase"`
}
type externalOrigin struct {
	SchemaVersion int    `json:"schema_version"`
	Home          string `json:"home"`
	Origin        string `json:"origin"`
	AttemptID     string `json:"attempt_id"`
	PlanSHA256    string `json:"plan_sha256"`
}
type externalLocator struct {
	SchemaVersion int    `json:"schema_version"`
	Home          string `json:"home"`
	AttemptID     string `json:"attempt_id"`
}

func externalStatus(p ExternalPlan, digest, state string) ExternalStatus {
	return externalStatusAt(p, digest, state, p.Home)
}
func externalStatusAt(p ExternalPlan, digest, state, retainedRoot string) ExternalStatus {
	s := ExternalStatus{State: state, Diagnostic: state, Home: p.Home, SourceRoot: p.SourceRoot, ProductRoot: p.ProductRoot, UserHome: p.UserHome, AttemptID: p.AttemptID, PlanSHA256: digest, ArchiveRoot: p.ArchiveRoot, RetainedPaths: map[string]string{}}
	for _, e := range p.Entries {
		if e.Disposition != "live_author" {
			s.InactiveCount++
			for _, n := range legacy {
				prefix := "data/" + n
				if e.Source == prefix || strings.HasPrefix(e.Source, prefix+"/") {
					s.RetainedPaths[n] = filepath.Join(retainedRoot, "lineage", p.AttemptID, prefix)
				}
			}
		}
	}
	return s
}

func externalClean(p string) error {
	if p == "" || !filepath.IsAbs(p) || filepath.Clean(p) != p || strings.ContainsAny(p, "\x00\r\n") {
		return errors.New("external root must be clean and absolute")
	}
	return nil
}
func externalContains(a, b string) bool {
	r, e := filepath.Rel(a, b)
	return e == nil && (r == "." || r != ".." && !strings.HasPrefix(r, ".."+string(os.PathSeparator)))
}

// Missing write leaves still have existing ancestors. Comparing those identities
// catches case/normalization aliases on the actual filesystem without treating
// distinct case-sensitive paths as aliases or requiring retired sources online.
func externalPhysicalContains(a, b string, held ...*externalTree) (bool, error) {
	info, err := externalPathInfo(a, held...)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return false, errors.New("external root is a symlink")
	}
	for current := b; ; current = filepath.Dir(current) {
		other, err := externalPathInfo(current, held...)
		if err == nil {
			if other.Mode()&os.ModeSymlink != 0 {
				return false, errors.New("external ancestor is a symlink")
			}
			if os.SameFile(info, other) {
				return true, nil
			}
		} else if !os.IsNotExist(err) {
			return false, err
		}
		if filepath.Dir(current) == current {
			return false, nil
		}
	}
}

func externalDisjoint(a, b string, both, physical bool, held ...*externalTree) error {
	if externalContains(a, b) {
		return errors.New("external mutation paths overlap")
	}
	if physical {
		contains, err := externalPhysicalContains(a, b, held...)
		if err != nil {
			return err
		}
		if contains {
			return errors.New("external physical mutation paths overlap")
		}
	}
	if both {
		return externalDisjoint(b, a, false, physical, held...)
	}
	return nil
}

func externalPrivateParents(e *externalTree, relative string) error {
	current := ""
	for _, component := range strings.Split(relative, "/") {
		if component == "." {
			continue
		}
		current = path.Join(current, component)
		info, err := e.stat(current)
		if os.IsNotExist(err) {
			return nil
		} else if err != nil {
			return err
		}
		if err := externalDirectoryInfo(info, true); err != nil {
			return err
		}
	}
	return nil
}

func externalProfileControls(userHome, attempt string) error {
	e, err := externalOpen(userHome)
	if err != nil {
		return err
	}
	defer e.close()
	names := []string{externalIntentControl, externalIntentControl + "/.locator-staging", externalIntentControl + "/retired/.staging"}
	if attempt != "" {
		names = append(names, path.Join(externalIntentControl, attempt, ".staging"))
	}
	for _, name := range names {
		if err := externalPrivateParents(e, name); err != nil {
			return err
		}
	}
	return nil
}

// Fixed profile-control spellings are portable reservations even when both
// leaves are absent. Arbitrary filesystem root names retain native semantics.
func externalReservedProfileWrite(userHome, write string, held ...*externalTree) error {
	profile, err := externalPathInfo(userHome)
	if err != nil {
		return err
	}
	var suffix []string
	for current := write; ; current = filepath.Dir(current) {
		info, err := externalPathInfo(current, held...)
		if err == nil {
			if info.Mode()&os.ModeSymlink != 0 {
				return errors.New("external profile ancestor is a symlink")
			}
			if os.SameFile(profile, info) {
				if len(suffix) == 0 {
					return errors.New("external write encloses profile controls")
				}
				key, err := collisionKey(suffix[len(suffix)-1])
				if err != nil {
					return err
				}
				if key == externalIntentControl || key == ".maestro-home.json" {
					return errors.New("external write uses reserved portable profile-control name")
				}
				return nil
			}
		} else if !os.IsNotExist(err) {
			return err
		}
		if filepath.Dir(current) == current {
			return nil
		}
		suffix = append(suffix, filepath.Base(current))
	}
}

func externalRoots(o ExternalOptions, original bool, held ...*externalTree) error {
	for _, p := range []string{o.SourceRoot, o.ProductRoot, o.UserHome, o.Home} {
		if err := externalClean(p); err != nil {
			return err
		}
	}
	if err := externalDisjoint(o.Home, o.UserHome, false, true, held...); err != nil {
		return err
	}
	if err := externalReservedProfileWrite(o.UserHome, o.Home, held...); err != nil {
		return err
	}
	for _, control := range []string{filepath.Join(o.UserHome, ".maestro-home.json"), filepath.Join(o.UserHome, externalIntentControl)} {
		if err := externalDisjoint(o.Home, control, true, true, held...); err != nil {
			return err
		}
	}
	for _, p := range []string{o.SourceRoot, o.ProductRoot} {
		if err := externalDisjoint(p, o.UserHome, false, original, held...); err != nil {
			return err
		}
		for _, write := range []string{o.Home, filepath.Join(o.UserHome, ".maestro-home.json"), filepath.Join(o.UserHome, externalIntentControl)} {
			if err := externalDisjoint(p, write, true, original, held...); err != nil {
				return err
			}
		}
	}
	if err := externalProfileControls(o.UserHome, ""); err != nil {
		return err
	}
	for _, p := range []string{o.UserHome, filepath.Dir(o.Home)} {
		if err := externalOwnedDirectory(p, false); err != nil {
			return err
		}
	}
	if original {
		for _, p := range []string{o.UserHome, filepath.Dir(o.Home)} {
			if err := externalWritableDirectory(p); err != nil {
				return err
			}
		}
		for _, p := range []string{o.SourceRoot, o.ProductRoot} {
			if err := scheduler.ValidateDirectory(p); err != nil {
				return err
			}
		}
	}
	return nil
}

func externalRead(e *externalTree, relative string, private bool) ([]byte, error) {
	maximum := int64(externalMaximum)
	if !private {
		maximum = 100 << 20
	}
	return externalReadLimit(e, relative, private, maximum)
}

func externalDecode(b []byte, out any) error {
	if len(b) > externalMaximum || !utf8.Valid(b) {
		return errors.New("invalid external JSON")
	}
	d := json.NewDecoder(bytes.NewReader(b))
	if err := externalUnique(d); err != nil {
		return err
	}
	if _, err := d.Token(); err != io.EOF {
		return errors.New("trailing external JSON")
	}
	d = json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if err := d.Decode(out); err != nil {
		return err
	}
	var actual, expected any
	if err := json.Unmarshal(b, &actual); err != nil {
		return err
	}
	normalized, err := json.Marshal(out)
	if err != nil {
		return err
	}
	if err = json.Unmarshal(normalized, &expected); err != nil {
		return err
	}
	return externalRequiredShape(actual, expected)
}
func externalRequiredShape(actual, expected any) error {
	if actual == nil {
		return errors.New("null external record field")
	}
	switch shape := expected.(type) {
	case map[string]any:
		object, ok := actual.(map[string]any)
		if !ok || len(object) != len(shape) {
			return errors.New("missing external record field")
		}
		for key, value := range shape {
			child, ok := object[key]
			if !ok {
				return errors.New("missing external record field")
			}
			if err := externalRequiredShape(child, value); err != nil {
				return err
			}
		}
	case []any:
		array, ok := actual.([]any)
		if !ok || len(array) != len(shape) {
			return errors.New("invalid external record array")
		}
		for i, value := range shape {
			if err := externalRequiredShape(array[i], value); err != nil {
				return err
			}
		}
	}
	return nil
}
func externalUnique(d *json.Decoder) error {
	t, err := d.Token()
	if err != nil {
		return err
	}
	delim, ok := t.(json.Delim)
	if !ok {
		return nil
	}
	if delim == '{' {
		seen := map[string]bool{}
		for d.More() {
			k, err := d.Token()
			if err != nil {
				return err
			}
			key, ok := k.(string)
			if !ok || seen[key] {
				return errors.New("duplicate external JSON key")
			}
			seen[key] = true
			if err = externalUnique(d); err != nil {
				return err
			}
		}
	} else if delim == '[' {
		for d.More() {
			if err := externalUnique(d); err != nil {
				return err
			}
		}
	} else {
		return errors.New("invalid external JSON delimiter")
	}
	_, err = d.Token()
	return err
}

func externalInventory(e *externalTree, tree string) ([]record, error) {
	if err := e.check(tree); err != nil {
		return nil, err
	}
	info, err := e.stat(tree)
	if os.IsNotExist(err) {
		return []record{}, nil
	}
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, errors.New("source tree is not a directory")
	}
	var records []record
	seen := map[string]bool{}
	err = e.walk(tree, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if p == tree {
			return nil
		}
		rel := strings.TrimPrefix(p, tree+"/")
		key, err := collisionKey(rel)
		if err != nil {
			return err
		}
		if seen[key] {
			return errors.New("portable source collision")
		}
		seen[key] = true
		info, err := d.Info()
		if err != nil {
			return err
		}
		r := record{Path: p, Kind: "directory"}
		if !info.IsDir() {
			b, err := externalRead(e, p, false)
			if err != nil {
				return err
			}
			r.Kind = "file"
			r.SHA256 = hash(b)
		}
		records = append(records, r)
		return nil
	})
	return records, err
}

func externalSource(source string) (string, string, []record, error) {
	e, err := externalOpenSource(source)
	if err != nil {
		return "", "", nil, err
	}
	defer e.close()
	return externalSourceTree(e)
}

func externalSourceTree(e *externalTree) (string, string, []record, error) {
	b, err := externalRead(e, "VERSION", false)
	if err != nil {
		return "", "", nil, err
	}
	version := strings.TrimSpace(string(b))
	data, err := externalInventory(e, "data")
	if err != nil {
		return "", "", nil, err
	}
	brain, err := externalInventory(e, "brain")
	if err != nil {
		return "", "", nil, err
	}
	layout := ""
	records := data
	switch {
	case version == "0.1.11" || version == "0.1.12":
		if len(data) == 0 || len(brain) != 0 {
			return "", "", nil, errors.New("ambiguous legacy layout")
		}
		layout = "legacy-data"
	case strings.HasPrefix(version, "0.2."):
		if len(brain) == 0 {
			return "", "", nil, errors.New("missing live brain")
		}
		layout = "brain"
		records = brain
		if len(data) != 0 {
			if err := externalV1Committed(e, data, brain); err != nil {
				return "", "", nil, fmt.Errorf("ambiguous dual source: %w", err)
			}
			layout = "brain-v1"
			records = append(records, data...)
		}
	default:
		return "", "", nil, errors.New("unsupported source version")
	}
	sort.Slice(records, func(i, j int) bool { return records[i].Path < records[j].Path })
	return version, layout, records, nil
}

func externalMachine(relative string) bool {
	// Reserve the same portable identity used for collision checks. Otherwise an
	// uppercase control can become an active lowercase reader on APFS/Windows.
	folded, err := collisionKey(relative)
	if err != nil {
		return true
	}
	relative = folded
	for _, component := range strings.Split(relative, "/") {
		if component == ".attestation" || component == ".maestro" || component == ".codex" || component == ".claude" {
			return true
		}
	}
	parts := strings.Split(relative, "/")
	if len(parts) > 4 && parts[0] == "accounts" && parts[2] == "cases" {
		return externalMachine(strings.Join(parts[4:], "/"))
	}
	// Owner enrollment hashes, reviewed drafts, auto-approved proposals and
	// confirmed observation evidence cannot authorize the relocated brain.
	for _, prefix := range []string{".maestro", "owner/maestro", "owner/interview", "owner/refinement", "maintenance", "workspaces", "updates", "runtime", ".codex", ".claude"} {
		if relative == prefix || strings.HasPrefix(relative, prefix+"/") {
			return true
		}
	}
	for _, name := range []string{".initialized", ".maestro-version", ".maestro-python", ".scaffold.log", ".upgrade-pending", "first-run-failed.txt", "memory/.dream-requested", "memory/.dream-ack", "memory/.dream-legacy-request", "owner/registry.json", "owner/observations/observations.jsonl", "owner/.eod-requested", "owner/.no-account-ack"} {
		if relative == name {
			return true
		}
	}
	return strings.HasPrefix(relative, ".recovered-")
}

func PlanExternal(o ExternalOptions) (ExternalPlan, error) {
	if err := externalRoots(o, true); err != nil {
		return ExternalPlan{}, err
	}
	if _, err := externalPathInfo(filepath.Join(o.UserHome, ".maestro-home.json")); err == nil {
		return ExternalPlan{}, errors.New("existing locator requires inspection, not a new migration")
	} else if !os.IsNotExist(err) {
		return ExternalPlan{}, err
	}
	p := ExternalPlan{SchemaVersion: 1, AttemptID: id(), ExternalOptions: o, Entries: []ExternalEntry{}}
	p.ArchiveRoot = filepath.Join(filepath.Dir(o.Home), ".maestro-rollback-"+p.AttemptID)
	if info, err := externalPathInfo(o.Home); err == nil {
		if !info.IsDir() {
			return p, errors.New("home is not an empty directory")
		}
		if err = externalOwnedDirectory(o.Home, true); err != nil {
			return p, err
		}
		entries, err := externalPathEntries(o.Home)
		if err != nil {
			return p, err
		}
		if len(entries) != 0 {
			return p, errors.New("home already populated")
		}
		p.PriorHome = true
	} else if !os.IsNotExist(err) {
		return p, err
	}
	version, layout, records, err := externalSource(o.SourceRoot)
	if err != nil {
		return p, err
	}
	p.SourceVersion, p.SourceLayout, p.SourceFingerprint = version, layout, fingerprint(records)
	e, err := externalOpenSource(o.SourceRoot)
	if err != nil {
		return p, err
	}
	defer e.close()
	for _, r := range records {
		entry := ExternalEntry{Source: r.Path, Kind: r.Kind, SHA256: r.SHA256, TargetSHA256: r.SHA256, Disposition: "live_author"}
		entry.Target, entry.Disposition, entry.Transform = externalMapping(p.SourceLayout, p.AttemptID, r.Path, r.Kind)
		if entry.Transform != "" {
			legacyEntry := Entry{Source: strings.TrimPrefix(r.Path, "data/"), SHA256: r.SHA256, Transform: entry.Transform}
			body, err := externalOutput(e, legacyEntry)
			if err != nil {
				return p, err
			}
			entry.TargetSHA256 = hash(body)
		}
		p.Entries = append(p.Entries, entry)
	}
	if err = externalValidatePlan(p); err != nil {
		return p, err
	}
	if err = externalArchiveRoots(p, true); err != nil {
		return p, err
	}
	return p, nil
}

func externalMapping(layout, attempt, source, kind string) (target, disposition, transform string) {
	disposition = "live_author"
	relative := strings.TrimPrefix(source, "brain/")
	if strings.HasPrefix(source, "data/") {
		relative = strings.TrimPrefix(source, "data/")
		mapped := targetFor(relative)
		if layout != "legacy-data" || mapped == "" {
			disposition = "preserved_inactive"
		} else {
			relative = mapped
		}
		if layout == "legacy-data" && (source == "data/cases/.active" || source == "data/cases/.pending") && kind == "file" {
			transform = "legacy_case_marker"
		}
	}
	if externalMachine(relative) {
		disposition = "preserved_inactive"
	}
	if disposition == "live_author" {
		target = path.Join("brain", relative)
	} else {
		target = path.Join("lineage", attempt, source)
	}
	return
}

func externalValidatePlan(p ExternalPlan, held ...*externalTree) error {
	if p.SchemaVersion != 1 || !externalAttempt.MatchString(p.AttemptID) || !externalDigestPattern.MatchString(p.SourceFingerprint) {
		return errors.New("invalid external plan identity")
	}
	if err := externalRoots(p.ExternalOptions, false, held...); err != nil {
		return err
	}
	if err := externalProfileControls(p.UserHome, p.AttemptID); err != nil {
		return err
	}
	if err := externalArchiveRoots(p, false, held...); err != nil {
		return err
	}
	if len(p.Entries) == 0 {
		return errors.New("empty external source plan")
	}
	if p.SourceLayout == "legacy-data" {
		if p.SourceVersion != "0.1.11" && p.SourceVersion != "0.1.12" {
			return errors.New("unsupported legacy version")
		}
	} else if p.SourceLayout != "brain" && p.SourceLayout != "brain-v1" {
		return errors.New("unsupported source layout")
	} else if !regexp.MustCompile(`^0\.2\.[0-9]+$`).MatchString(p.SourceVersion) {
		return errors.New("unsupported brain version")
	}
	sourceRecords := []record{}
	seen := map[string]string{}
	for _, entry := range p.Entries {
		expectedTarget, expectedDisposition, expectedTransform := externalMapping(p.SourceLayout, p.AttemptID, entry.Source, entry.Kind)
		if entry.Target != expectedTarget || entry.Disposition != expectedDisposition || entry.Transform != expectedTransform {
			return errors.New("noncanonical external source mapping")
		}
		if entry.Kind == "directory" && (entry.SHA256 != "" || entry.TargetSHA256 != "" || entry.Transform != "") {
			return errors.New("directory contains file authority")
		}
		if entry.Transform == "" && entry.SHA256 != entry.TargetSHA256 {
			return errors.New("untransformed target digest mismatch")
		}
		sourceRecords = append(sourceRecords, record{entry.Source, entry.Kind, entry.SHA256})

		if _, err := collisionKey(entry.Source); err != nil {
			return err
		}
		key, err := collisionKey(entry.Target)
		if err != nil {
			return err
		}
		if !strings.HasPrefix(entry.Source, "data/") && !strings.HasPrefix(entry.Source, "brain/") {
			return errors.New("invalid source entry")
		}
		if entry.Kind != "file" && entry.Kind != "directory" {
			return errors.New("invalid entry kind")
		}
		if entry.Kind == "file" && (!externalDigestPattern.MatchString(entry.SHA256) || !externalDigestPattern.MatchString(entry.TargetSHA256)) {
			return errors.New("invalid entry digest")
		}
		if entry.Disposition == "live_author" {
			if !strings.HasPrefix(entry.Target, "brain/") || externalMachine(strings.TrimPrefix(entry.Target, "brain/")) {
				return errors.New("invalid live target")
			}
		} else if entry.Disposition != "preserved_inactive" || entry.Target != path.Join("lineage", p.AttemptID, entry.Source) {
			return errors.New("invalid inactive target")
		}
		if entry.Transform != "" && entry.Transform != "legacy_case_marker" {
			return errors.New("unknown transform")
		}
		if kind, ok := seen[key]; ok && (kind != "directory" || entry.Kind != "directory") {
			return errors.New("external target collision")
		}
		seen[key] = entry.Kind
	}
	if fingerprint(sourceRecords) != p.SourceFingerprint {
		return errors.New("source inventory omitted or duplicated")
	}
	for k, kind := range seen {
		if kind == "file" {
			for other := range seen {
				if strings.HasPrefix(other, k+"/") {
					return errors.New("external ancestor collision")
				}
			}
		}
	}
	return nil
}

func externalArchiveRoots(p ExternalPlan, original bool, held ...*externalTree) error {
	if p.ArchiveRoot != filepath.Join(filepath.Dir(p.Home), ".maestro-rollback-"+p.AttemptID) {
		return errors.New("invalid rollback archive")
	}
	if err := externalReservedProfileWrite(p.UserHome, p.ArchiveRoot, held...); err != nil {
		return err
	}
	for index, r := range []string{p.SourceRoot, p.ProductRoot, p.UserHome, p.Home} {
		if err := externalDisjoint(p.ArchiveRoot, r, r != p.UserHome, original || index >= 2, held...); err != nil {
			return err
		}
	}
	for _, control := range []string{filepath.Join(p.UserHome, ".maestro-home.json"), filepath.Join(p.UserHome, externalIntentControl)} {
		if err := externalDisjoint(p.ArchiveRoot, control, true, true, held...); err != nil {
			return err
		}
	}
	return nil
}

func externalName(attempt, name string) string { return path.Join(externalControl, attempt, name) }
func externalCanonical(value any) []byte {
	b, _ := json.MarshalIndent(value, "", "  ")
	return append(b, '\n')
}
func externalTemporary(name string) string {
	if name == ".maestro-home.json" {
		return path.Join(externalIntentControl, ".locator-staging", hash([]byte(name))+".tmp")
	}
	if strings.HasPrefix(name, externalIntentControl+"/") {
		parts := strings.Split(name, "/")
		return path.Join(parts[0], parts[1], ".staging", hash([]byte(name))+".tmp")
	}
	return path.Join(externalControl, ".staging", hash([]byte(name))+".tmp")
}

func externalIntentName(attempt string) string {
	return path.Join(externalIntentControl, attempt, "intent.json")
}
func externalResumeTree(e *externalTree, p ExternalPlan) error {
	allowed := map[string]bool{".": true, "state": true}
	add := func(name string, directory bool) {
		allowed[name] = directory
		for parent := path.Dir(name); parent != "."; parent = path.Dir(parent) {
			allowed[parent] = true
		}
	}
	names := []string{externalName(p.AttemptID, "plan.json"), externalName(p.AttemptID, "prepared.json"), externalName(p.AttemptID, "copied.json"), externalName(p.AttemptID, "commit.json"), "brain/.maestro/origin.json"}
	for _, entry := range p.Entries {
		add(entry.Target, entry.Kind == "directory")
		if entry.Kind == "file" {
			names = append(names, entry.Target)
		}
	}
	for _, name := range names {
		add(name, false)
		add(externalTemporary(name), false)
	}
	return e.walk(".", func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		directory, ok := allowed[name]
		if !ok {
			return errors.New("unowned partial home content")
		}
		if err = e.check(name); err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if directory {
			if !info.IsDir() {
				return errors.New("partial directory obstruction")
			}
			return nil
		}
		return externalRegular(info, true)
	})
}
func externalProgress(progress func(ExternalProgress) error, phase string, copied, total int) error {
	if progress != nil {
		return progress(ExternalProgress{phase, copied, total})
	}
	return nil
}

func ApplyExternal(p ExternalPlan, digest string, progress func(ExternalProgress) error) (ExternalStatus, error) {
	s := externalStatus(p, digest, "blocked")
	if err := externalMutationAllowed(); err != nil {
		return s, err
	}
	if err := userlevel.EnsureNotElevated(); err != nil {
		return s, err
	}
	b, err := p.Bytes()
	if err != nil {
		return s, err
	}
	if hash(b) != digest {
		return s, errors.New("reviewed plan digest mismatch")
	}
	if err = externalValidatePlan(p); err != nil {
		return s, err
	}
	if current, err := InspectExternal(p.Home, p.AttemptID, digest); err == nil && current.Current {
		current.State = "already_migrated"
		return current, nil
	}
	if err = externalRoots(p.ExternalOptions, true); err != nil {
		return s, err
	}
	if err = externalArchiveRoots(p, true); err != nil {
		return s, err
	}
	source, err := externalOpenSource(p.SourceRoot)
	if err != nil {
		return s, err
	}
	defer source.close()
	product, err := externalOpenSource(p.ProductRoot)
	if err != nil {
		return s, err
	}
	defer product.close()
	if err = externalRoots(p.ExternalOptions, true, source, product); err != nil {
		return s, err
	}
	version, layout, records, err := externalSourceTree(source)
	if err != nil {
		return s, err
	}
	if version != p.SourceVersion || layout != p.SourceLayout || fingerprint(records) != p.SourceFingerprint {
		return s, errors.New("source drift")
	}
	if _, err = externalPathInfo(p.ArchiveRoot); !os.IsNotExist(err) {
		return s, errors.New("rollback archive already exists")
	}
	u, err := externalOpen(p.UserHome)
	if err != nil {
		return s, err
	}
	defer u.close()
	intent, intentErr := externalRead(u, externalIntentName(p.AttemptID), true)
	if intentErr == nil && !bytes.Equal(intent, b) {
		return s, errors.New("intent does not match reviewed plan")
	}
	if intentErr != nil && !os.IsNotExist(intentErr) {
		return s, intentErr
	}
	if os.IsNotExist(intentErr) {
		if info, er := externalPathInfo(p.Home); er == nil {
			if !info.IsDir() {
				return s, errors.New("home is not empty")
			}
			entries, er := externalPathEntries(p.Home)
			if er != nil || len(entries) != 0 {
				return s, errors.New("unowned populated home")
			}
		} else if !os.IsNotExist(er) {
			return s, er
		}
		if _, er := u.stat(".maestro-home.json"); !os.IsNotExist(er) {
			return s, errors.New("foreign locator")
		}
	}
	release, err := externalAcquire(u, p.AttemptID)
	if err != nil {
		return s, err
	}
	defer release()
	if err = externalLocatorOwned(u, p); err != nil {
		return s, err
	}
	if err = externalEnsure(u, externalIntentName(p.AttemptID), b); err != nil {
		return s, err
	}
	if _, err = externalPathInfo(p.Home); os.IsNotExist(err) {
		if err = externalCreateHome(p.Home); err != nil {
			return s, err
		}
	} else if err != nil {
		return s, err
	}
	if err = externalOwnedDirectory(p.Home, true); err != nil {
		return s, err
	}
	e, err := externalOpen(p.Home)
	if err != nil {
		return s, err
	}
	defer e.close()
	planName := externalName(p.AttemptID, "plan.json")
	if err = externalResumeTree(e, p); err != nil {
		return s, err
	}
	if err = externalEnsure(e, planName, b); err != nil {
		return s, err
	}
	if err = externalEnsure(e, externalName(p.AttemptID, "prepared.json"), externalCanonical(externalRecord{1, p.AttemptID, digest, "prepared"})); err != nil {
		return s, err
	}
	if err = externalProgress(progress, "prepared", 0, len(p.Entries)); err != nil {
		return s, err
	}
	origin := externalOrigin{1, p.Home, "migrated", p.AttemptID, digest}
	locator := externalLocator{1, p.Home, p.AttemptID}
	if err = externalEnsure(e, "brain/.maestro/origin.json", externalCanonical(origin)); err != nil {
		return s, err
	}
	if err = externalEnsure(u, ".maestro-home.json", externalCanonical(locator)); err != nil {
		return s, err
	}
	if err = externalProgress(progress, "pending", 0, len(p.Entries)); err != nil {
		return s, err
	}
	for i, entry := range p.Entries {
		if err = e.check(entry.Target); err != nil {
			return s, err
		}
		if entry.Kind == "directory" {
			if err = e.mkdir(entry.Target); err != nil {
				return s, err
			}
		} else {
			body, er := externalRead(source, entry.Source, false)
			if er != nil {
				return s, er
			}
			if hash(body) != entry.SHA256 {
				return s, errors.New("source entry drift")
			}
			if entry.Transform == "legacy_case_marker" {
				body, er = externalTransform(source, Entry{Source: strings.TrimPrefix(entry.Source, "data/"), SHA256: entry.SHA256, Transform: entry.Transform}, body)
				if er != nil {
					return s, er
				}
			}
			if hash(body) != entry.TargetSHA256 {
				return s, errors.New("target transform mismatch")
			}
			if err = externalEnsure(e, entry.Target, body); err != nil {
				return s, err
			}
		}
		if err = externalProgress(progress, "copying", i+1, len(p.Entries)); err != nil {
			return s, err
		}
	}
	if err = e.mkdir("state"); err != nil {
		return s, err
	}
	_, _, records, err = externalSourceTree(source)
	if err != nil {
		return s, err
	}
	if fingerprint(records) != p.SourceFingerprint {
		return s, errors.New("source changed before activation")
	}
	if err = externalEnsure(e, externalName(p.AttemptID, "copied.json"), externalCanonical(externalRecord{1, p.AttemptID, digest, "copied"})); err != nil {
		return s, err
	}
	if err = externalProgress(progress, "copied", len(p.Entries), len(p.Entries)); err != nil {
		return s, err
	}
	if err = externalEnsure(e, externalName(p.AttemptID, "commit.json"), externalCanonical(externalRecord{1, p.AttemptID, digest, "committed"})); err != nil {
		return s, err
	}
	s, err = InspectExternal(p.Home, p.AttemptID, digest)
	if err != nil {
		return s, err
	}
	return s, externalProgress(progress, "committed", len(p.Entries), len(p.Entries))
}

func externalLoad(home, attempt, digest string) (ExternalPlan, *externalTree, error) {
	return externalLoadTree(home, attempt, digest, false)
}
func externalLoadTree(home, attempt, digest string, movable bool) (ExternalPlan, *externalTree, error) {
	var p ExternalPlan
	if !externalAttempt.MatchString(attempt) || !externalDigestPattern.MatchString(digest) {
		return p, nil, errors.New("invalid external binding")
	}
	if err := externalOwnedDirectory(home, true); err != nil {
		return p, nil, err
	}
	open := externalOpen
	if movable {
		open = externalOpenMovable
	}
	e, err := open(home)
	if err != nil {
		return p, nil, err
	}
	b, err := externalRead(e, externalName(attempt, "plan.json"), true)
	if err == nil && hash(b) != digest {
		err = errors.New("external plan digest mismatch")
	}
	if err == nil {
		err = externalDecode(b, &p)
	}
	if err == nil {
		err = externalValidatePlan(p, e)
	}
	if err == nil && (p.AttemptID != attempt || p.Home != home) {
		err = errors.New("external plan binding mismatch")
	}
	if err != nil {
		e.close()
		return p, nil, err
	}
	return p, e, nil
}
func externalCheckRecord(e *externalTree, p ExternalPlan, digest, name, phase string) error {
	b, err := externalRead(e, externalName(p.AttemptID, name), true)
	if err != nil {
		return err
	}
	var r externalRecord
	if err = externalDecode(b, &r); err != nil {
		return err
	}
	if r != (externalRecord{1, p.AttemptID, digest, phase}) {
		return errors.New("external record mismatch")
	}
	return nil
}

func InspectExternal(home, attempt, digest string) (ExternalStatus, error) {
	s := ExternalStatus{State: "unverified", Diagnostic: "migration-unverified"}
	p, e, err := externalLoad(home, attempt, digest)
	archived := false
	if os.IsNotExist(err) {
		archive := filepath.Join(filepath.Dir(home), ".maestro-rollback-"+attempt)
		p, e, err = externalLoadArchived(home, archive, attempt, digest)
		archived = err == nil
	}
	if err != nil {
		return s, err
	}
	defer e.close()
	s = externalStatusAt(p, digest, "pending", e.project)
	if _, err = externalRead(e, externalName(attempt, "revoked.json"), true); err == nil {
		s.State = "revoked"
		s.Diagnostic = "migration-revoked"
		if archived {
			if err = externalCheckRecord(e, p, digest, "rollback.json", "rolled_back"); err == nil {
				s.State = "rolled_back"
				s.Diagnostic = "legacy-handoff-ready"
			} else if !os.IsNotExist(err) {
				return s, err
			}
		}
		return s, nil
	} else if !os.IsNotExist(err) {
		return s, err
	}
	for _, record := range []struct{ name, phase string }{{"prepared.json", "prepared"}, {"copied.json", "copied"}, {"commit.json", "committed"}} {
		if err = externalCheckRecord(e, p, digest, record.name, record.phase); err != nil {
			return s, err
		}
	}
	b, err := externalRead(e, "brain/.maestro/origin.json", true)
	if err != nil {
		return s, err
	}
	var origin externalOrigin
	if err = externalDecode(b, &origin); err != nil {
		return s, err
	}
	if origin != (externalOrigin{1, p.Home, "migrated", attempt, digest}) {
		return s, errors.New("external origin mismatch")
	}
	u, err := externalOpen(p.UserHome)
	if err != nil {
		return s, err
	}
	defer u.close()
	b, err = externalRead(u, ".maestro-home.json", true)
	if err != nil {
		return s, err
	}
	var locator externalLocator
	if err = externalDecode(b, &locator); err != nil {
		return s, err
	}
	if locator != (externalLocator{1, p.Home, attempt}) {
		return s, errors.New("external locator mismatch")
	}
	if err = externalOwnedDirectory(filepath.Join(home, "brain"), true); err != nil {
		return s, err
	}
	if err = externalOwnedDirectory(filepath.Join(home, "state"), true); err != nil {
		return s, err
	}
	s.Current = true
	s.State = "committed"
	s.Diagnostic = "ready"
	return s, nil
}

func RollbackExternal(home, attempt, digest string, progress func(ExternalProgress) error) (ExternalStatus, error) {
	s := ExternalStatus{State: "blocked", Diagnostic: "migration-unverified"}
	if err := externalMutationAllowed(); err != nil {
		return s, err
	}
	if err := userlevel.EnsureNotElevated(); err != nil {
		return s, err
	}
	p, e, err := externalLoadTree(home, attempt, digest, true)
	archive := filepath.Join(filepath.Dir(home), ".maestro-rollback-"+attempt)
	archived := false
	if os.IsNotExist(err) {
		p, e, err = externalLoadArchived(home, archive, attempt, digest)
		archived = err == nil
	}
	if err != nil {
		return s, err
	}
	defer e.close()
	if !archived {
		if err = externalRoots(p.ExternalOptions, true, e); err != nil {
			return s, err
		}
		if err = externalArchiveRoots(p, true, e); err != nil {
			return s, err
		}
	}
	for _, parent := range []string{p.UserHome, filepath.Dir(p.Home)} {
		if err = externalWritableDirectory(parent); err != nil {
			return s, err
		}
	}
	guard, guardErr := externalOpen(p.UserHome)
	if guardErr != nil {
		return s, guardErr
	}
	defer guard.close()
	release, guardErr := externalAcquire(guard, attempt)
	if guardErr != nil {
		return s, guardErr
	}
	defer release()
	if err = externalLocatorOwned(guard, p); err != nil {
		return s, err
	}
	if err = externalRetirementOwned(guard, p, digest); err != nil {
		return s, err
	}
	s = externalStatusAt(p, digest, "rollback_pending", e.project)
	if !archived {
		version, layout, _, sourceErr := externalSource(p.SourceRoot)
		if sourceErr != nil || version != p.SourceVersion || layout != p.SourceLayout {
			err = sourceErr
			if err == nil {
				err = errors.New("original layout changed")
			}
			return s, fmt.Errorf("original unavailable: %w", err)
		}
		if _, err = externalPathInfo(archive); !os.IsNotExist(err) {
			return s, errors.New("rollback archive exists")
		}
		if err = externalEnsure(e, externalName(attempt, "revoked.json"), externalCanonical(externalRecord{1, attempt, digest, "revoked"})); err != nil {
			return s, err
		}
		if err = externalProgress(progress, "revoked", 0, 0); err != nil {
			return s, err
		}
		moved, moveErr := e.move(archive)
		if moved {
			s = externalStatusAt(p, digest, "rollback_pending", archive)
		}
		if moveErr != nil {
			return s, moveErr
		}
		s = externalStatusAt(p, digest, "rollback_pending", archive)
		if err = externalProgress(progress, "archived", 0, 0); err != nil {
			return s, err
		}
	}
	u, err := externalOpen(p.UserHome)
	if err != nil {
		return s, err
	}
	defer u.close()
	if err = externalPublishRetirement(u, p, digest); err != nil {
		return s, err
	}
	if err = externalProgress(progress, "retired", 0, 0); err != nil {
		return s, err
	}
	_, err = u.removeExact(".maestro-home.json", externalCanonical(externalLocator{1, p.Home, attempt}))
	if err != nil {
		return s, err
	}
	if p.PriorHome {
		if err = externalCreateHome(home); err != nil && !os.IsExist(err) {
			return s, err
		}
		entries, er := externalPathEntries(home)
		if er != nil || len(entries) != 0 {
			return s, errors.New("prior empty home conflicts")
		}
	}
	if err = externalEnsure(e, externalName(attempt, "rollback.json"), externalCanonical(externalRecord{1, attempt, digest, "rolled_back"})); err != nil {
		return s, err
	}
	s.State = "rolled_back"
	s.Diagnostic = "legacy-handoff-ready"
	return s, externalProgress(progress, "rolled_back", 0, 0)
}

func externalLoadArchived(home, archive, attempt, digest string) (ExternalPlan, *externalTree, error) {
	var p ExternalPlan
	if !externalAttempt.MatchString(attempt) || !externalDigestPattern.MatchString(digest) {
		return p, nil, errors.New("invalid rollback binding")
	}
	if err := externalOwnedDirectory(archive, true); err != nil {
		return p, nil, err
	}
	e, err := externalOpen(archive)
	if err != nil {
		return p, nil, err
	}
	b, err := externalRead(e, externalName(attempt, "plan.json"), true)
	if err == nil && hash(b) != digest {
		err = errors.New("archived plan digest mismatch")
	}
	if err == nil {
		err = externalDecode(b, &p)
	}
	if err == nil {
		err = externalValidatePlan(p, e)
	}
	if err == nil && (p.Home != home || p.ArchiveRoot != archive || p.AttemptID != attempt) {
		err = errors.New("archived binding mismatch")
	}
	if err == nil {
		err = externalCheckRecord(e, p, digest, "revoked.json", "revoked")
	}
	if err != nil {
		e.close()
		return p, nil, err
	}
	return p, e, nil
}

func externalLocatorOwned(e *externalTree, p ExternalPlan) error {
	b, err := externalRead(e, ".maestro-home.json", true)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !bytes.Equal(b, externalCanonical(externalLocator{1, p.Home, p.AttemptID})) {
		return errors.New("locator belongs to another attempt")
	}
	return nil
}

type externalRetirement struct {
	SchemaVersion int    `json:"schema_version"`
	Home          string `json:"home"`
	UserHome      string `json:"user_home"`
	AttemptID     string `json:"attempt_id"`
	PlanSHA256    string `json:"plan_sha256"`
	ArchiveRoot   string `json:"archive_root"`
}

func externalRetirementName(home string) string {
	return path.Join(externalIntentControl, "retired", hash([]byte(home))+".json")
}
func externalRetirementBody(p ExternalPlan, digest string) []byte {
	return externalCanonical(externalRetirement{1, p.Home, p.UserHome, p.AttemptID, digest, p.ArchiveRoot})
}

// RetiredExternalHome is a deny-only lookup for an unconfirmed explicit HOME.
// It never selects a source, grants authority, creates or repairs records.
func RetiredExternalHome(userHome, home string) (bool, error) {
	if err := externalClean(userHome); err != nil {
		return false, err
	}
	if err := externalClean(home); err != nil {
		return false, err
	}
	if err := externalOwnedDirectory(userHome, false); err != nil {
		return false, err
	}
	if err := externalProfileControls(userHome, ""); err != nil {
		return false, err
	}
	u, err := externalOpen(userHome)
	if err != nil {
		return false, err
	}
	defer u.close()
	body, err := externalRead(u, externalRetirementName(home), true)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var r externalRetirement
	if err = externalDecode(body, &r); err != nil {
		return false, err
	}
	if r.SchemaVersion != 1 || r.Home != home || r.UserHome != userHome || !externalAttempt.MatchString(r.AttemptID) || !externalDigestPattern.MatchString(r.PlanSHA256) || r.ArchiveRoot != filepath.Join(filepath.Dir(home), ".maestro-rollback-"+r.AttemptID) {
		return false, errors.New("retired binding mismatch")
	}
	p, e, err := externalLoadArchived(home, r.ArchiveRoot, r.AttemptID, r.PlanSHA256)
	if err != nil {
		return false, err
	}
	defer e.close()
	if p.UserHome != userHome {
		return false, errors.New("retired profile mismatch")
	}
	if err = externalCheckRecord(e, p, r.PlanSHA256, "rollback.json", "rolled_back"); err != nil {
		return false, err
	}
	return true, nil
}
func externalRetirementOwned(u *externalTree, p ExternalPlan, digest string) error {
	body, err := externalRead(u, externalRetirementName(p.Home), true)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if bytes.Equal(body, externalRetirementBody(p, digest)) {
		return nil
	}
	retired, err := RetiredExternalHome(p.UserHome, p.Home)
	if err != nil {
		return err
	}
	if !retired {
		return errors.New("foreign retirement pointer")
	}
	return nil
}
func externalPublishRetirement(u *externalTree, p ExternalPlan, digest string) error {
	if err := externalRetirementOwned(u, p, digest); err != nil {
		return err
	}
	name := externalRetirementName(p.Home)
	body := externalRetirementBody(p, digest)
	previous, err := externalRead(u, name, true)
	if os.IsNotExist(err) {
		return externalEnsure(u, name, body)
	}
	if err != nil {
		return err
	}
	if bytes.Equal(previous, body) {
		return nil
	}
	next := name + ".next-" + p.AttemptID
	if err = externalEnsure(u, next, body); err != nil {
		return err
	}
	current, err := externalRead(u, name, true)
	if err != nil || !bytes.Equal(current, previous) {
		return errors.New("retirement pointer changed")
	}
	// Cooperative metadata writers hold the profile migration lease throughout.
	return u.publishRetirementUnderLease(next, name, body)
}

func externalOutput(e *externalTree, entry Entry) ([]byte, error) {
	body, err := externalRead(e, "data/"+entry.Source, false)
	if err != nil {
		return nil, err
	}
	if hash(body) != entry.SHA256 {
		return nil, fmt.Errorf("source_drift:%s", entry.Source)
	}
	return externalTransform(e, entry, body)
}
func externalTransform(e *externalTree, entry Entry, body []byte) ([]byte, error) {
	if entry.Transform == "legacy_case_marker" {
		name := strings.TrimSpace(strings.TrimPrefix(string(body), "\ufeff"))
		if name == "" || name == "." || name == ".." || strings.ContainsAny(name, "/\\\x00") {
			return nil, fmt.Errorf("invalid_case_marker:%s", entry.Source)
		}
		info, err := e.stat("data/cases/" + name)
		if err != nil || !info.IsDir() {
			return nil, fmt.Errorf("missing_active_case:%s", entry.Source)
		}
		body = []byte("_sem-conta/" + name + "\n")
	}
	return body, nil
}
