package setup

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installer"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

// CodeCommandConsent discloses only the original fixed local provider. The host
// digest narrows host consent; it is neither source authority nor registration.
type CodeCommandConsent struct {
	Kind               string `json:"kind"`
	PluginID           string `json:"plugin_id"`
	Command            string `json:"command"`
	Mode               string `json:"mode"`
	CatalogRevision    string `json:"catalog_revision"`
	SHA256             string `json:"sha256"`
	ProviderRoot       string `json:"provider_root"`
	ProviderTreeSHA256 string `json:"provider_tree_sha256"`
	LauncherPath       string `json:"launcher_path"`
	LauncherSHA256     string `json:"launcher_sha256"`
	LauncherScript     string `json:"launcher_script"`
}

func reviewCodeCommandConsent(r installer.ClientRecord) (*CodeCommandConsent, error) {
	if r.Materialization.Selection == nil {
		return nil, nil
	}
	root := installer.ClientMarketplaceRoot(r)
	if len(r.CarrierID) != 32 || strings.Trim(r.CarrierID, "0123456789abcdef") != "" || r.Code.Marketplace != "maestro-local-"+r.CarrierID || r.Code.PluginID != "maestro-product@"+r.Code.Marketplace || r.Materialization.CodeProviderRoot != root || !filepath.IsAbs(root) || filepath.Clean(root) != root {
		return nil, errors.New("fixed Code command-source binding unavailable")
	}
	current, err := installer.ClientTreeDigest(r.Materialization.CodeRoot)
	if err != nil || current != r.Materialization.CodeTreeSHA256 {
		return nil, errors.New("recorded current Code source changed before command consent")
	}
	if err := checkCodeProviderDigest(r); err != nil {
		return nil, err
	}
	if err := scheduler.ValidatePrivateDirectory(root); err != nil {
		return nil, err
	}
	name, command, err := commandSourceLauncher(root, runtime.GOOS)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 2 {
		return nil, errors.New("Code provider contains entries outside the fixed command source")
	}
	meta := filepath.Join(root, ".claude-plugin")
	if err := scheduler.ValidatePrivateDirectory(meta); err != nil {
		return nil, err
	}
	entries, err = os.ReadDir(meta)
	if err != nil || len(entries) != 1 || entries[0].Name() != "marketplace.json" {
		return nil, errors.New("Code provider catalog selection changed")
	}
	catalog, err := readCodeConsentFile(filepath.Join(meta, "marketplace.json"), 0600)
	if err != nil {
		return nil, err
	}
	installerName := "maestro-install"
	if runtime.GOOS == "windows" {
		installerName += ".exe"
	}
	generatedRoot, generated, generatedName, err := commandSourceFiles(installer.ClientOptions{SeededOperationOptions: installer.SeededOperationOptions{Self: filepath.Join(r.Binding.ManagedRoot, installerName), ManagedRoot: r.Binding.ManagedRoot, UserHome: r.Binding.UserHome}, Request: installer.ClientRequest{CodeRoot: r.Materialization.Selection.CodeRoot, CarrierID: r.CarrierID, Workspace: r.Binding.Workspace}}, runtime.GOOS)
	if err != nil || generatedRoot != root || generatedName != name || !bytes.Equal(catalog, generated[".claude-plugin/marketplace.json"]) {
		return nil, errors.New("Code provider is not the exact generated command catalog")
	}
	mode := os.FileMode(0700)
	if runtime.GOOS == "windows" {
		mode = 0600
	}
	launcher := filepath.Join(root, name)
	script, err := readCodeConsentFile(launcher, mode)
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(script, generated[name]) {
		return nil, errors.New("Code provider launcher is not the exact generated installed-client command")
	}
	if err := checkCodeProviderDigest(r); err != nil {
		return nil, err
	}
	catalogRevision, commandSHA, err := codeCommandHostDigests(catalog, r.Code.PluginID, command)
	if err != nil {
		return nil, err
	}
	return &CodeCommandConsent{Kind: "command_source", PluginID: r.Code.PluginID, Command: command, Mode: "copy", CatalogRevision: catalogRevision, SHA256: commandSHA, ProviderRoot: root, ProviderTreeSHA256: r.Materialization.CodeProviderSHA256, LauncherPath: launcher, LauncherSHA256: fmt.Sprintf("%x", sha256.Sum256(script)), LauncherScript: string(script)}, nil
}

func checkCodeProviderDigest(r installer.ClientRecord) error {
	digest, err := installer.ClientTreeDigest(r.Materialization.CodeProviderRoot)
	if err != nil || digest != r.Materialization.CodeProviderSHA256 || len(digest) != 64 {
		return errors.New("reviewed Code command provider changed; preserve the provider and review again")
	}
	return nil
}

func readCodeConsentFile(path string, mode os.FileMode) ([]byte, error) {
	file, err := scheduler.OpenRegularFile(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	if err := scheduler.ValidatePrivateFile(file, mode); err != nil {
		return nil, err
	}
	before, err := file.Stat()
	if err != nil || before.Size() <= 0 || before.Size() > 64<<10 {
		return nil, errors.New("Code consent file exceeds its fixed role or bound")
	}
	body, err := io.ReadAll(io.LimitReader(file, (64<<10)+1))
	after, statErr := file.Stat()
	if err != nil || statErr != nil {
		return nil, errors.Join(err, statErr)
	}
	if int64(len(body)) != before.Size() || !os.SameFile(before, after) || before.Size() != after.Size() || before.Mode() != after.Mode() || !before.ModTime().Equal(after.ModTime()) {
		return nil, errors.New("Code consent file changed while reading")
	}
	if err := scheduler.ValidatePrivateFile(file, mode); err != nil {
		return nil, err
	}
	return body, nil
}

// This is the observed public Claude Code 2.1.319 command-consent protocol:
// catalog = SHA256(JSON.stringify([F(catalogJSON), null])), where F encodes
// object keys in lexical order as nested arrays; no package.json is admitted.
// command = SHA256(JSON.stringify([kind, pluginID, command, mode, revision])).
// The unchanged host independently checks this narrow digest before executing.
// Generated catalog keys/values and launcher commands are printable ASCII, so
// JSON UTF-16 sorting/number-format differences cannot enter this finite input.
func codeCommandHostDigests(catalog []byte, pluginID, command string) (string, string, error) {
	var parsed any
	if err := json.Unmarshal(catalog, &parsed); err != nil {
		return "", "", err
	}
	canonical, err := codeHostJSON([]any{codeHostCatalogValue(parsed), nil})
	if err != nil {
		return "", "", err
	}
	revision := fmt.Sprintf("sha256:%x", sha256.Sum256(canonical))
	frame, err := codeHostJSON([]string{"command_source", pluginID, command, "copy", revision})
	if err != nil {
		return "", "", err
	}
	return revision, fmt.Sprintf("%x", sha256.Sum256(frame)), nil
}

func codeHostCatalogValue(value any) any {
	switch value := value.(type) {
	case []any:
		out := []any{"["}
		for _, child := range value {
			out = append(out, codeHostCatalogValue(child))
		}
		return out
	case map[string]any:
		keys := make([]string, 0, len(value))
		for key := range value {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		out := []any{"{"}
		for _, key := range keys {
			out = append(out, []any{key, codeHostCatalogValue(value[key])})
		}
		return out
	default:
		return value
	}
}

func codeHostJSON(value any) ([]byte, error) {
	var body bytes.Buffer
	encoder := json.NewEncoder(&body)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(body.Bytes(), []byte{'\n'}), nil
}

func checkReviewedCodeConsent(r installer.ClientRecord, expected *CodeCommandConsent) error {
	if expected == nil {
		return nil
	}
	current, err := reviewCodeCommandConsent(r)
	if err != nil {
		return err
	}
	if current == nil || *current != *expected {
		return errors.New("reviewed Code command consent changed")
	}
	return nil
}

func containsCodeOption(body []byte, option string) bool {
	for _, word := range strings.Fields(string(body)) {
		if word == option {
			return true
		}
	}
	return false
}

func codeMutationError(body []byte, cause error) error {
	// The host can precede its terminal JSON with human disclosure. Retain only
	// a closed refusal class, never the message, command text or arbitrary body.
	if len(body) <= 1<<20 {
		lines := bytes.Split(bytes.TrimSpace(body), []byte{'\n'})
		if len(lines) > 0 {
			var result struct {
				Outcome     string `json:"outcome"`
				FailureCode string `json:"failureCode"`
			}
			if json.Unmarshal(lines[len(lines)-1], &result) == nil && result.Outcome == "failed" {
				switch result.FailureCode {
				case "command_source_refused", "command_source_declined", "entry_helper_unconfirmed", "entry_helper_declined", "marketplace_policy_blocked", "plugin_policy_blocked":
					return fmt.Errorf("Code registration remains pending (%s); preserve current client state and review the command in your own terminal: %w", result.FailureCode, cause)
				}
			}
		}
	}
	return fmt.Errorf("Code command incomplete; reconcile actual lists: %w", cause)
}
