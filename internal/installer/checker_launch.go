package installer

import (
	"context"
	"errors"
	"os"
	"reflect"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/processidentity"
)

// VerifiedStableCheck is process-local authority to omit this child's automatic
// worker, never authority for a tool, model, job or domain mutation. Its zero
// value and values constructed outside the canonical admission owner refuse.
type VerifiedStableCheck struct {
	options  ClientOptions
	binding  StableCheckBinding
	admitted bool
}

type StableCheckBinding struct {
	ProductRoot, UserHome, Workspace, CarrierID, InstallerVersion string
	ManifestSHA256, ClientInputsSHA256                            string
}

// AdmitStableCheckServer repeats the original stable source/current/packet
// admission in the real receiver. Actual image and exact canonical argv bind
// the closed checker purpose to the existing authenticated child path.
func AdmitStableCheckServer(ctx context.Context, o ClientOptions) (*VerifiedStableCheck, error) {
	launch, err := PrepareStableCheckLaunch(ctx, o)
	if err != nil {
		return nil, err
	}
	actual, err := processidentity.Current()
	if err != nil {
		return nil, err
	}
	if actual.Path != launch.Executable || actual.SHA256 != launch.State.Server.SHA256 || !reflect.DeepEqual(os.Args[1:], launch.Args) || o.Request.CarrierID == "" {
		return nil, errors.New("authenticated installed checker child required")
	}
	binding := StableCheckBinding{ProductRoot: o.ManagedRoot, UserHome: o.UserHome, Workspace: o.Request.Workspace, CarrierID: o.Request.CarrierID, InstallerVersion: o.Version, ManifestSHA256: launch.State.Binding.ManifestSHA256, ClientInputsSHA256: launch.State.Binding.ClientInputsSHA256}
	return &VerifiedStableCheck{options: o, binding: binding, admitted: true}, ctx.Err()
}

func (c *VerifiedStableCheck) ValidateContext(ctx context.Context, binding StableCheckBinding) error {
	if c == nil || !c.admitted || c.binding != binding {
		return errors.New("authenticated installed checker binding required")
	}
	fresh, err := AdmitStableCheckServer(ctx, c.options)
	if err != nil {
		return err
	}
	if fresh.binding != binding {
		return errors.New("installed checker binding changed")
	}
	return ctx.Err()
}
