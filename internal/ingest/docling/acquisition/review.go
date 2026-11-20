// Package acquisition prepares explicitly reviewed engineering runtime inputs.
// It is separate from the offline converter and never runs acquired code.
package acquisition

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ingest/docling"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

const RecipeName = "runtime-acquisition.json"
const RecipeSignatureName = RecipeName + ".sig"
const Purpose = "maestro-engineering-docling-acquisition-v1"

// Options must come from the native carrier setup. RegistrySHA256 is its
// independently pinned authority digest, never a model-provided trust assertion.
// ServerPath selects the exact already-prepared server; this package hashes its
// signed bytes and never executes that server.
type Options struct {
	ReleaseDirectory, RecipeDirectory, ServerPath string
	RegistryPath, RegistrySHA256, CacheRoot       string
}

type Recipe struct {
	SchemaVersion    int                      `json:"schema_version"`
	Purpose          string                   `json:"purpose"`
	Issuer           releasecontract.Issuer   `json:"issuer"`
	ManifestSHA256   string                   `json:"manifest_sha256"`
	ServerSHA256     string                   `json:"server_sha256"`
	OS               string                   `json:"os"`
	Arch             string                   `json:"arch"`
	Profile          string                   `json:"profile"`
	DescriptorSHA256 string                   `json:"descriptor_sha256"`
	InputCount       int                      `json:"input_count"`
	TotalInputBytes  int64                    `json:"total_input_bytes"`
	RuntimeArtifact  releasecontract.Artifact `json:"runtime_artifact"`
	IssuedAt         string                   `json:"issued_at"`
	ExpiresAt        string                   `json:"expires_at"`
}

type InputState struct {
	docling.Input
	Cached bool `json:"cached"`
}

type Plan struct {
	SHA256                 string       `json:"sha256"`
	ReviewedAt             string       `json:"reviewed_at"`
	ValidUntil             string       `json:"valid_until"`
	Recipe                 Recipe       `json:"recipe"`
	Inputs                 []InputState `json:"inputs"`
	Attempt                int          `json:"attempt"`
	MaximumAdditionalBytes int64        `json:"maximum_additional_bytes"`
	RetainedBytes          int64        `json:"retained_bytes"`
}

// Review authenticates public metadata before observing private cache state.
// It never creates files, contacts an upstream host or executes a runtime.
func Review(ctx context.Context, o Options) (Plan, error) {
	now := time.Now().UTC().Truncate(time.Second)
	return reviewAt(ctx, o, now, now)
}

func digest(body []byte) string { sum := sha256.Sum256(body); return hex.EncodeToString(sum[:]) }

func readBounded(name string, maximum int64) ([]byte, error) {
	file, err := scheduler.OpenRegularFile(name)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	before, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !before.Mode().IsRegular() || before.Size() < 0 || before.Size() > maximum || scheduler.ValidateSingleLink(file) != nil {
		return nil, errors.New("unsafe_acquisition_metadata")
	}
	body, err := io.ReadAll(io.LimitReader(file, maximum+1))
	if err != nil {
		return nil, err
	}
	after, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !os.SameFile(before, after) || before.Size() != int64(len(body)) || after.Size() != before.Size() || !before.ModTime().Equal(after.ModTime()) || scheduler.ValidateSingleLink(file) != nil {
		return nil, errors.New("changed_acquisition_metadata")
	}
	return body, nil
}
