// Package distribution delivers only the synthetic Maestro team experiment.
// Its independent pinned-key contract grants no production release authority.
package distribution

import "crypto/ed25519"

const Channel = "maestro-synthetic-team-pilot"

type Artifact struct {
	Name   string `json:"name"`
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

type Manifest struct {
	SchemaVersion int        `json:"schema_version"`
	Channel       string     `json:"channel"`
	Version       string     `json:"version"`
	Target        string     `json:"target"`
	Sequence      uint64     `json:"sequence"`
	Artifacts     []Artifact `json:"artifacts"`
}

type ReleaseInput struct {
	FeedRoot, Version, Target, Server, Installer string
	Sequence                                     uint64
	PrivateKey                                   ed25519.PrivateKey
}

type InstallInput struct {
	FeedRoot, Home, Target string
	PublicKey              ed25519.PublicKey
}

type Receipt struct {
	Status      string            `json:"status"`
	Version     string            `json:"version"`
	Target      string            `json:"target"`
	Home        string            `json:"home"`
	Marketplace string            `json:"marketplace"`
	Extension   string            `json:"extension"`
	Installer   string            `json:"installer"`
	StateRoot   string            `json:"state_root"`
	Sequence    uint64            `json:"sequence"`
	Changed     bool              `json:"changed"`
	FeedRoot    string            `json:"feed_root"`
	FeedSHA256  string            `json:"feed_sha256"`
	Files       map[string]string `json:"files"`
}
