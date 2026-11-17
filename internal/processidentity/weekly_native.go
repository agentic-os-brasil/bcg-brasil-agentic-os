package processidentity

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"time"
)

const (
	WeeklyNativeDarwinStandalone  = "darwin_signed_standalone_native"
	WeeklyNativeWindowsStandalone = "windows_signed_standalone_native"
)

// WeeklyNativeDeclaration is accepted metadata only, never observed facts or authority.
type WeeklyNativeDeclaration struct {
	Topology                string `json:"topology"`
	TeamID                  string `json:"team_id,omitempty"`
	SigningIdentifier       string `json:"signing_identifier,omitempty"`
	CodeDirectoryHash       string `json:"code_directory_hash,omitempty"`
	Architecture            string `json:"architecture"`
	SignatureMode           string `json:"signature_mode,omitempty"`
	SignerCertificateSHA256 string `json:"signer_certificate_sha256,omitempty"`
	RevocationPolicy        string `json:"revocation_policy,omitempty"`
}

var weeklyTeam = regexp.MustCompile(`^[A-Z0-9]{10}$`)
var weeklySigningID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,255}$`)
var weeklyCDHash = regexp.MustCompile(`^[a-f0-9]{40}$`)
var weeklySignerHash = regexp.MustCompile(`^[a-f0-9]{64}$`)

func (d WeeklyNativeDeclaration) Validate() error {
	if d.Architecture != "arm64" && d.Architecture != "amd64" {
		return ErrUnavailable
	}
	switch d.Topology {
	case WeeklyNativeDarwinStandalone:
		if !weeklyTeam.MatchString(d.TeamID) || !weeklySigningID.MatchString(d.SigningIdentifier) || !weeklyCDHash.MatchString(d.CodeDirectoryHash) || d.SignatureMode != "" || d.SignerCertificateSHA256 != "" || d.RevocationPolicy != "" {
			return errors.New("weekly Darwin native declaration is unsupported")
		}
	case WeeklyNativeWindowsStandalone:
		if d.TeamID != "" || d.SigningIdentifier != "" || d.CodeDirectoryHash != "" || d.SignatureMode != "embedded_primary_index0" || !weeklySignerHash.MatchString(d.SignerCertificateSHA256) || d.RevocationPolicy != "cache_only_chain" {
			return errors.New("weekly Windows native declaration is unsupported")
		}
	default:
		return ErrUnavailable
	}
	return nil
}

// WeeklyNativeChild is an opaque OS observation. Only a separately owned live
// attempt can associate it with actual Start/terminal events; it grants nothing.
type WeeklyNativeChild struct {
	valid                                  bool
	platform                               string
	pid, parent                            int
	realUID, effectiveUID                  uint32
	startSeconds                           int64
	startMicroseconds                      int32
	creationTime                           uint64
	processHandle, threadHandle, jobHandle uintptr
	threadID                               uint32
	imagePath                              string
}

func (o WeeklyNativeChild) PID() int {
	if !o.valid {
		return 0
	}
	return o.pid
}
func (o WeeklyNativeChild) ImagePath() string {
	if !o.valid {
		return ""
	}
	return o.imagePath
}
func (o WeeklyNativeChild) StartedAt() time.Time {
	if !o.valid {
		return time.Time{}
	}
	if o.platform == "windows" {
		return time.Unix(int64(o.creationTime/10000000)-11644473600, int64(o.creationTime%10000000)*100).UTC()
	}
	return time.Unix(o.startSeconds, int64(o.startMicroseconds)*1000).UTC()
}
func (o WeeklyNativeChild) Same(other WeeklyNativeChild) bool {
	return o.valid && other.valid && o == other
}
func (o WeeklyNativeChild) Digest() string {
	if !o.valid {
		return ""
	}
	body, _ := json.Marshal([]any{o.platform, o.pid, o.parent, o.realUID, o.effectiveUID, o.startSeconds, o.startMicroseconds, o.creationTime, o.processHandle, o.threadHandle, o.jobHandle, o.threadID, o.imagePath})
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}
