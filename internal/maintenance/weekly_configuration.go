package maintenance

import (
	"errors"
	"path/filepath"
	"runtime"
	"strings"
	"unicode/utf8"
)

type WeeklyConfigurationEntry struct {
	Path   string `json:"path"`
	Role   string `json:"role"`
	SHA256 string `json:"sha256,omitempty"`
	Absent bool   `json:"absent,omitempty"`
}

// WeeklyConfiguration declares only explicitly accepted nonsecret files or
// absences. It does not discover or attest the runtime's complete settings sources.
type WeeklyConfiguration []WeeklyConfigurationEntry

func (c WeeklyConfiguration) Validate() error {
	if len(c) < 2 || len(c) > 32 {
		return errors.New("weekly configuration inventory bounds invalid")
	}
	roles := map[string]bool{}
	paths := map[string]bool{}
	previous := ""
	for _, entry := range c {
		key := entry.Path
		if runtime.GOOS == "windows" {
			key = strings.ToLower(key)
		}
		if entry.Path == "" || len(entry.Path) > 32768*4 || !utf8.ValidString(entry.Path) || strings.ContainsAny(entry.Path, "\x00\r\n") || !filepath.IsAbs(entry.Path) || filepath.Clean(entry.Path) != entry.Path || previous != "" && entry.Path <= previous || paths[key] {
			return errors.New("weekly configuration paths must be unique sorted absolute paths")
		}
		if entry.Role != "settings" && entry.Role != "managed_policy" {
			return errors.New("weekly configuration role unsupported")
		}
		if entry.Absent && entry.SHA256 != "" || !entry.Absent && !digestPattern.MatchString(entry.SHA256) {
			return errors.New("weekly configuration file or absence digest invalid")
		}
		roles[entry.Role] = true
		paths[key] = true
		previous = entry.Path
	}
	if !roles["settings"] || !roles["managed_policy"] {
		return errors.New("weekly configuration must declare settings and managed policy")
	}
	return nil
}
func (c WeeklyConfiguration) Digest(role string) string {
	if c.Validate() != nil || (role != "settings" && role != "managed_policy") {
		return ""
	}
	entries := make([]WeeklyConfigurationEntry, 0, len(c))
	for _, entry := range c {
		if entry.Role == role {
			entries = append(entries, entry)
		}
	}
	return productDigest(struct {
		SchemaVersion int                        `json:"schema_version"`
		Role          string                     `json:"role"`
		Entries       []WeeklyConfigurationEntry `json:"entries"`
	}{1, role, entries})
}
