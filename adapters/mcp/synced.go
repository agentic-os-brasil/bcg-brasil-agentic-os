package product

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installtx"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/processidentity"
)

var ErrSourceRecoveryRequired = errors.New("signed source unavailable; use the independent maestro-install source-review/source-check recovery commands")

func (b *Bound) installedExpectation() installtx.ReleaseBinding {
	return installtx.ReleaseBinding{ManifestSHA256: b.options.InstalledManifestSHA256, ClientInputsSHA256: b.options.InstalledClientInputsSHA256}
}

type sourceStateReader func(string, string) (installtx.State, error)

// sourceOwningContext preserves the one owning observation used to classify
// this caller. It is a snapshot for routing, never authentication authority.
type sourceOwningContext struct {
	required bool
	state    *installtx.State
}

func (b *Bound) sourceRequired() (bool, error) {
	return b.sourceRequiredContext(context.Background())
}

func (b *Bound) sourceRequiredContext(ctx context.Context) (bool, error) {
	owning, err := b.sourceContextWithContext(ctx)
	return owning.required, err
}
func (b *Bound) sourceContext() (sourceOwningContext, error) {
	return b.sourceContextWithContext(context.Background())
}

func (b *Bound) sourceContextWithContext(ctx context.Context) (sourceOwningContext, error) {
	return b.sourceContextWithReaderContext(ctx, func(dataRoot, managedRoot string) (installtx.State, error) {
		return b.readInstalledSourceStateContext(ctx, dataRoot, managedRoot)
	})
}
func (b *Bound) sourceContextWithReader(read sourceStateReader) (sourceOwningContext, error) {
	return b.sourceContextWithReaderContext(context.Background(), read)
}

func (b *Bound) sourceContextWithReaderContext(ctx context.Context, read sourceStateReader) (sourceOwningContext, error) {
	if err := ctx.Err(); err != nil {
		return sourceOwningContext{}, err
	}
	pinned := b.options.InstalledManifestSHA256 != "" || b.options.InstalledClientInputsSHA256 != ""
	installed := false
	if b.options.GuardProcess {
		identity, err := processidentity.Current()
		if err != nil {
			return sourceOwningContext{}, err
		}
		directory := filepath.Dir(identity.Path)
		name := filepath.Base(identity.Path)
		if name == "maestro-mcp" || name == "maestro-mcp.exe" {
			if filepath.Base(directory) == "bin" {
				installed = true
				if filepath.Clean(b.options.ProductRoot) != filepath.Dir(directory) {
					return sourceOwningContext{}, ErrSourceRecoveryRequired
				}
			} else if (filepath.Base(directory) == "scripts" || filepath.Base(directory) == "server") && filepath.Dir(directory) == filepath.Clean(b.options.ProductRoot) {
				// Ordinary unbound packets retain their existing MCP profile.
				// They do not become qualified installed-source launchers by
				// carrying a source-capable executable or owner selections.
				return sourceOwningContext{required: pinned}, nil
			}
		}
	}
	dataRoot := filepath.Join(b.options.UserHome, ".maestro-install")
	state, err := read(dataRoot, b.options.ProductRoot)
	if cancelled := ctx.Err(); cancelled != nil {
		return sourceOwningContext{}, cancelled
	}
	if errors.Is(err, os.ErrNotExist) {
		return sourceOwningContext{required: installed || pinned}, nil
	}
	if err != nil {
		return sourceOwningContext{}, err
	}
	// Enrollment is an owning state transition, never an option supplied by
	// the model. Once enrolled, direct callbacks and publication guards enforce
	// current source custody even when the optional carrier pins are omitted.
	// A source-capable first installation before enrollment keeps plain MCP
	// available but cannot grant a qualified source launch.
	return sourceOwningContext{required: pinned || state.Source != nil || installed && state.SchemaVersion != 4, state: &state}, nil
}

func (b *Bound) validateInstalledSource() error {
	return b.validateInstalledSourceContext(context.Background())
}

func (b *Bound) validateInstalledSourceContext(ctx context.Context) error {
	return b.validateInstalledSourceWithContext(ctx, b.sourceRequiredContext, b.validatedQualifiedStateContext)
}

// validateContext has just performed full experimental installed admission.
// Share that entry observation only with directly nested read-only work in this
// call. The full qualified validator and its final experimental fence stay fresh;
// no observation escapes into another validation, callback or process.
func (b *Bound) validateInstalledSourceAfterExperimentalCheckContext(ctx context.Context, read sourceStateReader) error {
	required := func(ctx context.Context) (bool, error) {
		owning, err := b.sourceContextWithReaderContext(ctx, func(dataRoot, managedRoot string) (installtx.State, error) {
			state, err := read(dataRoot, managedRoot)
			if err != nil {
				return state, err
			}
			if err := b.checkExperimentalStateRoleContext(ctx, state); err != nil {
				return installtx.State{}, err
			}
			return state, ctx.Err()
		})
		if err == nil && !owning.required {
			// Pre-enrollment has no qualified-work fence below. Reauthenticate
			// the complete experimental binding after its classification read.
			err = b.validateExperimentalSeedContext(ctx)
		}
		return owning.required, err
	}
	validate := func(ctx context.Context, expected installtx.ReleaseBinding) (installtx.State, error) {
		state, err := installtx.ValidateQualifiedLaunchContext(ctx, b.options.ProductRoot, filepath.Join(b.options.UserHome, ".maestro-install"), expected, time.Now())
		if err != nil {
			return state, err
		}
		if err := b.fenceExperimentalStateContext(ctx, state); err != nil {
			return installtx.State{}, err
		}
		return state, ctx.Err()
	}
	return b.validateInstalledSourceWithContext(ctx, required, validate)
}

func (b *Bound) validateInstalledSourceWithContext(ctx context.Context, required func(context.Context) (bool, error), validate func(context.Context, installtx.ReleaseBinding) (installtx.State, error)) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	needed, err := required(ctx)
	if err != nil {
		if cancelled := ctx.Err(); cancelled != nil {
			return cancelled
		}
		return ErrSourceRecoveryRequired
	}
	if !needed {
		return nil
	}
	state, err := validate(ctx, b.installedExpectation())
	if err != nil {
		if cancelled := ctx.Err(); cancelled != nil {
			return cancelled
		}
		return ErrSourceRecoveryRequired
	}
	if b.options.GuardProcess {
		identity, err := processidentity.Current()
		if err != nil {
			return err
		}
		if identity.SHA256 != state.Server.SHA256 || b.options.CLIVersion != state.Server.Version {
			return processidentity.ErrChanged
		}
	}
	return ctx.Err()
}

func (b *Bound) guardPublication(publish func() error) error {
	return b.guardPublicationContext(context.Background(), publish)
}

func (b *Bound) guardPublicationContext(ctx context.Context, publish func() error) error {
	if err := b.validateContext(ctx); err != nil {
		return err
	}
	required, err := b.sourceRequiredContext(ctx)
	if err != nil {
		return err
	}
	if !required {
		if err := ctx.Err(); err != nil {
			return err
		}
		return publish()
	}
	return b.guardQualifiedPublicationContext(ctx, publish)
}

// guardQualifiedPublication keeps a caller already classified as qualified
// within the original qualified installation owner. It cannot recategorize
// a recovered Source=nil state into the ordinary unqualified wrapper.
func (b *Bound) guardQualifiedPublication(publish func() error) error {
	return b.guardQualifiedPublicationContext(context.Background(), publish)
}

func (b *Bound) guardQualifiedPublicationContext(ctx context.Context, publish func() error) error {
	if err := b.validateContext(ctx); err != nil {
		return err
	}
	return installtx.GuardQualifiedCurrentContext(ctx, b.options.ProductRoot, filepath.Join(b.options.UserHome, ".maestro-install"), b.installedExpectation(), time.Now(), func() error {
		// Repeat running identity and the complete owner/workspace binding
		// under the same canonical installation lock before publication.
		if err := b.validateContext(ctx); err != nil {
			return err
		}
		return b.withExperimentalSeedFenceContext(ctx, publish)
	})
}
