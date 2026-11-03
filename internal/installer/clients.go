package installer

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/datahome"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installtx"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspace"
)

// Stable client records are local materialization evidence. They never supply
// release authority, freshness, source custody, executable selection or native
// host qualification. Keep seed-status and release-bound launch unchanged.
const StableClientProfile = "stable-local-v1"
const StableClientProtocol = "maestro-stable-carrier-v1"
const StableCarrierVersion = "1.0.0"

type ClientCapabilities struct {
	SchemaVersion     int    `json:"schema_version"`
	Product           string `json:"product"`
	Component         string `json:"component"`
	Version           string `json:"version"`
	RegistrySHA256    string `json:"authority_registry_sha256"`
	Profile           string `json:"profile"`
	Protocol          string `json:"protocol"`
	CarrierVersion    string `json:"carrier_version"`
	CompatibleCarrier string `json:"compatible_carrier"`
}

func StableClientCapabilities(component, version, pin string) ClientCapabilities {
	return ClientCapabilities{1, "maestro", component, version, pin, StableClientProfile, StableClientProtocol, StableCarrierVersion, "1.0.0"}
}

type ClientRequest struct {
	CarrierID      string `json:"carrier_id"`
	Workspace      string `json:"workspace"`
	CodeRoot       string `json:"code_root"`
	DesktopPath    string `json:"desktop_path"`
	CodeExecutable string `json:"code_executable,omitempty"`
	CodeScope      string `json:"code_scope"`
	// ManualDesktopUpdate creates the documented replacement bundle instead of
	// reusing the byte-identical external-launcher candidate.
	ManualDesktopUpdate bool `json:"manual_desktop_update,omitempty"`
	Reconcile           bool `json:"reconcile,omitempty"`
}
type ClientOptions struct {
	SeededOperationOptions
	Request   ClientRequest
	LookupEnv func(string) (string, bool)
}
type ClientBinding struct {
	ManagedRoot    string `json:"managed_root"`
	UserHome       string `json:"user_home"`
	Home           string `json:"home"`
	Workspace      string `json:"workspace"`
	WorkspaceID    string `json:"workspace_id"`
	RegistrySHA256 string `json:"authority_registry_sha256"`
}
type ClientMaterialization struct {
	Release                       string `json:"release"`
	ManifestSHA256                string `json:"manifest_sha256"`
	ClientInputsSHA256            string `json:"client_inputs_sha256"`
	ServerSHA256                  string `json:"server_sha256"`
	EmbeddedInputsSHA256          string `json:"embedded_inputs_sha256"`
	BundleSHA256                  string `json:"bundle_sha256"`
	CodeRoot                      string `json:"code_root"`
	CodeTreeSHA256                string `json:"code_tree_sha256"`
	CodeBackupTreeSHA256          string `json:"code_backup_tree_sha256,omitempty"`
	DesktopPath                   string `json:"desktop_path"`
	DesktopSHA256                 string `json:"desktop_sha256"`
	DesktopBackupSHA256           string `json:"desktop_backup_sha256,omitempty"`
	DesktopNativeContractSHA256   string `json:"desktop_native_contract_sha256"`
	BootstrapServerSHA256         string `json:"bootstrap_server_sha256"`
	BootstrapEmbeddedInputsSHA256 string `json:"bootstrap_embedded_inputs_sha256"`
}
type CodeObservation struct {
	Executable       string `json:"executable,omitempty"`
	ExecutableSHA256 string `json:"executable_sha256,omitempty"`
	Version          string `json:"version,omitempty"`
	Marketplace      string `json:"marketplace"`
	PluginID         string `json:"plugin_id"`
	Scope            string `json:"scope"`
	SourcePath       string `json:"source_path,omitempty"`
	InstallPath      string `json:"install_path,omitempty"`
	InstalledVersion string `json:"installed_version,omitempty"`
	FolderVersion    string `json:"folder_version,omitempty"`
	ReadFromFolder   bool   `json:"read_from_folder"`
	Enabled          bool   `json:"enabled"`
	Errors           int    `json:"errors"`
	Notes            int    `json:"notes"`
	State            string `json:"state"`
	EvidenceClass    string `json:"evidence_class"`
}
type DesktopObservation struct {
	Handoff              string `json:"handoff"`
	InstallConfirmation  string `json:"install_confirmation"`
	Connection           string `json:"connection"`
	FunctionalCheck      string `json:"functional_check"`
	PackageSHA256        string `json:"package_sha256,omitempty"`
	NativeContractSHA256 string `json:"native_contract_sha256,omitempty"`
	ManifestSHA256       string `json:"manifest_sha256,omitempty"`
	EvidenceClass        string `json:"evidence_class"`
}
type ClientRecord struct {
	SchemaVersion   int                   `json:"schema_version"`
	Profile         string                `json:"profile"`
	Protocol        string                `json:"protocol"`
	CarrierVersion  string                `json:"carrier_version"`
	CarrierID       string                `json:"carrier_id"`
	Binding         ClientBinding         `json:"binding"`
	Materialization ClientMaterialization `json:"materialization"`
	Code            CodeObservation       `json:"code"`
	Desktop         DesktopObservation    `json:"desktop"`
}
type ClientReview struct {
	SchemaVersion        int                   `json:"schema_version"`
	Request              ClientRequest         `json:"request"`
	Binding              ClientBinding         `json:"binding"`
	Current              ClientMaterialization `json:"current"`
	PreviousRecordSHA256 string                `json:"previous_record_sha256,omitempty"`
	Effects              []string              `json:"effects"`
	Digest               string                `json:"review_digest"`
}
type ClientStatus struct {
	State            string        `json:"state"`
	Reason           string        `json:"reason,omitempty"`
	Record           *ClientRecord `json:"record,omitempty"`
	CodeRegistration string        `json:"code_registration"`
	SessionLoading   string        `json:"session_loading"`
	HostObservations string        `json:"host_observations"`
}
type ClientMaterializer func(context.Context, installtx.State, ClientBinding, *ClientRecord) (ClientMaterialization, error)

var carrierIDPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)
var clientSHA = regexp.MustCompile(`^[0-9a-f]{64}$`)

func clientRecordPath(o SeededOperationOptions, id string) (string, error) {
	if !carrierIDPattern.MatchString(id) {
		return "", errors.New("exact opaque carrier ID required")
	}
	if o.DataRoot != filepath.Join(o.UserHome, ".maestro-install") {
		return "", errors.New("fixed private client custody required")
	}
	return filepath.Join(o.DataRoot, "clients", id+".json"), nil
}
func readClientRecord(o SeededOperationOptions, id string) (ClientRecord, []byte, error) {
	p, err := clientRecordPath(o, id)
	if err != nil {
		return ClientRecord{}, nil, err
	}
	body, err := scheduler.ReadPrivateStateFile(p, 64<<10)
	if err != nil {
		return ClientRecord{}, nil, err
	}
	var r ClientRecord
	if err = releasecontract.DecodeStrictJSON(body, &r); err != nil {
		return r, nil, err
	}
	if r.SchemaVersion != 1 || r.Profile != StableClientProfile || r.Protocol != StableClientProtocol || r.CarrierVersion != StableCarrierVersion || r.CarrierID != id || r.Binding.ManagedRoot != o.ManagedRoot || r.Binding.UserHome != o.UserHome || r.Binding.RegistrySHA256 != o.AuthorityRegistrySHA256 {
		return r, nil, errors.New("client record binding/profile mismatch")
	}
	return r, body, nil
}
func clientBinding(o ClientOptions) (ClientBinding, error) {
	h, err := datahome.Resolve(datahome.Options{ProductRoot: o.ManagedRoot, UserHome: o.UserHome, LookupEnv: o.LookupEnv})
	if err != nil {
		return ClientBinding{}, err
	}
	if !h.Writable {
		return ClientBinding{}, errors.New("client materialization requires admitted home")
	}
	w, err := workspace.Inspect(o.Request.Workspace, filepath.Join(h.Home, "state"))
	if err != nil {
		return ClientBinding{}, err
	}
	for _, p := range []string{o.Request.CodeRoot, o.Request.DesktopPath, o.Request.Workspace} {
		if !filepath.IsAbs(p) || filepath.Clean(p) != p || strings.ContainsAny(p, "\x00\r\n") {
			return ClientBinding{}, errors.New("clean absolute client paths required")
		}
	}
	if filepath.Ext(o.Request.DesktopPath) != ".mcpb" {
		return ClientBinding{}, errors.New("Desktop package requires .mcpb")
	}
	for _, destination := range []string{o.Request.CodeRoot, o.Request.DesktopPath} {
		if err := scheduler.ValidateDirectory(filepath.Dir(destination)); err != nil {
			return ClientBinding{}, err
		}
		for _, root := range []string{o.ManagedRoot, o.DataRoot, h.Home, o.Request.Workspace} {
			if clientOverlap(root, destination) {
				return ClientBinding{}, errors.New("client destination overlaps admitted private binding")
			}
		}
	}
	if clientOverlap(o.Request.CodeRoot, o.Request.DesktopPath) {
		return ClientBinding{}, errors.New("client destinations overlap")
	}
	if o.Request.CodeScope != "user" && o.Request.CodeScope != "local" && o.Request.CodeScope != "project" {
		return ClientBinding{}, errors.New("explicit user/local/project scope required")
	}
	return ClientBinding{o.ManagedRoot, o.UserHome, h.Home, o.Request.Workspace, w.WorkspaceID, o.AuthorityRegistrySHA256}, nil
}
func clientOverlap(a, b string) bool {
	a, b = strings.ToLower(a), strings.ToLower(b)
	for _, pair := range [][2]string{{a, b}, {b, a}} {
		rel, e := filepath.Rel(pair[0], pair[1])
		if e == nil && (rel == "." || rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))) {
			return true
		}
	}
	return false
}
func currentClients(s installtx.State) ClientMaterialization {
	return ClientMaterialization{Release: s.Release, ManifestSHA256: s.Binding.ManifestSHA256, ClientInputsSHA256: s.Binding.ClientInputsSHA256, ServerSHA256: s.Server.SHA256, EmbeddedInputsSHA256: s.Server.EmbeddedInputsSHA256, BundleSHA256: s.Binding.BundleSHA256}
}
func sameClientCurrent(m ClientMaterialization, s installtx.State) bool {
	return m.Release == s.Release && m.ManifestSHA256 == s.Binding.ManifestSHA256 && m.ClientInputsSHA256 == s.Binding.ClientInputsSHA256 && m.ServerSHA256 == s.Server.SHA256 && m.EmbeddedInputsSHA256 == s.Server.EmbeddedInputsSHA256 && m.BundleSHA256 == s.Binding.BundleSHA256
}

func readClientCapabilities(ctx context.Context, path, component, version, pin string) error {
	deadline, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	body, err := execCommand(deadline, path, "client-capabilities")
	if err != nil {
		return fmt.Errorf("actual %s stable capability unavailable: %w", component, err)
	}
	if len(body) > 64<<10 {
		return errors.New("client capability size")
	}
	var c ClientCapabilities
	if err = releasecontract.DecodeStrictJSON(body, &c); err != nil {
		return err
	}
	if c != StableClientCapabilities(component, version, pin) {
		return errors.New("actual stable capability differs from admitted component")
	}
	return nil
}
func ReviewClientsSeeded(ctx context.Context, o ClientOptions) (ClientReview, error) {
	if _, err := admitOperationSeed(ctx, o.SeededOperationOptions); err != nil {
		return ClientReview{}, err
	}
	if _, err := clientRecordPath(o.SeededOperationOptions, o.Request.CarrierID); err != nil {
		return ClientReview{}, err
	}
	s, err := installtx.ValidateQualifiedLaunch(o.ManagedRoot, o.DataRoot, installtx.ReleaseBinding{}, time.Now())
	if err != nil {
		return ClientReview{}, err
	}
	if err = readClientCapabilities(ctx, o.Self, "installer", o.Version, o.AuthorityRegistrySHA256); err != nil {
		return ClientReview{}, err
	}
	if err = readClientCapabilities(ctx, installedExecutablePath(o.ManagedRoot, s.TargetOS, s.Profile), "server", s.Server.Version, o.AuthorityRegistrySHA256); err != nil {
		return ClientReview{}, err
	}
	binding, err := clientBinding(o)
	if err != nil {
		return ClientReview{}, err
	}
	review := ClientReview{SchemaVersion: 1, Request: o.Request, Binding: binding, Current: currentClients(s), Effects: []string{"generate_and_verify_current_server_packets", "replace_only_owned_local_packets_with_one_backup", "publish_private_current_client_record", "materialize_bound_operator_launcher"}}
	if o.Request.Reconcile {
		review.Effects = append(review.Effects, "reclaim_only_dead_installation_lock_and_reconcile_actual_owned_packets")
	}
	old, body, err := readClientRecord(o.SeededOperationOptions, o.Request.CarrierID)
	if err == nil {
		if old.Binding != binding || old.Materialization.CodeRoot != o.Request.CodeRoot || old.Materialization.DesktopPath != o.Request.DesktopPath || old.Code.Scope != o.Request.CodeScope || old.Code.Executable != o.Request.CodeExecutable && !(old.Code.Executable == "" && old.Code.State == "registration_pending" && old.Code.EvidenceClass == "not_observed") {
			return ClientReview{}, errors.New("existing client selection differs; preserved unchanged")
		}
		if old.Code.Executable != o.Request.CodeExecutable {
			if !filepath.IsAbs(o.Request.CodeExecutable) || filepath.Clean(o.Request.CodeExecutable) != o.Request.CodeExecutable || strings.ContainsAny(o.Request.CodeExecutable, "\x00\r\n") {
				return ClientReview{}, errors.New("clean absolute first Code executable selection required")
			}
			review.Effects = append(review.Effects, "select_previously_unselected_Code_executable_without_changing_fixed_scope_or_binding")
		}
		review.PreviousRecordSHA256 = clientDigest(body)
	} else if !errors.Is(err, os.ErrNotExist) {
		return ClientReview{}, err
	}
	body, err = releasecontract.CanonicalJSON(review)
	if err != nil {
		return ClientReview{}, err
	}
	review.Digest = clientDigest(body)
	return review, nil
}
func MaterializeClientsSeeded(ctx context.Context, o ClientOptions, digest string, confirmed bool, build ClientMaterializer) (ClientRecord, error) {
	if !confirmed || !clientSHA.MatchString(digest) || build == nil {
		return ClientRecord{}, errors.New("exact client review and explicit confirmation required")
	}
	review, err := ReviewClientsSeeded(ctx, o)
	if err != nil {
		return ClientRecord{}, err
	}
	if review.Digest != digest {
		return ClientRecord{}, errors.New("client review changed")
	}
	var result ClientRecord
	guard := installtx.GuardQualifiedCurrent
	if o.Request.Reconcile {
		guard = installtx.GuardQualifiedReconciliationCurrent
	}
	err = guard(o.ManagedRoot, o.DataRoot, installtx.ReleaseBinding{ManifestSHA256: review.Current.ManifestSHA256, ClientInputsSHA256: review.Current.ClientInputsSHA256}, time.Now(), func() error {
		// Recompute review under the same installation serialization. These readers
		// never acquire that lock or perform historical state migration.
		fresh, e := ReviewClientsSeeded(ctx, o)
		if e != nil {
			return e
		}
		if fresh.Digest != digest {
			return errors.New("client review changed before materialization")
		}
		s, e := installtx.ReadSourceState(o.DataRoot, o.ManagedRoot)
		if e != nil {
			return e
		}
		var previous *ClientRecord
		old, _, e := readClientRecord(o.SeededOperationOptions, o.Request.CarrierID)
		if e == nil {
			previous = &old
		} else if !errors.Is(e, os.ErrNotExist) {
			return e
		}
		material, e := build(ctx, s, review.Binding, previous)
		if e != nil {
			return fmt.Errorf("activation-applied/client-materialization-pending: %w", e)
		}
		if !sameClientCurrent(material, s) || material.CodeRoot != o.Request.CodeRoot || material.DesktopPath != o.Request.DesktopPath {
			return errors.New("materializer returned another current binding")
		}
		if e = validateClientFiles(material); e != nil {
			return e
		}
		if _, e = installtx.ValidateQualifiedLaunch(o.ManagedRoot, o.DataRoot, s.Binding, time.Now()); e != nil {
			return e
		}
		result = ClientRecord{SchemaVersion: 1, Profile: StableClientProfile, Protocol: StableClientProtocol, CarrierVersion: StableCarrierVersion, CarrierID: o.Request.CarrierID, Binding: review.Binding, Materialization: material,
			Code:    CodeObservation{Executable: o.Request.CodeExecutable, Marketplace: "maestro-local-" + o.Request.CarrierID, PluginID: "maestro-product@maestro-local-" + o.Request.CarrierID, Scope: o.Request.CodeScope, State: "registration_pending", EvidenceClass: "not_observed"},
			Desktop: DesktopObservation{Handoff: "bundle_ready", InstallConfirmation: "awaiting_user_install", Connection: "pending", FunctionalCheck: "pending", EvidenceClass: "not_observed"}}
		if previous != nil && previous.Materialization.CodeTreeSHA256 == material.CodeTreeSHA256 && previous.Code.Executable == o.Request.CodeExecutable {
			result.Code = previous.Code
		}
		if previous != nil && previous.Materialization.DesktopSHA256 == material.DesktopSHA256 && previous.Materialization.DesktopNativeContractSHA256 == material.DesktopNativeContractSHA256 {
			result.Desktop = previous.Desktop
			if previous.Materialization.ManifestSHA256 != material.ManifestSHA256 {
				result.Desktop.Connection = "restart_required"
				result.Desktop.FunctionalCheck = "pending"
			}
		}
		return writeClientRecord(o.SeededOperationOptions, result)
	})
	return result, err
}

func ReadClientsSeeded(ctx context.Context, o ClientOptions) (ClientStatus, error) {
	status := ClientStatus{State: "client-materialization-pending", CodeRegistration: "pending", SessionLoading: "pending", HostObservations: "historical_not_refreshed"}
	if _, err := admitOperationSeed(ctx, o.SeededOperationOptions); err != nil {
		return status, err
	}
	record, _, err := readClientRecord(o.SeededOperationOptions, o.Request.CarrierID)
	if err != nil {
		return status, err
	}
	status.Record = &record
	s, err := installtx.ValidateQualifiedLaunch(o.ManagedRoot, o.DataRoot, installtx.ReleaseBinding{}, time.Now())
	if err != nil {
		status.Reason = "source_or_current_unavailable"
		return status, err
	}
	if !sameClientCurrent(record.Materialization, s) {
		status.Reason = "current_release_changed"
		return status, nil
	}
	if o.Request.Workspace == "" {
		o.Request.Workspace = record.Binding.Workspace
	}
	if o.Request.CodeRoot == "" {
		o.Request.CodeRoot = record.Materialization.CodeRoot
	}
	if o.Request.DesktopPath == "" {
		o.Request.DesktopPath = record.Materialization.DesktopPath
	}
	if o.Request.CodeScope == "" {
		o.Request.CodeScope = record.Code.Scope
	}
	binding, err := clientBinding(o)
	if err != nil || binding != record.Binding {
		status.Reason = "binding_changed"
		return status, errors.New("current admitted client binding differs")
	}
	if err = validateClientFiles(record.Materialization); err != nil {
		status.Reason = "packet_changed"
		return status, err
	}
	status.State = "current_materialization"
	status.CodeRegistration = record.Code.State
	if record.Code.State == "registered" {
		status.CodeRegistration = "historical_registered_readback"
	}
	status.SessionLoading = "native_loading_unattested"
	return status, nil
}
func PrepareStableLaunch(ctx context.Context, o ClientOptions) (QualifiedLaunch, error) {
	if _, err := admitOperationSeed(ctx, o.SeededOperationOptions); err != nil {
		return QualifiedLaunch{}, err
	}
	var launch QualifiedLaunch
	err := installtx.GuardQualifiedCurrent(o.ManagedRoot, o.DataRoot, installtx.ReleaseBinding{}, time.Now(), func() error {
		r, _, e := readClientRecord(o.SeededOperationOptions, o.Request.CarrierID)
		if e != nil {
			return e
		}
		if o.Request.Workspace != r.Binding.Workspace {
			return errors.New("stable workspace mismatch")
		}
		bound := o
		bound.Request.CodeRoot = r.Materialization.CodeRoot
		bound.Request.DesktopPath = r.Materialization.DesktopPath
		bound.Request.CodeScope = r.Code.Scope
		status, e := ReadClientsSeeded(ctx, bound)
		if e != nil {
			return e
		}
		if status.State != "current_materialization" {
			return errors.New("stable binding stale; explicit client regeneration required")
		}
		launch, e = PrepareQualifiedLaunch(ctx, QualifiedLaunchOptions{SeededOperationOptions: o.SeededOperationOptions, ExpectedBinding: installtx.ReleaseBinding{ManifestSHA256: r.Materialization.ManifestSHA256, ClientInputsSHA256: r.Materialization.ClientInputsSHA256}, Workspace: r.Binding.Workspace})
		return e
	})
	return launch, err
}

// ObserveClientsSeeded records only fixed typed, package-bound observations.
// It never turns operator testimony into native connection/functional proof.
func ObserveClientsSeeded(ctx context.Context, o ClientOptions, expectedRecord string, code *CodeObservation, desktop *DesktopObservation) (ClientRecord, error) {
	return observeClientsSeeded(ctx, o, expectedRecord, code, desktop, nil)
}

// RegisterCodeSeeded fences the supported native commands and their readback
// against canonical activation/source changes, under the existing lock.
func RegisterCodeSeeded(ctx context.Context, o ClientOptions, expectedRecord string, register func(ClientRecord) (CodeObservation, error)) (ClientRecord, error) {
	if register == nil {
		return ClientRecord{}, errors.New("typed Code registration producer required")
	}
	return observeClientsSeeded(ctx, o, expectedRecord, nil, nil, register)
}
func observeClientsSeeded(ctx context.Context, o ClientOptions, expectedRecord string, code *CodeObservation, desktop *DesktopObservation, register func(ClientRecord) (CodeObservation, error)) (ClientRecord, error) {
	if _, err := admitOperationSeed(ctx, o.SeededOperationOptions); err != nil {
		return ClientRecord{}, err
	}
	var r ClientRecord
	err := installtx.GuardQualifiedCurrent(o.ManagedRoot, o.DataRoot, installtx.ReleaseBinding{}, time.Now(), func() error {
		var body []byte
		var e error
		r, body, e = readClientRecord(o.SeededOperationOptions, o.Request.CarrierID)
		if e != nil {
			return e
		}
		if clientDigest(body) != expectedRecord {
			return errors.New("client observation changed")
		}
		s, e := installtx.ValidateQualifiedLaunch(o.ManagedRoot, o.DataRoot, installtx.ReleaseBinding{}, time.Now())
		if e != nil {
			return e
		}
		if !sameClientCurrent(r.Materialization, s) {
			return errors.New("client observation has stale current materialization")
		}
		if e = validateClientFiles(r.Materialization); e != nil {
			return e
		}
		if register != nil {
			observation, err := register(r)
			if err != nil {
				return err
			}
			code = &observation
		}
		if code != nil {
			if code.Marketplace != r.Code.Marketplace || code.PluginID != r.Code.PluginID || code.Scope != r.Code.Scope || code.Executable != r.Code.Executable || code.State != "registered" || code.EvidenceClass != "supported_cli_readback" && code.EvidenceClass != "engineering_process_witness" {
				return errors.New("unsupported Code observation")
			}
			r.Code = *code
		}
		if desktop != nil {
			if desktop.PackageSHA256 != r.Materialization.DesktopSHA256 || desktop.NativeContractSHA256 != r.Materialization.DesktopNativeContractSHA256 || desktop.ManifestSHA256 != r.Materialization.ManifestSHA256 || desktop.EvidenceClass != "operator_attested" || desktop.Connection != "pending" || desktop.FunctionalCheck != "pending" || desktop.InstallConfirmation != "user_confirmed_install" {
				return errors.New("Desktop confirmation grants no observed runtime evidence")
			}
			r.Desktop = *desktop
		}
		if _, e = installtx.ValidateQualifiedLaunch(o.ManagedRoot, o.DataRoot, s.Binding, time.Now()); e != nil {
			return e
		}
		return writeClientRecord(o.SeededOperationOptions, r)
	})
	return r, err
}
func ClientRecordDigest(r ClientRecord) string {
	body, _ := json.MarshalIndent(r, "", "  ")
	return clientDigest(append(body, '\n'))
}
func writeClientRecord(o SeededOperationOptions, r ClientRecord) error {
	p, err := clientRecordPath(o, r.CarrierID)
	if err != nil {
		return err
	}
	body, err := json.MarshalIndent(r, "", "  ")
	body = append(body, '\n')
	if err != nil || len(body) > 64<<10 {
		return errors.New("bounded client record required")
	}
	directory := filepath.Dir(p)
	if err = os.MkdirAll(directory, 0700); err != nil {
		return err
	}
	if err = scheduler.ValidatePrivateDirectory(directory); err != nil {
		return err
	}
	if _, err = os.Lstat(p); err == nil {
		if _, e := scheduler.ReadPrivateStateFile(p, 64<<10); e != nil {
			return e
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return scheduler.WritePrivateFileAtomic(p, body)
}
func clientDigest(body []byte) string { return fmt.Sprintf("%x", sha256.Sum256(body)) }
func ClientFileDigest(path string) (string, error) {
	f, e := scheduler.OpenRegularFile(path)
	if e != nil {
		return "", e
	}
	defer f.Close()
	h := sha256.New()
	n, e := io.Copy(h, io.LimitReader(f, 128<<20+1))
	if e != nil || n > 128<<20 {
		return "", errors.New("client file bound")
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}
func ClientTreeDigest(root string) (string, error) {
	if e := scheduler.ValidateDirectory(root); e != nil {
		return "", e
	}
	var vector []string
	var total int64
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if d.Type()&os.ModeSymlink != 0 {
			return errors.New("client tree alias")
		}
		if d.IsDir() {
			return nil
		}
		if !d.Type().IsRegular() {
			return errors.New("client tree type")
		}
		info, e := d.Info()
		if e != nil {
			return e
		}
		total += info.Size()
		if len(vector) >= 4096 || total > 256<<20 {
			return errors.New("client tree bound")
		}
		sha, e := ClientFileDigest(p)
		if e != nil {
			return e
		}
		rel, e := filepath.Rel(root, p)
		if e != nil {
			return e
		}
		vector = append(vector, fmt.Sprintf("%s:%04o:%s", filepath.ToSlash(rel), info.Mode().Perm(), sha))
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(vector)
	body, _ := releasecontract.CanonicalJSON(vector)
	return clientDigest(body), nil
}
func validateClientFiles(m ClientMaterialization) error {
	for _, v := range []string{m.CodeTreeSHA256, m.DesktopSHA256, m.DesktopNativeContractSHA256, m.BootstrapServerSHA256, m.BootstrapEmbeddedInputsSHA256} {
		if !clientSHA.MatchString(v) {
			return errors.New("client materialization digest missing")
		}
	}
	code, e := ClientTreeDigest(m.CodeRoot)
	if e != nil {
		return e
	}
	desktop, e := ClientFileDigest(m.DesktopPath)
	if e != nil {
		return e
	}
	if code != m.CodeTreeSHA256 || desktop != m.DesktopSHA256 {
		return errors.New("client packet identity changed")
	}
	return nil
}
