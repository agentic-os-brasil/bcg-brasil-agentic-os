package releasecontract

import (
	"errors"
	"fmt"
	"time"
)

const SyncedSourceContract = "maestro-synced-source-v1"
const GenerationProtocol = "maestro-synced-generation-v1"
const MaximumGenerationTTL = 30 * 24 * time.Hour
const MaximumSourceRecalls = 256

// MaximumSyncedMetadataBytes bounds exact signed input, including whitespace,
// before it can be admitted into the installation owner's bounded custody.
const MaximumSyncedMetadataBytes = 64 << 10

type SourceTuple struct {
	SourceID string `json:"source_id"`
	Channel  string `json:"channel"`
	OS       string `json:"os"`
	Arch     string `json:"arch"`
}

func (s SourceTuple) Validate() error {
	if !identifierPattern.MatchString(s.SourceID) || (s.Channel != "stable" && s.Channel != "beta" && s.Channel != "canary") {
		return errors.New("source identity/channel is invalid")
	}
	if (s.OS != "darwin" && s.OS != "windows" && s.OS != "linux") || (s.Arch != "amd64" && s.Arch != "arm64") {
		return errors.New("source requires one exact supported target")
	}
	return nil
}

type ReleaseRecall struct {
	Release        string `json:"release"`
	ManifestSHA256 string `json:"manifest_sha256"`
}

func (r ReleaseRecall) Validate() error {
	if _, err := parseVersion(r.Release); err != nil {
		return err
	}
	if !hashPattern.MatchString(r.ManifestSHA256) {
		return errors.New("recall requires exact manifest identity")
	}
	return nil
}

type Generation struct {
	SchemaVersion         int             `json:"schema_version"`
	Product               string          `json:"product"`
	Protocol              string          `json:"protocol"`
	Issuer                Issuer          `json:"issuer"`
	Source                SourceTuple     `json:"source"`
	Sequence              uint64          `json:"sequence"`
	PublishedAt           string          `json:"published_at"`
	ExpiresAt             string          `json:"expires_at"`
	Release               string          `json:"release"`
	ManifestSHA256        string          `json:"manifest_sha256"`
	ClientInputsSHA256    string          `json:"client_inputs_sha256"`
	Recalls               []ReleaseRecall `json:"recalls"`
	MinimumReaderContract string          `json:"minimum_reader_contract"`
}

// Validate authenticates no bytes and intentionally does not check freshness.
// Expired authentic higher metadata remains meaningful denial information.
func (g Generation) Validate() error {
	if g.SchemaVersion != 1 || g.Product != "maestro" || g.Protocol != GenerationProtocol || g.Sequence == 0 || g.MinimumReaderContract != SyncedSourceContract {
		return errors.New("unsupported synced generation contract")
	}
	if err := g.Source.Validate(); err != nil {
		return err
	}
	if !identifierPattern.MatchString(g.Issuer.ID) || !identifierPattern.MatchString(g.Issuer.KeyID) {
		return errors.New("generation authority identity invalid")
	}
	if _, err := parseVersion(g.Release); err != nil {
		return err
	}
	if !hashPattern.MatchString(g.ManifestSHA256) || !hashPattern.MatchString(g.ClientInputsSHA256) {
		return errors.New("generation release input identity invalid")
	}
	published, err := generationTime(g.PublishedAt)
	if err != nil {
		return err
	}
	expires, err := generationTime(g.ExpiresAt)
	if err != nil {
		return err
	}
	if !published.Before(expires) || expires.Sub(published) > MaximumGenerationTTL {
		return errors.New("generation expiry must be positive and within 30 days")
	}
	if g.Recalls == nil || len(g.Recalls) > MaximumSourceRecalls {
		return errors.New("generation recalls must be a bounded explicit array")
	}
	seen := map[ReleaseRecall]bool{}
	for _, r := range g.Recalls {
		if err := r.Validate(); err != nil {
			return err
		}
		if seen[r] {
			return errors.New("duplicate generation recall")
		}
		seen[r] = true
	}
	return nil
}

func generationTime(value string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339, value)
	if err != nil || t.UTC().Format(time.RFC3339) != value {
		return time.Time{}, errors.New("generation timestamps require canonical second-precision UTC")
	}
	return t, nil
}

// CheckTime is called after confirmed custody observation, never as a substitute
// for authenticating and retaining a higher generation's denial information.
func (g Generation) CheckTime(now, lastObserved time.Time) error {
	if err := g.Validate(); err != nil {
		return err
	}
	if now.IsZero() || (!lastObserved.IsZero() && now.Before(lastObserved)) {
		return errors.New("trusted source clock moved backwards")
	}
	published, _ := generationTime(g.PublishedAt)
	expires, _ := generationTime(g.ExpiresAt)
	if published.After(now.Add(5 * time.Minute)) {
		return errors.New("source publication is implausibly future dated")
	}
	if !now.Before(expires) {
		return errors.New("signed source generation expired")
	}
	return nil
}

func (g Generation) GenerationDirectory(bodySHA256 string) string {
	return fmt.Sprintf("%d-%s", g.Sequence, bodySHA256)
}

func ParseGeneration(body []byte) (Generation, error) {
	var g Generation
	if len(body) > MaximumSyncedMetadataBytes {
		return g, fmt.Errorf("signed source generation exceeds %d exact bytes", MaximumSyncedMetadataBytes)
	}
	if err := DecodeStrictJSON(body, &g); err != nil {
		return g, err
	}
	return g, g.Validate()
}
