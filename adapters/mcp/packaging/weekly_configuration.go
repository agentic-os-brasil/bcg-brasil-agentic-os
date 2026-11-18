package packaging

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/maintenance"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

var errWeeklyConfiguration = errors.New("weekly declared configuration unavailable or incompatible")

// VerifyWeeklyConfiguration measures only explicitly accepted sources; inventory
// completeness and native qualification remain with the owning registration.
func VerifyWeeklyConfiguration(ctx context.Context, configuration maintenance.WeeklyConfiguration) error {
	if ctx == nil || configuration.Validate() != nil {
		return errWeeklyConfiguration
	}
	var total int64
	for _, entry := range configuration {
		if err := ctx.Err(); err != nil {
			return err
		}
		n, err := verifyWeeklyConfigurationEntry(ctx, entry, (256<<10)-total)
		if err != nil {
			return err
		}
		total += n
		if total > 256<<10 {
			return errWeeklyConfiguration
		}
	}
	return ctx.Err()
}

func verifyWeeklyConfigurationEntry(ctx context.Context, entry maintenance.WeeklyConfigurationEntry, remaining int64) (size int64, err error) {
	f, openErr := scheduler.OpenRegularFile(entry.Path)
	if entry.Absent {
		if f != nil {
			return 0, errors.Join(errWeeklyConfiguration, f.Close())
		}
		if errors.Is(openErr, os.ErrNotExist) {
			return 0, ctx.Err()
		}
		return 0, errWeeklyConfiguration
	}
	if openErr != nil {
		return 0, errWeeklyConfiguration
	}
	defer func() { err = errors.Join(err, f.Close()) }()
	before, statErr := f.Stat()
	if statErr != nil || before.Size() > 64<<10 || before.Size() > remaining || scheduler.ValidateSingleLink(f) != nil {
		return 0, errWeeklyConfiguration
	}
	path, pathErr := weeklyIdentityCanonicalPath(f)
	if pathErr != nil || !weeklyIdentityPathsMatch(entry.Path, path) {
		return 0, errWeeklyConfiguration
	}
	body, readErr := io.ReadAll(io.LimitReader(f, (64<<10)+1))
	if readErr != nil || len(body) > 64<<10 || int64(len(body)) != before.Size() {
		return 0, errWeeklyConfiguration
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	after, statErr := f.Stat()
	if statErr != nil || !sameHostMetadata(before, after) {
		return 0, errWeeklyConfiguration
	}
	current, openErr := scheduler.OpenRegularFile(entry.Path)
	if openErr != nil {
		return 0, errWeeklyConfiguration
	}
	currentInfo, statErr := current.Stat()
	closeErr := current.Close()
	if statErr != nil || closeErr != nil || !os.SameFile(before, currentInfo) || !sameHostMetadata(before, currentInfo) {
		return 0, errWeeklyConfiguration
	}
	digest := sha256.Sum256(body)
	if hex.EncodeToString(digest[:]) != entry.SHA256 {
		return 0, errWeeklyConfiguration
	}
	var settings map[string]json.RawMessage
	if releasecontract.DecodeStrictJSON(body, &settings) != nil || settings == nil {
		return 0, errWeeklyConfiguration
	}
	// These settings can execute code, add tools/delegation or replace the closed
	// process environment; unrelated harmless vendor settings remain admissible.
	for _, key := range []string{"hooks", "enabledPlugins", "extraKnownMarketplaces", "pluginConfigs", "mcpServers", "enableAllProjectMcpServers", "enabledMcpjsonServers", "allowedMcpServers", "agents", "agent", "env", "apiKeyHelper", "awsAuthRefresh", "awsCredentialExport", "otelHeadersHelper", "statusLine", "fileSuggestion"} {
		if _, exists := settings[key]; exists {
			return 0, errWeeklyConfiguration
		}
	}
	return int64(len(body)), ctx.Err()
}
