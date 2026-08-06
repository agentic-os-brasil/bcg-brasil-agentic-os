// Package agentorchestration provides the fail-closed semantic enforcement
// shared by thin Claude and Codex orchestration adapters.
package agentorchestration

import (
	"bytes"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/agentcatalog"
)

type NativeEvent struct {
	Name            string
	BranchID        string
	DispatchID      string
	ActorID         string
	ActorCapability string
	TargetID        string
	Scope           string
	ScopeKind       string
	Tool            string
	Operation       string
	Resource        string
	FenceEpoch      uint64
}

type Decision struct {
	Allowed bool   `json:"allowed"`
	Code    string `json:"code"`
}

type ToolGrant struct {
	Tool           string
	Operation      string
	ResourcePrefix string
}

type Authorization struct {
	AgentID    string
	Role       string
	Scope      string
	ScopeKind  string
	Capability string
	Tools      []ToolGrant
}

type authorization struct {
	role             string
	scope            string
	scopeKind        string
	capabilitySHA256 [sha256.Size]byte
	tools            []ToolGrant
}

type StateSnapshot struct {
	PolicySHA256    string       `json:"policy_sha256"`
	BranchID        string       `json:"branch_id"`
	ScopeID         string       `json:"scope_id"`
	ScopeKind       string       `json:"scope_kind"`
	RootID          string       `json:"root_id"`
	ChildID         string       `json:"child_id,omitempty"`
	ChildDispatchID string       `json:"child_dispatch_id,omitempty"`
	Updated         time.Time    `json:"updated"`
	FenceEpoch      uint64       `json:"fence_epoch"`
	BreadcrumbSeq   uint64       `json:"breadcrumb_seq"`
	BreadcrumbTail  []Breadcrumb `json:"breadcrumb_tail,omitempty"`
}

// Breadcrumb is the bounded, durable control-plane trace for one accepted or
// denied orchestration event. It deliberately contains no prompt, arguments,
// output, error body or resource path. The tail is rehydration evidence, not
// model context; callers must request it explicitly.
type Breadcrumb struct {
	SchemaVersion  int       `json:"schema_version"`
	Sequence       uint64    `json:"sequence"`
	Event          string    `json:"event"`
	ActorID        string    `json:"actor_id"`
	TargetID       string    `json:"target_id,omitempty"`
	BranchID       string    `json:"branch_id,omitempty"`
	DispatchID     string    `json:"dispatch_id,omitempty"`
	Tool           string    `json:"tool,omitempty"`
	Operation      string    `json:"operation,omitempty"`
	ResourceSHA256 string    `json:"resource_sha256,omitempty"`
	Allowed        bool      `json:"allowed"`
	DecisionCode   string    `json:"decision_code"`
	OccurredAt     time.Time `json:"occurred_at"`
	PreviousDigest string    `json:"previous_digest,omitempty"`
	Digest         string    `json:"digest"`
}

const MaximumBreadcrumbTail = 64

type StateStore struct {
	mu             sync.Mutex
	state          StateSnapshot
	recoverySHA256 [sha256.Size]byte
	persistPath    string
	persistMu      sync.Mutex
}

// MaximumDurableStateBytes is the shared bound for the workspace-local
// orchestration snapshot. Every writer, readiness check and hook reader must
// enforce the same ceiling so bootstrap cannot succeed with a state that a
// later lifecycle invocation rejects.
const MaximumDurableStateBytes = 64 << 10

func NewStateStore(recoveryCapability string) (*StateStore, error) {
	if recoveryCapability == "" {
		return nil, errors.New("orchestration state store requires a recovery capability")
	}
	return &StateStore{recoverySHA256: sha256.Sum256([]byte(recoveryCapability))}, nil
}

// NewDurableStateStore opens the single installation state used by all
// runtime adapters. The file contains only policy/branch metadata and is
// atomically replaced after each accepted transition.
func NewDurableStateStore(path, recoveryCapability string) (*StateStore, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("durable orchestration state path is required")
	}
	if recoveryCapability == "" {
		return nil, errors.New("orchestration state store requires a recovery capability")
	}
	store := &StateStore{persistPath: path, recoverySHA256: sha256.Sum256([]byte(recoveryCapability))}
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return store, nil
	}
	if err != nil {
		return nil, fmt.Errorf("inspect durable orchestration state: %w", err)
	}
	if err := validateDurableStateFileInfo(info); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read durable orchestration state: %w", err)
	}
	if len(data) == 0 {
		return store, nil
	}
	if err := decodeStateSnapshot(data, &store.state); err != nil {
		return nil, fmt.Errorf("decode durable orchestration state: %w", err)
	}
	if err := validateSnapshot(store.state); err != nil {
		return nil, err
	}
	return store, nil
}

// EnsureDurableState materializes the empty, valid snapshot required before a
// workspace-local runtime hook is handed to a native adapter. It is separate
// from NewDurableStateStore because opening a missing state file must remain a
// read-only operation for hook inspection. Existing valid state is never
// replaced; an empty interrupted file is repaired to the valid empty snapshot.
func EnsureDurableState(path, recoveryCapability string) error {
	// Inspect the directory entry before opening it. Hooks must never accept a
	// symlink as the durable state target, even if the symlink currently points
	// at valid JSON.
	info, err := os.Lstat(path)
	if err == nil {
		if err := validateDurableStateFileInfo(info); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect durable orchestration state: %w", err)
	}
	store, err := NewDurableStateStore(path, recoveryCapability)
	if err != nil {
		return err
	}
	if info != nil && info.Size() > 0 {
		return nil
	}

	unlock, err := acquireStateFileLock(path)
	if err != nil {
		return err
	}
	defer func() { _ = unlock() }()
	info, err = os.Lstat(path)
	if err == nil {
		if err := validateDurableStateFileInfo(info); err != nil {
			return err
		}
		if info.Size() > 0 {
			return nil
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect durable orchestration state: %w", err)
	}
	return store.persistLocked()
}

func validateDurableStateFileInfo(info os.FileInfo) error {
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return errors.New("orchestration state target is not a regular non-symlink file")
	}
	if info.Size() > MaximumDurableStateBytes {
		return errors.New("orchestration state exceeds the bounded JSON limit")
	}
	if info.Mode().Perm()&0o077 != 0 {
		return errors.New("orchestration state must be owner-only (0600 or stricter)")
	}
	return nil
}

func decodeStateSnapshot(data []byte, target *StateSnapshot) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return errors.New("orchestration state must be a JSON object")
	}
	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("orchestration state contains multiple JSON values")
		}
		return err
	}
	return nil
}

// DecodeStateSnapshot applies the canonical bounded-state JSON contract to a
// read-only payload. Runtime adapters use it instead of maintaining a second
// decoder with subtly different null/unknown-field/trailing-data semantics.
func DecodeStateSnapshot(data []byte) (StateSnapshot, error) {
	var snapshot StateSnapshot
	if err := decodeStateSnapshot(data, &snapshot); err != nil {
		return StateSnapshot{}, err
	}
	return snapshot, nil
}

// ValidateStateSnapshot exposes the same structural invariant used by the
// durable store so read-only readiness checks cannot drift from persisted
// transition validation.
func ValidateStateSnapshot(snapshot StateSnapshot) error {
	return validateSnapshot(snapshot)
}

func RestoreStateStore(snapshot StateSnapshot, recoveryCapability string) (*StateStore, error) {
	if recoveryCapability == "" {
		return nil, errors.New("orchestration state store requires a recovery capability")
	}
	if err := validateSnapshot(snapshot); err != nil {
		return nil, err
	}
	return &StateStore{state: snapshot, recoverySHA256: sha256.Sum256([]byte(recoveryCapability))}, nil
}

func validateSnapshot(snapshot StateSnapshot) error {
	if (snapshot.BranchID == "") != (snapshot.RootID == "") ||
		(snapshot.BranchID == "") != (snapshot.ScopeID == "") ||
		(snapshot.BranchID == "") != (snapshot.ScopeKind == "") {
		return errors.New("orchestration snapshot has incomplete branch identity")
	}
	if (snapshot.ChildID == "") != (snapshot.ChildDispatchID == "") || (snapshot.ChildID != "" && snapshot.RootID == "") {
		return errors.New("orchestration snapshot has a child without a root")
	}
	if len(snapshot.BreadcrumbTail) > MaximumBreadcrumbTail {
		return errors.New("orchestration breadcrumb tail exceeds its bounded limit")
	}
	var previous string
	for index, breadcrumb := range snapshot.BreadcrumbTail {
		if err := validateBreadcrumb(breadcrumb); err != nil {
			return err
		}
		if index > 0 && breadcrumb.Sequence != snapshot.BreadcrumbTail[index-1].Sequence+1 {
			return errors.New("orchestration breadcrumb sequence is not contiguous")
		}
		if index > 0 && breadcrumb.PreviousDigest != previous {
			return errors.New("orchestration breadcrumb chain is not linked")
		}
		previous = breadcrumb.Digest
	}
	if len(snapshot.BreadcrumbTail) > 0 && snapshot.BreadcrumbSeq != snapshot.BreadcrumbTail[len(snapshot.BreadcrumbTail)-1].Sequence {
		return errors.New("orchestration breadcrumb sequence is not current")
	}
	return nil
}

func validateBreadcrumb(breadcrumb Breadcrumb) error {
	if breadcrumb.SchemaVersion != 1 || breadcrumb.Sequence == 0 ||
		!breadcrumbEvents[breadcrumb.Event] ||
		!agentcatalog.ValidAgentID(breadcrumb.ActorID) ||
		(breadcrumb.TargetID != "" && !agentcatalog.ValidAgentID(breadcrumb.TargetID)) ||
		!validBreadcrumbToken(breadcrumb.BranchID, 128) || !validBreadcrumbToken(breadcrumb.DispatchID, 128) ||
		!validBreadcrumbToken(breadcrumb.Tool, 64) || !validBreadcrumbToken(breadcrumb.Operation, 64) ||
		!validBreadcrumbToken(breadcrumb.DecisionCode, 96) ||
		breadcrumb.OccurredAt.IsZero() || !validDigest(breadcrumb.Digest) {
		return errors.New("orchestration breadcrumb is invalid")
	}
	if breadcrumb.PreviousDigest != "" && !validDigest(breadcrumb.PreviousDigest) {
		return errors.New("orchestration breadcrumb previous digest is invalid")
	}
	if breadcrumb.ResourceSHA256 != "" && !validDigest(breadcrumb.ResourceSHA256) {
		return errors.New("orchestration breadcrumb resource digest is invalid")
	}
	if breadcrumb.Digest != breadcrumbDigest(breadcrumb) {
		return errors.New("orchestration breadcrumb digest is invalid")
	}
	return nil
}

func validBreadcrumbToken(value string, maximum int) bool {
	if value == "" {
		return true
	}
	if len(value) > maximum {
		return false
	}
	for index, char := range value {
		if !(char >= 'A' && char <= 'Z' || char >= 'a' && char <= 'z' || char >= '0' && char <= '9' || char == '_' || char == '-' || char == '.') || index == 0 && char == '.' {
			return false
		}
	}
	return true
}

func validDigest(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func breadcrumbDigest(breadcrumb Breadcrumb) string {
	digest := breadcrumb.Digest
	breadcrumb.Digest = ""
	body, _ := json.Marshal(breadcrumb)
	breadcrumb.Digest = digest
	return digestBytes(body)
}

func digestBytes(value []byte) string {
	digest := sha256.Sum256(value)
	return hex.EncodeToString(digest[:])
}

func appendBreadcrumb(state *StateSnapshot, event string, native NativeEvent, decision Decision, occurredAt time.Time) error {
	sequence := state.BreadcrumbSeq + 1
	previous := ""
	if len(state.BreadcrumbTail) > 0 {
		previous = state.BreadcrumbTail[len(state.BreadcrumbTail)-1].Digest
	}
	resourceDigest := ""
	if native.Resource != "" {
		resourceDigest = digestBytes([]byte(native.Resource))
	}
	breadcrumb := Breadcrumb{
		SchemaVersion: 1, Sequence: sequence, Event: event, ActorID: native.ActorID,
		TargetID: native.TargetID, BranchID: native.BranchID, DispatchID: native.DispatchID,
		Tool: native.Tool, Operation: native.Operation, ResourceSHA256: resourceDigest,
		Allowed: decision.Allowed, DecisionCode: decision.Code, OccurredAt: occurredAt.UTC(),
		PreviousDigest: previous,
	}
	breadcrumb.Digest = breadcrumbDigest(breadcrumb)
	if err := validateBreadcrumb(breadcrumb); err != nil {
		return err
	}
	state.BreadcrumbSeq = sequence
	state.BreadcrumbTail = append(state.BreadcrumbTail, breadcrumb)
	if len(state.BreadcrumbTail) > MaximumBreadcrumbTail {
		state.BreadcrumbTail = append([]Breadcrumb(nil), state.BreadcrumbTail[len(state.BreadcrumbTail)-MaximumBreadcrumbTail:]...)
	}
	return nil
}

func (store *StateStore) persistLocked() error {
	if store.persistPath == "" {
		return nil
	}
	if err := validateSnapshot(store.state); err != nil {
		return err
	}
	data, err := json.MarshalIndent(store.state, "", "  ")
	if err != nil {
		return err
	}
	directory := filepath.Dir(store.persistPath)
	if err := validateStateParents(store.persistPath); err != nil {
		return err
	}
	if info, err := os.Lstat(store.persistPath); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return errors.New("orchestration state target is not a regular file")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	store.persistMu.Lock()
	defer store.persistMu.Unlock()
	temporary, err := os.CreateTemp(directory, ".maestro-state-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(append(data, '\n')); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, store.persistPath); err != nil {
		return err
	}
	if runtime.GOOS == "windows" {
		return nil
	}
	directoryFile, err := os.Open(directory)
	if err != nil {
		return err
	}
	defer directoryFile.Close()
	return directoryFile.Sync()
}

func (store *StateStore) refreshLocked() error {
	if store.persistPath == "" {
		return nil
	}
	info, err := os.Lstat(store.persistPath)
	if errors.Is(err, os.ErrNotExist) {
		if store.state.BranchID != "" {
			return errors.New("durable orchestration state disappeared")
		}
		return nil
	}
	if err != nil {
		return err
	}
	if err := validateDurableStateFileInfo(info); err != nil {
		return err
	}
	data, err := os.ReadFile(store.persistPath)
	if err != nil {
		return err
	}
	if len(data) == 0 {
		return nil
	}
	var state StateSnapshot
	if err := decodeStateSnapshot(data, &state); err != nil {
		return err
	}
	if err := validateSnapshot(state); err != nil {
		return err
	}
	store.state = state
	return nil
}

func (store *StateStore) Snapshot() StateSnapshot {
	store.mu.Lock()
	defer store.mu.Unlock()
	_ = store.refreshLocked()
	snapshot := store.state
	snapshot.BreadcrumbTail = append([]Breadcrumb(nil), store.state.BreadcrumbTail...)
	return snapshot
}

type Adapter struct {
	runtime        string
	catalog        agentcatalog.Catalog
	authorizations map[string]authorization
	store          *StateStore
	now            func() time.Time
}

// RoleForAgent exposes only the already-validated catalog role to trusted
// runtime-neutral dispatch code. It never authenticates a caller or grants
// tools, data access or delegation authority.
func (adapter *Adapter) RoleForAgent(agentID string) (string, bool) {
	authorization, ok := adapter.authorizations[agentID]
	if !ok {
		return "", false
	}
	return authorization.role, true
}

var adapterEvents = map[string]map[string]string{
	"claude": {
		"agent_branch_start": "branch_start",
		"agent_child_start":  "child_start",
		"pre_tool_use":       "tool_request",
		"agent_child_stop":   "child_finish",
		"agent_branch_stop":  "branch_finish",
	},
	"codex": {
		"collaboration_branch_start": "branch_start",
		"collaboration_child_start":  "child_start",
		"tool_call_guard":            "tool_request",
		"collaboration_child_stop":   "child_finish",
		"collaboration_branch_stop":  "branch_finish",
	},
}

var breadcrumbEvents = map[string]bool{
	"branch_start": true, "child_start": true, "tool_request": true, "child_finish": true, "branch_finish": true,
}

var roleScopeKinds = map[string]map[string]bool{
	"case_agent":           {"case": true, "workspace": true},
	"client_account_agent": {"account": true},
	"errand_helper":        {"errand": true},
	"governance_analyst":   {"health": true},
	"hub":                  {"control": true},
	"pa_expert":            {"practice": true},
	"quality_guardian":     {"workspace": true},
	"reviewer":             {"review": true},
}

func NewAdapter(runtime string, catalog agentcatalog.Catalog, grants []Authorization, store *StateStore) (*Adapter, error) {
	if _, ok := adapterEvents[runtime]; !ok {
		return nil, errors.New("agent orchestration runtime is unsupported")
	}
	if store == nil {
		return nil, errors.New("agent orchestration requires a shared state store")
	}
	if err := catalog.Validate(); err != nil {
		return nil, err
	}
	authorizations, policySHA256, err := validateAuthorizations(catalog, grants)
	if err != nil {
		return nil, err
	}
	adapter := &Adapter{
		runtime: runtime, catalog: catalog, authorizations: authorizations,
		store: store, now: time.Now,
	}
	if err := adapter.bindPolicy(policySHA256); err != nil {
		return nil, err
	}
	if err := adapter.validateStoreState(); err != nil {
		return nil, err
	}
	return adapter, nil
}

func validateAuthorizations(catalog agentcatalog.Catalog, grants []Authorization) (map[string]authorization, string, error) {
	values := make(map[string]authorization, len(grants))
	for _, grant := range grants {
		if err := catalog.RejectLegacyRegistration(grant.AgentID, grant.Role); err != nil {
			return nil, "", err
		}
		role := catalog.CanonicalRole(grant.Role)
		grant.Role = role
		contract, ok := catalog.ContractForRole(role)
		if !agentcatalog.ValidAgentID(grant.AgentID) || !ok || grant.Capability == "" {
			return nil, "", errors.New("agent authorization is incomplete or uses an unsupported role")
		}
		if _, exists := values[grant.AgentID]; exists {
			return nil, "", errors.New("agent authorization IDs must be unique")
		}
		if !roleScopeKinds[grant.Role][grant.ScopeKind] || (grant.Role != "hub" && grant.Scope == "") || (grant.Role == "hub" && grant.Scope != "") {
			return nil, "", errors.New("agent authorization has an invalid scope kind or identity")
		}
		if contract.ToolAccess == "none" && len(grant.Tools) != 0 {
			return nil, "", errors.New("no-tool role cannot receive tool grants")
		}
		if contract.ToolAccess == "scoped" && grant.Scope == "" {
			return nil, "", errors.New("scoped agent authorization requires an immutable scope")
		}
		normalizedTools := make([]ToolGrant, 0, len(grant.Tools))
		for _, tool := range grant.Tools {
			resourcePrefix, valid := canonicalResource(tool.ResourcePrefix, true)
			if contract.ToolAccess != "scoped" || tool.Tool == "" || tool.Operation == "" || !valid || !resourceBoundToScope(resourcePrefix, grant.Scope, grant.ScopeKind) {
				return nil, "", errors.New("agent tool grant is invalid or unscoped")
			}
			tool.ResourcePrefix = resourcePrefix
			normalizedTools = append(normalizedTools, tool)
		}
		sort.Slice(normalizedTools, func(i, j int) bool {
			left, right := normalizedTools[i], normalizedTools[j]
			return left.Tool+"\x00"+left.Operation+"\x00"+left.ResourcePrefix < right.Tool+"\x00"+right.Operation+"\x00"+right.ResourcePrefix
		})
		values[grant.AgentID] = authorization{
			role: grant.Role, scope: grant.Scope, scopeKind: grant.ScopeKind,
			capabilitySHA256: sha256.Sum256([]byte(grant.Capability)),
			tools:            normalizedTools,
		}
	}
	maestro, ok := values["maestro"]
	if !ok || maestro.role != "hub" {
		return nil, "", errors.New("agent authorizations require the canonical Maestro identity")
	}
	return values, authorizationFingerprint(values), nil
}

func canonicalResource(value string, requirePrefix bool) (string, bool) {
	if strings.Contains(value, "%") || strings.Contains(value, "\\") {
		return "", false
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != "bcgos" || parsed.Host == "" || parsed.Host != strings.ToLower(parsed.Host) ||
		parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Opaque != "" || parsed.RawPath != "" {
		return "", false
	}
	resourcePath := parsed.Path
	if !strings.HasPrefix(resourcePath, "/") || strings.Contains(resourcePath, "//") {
		return "", false
	}
	trimmed := strings.TrimSuffix(resourcePath, "/")
	if trimmed == "" {
		trimmed = "/"
	}
	if path.Clean(resourcePath) != trimmed || strings.Contains(resourcePath, "/../") || strings.Contains(resourcePath, "/./") {
		return "", false
	}
	if requirePrefix != strings.HasSuffix(resourcePath, "/") {
		return "", false
	}
	return "bcgos://" + parsed.Host + resourcePath, true
}

func NormalizeResource(value string) (string, bool) {
	return canonicalResource(value, strings.HasSuffix(value, "/"))
}

func ResourceWithinScope(resource, scopeKind, scope string) bool {
	normalized, valid := NormalizeResource(resource)
	if !valid || strings.HasSuffix(normalized, "/") {
		return false
	}
	parsed, _ := url.Parse(normalized)
	if parsed.Host == "public" {
		return parsed.Path != "/"
	}
	scopeRoot := "/" + scope + "/"
	return parsed.Host == scopeKind && strings.HasPrefix(parsed.Path, scopeRoot) && len(parsed.Path) > len(scopeRoot)
}

func resourceBoundToScope(resourcePrefix, scope, scopeKind string) bool {
	parsed, _ := url.Parse(resourcePrefix)
	if parsed.Host == "public" {
		return parsed.Path == "/"
	}
	return parsed.Host == scopeKind && parsed.Path == "/"+scope+"/"
}

func authorizationFingerprint(values map[string]authorization) string {
	ids := make([]string, 0, len(values))
	for id := range values {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	hasher := sha256.New()
	for _, id := range ids {
		value := values[id]
		hasher.Write([]byte(id + "\x00" + value.role + "\x00" + value.scopeKind + "\x00" + value.scope + "\x00"))
		hasher.Write(value.capabilitySHA256[:])
		for _, tool := range value.tools {
			hasher.Write([]byte("\x00" + tool.Tool + "\x00" + tool.Operation + "\x00" + tool.ResourcePrefix))
		}
	}
	return hex.EncodeToString(hasher.Sum(nil))
}

func (adapter *Adapter) bindPolicy(policySHA256 string) error {
	unlock, err := func() (func() error, error) {
		if adapter.store.persistPath == "" {
			return func() error { return nil }, nil
		}
		return acquireStateFileLock(adapter.store.persistPath)
	}()
	if err != nil {
		return err
	}
	defer unlock()
	adapter.store.mu.Lock()
	defer adapter.store.mu.Unlock()
	if err := adapter.store.refreshLocked(); err != nil {
		return err
	}
	if adapter.store.state.PolicySHA256 == "" {
		adapter.store.state.PolicySHA256 = policySHA256
		return adapter.store.persistLocked()
	}
	if adapter.store.state.PolicySHA256 != policySHA256 {
		return errors.New("orchestration state store authorization policy changed")
	}
	return nil
}

func (adapter *Adapter) validateStoreState() error {
	adapter.store.mu.Lock()
	defer adapter.store.mu.Unlock()
	state := adapter.store.state
	if state.BranchID == "" {
		return nil
	}
	if state.ChildID != "" || state.ChildDispatchID != "" {
		return errors.New("orchestration state contains a forbidden nested child")
	}
	root, ok := adapter.authorizations[state.RootID]
	if !ok || root.scope != state.ScopeID || root.scopeKind != state.ScopeKind || !adapter.catalog.AllowsDelegation("hub", root.role, 1) || state.Updated.IsZero() {
		return errors.New("orchestration state references an unauthorized root")
	}
	return nil
}

func (adapter *Adapter) Handle(event NativeEvent) Decision {
	semanticEvent, ok := adapterEvents[adapter.runtime][event.Name]
	if !ok {
		return denied("event_unsupported")
	}
	actor, ok := adapter.authenticate(event.ActorID, event.ActorCapability)
	if !ok {
		return denied("actor_denied")
	}
	unlock, err := func() (func() error, error) {
		if adapter.store.persistPath == "" {
			return func() error { return nil }, nil
		}
		return acquireStateFileLock(adapter.store.persistPath)
	}()
	if err != nil {
		return denied("state_lock_unavailable")
	}
	defer unlock()

	adapter.store.mu.Lock()
	defer adapter.store.mu.Unlock()
	if err := adapter.store.refreshLocked(); err != nil {
		return denied("state_refresh_failed")
	}
	previous := adapter.store.state
	if semanticEvent != "branch_start" && event.FenceEpoch != 0 && event.FenceEpoch != adapter.store.state.FenceEpoch {
		return denied("fence_epoch_mismatch")
	}

	var decision Decision
	switch semanticEvent {
	case "branch_start":
		decision = adapter.startBranch(event, actor)
	case "child_start":
		decision = adapter.startChild(event, actor)
	case "tool_request":
		decision = adapter.guardTool(event, actor)
	case "child_finish":
		decision = adapter.finishChild(event)
	case "branch_finish":
		decision = adapter.finishBranch(event)
	default:
		decision = denied("event_unsupported")
	}
	if decision.Allowed {
		adapter.store.state.Updated = adapter.now().UTC()
	}
	if err := appendBreadcrumb(&adapter.store.state, semanticEvent, event, decision, adapter.now()); err != nil {
		adapter.store.state = previous
		return denied("breadcrumb_invalid")
	}
	if err := adapter.store.persistLocked(); err != nil {
		adapter.store.state = previous
		return denied("state_persist_failed")
	}
	return decision
}

func (adapter *Adapter) StartBranch(actorID, capability, targetID, branchID, scopeID, scopeKind string) Decision {
	return adapter.Handle(NativeEvent{
		Name:    adapter.nativeEvent("branch_start"),
		ActorID: actorID, ActorCapability: capability, TargetID: targetID,
		BranchID: branchID, Scope: scopeID, ScopeKind: scopeKind,
	})
}

func (adapter *Adapter) StartChild(actorID, capability, targetID, branchID, dispatchID, scopeID, scopeKind string) Decision {
	return adapter.Handle(NativeEvent{
		Name:    adapter.nativeEvent("child_start"),
		ActorID: actorID, ActorCapability: capability, TargetID: targetID,
		BranchID: branchID, DispatchID: dispatchID, Scope: scopeID, ScopeKind: scopeKind,
		FenceEpoch: adapter.store.Snapshot().FenceEpoch,
	})
}

func (adapter *Adapter) FinishChild(actorID, capability, branchID, dispatchID string) Decision {
	return adapter.Handle(NativeEvent{
		Name:    adapter.nativeEvent("child_finish"),
		ActorID: actorID, ActorCapability: capability, BranchID: branchID,
		DispatchID: dispatchID, FenceEpoch: adapter.store.Snapshot().FenceEpoch,
	})
}

func (adapter *Adapter) FinishBranch(actorID, capability, branchID string) Decision {
	return adapter.Handle(NativeEvent{
		Name:    adapter.nativeEvent("branch_finish"),
		ActorID: actorID, ActorCapability: capability, BranchID: branchID, FenceEpoch: adapter.store.Snapshot().FenceEpoch,
	})
}

func (adapter *Adapter) GuardTool(actorID, capability, branchID, dispatchID, scopeID, scopeKind, tool, operation, resource string) Decision {
	return adapter.Handle(NativeEvent{
		Name:    adapter.nativeEvent("tool_request"),
		ActorID: actorID, ActorCapability: capability,
		BranchID: branchID, DispatchID: dispatchID,
		Scope: scopeID, ScopeKind: scopeKind,
		FenceEpoch: adapter.store.Snapshot().FenceEpoch,
		Tool:       tool, Operation: operation, Resource: resource,
	})
}

// AuthorizeActiveRoot proves that a capability-bound root agent still owns the
// active branch before it performs a local, non-tool action such as selecting
// a managed skill. It creates no lifecycle event and grants no resource access.
func (adapter *Adapter) AuthorizeActiveRoot(actorID, capability, branchID, scopeID, scopeKind string) Decision {
	actor, ok := adapter.authenticate(actorID, capability)
	if !ok {
		return denied("actor_denied")
	}
	unlock := func() error { return nil }
	if adapter.store.persistPath != "" {
		var err error
		unlock, err = acquireStateFileLock(adapter.store.persistPath)
		if err != nil {
			return denied("state_lock_unavailable")
		}
	}
	defer func() { _ = unlock() }()
	adapter.store.mu.Lock()
	defer adapter.store.mu.Unlock()
	if err := adapter.store.refreshLocked(); err != nil {
		return denied("state_refresh_failed")
	}
	state := adapter.store.state
	if state.BranchID == "" {
		return denied("branch_missing")
	}
	if state.ChildID != "" {
		return denied("child_active")
	}
	if actorID != state.RootID || branchID != state.BranchID ||
		scopeID != state.ScopeID || scopeKind != state.ScopeKind ||
		actor.scope != state.ScopeID || actor.scopeKind != state.ScopeKind {
		return denied("actor_denied")
	}
	return allowed()
}

func (adapter *Adapter) nativeEvent(semantic string) string {
	for native, candidate := range adapterEvents[adapter.runtime] {
		if candidate == semantic {
			return native
		}
	}
	return ""
}

func (adapter *Adapter) RecoverStale(maxAge time.Duration, recoveryCapability string) bool {
	digest := sha256.Sum256([]byte(recoveryCapability))
	if maxAge <= 0 || subtle.ConstantTimeCompare(adapter.store.recoverySHA256[:], digest[:]) != 1 {
		return false
	}
	unlock, err := func() (func() error, error) {
		if adapter.store.persistPath == "" {
			return func() error { return nil }, nil
		}
		return acquireStateFileLock(adapter.store.persistPath)
	}()
	if err != nil {
		return false
	}
	defer unlock()
	adapter.store.mu.Lock()
	defer adapter.store.mu.Unlock()
	if err := adapter.store.refreshLocked(); err != nil {
		return false
	}
	state := adapter.store.state
	if state.BranchID == "" || state.Updated.IsZero() || adapter.now().UTC().Sub(state.Updated) <= maxAge {
		return false
	}
	adapter.store.state = StateSnapshot{PolicySHA256: state.PolicySHA256, FenceEpoch: state.FenceEpoch}
	if err := adapter.store.persistLocked(); err != nil {
		adapter.store.state = state
		return false
	}
	return true
}

// Runtime identifies the native event vocabulary bound to this adapter. It is
// metadata only; it does not expose authorization capabilities or state.
func (adapter *Adapter) Runtime() string {
	return adapter.runtime
}

// Snapshot exposes metadata-only orchestration state for governed status and
// completion checks. It never exposes capabilities or tool grants.
func (adapter *Adapter) Snapshot() StateSnapshot {
	return adapter.store.Snapshot()
}

func (adapter *Adapter) authenticate(id, capability string) (authorization, bool) {
	value, ok := adapter.authorizations[id]
	if !ok || capability == "" {
		return authorization{}, false
	}
	digest := sha256.Sum256([]byte(capability))
	return value, subtle.ConstantTimeCompare(value.capabilitySHA256[:], digest[:]) == 1
}

func (adapter *Adapter) startBranch(event NativeEvent, actor authorization) Decision {
	if adapter.store.state.BranchID != "" {
		return denied("branch_active")
	}
	target, ok := adapter.authorizations[event.TargetID]
	if !ok {
		return denied("target_denied")
	}
	if event.BranchID == "" || event.Scope == "" || actor.role != "hub" || event.ActorID != "maestro" ||
		target.scope != event.Scope || target.scopeKind != event.ScopeKind ||
		!adapter.catalog.AllowsDelegation(actor.role, target.role, 1) {
		return denied("edge_denied")
	}
	adapter.store.state = StateSnapshot{
		PolicySHA256: adapter.store.state.PolicySHA256, BranchID: event.BranchID,
		ScopeID: event.Scope, ScopeKind: target.scopeKind, RootID: event.TargetID,
		FenceEpoch:    adapter.store.state.FenceEpoch + 1,
		BreadcrumbSeq: adapter.store.state.BreadcrumbSeq, BreadcrumbTail: append([]Breadcrumb(nil), adapter.store.state.BreadcrumbTail...),
	}
	return allowed()
}

func (adapter *Adapter) startChild(event NativeEvent, actor authorization) Decision {
	// Depth one is a hard product invariant. The native event names remain
	// understood only so an installed legacy adapter receives a deterministic
	// denial instead of accidentally creating a nested branch.
	_ = event
	_ = actor
	return denied("depth_one_no_children")
}

func (adapter *Adapter) guardTool(event NativeEvent, actor authorization) Decision {
	state := adapter.store.state
	contract, _ := adapter.catalog.ContractForRole(actor.role)
	if contract.ToolAccess != "scoped" {
		return denied("tool_denied")
	}
	if state.BranchID == "" {
		return denied("branch_missing")
	}
	if event.BranchID != state.BranchID || event.Scope != actor.scope || actor.scope != state.ScopeID || actor.scopeKind != state.ScopeKind {
		return denied("scope_denied")
	}
	if event.ActorID != state.RootID && event.ActorID != state.ChildID {
		return denied("actor_denied")
	}
	if (event.ActorID == state.ChildID && event.DispatchID != state.ChildDispatchID) ||
		(event.ActorID == state.RootID && event.DispatchID != "") {
		return denied("dispatch_denied")
	}
	resource, valid := canonicalResource(event.Resource, false)
	if !valid {
		return denied("resource_denied")
	}
	for _, grant := range actor.tools {
		if event.Tool == grant.Tool && event.Operation == grant.Operation && strings.HasPrefix(resource, grant.ResourcePrefix) {
			return allowed()
		}
	}
	return denied("resource_denied")
}

func (adapter *Adapter) finishChild(event NativeEvent) Decision {
	state := adapter.store.state
	if state.BranchID == "" {
		return denied("branch_missing")
	}
	if state.ChildID == "" {
		return denied("child_missing")
	}
	if event.BranchID != state.BranchID || event.DispatchID != state.ChildDispatchID || event.ActorID != state.ChildID {
		return denied("actor_denied")
	}
	adapter.store.state.ChildID = ""
	adapter.store.state.ChildDispatchID = ""
	return allowed()
}

func (adapter *Adapter) finishBranch(event NativeEvent) Decision {
	state := adapter.store.state
	if state.BranchID == "" {
		return denied("branch_missing")
	}
	if state.ChildID != "" {
		return denied("child_active")
	}
	if event.BranchID != state.BranchID || event.ActorID != state.RootID {
		return denied("actor_denied")
	}
	adapter.store.state = StateSnapshot{
		PolicySHA256: state.PolicySHA256, FenceEpoch: state.FenceEpoch,
		BreadcrumbSeq: state.BreadcrumbSeq, BreadcrumbTail: append([]Breadcrumb(nil), state.BreadcrumbTail...),
	}
	return allowed()
}

func allowed() Decision {
	return Decision{Allowed: true, Code: "allowed"}
}

func denied(code string) Decision {
	return Decision{Allowed: false, Code: code}
}
