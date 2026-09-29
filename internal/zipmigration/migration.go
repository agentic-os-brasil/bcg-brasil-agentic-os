package zipmigration

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/userlevel"
	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

const control = "brain/.maestro/migration"

var legacy = []string{"agents", "workspaces", "canary"}

type Options struct {
	DryRun, Status          bool
	Rollback, ResolveLegacy string
}
type Result struct {
	Current           bool              "json:\"current\""
	Verification      string            "json:\"verification\""
	SchemaVersion     int               "json:\"schema_version\""
	State             string            "json:\"state\""
	AttemptID         string            "json:\"attempt_id\""
	SourceFingerprint string            "json:\"source_fingerprint\""
	TargetFingerprint string            "json:\"target_fingerprint\""
	PlanSHA256        string            "json:\"plan_sha256\""
	Errors            []string          "json:\"errors\""
	LegacyMap         map[string]string "json:\"legacy_map\""
	RestoredRuntime   bool              "json:\"restored_runtime\""
	Resumable         bool              "json:\"resumable\""
	NextAction        string            "json:\"next_action,omitempty\""
	Namespace         string            "json:\"namespace,omitempty\""
	Path              string            "json:\"path,omitempty\""
}
type Entry struct {
	Source       string "json:\"source\""
	Target       string "json:\"target\""
	Kind         string "json:\"kind\""
	Action       string "json:\"action\""
	SHA256       string "json:\"sha256\""
	TargetSHA256 string "json:\"target_sha256\""
	Transform    string "json:\"transform\""
}
type Plan struct {
	SchemaVersion     int               "json:\"schema_version\""
	AttemptID         string            "json:\"attempt_id\""
	Project           string            "json:\"project\""
	SourceFingerprint string            "json:\"source_fingerprint\""
	LegacyMap         map[string]string "json:\"legacy_map\""
	Entries           []Entry           "json:\"entries\""
}
type record struct{ Path, Kind, SHA256 string }
type engine struct {
	root    *os.Root
	project string
}

func hash(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }
func id() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}
func baseResult(state string) Result {
	r := Result{SchemaVersion: 1, State: state, Errors: []string{}, LegacyMap: map[string]string{}, Verification: "unverified"}
	if state == "not_needed" {
		r.Current = true
		r.Verification = "not_needed"
	}
	return r
}
func fromPlan(state string, p Plan, sha string) Result {
	r := baseResult(state)
	r.AttemptID = p.AttemptID
	r.SourceFingerprint = p.SourceFingerprint
	r.LegacyMap = p.LegacyMap
	r.PlanSHA256 = sha
	if state == "committed" {
		r.Current = true
		r.Verification = "current"
	}
	if state == "rolled_back" {
		r.Verification = "revoked"
	}
	return r
}

func (e *engine) validateReceipt(state Result, p Plan) error {
	b, err := e.read(control + "/attempts/" + p.AttemptID + "/receipt.json")
	if err != nil {
		return err
	}
	var receipt Result
	if err = json.Unmarshal(b, &receipt); err != nil {
		return err
	}
	a, _ := json.Marshal(state)
	c, _ := json.Marshal(receipt)
	if string(a) != string(c) {
		return errors.New("receipt_mismatch")
	}
	return nil
}
func (e *engine) validateLegacy(p Plan, namespace string) error {
	relative, ok := p.LegacyMap[namespace]
	if !ok {
		return errors.New("legacy_namespace_unavailable")
	}
	actual, err := e.inventory(relative)
	if err != nil {
		return err
	}
	expected := []record{}
	for _, entry := range p.Entries {
		if strings.HasPrefix(entry.Source, namespace+"/") {
			expected = append(expected, record{strings.TrimPrefix(entry.Source, namespace+"/"), entry.Kind, entry.SHA256})
		}
	}
	if fingerprint(actual) != fingerprint(expected) {
		return errors.New("legacy_source_drift")
	}
	return nil
}
func fingerprint(records []record) string {
	lines := make([]string, 0, len(records))
	for _, r := range records {
		sha := r.SHA256
		if sha == "" {
			sha = "-"
		}
		lines = append(lines, base64.StdEncoding.EncodeToString([]byte(r.Path))+"\t"+r.Kind+"\t"+sha+"\n")
	}
	sort.Strings(lines)
	return hash([]byte(strings.Join(lines, "")))
}
func collisionKey(p string) (string, error) {
	if !fs.ValidPath(p) || strings.ContainsAny(p, "\\\x00\r\n\t") {
		return "", fmt.Errorf("unsafe_path:%s", p)
	}
	for _, part := range strings.Split(p, "/") {
		if strings.HasSuffix(part, ".") || strings.HasSuffix(part, " ") || strings.Contains(part, ":") {
			return "", fmt.Errorf("unsupported_windows_path:%s", p)
		}
		stem := strings.ToUpper(strings.Split(part, ".")[0])
		if stem == "CON" || stem == "PRN" || stem == "AUX" || stem == "NUL" || (len(stem) == 4 && (strings.HasPrefix(stem, "COM") || strings.HasPrefix(stem, "LPT")) && stem[3] >= '1' && stem[3] <= '9') {
			return "", fmt.Errorf("reserved_windows_path:%s", p)
		}
	}
	return cases.Fold().String(norm.NFC.String(p)), nil
}

// All effects use os.Root; explicit symlinks are rejected and concurrent
// replacement cannot escape the installation root.
func (e *engine) check(p string) error {
	if !fs.ValidPath(p) {
		return fmt.Errorf("unsafe_path:%s", p)
	}
	current := ""
	for _, part := range strings.Split(p, "/") {
		current = path.Join(current, part)
		info, err := e.root.Lstat(current)
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink_path:%s", p)
		}
	}
	return nil
}
func (e *engine) read(p string) ([]byte, error) {
	if err := e.check(p); err != nil {
		return nil, err
	}
	f, err := e.root.Open(p)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("not_regular_file:%s", p)
	}
	return io.ReadAll(f)
}
func (e *engine) inventory(tree string) ([]record, error) {
	if err := e.check(tree); err != nil {
		return nil, err
	}
	if _, err := e.root.Stat(tree); errors.Is(err, fs.ErrNotExist) {
		return []record{}, nil
	} else if err != nil {
		return nil, err
	}
	records := []record{}
	seen := map[string]string{}
	err := fs.WalkDir(e.root.FS(), tree, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if p == tree {
			if !d.IsDir() {
				return fmt.Errorf("tree_not_directory:%s", tree)
			}
			return nil
		}
		relative := strings.TrimPrefix(p, tree+"/")
		key, err := collisionKey(relative)
		if err != nil {
			return err
		}
		if previous, ok := seen[key]; ok {
			return fmt.Errorf("case_collision:%s:%s", previous, relative)
		}
		seen[key] = relative
		info, err := d.Info()
		if err != nil {
			return err
		}
		kind := "file"
		sha := ""
		if info.IsDir() {
			kind = "directory"
		} else if info.Mode().IsRegular() {
			b, err := e.read(p)
			if err != nil {
				return err
			}
			sha = hash(b)
		} else {
			return fmt.Errorf("unsupported_file_type:%s", relative)
		}
		records = append(records, record{relative, kind, sha})
		return nil
	})
	return records, err
}
func targetFor(relative string) string {
	p := strings.Split(relative, "/")
	head := p[0]
	for _, n := range legacy {
		if head == n {
			return ""
		}
	}
	if relative == "README.md" {
		return ""
	}
	target := relative
	switch {
	case head == "profile":
		target = path.Join(append([]string{"owner"}, p[1:]...)...)
	case len(p) >= 2 && head == "owner" && p[1] == "atlas":
		if len(p) == 2 {
			return ""
		}
		known := false
		for _, n := range []string{"daily", "craft", "learnings", "people", "development"} {
			if p[2] == n {
				known = true
			}
		}
		if known {
			target = path.Join(p[2:]...)
		} else {
			target = path.Join(append([]string{"owner"}, p[2:]...)...)
		}
	case head == "accounts" && len(p) == 3 && p[2] == "account.md":
		target = path.Join("accounts", p[1], p[1]+".md")
	case head == "accounts" && len(p) >= 5 && p[2] == "cases" && p[4] == "brain":
		rest := p[5:]
		if len(rest) == 2 && rest[0] == "projects" && rest[1] == p[3]+".md" {
			rest = rest[1:]
		}
		target = path.Join(append(append([]string{}, p[:4]...), rest...)...)
	case head == "cases":
		if len(p) == 1 {
			target = "accounts/_sem-conta/cases"
		} else if len(p) == 2 && (p[1] == ".active" || p[1] == ".pending") {
			target = "accounts/" + p[1]
		} else {
			rest := p[2:]
			if len(rest) > 0 && rest[0] == "brain" {
				rest = rest[1:]
			}
			target = path.Join(append([]string{"accounts", "_sem-conta", "cases", p[1]}, rest...)...)
		}
	}
	switch target {
	case "craft/index.md":
		target = "craft/craft.md"
	case "learnings/index.md":
		target = "learnings/learnings.md"
	case "people/index.md":
		target = "people/people.md"
	}
	if target == ".maestro" || strings.HasPrefix(target, ".maestro/") || target == ".initialized" || target == ".scaffold.log" {
		return ""
	}
	return target
}
func (e *engine) output(entry Entry) ([]byte, error) {
	b, err := e.read("data/" + entry.Source)
	if err != nil {
		return nil, err
	}
	if hash(b) != entry.SHA256 {
		return nil, fmt.Errorf("source_drift:%s", entry.Source)
	}
	if entry.Transform == "legacy_case_marker" {
		// PowerShell 5.1 emits UTF-8 BOM by default. Normalize the logical marker,
		// never the source bytes represented by the inventory and source digest.
		name := strings.TrimSpace(strings.TrimPrefix(string(b), "\ufeff"))
		if name == "" || name == "." || name == ".." || strings.ContainsAny(name, "/\\\x00") {
			return nil, fmt.Errorf("invalid_case_marker:%s", entry.Source)
		}
		if err := e.check("data/cases/" + name); err != nil {
			return nil, err
		}
		info, err := e.root.Stat("data/cases/" + name)
		if err != nil || !info.IsDir() {
			return nil, fmt.Errorf("missing_active_case:%s", entry.Source)
		}
		b = []byte("_sem-conta/" + name + "\n")
	}
	return b, nil
}
func (e *engine) plan(records []record) (Plan, error) {
	p := Plan{SchemaVersion: 1, AttemptID: id(), Project: e.project, SourceFingerprint: fingerprint(records), LegacyMap: map[string]string{}, Entries: []Entry{}}
	targets := map[string]Entry{}
	for _, r := range records {
		target := targetFor(r.Path)
		action := "copy"
		if target == "" {
			action = "retain"
		}
		entry := Entry{Source: r.Path, Target: target, Kind: r.Kind, Action: action, SHA256: r.SHA256, TargetSHA256: r.SHA256}
		if (r.Path == "cases/.active" || r.Path == "cases/.pending") && r.Kind == "file" {
			entry.Transform = "legacy_case_marker"
			b, err := e.output(entry)
			if err != nil {
				return p, err
			}
			entry.TargetSHA256 = hash(b)
		}
		if target != "" {
			key, err := collisionKey(target)
			if err != nil {
				return p, err
			}
			if previous, exists := targets[key]; exists && (r.Kind != "directory" || previous.Kind != "directory") {
				return p, fmt.Errorf("target_collision:%s", target)
			}
			targets[key] = entry
		}
		p.Entries = append(p.Entries, entry)
	}
	for key, entry := range targets {
		if entry.Kind == "file" {
			for other := range targets {
				if strings.HasPrefix(other, key+"/") {
					return p, fmt.Errorf("file_directory_collision:%s", entry.Target)
				}
			}
		}
	}
	for _, name := range legacy {
		if info, err := e.root.Stat("data/" + name); err == nil && info.IsDir() {
			p.LegacyMap[name] = "data/" + name
		}
	}
	return p, nil
}
func (e *engine) write(p string, value any, exclusive bool) (string, error) {
	if err := e.check(p); err != nil {
		return "", err
	}
	b, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return "", err
	}
	b = append(b, '\n')
	if err = e.root.MkdirAll(path.Dir(p), 0700); err != nil {
		return "", err
	}
	name := p
	if !exclusive {
		name = p + "." + id() + ".tmp"
	}
	f, err := e.root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return "", err
	}
	_, err = f.Write(b)
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return "", err
	}
	if !exclusive {
		err = e.root.Rename(name, p)
	}
	return hash(b), err
}
func (e *engine) validateTargets(p Plan, partial bool) (string, error) {
	if partial {
		allowed := map[string]bool{}
		for _, entry := range p.Entries {
			if entry.Action != "copy" {
				continue
			}
			for name := entry.Target; name != "."; name = path.Dir(name) {
				allowed[name] = true
			}
		}
		existing, err := e.inventory("brain")
		if err != nil {
			return "", err
		}
		for _, item := range existing {
			if item.Path == ".maestro" || item.Path == ".maestro/migration" || strings.HasPrefix(item.Path, ".maestro/migration/") {
				continue
			}
			if !allowed[item.Path] {
				return "", fmt.Errorf("unowned_target_content:%s", item.Path)
			}
		}
	}
	targets := map[string]record{}
	for _, entry := range p.Entries {
		if entry.Action != "copy" {
			continue
		}
		name := "brain/" + entry.Target
		if err := e.check(name); err != nil {
			return "", err
		}
		info, err := e.root.Stat(name)
		if errors.Is(err, fs.ErrNotExist) && partial {
			continue
		}
		if err != nil {
			return "", fmt.Errorf("target_missing:%s", entry.Target)
		}
		sha := ""
		if entry.Kind == "directory" {
			if !info.IsDir() {
				return "", fmt.Errorf("target_type_changed:%s", entry.Target)
			}
		} else {
			b, err := e.read(name)
			if err != nil {
				return "", err
			}
			sha = hash(b)
			if sha != entry.TargetSHA256 {
				return "", fmt.Errorf("target_drift:%s", entry.Target)
			}
		}
		targets[entry.Target] = record{entry.Target, entry.Kind, sha}
	}
	records := []record{}
	for _, r := range targets {
		records = append(records, r)
	}
	return fingerprint(records), nil
}

// Committed targets can evolve through ordinary authoring, but an IO or
// topology error is never evidence of such evolution. Inventory the entire
// target before comparing content, so an early edit cannot mask a later
// symlink, unreadable file, special file or directory traversal failure.
func (e *engine) validateCommittedTargets(p Plan) (string, bool, error) {
	actual, err := e.inventory("brain")
	if err != nil {
		return "", false, err
	}
	found := make(map[string]record, len(actual))
	for _, r := range actual {
		found[r.Path] = r
	}
	allowed := map[string]bool{}
	targets := map[string]record{}
	evolved := false
	for _, entry := range p.Entries {
		if entry.Action != "copy" {
			continue
		}
		for ancestor := path.Dir(entry.Target); ancestor != "."; ancestor = path.Dir(ancestor) {
			if current, exists := found[ancestor]; exists && current.Kind != "directory" {
				return "", false, fmt.Errorf("target_ancestor_type_changed:%s", ancestor)
			}
		}
		for name := entry.Target; name != "."; name = path.Dir(name) {
			allowed[name] = true
		}
		current, exists := found[entry.Target]
		if !exists {
			evolved = true
			continue
		} // ordinary deletion after commit
		if current.Kind != entry.Kind {
			return "", false, fmt.Errorf("target_type_changed:%s", entry.Target)
		}
		if current.Kind == "file" && current.SHA256 != entry.TargetSHA256 {
			evolved = true
		}
		targets[entry.Target] = current
	}
	for _, r := range actual {
		if r.Path == ".maestro" || r.Path == ".maestro/migration" || strings.HasPrefix(r.Path, ".maestro/migration/") {
			continue
		}
		if !allowed[r.Path] {
			evolved = true
		} // new regular files/directories
	}
	records := make([]record, 0, len(targets))
	for _, r := range targets {
		records = append(records, r)
	}
	return fingerprint(records), evolved, nil
}
func (e *engine) copyEntry(entry Entry) error {
	if entry.Action != "copy" {
		return nil
	}
	target := "brain/" + entry.Target
	if err := e.check(target); err != nil {
		return err
	}
	if entry.Kind == "directory" {
		return e.root.MkdirAll(target, 0700)
	}
	if _, err := e.root.Stat(target); err == nil {
		b, err := e.read(target)
		if err != nil {
			return err
		}
		if hash(b) != entry.TargetSHA256 {
			return fmt.Errorf("target_drift:%s", entry.Target)
		}
		return nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	b, err := e.output(entry)
	if err != nil {
		return err
	}
	if err = e.root.MkdirAll(path.Dir(target), 0700); err != nil {
		return err
	}
	stage := control + "/staging/" + id()
	if err = e.root.MkdirAll(path.Dir(stage), 0700); err != nil {
		return err
	}
	f, err := e.root.OpenFile(stage, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	_, err = f.Write(b)
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	staged, err := e.read(stage)
	if err != nil {
		return err
	}
	if hash(staged) != entry.TargetSHA256 {
		return errors.New("staged_copy_invalid")
	}
	if err = e.check(target); err != nil {
		return err
	}
	if err = e.root.Link(stage, target); err != nil {
		return err
	}
	return e.root.Remove(stage)
}
func (e *engine) load(state Result) (Plan, error) {
	p := Plan{}
	if len(state.AttemptID) != 32 {
		return p, errors.New("invalid_attempt")
	}
	if _, err := hex.DecodeString(state.AttemptID); err != nil {
		return p, errors.New("invalid_attempt")
	}
	b, err := e.read(control + "/attempts/" + state.AttemptID + "/plan.json")
	if err != nil {
		return p, err
	}
	if hash(b) != state.PlanSHA256 {
		return p, errors.New("plan_drift")
	}
	if err = json.Unmarshal(b, &p); err != nil {
		return p, err
	}
	if p.SchemaVersion != 1 || p.Project != e.project || p.AttemptID != state.AttemptID {
		return p, errors.New("plan_binding_mismatch")
	}
	return p, nil
}
func (e *engine) run(o Options) (Result, error) {
	state := baseResult("planned")
	var p Plan
	statePath := control + "/state.json"
	readonly := o.DryRun || o.Status || o.ResolveLegacy != ""
	if err := e.check("brain"); err != nil {
		return state, err
	}
	if b, err := e.read(statePath); err == nil {
		if err = json.Unmarshal(b, &state); err != nil {
			return state, err
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return state, err
	}
	var records []record
	var err error
	if o.ResolveLegacy == "" {
		records, err = e.inventory("data")
		if err != nil {
			return state, err
		}
	}
	if state.AttemptID != "" {
		p, err = e.load(state)
		if err != nil {
			return state, err
		}
		if o.Rollback != "" {
			if o.Rollback != p.AttemptID {
				return state, errors.New("rollback_attempt_mismatch")
			}
			revoked := fromPlan("rolled_back", p, state.PlanSHA256)
			revoked.NextAction = "Reopen the previous untouched installation and validate that runtime; files and runtime were not restored."
			_, err = e.write(statePath, revoked, false)
			return revoked, err
		}
		if state.State == "rolled_back" {
			return state, nil
		}
		if state.State == "committed" {
			if err = e.validateReceipt(state, p); err != nil {
				return state, err
			}
			if o.ResolveLegacy != "" {
				if err = e.validateLegacy(p, o.ResolveLegacy); err != nil {
					return state, err
				}
				state.Current = true
				state.Verification = "legacy_current"
				return state, nil
			}
			if fingerprint(records) != p.SourceFingerprint {
				return state, errors.New("source_drift")
			}
			targetHash, evolved, targetErr := e.validateCommittedTargets(p)
			if targetErr != nil {
				return state, targetErr
			}
			if evolved || targetHash != state.TargetFingerprint {
				state.Current = false
				state.Verification = "target_evolved"
				state.NextAction = "The committed copy is historical evidence. Preserve authored work; do not reuse it as a current update PASS."
			}
			return state, nil
		}
		if fingerprint(records) != p.SourceFingerprint {
			return state, errors.New("source_drift")
		}
		targetHash, err := e.validateTargets(p, state.State != "committed")
		if err != nil {
			state.State = "blocked"
			return state, err
		}
		if readonly {
			state = fromPlan("partial", p, state.PlanSHA256)
			state.Resumable = true
			state.TargetFingerprint = targetHash
			return state, nil
		}
	} else {
		if o.Rollback != "" {
			return state, errors.New("no_attempt_to_rollback")
		}
		if len(records) == 0 {
			return baseResult("not_needed"), nil
		}
		existing, err := e.inventory("brain")
		if err != nil {
			return state, err
		}
		for _, r := range existing {
			if r.Path != ".maestro" && r.Path != ".maestro/migration" && !strings.HasPrefix(r.Path, ".maestro/migration/") {
				return state, errors.New("dual_tree_content")
			}
		}
		p, err = e.plan(records)
		if err != nil {
			return state, err
		}
		state = fromPlan("planned", p, "")
		if readonly {
			return state, nil
		}
		planHash, err := e.write(control+"/attempts/"+p.AttemptID+"/plan.json", p, true)
		if err != nil {
			return state, err
		}
		state.PlanSHA256 = planHash
		if _, err = e.write(statePath, state, false); err != nil {
			return state, err
		}
	}
	state = fromPlan("partial", p, state.PlanSHA256)
	state.Resumable = true
	if _, err = e.write(statePath, state, false); err != nil {
		return state, err
	}
	for _, entry := range p.Entries {
		if err = e.copyEntry(entry); err != nil {
			return state, err
		}
	}
	final, err := e.inventory("data")
	if err != nil {
		return state, err
	}
	if fingerprint(final) != p.SourceFingerprint {
		return state, errors.New("source_drift")
	}
	targetHash, err := e.validateTargets(p, false)
	if err != nil {
		return state, err
	}
	state = fromPlan("committed", p, state.PlanSHA256)
	state.TargetFingerprint = targetHash
	receiptPath := control + "/attempts/" + p.AttemptID + "/receipt.json"
	if b, err := e.read(receiptPath); err == nil {
		var receipt Result
		if err = json.Unmarshal(b, &receipt); err != nil {
			return state, err
		}
		a, _ := json.Marshal(state)
		c, _ := json.Marshal(receipt)
		if string(a) != string(c) {
			return state, errors.New("receipt_conflict")
		}
	} else if errors.Is(err, fs.ErrNotExist) {
		if _, err = e.write(receiptPath, state, true); err != nil {
			return state, err
		}
	} else {
		return state, err
	}
	_, err = e.write(statePath, state, false)
	return state, err
}

// Run returns fresh validation; read-only calls never create state.
func Run(project string, o Options) Result {
	if !o.DryRun && !o.Status && o.ResolveLegacy == "" {
		if err := userlevel.EnsureNotElevated(); err != nil {
			r := baseResult("blocked")
			r.Errors = append(r.Errors, err.Error())
			return r
		}
	}
	absolute, err := filepath.Abs(project)
	if err != nil {
		r := baseResult("blocked")
		r.Errors = append(r.Errors, err.Error())
		return r
	}
	info, err := os.Lstat(absolute)
	if err != nil || info.Mode()&os.ModeSymlink != 0 {
		r := baseResult("blocked")
		r.Errors = append(r.Errors, "invalid_project_root")
		return r
	}
	root, err := os.OpenRoot(absolute)
	if err != nil {
		r := baseResult("blocked")
		r.Errors = append(r.Errors, err.Error())
		return r
	}
	defer root.Close()
	e := engine{root, absolute}
	r, err := e.run(o)
	if err != nil {
		// An invocation-time safety failure must not replace the immutable terminal
		// authority. Once the unsafe path is fixed, the same receipt can be checked
		// again. The returned blocked diagnostic remains visible to the caller.
		preserveCommitted := r.State == "committed"
		r.Current = false
		r.Verification = "unverified"
		if r.State != "partial" || o.Status || o.ResolveLegacy != "" || strings.Contains(err.Error(), "drift") {
			r.State = "blocked"
			r.Resumable = false
		}
		r.Errors = append(r.Errors, err.Error())
		if !preserveCommitted && !o.DryRun && !o.Status && o.ResolveLegacy == "" {
			if _, writeErr := e.write(control+"/state.json", r, false); writeErr != nil {
				r.Errors = append(r.Errors, "state_unwritable")
			}
		}
	}
	if o.ResolveLegacy != "" {
		relative, exists := r.LegacyMap[o.ResolveLegacy]
		if r.State != "committed" || !exists {
			r.State = "blocked"
			r.Errors = append(r.Errors, "legacy_namespace_unavailable")
		} else if checkErr := e.check(relative); checkErr != nil {
			r.State = "blocked"
			r.Errors = append(r.Errors, checkErr.Error())
		} else {
			r.Namespace = o.ResolveLegacy
			r.Path = filepath.Join(absolute, filepath.FromSlash(relative))
		}
	}
	return r
}
