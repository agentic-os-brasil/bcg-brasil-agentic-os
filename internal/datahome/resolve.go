// Package datahome resolves owner state read-only and explicitly bootstraps fresh homes.
package datahome

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

type Options struct {
	UserHome    string
	ProductRoot string
	SourceRoot  string
	LookupEnv   func(string) (string, bool)
}

type Result struct {
	Home       string `json:"home"`
	BrainRoot  string `json:"brain_root"`
	SelectedBy string `json:"selected_by"`
	Origin     string `json:"origin"`
	Writable   bool   `json:"writable"`
	Diagnostic string `json:"diagnostic"`
}

func Resolve(options Options) (Result, error) {
	var result Result
	for name, path := range map[string]string{"user-home": options.UserHome, "product-root": options.ProductRoot} {
		if err := existingDirectory(path); err != nil {
			return failed(result, name+"-invalid", err)
		}
	}
	if err := validateOwnership(options.UserHome); err != nil {
		return failed(result, "user-home-invalid", err)
	}
	if options.SourceRoot != "" {
		if err := existingDirectory(options.SourceRoot); err != nil {
			return failed(result, "source-root-invalid", err)
		}
	}
	lookup := options.LookupEnv
	if lookup == nil {
		lookup = os.LookupEnv
	}
	defaultHome := filepath.Join(options.UserHome, ".maestro")
	result = Result{Home: defaultHome, SelectedBy: "default", Origin: "fresh-unconfirmed", Diagnostic: "fresh-unconfirmed"}
	if value, present := lookup("MAESTRO_HOME"); present {
		result.Home, result.SelectedBy = value, "env"
	}
	marker, err := readRecord(filepath.Join(options.UserHome, ".maestro-home.json"), "home", "attempt_id")
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return failed(result, "marker-invalid", err)
	}
	if marker != nil {
		if !validAttempt(marker["attempt_id"], true) {
			return failed(result, "marker-invalid", errors.New("invalid attempt_id"))
		}
		if err := existingDirectory(marker["home"]); err != nil {
			code := "marker-invalid"
			if errors.Is(err, os.ErrNotExist) {
				code = "marker-target-missing"
			}
			return failed(result, code, err)
		}
		if result.SelectedBy == "env" && !samePath(result.Home, marker["home"]) {
			return failed(result, "marker-conflict", errors.New("MAESTRO_HOME conflicts with locator"))
		}
		if result.SelectedBy != "env" {
			result.Home, result.SelectedBy = marker["home"], "marker"
		}
	}
	result.BrainRoot = filepath.Join(result.Home, "brain")
	if err := absolutePath(result.Home); err != nil {
		return failed(result, "home-invalid", err)
	}
	for _, other := range []string{options.ProductRoot, options.SourceRoot} {
		if other != "" && (contains(result.Home, other) || contains(other, result.Home)) {
			return failed(result, "home-overlap", errors.New("home overlaps product or source"))
		}
	}
	if err := scheduler.ValidatePrivateDirectory(result.Home); err != nil {
		if errors.Is(err, os.ErrNotExist) && marker == nil {
			return result, nil
		}
		return failed(result, "home-invalid", err)
	}
	if err := validateOwnership(result.Home); err != nil {
		return failed(result, "home-invalid", err)
	}
	origin, err := readRecord(filepath.Join(result.BrainRoot, ".maestro", "origin.json"), "home", "origin", "attempt_id", "plan_sha256")
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			entries, readErr := os.ReadDir(result.Home)
			if readErr != nil {
				return failed(result, "home-inaccessible", readErr)
			}
			if len(entries) == 0 && marker == nil {
				return result, nil
			}
			return failed(result, "origin-unconfirmed", errors.New("home contains data or a locator but has no origin"))
		}
		return failed(result, "origin-invalid", err)
	}
	if !samePath(origin["home"], result.Home) {
		return failed(result, "origin-conflict", errors.New("origin home differs from selected home"))
	}
	result.Origin = origin["origin"]
	switch result.Origin {
	case "fresh-confirmed":
		if origin["attempt_id"] != "" || origin["plan_sha256"] != "" {
			return failed(result, "origin-invalid", errors.New("fresh origin contains migration fields"))
		}
	case "migrated":
		if !validAttempt(origin["attempt_id"], false) || !digestPattern.MatchString(origin["plan_sha256"]) {
			return failed(result, "origin-invalid", errors.New("migration origin requires attempt and plan digest"))
		}
	default:
		return failed(result, "origin-invalid", errors.New("unsupported origin"))
	}
	if marker != nil && marker["attempt_id"] != origin["attempt_id"] {
		return failed(result, "origin-conflict", errors.New("locator attempt differs from origin"))
	}
	if marker == nil && !samePath(result.Home, defaultHome) {
		return failed(result, "marker-required", errors.New("nondefault home requires locator"))
	}
	if result.Origin == "migrated" {
		result.Diagnostic = "migration-unverified"
		return result, nil
	}
	if !ownershipVerified() {
		result.Diagnostic = "ownership-unverified"
		return result, nil
	}
	result.Writable, result.Diagnostic = true, "ready"
	return result, nil
}

var attemptPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)
var digestPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

func validAttempt(value string, empty bool) bool {
	return empty && value == "" || attemptPattern.MatchString(value)
}

func failed(result Result, code string, err error) (Result, error) {
	result.Writable = false
	result.Diagnostic = code
	return result, fmt.Errorf("%s: %w", code, err)
}

func absolutePath(path string) error {
	if path == "" || !filepath.IsAbs(path) || strings.ContainsAny(path, "\x00\r\n") || filepath.Clean(path) != path {
		return errors.New("path must be clean and absolute")
	}
	return nil
}

func existingDirectory(path string) error {
	if err := absolutePath(path); err != nil {
		return err
	}
	return scheduler.ValidateDirectory(path)
}

func samePath(a, b string) bool {
	if a == b {
		return true
	}
	if existingDirectory(a) != nil || existingDirectory(b) != nil {
		return false
	}
	aInfo, aErr := os.Stat(a)
	bInfo, bErr := os.Stat(b)
	return aErr == nil && bErr == nil && os.SameFile(aInfo, bInfo)
}

func contains(parent, child string) bool {
	parent, child = strings.ToLower(parent), strings.ToLower(child)
	rel, err := filepath.Rel(parent, child)
	return err == nil && (rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))))
}

func readRecord(path string, fields ...string) (map[string]string, error) {
	body, err := scheduler.ReadPrivateFile(path, 16<<10)
	if err != nil {
		return nil, err
	}
	if err := validateOwnership(path); err != nil {
		return nil, err
	}
	if !utf8.Valid(body) {
		return nil, errors.New("record must be UTF-8")
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return nil, errors.New("record must be a JSON object")
	}
	allowed := map[string]bool{"schema_version": true}
	for _, field := range fields {
		allowed[field] = true
	}
	values := map[string]json.RawMessage{}
	for decoder.More() {
		token, err = decoder.Token()
		if err != nil {
			return nil, err
		}
		key, ok := token.(string)
		if !ok || !allowed[key] {
			return nil, errors.New("unknown record field")
		}
		if _, exists := values[key]; exists {
			return nil, fmt.Errorf("duplicate field %s", key)
		}
		var value json.RawMessage
		if err = decoder.Decode(&value); err != nil {
			return nil, err
		}
		values[key] = value
	}
	if _, err = decoder.Token(); err != nil {
		return nil, err
	}
	if _, err = decoder.Token(); err != io.EOF {
		return nil, errors.New("trailing record content")
	}
	if len(values) != len(allowed) {
		return nil, errors.New("record is missing required fields")
	}
	var version int
	if err = json.Unmarshal(values["schema_version"], &version); err != nil || version != 1 {
		return nil, errors.New("unsupported schema_version")
	}
	result := map[string]string{}
	for _, field := range fields {
		if bytes.Equal(values[field], []byte("null")) {
			return nil, fmt.Errorf("%s must be a string", field)
		}
		var value string
		if err = json.Unmarshal(values[field], &value); err != nil {
			return nil, err
		}
		result[field] = value
	}
	return result, nil
}
