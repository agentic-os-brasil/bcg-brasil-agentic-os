package activationpolicy

import (
	"encoding/json"
	"errors"
)

// DepthProfile is the deterministic Maestro configuration used to calibrate
// how readily the hub spends additional bounded effort. It is not a user
// authority and it does not remove a safety or governance floor.
type DepthProfile string

const (
	ProfileShallow  DepthProfile = "shallow"
	ProfileBalanced DepthProfile = "balanced"
	ProfileLoopy    DepthProfile = "loopy"
)

// DepthLevel is the depth resolved by Maestro for one episode.
type DepthLevel string

const (
	DepthShallow  DepthLevel = "shallow"
	DepthBalanced DepthLevel = "balanced"
	DepthLoopy    DepthLevel = "loopy"
)

// DepthProfileRules are calibration knobs, not authority-bearing thresholds.
// The policy version and digest pin the exact rules used by an episode.
type DepthProfileRules struct {
	PracticeNeedDepth DepthLevel `json:"practice_need_depth"`
	UncertaintyDepth  DepthLevel `json:"uncertainty_depth"`
}

type DepthPolicyConfig struct {
	Version        string                             `json:"version"`
	DefaultProfile DepthProfile                       `json:"default_profile"`
	Profiles       map[DepthProfile]DepthProfileRules `json:"profiles"`
}

// DefaultDepthPolicy is intentionally small: it provides one calibratable
// profile choice while keeping composition and governance decisions in Maestro.
func DefaultDepthPolicy() DepthPolicyConfig {
	return DepthPolicyConfig{
		Version:        PolicyVersion,
		DefaultProfile: ProfileBalanced,
		Profiles: map[DepthProfile]DepthProfileRules{
			ProfileShallow: {
				PracticeNeedDepth: DepthBalanced,
				UncertaintyDepth:  DepthShallow,
			},
			ProfileBalanced: {
				PracticeNeedDepth: DepthBalanced,
				UncertaintyDepth:  DepthBalanced,
			},
			ProfileLoopy: {
				PracticeNeedDepth: DepthLoopy,
				UncertaintyDepth:  DepthLoopy,
			},
		},
	}
}

func (config DepthPolicyConfig) Validate() error {
	if !validID(config.Version) || !validDepthProfile(config.DefaultProfile) {
		return errors.New("depth policy has an invalid version or default profile")
	}
	if len(config.Profiles) != 3 {
		return errors.New("depth policy contains an unknown profile")
	}
	for _, profile := range []DepthProfile{ProfileShallow, ProfileBalanced, ProfileLoopy} {
		rules, ok := config.Profiles[profile]
		if !ok || !validDepthLevel(rules.PracticeNeedDepth) || !validDepthLevel(rules.UncertaintyDepth) {
			return errors.New("depth policy is missing a valid profile")
		}
	}
	return nil
}

func (config DepthPolicyConfig) Digest() string {
	body, err := json.Marshal(config)
	if err != nil {
		return ""
	}
	return SHA256Hex(body)
}

func validDepthProfile(profile DepthProfile) bool {
	return profile == ProfileShallow || profile == ProfileBalanced || profile == ProfileLoopy
}

func validDepthLevel(depth DepthLevel) bool {
	return depth == DepthShallow || depth == DepthBalanced || depth == DepthLoopy
}

func depthRank(depth DepthLevel) int {
	switch depth {
	case DepthLoopy:
		return 3
	case DepthBalanced:
		return 2
	default:
		return 1
	}
}

func maxDepth(left, right DepthLevel) DepthLevel {
	if depthRank(right) > depthRank(left) {
		return right
	}
	return left
}

func profileFromPosture(posture Posture) (DepthProfile, error) {
	switch posture {
	case Direct:
		return ProfileShallow, nil
	case Balanced:
		return ProfileBalanced, nil
	case Deliberative:
		return ProfileLoopy, nil
	default:
		return "", errors.New("legacy posture is unsupported")
	}
}

func postureFromProfile(profile DepthProfile) Posture {
	switch profile {
	case ProfileShallow:
		return Direct
	case ProfileLoopy:
		return Deliberative
	default:
		return Balanced
	}
}

func routeForDepth(depth DepthLevel) Route {
	switch depth {
	case DepthLoopy:
		return D2Governed
	case DepthBalanced:
		return D1Targeted
	default:
		return D0Direct
	}
}

func depthForRoute(route Route) DepthLevel {
	switch route {
	case D2Governed:
		return DepthLoopy
	case D1Targeted:
		return DepthBalanced
	default:
		return DepthShallow
	}
}
