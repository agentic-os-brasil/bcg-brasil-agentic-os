package agentidentity

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

const identityDraftSchemaVersion = 1

type GuidedQuestion struct {
	Kind         string `json:"kind"`
	Role         string `json:"role"`
	QuestionID   string `json:"question_id"`
	Version      int    `json:"version"`
	Question     string `json:"question"`
	AudioPrompt  string `json:"audio_prompt"`
	Instructions string `json:"instructions"`
}

type GuidedInterview struct {
	State        string          `json:"state"`
	NextQuestion *GuidedQuestion `json:"next_question,omitempty"`
	OpenDraftID  string          `json:"open_draft_id,omitempty"`
	Guidance     string          `json:"guidance,omitempty"`
	Catalog      Interview       `json:"catalog"`
}

type ProfileDraft struct {
	SchemaVersion   int       `json:"schema_version"`
	ID              string    `json:"id"`
	Kind            string    `json:"kind"`
	QuestionVersion int       `json:"question_version"`
	Profile         Profile   `json:"profile"`
	BaseSHA256      string    `json:"base_sha256"`
	Consent         bool      `json:"consent"`
	NoClientData    bool      `json:"no_client_data"`
	ReviewDigest    string    `json:"review_digest"`
	State           string    `json:"state"`
	CreatedAt       time.Time `json:"created_at"`
	AppliedAt       time.Time `json:"applied_at,omitempty"`
}

var identityWriteJSON = writeIdentityJSON
var identityCompactDrafts = compactIdentityDrafts

func GuidedIdentityInterview(root string) GuidedInterview {
	catalog := InitialInterview()
	// The catalog keeps role, narrative and track choices, but only
	// NextQuestion is an actionable question in this response.
	catalog.Steps = nil
	openDraftID, err := openIdentityDraftID(root)
	if err != nil {
		return GuidedInterview{State: "blocked", Guidance: "agent identity draft state is invalid; review local state before continuing", Catalog: catalog}
	}
	if openDraftID != "" {
		return GuidedInterview{State: "review_required", OpenDraftID: openDraftID, Guidance: "review or confirm the open agent identity draft before asking another question", Catalog: catalog}
	}
	configured := map[string]bool{}
	state, err := ReadProfile(root)
	if err != nil {
		return GuidedInterview{State: "blocked", Guidance: "agent identity profile is invalid; preserve it and inspect the admitted state", Catalog: catalog}
	}
	if state.Profile != nil {
		for _, selection := range state.Profile.Selections {
			configured[CanonicalRole(selection.Role)] = true
		}
	}
	questions := []GuidedQuestion{
		{Kind: "managed_agent_identity", Role: "maestro", QuestionID: "identity-maestro", Version: 1, Question: "Como você quer chamar o agente que fala com você e rege o trabalho profissional — e qual emoji deve representá-lo?", AudioPrompt: "Qual nome e emoji você quer para o agente principal?"},
		{Kind: "managed_agent_identity", Role: "yoda", QuestionID: "identity-yoda", Version: 1, Question: "Como você quer chamar o revisor interno que faz o pressure-test antes de algo importante chegar a você — e qual emoji combina com esse papel?", AudioPrompt: "Qual nome e emoji você quer para o revisor interno?"},
		{Kind: "managed_agent_identity", Role: "darwin", QuestionID: "identity-darwin", Version: 1, Question: "Como você quer chamar o agente que observa saúde, drift e evolução do sistema — e qual emoji deve representá-lo?", AudioPrompt: "Qual nome e emoji você quer para o agente de evolução do sistema?"},
	}
	for index := range questions {
		if !configured[questions[index].Role] {
			questions[index].Instructions = "Faça somente esta pergunta. Use apenas preferências declaradas explicitamente. Mostre o profile draft completo antes de pedir confirmação; nome e emoji nunca mudam autoridade."
			return GuidedInterview{State: "action_required", NextQuestion: &questions[index], Catalog: catalog}
		}
	}
	return GuidedInterview{State: "complete", Catalog: catalog}
}

func DraftProfile(root string, profile Profile, consent, noClientData bool) (ProfileDraft, error) {
	if !consent || !noClientData {
		return ProfileDraft{}, errors.New("explicit owner consent and owner no-client-data attestation are required")
	}
	if state := GuidedIdentityInterview(root); state.State == "blocked" {
		return ProfileDraft{}, ErrIdentityState
	}
	var result ProfileDraft
	err := withIdentityTransition(root, func() (err error) { result, err = draftProfileLocked(root, profile, consent, noClientData); return err })
	return result, err
}

func draftProfileLocked(root string, profile Profile, consent, noClientData bool) (ProfileDraft, error) {
	if !consent || !noClientData {
		return ProfileDraft{}, errors.New("explicit owner consent and owner no-client-data attestation are required")
	}
	if err := ensureNoOpenIdentityDraft(root); err != nil {
		return ProfileDraft{}, err
	}
	profile.UpdatedAt = time.Now().UTC()
	for index := range profile.Selections {
		profile.Selections[index].Role = CanonicalRole(profile.Selections[index].Role)
	}
	SortSelections(&profile)
	sort.Strings(profile.CapabilityTracks)
	if err := profile.Validate(); err != nil {
		return ProfileDraft{}, err
	}
	guided := GuidedIdentityInterview(root)
	if guided.State == "blocked" {
		return ProfileDraft{}, ErrIdentityState
	}
	if guided.NextQuestion != nil {
		current := Profile{}
		if loaded, err := Load(root); err == nil {
			current = loaded
		}
		expected := guided.NextQuestion.Role
		if _, ok := profileSelection(profile, expected); !ok {
			return ProfileDraft{}, errors.New("agent identity draft is off-sequence: answer only the current next_question")
		}
		for _, role := range []string{"maestro", "yoda", "darwin"} {
			before, existed := profileSelection(current, role)
			after, proposed := profileSelection(profile, role)
			if role != expected && !existed && proposed {
				return ProfileDraft{}, errors.New("agent identity draft batches a future main-agent question")
			}
			if role != expected && existed && (!proposed || before != after) {
				return ProfileDraft{}, errors.New("agent identity draft changes a main-agent answer outside the current question")
			}
		}
	}
	base, err := profileBaseSHA(root)
	if err != nil {
		return ProfileDraft{}, err
	}
	d := ProfileDraft{SchemaVersion: identityDraftSchemaVersion, Kind: "agent_identity_personalization", QuestionVersion: 1, Profile: profile, BaseSHA256: base, Consent: true, NoClientData: true, State: "drafted", CreatedAt: time.Now().UTC()}
	d.ReviewDigest = profileDraftDigest(d)
	d.ID = "identity-draft-" + d.ReviewDigest[:24]
	if err := identityWriteJSON(filepath.Join(root, "agents", "interview", "drafts", d.ID+".json"), d); err != nil {
		return ProfileDraft{}, err
	}
	return d, nil
}

func profileSelection(profile Profile, role string) (Selection, bool) {
	role = CanonicalRole(role)
	managedID := managedAgentID(role)
	for _, selection := range profile.Selections {
		if CanonicalRole(selection.Role) == role && (selection.AgentID == "" || managedID != "" && selection.AgentID == managedID) {
			selection.Role = CanonicalRole(selection.Role)
			return selection, true
		}
	}
	return Selection{}, false
}

func managedAgentID(role string) string {
	role = CanonicalRole(role)
	for _, target := range managedTargets {
		if target.Role == role {
			return target.AgentID
		}
	}
	return ""
}

func ReviewProfileDraft(root, id string) (ProfileDraft, error) {
	if !validDraftID(id) {
		return ProfileDraft{}, errors.New("invalid agent identity draft id")
	}
	return readProfileDraft(filepath.Join(root, "agents", "interview", "drafts", id+".json"))
}

func ConfirmProfileDraft(root, id, reviewDigest string, confirmed bool) (ProfileDraft, error) {
	if !confirmed || !validDraftID(id) {
		return ProfileDraft{}, errors.New("exact agent identity confirmation is required")
	}
	var result ProfileDraft
	err := withIdentityTransition(root, func() (err error) {
		result, err = confirmProfileDraftLocked(root, id, reviewDigest, confirmed)
		return err
	})
	return result, err
}

func confirmProfileDraftLocked(root, id, reviewDigest string, confirmed bool) (ProfileDraft, error) {
	if !confirmed {
		return ProfileDraft{}, errors.New("explicit owner confirmation is required")
	}
	d, err := ReviewProfileDraft(root, id)
	if err != nil {
		return ProfileDraft{}, err
	}
	if (d.State != "drafted" && d.State != "prepared" && d.State != "applied") || !validProfileDraft(d) || !digestEqual(reviewDigest, d.ReviewDigest) {
		return ProfileDraft{}, errors.New("agent identity confirmation denied because the reviewed envelope is invalid or closed")
	}
	base, err := profileBaseSHA(root)
	if err != nil {
		return ProfileDraft{}, err
	}
	expected, err := storedProfileSHA(d.Profile)
	if err != nil {
		return ProfileDraft{}, err
	}
	if d.State == "applied" {
		if base != expected {
			return ProfileDraft{}, ErrIdentityChanged
		}
		return d, nil
	}
	if d.State == "drafted" {
		if base != d.BaseSHA256 {
			return ProfileDraft{}, errors.New("agent identity profile changed since this draft; refusing to overwrite newer content")
		}
		d.State = "prepared"
		if err := identityWriteJSON(filepath.Join(root, "agents", "interview", "drafts", d.ID+".json"), d); err != nil {
			return ProfileDraft{}, err
		}
	} else if base != d.BaseSHA256 && base != expected {
		return ProfileDraft{}, errors.New("agent identity profile changed during prepared recovery; refusing to overwrite newer content")
	}
	if base != expected {
		if err := saveProfileLocked(root, d.Profile); err != nil {
			return ProfileDraft{}, err
		}
	}
	if err := identityCompactDrafts(root, d.ID); err != nil {
		return ProfileDraft{}, err
	}
	d.State, d.AppliedAt = "applied", time.Now().UTC()
	if err := identityWriteJSON(filepath.Join(root, "agents", "interview", "drafts", d.ID+".json"), d); err != nil {
		return ProfileDraft{}, err
	}
	return d, nil
}

func profileBaseSHA(root string) (string, error) {
	state, err := ReadProfile(root)
	return state.Revision, err
}
func profileDraftDigest(d ProfileDraft) string {
	envelope := struct {
		SchemaVersion   int     `json:"schema_version"`
		Kind            string  `json:"kind"`
		QuestionVersion int     `json:"question_version"`
		Profile         Profile `json:"profile"`
		BaseSHA256      string  `json:"base_sha256"`
		Consent         bool    `json:"consent"`
		NoClientData    bool    `json:"no_client_data"`
	}{d.SchemaVersion, d.Kind, d.QuestionVersion, d.Profile, d.BaseSHA256, d.Consent, d.NoClientData}
	body, _ := json.Marshal(envelope)
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}
func validProfileDraft(d ProfileDraft) bool {
	validState := (d.State == "drafted" || d.State == "prepared") && d.AppliedAt.IsZero() || d.State == "applied" && !d.AppliedAt.IsZero()
	validID := d.ReviewDigest != "" && d.ID == "identity-draft-"+d.ReviewDigest[:min(24, len(d.ReviewDigest))]
	return d.SchemaVersion == identityDraftSchemaVersion && d.Kind == "agent_identity_personalization" && d.QuestionVersion == 1 && d.Consent && d.NoClientData && !d.CreatedAt.IsZero() && validState && validID && d.BaseSHA256 != "" && digestEqual(d.ReviewDigest, profileDraftDigest(d)) && d.Profile.Validate() == nil
}
func digestEqual(a, b string) bool {
	if len(a) != 64 || len(b) != 64 {
		return false
	}
	if _, err := hex.DecodeString(a); err != nil {
		return false
	}
	if _, err := hex.DecodeString(b); err != nil {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(strings.ToLower(a)), []byte(strings.ToLower(b))) == 1
}
func readProfileDraft(path string) (ProfileDraft, error) {
	body, err := scheduler.ReadPrivateStateFile(path, maximumIdentityBytes)
	if err != nil {
		return ProfileDraft{}, err
	}
	if !utf8.Valid(body) {
		return ProfileDraft{}, ErrIdentityState
	}
	var d ProfileDraft
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&d); err != nil || !validProfileDraft(d) {
		return ProfileDraft{}, errors.New("agent identity draft is invalid")
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return ProfileDraft{}, errors.New("agent identity draft contains trailing JSON")
	}
	if filepath.Base(path) != d.ID+".json" {
		return ProfileDraft{}, errors.New("agent identity draft path does not match its digest-bound ID")
	}
	return d, nil
}
func storedProfileSHA(profile Profile) (string, error) {
	profile.UpdatedAt = profile.UpdatedAt.UTC()
	body, err := json.MarshalIndent(profile, "", "  ")
	if err != nil {
		return "", err
	}
	body = append(body, '\n')
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:]), nil
}
func ensureNoOpenIdentityDraft(root string) error {
	id, err := openIdentityDraftID(root)
	if err != nil {
		return err
	}
	if id != "" {
		return errors.New("review or confirm the open agent identity draft before creating another")
	}
	return nil
}

func openIdentityDraftID(root string) (string, error) {
	paths, err := identityDraftPaths(root)
	if err != nil {
		return "", err
	}
	openID := ""
	for _, path := range paths {
		draft, err := readProfileDraft(path)
		if err != nil {
			return "", err
		}
		if draft.State == "drafted" || draft.State == "prepared" {
			if openID != "" {
				return "", errors.New("multiple open agent identity drafts violate the single-review boundary")
			}
			openID = draft.ID
		}
	}
	return openID, nil
}
func compactIdentityDrafts(root, currentID string) error {
	paths, err := identityDraftPaths(root)
	if err != nil {
		return err
	}
	for _, path := range paths {
		if filepath.Base(path) == currentID+".json" {
			continue
		}
		draft, err := readProfileDraft(path)
		if err != nil {
			return err
		}
		if draft.State == "applied" {
			if err := removeIdentityEntry(path); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
			if err := removeIdentityEntry(path + ".lock"); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
		}
	}
	return nil
}
func writeIdentityJSON(path string, value any) error {
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	if len(body) > maximumIdentityBytes {
		return errors.New("agent identity state exceeds its bounded record size")
	}
	return scheduler.UpdatePrivateFile(path, maximumIdentityBytes, func([]byte) ([]byte, error) { return body, nil })
}
