package installtx

import (
	"errors"
	"runtime"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
)

// ValidateActivatedCurrent reads an authenticated initial schema4 activation
// before source enrollment. Exact immutable pins are mandatory. Enrolled source
// custody belongs exclusively to ValidateQualifiedLaunch; this is no fallback.
func ValidateActivatedCurrent(managedRoot, dataRoot string, expected ReleaseBinding) (State, error) {
	if !sha256Pattern.MatchString(expected.ManifestSHA256) || !sha256Pattern.MatchString(expected.ClientInputsSHA256) {
		return State{}, errors.New("exact initial Current manifest/client binding required")
	}
	state, err := ReadSourceState(dataRoot, managedRoot)
	if err != nil {
		return State{}, err
	}
	if state.SchemaVersion != 4 || state.Profile != releasecontract.ModernProfile || state.Source != nil || state.TargetOS != runtime.GOOS || state.TargetArch != runtime.GOARCH {
		return State{}, errors.New("initial Current requires a modern schema4 activation before source enrollment")
	}
	if state.Binding.ManifestSHA256 != expected.ManifestSHA256 || state.Binding.ClientInputsSHA256 != expected.ClientInputsSHA256 {
		return State{}, errors.New("initial Current release binding changed")
	}
	if err = CheckActivatedServer(managedRoot, state); err != nil {
		return State{}, err
	}
	if err = verifyReleaseBinding(managedRoot, dataRoot, state, state.Binding, state.Server, true); err != nil {
		return State{}, err
	}
	return state, nil
}

// GuardActivatedCurrent reuses the installation owner and validates the exact
// initial activation under its existing lock. It never reclaims a lock, enrolls
// source, migrates state or supplies native/placement authority.
func GuardActivatedCurrent(managedRoot, dataRoot string, expected ReleaseBinding, publish func() error) error {
	if publish == nil {
		return errors.New("publication callback required")
	}
	unlock, err := acquireLock(managedRoot)
	if err != nil {
		return err
	}
	defer unlock()
	if _, err = ValidateActivatedCurrent(managedRoot, dataRoot, expected); err != nil {
		return err
	}
	return publish()
}
