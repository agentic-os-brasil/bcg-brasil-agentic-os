package installer

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installtx"
)

// ClientPacketSelection retains the reviewed stable output selections while
// each release publishes into a fresh sibling slot.
type ClientPacketSelection struct {
	Target      string `json:"target,omitempty"`
	CodeRoot    string `json:"code_root"`
	DesktopPath string `json:"desktop_path"`
}

// ClientMethodFrontend is content inventory only. Native Desktop installation,
// discovery and execution remain separate observations.
type ClientMethodFrontend struct {
	BootstrapProtocol string `json:"bootstrap_protocol"`
	Archive           string `json:"archive"`
	SHA256            string `json:"sha256"`
	Version           string `json:"version"`
	Files             int    `json:"files"`
}

type ClientUpdatePhases struct {
	PacketInventory          string `json:"packet_inventory"`
	CodeRegistration         string `json:"code_registration"`
	SessionLoading           string `json:"session_loading"`
	NativeQualification      string `json:"native_qualification"`
	DesktopConnector         string `json:"desktop_connector"`
	DesktopMethods           string `json:"desktop_methods"`
	MaintenanceQualification string `json:"maintenance_qualification"`
	FunctionalAcceptance     string `json:"functional_acceptance"`
}

func ClientPhases(r ClientRecord) *ClientUpdatePhases {
	if r.Materialization.Selection == nil {
		return nil
	}
	p := &ClientUpdatePhases{PacketInventory: "recorded_requires_current_validation", CodeRegistration: "pending", SessionLoading: "reload_reconnect_new_session_pending", NativeQualification: "unavailable_MCP_only_profile", DesktopConnector: r.Desktop.Connection, DesktopMethods: "live_canonical_methods_once_installed_bootstrap_unattested", MaintenanceQualification: "inspect_current_build_per_job_requalification_required", FunctionalAcceptance: "pending_actual_host_checks"}
	if r.Code.State == "registered" {
		p.CodeRegistration = "historical_supported_CLI_readback"
	}
	if r.Materialization.DesktopOnly() {
		p.CodeRegistration = "not_selected"
		p.NativeQualification = "not_applicable_Desktop_has_no_Code_hooks"
	}
	if r.Materialization.NativeEnforcement {
		p.NativeQualification = "genuine_producer_observed_for_packet_not_current_session"
	}
	return p
}

func validateClientMethodBinding(m ClientMaterialization) error {
	if m.Selection == nil {
		if m.DesktopMethods != nil {
			return errors.New("legacy record contains versioned methods")
		}
		return nil
	}
	f := m.DesktopMethods
	if f == nil || f.BootstrapProtocol != "maestro-live-method-bootstrap-v1" || f.Version != m.Release || f.Files < 1 || f.Files > 4096 || !clientSHA.MatchString(f.SHA256) || f.Archive != filepath.Join(ClientMethodsRoot(m), "maestro-desktop-methods-"+m.Release+".zip") {
		return errors.New("Desktop method frontend differs from current generation")
	}
	return nil
}

func sameClientSelection(a, b *ClientPacketSelection) bool {
	return a == nil && b == nil || a != nil && b != nil && *a == *b
}

func ClientGenerationPaths(r ClientRequest, manifest string) (ClientMaterialization, error) {
	m := ClientMaterialization{Target: r.Target, CodeRoot: r.CodeRoot, DesktopPath: r.DesktopPath}
	if err := validateClientTarget(r); err != nil {
		return m, err
	}
	if !r.VersionedSlots {
		if r.GenerationID != "" {
			return m, errors.New("legacy clients have no packet generation ID")
		}
		return m, nil
	}
	if !carrierIDPattern.MatchString(r.CarrierID) || !clientSHA.MatchString(manifest) {
		return m, errors.New("versioned client carrier/manifest binding required")
	}
	if r.GenerationID != "" && !carrierIDPattern.MatchString(r.GenerationID) {
		return m, errors.New("closed 32-hex packet generation ID required")
	}
	paths := []string{r.DesktopPath}
	if !r.DesktopOnly() {
		paths = append(paths, r.CodeRoot)
	}
	for _, path := range paths {
		if !filepath.IsAbs(path) || filepath.Clean(path) != path || strings.ContainsAny(path, "\x00\r\n") {
			return m, errors.New("versioned client selection must be clean absolute paths")
		}
	}
	if filepath.Ext(r.DesktopPath) != ".mcpb" {
		return m, errors.New("versioned Desktop selection requires .mcpb")
	}
	m.Selection = &ClientPacketSelection{Target: r.Target, CodeRoot: r.CodeRoot, DesktopPath: r.DesktopPath}
	generationIdentity := r.CarrierID + "\x00" + manifest + "\x00" + r.GenerationID
	if r.DesktopOnly() {
		generationIdentity += "\x00desktop"
	}
	identity := sha256.Sum256([]byte(generationIdentity))
	suffix := ".generation-" + fmt.Sprintf("%x", identity[:16])
	m.GenerationID = r.GenerationID
	if !r.DesktopOnly() {
		m.CodeRoot = r.CodeRoot + suffix
	}
	m.DesktopPath = strings.TrimSuffix(r.DesktopPath, ".mcpb") + suffix + ".mcpb"
	return m, nil
}

func ClientRequestFromRecord(r ClientRecord) ClientRequest {
	request := ClientRequest{Target: r.Materialization.Target, CarrierID: r.CarrierID, Workspace: r.Binding.Workspace, CodeRoot: r.Materialization.CodeRoot, DesktopPath: r.Materialization.DesktopPath, CodeExecutable: r.Code.Executable, CodeConfigurationDirectory: r.Code.ConfigurationDirectory, CodeScope: r.Code.Scope, LifecycleContext: r.Materialization.LifecycleContext}
	request.NativeEnforcement = r.Materialization.NativeEnforcement
	request.OwnerReviewChannel = r.Materialization.OwnerReviewChannel
	request.GenerationID = r.Materialization.GenerationID
	if s := r.Materialization.Selection; s != nil {
		request.VersionedSlots = true
		request.CodeRoot, request.DesktopPath = s.CodeRoot, s.DesktopPath
	}
	return request
}

func ClientMarketplaceRoot(r ClientRecord) string {
	if r.Materialization.DesktopOnly() {
		return ""
	}
	if s := r.Materialization.Selection; s != nil {
		return s.CodeRoot + ".provider-" + r.CarrierID
	}
	return r.Materialization.CodeRoot
}

// CurrentClientPluginPath reads the same admitted current record as stable-launch.
// It never generates, repairs, selects a source or activates a release.
func CurrentClientPluginPath(ctx context.Context, o ClientOptions) (string, error) {
	if _, err := admitOperationSeed(ctx, o.SeededOperationOptions); err != nil {
		return "", err
	}
	// The supported Code command may run under RegisterCodeSeeded's existing
	// installation lock. This read-only provider must not recursively acquire
	// that lock in a child process. Bracket the snapshot with current/record
	// identity readback; the registration owner separately verifies the copy.
	state, err := installtx.ValidateQualifiedLaunch(o.ManagedRoot, o.DataRoot, installtx.ReleaseBinding{}, time.Now())
	if err != nil {
		return "", err
	}
	r, before, err := readClientRecord(o.SeededOperationOptions, o.Request.CarrierID)
	if err != nil {
		return "", err
	}
	if r.Materialization.DesktopOnly() || r.Materialization.Selection == nil || r.Binding.Workspace != o.Request.Workspace {
		return "", errors.New("current versioned client binding required")
	}
	bound := o
	bound.Request = ClientRequestFromRecord(r)
	status, err := ReadClientsSeeded(ctx, bound)
	if err != nil {
		return "", err
	}
	if status.State != "current_materialization" {
		return "", errors.New("current client generation is unavailable")
	}
	afterState, err := installtx.ValidateQualifiedLaunch(o.ManagedRoot, o.DataRoot, state.Binding, time.Now())
	if err != nil || afterState.Server != state.Server {
		return "", errors.New("current plugin source changed during readback")
	}
	_, after, err := readClientRecord(o.SeededOperationOptions, o.Request.CarrierID)
	if err != nil || clientDigest(before) != clientDigest(after) {
		return "", errors.New("current client record changed during readback")
	}
	if err := recheckExperimentalOperation(o.SeededOperationOptions); err != nil {
		return "", err
	}
	return filepath.Join(r.Materialization.CodeRoot, "plugins", "maestro-product"), nil
}
