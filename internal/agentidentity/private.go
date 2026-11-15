package agentidentity

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

const maximumIdentityBytes = 256 << 10

var ErrIdentityState = errors.New("agent identity state is invalid")
var ErrIdentityChanged = errors.New("agent identity review changed")
var ErrIdentityDraftOpen = errors.New("agent identity review is already pending")

type ProfileState struct {
	Revision string   `json:"profile_revision"`
	Profile  *Profile `json:"profile,omitempty"`
}

// ReadProfile is noncreating. Absence is distinct from invalid, aliased,
// unreadable, oversized or unconfirmed existing state.
func ReadProfile(root string) (ProfileState, error) {
	if err := scheduler.ValidatePrivateDirectory(filepath.Join(root, "agents")); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ProfileState{Revision: "absent"}, nil
		}
		return ProfileState{}, errors.Join(ErrIdentityState, err)
	}
	body, err := scheduler.ReadPrivateStateFile(filepath.Join(root, "agents", "personalization.json"), maximumIdentityBytes)
	if errors.Is(err, os.ErrNotExist) {
		return ProfileState{Revision: "absent"}, nil
	}
	if err != nil {
		return ProfileState{}, errors.Join(ErrIdentityState, err)
	}
	var profile Profile
	if !utf8.Valid(body) {
		return ProfileState{}, ErrIdentityState
	}
	if err = DecodeStrict(body, &profile); err != nil {
		return ProfileState{}, errors.Join(ErrIdentityState, err)
	}
	if err = profile.Validate(); err != nil {
		return ProfileState{}, errors.Join(ErrIdentityState, err)
	}
	sum := sha256.Sum256(body)
	return ProfileState{Revision: hex.EncodeToString(sum[:]), Profile: &profile}, nil
}

func withIdentityTransition(root string, action func() error) error {
	return scheduler.WithPrivateLock(filepath.Join(root, "agents", "interview", ".transition.lock"), action)
}

func validDraftID(id string) bool {
	if !strings.HasPrefix(id, "identity-draft-") || len(id) != len("identity-draft-")+24 {
		return false
	}
	_, err := hex.DecodeString(strings.TrimPrefix(id, "identity-draft-"))
	return err == nil
}

func identityDraftPaths(root string) ([]string, error) {
	path := filepath.Join(root, "agents", "interview", "drafts")
	if err := scheduler.ValidatePrivateDirectory(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	directory, err := scheduler.OpenDirectory(path)
	if err != nil {
		return nil, err
	}
	defer directory.Close()
	entries, err := directory.ReadDir(1025)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	if len(entries) > 1024 {
		return nil, errors.New("agent identity draft enumeration exceeds its bound")
	}
	paths := []string{}
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".json") {
			paths = append(paths, filepath.Join(path, entry.Name()))
		}
	}
	sort.Strings(paths)
	return paths, nil
}
