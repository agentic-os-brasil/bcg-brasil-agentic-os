package agentscaffold

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	baseagents "github.com/agentic-os-brasil/bcg-brasil-agentic-os/bundles/base/agents"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/agentcatalog"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/agentidentity"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

// Enrollment is a noncreating, fully resolved local registration proposal.
// It is not release/publisher signing or a runtime/tool grant.
type Enrollment struct {
	Contract       agentcatalog.RoleContract `json:"contract"`
	DefinitionPath string                    `json:"definition_path"`
	Request        Request                   `json:"request"`
	TemplateSHA256 string                    `json:"template_sha256"`
	CatalogSHA256  string                    `json:"catalog_sha256"`
	Existing       *Instance                 `json:"existing,omitempty"`
}

func PreviewEnrollment(dataRoot string, request Request) (Enrollment, error) {
	files, e := scheduler.HoldDirectory(dataRoot, nil)
	if e != nil {
		return Enrollment{}, e
	}
	defer files.Close()
	return PreviewEnrollmentAt(files, request)
}
func PreviewEnrollmentAt(files *scheduler.HeldDirectory, request Request) (Enrollment, error) {
	var out Enrollment
	root := &scaffoldRoot{files: files}
	// An exact existing registration wins over presentation defaults. Its
	// immutable names are not silently re-signed from a fresh generic default.
	if prior, e := InspectAt(files, request.AgentID); e == nil {
		if request.DisplayName == "" {
			request.DisplayName = prior.Instance.DisplayName
		}
		if request.Emoji == "" {
			request.Emoji = prior.Instance.Emoji
		}
		if request.OwnerID == "" {
			request.OwnerID = prior.Instance.OwnerID
		}
		if request.OwnershipScope == "" {
			request.OwnershipScope = prior.Instance.OwnershipScope
		}
	} else if !errors.Is(e, os.ErrNotExist) {
		return out, e
	}
	catalog, err := baseagents.Catalog()
	if err != nil {
		return out, err
	}
	if err = applyIdentity(root, &request); err != nil {
		return out, err
	}
	request.Mandate = strings.TrimSpace(request.Mandate)
	request.Role = catalog.CanonicalRole(request.Role)
	request.ParentRole = catalog.CanonicalRole(request.ParentRole)
	if out.Contract, err = validateRequest(catalog, request); err != nil {
		return out, err
	}
	body, err := baseagents.Template(request.Role)
	if err != nil {
		return out, err
	}
	sum := sha256.Sum256(body)
	out.TemplateSHA256 = hex.EncodeToString(sum[:])
	body, err = json.Marshal(catalog)
	if err != nil {
		return out, err
	}
	sum = sha256.Sum256(body)
	out.CatalogSHA256 = hex.EncodeToString(sum[:])
	out.Request = request
	out.DefinitionPath = filepath.ToSlash(filepath.Join(instanceDirectory(request.AgentID), "AGENT.md"))
	key, err := readIntegrityKey(root)
	if errors.Is(err, os.ErrNotExist) {
		exists, e := scaffoldInstancesExist(root)
		if e != nil {
			return out, e
		}
		if exists {
			return out, errors.New("enrollment_key_missing_with_instances")
		}
		return out, nil
	}

	if err != nil {
		return out, err
	}
	prior, err := inspect(root, request.AgentID, key)
	if errors.Is(err, os.ErrNotExist) {
		if _, e := files.StatDirectory(instanceDirectory(request.AgentID)); e == nil {
			return out, errors.New("enrollment_incomplete_existing_instance")
		} else if !errors.Is(e, os.ErrNotExist) {
			return out, e
		}
		return out, nil
	}
	if err != nil {
		return out, err
	}
	contract, _ := catalog.ContractForRole(request.Role)
	if !matchesRequest(prior.Instance, request, contract, out.TemplateSHA256) {
		return out, errors.New("enrollment_immutable_conflict")
	}
	out.Existing = &prior.Instance
	return out, nil
}

func enrollmentIdentity(root *scaffoldRoot) (agentidentity.Profile, error) {
	body, err := root.files.ReadFile(root.rel("agents/personalization.json"), 1<<20, true)
	if err != nil {
		return agentidentity.Profile{}, err
	}
	var profile agentidentity.Profile
	if err = agentidentity.DecodeStrict(body, &profile); err != nil {
		return profile, err
	}
	return profile, profile.Validate()
}

// RegisterEnrollment recomputes the complete resolved proposal under the local
// enrollment guard. Only canonical timestamps/authenticators are generated later.
func RegisterEnrollment(path string, expected Enrollment) (Status, error) {
	files, e := scheduler.HoldDirectory(path, nil)
	if e != nil {
		return Status{}, e
	}
	defer files.Close()
	return RegisterEnrollmentAt(files, expected)
}
func RegisterEnrollmentAt(files *scheduler.HeldDirectory, expected Enrollment) (status Status, err error) {
	err = files.WithPrivateLock("config/agent-enrollment.lock", func() error {
		actual, e := PreviewEnrollmentAt(files, expected.Request)
		if e != nil {
			return e
		}
		a, _ := json.Marshal(actual)
		b, _ := json.Marshal(expected)
		if string(a) != string(b) {
			return errors.New("enrollment_review_changed")
		}
		status, e = scaffold(files, expected.Request, true)
		return e
	})
	return
}
func ValidateEnrollmentStore(path string) error {
	files, e := scheduler.HoldDirectory(path, nil)
	if e != nil {
		return e
	}
	defer files.Close()
	return ValidateEnrollmentStoreAt(files)
}
func ValidateEnrollmentStoreAt(files *scheduler.HeldDirectory) error {
	root := &scaffoldRoot{files: files}
	_, err := readIntegrityKey(root)
	if errors.Is(err, os.ErrNotExist) {
		exists, e := scaffoldInstancesExist(root)
		if e != nil {
			return e
		}
		if exists {
			return errors.New("enrollment_key_missing_with_instances")
		}
		return nil
	}
	return err
}
