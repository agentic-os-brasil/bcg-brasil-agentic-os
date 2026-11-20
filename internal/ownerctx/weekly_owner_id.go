package ownerctx

import (
	"errors"
	"os"
)

// ReadWeeklySelfOwnerID reads an existing namespace label, never a person or
// enterprise identity. It must not initialize, relabel or approve any source.
func ReadWeeklySelfOwnerID(root string) (string, error) {
	label := ""
	merge := func(value string) error {
		if value == "" {
			return nil
		}
		if !observationIdentifier.MatchString(value) || label != "" && label != value {
			return ErrWeeklySelfChanged
		}
		label = value
		return nil
	}
	err := withWeeklySelfOwner(root, func() error {
		return withWeeklyHistorySnapshot(root, func(records []PromptHistoryEntry, config PromptHistoryConfig) error {
			if err := merge(config.OwnerID); err != nil {
				return err
			}
			for _, entry := range records {
				if err := merge(entry.OwnerID); err != nil {
					return err
				}
			}
			err := withWeeklySelfState(root, false, func(state *weeklySelfState) error { return merge(state.OwnerID) })
			if !errors.Is(err, os.ErrNotExist) {
				return err
			}
			// Absence is not initialization. An orphan state without its
			// original serialization lock is inconsistent, not a new namespace.
			if _, readErr := readWeeklySelfState(root); readErr == nil {
				return ErrWeeklySelfChanged
			} else if !errors.Is(readErr, os.ErrNotExist) {
				return readErr
			}
			return nil
		})
	})
	if err != nil {
		return "", err
	}
	return label, nil
}
