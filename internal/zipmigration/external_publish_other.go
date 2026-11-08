//go:build !darwin && !linux

package zipmigration

import "errors"

func externalRenameExclusive(int, string, int, string) error {
	return errors.New("external atomic publication is unavailable on this platform")
}
