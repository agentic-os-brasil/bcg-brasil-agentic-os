package installer

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
)

const DesktopClientTargetProtocol = "maestro-client-target-desktop-v1"

// Empty target preserves the legacy both-client request and its exact JSON.
func (r ClientRequest) DesktopOnly() bool         { return r.Target == "desktop" }
func (m ClientMaterialization) DesktopOnly() bool { return m.Target == "desktop" }
func validateClientTarget(r ClientRequest) error {
	if r.Target == "" {
		return nil
	}
	if !r.DesktopOnly() {
		return errors.New("unsupported client target")
	}
	if !r.VersionedSlots || r.CodeRoot != "" || r.CodeScope != "" || r.CodeExecutable != "" || r.CodeConfigurationDirectory != "" || r.NativeEnforcement || r.LifecycleContext {
		return errors.New("Desktop target requires versioned Desktop-only inputs without Code configuration")
	}
	return nil
}
func clientRecordSchema(target string) int {
	if target == "desktop" {
		return 2
	}
	return 1
}
func validateClientTargetRecord(r ClientRecord) error {
	if r.Materialization.Target == "" {
		if r.SchemaVersion != 1 || r.Materialization.Selection != nil && r.Materialization.Selection.Target != "" {
			return errors.New("unsupported client record schema")
		}
		return nil
	}
	if !r.Materialization.DesktopOnly() || r.SchemaVersion != 2 || validateClientTarget(ClientRequestFromRecord(r)) != nil || r.Materialization.Selection == nil || r.Materialization.Selection.Target != "desktop" || validateDesktopClientShape(r.Materialization) != nil || r.Code != (CodeObservation{State: "not_selected", EvidenceClass: "not_applicable"}) {
		return errors.New("invalid Desktop target current record")
	}
	return nil
}

// Target capabilities are a separate command: legacy capability JSON is unchanged.
type ClientTargetCapability struct {
	SchemaVersion  int    `json:"schema_version"`
	Product        string `json:"product"`
	Component      string `json:"component"`
	Version        string `json:"version"`
	RegistrySHA256 string `json:"authority_registry_sha256"`
	Protocol       string `json:"protocol"`
}

func DesktopClientCapabilities(component, version, pin string) ClientTargetCapability {
	return ClientTargetCapability{1, "maestro", component, version, pin, DesktopClientTargetProtocol}
}
func requireDesktopClientCapabilities(ctx context.Context, path, component, version, pin string) error {
	bounded, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	body, err := execCommand(bounded, path, "client-target-capabilities")
	if err != nil {
		return errors.New("Desktop target requires a compatible installed bootstrap and current server")
	}
	var c ClientTargetCapability
	if len(body) > 64<<10 || releasecontract.DecodeStrictJSON(body, &c) != nil || c != DesktopClientCapabilities(component, version, pin) {
		return errors.New("Desktop target reader capability differs from authenticated component")
	}
	return nil
}

// ClientMethodsRoot owns the complete method frontend independently of Code.
func ClientMethodsRoot(m ClientMaterialization) string {
	if m.DesktopOnly() {
		return strings.TrimSuffix(m.DesktopPath, ".mcpb") + ".desktop-methods"
	}
	return m.CodeRoot + ".desktop-methods"
}
func ClientManagementRoot(m ClientMaterialization) string {
	if m.DesktopOnly() {
		return ClientMethodsRoot(m)
	}
	return m.CodeRoot
}
func validateDesktopClientShape(m ClientMaterialization) error {
	if m.Selection == nil || m.Selection.Target != "desktop" || m.Selection.CodeRoot != "" || m.CodeRoot != "" || m.CodeTreeSHA256 != "" || m.CodeProviderRoot != "" || m.CodeProviderSHA256 != "" || m.CodeBackupTreeSHA256 != "" || m.DesktopBackupSHA256 != "" || m.NativeEnforcement || m.LifecycleContext || len(m.Backups) != 0 {
		return errors.New("Desktop target contains Code or unsupported backup state")
	}
	return nil
}
func validateDesktopClientFiles(m ClientMaterialization) error {
	return validateDesktopClientFilesContext(context.Background(), m)
}

func validateDesktopClientFilesContext(ctx context.Context, m ClientMaterialization) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := validateDesktopClientShape(m); err != nil {
		return err
	}
	if validateClientMethodBinding(m) != nil {
		return errors.New("Desktop target method frontend differs")
	}
	for _, v := range []string{m.DesktopSHA256, m.DesktopNativeContractSHA256, m.BootstrapServerSHA256, m.BootstrapEmbeddedInputsSHA256} {
		if !clientSHA.MatchString(v) {
			return errors.New("Desktop materialization digest missing")
		}
	}
	archive, err := ClientArchiveDigestContext(ctx, m.DesktopPath)
	if err != nil || archive != m.DesktopSHA256 {
		return errors.New("Desktop target package changed")
	}
	methods, err := ClientFileDigestContext(ctx, m.DesktopMethods.Archive)
	if err != nil || methods != m.DesktopMethods.SHA256 {
		return errors.New("Desktop target methods changed")
	}
	if filepath.Clean(m.DesktopPath) != m.DesktopPath {
		return errors.New("Desktop target path differs")
	}
	return ctx.Err()
}
