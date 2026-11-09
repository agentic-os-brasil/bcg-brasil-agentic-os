package setup

import (
	"context"
	"errors"
	"path/filepath"
	"runtime"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp/packaging"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installer"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installtx"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
)

// The stable installer may predate the current release's canonical methods.
// Invoke the actual authenticated current generator's same closed validator;
// older linked assets and a self-consistent diagnostic inventory are insufficient.
func inspectCurrentPacket(ctx context.Context, path, format string, o installer.ClientOptions, identity releasecontract.ServerIdentity, serverSHA string) (packaging.PacketStatus, error) {
	state, err := installtx.ValidateQualifiedLaunch(o.ManagedRoot, o.DataRoot, installtx.ReleaseBinding{}, time.Now())
	if err != nil {
		return packaging.PacketStatus{}, err
	}
	if state.Server.SHA256 != serverSHA || state.Server.Version != identity.Version || state.Server.EmbeddedInputsSHA256 != identity.EmbeddedInputsSHA256 {
		return packaging.PacketStatus{}, errors.New("current packet verifier source identity changed")
	}
	name := "maestro-mcp"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	server := filepath.Join(o.ManagedRoot, "bin", name)
	actual, err := installer.ClientFileDigest(server)
	if err != nil || actual != serverSHA {
		return packaging.PacketStatus{}, errors.New("current packet verifier executable changed")
	}
	args := []string{"packet-verify", "--destination", path, "--packet-format", format, "--carrier-profile", installer.StableClientProfile, "--carrier-id", o.Request.CarrierID, "--carrier-version", installer.StableCarrierVersion}
	body, err := clientCommand(ctx, server, args, o.Request.Workspace)
	if err != nil {
		return packaging.PacketStatus{}, err
	}
	var status packaging.PacketStatus
	if err := releasecontract.DecodeStrictJSON(body, &status); err != nil {
		return packaging.PacketStatus{}, err
	}
	expected := stableCarrier(o.Request.CarrierID)
	expectedMode := "mcp_only"
	if format == "plugin" && o.Request.LifecycleContext {
		expectedMode = "mcp_lifecycle"
	}
	if status.Stable == nil || *status.Stable != expected || status.Format != format || status.ServerSHA256 != serverSHA || status.Version != identity.Version || status.EmbeddedInputsSHA256 != identity.EmbeddedInputsSHA256 || status.RegistrySHA256 != identity.AuthorityRegistrySHA256 || status.NativeMode != expectedMode {
		return packaging.PacketStatus{}, errors.New("actual current packet verification differs")
	}
	return status, nil
}
