package memory

import (
	"errors"
	"runtime"
)

// This gate is independent of ordinary owned-memory confinement. Enabling that
// platform elsewhere cannot qualify private-wiki publication or retirement.
var ErrPrivateWikiNativeUnavailable = errors.New("private wiki native confinement is unqualified")

func privateWikiNativeAvailable() error {
	if runtime.GOOS == "windows" {
		return ErrPrivateWikiNativeUnavailable
	}
	return nil
}
