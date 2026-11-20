package setup

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	product "github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp/packaging"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installer"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installtx"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/maintenance"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ownerctx"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

const modelRegistrationTokens = 2048

var ErrModelProfilePreparationRequired = errors.New("review and prepare the dedicated model profile before registering")
var ErrModelProfileLoginRequired = errors.New("use the dedicated model profile's official login before registering")

type ModelRegistrationSelection struct {
	JobID, Provider, Model string
	OwnerPolicy            ownerctx.WeeklySelfOptions
}

// ModelProfileReview describes only private profile preparation. It approves
// neither authentication, a model call, a registration nor activation.
type ModelProfileReview struct {
	CarrierID          string   `json:"carrier_id"`
	ClientRecordSHA256 string   `json:"client_record_sha256"`
	Executable         string   `json:"executable"`
	ExecutableSHA256   string   `json:"executable_sha256"`
	Root               string   `json:"root"`
	ProfileDirectory   string   `json:"profile_directory"`
	WorkingDirectory   string   `json:"working_directory"`
	Prepared           bool     `json:"prepared"`
	Effects            []string `json:"effects"`
	Digest             string   `json:"review_digest"`
}

type ModelProfileStatus struct {
	ProfileDirectory string `json:"profile_directory"`
	WorkingDirectory string `json:"working_directory"`
	Prepared         bool   `json:"prepared"`
	Authentication   string `json:"authentication"`
	Reason           string `json:"reason,omitempty"`
}

// ModelRegistrationReview retains the original server review plus every
// measured input needed to reproduce it. A staged declaration is a bounded
// private proposal, not a qualification or execution grant.
type ModelRegistrationReview struct {
	Selection               ModelRegistrationSelection          `json:"selection"`
	Profile                 ModelProfileReview                  `json:"profile"`
	Observation             packaging.WeeklyRuntimeObservation  `json:"observation"`
	Registration            maintenance.WeeklyModelRegistration `json:"registration"`
	RegistrationFile        string                              `json:"registration_file"`
	RegistrationSHA256      string                              `json:"registration_sha256"`
	Review                  maintenance.ProductReview           `json:"review"`
	MaintenanceReviewDigest string                              `json:"maintenance_review_digest"`
	Digest                  string                              `json:"review_digest"`
}

func modelReviewDigest(value any) (string, error) {
	body, err := releasecontract.CanonicalJSON(value)
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(body)
	return hex.EncodeToString(h[:]), nil
}

func modelClient(ctx context.Context, o installer.ClientOptions) (installer.ClientRecord, error) {
	status, err := installer.ReadClientsSeeded(ctx, o)
	if err != nil {
		return installer.ClientRecord{}, err
	}
	if status.State != "current_materialization" || status.Record == nil {
		return installer.ClientRecord{}, errors.New("current admitted client selection required")
	}
	return *status.Record, nil
}

func modelProfileMarker(carrier string) []byte {
	body, _ := json.Marshal(struct {
		SchemaVersion int    `json:"schema_version"`
		CarrierID     string `json:"carrier_id"`
		Purpose       string `json:"purpose"`
	}{1, carrier, "bounded-model-maintenance"})
	return append(body, '\n')
}

func ReviewModelProfile(ctx context.Context, o installer.ClientOptions) (review ModelProfileReview, err error) {
	if ctx == nil {
		return review, errors.New("model profile context required")
	}
	r, err := modelClient(ctx, o)
	if err != nil {
		return review, err
	}
	executable, err := filepath.EvalSymlinks(r.Code.Executable)
	if err != nil || !filepath.IsAbs(executable) || filepath.Clean(executable) != executable {
		return review, errors.New("selected canonical native Code executable unavailable")
	}
	sha, target, err := packaging.InspectNativeExecutable(executable)
	if err != nil || target.OS != runtime.GOOS || target.Arch != runtime.GOARCH {
		return review, errors.New("selected supported native Code image required")
	}
	root := filepath.Join(o.DataRoot, "model-profiles", r.CarrierID)
	review = ModelProfileReview{CarrierID: r.CarrierID, ClientRecordSHA256: installer.ClientRecordDigest(r), Executable: executable, ExecutableSHA256: sha, Root: root, ProfileDirectory: filepath.Join(root, "config"), WorkingDirectory: filepath.Join(root, "workspace"), Effects: []string{"create_or_reuse_only_the_reviewed_owner_private_model_profile", "keep_the_daily_Maestro_profile_and_its_hooks_plugins_and_credentials_unchanged", "seed_autoMemoryEnabled_false_and_syncClaudeAiPlugins_false_only_in_the_new_profile_and_create_private_working_temporary_and_proposal_directories", "authentication_registration_qualification_and_enrollment_require_separate_explicit_actions"}}
	marker, readErr := scheduler.ReadPrivateStateFile(filepath.Join(root, "model-profile.json"), 4096)
	if readErr == nil {
		if !bytes.Equal(marker, modelProfileMarker(r.CarrierID)) {
			return ModelProfileReview{}, errors.New("existing model profile purpose differs; preserved unchanged")
		}
		for _, p := range []string{root, review.ProfileDirectory, review.WorkingDirectory, filepath.Join(root, "tmp"), filepath.Join(root, "registrations")} {
			if err := scheduler.ValidatePrivateDirectory(p); err != nil {
				return ModelProfileReview{}, err
			}
		}
		review.Prepared = true
	} else if !errors.Is(readErr, os.ErrNotExist) {
		return ModelProfileReview{}, readErr
	} else if _, statErr := os.Lstat(root); !errors.Is(statErr, os.ErrNotExist) {
		return ModelProfileReview{}, errors.New("unrecognized or partial model profile directory; preserve it for explicit reconciliation")
	}
	review.Digest, err = modelReviewDigest(review)
	return review, err
}

func ApplyModelProfile(ctx context.Context, o installer.ClientOptions, reviewed ModelProfileReview, digest string, confirmed bool) (ModelProfileStatus, error) {
	if !confirmed {
		return ModelProfileStatus{}, errors.New("explicit dedicated model profile preparation review required")
	}
	fresh, err := ReviewModelProfile(ctx, o)
	if err != nil {
		return ModelProfileStatus{}, err
	}
	if digest == "" || reviewed.Digest != digest || fresh.Digest != digest {
		return ModelProfileStatus{}, errors.New("model profile review changed")
	}
	if !fresh.Prepared {
		for _, p := range []string{filepath.Dir(fresh.Root), fresh.Root, fresh.ProfileDirectory, fresh.WorkingDirectory, filepath.Join(fresh.Root, "tmp"), filepath.Join(fresh.Root, "registrations")} {
			if err := scheduler.EnsurePrivateDirectory(p); err != nil {
				return ModelProfileStatus{}, fmt.Errorf("model profile preparation partial; preserve existing files: %w", err)
			}
		}
		if err := scheduler.PublishNewPrivateFile(filepath.Join(fresh.ProfileDirectory, "settings.json"), []byte("{\"autoMemoryEnabled\":false,\"syncClaudeAiPlugins\":false}\n")); err != nil {
			return ModelProfileStatus{}, err
		}
		if err := scheduler.PublishNewPrivateFile(filepath.Join(fresh.Root, "model-profile.json"), modelProfileMarker(fresh.CarrierID)); err != nil {
			return ModelProfileStatus{}, err
		}
	}
	after, sourceErr := ReviewModelProfile(ctx, o)
	if sourceErr != nil || after.ClientRecordSHA256 != fresh.ClientRecordSHA256 || after.ExecutableSHA256 != fresh.ExecutableSHA256 {
		return ModelProfileStatus{}, errors.New("model profile preparation partial after current selection changed; preserve and reconcile")
	}
	return ModelProfileStatus{fresh.ProfileDirectory, fresh.WorkingDirectory, true, "pending", "official_login_required"}, nil
}

func modelBaseEnvironment(o installer.ClientOptions, profile ModelProfileReview) ([]string, error) {
	env := []string{"HOME=" + o.UserHome, "CLAUDE_CONFIG_DIR=" + profile.ProfileDirectory, "LANG=C", "LC_ALL=C", "TEMP=" + filepath.Join(profile.Root, "tmp"), "TMP=" + filepath.Join(profile.Root, "tmp")}
	if runtime.GOOS == "windows" {
		lookup := o.LookupEnv
		if lookup == nil {
			lookup = os.LookupEnv
		}
		root, ok := lookup("SystemRoot")
		if !ok || !filepath.IsAbs(root) {
			return nil, errors.New("observed Windows system root required")
		}
		env = append(env, "USERPROFILE="+o.UserHome, "SystemRoot="+root, "PATH="+filepath.Join(root, "System32"))
		for _, key := range []string{"APPDATA", "LOCALAPPDATA"} {
			value, present := lookup(key)
			if !present || !filepath.IsAbs(value) {
				return nil, errors.New("observed Windows application data directory required")
			}
			env = append(env, key+"="+value)
		}
	} else {
		env = append(env, "PATH=/usr/bin:/bin")
	}
	sort.Strings(env)
	return env, maintenance.ValidateWeeklyModelEnvironment(env)
}

func modelProfileObservation(ctx context.Context, o installer.ClientOptions, profile ModelProfileReview) (packaging.WeeklyRuntimeObservation, []string, error) {
	base, err := modelBaseEnvironment(o, profile)
	if err != nil {
		return packaging.WeeklyRuntimeObservation{}, nil, err
	}
	effective, err := packaging.WeeklyModelEnvironment(base, modelRegistrationTokens)
	if err != nil {
		return packaging.WeeklyRuntimeObservation{}, nil, err
	}
	observation, err := packaging.ObserveWeeklyRuntime(ctx, profile.Executable, effective, profile.WorkingDirectory)
	if err == nil && observation.ExecutableSHA256 != profile.ExecutableSHA256 {
		err = errors.New("reviewed native Code image changed")
	}
	return observation, base, err
}

func modelAuthentication(ctx context.Context, profile ModelProfileReview, env []string) (bool, error) {
	bounded, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(bounded, profile.Executable, "auth", "status")
	cmd.Dir = profile.WorkingDirectory
	cmd.Env = env
	cmd.WaitDelay = 2 * time.Second
	var stdout boundedClientOutput
	cmd.Stdout = &stdout
	cmd.Stderr = &boundedClientOutput{}
	runErr := cmd.Run()
	actual, _, identityErr := packaging.InspectNativeExecutable(profile.Executable)
	if identityErr != nil || actual != profile.ExecutableSHA256 {
		return false, errors.New("model profile executable changed during authentication observation")
	}
	if runErr != nil {
		return false, nil
	}
	var status struct {
		LoggedIn      bool   `json:"loggedIn"`
		AuthMethod    string `json:"authMethod"`
		Configuration string `json:"configDirectory"`
	}
	if len(stdout.Bytes()) > 16<<10 || json.Unmarshal(stdout.Bytes(), &status) != nil {
		return false, errors.New("official model profile authentication status unavailable")
	}
	if status.Configuration != "" && filepath.Clean(status.Configuration) != profile.ProfileDirectory {
		return false, errors.New("official authentication status selected another profile")
	}
	return status.LoggedIn && status.AuthMethod != "" && status.AuthMethod != "none", nil
}

// InspectModelProfile reads actual official authentication metadata; no
// credential file is opened and no model or login command is invoked.
func InspectModelProfile(ctx context.Context, o installer.ClientOptions) (ModelProfileStatus, error) {
	profile, err := ReviewModelProfile(ctx, o)
	if err != nil {
		return ModelProfileStatus{}, err
	}
	status := ModelProfileStatus{profile.ProfileDirectory, profile.WorkingDirectory, profile.Prepared, "pending", "profile_preparation_required"}
	if !profile.Prepared {
		return status, nil
	}
	_, base, err := modelProfileObservation(ctx, o, profile)
	if err != nil {
		return status, err
	}
	loggedIn, err := modelAuthentication(ctx, profile, base)
	if err != nil {
		return status, err
	}
	status.Reason = "official_login_required"
	if loggedIn {
		status.Authentication = "authenticated"
		status.Reason = ""
	}
	return status, nil
}

// ModelProfileLoginReview binds the official authentication action separately
// from preparing the profile. Neither review authorizes registration or models.
type ModelProfileLoginReview struct {
	Operation     string                             `json:"operation"`
	Profile       ModelProfileReview                 `json:"profile"`
	Observation   packaging.WeeklyRuntimeObservation `json:"observation"`
	Arguments     []string                           `json:"arguments"`
	Effects       []string                           `json:"effects"`
	Digest        string                             `json:"review_digest"`
	Configuration maintenance.WeeklyConfiguration    `json:"configuration"`
	Environment   []string                           `json:"environment"`
}

func ReviewModelProfileLogin(ctx context.Context, o installer.ClientOptions) (review ModelProfileLoginReview, err error) {
	profile, err := ReviewModelProfile(ctx, o)
	if err != nil {
		return review, err
	}
	if !profile.Prepared {
		return review, ErrModelProfilePreparationRequired
	}
	observation, base, err := modelProfileObservation(ctx, o, profile)
	if err != nil {
		return review, err
	}
	configuration, err := packaging.ObserveWeeklyConfiguration(ctx, profile.ProfileDirectory, profile.WorkingDirectory)
	if err != nil {
		return review, err
	}
	review = ModelProfileLoginReview{Environment: base, Configuration: configuration, Operation: "official_model_profile_login", Profile: profile, Observation: observation, Arguments: []string{"auth", "login"}, Effects: []string{"open_the_vendor_official_login_flow_for_only_this_reviewed_private_profile", "the_operator_completes_normal_authentication_in_the_terminal_or_browser", "do_not_read_or_copy_credentials_or_change_the_daily_Maestro_profile", "do_not_register_qualify_enroll_or_invoke_a_model"}}
	review.Digest, err = modelReviewDigest(review)
	return review, err
}

// LoginModelProfile invokes only a separately reviewed official login action,
// attached to the operator's terminal. It never reads or copies credentials.
func LoginModelProfile(ctx context.Context, o installer.ClientOptions, reviewed ModelProfileLoginReview, digest string, confirmed bool) (ModelProfileStatus, error) {
	if !confirmed {
		return ModelProfileStatus{}, errors.New("explicit official model profile login action required")
	}
	review, err := ReviewModelProfileLogin(ctx, o)
	if err != nil {
		return ModelProfileStatus{}, err
	}
	if digest == "" || reviewed.Digest != digest || review.Digest != digest {
		return ModelProfileStatus{}, errors.New("official model profile login review changed")
	}
	profile := review.Profile
	base := append([]string(nil), review.Environment...)
	bounded, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(bounded, profile.Executable, review.Arguments...)
	cmd.Dir = profile.WorkingDirectory
	cmd.Env = base
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.WaitDelay = 2 * time.Second
	if err := cmd.Run(); err != nil {
		return ModelProfileStatus{profile.ProfileDirectory, profile.WorkingDirectory, true, "pending", "official_login_incomplete"}, err
	}
	fresh, err := ReviewModelProfile(ctx, o)
	if err != nil || fresh.Digest != profile.Digest {
		return ModelProfileStatus{profile.ProfileDirectory, profile.WorkingDirectory, true, "pending", "profile_or_current_source_changed_during_login"}, errors.New("reviewed model profile changed during official login; reconcile actual authentication")
	}
	return InspectModelProfile(ctx, o)
}

func validateModelSelection(s ModelRegistrationSelection) error {
	if s.JobID != maintenance.YodaSelfReviewWeeklyJobID && s.JobID != maintenance.MemoryModelJobID {
		return errors.New("unsupported bounded model maintenance job")
	}
	for _, v := range []string{s.Provider, s.Model} {
		if v == "" || len(v) > 128 || !utf8.ValidString(v) || strings.TrimSpace(v) != v || strings.HasPrefix(v, "-") || strings.ContainsAny(v, "\x00\r\n") {
			return errors.New("explicit bounded provider and model selection required")
		}
	}
	if s.JobID == maintenance.MemoryModelJobID && !reflect.DeepEqual(s.OwnerPolicy, ownerctx.WeeklySelfOptions{}) {
		return errors.New("workspace memory registration cannot borrow owner self policy")
	}
	return nil
}

// ReviewModelRegistration may stage one immutable 0600 declaration proposal.
// It publishes no accepted registration and invokes no model. The original
// admitted maintenance server produces the sole authoritative job review.
func ReviewModelRegistration(ctx context.Context, o installer.ClientOptions, selection ModelRegistrationSelection) (review ModelRegistrationReview, err error) {
	if err := validateModelSelection(selection); err != nil {
		return review, err
	}
	profile, err := ReviewModelProfile(ctx, o)
	if err != nil {
		return review, err
	}
	if !profile.Prepared {
		return review, ErrModelProfilePreparationRequired
	}
	observation, base, err := modelProfileObservation(ctx, o, profile)
	if err != nil {
		return review, err
	}
	loggedIn, err := modelAuthentication(ctx, profile, base)
	if err != nil {
		return review, err
	}
	if !loggedIn {
		return review, ErrModelProfileLoginRequired
	}
	configuration, err := packaging.ObserveWeeklyConfiguration(ctx, profile.ProfileDirectory, profile.WorkingDirectory)
	if err != nil {
		return review, err
	}
	effective, err := packaging.WeeklyModelEnvironment(base, modelRegistrationTokens)
	if err != nil {
		return review, err
	}
	validator := "weekly-self-v1"
	registration := maintenance.WeeklyModelRegistration{SchemaVersion: 1, Configuration: configuration, WorkingDirectory: profile.WorkingDirectory, Environment: base, Policy: selection.OwnerPolicy}
	if selection.JobID == maintenance.MemoryModelJobID {
		policy := maintenance.DefaultMemoryModelPolicy()
		registration.Memory = &policy
		validator = "weekly-memory-v1"
	}
	registration.Descriptor = maintenance.WeeklyModelDescriptor{SchemaVersion: 1, JobID: selection.JobID, Runtime: "claude_code_cli", RuntimeVersion: observation.RuntimeVersion, Executable: observation.Executable, ExecutableSHA256: observation.ExecutableSHA256, ConfigurationSHA256: configuration.Digest("settings"), ManagedPolicySHA256: configuration.Digest("managed_policy"), ProfileReferenceSHA256: hashModelBytes([]byte(profile.ProfileDirectory)), TransportSHA256: maintenance.WeeklyModelTransportDigest(profile.WorkingDirectory, effective), Provider: selection.Provider, Model: selection.Model, CredentialMode: "runtime_owned_session", ValidatorVersion: validator, RuntimeIdentity: &observation.Declaration, Limits: maintenance.WeeklyModelLimits{MaxInputBytes: 32768, MaxOutputBytes: 32768, MaxStderrBytes: 4096, MaxOutputTokens: modelRegistrationTokens, TimeoutSeconds: 60, Attempts: 1, AgenticTurns: 1}}
	if err := registration.Validate(); err != nil {
		return review, err
	}
	body, err := releasecontract.CanonicalJSON(registration)
	if err != nil || len(body) > 32768 {
		return review, errors.New("bounded model declaration unavailable")
	}
	sha := hashModelBytes(body)
	path := filepath.Join(profile.Root, "registrations", sha+".json")
	if err := scheduler.PublishNewPrivateFile(path, body); err != nil {
		if !errors.Is(err, os.ErrExist) {
			return review, err
		}
		previous, readErr := scheduler.ReadPrivateStateFile(path, 32768)
		if readErr != nil || !bytes.Equal(previous, body) {
			return review, errors.New("private declaration proposal changed; preserved unchanged")
		}
	}
	var status ClientMaintenanceStatus
	statusBody, err := modelRegistrationCommand(ctx, o, "maintenance-status", nil, "", "", profile.ClientRecordSHA256)
	if err != nil {
		return review, err
	}
	if err = releasecontract.DecodeStrictJSON(statusBody, &status); err != nil {
		return review, err
	}
	var selected ClientMaintenanceJob
	found := false
	for _, job := range status.Jobs {
		if job.JobID == selection.JobID {
			if found {
				return review, errors.New("ambiguous model job status")
			}
			selected = job
			found = true
		}
	}
	if !found {
		return review, errors.New("selected model job absent from actual installed catalog")
	}
	request := product.MaintenanceMutation{Operation: "maintenance.register", JobID: selection.JobID, ExpectedRevision: selected.Revision}
	reviewBody, err := modelRegistrationCommand(ctx, o, "maintenance-review", request, "", path, profile.ClientRecordSHA256)
	if err != nil {
		return review, err
	}
	var original product.LocalMaintenanceReview
	if err = releasecontract.DecodeStrictJSON(reviewBody, &original); err != nil {
		return review, err
	}
	if err := validateMeasuredModelReview(status, selected, registration, original); err != nil {
		return review, err
	}
	review = ModelRegistrationReview{Selection: selection, Profile: profile, Observation: observation, Registration: registration, RegistrationFile: path, RegistrationSHA256: sha, Review: original.Review, MaintenanceReviewDigest: original.ReviewDigest}
	review.Digest, err = modelReviewDigest(review)
	return review, err
}

func validateMeasuredModelReview(status ClientMaintenanceStatus, job ClientMaintenanceJob, registration maintenance.WeeklyModelRegistration, original product.LocalMaintenanceReview) error {
	scope, scopeID := "workspace", status.WorkspaceID
	switch job.JobID {
	case maintenance.YodaSelfReviewWeeklyJobID:
		if job.Scope != "owner" || job.ScopeID == "" {
			return errors.New("actual owner model job scope unavailable")
		}
		scope, scopeID = job.Scope, job.ScopeID
	case maintenance.MemoryModelJobID:
		if job.Scope != "" && job.Scope != "workspace" || job.ScopeID != "" && job.ScopeID != scopeID {
			return errors.New("actual workspace model job scope differs")
		}
	default:
		return errors.New("unsupported bounded model maintenance job")
	}
	if original.Review.Operation.Kind != "register" || original.Review.Operation.JobID != job.JobID || original.Review.Operation.ExpectedRevision != job.Revision || scopeID == "" || original.Review.Scope != scope || original.Review.WorkspaceID != scopeID || original.Review.WeeklyRegistration == nil || !reflect.DeepEqual(*original.Review.WeeklyRegistration, registration) || original.ReviewDigest == "" {
		return errors.New("original model registration review differs from measured proposal")
	}
	return nil
}

func ApplyModelRegistration(ctx context.Context, o installer.ClientOptions, review ModelRegistrationReview, digest string, confirmed bool) (maintenance.ProductJobStatus, error) {
	if !confirmed {
		return maintenance.ProductJobStatus{}, errors.New("explicit exact model registration review required")
	}
	if digest == "" || review.Digest != digest {
		return maintenance.ProductJobStatus{}, errors.New("model registration review digest required")
	}
	fresh, err := ReviewModelRegistration(ctx, o, review.Selection)
	if err != nil {
		return maintenance.ProductJobStatus{}, err
	}
	if fresh.Digest != digest {
		return maintenance.ProductJobStatus{}, errors.New("model registration inputs or original review changed")
	}
	body, err := scheduler.ReadPrivateStateFile(fresh.RegistrationFile, 32768)
	if err != nil || hashModelBytes(body) != fresh.RegistrationSHA256 {
		return maintenance.ProductJobStatus{}, errors.New("reviewed model declaration changed")
	}
	request := product.MaintenanceMutation{Operation: "maintenance.register", JobID: fresh.Review.Operation.JobID, ExpectedRevision: fresh.Review.Operation.ExpectedRevision}
	result, err := modelRegistrationCommand(ctx, o, "maintenance-apply", request, fresh.MaintenanceReviewDigest, fresh.RegistrationFile, fresh.Profile.ClientRecordSHA256)
	if err != nil {
		return maintenance.ProductJobStatus{}, err
	}
	var status maintenance.ProductJobStatus
	if err = releasecontract.DecodeStrictJSON(result, &status); err != nil {
		return status, err
	}
	if status.JobID != fresh.Selection.JobID || status.State != "inactive" || status.QualificationState != "unqualified" {
		return status, errors.New("original model registration result requires reconciliation")
	}
	return status, nil
}

func hashModelBytes(body []byte) string { h := sha256.Sum256(body); return hex.EncodeToString(h[:]) }

func modelRegistrationCommand(ctx context.Context, o installer.ClientOptions, command string, input any, digest, path, recordSHA string) ([]byte, error) {
	if command != "maintenance-status" && command != "maintenance-review" && command != "maintenance-apply" {
		return nil, errors.New("unsupported model registration command")
	}
	record, err := modelClient(ctx, o)
	if err != nil {
		return nil, err
	}
	if installer.ClientRecordDigest(record) != recordSHA {
		return nil, errors.New("reviewed current client changed before model registration command")
	}
	workspace := record.Binding.Workspace
	current, err := installtx.ValidateQualifiedLaunch(o.ManagedRoot, o.DataRoot, installtx.ReleaseBinding{}, time.Now())
	if err != nil {
		return nil, err
	}
	launch, err := installer.PrepareQualifiedLaunch(ctx, installer.QualifiedLaunchOptions{SeededOperationOptions: o.SeededOperationOptions, ExpectedBinding: current.Binding, Workspace: workspace})
	if err != nil {
		return nil, err
	}
	if len(launch.Args) < 1 || launch.Args[0] != "serve" {
		return nil, errors.New("original installed launch unavailable")
	}
	before, err := installer.ClientFileDigest(launch.Executable)
	if err != nil || before != current.Server.SHA256 {
		return nil, errors.New("current model registration server changed")
	}
	args := append([]string{command}, launch.Args[1:]...)
	if path != "" {
		args = append(args, "--weekly-registration-file", path)
	}
	if digest != "" {
		args = append(args, "--review-digest", digest, "--confirm")
	}
	bounded, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(bounded, launch.Executable, args...)
	cmd.Dir = workspace
	cmd.WaitDelay = 2 * time.Second
	if input != nil {
		body, err := json.Marshal(input)
		if err != nil {
			return nil, err
		}
		cmd.Stdin = bytes.NewReader(body)
	}
	var output boundedClientOutput
	cmd.Stdout = &output
	cmd.Stderr = &boundedClientOutput{}
	if err := cmd.Run(); err != nil {
		return nil, errors.New("original installed model registration command refused; preserve the current registration")
	}
	after, err := installer.ClientFileDigest(launch.Executable)
	if err != nil || after != before {
		return nil, errors.New("model registration server changed during operation")
	}
	afterRecord, recordErr := modelClient(ctx, o)
	if recordErr != nil || installer.ClientRecordDigest(afterRecord) != recordSHA {
		return nil, errors.New("current client changed during model registration command; reconcile actual inactive registration")
	}
	if _, err := installer.PrepareQualifiedLaunch(ctx, installer.QualifiedLaunchOptions{SeededOperationOptions: o.SeededOperationOptions, ExpectedBinding: current.Binding, Workspace: workspace}); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}
