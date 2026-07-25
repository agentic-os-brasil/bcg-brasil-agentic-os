package updateservice

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installtx"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releaseverify"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/updateplan"
)

var pendingPlanIDPattern = regexp.MustCompile(`^[a-f0-9]{32}$`)

type Pending struct {
	SchemaVersion      int             `json:"schema_version"`
	Plan               updateplan.Plan `json:"plan"`
	VerifiedDirectory  string          `json:"verified_directory"`
	ActivationPlanPath string          `json:"activation_plan_path"`
	CreatedAt          time.Time       `json:"created_at"`
}

func StagePending(dataRoot string, current installtx.State, result CheckResult, now time.Time) (Pending, error) {
	absoluteDataRoot, err := filepath.Abs(dataRoot)
	if err != nil {
		return Pending{}, err
	}
	absoluteDataRoot = filepath.Clean(absoluteDataRoot)
	if current.ManagedRoot == "" || result.Plan.ID == "" || result.Verified.Directory == "" {
		return Pending{}, errors.New("installed state and checked update are required")
	}
	if result.Plan.SchemaVersion != 2 ||
		result.Plan.FromRelease != current.Release ||
		result.Plan.FromChannel != current.Channel ||
		result.Plan.FromCLIVersion != current.CLIVersion ||
		result.Plan.FromBundleVersion != current.BundleVersion ||
		result.Plan.TargetOS != current.TargetOS ||
		result.Plan.TargetArch != current.TargetArch ||
		result.Plan.ManifestSHA256 != result.Verified.ManifestSHA256 {
		return Pending{}, errors.New("checked update does not match the installed state or verified release")
	}
	if !within(filepath.Join(absoluteDataRoot, "updates"), result.Verified.Directory) {
		return Pending{}, errors.New("verified release directory is outside owner-data update staging")
	}
	activationPlanPath, err := installtx.Prepare(result.Verified, installtx.PrepareOptions{
		Transition:         "update",
		ConfirmationPlanID: result.Plan.ID,
		FromRelease:        result.Plan.FromRelease,
		FromChannel:        result.Plan.FromChannel,
		FromCLIVersion:     result.Plan.FromCLIVersion,
		FromBundleVersion:  result.Plan.FromBundleVersion,
		TargetOS:           result.Plan.TargetOS,
		TargetArch:         result.Plan.TargetArch,
		ManagedRoot:        current.ManagedRoot,
		DataRoot:           absoluteDataRoot,
	})
	if err != nil {
		return Pending{}, err
	}
	pending := Pending{
		SchemaVersion: 1, Plan: result.Plan,
		VerifiedDirectory:  filepath.Clean(result.Verified.Directory),
		ActivationPlanPath: filepath.Clean(activationPlanPath),
		CreatedAt:          now.UTC(),
	}
	if err := validatePending(absoluteDataRoot, pending); err != nil {
		_ = os.RemoveAll(filepath.Dir(activationPlanPath))
		return Pending{}, err
	}
	if err := writePendingAtomic(pendingPath(absoluteDataRoot), pending); err != nil {
		_ = os.RemoveAll(filepath.Dir(activationPlanPath))
		return Pending{}, err
	}
	return pending, nil
}

func ConfirmPending(
	dataRoot, managedRoot, planID string,
	registry releaseverify.KeyRegistry,
) (Pending, releaseverify.VerifiedRelease, error) {
	if managedRoot == "" || !pendingPlanIDPattern.MatchString(planID) || registry == nil {
		return Pending{}, releaseverify.VerifiedRelease{}, errors.New("managed root, pending plan ID and release-key registry are required")
	}
	pending, err := LoadPending(dataRoot)
	if err != nil {
		return Pending{}, releaseverify.VerifiedRelease{}, err
	}
	if pending.Plan.ID != planID {
		return Pending{}, releaseverify.VerifiedRelease{}, errors.New("pending update confirmation does not match the stored plan")
	}
	current, err := installtx.ReadStateForManagedRoot(dataRoot, managedRoot)
	if err != nil {
		return Pending{}, releaseverify.VerifiedRelease{}, err
	}
	if pending.Plan.TargetOS != current.TargetOS || pending.Plan.TargetArch != current.TargetArch {
		return Pending{}, releaseverify.VerifiedRelease{}, errors.New("pending update target does not match the installed target")
	}
	verified, err := releaseverify.VerifyDirectory(pending.VerifiedDirectory, registry)
	if err != nil {
		return Pending{}, releaseverify.VerifiedRelease{}, err
	}
	if verified.ManifestSHA256 != pending.Plan.ManifestSHA256 {
		return Pending{}, releaseverify.VerifiedRelease{}, errors.New("pending update manifest no longer matches the confirmed plan")
	}
	recomputed, err := updateplan.Build(
		current, verified.Manifest, pending.Plan.TargetOS, pending.Plan.TargetArch,
		updateplan.SourceBinding{
			Provider: pending.Plan.Provider, ProviderReleaseID: pending.Plan.ProviderReleaseID,
			ManifestSHA256: verified.ManifestSHA256,
		},
	)
	if err != nil {
		return Pending{}, releaseverify.VerifiedRelease{}, err
	}
	if recomputed.ID != pending.Plan.ID {
		return Pending{}, releaseverify.VerifiedRelease{}, errors.New("pending update plan is stale")
	}
	if _, err := installtx.ValidatePrepared(
		pending.ActivationPlanPath,
		verified,
		installtx.PrepareOptions{
			Transition:         "update",
			ConfirmationPlanID: pending.Plan.ID,
			FromRelease:        pending.Plan.FromRelease,
			FromChannel:        pending.Plan.FromChannel,
			FromCLIVersion:     pending.Plan.FromCLIVersion,
			FromBundleVersion:  pending.Plan.FromBundleVersion,
			TargetOS:           current.TargetOS,
			TargetArch:         current.TargetArch,
			ManagedRoot:        current.ManagedRoot,
			DataRoot:           dataRoot,
		},
	); err != nil {
		return Pending{}, releaseverify.VerifiedRelease{}, fmt.Errorf("validate prepared activation: %w", err)
	}
	return pending, verified, nil
}

func ReconcilePending(
	dataRoot, managedRoot, planID string,
	registry releaseverify.KeyRegistry,
) (Pending, bool, error) {
	if managedRoot == "" || !pendingPlanIDPattern.MatchString(planID) || registry == nil {
		return Pending{}, false, errors.New("managed root, pending plan ID and release-key registry are required")
	}
	pending, err := LoadPending(dataRoot)
	if err != nil {
		return Pending{}, false, err
	}
	if pending.Plan.ID != planID {
		return Pending{}, false, errors.New("pending update reconciliation does not match the stored plan")
	}
	if err := updateplan.Validate(pending.Plan); err != nil {
		return Pending{}, false, err
	}
	verified, err := releaseverify.VerifyDirectory(pending.VerifiedDirectory, registry)
	if err != nil {
		return Pending{}, false, err
	}
	if verified.ManifestSHA256 != pending.Plan.ManifestSHA256 {
		return Pending{}, false, errors.New("pending update manifest no longer matches its plan")
	}
	options := installtx.PrepareOptions{
		Transition:         "update",
		ConfirmationPlanID: pending.Plan.ID,
		FromRelease:        pending.Plan.FromRelease,
		FromChannel:        pending.Plan.FromChannel,
		FromCLIVersion:     pending.Plan.FromCLIVersion,
		FromBundleVersion:  pending.Plan.FromBundleVersion,
		TargetOS:           pending.Plan.TargetOS,
		TargetArch:         pending.Plan.TargetArch,
		ManagedRoot:        managedRoot,
		DataRoot:           dataRoot,
	}
	if _, err := installtx.ValidatePrepared(pending.ActivationPlanPath, verified, options); err != nil {
		return Pending{}, false, fmt.Errorf("validate reconciled activation: %w", err)
	}
	reconciled, err := installtx.ReconcileActivated(
		pending.ActivationPlanPath,
		installtx.ActivateOptions{PrepareOptions: options},
	)
	if err != nil {
		return Pending{}, false, err
	}
	return pending, reconciled, nil
}

func LoadPending(dataRoot string) (Pending, error) {
	absoluteDataRoot, err := filepath.Abs(dataRoot)
	if err != nil {
		return Pending{}, err
	}
	var pending Pending
	if err := readPendingStrict(pendingPath(absoluteDataRoot), &pending); err != nil {
		return Pending{}, err
	}
	if err := validatePending(filepath.Clean(absoluteDataRoot), pending); err != nil {
		return Pending{}, err
	}
	return pending, nil
}

func RemovePending(dataRoot, planID string) error {
	pending, err := LoadPending(dataRoot)
	if err != nil {
		return err
	}
	if pending.Plan.ID != planID {
		return errors.New("cannot remove a different pending update")
	}
	absoluteDataRoot, err := filepath.Abs(dataRoot)
	if err != nil {
		return err
	}
	return os.Remove(pendingPath(filepath.Clean(absoluteDataRoot)))
}

func validatePending(dataRoot string, pending Pending) error {
	if pending.SchemaVersion != 1 ||
		pending.Plan.SchemaVersion != 2 ||
		!pendingPlanIDPattern.MatchString(pending.Plan.ID) ||
		pending.Plan.State != "available" ||
		!pending.Plan.ConfirmationRequired ||
		pending.CreatedAt.IsZero() ||
		pending.CreatedAt.Location() != time.UTC {
		return errors.New("invalid pending update envelope")
	}
	updatesRoot := filepath.Join(dataRoot, "updates")
	if !filepath.IsAbs(pending.VerifiedDirectory) ||
		!filepath.IsAbs(pending.ActivationPlanPath) ||
		!within(updatesRoot, pending.VerifiedDirectory) ||
		!within(updatesRoot, pending.ActivationPlanPath) ||
		filepath.Base(pending.ActivationPlanPath) != installtx.PlanName {
		return errors.New("pending update paths escape owner-data staging")
	}
	return nil
}

func pendingPath(dataRoot string) string {
	return filepath.Join(dataRoot, "config", "pending-update.json")
}

func within(root, candidate string) bool {
	relative, err := filepath.Rel(filepath.Clean(root), filepath.Clean(candidate))
	return err == nil && relative != "." && relative != ".." &&
		!strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func writePendingAtomic(path string, pending Pending) error {
	body, err := json.MarshalIndent(pending, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".pending-update-")
	if err != nil {
		return err
	}
	temporary := file.Name()
	defer os.Remove(temporary)
	if err := file.Chmod(0o600); err != nil {
		file.Close()
		return err
	}
	if _, err := file.Write(body); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(temporary, path)
}

func readPendingStrict(path string, pending *Pending) error {
	body, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if len(body) > 1<<20 {
		return errors.New("pending update exceeds 1 MiB")
	}
	if err := rejectDuplicatePendingJSONKeys(body); err != nil {
		return fmt.Errorf("decode pending update: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(pending); err != nil {
		return fmt.Errorf("decode pending update: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("pending update contains trailing content")
	}
	return nil
}

func rejectDuplicatePendingJSONKeys(body []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(body))
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	if err := walkPendingJSONValue(decoder, token); err != nil {
		return err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple JSON values")
		}
		return err
	}
	return nil
}

func walkPendingJSONValue(decoder *json.Decoder, token json.Token) error {
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delimiter {
	case '{':
		seen := map[string]bool{}
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return errors.New("object key is not a string")
			}
			if seen[key] {
				return fmt.Errorf("duplicate JSON object key %q", key)
			}
			seen[key] = true
			valueToken, err := decoder.Token()
			if err != nil {
				return err
			}
			if err := walkPendingJSONValue(decoder, valueToken); err != nil {
				return err
			}
		}
		end, err := decoder.Token()
		if err != nil {
			return err
		}
		if end != json.Delim('}') {
			return errors.New("unterminated JSON object")
		}
	case '[':
		for decoder.More() {
			valueToken, err := decoder.Token()
			if err != nil {
				return err
			}
			if err := walkPendingJSONValue(decoder, valueToken); err != nil {
				return err
			}
		}
		end, err := decoder.Token()
		if err != nil {
			return err
		}
		if end != json.Delim(']') {
			return errors.New("unterminated JSON array")
		}
	default:
		return fmt.Errorf("unexpected JSON delimiter %q", delimiter)
	}
	return nil
}
