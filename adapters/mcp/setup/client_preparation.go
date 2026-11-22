package setup

import (
	"context"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installer"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installtx"
)

// ApplyDesktopPreparation keeps the original full both-client selection at the
// canonical publisher. Only its trusted builder receives the Desktop projection;
// neither Code's qualifier nor its registration process is invoked here.
func ApplyDesktopPreparation(ctx context.Context, o installer.ClientOptions, digest string, confirmed bool) (installer.ClientRecord, error) {
	projection, err := installer.DesktopPreparationOptions(o)
	if err != nil {
		return installer.ClientRecord{}, err
	}
	build := desktopClientMaterializer(projection)
	return installer.MaterializeDesktopPreparationSeeded(ctx, o, digest, confirmed, func(ctx context.Context, state installtx.State, binding installer.ClientBinding, previous *installer.ClientRecord) (installer.ClientMaterialization, error) {
		var desktopPrevious *installer.ClientRecord
		if previous != nil {
			view, err := installer.DesktopPreparationRecord(*previous)
			if err != nil {
				return installer.ClientMaterialization{}, err
			}
			desktopPrevious = &view
		}
		return build(ctx, state, binding, desktopPrevious)
	})
}
