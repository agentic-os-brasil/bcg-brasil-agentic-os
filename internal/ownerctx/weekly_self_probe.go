package ownerctx

import (
	"context"
	"errors"
	"time"
)

// InspectWeeklySelfInput builds one bounded immutable PRIVATE in-memory view
// under the same canonical/history/observation read policy as normal weekly
// input. It writes no manifest, working projection, journal or message and
// cannot publish. The trusted qualify owner alone binds its probe intent/grant.
// A new attended probe gets its own occurrence; no persistent retry cache exists.
func InspectWeeklySelfInput(ctx context.Context, root string, options WeeklySelfOptions, normalize func(string, string, string) (string, error), now time.Time) (input WeeklySelfInput, err error) {
	if ctx == nil {
		return input, errors.New("weekly probe context is required")
	}
	if err := ctx.Err(); err != nil {
		return input, err
	}
	options.Facets = append([]string(nil), options.Facets...)
	if err := validateWeeklyOptions(options, now); err != nil {
		return input, err
	}
	err = withWeeklySelfSources(root, options, func(sources weeklySelfSources) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		var buildErr error
		input, buildErr = buildWeeklySelfInput(options, sources, normalize, now, 0)
		if buildErr != nil {
			return buildErr
		}
		return ctx.Err()
	})
	if err != nil {
		return WeeklySelfInput{}, err
	}
	return input, nil
}

// ValidateWeeklySelfPolicy validates only a declarative selection policy, using
// the owning weekly validator. It grants no source access or occurrence. Owner
// authorization is derived later from the trusted accepted review, never input.
func ValidateWeeklySelfPolicy(options WeeklySelfOptions) error {
	if options.OccurrenceID != "" || !options.ScheduledFor.IsZero() || options.SensitiveOwnerAuthorized {
		return errors.New("weekly registration policy cannot select occurrence or assert owner authorization")
	}
	options.OccurrenceID = "0000000000000000000000000000000000000000000000000000000000000000"
	at := time.Unix(1, 0).UTC()
	options.ScheduledFor = at
	return validateWeeklyOptions(options, at)
}

// ValidateWeeklySelfInputCurrent rechecks the selected immutable source bindings
// before input release. Unrelated appends do not select a new snapshot.
func ValidateWeeklySelfInputCurrent(ctx context.Context, root string, input WeeklySelfInput, now time.Time) error {
	return WithCurrentWeeklySelfInput(ctx, root, input, now, func() error { return nil })
}

// WithCurrentWeeklySelfInput keeps source mutation serialization through bounded
// input release. It grants no model or domain publication authority.
func WithCurrentWeeklySelfInput(ctx context.Context, root string, input WeeklySelfInput, now time.Time, action func() error) error {
	if ctx == nil || action == nil {
		return errors.New("weekly input validation needs context and action")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := input.Validate(); err != nil {
		return err
	}
	if !now.Before(input.RecoverUntil) {
		return ErrWeeklySelfExpired
	}
	return withWeeklySelfSources(root, input.Options, func(sources weeklySelfSources) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := validateWeeklySource(sourceFromWeeklyInput(input), sources, now); err != nil {
			return err
		}
		return action()
	})
}
