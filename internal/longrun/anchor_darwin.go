//go:build darwin

package longrun

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"strings"
)

const defaultKeychainAnchorService = "com.bcgbrasil.maestro.longrun.anchor.v1"

// DefaultAnchor uses the logged-in user's macOS Keychain. The anchor survives
// normal restoration of the product's local-data directory, which is exactly
// the property required to reject an old, otherwise valid event-log prefix.
func DefaultAnchor() (MonotonicAnchor, error) {
	if _, err := exec.LookPath("security"); err != nil {
		return nil, ErrMonotonicAnchorUnavailable
	}
	return &keychainAnchor{service: defaultKeychainAnchorService, run: runSecurity}, nil
}

type keychainAnchor struct {
	service string
	run     func(args ...string) ([]byte, error)
}

func runSecurity(args ...string) ([]byte, error) { return exec.Command("security", args...).Output() }

func (anchor *keychainAnchor) Load(goalID string) (AnchorRecord, error) {
	output, err := anchor.run("find-generic-password", "-s", anchor.service, "-a", goalID, "-w")
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return AnchorRecord{}, os.ErrNotExist
		}
		var exit *exec.ExitError
		if errors.As(err, &exit) && exit.ExitCode() == 44 {
			return AnchorRecord{}, os.ErrNotExist
		}
		return AnchorRecord{}, ErrMonotonicAnchorUnavailable
	}
	decoded, err := base64.RawStdEncoding.DecodeString(strings.TrimSpace(string(output)))
	if err != nil {
		return AnchorRecord{}, errors.New("invalid macOS keychain long-running anchor")
	}
	var record AnchorRecord
	if err := json.Unmarshal(decoded, &record); err != nil {
		return AnchorRecord{}, errors.New("invalid macOS keychain long-running anchor")
	}
	return record, nil
}

func (anchor *keychainAnchor) Store(next AnchorRecord) error {
	if current, err := anchor.Load(next.GoalID); err == nil {
		if next.Sequence < current.Sequence || (next.Sequence == current.Sequence && next.MAC != current.MAC) {
			return errors.New("macOS keychain long-running anchor cannot move backward")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	encoded, err := json.Marshal(next)
	if err != nil {
		return err
	}
	secret := base64.RawStdEncoding.EncodeToString(encoded)
	if _, err := anchor.run("add-generic-password", "-U", "-s", anchor.service, "-a", next.GoalID, "-w", secret); err != nil {
		return ErrMonotonicAnchorUnavailable
	}
	return nil
}
