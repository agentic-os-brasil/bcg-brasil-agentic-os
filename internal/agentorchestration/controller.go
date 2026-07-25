// Package agentorchestration provides the fail-closed semantic enforcement
// shared by thin Claude and Codex orchestration adapters.
package agentorchestration

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"net/url"
	"path"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/agentcatalog"
)

type NativeEvent struct {
	Name            string
	BranchID        string
	ActorID         string
	ActorCapability string
	TargetID        string
	Scope           string
	Tool            string
	Operation       string
	Resource        string
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
	PolicySHA256 string    `json:"policy_sha256"`
	BranchID     string    `json:"branch_id"`
	ScopeKind    string    `json:"scope_kind"`
	RootID       string    `json:"root_id"`
	ChildID      string    `json:"child_id,omitempty"`
	Updated      time.Time `json:"updated"`
}

type StateStore struct {
	mu             sync.Mutex
	state          StateSnapshot
	recoverySHA256 [sha256.Size]byte
}

func NewStateStore(recoveryCapability string) (*StateStore, error) {
	if recoveryCapability == "" {
		return nil, errors.New("orchestration state store requires a recovery capability")
	}
	return &StateStore{recoverySHA256: sha256.Sum256([]byte(recoveryCapability))}, nil
}

func RestoreStateStore(snapshot StateSnapshot, recoveryCapability string) (*StateStore, error) {
	if recoveryCapability == "" {
		return nil, errors.New("orchestration state store requires a recovery capability")
	}
	if (snapshot.BranchID == "") != (snapshot.RootID == "") || (snapshot.BranchID == "") != (snapshot.ScopeKind == "") {
		return nil, errors.New("orchestration snapshot has incomplete branch identity")
	}
	if snapshot.ChildID != "" && snapshot.RootID == "" {
		return nil, errors.New("orchestration snapshot has a child without a root")
	}
	return &StateStore{state: snapshot, recoverySHA256: sha256.Sum256([]byte(recoveryCapability))}, nil
}

func (store *StateStore) Snapshot() StateSnapshot {
	store.mu.Lock()
	defer store.mu.Unlock()
	return store.state
}

type Adapter struct {
	runtime        string
	catalog        agentcatalog.Catalog
	authorizations map[string]authorization
	store          *StateStore
	now            func() time.Time
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

var roleScopeKinds = map[string]map[string]bool{
	"account_agent":         {"account": true},
	"capability_specialist": {"account": true, "workspace": true},
	"errand_helper":         {"errand": true},
	"governance_analyst":    {"health": true},
	"hub":                   {"control": true},
	"practice_agent":        {"practice": true},
	"reviewer":              {"review": true},
	"subject_specialist":    {"practice": true},
	"workspace_agent":       {"workspace": true},
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
		contract, ok := catalog.ContractForRole(grant.Role)
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
	adapter.store.mu.Lock()
	defer adapter.store.mu.Unlock()
	if adapter.store.state.PolicySHA256 == "" {
		adapter.store.state.PolicySHA256 = policySHA256
		return nil
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
	root, ok := adapter.authorizations[state.RootID]
	if !ok || root.scope != state.BranchID || root.scopeKind != state.ScopeKind || !adapter.catalog.AllowsDelegation("hub", root.role, 1) || state.Updated.IsZero() {
		return errors.New("orchestration state references an unauthorized root")
	}
	if state.ChildID == "" {
		return nil
	}
	child, ok := adapter.authorizations[state.ChildID]
	if !ok || child.scope != state.BranchID || child.scopeKind != state.ScopeKind || !adapter.catalog.AllowsDelegation(root.role, child.role, 2) {
		return errors.New("orchestration state references an unauthorized child")
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

	adapter.store.mu.Lock()
	defer adapter.store.mu.Unlock()

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
	return decision
}

func (adapter *Adapter) RecoverStale(maxAge time.Duration, recoveryCapability string) bool {
	digest := sha256.Sum256([]byte(recoveryCapability))
	if maxAge <= 0 || subtle.ConstantTimeCompare(adapter.store.recoverySHA256[:], digest[:]) != 1 {
		return false
	}
	adapter.store.mu.Lock()
	defer adapter.store.mu.Unlock()
	state := adapter.store.state
	if state.BranchID == "" || state.Updated.IsZero() || adapter.now().UTC().Sub(state.Updated) <= maxAge {
		return false
	}
	adapter.store.state = StateSnapshot{PolicySHA256: state.PolicySHA256}
	return true
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
	if event.BranchID == "" || actor.role != "hub" || event.ActorID != "maestro" ||
		target.scope != event.BranchID || !adapter.catalog.AllowsDelegation(actor.role, target.role, 1) {
		return denied("edge_denied")
	}
	adapter.store.state = StateSnapshot{
		PolicySHA256: adapter.store.state.PolicySHA256, BranchID: event.BranchID,
		ScopeKind: target.scopeKind, RootID: event.TargetID,
	}
	return allowed()
}

func (adapter *Adapter) startChild(event NativeEvent, actor authorization) Decision {
	state := adapter.store.state
	if state.BranchID == "" {
		return denied("branch_missing")
	}
	if state.ChildID != "" {
		return denied("child_active")
	}
	target, ok := adapter.authorizations[event.TargetID]
	if !ok {
		return denied("target_denied")
	}
	if event.BranchID != state.BranchID || event.ActorID != state.RootID ||
		actor.scope != state.BranchID || target.scope != state.BranchID ||
		actor.scopeKind != state.ScopeKind || target.scopeKind != state.ScopeKind ||
		!adapter.catalog.AllowsDelegation(actor.role, target.role, 2) {
		return denied("edge_denied")
	}
	adapter.store.state.ChildID = event.TargetID
	return allowed()
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
	if event.BranchID != state.BranchID || event.Scope != actor.scope || actor.scope != state.BranchID || actor.scopeKind != state.ScopeKind {
		return denied("scope_denied")
	}
	if event.ActorID != state.RootID && event.ActorID != state.ChildID {
		return denied("actor_denied")
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
	if event.BranchID != state.BranchID || event.ActorID != state.ChildID {
		return denied("actor_denied")
	}
	adapter.store.state.ChildID = ""
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
	adapter.store.state = StateSnapshot{PolicySHA256: state.PolicySHA256}
	return allowed()
}

func allowed() Decision {
	return Decision{Allowed: true, Code: "allowed"}
}

func denied(code string) Decision {
	return Decision{Allowed: false, Code: code}
}
