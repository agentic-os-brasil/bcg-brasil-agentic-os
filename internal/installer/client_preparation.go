package installer

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installtx"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
)

const ClientPreparationProtocol = "maestro-client-preparation-v1"
const ClientPreparationDesktopReady = "desktop_prepared_code_pending"
const ClientPartialPreparation = "partial_preparation"
const ClientCodeQualificationPending = "pending_qualification"

// ClientPreparation separates the selected full Code contract from the already
// prepared Desktop artifact. It never supplies native qualification evidence.
type ClientPreparation struct {
	Phase     string        `json:"phase"`
	Selection ClientRequest `json:"selection"`
}

type ClientPreparationStatus struct {
	Phase          string `json:"phase"`
	SelectedTarget string `json:"selected_target"`
	Desktop        string `json:"desktop"`
	Code           string `json:"code"`
}

func (r ClientRecord) CodePreparationPending() bool {
	return r.SchemaVersion == 3 && r.Preparation != nil && r.Preparation.Phase == ClientPreparationDesktopReady
}

func ClientSelectedTarget(r ClientRecord) string {
	if r.CodePreparationPending() {
		return r.Preparation.Selection.Target
	}
	return r.Materialization.Target
}

func ClientPreparationCapabilities(component, version, pin string) ClientTargetCapability {
	return ClientTargetCapability{1, "maestro", component, version, pin, ClientPreparationProtocol}
}

func RequireClientPreparationCapabilities(ctx context.Context, path, component, version, pin string) error {
	bounded, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	body, err := execCommand(bounded, path, "client-preparation-capabilities")
	if err != nil {
		return errors.New("client preparation requires a compatible installed reader")
	}
	return validateClientPreparationCapability(body, component, version, pin)
}

func validateClientPreparationCapability(body []byte, component, version, pin string) error {
	var capability ClientTargetCapability
	if len(body) > 64<<10 || releasecontract.DecodeStrictJSON(body, &capability) != nil || capability != ClientPreparationCapabilities(component, version, pin) {
		return errors.New("client preparation reader capability differs from authenticated component")
	}
	return nil
}

// DesktopPreparationOptions is a trusted host projection, not a new selected
// carrier. The publisher retains and revalidates the original full selection.
func DesktopPreparationOptions(o ClientOptions) (ClientOptions, error) {
	r := o.Request
	if r.Target != "" || !r.VersionedSlots || !r.NativeEnforcement || r.LifecycleContext || r.CodeRoot == "" || r.CodeExecutable == "" || r.CodeScope == "" || !carrierIDPattern.MatchString(r.GenerationID) {
		return ClientOptions{}, errors.New("Desktop preparation requires the unchanged full both-client selection")
	}
	selection := r
	selection.Reconcile, selection.ManualDesktopUpdate = false, false
	o.PreparationSelection = &selection
	o.Request = ClientRequest{Target: "desktop", OwnerReviewChannel: r.OwnerReviewChannel, VersionedSlots: true, CarrierID: r.CarrierID, Workspace: r.Workspace, DesktopPath: r.DesktopPath, Reconcile: r.Reconcile, ManualDesktopUpdate: r.ManualDesktopUpdate}
	return o, nil
}

// DesktopPreparationRecord gives the existing Desktop custody owner its exact
// Desktop view. Selection and publication authority stay in the original record.
func DesktopPreparationRecord(r ClientRecord) (ClientRecord, error) {
	if r.Profile != StableClientProfile || r.Protocol != StableClientProtocol || r.CarrierVersion != StableCarrierVersion || !carrierIDPattern.MatchString(r.CarrierID) {
		return ClientRecord{}, errors.New("validated client identity required for Desktop custody")
	}
	if r.CodePreparationPending() {
		if validateClientPreparationRecord(r) != nil {
			return ClientRecord{}, errors.New("validated partial preparation required")
		}
	} else if r.SchemaVersion != 1 || r.Preparation != nil || r.Materialization.Selection == nil || r.Materialization.Target != "" {
		return ClientRecord{}, errors.New("validated partial preparation required")
	}
	r.SchemaVersion = 2
	r.Preparation = nil
	r.Code = CodeObservation{State: "not_selected", EvidenceClass: "not_applicable"}
	m := &r.Materialization
	m.Target = "desktop"
	m.Selection = &ClientPacketSelection{Target: "desktop", DesktopPath: m.Selection.DesktopPath}
	m.CodeRoot, m.CodeTreeSHA256, m.CodeProviderRoot, m.CodeProviderSHA256 = "", "", "", ""
	m.CodeBackupTreeSHA256, m.DesktopBackupSHA256 = "", ""
	m.Backups = nil
	m.NativeEnforcement, m.LifecycleContext = false, false
	return r, nil
}

func pendingClientCode(selection ClientRequest) CodeObservation {
	return CodeObservation{Executable: selection.CodeExecutable, ConfigurationDirectory: selection.CodeConfigurationDirectory, Marketplace: "maestro-local-" + selection.CarrierID, PluginID: "maestro-product@maestro-local-" + selection.CarrierID, Scope: selection.CodeScope, State: ClientCodeQualificationPending, EvidenceClass: "not_observed"}
}

func validateClientPreparationRecord(r ClientRecord) error {
	if !r.CodePreparationPending() {
		return errors.New("unsupported client preparation phase")
	}
	selection := r.Preparation.Selection
	if selection.CarrierID != r.CarrierID || selection.Workspace != r.Binding.Workspace || selection.Reconcile || selection.ManualDesktopUpdate || selection.OwnerReviewChannel != r.Materialization.OwnerReviewChannel || r.Code != pendingClientCode(selection) {
		return errors.New("partial client selection differs from its fixed binding")
	}
	projection, err := DesktopPreparationOptions(ClientOptions{Request: selection})
	if err != nil {
		return err
	}
	if selection.CodeScope != "user" && selection.CodeScope != "local" && selection.CodeScope != "project" {
		return errors.New("partial client Code scope missing")
	}
	for _, path := range []string{selection.CodeRoot, selection.DesktopPath, selection.CodeExecutable, selection.Workspace} {
		if !filepath.IsAbs(path) || filepath.Clean(path) != path || strings.ContainsAny(path, "\x00\r\n") {
			return errors.New("partial client selection has an invalid path")
		}
	}
	expected, err := ClientGenerationPaths(projection.Request, r.Materialization.ManifestSHA256)
	if err != nil || !r.Materialization.DesktopOnly() || expected.DesktopPath != r.Materialization.DesktopPath || !sameClientSelection(expected.Selection, r.Materialization.Selection) || r.Materialization.GenerationID != "" {
		return errors.New("partial Desktop generation differs from its selected role")
	}
	if err := validateDesktopClientShape(r.Materialization); err != nil {
		return err
	}
	return validateClientMethodBinding(r.Materialization)
}

// ReviewDesktopPreparationSeeded reviews a Desktop-first phase of the original
// full selection. It deliberately cannot review publication of a Code packet.
func ReviewDesktopPreparationSeeded(ctx context.Context, o ClientOptions) (ClientReview, error) {
	if err := ctx.Err(); err != nil {
		return ClientReview{}, err
	}
	if _, err := DesktopPreparationOptions(o); err != nil {
		return ClientReview{}, err
	}
	review, err := ReviewClientsSeeded(ctx, o)
	if err != nil {
		return ClientReview{}, err
	}
	state, err := installtx.ValidateQualifiedLaunchContext(ctx, o.ManagedRoot, o.DataRoot, installtx.ReleaseBinding{ManifestSHA256: review.Current.ManifestSHA256, ClientInputsSHA256: review.Current.ClientInputsSHA256}, time.Now())
	if err != nil {
		return ClientReview{}, err
	}
	installerName := "maestro-install"
	if state.TargetOS == "windows" {
		installerName += ".exe"
	}
	readers := []struct{ path, component, version string }{{o.Self, "installer", o.Version}, {filepath.Join(o.ManagedRoot, installerName), "installer", o.Version}, {installedExecutablePath(o.ManagedRoot, state.TargetOS, state.Profile), "server", state.Server.Version}}
	for _, reader := range readers {
		if err := RequireClientPreparationCapabilities(ctx, reader.path, reader.component, reader.version, o.AuthorityRegistrySHA256); err != nil {
			return ClientReview{}, err
		}
	}
	review.Effects = []string{"prepare_and_inspect_complete_Desktop_connector_and_live_methods", "preserve_selected_both_carrier_and_full_Code_profile", "leave_Code_qualification_and_registration_pending_without_invocation", "publish_strict_partial_client_record_last", "preserve_previous_and_partial_packets_for_review"}
	if o.Request.Reconcile {
		review.Effects = append(review.Effects, "reclaim_only_dead_installation_lock_and_reconcile_actual_owned_packets")
	}
	review.Digest = ""
	body, err := releasecontract.CanonicalJSON(review)
	if err != nil {
		return ClientReview{}, err
	}
	review.Digest = clientDigest(body)
	return review, nil
}

// MaterializeDesktopPreparationSeeded composes the genuine Desktop owner with
// the unchanged full selection. Every retry invokes and inspects the builder;
// neither occupied files nor a historical record can promote the pending Code.
func MaterializeDesktopPreparationSeeded(ctx context.Context, o ClientOptions, digest string, confirmed bool, build ClientMaterializer) (ClientRecord, error) {
	if err := ctx.Err(); err != nil {
		return ClientRecord{}, err
	}
	if !confirmed || !clientSHA.MatchString(digest) || build == nil {
		return ClientRecord{}, errors.New("exact Desktop preparation review and confirmation required")
	}
	review, err := ReviewDesktopPreparationSeeded(ctx, o)
	if err != nil || review.Digest != digest {
		return ClientRecord{}, errors.New("Desktop preparation review changed")
	}
	var result ClientRecord
	guard := installtx.GuardQualifiedCurrent
	if o.Request.Reconcile {
		guard = installtx.GuardQualifiedReconciliationCurrent
	}
	err = guard(o.ManagedRoot, o.DataRoot, installtx.ReleaseBinding{ManifestSHA256: review.Current.ManifestSHA256, ClientInputsSHA256: review.Current.ClientInputsSHA256}, time.Now(), func() error {
		fresh, err := ReviewDesktopPreparationSeeded(ctx, o)
		if err != nil || fresh.Digest != digest {
			return errors.New("Desktop preparation review changed before publication")
		}
		state, err := installtx.ReadSourceState(o.DataRoot, o.ManagedRoot)
		if err != nil {
			return err
		}
		var previous *ClientRecord
		old, _, err := readClientRecord(o.SeededOperationOptions, o.Request.CarrierID)
		if err == nil {
			previous = &old
		} else if !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		material, err := buildClientsWithSeedFence(ctx, o, state, review.Binding, previous, build)
		if err != nil {
			return fmt.Errorf("Desktop preparation incomplete; existing packets preserved: %w", err)
		}
		selection := o.Request
		selection.Reconcile, selection.ManualDesktopUpdate = false, false
		result = ClientRecord{SchemaVersion: 3, Profile: StableClientProfile, Protocol: StableClientProtocol, CarrierVersion: StableCarrierVersion, CarrierID: selection.CarrierID, Binding: review.Binding, Materialization: material, Code: pendingClientCode(selection), Desktop: clientDesktopObservation(previous, material), Preparation: &ClientPreparation{Phase: ClientPreparationDesktopReady, Selection: selection}}
		if !sameClientCurrent(material, state) || validateClientPreparationRecord(result) != nil {
			return errors.New("Desktop preparation materializer returned another binding")
		}
		if err := validateClientFilesContext(ctx, material); err != nil {
			return err
		}
		if _, err := installtx.ValidateQualifiedLaunchContext(ctx, o.ManagedRoot, o.DataRoot, state.Binding, time.Now()); err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := requirePreviousClientRecord(o, review.PreviousRecordSHA256); err != nil {
			return err
		}
		return writeClientRecord(o.SeededOperationOptions, result)
	})
	return result, err
}

func requirePreviousClientRecord(o ClientOptions, expected string) error {
	_, body, err := readClientRecord(o.SeededOperationOptions, o.Request.CarrierID)
	if expected == "" && errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil || expected == "" || clientDigest(body) != expected {
		return errors.New("client record changed during preparation; existing state preserved")
	}
	return nil
}

func clientDesktopObservation(previous *ClientRecord, material ClientMaterialization) DesktopObservation {
	observation := DesktopObservation{Handoff: "bundle_ready", InstallConfirmation: "awaiting_user_install", Connection: "pending", FunctionalCheck: "pending", EvidenceClass: "not_observed"}
	if previous != nil && previous.Materialization.DesktopSHA256 == material.DesktopSHA256 && previous.Materialization.DesktopNativeContractSHA256 == material.DesktopNativeContractSHA256 {
		observation = previous.Desktop
		if previous.Materialization.ManifestSHA256 != material.ManifestSHA256 {
			observation.Connection, observation.FunctionalCheck = "restart_required", "pending"
		}
	}
	return observation
}
