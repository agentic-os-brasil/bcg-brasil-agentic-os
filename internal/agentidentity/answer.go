package agentidentity

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

// Answer contains presentation choices and optimistic concurrency, never
// caller-supplied consent, authority, scope or profile mutation flags.
type Answer struct {
	OwnerID          string `json:"owner_id"`
	Role             string `json:"role"`
	AgentID          string `json:"agent_id,omitempty"`
	DisplayName      string `json:"display_name"`
	Emoji            string `json:"emoji"`
	QuestionID       string `json:"question_id"`
	ExpectedRevision string `json:"expected_profile_revision"`
}

type ProfileChoices struct {
	OwnerID          string      `json:"owner_id"`
	Selections       []Selection `json:"selections"`
	CapabilityTracks []string    `json:"capability_tracks,omitempty"`
}

type AnswerReview struct {
	Scope    string
	Answer   Answer
	Current  ProfileState
	Proposed ProfileChoices
	Digest   string
}

type DraftReview struct {
	Scope   string
	Current ProfileState
	Draft   ProfileDraft
	Digest  string
}

func identityReviewDigest(value any) string {
	body, _ := json.Marshal(value)
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

// PrepareAnswer is a deterministic noncreating projection. Timestamps and
// confirmation metadata are assigned only after the native review succeeds.
func PrepareAnswer(root string, answer Answer) (AnswerReview, error) {
	canonical, err := scheduler.CanonicalPrivatePath(root)
	if err != nil {
		return AnswerReview{}, err
	}
	current, err := ReadProfile(canonical)
	if err != nil {
		return AnswerReview{}, err
	}
	if current.Revision != answer.ExpectedRevision {
		return AnswerReview{}, ErrIdentityChanged
	}
	guided := GuidedIdentityInterview(canonical)
	if guided.State == "blocked" {
		return AnswerReview{}, ErrIdentityState
	}
	if guided.OpenDraftID != "" {
		return AnswerReview{}, ErrIdentityDraftOpen
	}
	role := CanonicalRole(answer.Role)
	descriptor, ok := Default(role)
	if !ok || answer.QuestionID != "identity-"+role {
		return AnswerReview{}, errors.New("invalid identity question or role")
	}
	if guided.NextQuestion != nil && guided.NextQuestion.Role != role {
		return AnswerReview{}, errors.New("answer only the current identity question")
	}
	if managed := managedAgentID(role); managed != "" && answer.AgentID != "" && answer.AgentID != managed {
		return AnswerReview{}, errors.New("identity managed agent id does not match its role")
	}
	selection := Selection{Role: role, AgentID: answer.AgentID, DisplayName: answer.DisplayName, Emoji: answer.Emoji, OwnerID: answer.OwnerID, OwnershipScope: descriptor.OwnershipScope}
	if err := ValidateSelection(selection); err != nil {
		return AnswerReview{}, err
	}
	if (role == "client_account_agent" || role == "case_agent") && selection.AgentID == "" {
		return AnswerReview{}, errors.New("instance identity requires an agent id")
	}
	proposed := ProfileChoices{OwnerID: answer.OwnerID, Selections: []Selection{}}
	if current.Profile != nil {
		if current.Profile.OwnerID != answer.OwnerID {
			return AnswerReview{}, errors.New("identity owner cannot change through presentation customization")
		}
		proposed.Selections = append(proposed.Selections, current.Profile.Selections...)
		proposed.CapabilityTracks = append([]string(nil), current.Profile.CapabilityTracks...)
	}
	replaced := false
	for i, existing := range proposed.Selections {
		if CanonicalRole(existing.Role) == role && (existing.AgentID == selection.AgentID || managedAgentID(role) != "") {
			if replaced {
				return AnswerReview{}, ErrIdentityState
			}
			proposed.Selections[i] = selection
			replaced = true
		}
	}
	if !replaced {
		proposed.Selections = append(proposed.Selections, selection)
	}
	// The sentinel is validation-only, never a persisted timestamp or authority.
	validation := proposed.profile(time.Unix(1, 0))
	if err := validation.Validate(); err != nil {
		return AnswerReview{}, err
	}
	normalized := normalizedProfile(validation)
	proposed.Selections = normalized.Selections
	fresh, err := ReadProfile(canonical)
	if err != nil {
		return AnswerReview{}, err
	}
	if fresh.Revision != current.Revision {
		return AnswerReview{}, ErrIdentityChanged
	}
	review := AnswerReview{Scope: canonical, Answer: answer, Current: current, Proposed: proposed}
	review.Digest = identityReviewDigest(review)
	return review, nil
}

func (choices ProfileChoices) profile(now time.Time) Profile {
	return Profile{SchemaVersion: SchemaVersion, OwnerID: choices.OwnerID, Confirmed: true, UpdatedAt: now.UTC(), Selections: append([]Selection(nil), choices.Selections...), CapabilityTracks: append([]string(nil), choices.CapabilityTracks...)}
}

// CommitAnswer is called only after exact attended approval. No lock is held
// while asking a human. All cooperating canonical identity writers share this
// transition; the complete question/base/proposal is reconstructed under it.
func CommitAnswer(root string, answer Answer, reviewedDigest string, apply bool, beforeMutation func() error) (ProfileDraft, error) {
	var result ProfileDraft
	err := withIdentityTransition(root, func() error {
		if beforeMutation != nil {
			if err := beforeMutation(); err != nil {
				return err
			}
		}
		review, err := PrepareAnswer(root, answer)
		if err != nil {
			return err
		}
		if !digestEqual(reviewedDigest, review.Digest) {
			return ErrIdentityChanged
		}
		result, err = draftProfileLocked(root, review.Proposed.profile(time.Now()), true, true)
		if err != nil {
			return err
		}
		if !apply {
			return nil
		}
		if beforeMutation != nil {
			if err := beforeMutation(); err != nil {
				return err
			}
		}
		applied, err := confirmProfileDraftLocked(root, result.ID, result.ReviewDigest, true)
		if err == nil {
			result = applied
		} else if pending, readErr := ReviewProfileDraft(root, result.ID); readErr == nil {
			result = pending
		}
		return err
	})
	return result, err
}

func PrepareDraftReview(root, id string) (DraftReview, error) {
	canonical, err := scheduler.CanonicalPrivatePath(root)
	if err != nil {
		return DraftReview{}, err
	}
	current, err := ReadProfile(canonical)
	if err != nil {
		return DraftReview{}, err
	}
	draft, err := ReviewProfileDraft(canonical, id)
	if err != nil {
		return DraftReview{}, err
	}
	expected, err := storedProfileSHA(draft.Profile)
	if err != nil {
		return DraftReview{}, err
	}
	if (draft.State == "drafted" && current.Revision != draft.BaseSHA256) || (draft.State == "prepared" && current.Revision != draft.BaseSHA256 && current.Revision != expected) || (draft.State == "applied" && current.Revision != expected) {
		return DraftReview{}, ErrIdentityChanged
	}
	review := DraftReview{Scope: canonical, Current: current, Draft: draft}
	review.Digest = identityReviewDigest(review)
	return review, nil
}

func CommitDraftReview(root, id, reviewedDigest string, beforeMutation func() error) (ProfileDraft, error) {
	var result ProfileDraft
	err := withIdentityTransition(root, func() error {
		if beforeMutation != nil {
			if err := beforeMutation(); err != nil {
				return err
			}
		}
		review, err := PrepareDraftReview(root, id)
		if err != nil {
			return err
		}
		if !digestEqual(reviewedDigest, review.Digest) {
			return ErrIdentityChanged
		}
		result = review.Draft
		applied, err := confirmProfileDraftLocked(root, id, review.Draft.ReviewDigest, true)
		if err == nil {
			result = applied
		} else if pending, readErr := ReviewProfileDraft(root, id); readErr == nil {
			result = pending
		}
		return err
	})
	return result, err
}
