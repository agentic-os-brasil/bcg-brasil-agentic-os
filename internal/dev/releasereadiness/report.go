// Package releasereadiness reports local release evidence without contacting a
// provider or mutating a release, credential store or workspace.
package releasereadiness

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/dev/releasepack"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releaseprovider"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releaseverify"
)

const (
	StatePass         = "pass"
	StateConfigured   = "configured"
	StateUnavailable  = "unavailable"
	StateBlocked      = "blocked"
	StateNotEvaluated = "not_evaluated"
	ReadinessClaim    = "local_contract_evidence"
	Capability        = "release_readiness"
)

// Options identifies only explicit, read-only inputs. Empty optional paths
// remain unavailable; the evaluator never searches the current directory.
type Options struct {
	Root                    string
	ProviderConfig          string
	AuthorityRegistry       string
	AuthorityRegistrySHA256 string
	Candidate               string
	Clock                   func() time.Time
}

type Check struct {
	ID         string `json:"id"`
	State      string `json:"state"`
	Reason     string `json:"reason"`
	NextAction string `json:"next_action,omitempty"`
}

type Report struct {
	SchemaVersion  int      `json:"schema_version"`
	Capability     string   `json:"capability"`
	ReadinessClaim string   `json:"readiness_claim"`
	State          string   `json:"state"`
	Promotion      string   `json:"promotion"`
	Checks         []Check  `json:"checks"`
	Blockers       []string `json:"blockers"`
}

// Evaluate validates the local release contracts and any explicitly supplied
// candidate/authority inputs. It never authenticates, signs, publishes or
// installs a release. Report ordering and blocker codes are stable.
func Evaluate(options Options) Report {
	if options.Clock == nil {
		options.Clock = time.Now
	}
	checks := []Check{
		evaluateContract(options.Root),
		evaluateProvider(options.ProviderConfig),
		evaluateAuthority(options),
		evaluateCandidate(options),
		evaluateWorkflows(options.Root),
		{ID: "signature", State: StateNotEvaluated, Reason: "detached release signatures are not evaluated by the local readiness report", NextAction: "run the protected signed-release workflow with approved signing authority"},
		{ID: "provider_auth", State: StateNotEvaluated, Reason: "provider authentication and publication are not evaluated locally", NextAction: "configure and review the authenticated private-release provider"},
		{ID: "clean_device", State: StateUnavailable, Reason: "no clean managed-device install, update and rollback evidence was supplied", NextAction: "run the approved Windows and macOS clean-device acceptance"},
		{ID: "runtime_pack", State: StateUnavailable, Reason: "the verified ingestion runtime pack is not part of release manifest v1", NextAction: "qualify a separately versioned runtime pack before pilot use"},
	}
	blockers := make([]string, 0, len(checks))
	for _, check := range checks {
		if check.State == StatePass || check.State == StateConfigured {
			continue
		}
		blockers = append(blockers, check.ID+"_"+check.State)
	}
	sort.Strings(blockers)
	return Report{
		SchemaVersion:  1,
		Capability:     Capability,
		ReadinessClaim: ReadinessClaim,
		State:          ReadinessClaim,
		Promotion:      "not_permitted",
		Checks:         checks,
		Blockers:       blockers,
	}
}

func evaluateContract(root string) Check {
	if root == "" {
		return Check{ID: "contract", State: StateBlocked, Reason: "repository root is required", NextAction: "run from a repository resolved by the development harness"}
	}
	checks := []error{
		releasecontract.ValidateSchemaFile(filepath.Join(root, "schemas", "release-manifest.schema.json")),
		releaseverify.ValidateAuthorityRegistrySchemaFile(filepath.Join(root, "schemas", "release-authority-registry.schema.json")),
	}
	providerSchema, err := os.Open(filepath.Join(root, "schemas", "release-provider.schema.json"))
	if err != nil {
		checks = append(checks, err)
	} else {
		checks = append(checks, releaseprovider.ValidateProviderConfigSchema(providerSchema))
		_ = providerSchema.Close()
	}
	for _, checkErr := range checks {
		if checkErr != nil {
			return Check{ID: "contract", State: StateBlocked, Reason: "one or more managed release schemas failed validation", NextAction: "repair the repository contract before producing a candidate"}
		}
	}
	return Check{ID: "contract", State: StatePass, Reason: "manifest, provider and authority schemas pass their executable validators"}
}

func evaluateProvider(path string) Check {
	if path == "" {
		return Check{ID: "provider_config", State: StateUnavailable, Reason: "no provider configuration path was supplied", NextAction: "supply --provider-config with the approved public provider configuration"}
	}
	body, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Check{ID: "provider_config", State: StateUnavailable, Reason: "provider configuration is not present", NextAction: "supply the approved public provider configuration"}
		}
		return Check{ID: "provider_config", State: StateBlocked, Reason: "provider configuration could not be read", NextAction: "repair the provider configuration path and permissions"}
	}
	config, err := releaseprovider.ParseConfig(bytes.NewReader(body))
	if err != nil {
		return Check{ID: "provider_config", State: StateBlocked, Reason: "provider configuration failed contract validation", NextAction: "repair or replace the provider configuration"}
	}
	if !config.Approved() {
		return Check{ID: "provider_config", State: StateUnavailable, Reason: "provider registration is explicitly unavailable", NextAction: "complete the approved private-provider registration outside the repository"}
	}
	return Check{ID: "provider_config", State: StateConfigured, Reason: "provider configuration is structurally approved; authentication remains unevaluated"}
}

func evaluateAuthority(options Options) Check {
	if options.AuthorityRegistry == "" {
		return Check{ID: "authority_registry", State: StateUnavailable, Reason: "no authority registry path was supplied", NextAction: "supply --authority-registry and its exact SHA-256 pin"}
	}
	if options.AuthorityRegistrySHA256 == "" {
		return Check{ID: "authority_registry", State: StateUnavailable, Reason: "authority registry digest pin was not supplied", NextAction: "supply --authority-registry-sha256 from the approved seed"}
	}
	if _, err := os.Stat(options.AuthorityRegistry); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Check{ID: "authority_registry", State: StateUnavailable, Reason: "authority registry is not present", NextAction: "supply the approved public authority registry"}
		}
		return Check{ID: "authority_registry", State: StateBlocked, Reason: "authority registry could not be read", NextAction: "repair the authority registry path and permissions"}
	}
	if !isSHA256(options.AuthorityRegistrySHA256) {
		return Check{ID: "authority_registry", State: StateBlocked, Reason: "authority registry digest pin is not canonical SHA-256", NextAction: "replace the digest with the exact lowercase SHA-256 of the approved registry"}
	}
	registry, err := releaseverify.LoadPinnedAuthorityRegistry(options.AuthorityRegistry, options.AuthorityRegistrySHA256, options.Clock)
	if err != nil {
		return Check{ID: "authority_registry", State: StateBlocked, Reason: "authority registry failed digest or contract validation", NextAction: "replace the registry or its pin from the approved authority source"}
	}
	if !registry.HasActiveKey() {
		return Check{ID: "authority_registry", State: StateBlocked, Reason: "authority registry has no active key inside its validity window", NextAction: "supply an approved registry with a currently active release key"}
	}
	if options.Candidate != "" {
		manifestBody, err := os.ReadFile(filepath.Join(options.Candidate, releasepack.ManifestName))
		if err != nil {
			return Check{ID: "authority_registry", State: StateBlocked, Reason: "candidate manifest could not be read for authority matching", NextAction: "rebuild the candidate before checking authority coverage"}
		}
		manifest, err := releasecontract.Parse(bytes.NewReader(manifestBody))
		if err != nil {
			return Check{ID: "authority_registry", State: StateBlocked, Reason: "candidate manifest failed semantic validation before authority matching", NextAction: "rebuild the candidate from the exact reviewed source snapshot"}
		}
		if _, ok := registry.Lookup(manifest.Product, manifest.Issuer.ID, manifest.Issuer.KeyID); !ok {
			return Check{ID: "authority_registry", State: StateBlocked, Reason: "candidate issuer is not active in the pinned authority registry", NextAction: "use an approved active issuer and key for the release"}
		}
	}
	return Check{ID: "authority_registry", State: StateConfigured, Reason: "authority registry matches its supplied digest and passes structural validation; signature trust remains unevaluated"}
}

func evaluateCandidate(options Options) Check {
	if options.Candidate == "" {
		return Check{ID: "candidate_manifest", State: StateUnavailable, Reason: "no candidate directory was supplied", NextAction: "supply --candidate with an explicit unsigned candidate directory"}
	}
	if _, err := os.Stat(options.Candidate); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Check{ID: "candidate_manifest", State: StateUnavailable, Reason: "candidate directory is not present", NextAction: "build a deterministic candidate before checking its closure"}
		}
		return Check{ID: "candidate_manifest", State: StateBlocked, Reason: "candidate directory could not be read", NextAction: "repair the candidate path and permissions"}
	}
	if err := releasepack.VerifyCandidate(options.Candidate); err != nil {
		return Check{ID: "candidate_manifest", State: StateBlocked, Reason: "candidate manifest or artifact closure failed validation", NextAction: "rebuild the candidate from the exact reviewed source snapshot"}
	}
	return Check{ID: "candidate_manifest", State: StateConfigured, Reason: "unsigned candidate manifest and artifact closure pass locally; authenticity is not established"}
}

func evaluateWorkflows(root string) Check {
	paths := []string{
		filepath.Join(root, ".github", "workflows", "release-candidate.yml"),
		filepath.Join(root, ".github", "workflows", "signed-prerelease.yml"),
	}
	for _, path := range paths {
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() {
			return Check{ID: "release_workflows", State: StateBlocked, Reason: "required release workflow is missing or not a regular file", NextAction: "restore the reviewed release workflow definitions"}
		}
		body, err := os.ReadFile(path)
		if err != nil || !strings.Contains(string(body), "workflow_dispatch:") {
			return Check{ID: "release_workflows", State: StateBlocked, Reason: "required release workflow is not dispatchable", NextAction: "restore workflow_dispatch on the reviewed release workflows"}
		}
	}
	return Check{ID: "release_workflows", State: StatePass, Reason: "candidate and signed-release workflows are present and dispatchable"}
}

func isSHA256(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size && strings.ToLower(value) == value
}
