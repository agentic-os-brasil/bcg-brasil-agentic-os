// Package agentscaffold materializes governed, data-free agent instances from
// managed role templates. It does not activate a runtime or grant tools.
package agentscaffold

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"

	baseagents "github.com/agentic-os-brasil/bcg-brasil-agentic-os/bundles/base/agents"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/agentcatalog"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/agentidentity"
)

var expertVersion = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?$`)

func validExpertVersion(value string) bool {
	return expertVersion.MatchString(value)
}

type Request struct {
	AgentID         string `json:"agent_id"`
	Role            string `json:"role"`
	ScopeKind       string `json:"scope_kind"`
	ScopeID         string `json:"scope_id"`
	ParentAgent     string `json:"parent_agent_id"`
	ParentRole      string `json:"parent_role"`
	AccountAgentID  string `json:"account_agent_id,omitempty"`
	Owner           string `json:"owner,omitempty"`
	Mandate         string `json:"mandate,omitempty"`
	CanonPath       string `json:"canon_path,omitempty"`
	CanonSHA256     string `json:"canon_sha256,omitempty"`
	ExpertKind      string `json:"expert_kind,omitempty"`
	ExpertVersion   string `json:"expert_version,omitempty"`
	ExpertLifecycle string `json:"expert_lifecycle,omitempty"`
	DisplayName     string `json:"display_name,omitempty"`
	Emoji           string `json:"emoji,omitempty"`
	OwnerID         string `json:"owner_id,omitempty"`
	OwnershipScope  string `json:"ownership_scope,omitempty"`
}

type Instance struct {
	SchemaVersion     int       `json:"schema_version"`
	AgentID           string    `json:"agent_id"`
	Role              string    `json:"role"`
	ScopeKind         string    `json:"scope_kind"`
	ScopeID           string    `json:"scope_id"`
	ParentAgentID     string    `json:"parent_agent_id"`
	ParentRole        string    `json:"parent_role"`
	AccountAgentID    string    `json:"account_agent_id,omitempty"`
	Owner             string    `json:"owner,omitempty"`
	Mandate           string    `json:"mandate,omitempty"`
	CanonPath         string    `json:"canon_path,omitempty"`
	CanonSHA256       string    `json:"canon_sha256,omitempty"`
	ExpertKind        string    `json:"expert_kind,omitempty"`
	ExpertVersion     string    `json:"expert_version,omitempty"`
	ExpertLifecycle   string    `json:"expert_lifecycle,omitempty"`
	DisplayName       string    `json:"display_name"`
	Emoji             string    `json:"emoji"`
	OwnerID           string    `json:"owner_id"`
	OwnershipScope    string    `json:"ownership_scope"`
	InputContract     string    `json:"input_contract"`
	ToolAccess        string    `json:"tool_access"`
	MayDelegate       bool      `json:"may_delegate"`
	DefinitionPath    string    `json:"definition_path"`
	DefinitionSHA256  string    `json:"definition_sha256"`
	StateSHA256       string    `json:"state_sha256"`
	RegistrationState string    `json:"registration_state"`
	RuntimeState      string    `json:"runtime_state"`
	CreatedAt         time.Time `json:"created_at"`
	HMACSHA256        string    `json:"hmac_sha256"`
}

type OperationalState struct {
	SchemaVersion int       `json:"schema_version"`
	AgentID       string    `json:"agent_id"`
	Lifecycle     string    `json:"lifecycle"`
	RuntimeState  string    `json:"runtime_state"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type Pointer struct {
	Path      string `json:"path"`
	Available bool   `json:"available"`
	SHA256    string `json:"sha256,omitempty"`
}

type Status struct {
	Initialized bool     `json:"initialized"`
	Existing    bool     `json:"existing"`
	Instance    Instance `json:"instance"`
	Definition  Pointer  `json:"definition"`
	State       Pointer  `json:"state"`
}

func WorkspaceRequest(workspaceID string) Request {
	return Request{
		AgentID: "workspace-agent-" + workspaceID, Role: "workspace_agent",
		ScopeKind: "workspace", ScopeID: workspaceID,
		ParentAgent: "maestro", ParentRole: "hub",
	}
}

func Scaffold(dataRoot string, request Request) (Status, error) {
	if err := applyIdentity(dataRoot, &request); err != nil {
		return Status{}, err
	}
	catalog, err := baseagents.Catalog()
	if err != nil {
		return Status{}, err
	}
	request.Role = catalog.CanonicalRole(request.Role)
	request.ParentRole = catalog.CanonicalRole(request.ParentRole)
	contract, err := validateRequest(catalog, request)
	if err != nil {
		return Status{}, err
	}
	template, err := baseagents.Template(request.Role)
	if err != nil {
		return Status{}, err
	}
	templateDigest := sha256.Sum256(template)
	templateSHA256 := hex.EncodeToString(templateDigest[:])

	if err := os.MkdirAll(dataRoot, 0o700); err != nil {
		return Status{}, err
	}
	root, err := os.OpenRoot(dataRoot)
	if err != nil {
		return Status{}, err
	}
	defer root.Close()
	integrityKey, err := loadOrCreateIntegrityKey(root)
	if err != nil {
		return Status{}, err
	}
	if err := validateResolvedBindings(root, integrityKey, catalog, request); err != nil {
		return Status{}, err
	}

	if current, err := inspect(root, request.AgentID, integrityKey); err == nil {
		if !matchesRequest(current.Instance, request, contract, templateSHA256) {
			return Status{}, errors.New("agent scaffold ID already belongs to a different immutable registration")
		}
		current.Existing = true
		return current, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return Status{}, err
	}

	created := time.Now().UTC()
	finalDirectory := instanceDirectory(request.AgentID)
	parentDirectory := filepath.Dir(finalDirectory)
	if err := root.MkdirAll(parentDirectory, 0o700); err != nil {
		return Status{}, err
	}
	if err := syncPathChain(root, parentDirectory); err != nil {
		return Status{}, err
	}
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return Status{}, err
	}
	stagingDirectory := filepath.Join(parentDirectory, ".scaffold-"+hex.EncodeToString(nonce))
	if err := root.Mkdir(stagingDirectory, 0o700); err != nil {
		return Status{}, err
	}
	defer root.RemoveAll(stagingDirectory)

	instance := Instance{
		SchemaVersion: 1, AgentID: request.AgentID, Role: request.Role,
		ScopeKind: request.ScopeKind, ScopeID: request.ScopeID,
		ParentAgentID: request.ParentAgent, ParentRole: request.ParentRole,
		AccountAgentID: request.AccountAgentID,
		Owner:          request.Owner, Mandate: strings.TrimSpace(request.Mandate),
		CanonPath: request.CanonPath, CanonSHA256: strings.ToLower(request.CanonSHA256),
		ExpertKind: request.ExpertKind, ExpertVersion: request.ExpertVersion,
		ExpertLifecycle: request.ExpertLifecycle,
		DisplayName:     request.DisplayName, Emoji: request.Emoji,
		OwnerID: request.OwnerID, OwnershipScope: request.OwnershipScope,
		InputContract: contract.InputContract, ToolAccess: contract.ToolAccess,
		MayDelegate:       contract.MayDelegate,
		DefinitionPath:    filepath.ToSlash(filepath.Join(finalDirectory, "AGENT.md")),
		DefinitionSHA256:  templateSHA256,
		RegistrationState: "scaffolded", RuntimeState: "unavailable",
		CreatedAt: created,
	}
	state := OperationalState{
		SchemaVersion: 1, AgentID: request.AgentID,
		Lifecycle: "scaffolded", RuntimeState: "unavailable", UpdatedAt: created,
	}
	stateBody, err := json.Marshal(state)
	if err != nil {
		return Status{}, err
	}
	stateDigest := sha256.Sum256(stateBody)
	instance.StateSHA256 = hex.EncodeToString(stateDigest[:])
	instance.HMACSHA256, err = signInstance(instance, integrityKey)
	if err != nil {
		return Status{}, err
	}
	for name, value := range map[string]any{
		"AGENT.md":      template,
		"state.json":    state,
		"instance.json": instance,
	} {
		if err := writeStagingFile(root, filepath.Join(stagingDirectory, name), value); err != nil {
			return Status{}, err
		}
	}
	if err := syncDirectory(root, stagingDirectory); err != nil {
		return Status{}, err
	}
	if err := root.Rename(stagingDirectory, finalDirectory); err != nil {
		if current, inspectErr := inspect(root, request.AgentID, integrityKey); inspectErr == nil &&
			matchesRequest(current.Instance, request, contract, templateSHA256) {
			current.Existing = true
			return current, nil
		}
		return Status{}, err
	}
	if err := syncDirectory(root, parentDirectory); err != nil {
		return Status{}, err
	}
	return inspect(root, request.AgentID, integrityKey)
}

func Inspect(dataRoot, agentID string) (Status, error) {
	if !agentcatalog.ValidAgentID(agentID) {
		return Status{}, errors.New("agent scaffold ID is invalid")
	}
	root, err := os.OpenRoot(dataRoot)
	if err != nil {
		return Status{}, err
	}
	defer root.Close()
	integrityKey, err := readIntegrityKey(root)
	if err != nil {
		return Status{}, err
	}
	status, err := inspect(root, agentID, integrityKey)
	if err != nil {
		return Status{}, err
	}
	catalog, err := baseagents.Catalog()
	if err != nil {
		return Status{}, err
	}
	request := Request{
		AgentID: status.Instance.AgentID, Role: status.Instance.Role,
		ScopeKind: status.Instance.ScopeKind, ScopeID: status.Instance.ScopeID,
		ParentAgent: status.Instance.ParentAgentID, ParentRole: status.Instance.ParentRole,
		AccountAgentID: status.Instance.AccountAgentID,
		Owner:          status.Instance.Owner, Mandate: status.Instance.Mandate,
		CanonPath: status.Instance.CanonPath, CanonSHA256: status.Instance.CanonSHA256,
		ExpertKind: status.Instance.ExpertKind, ExpertVersion: status.Instance.ExpertVersion,
		ExpertLifecycle: status.Instance.ExpertLifecycle,
		DisplayName:     status.Instance.DisplayName, Emoji: status.Instance.Emoji,
		OwnerID: status.Instance.OwnerID, OwnershipScope: status.Instance.OwnershipScope,
	}
	if err := validateResolvedBindings(root, integrityKey, catalog, request); err != nil {
		return Status{}, err
	}
	return status, nil
}

// ListPAExperts returns only fully validated, signed PA Expert registrations.
// A malformed or tampered registration fails the whole listing rather than
// silently changing the routing registry.
func ListPAExperts(dataRoot string) ([]Instance, error) {
	root, err := os.OpenRoot(dataRoot)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	directory, err := root.Open(filepath.Join("agents", "instances"))
	if errors.Is(err, os.ErrNotExist) {
		return []Instance{}, nil
	}
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
	integrityKey, err := readIntegrityKey(root)
	if err != nil {
		return nil, err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	instances := []Instance{}
	for _, entry := range entries {
		if !entry.IsDir() || !agentcatalog.ValidAgentID(entry.Name()) {
			return nil, errors.New("agent registry contains an invalid instance entry")
		}
		status, err := inspect(root, entry.Name(), integrityKey)
		if err != nil {
			return nil, err
		}
		if status.Instance.Role == "pa_expert" {
			instances = append(instances, status.Instance)
		}
	}
	return instances, nil
}

func inspect(root *os.Root, agentID string, integrityKey []byte) (Status, error) {
	if !agentcatalog.ValidAgentID(agentID) {
		return Status{}, errors.New("agent scaffold ID is invalid")
	}
	directory := instanceDirectory(agentID)
	var instance Instance
	if err := readStrictJSON(root, filepath.Join(directory, "instance.json"), &instance); err != nil {
		return Status{}, err
	}
	if err := verifyInstanceSignature(instance, integrityKey); err != nil {
		return Status{}, err
	}
	catalog, err := baseagents.Catalog()
	if err != nil {
		return Status{}, err
	}
	request := Request{
		AgentID: instance.AgentID, Role: instance.Role,
		ScopeKind: instance.ScopeKind, ScopeID: instance.ScopeID,
		ParentAgent: instance.ParentAgentID, ParentRole: instance.ParentRole,
		AccountAgentID: instance.AccountAgentID,
		Owner:          instance.Owner, Mandate: instance.Mandate,
		CanonPath: instance.CanonPath, CanonSHA256: instance.CanonSHA256,
		ExpertKind: instance.ExpertKind, ExpertVersion: instance.ExpertVersion,
		ExpertLifecycle: instance.ExpertLifecycle,
		DisplayName:     instance.DisplayName, Emoji: instance.Emoji,
		OwnerID: instance.OwnerID, OwnershipScope: instance.OwnershipScope,
	}
	contract, err := validateRequest(catalog, request)
	if err != nil || instance.SchemaVersion != 1 ||
		instance.InputContract != contract.InputContract ||
		instance.ToolAccess != contract.ToolAccess ||
		instance.MayDelegate != contract.MayDelegate ||
		instance.RegistrationState != "scaffolded" ||
		instance.RuntimeState != "unavailable" || instance.CreatedAt.IsZero() ||
		instance.DefinitionPath != filepath.ToSlash(filepath.Join(directory, "AGENT.md")) {
		return Status{}, errors.New("agent scaffold manifest violates its governed role contract")
	}
	definition, err := root.ReadFile(filepath.Join(directory, "AGENT.md"))
	if err != nil {
		return Status{}, err
	}
	definitionDigest := sha256.Sum256(definition)
	definitionSHA256 := hex.EncodeToString(definitionDigest[:])
	managedTemplate, err := baseagents.Template(instance.Role)
	if err != nil {
		return Status{}, err
	}
	managedDigest := sha256.Sum256(managedTemplate)
	if definitionSHA256 != instance.DefinitionSHA256 ||
		instance.DefinitionSHA256 != hex.EncodeToString(managedDigest[:]) {
		return Status{}, errors.New("agent scaffold definition does not match its immutable registration")
	}
	var state OperationalState
	if err := readStrictJSON(root, filepath.Join(directory, "state.json"), &state); err != nil {
		return Status{}, err
	}
	if state.SchemaVersion != 1 || state.AgentID != agentID ||
		state.Lifecycle != "scaffolded" || state.RuntimeState != "unavailable" ||
		state.UpdatedAt.IsZero() {
		return Status{}, errors.New("agent scaffold operational state is invalid")
	}
	stateBody, err := json.Marshal(state)
	if err != nil {
		return Status{}, err
	}
	stateDigest := sha256.Sum256(stateBody)
	stateSHA256 := hex.EncodeToString(stateDigest[:])
	if stateSHA256 != instance.StateSHA256 {
		return Status{}, errors.New("agent scaffold state does not match its signed registration")
	}
	return Status{
		Initialized: true, Instance: instance,
		Definition: Pointer{
			Path: instance.DefinitionPath, Available: true, SHA256: definitionSHA256,
		},
		State: Pointer{
			Path:      filepath.ToSlash(filepath.Join(directory, "state.json")),
			Available: true, SHA256: stateSHA256,
		},
	}, nil
}

func validateRequest(catalog agentcatalog.Catalog, request Request) (agentcatalog.RoleContract, error) {
	canonicalRole := catalog.CanonicalRole(request.Role)
	if err := agentidentity.ValidateSelection(agentidentity.Selection{
		Role: canonicalRole, AgentID: request.AgentID, DisplayName: request.DisplayName,
		Emoji: request.Emoji, OwnerID: request.OwnerID, OwnershipScope: request.OwnershipScope,
	}); err != nil {
		return agentcatalog.RoleContract{}, err
	}
	if !agentcatalog.ValidAgentID(request.AgentID) ||
		!agentcatalog.ValidAgentID(request.ScopeID) ||
		!agentcatalog.ValidAgentID(request.ParentAgent) {
		return agentcatalog.RoleContract{}, errors.New("agent scaffold identities must be path-safe lowercase slugs")
	}
	contract, ok := catalog.ContractForRole(canonicalRole)
	if !ok || contract.DirectUserAccess {
		return agentcatalog.RoleContract{}, errors.New("agent scaffold role is unsupported")
	}
	if canonicalRole != "case_agent" && request.AccountAgentID != "" {
		return agentcatalog.RoleContract{}, errors.New("only a case agent may declare a Client Account Agent relation")
	}
	if canonicalRole != "pa_expert" && request.ExpertLifecycle != "" {
		return agentcatalog.RoleContract{}, errors.New("only a PA expert may declare a PA Expert registry lifecycle")
	}
	hasRootMetadata := request.Owner != "" || strings.TrimSpace(request.Mandate) != "" ||
		request.CanonPath != "" || request.CanonSHA256 != "" ||
		request.ExpertKind != "" || request.ExpertVersion != "" || request.ExpertLifecycle != ""
	switch canonicalRole {
	case "practice_agent":
		if request.AgentID != "practice-agent-"+request.ScopeID ||
			request.ScopeKind != "practice" ||
			request.ParentAgent != "maestro" || request.ParentRole != "hub" ||
			!agentcatalog.ValidAgentID(request.Owner) ||
			strings.TrimSpace(request.Mandate) == "" || len([]byte(strings.TrimSpace(request.Mandate))) > 500 ||
			request.CanonPath == "" || !validSHA256(request.CanonSHA256) ||
			!catalog.AllowsDelegation("hub", "practice_agent", 1) {
			return agentcatalog.RoleContract{}, errors.New("practice agent scaffold requires an owner, bounded mandate, verified canon and exact Maestro-owned practice scope")
		}
	case "client_account_agent":
		if request.AgentID != "client-account-agent-"+request.ScopeID && request.AgentID != "account-agent-"+request.ScopeID ||
			request.ScopeKind != "account" ||
			request.ParentAgent != "maestro" || request.ParentRole != "hub" ||
			!agentcatalog.ValidAgentID(request.Owner) ||
			strings.TrimSpace(request.Mandate) == "" || len([]byte(strings.TrimSpace(request.Mandate))) > 500 ||
			request.CanonPath != "" || request.CanonSHA256 != "" ||
			request.ExpertKind != "" || request.ExpertVersion != "" ||
			!catalog.AllowsDelegation("hub", "client_account_agent", 1) {
			return agentcatalog.RoleContract{}, errors.New("client account agent scaffold requires an owner, bounded mandate and exact Maestro-owned account scope")
		}
	case "case_agent":
		if request.AgentID != "case-agent-"+request.ScopeID && request.AgentID != "workspace-agent-"+request.ScopeID ||
			(request.ScopeKind != "case" && request.ScopeKind != "workspace") ||
			request.ParentAgent != "maestro" || request.ParentRole != "hub" ||
			(request.ScopeKind == "case" && !agentcatalog.ValidAgentID(request.AccountAgentID)) ||
			(request.ScopeKind == "case" && hasRootMetadata) ||
			!catalog.AllowsDelegation("hub", "case_agent", 1) {
			return agentcatalog.RoleContract{}, errors.New("case agent scaffold requires Maestro ownership and a registered Client Account Agent relation for case scope")
		}
	case "pa_expert":
		if request.AgentID != "pa-expert-"+strings.ToLower(request.ExpertKind)+"-"+request.ScopeID ||
			request.ScopeKind != "practice" ||
			request.ParentAgent != "maestro" || request.ParentRole != "hub" ||
			!agentcatalog.ValidAgentID(request.Owner) ||
			strings.TrimSpace(request.Mandate) == "" || len([]byte(strings.TrimSpace(request.Mandate))) > 500 ||
			request.CanonPath == "" || !validSHA256(request.CanonSHA256) ||
			(request.ExpertKind != "FPA" && request.ExpertKind != "IPA") ||
			!validExpertVersion(request.ExpertVersion) ||
			request.ExpertLifecycle != "draft" ||
			!catalog.AllowsDelegation("hub", "pa_expert", 1) {
			return agentcatalog.RoleContract{}, errors.New("PA expert scaffold requires a PA Expert curator, FPA/IPA kind, semantic version, bounded mandate and verified canon")
		}
	case "capability_specialist":
		validParent := (catalog.CanonicalRole(request.ParentRole) == "case_agent" && request.ScopeKind == "workspace")
		if !strings.HasPrefix(request.AgentID, "capability-") || hasRootMetadata || !validParent ||
			!catalog.AllowsDelegation(request.ParentRole, "capability_specialist", 2) {
			return agentcatalog.RoleContract{}, errors.New("capability specialist scaffold has an invalid parent or scope")
		}
	case "subject_specialist":
		if !strings.HasPrefix(request.AgentID, "subject-") ||
			hasRootMetadata ||
			request.ParentRole != "practice_agent" || request.ScopeKind != "practice" ||
			!catalog.AllowsDelegation(request.ParentRole, request.Role, 2) {
			return agentcatalog.RoleContract{}, errors.New("subject specialist scaffold has an invalid practice parent or scope")
		}
	default:
		return agentcatalog.RoleContract{}, errors.New("agent role has no managed scaffold template")
	}
	return contract, nil
}

func validateResolvedBindings(root *os.Root, integrityKey []byte, catalog agentcatalog.Catalog, request Request) error {
	if catalog.CanonicalRole(request.Role) == "case_agent" && request.ScopeKind == "workspace" {
		if !managedAgentHasRole(catalog, request.ParentAgent, request.ParentRole) {
			return errors.New("workspace agent scaffold parent is not the registered Maestro hub")
		}
		return validateWorkspaceScope(root, request.ScopeID)
	}
	if catalog.CanonicalRole(request.Role) == "case_agent" && request.ScopeKind == "case" {
		if !managedAgentHasRole(catalog, request.ParentAgent, request.ParentRole) {
			return errors.New("case agent parent is not the registered Maestro hub")
		}
		account, err := inspect(root, request.AccountAgentID, integrityKey)
		if err != nil || account.Instance.Role != "client_account_agent" ||
			account.Instance.ScopeKind != "account" {
			return errors.New("case agent account relation is not a valid registered Client Account Agent")
		}
		return nil
	}
	if catalog.CanonicalRole(request.Role) == "practice_agent" ||
		catalog.CanonicalRole(request.Role) == "client_account_agent" || catalog.CanonicalRole(request.Role) == "pa_expert" {
		if !managedAgentHasRole(catalog, request.ParentAgent, request.ParentRole) {
			return errors.New("root agent parent is not the registered Maestro hub")
		}
		if request.Role == "practice_agent" {
			return validatePracticeCanon(root, request.ScopeID, request.CanonPath, request.CanonSHA256)
		}
		if request.Role == "pa_expert" {
			return validatePAExpertCanon(root, request.AgentID, request.CanonPath, request.CanonSHA256)
		}
		return nil
	}

	parent, err := inspect(root, request.ParentAgent, integrityKey)
	if err != nil {
		return errors.New("specialist scaffold parent is not a valid registered local instance")
	}
	if catalog.CanonicalRole(parent.Instance.Role) != catalog.CanonicalRole(request.ParentRole) ||
		parent.Instance.ScopeKind != request.ScopeKind ||
		parent.Instance.ScopeID != request.ScopeID {
		return errors.New("specialist scaffold parent does not share the declared role and immutable scope")
	}
	if parent.Instance.Role == "practice_agent" {
		if err := validatePracticeCanon(root, parent.Instance.ScopeID, parent.Instance.CanonPath, parent.Instance.CanonSHA256); err != nil {
			return err
		}
	}
	if request.ScopeKind == "workspace" {
		if err := validateWorkspaceScope(root, request.ScopeID); err != nil {
			return err
		}
	}
	return nil
}

func validatePracticeCanon(root *os.Root, practiceID, canonPath, expectedSHA256 string) error {
	cleaned := filepath.Clean(canonPath)
	slashed := filepath.ToSlash(cleaned)
	prefix := "practices/" + practiceID + "/"
	if filepath.IsAbs(cleaned) || slashed == "." || !strings.HasPrefix(slashed, prefix) ||
		len(slashed) <= len(prefix) || !validSHA256(expectedSHA256) {
		return errors.New("practice canon must be a specific artifact inside the registered practice scope")
	}
	practiceRoot, err := root.OpenRoot(filepath.Join("practices", practiceID))
	if err != nil {
		return errors.New("practice canon scope is unavailable")
	}
	defer practiceRoot.Close()
	relative := strings.TrimPrefix(slashed, prefix)
	file, err := practiceRoot.Open(filepath.FromSlash(relative))
	if err != nil {
		return errors.New("practice canon is unavailable")
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return errors.New("practice canon must be a regular scoped artifact")
	}
	digest := sha256.New()
	if _, err := io.Copy(digest, file); err != nil {
		return err
	}
	actual := hex.EncodeToString(digest.Sum(nil))
	if !hmac.Equal([]byte(actual), []byte(strings.ToLower(expectedSHA256))) {
		return errors.New("practice canon hash does not match the registered artifact")
	}
	return nil
}

func validatePAExpertCanon(root *os.Root, expertID, canonPath, expectedSHA256 string) error {
	cleaned := filepath.Clean(canonPath)
	slashed := filepath.ToSlash(cleaned)
	prefix := "pa-experts/" + expertID + "/"
	if filepath.IsAbs(cleaned) || slashed == "." || !strings.HasPrefix(slashed, prefix) ||
		len(slashed) <= len(prefix) || !validSHA256(expectedSHA256) {
		return errors.New("PA expert canon must be a specific artifact inside its registry scope")
	}
	expertRoot, err := root.OpenRoot(filepath.Join("pa-experts", expertID))
	if err != nil {
		return errors.New("PA Expert canon scope is unavailable")
	}
	defer expertRoot.Close()
	file, err := expertRoot.Open(filepath.FromSlash(strings.TrimPrefix(slashed, prefix)))
	if err != nil {
		return errors.New("PA Expert canon is unavailable")
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return errors.New("PA Expert canon must be a regular scoped artifact")
	}
	digest := sha256.New()
	if _, err := io.Copy(digest, file); err != nil {
		return err
	}
	actual := hex.EncodeToString(digest.Sum(nil))
	if !hmac.Equal([]byte(actual), []byte(strings.ToLower(expectedSHA256))) {
		return errors.New("PA Expert canon hash does not match the registered artifact")
	}
	return nil
}

func managedAgentHasRole(catalog agentcatalog.Catalog, agentID, role string) bool {
	for _, agent := range catalog.Agents {
		if agent.ID == agentID {
			return catalog.CanonicalRole(agent.Role) == catalog.CanonicalRole(role)
		}
	}
	return false
}

func validateWorkspaceScope(root *os.Root, workspaceID string) error {
	var registration struct {
		SchemaVersion int    `json:"schema_version"`
		WorkspaceID   string `json:"workspace_id"`
		AgentID       string `json:"agent_id"`
		Role          string `json:"role"`
	}
	path := filepath.Join("workspaces", workspaceID, "agent", "agent.json")
	if err := readStrictJSON(root, path, &registration); err != nil {
		return errors.New("agent scaffold workspace scope is not initialized")
	}
	canonicalAgentID := "workspace-agent-" + workspaceID
	if registration.SchemaVersion != 1 ||
		registration.WorkspaceID != workspaceID ||
		registration.AgentID != canonicalAgentID ||
		(registration.Role != "" && registration.Role != "case_agent" && registration.Role != "workspace_agent") {
		return errors.New("agent scaffold workspace scope does not match its registered gatekeeper")
	}
	return nil
}

func matchesRequest(instance Instance, request Request, contract agentcatalog.RoleContract, templateSHA256 string) bool {
	return instance.AgentID == request.AgentID &&
		instance.Role == request.Role &&
		instance.ScopeKind == request.ScopeKind &&
		instance.ScopeID == request.ScopeID &&
		instance.ParentAgentID == request.ParentAgent &&
		instance.ParentRole == request.ParentRole &&
		instance.AccountAgentID == request.AccountAgentID &&
		instance.Owner == request.Owner &&
		instance.Mandate == strings.TrimSpace(request.Mandate) &&
		instance.CanonPath == request.CanonPath &&
		instance.CanonSHA256 == strings.ToLower(request.CanonSHA256) &&
		instance.ExpertKind == request.ExpertKind &&
		instance.ExpertVersion == request.ExpertVersion &&
		instance.ExpertLifecycle == request.ExpertLifecycle &&
		instance.DisplayName == request.DisplayName && instance.Emoji == request.Emoji &&
		instance.OwnerID == request.OwnerID && instance.OwnershipScope == request.OwnershipScope &&
		instance.InputContract == contract.InputContract &&
		instance.ToolAccess == contract.ToolAccess &&
		instance.MayDelegate == contract.MayDelegate &&
		instance.DefinitionSHA256 == templateSHA256 &&
		validSHA256(instance.StateSHA256) &&
		instance.RegistrationState == "scaffolded" &&
		instance.RuntimeState == "unavailable"
}

func applyIdentity(dataRoot string, request *Request) error {
	if profile, err := agentidentity.Load(dataRoot); err == nil {
		if selection, ok := agentidentity.Resolve(profile, request.Role, request.AgentID); ok {
			request.DisplayName, request.Emoji = selection.DisplayName, selection.Emoji
			request.OwnerID, request.OwnershipScope = selection.OwnerID, selection.OwnershipScope
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if request.DisplayName == "" || request.Emoji == "" || request.OwnerID == "" || request.OwnershipScope == "" {
		selection, ok := agentidentity.Default(request.Role)
		if !ok {
			return errors.New("agent role has no default personalization profile")
		}
		if request.DisplayName == "" {
			request.DisplayName = selection.DisplayName
		}
		if request.Emoji == "" {
			request.Emoji = selection.Emoji
		}
		if request.OwnerID == "" {
			request.OwnerID = selection.OwnerID
		}
		if request.OwnershipScope == "" {
			request.OwnershipScope = selection.OwnershipScope
		}
	}
	return nil
}

func signInstance(instance Instance, integrityKey []byte) (string, error) {
	instance.HMACSHA256 = ""
	body, err := json.Marshal(instance)
	if err != nil {
		return "", err
	}
	mac := hmac.New(sha256.New, integrityKey)
	if _, err := mac.Write(body); err != nil {
		return "", err
	}
	return hex.EncodeToString(mac.Sum(nil)), nil
}

func verifyInstanceSignature(instance Instance, integrityKey []byte) error {
	if !validSHA256(instance.HMACSHA256) {
		return errors.New("agent scaffold registration has an invalid authenticator")
	}
	actual := instance.HMACSHA256
	expected, err := signInstance(instance, integrityKey)
	if err != nil {
		return err
	}
	if !hmac.Equal([]byte(actual), []byte(expected)) {
		return errors.New("agent scaffold registration failed integrity validation")
	}
	return nil
}

func loadOrCreateIntegrityKey(root *os.Root) ([]byte, error) {
	key, err := readIntegrityKey(root)
	if err == nil {
		return key, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	exists, err := scaffoldInstancesExist(root)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, errors.New("agent scaffold integrity key is missing while instances exist; explicit recovery is required")
	}
	if err := root.MkdirAll("config", 0o700); err != nil {
		return nil, err
	}
	if err := syncPathChain(root, "config"); err != nil {
		return nil, err
	}
	key = make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	file, err := root.OpenFile(integrityKeyPath(), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return readIntegrityKey(root)
		}
		return nil, err
	}
	if _, err := file.Write(key); err != nil {
		file.Close()
		return nil, err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return nil, err
	}
	if err := file.Close(); err != nil {
		return nil, err
	}
	if err := syncDirectory(root, "config"); err != nil {
		return nil, err
	}
	return key, nil
}

func readIntegrityKey(root *os.Root) ([]byte, error) {
	key, err := root.ReadFile(integrityKeyPath())
	if err != nil {
		return nil, err
	}
	if len(key) != 32 {
		return nil, errors.New("agent scaffold integrity key is invalid")
	}
	return key, nil
}

func integrityKeyPath() string {
	return filepath.Join("config", "agent-scaffold-integrity.key")
}

func validSHA256(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size
}

func scaffoldInstancesExist(root *os.Root) (bool, error) {
	directory, err := root.Open(filepath.Join("agents", "instances"))
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer directory.Close()
	entries, err := directory.ReadDir(1)
	if errors.Is(err, io.EOF) {
		return false, nil
	}
	return len(entries) != 0, err
}

func syncPathChain(root *os.Root, directory string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	if err := syncDirectory(root, "."); err != nil {
		return err
	}
	current := ""
	for _, part := range strings.Split(filepath.Clean(directory), string(filepath.Separator)) {
		if part == "" || part == "." {
			continue
		}
		current = filepath.Join(current, part)
		if err := syncDirectory(root, current); err != nil {
			return err
		}
	}
	return nil
}

func syncDirectory(root *os.Root, directory string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	file, err := root.Open(directory)
	if err != nil {
		return err
	}
	defer file.Close()
	return file.Sync()
}

func instanceDirectory(agentID string) string {
	return filepath.Join("agents", "instances", agentID)
}

func writeStagingFile(root *os.Root, path string, value any) error {
	file, err := root.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	var body []byte
	switch typed := value.(type) {
	case []byte:
		body = typed
	default:
		body, err = json.MarshalIndent(value, "", "  ")
		if err == nil {
			body = append(body, '\n')
		}
	}
	if err == nil {
		_, err = file.Write(body)
	}
	if err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err != nil {
		return err
	}
	return closeErr
}

func readStrictJSON(root *os.Root, path string, target any) error {
	file, err := root.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	decoder := json.NewDecoder(io.LimitReader(file, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return fmt.Errorf("%s contains multiple JSON values", path)
		}
		return err
	}
	return nil
}
